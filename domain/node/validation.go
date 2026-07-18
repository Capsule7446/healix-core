package node

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/interpolation"
)

// ValidationAssertion is the execution-side representation of one
// framework-neutral verification statement.  Workspace maps its persisted
// value object here at the materialization boundary, keeping workspace assets
// independent from the execution context.
type ValidationAssertion struct {
	Kind           string
	Expected       string
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
}

// ValidationObservation is an append-only execution fact. It contains only
// framework-neutral assertion data; infrastructure maps it to a concrete
// StepExecution and retained event-timeline location. Final is deliberately explicit:
// the last record is retained even when it is identical to the prior poll.
type ValidationObservation struct {
	NodeID       string
	GroupID      string
	BranchID     string
	Assertion    ValidationAssertion
	Actual       string
	Passed       bool
	Reason       string
	Selector     fingerprint.Selector
	ObservedAtMS int64
	Final        bool
}

// ValidationStateReader is an optional capability of an Element.  Existing
// action-only drivers remain source-compatible; verification-capable drivers
// provide one standard DOM/ARIA projection without leaking framework classes
// into the domain.
type ValidationStateReader interface {
	ValidationState(context.Context) (ValidationState, error)
}

type ValidationState struct {
	Value          string
	Enabled        bool
	Checked        bool
	Mixed          bool
	Selected       bool
	Pressed        bool
	SelectedTexts  []string
	SelectedValues []string
}

const validationPollInterval = 200 * time.Millisecond

type ValidationNode struct {
	NodeID string
	// GroupID and BranchID are empty for an independent validation. They are
	// execution identities, not a persisted expression model, and let the
	// evidence adapter attach member observations to their group StepExecution.
	GroupID   string
	BranchID  string
	Target    fingerprint.NodeSpec
	Assertion ValidationAssertion
	MaxWait   time.Duration
	Stability time.Duration
}

func (v *ValidationNode) ID() string { return v.NodeID }

func (v *ValidationNode) Run(ctx context.Context, rt *Runtime) error {
	if err := rt.waitBeforeStep(ctx); err != nil {
		return fmt.Errorf("validation %s: wait step interval: %w", v.NodeID, err)
	}
	execution := NewStepExecution(v.NodeID)
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseRunning); err != nil {
		return err
	}
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseValidating); err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	if err := v.waitStable(ctx, rt); err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseSucceeded); err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	return nil
}

func (v *ValidationNode) waitStable(parent context.Context, rt *Runtime) error {
	maxWait := v.MaxWait
	if maxWait <= 0 {
		maxWait = 10 * time.Second
	}
	stability := v.Stability
	if stability <= 0 {
		stability = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(parent, maxWait)
	defer cancel()
	observations := newValidationObservationRecorder()
	var stableSince time.Time
	var lastActual string
	for {
		ok, actual, err := v.evaluate(ctx, rt)
		if err != nil {
			if recordErr := observations.record(ctx, rt, v, false, actual, "system_error", true); recordErr != nil {
				return recordErr
			}
			return fmt.Errorf("validation %s: %w", v.NodeID, err)
		}
		lastActual = actual
		if err := observations.record(ctx, rt, v, ok, actual, validationReason(ok), false); err != nil {
			return err
		}
		if ok {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= stability {
				if err := observations.record(ctx, rt, v, true, actual, "passed", true); err != nil {
					return err
				}
				return nil
			}
		} else {
			stableSince = time.Time{}
		}
		select {
		case <-time.After(validationPollInterval):
		case <-ctx.Done():
			if err := observations.record(ctx, rt, v, false, lastActual, "timeout", true); err != nil {
				return err
			}
			return fmt.Errorf("assertion was not continuously satisfied within %s (last actual %q): %w", maxWait, lastActual, ctx.Err())
		}
	}
}

// evaluate performs exactly one read/check round.  ValidationGroupNode calls
// this for every member before it derives a branch result, preserving the
// "same-round AND" invariant.
func (v *ValidationNode) evaluate(ctx context.Context, rt *Runtime) (bool, string, error) {
	assertion, err := v.resolvedAssertion(rt)
	if err != nil {
		return false, "", err
	}
	el, absent, err := v.locate(ctx, rt)
	if err != nil {
		return false, "", fmt.Errorf("locate: %w", err)
	}
	if absent {
		if assertion.Kind == "not_exists" {
			return true, "<absent>", nil
		}
		// Deletion is deliberately not "not visible"; callers must assert
		// not_exists for that semantic.
		return false, "<absent>", nil
	}
	exists, err := el.Exists(ctx)
	if err != nil {
		if errors.Is(err, ErrElementNotFound) {
			return assertion.Kind == "not_exists", "<absent>", nil
		}
		return false, "", err
	}
	if !exists {
		return assertion.Kind == "not_exists", "<absent>", nil
	}
	if assertion.Kind == "exists" {
		return true, "<present>", nil
	}
	if assertion.Kind == "not_exists" {
		return false, "<present>", nil
	}
	visible, err := el.Visible(ctx)
	if err != nil {
		return false, "", err
	}
	switch assertion.Kind {
	case "visible":
		return visible, fmt.Sprint(visible), nil
	case "not_visible":
		return !visible, fmt.Sprint(visible), nil
	}

	if assertion.Kind == "text_equals" || assertion.Kind == "text_contains" || assertion.Kind == "text_matches" {
		text, err := el.Text(ctx)
		if err != nil {
			return false, "", err
		}
		return compareText(assertion, text)
	}
	if assertion.Kind == "attribute_equals" || assertion.Kind == "attribute_contains" {
		value, present, err := el.Attribute(ctx, assertion.Attribute)
		if err != nil {
			return false, "", err
		}
		if !present {
			return false, "<undefined>", nil
		}
		return compareScalar(assertion, value, false)
	}

	reader, ok := el.(ValidationStateReader)
	if !ok {
		return false, "", fmt.Errorf("driver element does not provide validation state for %q", assertion.Kind)
	}
	state, err := reader.ValidationState(ctx)
	if err != nil {
		return false, "", err
	}
	switch assertion.Kind {
	case "value_equals", "value_contains", "value_matches", "value_not_empty":
		if assertion.Kind == "value_not_empty" {
			return state.Value != "", state.Value, nil
		}
		return compareScalar(assertion, state.Value, false)
	case "enabled":
		return state.Enabled, fmt.Sprint(state.Enabled), nil
	case "disabled":
		return !state.Enabled, fmt.Sprint(state.Enabled), nil
	case "checked":
		return state.Checked && !state.Mixed, fmt.Sprint(state.Checked), nil
	case "unchecked":
		return !state.Checked && !state.Mixed, fmt.Sprint(state.Checked), nil
	case "mixed":
		return state.Mixed, fmt.Sprint(state.Mixed), nil
	case "selected":
		return state.Selected, fmt.Sprint(state.Selected), nil
	case "unselected":
		return !state.Selected, fmt.Sprint(state.Selected), nil
	case "pressed":
		return state.Pressed, fmt.Sprint(state.Pressed), nil
	case "unpressed":
		return !state.Pressed, fmt.Sprint(state.Pressed), nil
	case "selected_text_equals", "selected_text_contains":
		value := firstValue(state.SelectedTexts)
		return compareScalar(assertion, value, false)
	case "selected_value_equals", "selected_value_contains":
		value := firstValue(state.SelectedValues)
		return compareScalar(assertion, value, false)
	case "selected_set_equals", "selected_set_contains":
		return compareSet(assertion, state.SelectedTexts)
	default:
		return false, "", fmt.Errorf("unsupported validation assertion %q", assertion.Kind)
	}
}

// resolvedAssertion expands only values permitted by the persisted assertion
// contract. Attribute names are deliberately static: allowing interpolation in
// a DOM attribute name makes validation shape data-dependent and is rejected
// during workspace validation.
func (v *ValidationNode) resolvedAssertion(rt *Runtime) (ValidationAssertion, error) {
	resolved := v.Assertion
	resolved.ExpectedValues = append([]string(nil), v.Assertion.ExpectedValues...)
	variables := runtimeVariables{rt: rt}
	expected, err := interpolation.Expand(resolved.Expected, variables)
	if err != nil {
		return ValidationAssertion{}, fmt.Errorf("validation %s expected: %w", v.NodeID, err)
	}
	resolved.Expected = expected
	for index, value := range resolved.ExpectedValues {
		expanded, err := interpolation.Expand(value, variables)
		if err != nil {
			return ValidationAssertion{}, fmt.Errorf("validation %s expected_values[%d]: %w", v.NodeID, index, err)
		}
		resolved.ExpectedValues[index] = expanded
	}
	return resolved, nil
}

// locate applies the same deterministic healing decision as action steps.
// For a not_exists assertion, an applicable healed candidate is evidence that
// the element still exists and must block a false positive; only a genuine
// no_candidate outcome is treated as absence.
func (v *ValidationNode) locate(ctx context.Context, rt *Runtime) (Element, bool, error) {
	target := rt.effectiveSpec(v.Target)
	el, err := rt.Driver.Locate(ctx, target)
	if err == nil {
		return el, false, nil
	}
	if !errors.Is(err, ErrElementNotFound) {
		return nil, false, err
	}
	if rt.Healer == nil {
		return nil, true, nil
	}
	snapshot, err := rt.Driver.Snapshot(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("snapshot for healing: %w", err)
	}
	decision, err := rt.Healer.Heal(ctx, target, snapshot)
	if err != nil {
		return nil, false, err
	}
	if err := decision.Validate(); err != nil {
		return nil, false, fmt.Errorf("invalid heal decision: %w", err)
	}
	if rt.Facts != nil {
		oldSelector := fingerprint.Selector{}
		if len(target.Selectors) > 0 {
			oldSelector = target.Selectors[0]
		}
		if err := rt.Facts.RecordHealDecision(ctx, rt.RunID, v.NodeID, target.ID, oldSelector, decision); err != nil {
			return nil, false, fmt.Errorf("record heal decision: %w", err)
		}
	}
	if decision.Outcome == heal.OutcomeNoCandidate || decision.Best == nil {
		return nil, true, nil
	}
	healed := target
	healed.Selectors = append([]fingerprint.Selector{decision.Best.Selector}, healed.Selectors...)
	el, err = rt.Driver.Locate(ctx, healed)
	if err != nil {
		return nil, false, fmt.Errorf("re-locate after heal: %w", err)
	}
	rt.setSelectorOverlay(healed)
	return el, false, nil
}

func compareText(assertion ValidationAssertion, actual string) (bool, string, error) {
	if assertion.Kind == "text_matches" {
		re, err := regexp.Compile(assertion.Expected)
		if err != nil {
			return false, actual, err
		}
		return re.MatchString(actual), actual, nil
	}
	return compareScalar(assertion, actual, true)
}

func compareScalar(assertion ValidationAssertion, actual string, normalizeWhitespace bool) (bool, string, error) {
	expected := assertion.Expected
	if normalizeWhitespace {
		actual, expected = normalizeValidationText(actual), normalizeValidationText(expected)
	}
	if assertion.IgnoreCase {
		actual, expected = strings.ToLower(actual), strings.ToLower(expected)
	}
	switch assertion.Kind {
	case "text_equals", "value_equals", "selected_text_equals", "selected_value_equals", "attribute_equals":
		return actual == expected, actual, nil
	case "text_contains", "value_contains", "selected_text_contains", "selected_value_contains", "attribute_contains":
		return strings.Contains(actual, expected), actual, nil
	case "value_matches":
		re, err := regexp.Compile(expected)
		if err != nil {
			return false, actual, err
		}
		return re.MatchString(actual), actual, nil
	default:
		return false, actual, fmt.Errorf("unsupported scalar validation %q", assertion.Kind)
	}
}

func compareSet(assertion ValidationAssertion, actual []string) (bool, string, error) {
	expected := append([]string(nil), assertion.ExpectedValues...)
	actual = append([]string(nil), actual...)
	sort.Strings(expected)
	sort.Strings(actual)
	if assertion.Kind == "selected_set_equals" {
		return strings.Join(expected, "\x1f") == strings.Join(actual, "\x1f"), strings.Join(actual, ", "), nil
	}
	all := make(map[string]bool, len(actual))
	for _, value := range actual {
		all[value] = true
	}
	for _, value := range expected {
		if !all[value] {
			return false, strings.Join(actual, ", "), nil
		}
	}
	return true, strings.Join(actual, ", "), nil
}

func normalizeValidationText(value string) string { return strings.Join(strings.Fields(value), " ") }
func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

type ValidationBranch struct {
	ID    string
	Nodes []*ValidationNode
}

type ValidationGroupNode struct {
	NodeID    string
	Branches  []ValidationBranch
	MaxWait   time.Duration
	Stability time.Duration
}

func (g *ValidationGroupNode) ID() string { return g.NodeID }

func (g *ValidationGroupNode) Run(ctx context.Context, rt *Runtime) error {
	if err := rt.waitBeforeStep(ctx); err != nil {
		return fmt.Errorf("validation group %s: wait step interval: %w", g.NodeID, err)
	}
	execution := NewStepExecution(g.NodeID)
	if err := transitionValidation(ctx, rt, execution, g.NodeID, PhaseRunning); err != nil {
		return err
	}
	if err := transitionValidation(ctx, rt, execution, g.NodeID, PhaseValidating); err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	if err := g.waitStable(ctx, rt); err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	if err := transitionValidation(ctx, rt, execution, g.NodeID, PhaseSucceeded); err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	return nil
}

func (g *ValidationGroupNode) waitStable(parent context.Context, rt *Runtime) error {
	maxWait, stability := g.MaxWait, g.Stability
	if maxWait <= 0 {
		maxWait = 10 * time.Second
	}
	if stability <= 0 {
		stability = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(parent, maxWait)
	defer cancel()
	observations := newValidationObservationRecorder()
	stableSince := make([]time.Time, len(g.Branches))
	last := make(map[string]validationObservationState)
	for {
		// This loop intentionally evaluates every member in every branch before
		// selecting a branch.  No member result is latched across rounds.
		branchPassed := make([]bool, len(g.Branches))
		for i, branch := range g.Branches {
			branchPassed[i] = len(branch.Nodes) > 0
			for _, member := range branch.Nodes {
				ok, actual, err := member.evaluate(ctx, rt)
				if err != nil {
					if recordErr := observations.record(ctx, rt, member, false, actual, "system_error", true); recordErr != nil {
						return recordErr
					}
					return fmt.Errorf("branch %s: %w", branch.ID, err)
				}
				last[member.NodeID] = validationObservationState{passed: ok, actual: actual, reason: validationReason(ok)}
				if err := observations.record(ctx, rt, member, ok, actual, validationReason(ok), false); err != nil {
					return err
				}
				if !ok {
					branchPassed[i] = false
				}
			}
		}
		now := time.Now()
		for i, passed := range branchPassed {
			if !passed {
				stableSince[i] = time.Time{}
				continue
			}
			if stableSince[i].IsZero() {
				stableSince[i] = now
			}
			if now.Sub(stableSince[i]) >= stability {
				for _, branch := range g.Branches {
					for _, member := range branch.Nodes {
						state := last[member.NodeID]
						reason := state.reason
						if state.passed {
							reason = "passed"
						}
						if err := observations.record(ctx, rt, member, state.passed, state.actual, reason, true); err != nil {
							return err
						}
					}
				}
				return nil
			}
		}
		select {
		case <-time.After(validationPollInterval):
		case <-ctx.Done():
			for _, branch := range g.Branches {
				for _, member := range branch.Nodes {
					state := last[member.NodeID]
					if err := observations.record(ctx, rt, member, state.passed, state.actual, "timeout", true); err != nil {
						return err
					}
				}
			}
			return fmt.Errorf("no validation branch was continuously satisfied within %s: %w", maxWait, ctx.Err())
		}
	}
}

type validationObservationState struct {
	passed bool
	actual string
	reason string
}

type validationObservationRecorder struct {
	last map[string]validationObservationState
}

func newValidationObservationRecorder() *validationObservationRecorder {
	return &validationObservationRecorder{last: make(map[string]validationObservationState)}
}

func (r *validationObservationRecorder) record(ctx context.Context, rt *Runtime, validation *ValidationNode,
	passed bool, actual, reason string, final bool) error {
	if rt.Facts == nil {
		return nil
	}
	state := validationObservationState{passed: passed, actual: actual, reason: reason}
	previous, seen := r.last[validation.NodeID]
	if !final && seen && previous == state {
		return nil
	}
	r.last[validation.NodeID] = state
	spec := rt.effectiveSpec(validation.Target)
	selector := fingerprint.Selector{}
	if len(spec.Selectors) > 0 {
		selector = spec.Selectors[0]
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	assertion := validation.Assertion
	if validationEvidenceIsSensitive(validation.Target, assertion) {
		actual = "••••••••"
		if assertion.Expected != "" {
			assertion.Expected = "••••••••"
		}
		assertion.ExpectedValues = nil
	}
	return rt.Facts.RecordValidationObservation(cleanupCtx, rt.RunID, ValidationObservation{
		NodeID: validation.NodeID, GroupID: validation.GroupID, BranchID: validation.BranchID,
		Assertion: assertion, Actual: actual, Passed: passed, Reason: reason,
		Selector: selector, ObservedAtMS: time.Now().UnixMilli(), Final: final,
	})
}

func validationEvidenceIsSensitive(target fingerprint.NodeSpec, assertion ValidationAssertion) bool {
	if !strings.HasPrefix(assertion.Kind, "value_") && assertion.Kind != "attribute_equals" && assertion.Kind != "attribute_contains" {
		return false
	}
	attributes := target.Fingerprint.Attributes
	for _, value := range []string{attributes["type"], attributes["name"], attributes["id"], attributes["autocomplete"], target.Fingerprint.ARIA.Name} {
		value = strings.ToLower(value)
		if value == "password" || value == "file" || strings.Contains(value, "token") || strings.Contains(value, "secret") || strings.Contains(value, "password") || strings.Contains(value, "api_key") {
			return true
		}
	}
	return false
}

func validationReason(passed bool) string {
	if passed {
		return "satisfied"
	}
	return "normal_unsatisfied"
}

func transitionValidation(ctx context.Context, rt *Runtime, execution *StepExecution, nodeID string, next Phase) error {
	if err := execution.CanTransition(next); err != nil {
		return err
	}
	if err := rt.emit(ctx, nodeID, next); err != nil {
		return err
	}
	return execution.Transition(next)
}

func validationFail(ctx context.Context, rt *Runtime, execution *StepExecution, nodeID string, cause error) error {
	phase := failurePhase(ctx)
	if err := execution.CanTransition(phase); err != nil {
		return errors.Join(cause, err)
	}
	if err := rt.emitTerminal(ctx, nodeID, phase); err != nil {
		return errors.Join(cause, err)
	}
	if err := execution.Transition(phase); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
