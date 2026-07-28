package node

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/interpolation"
)

// ValidationAssertion 是一个与框架无关的验证语句的执行端表示。  工作区将其持久值对象映射到物化边界，使工作区资产独立于执行上下文。
type ValidationAssertion struct {
	Kind           string
	Expected       string
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
}

// ValidationObservation 是一个仅附加执行事实。它仅包含框架中立的断言数据；基础设施将其映射到具体的 StepExecution 并保留事件时间线位置。 Final 是故意明确的：即使最后的记录与之前的轮询相同，它也会被保留。
type ValidationObservation struct {
	NodeID            string
	GroupID           string
	BranchID          string
	Assertion         ValidationAssertion
	Actual            string
	ActualValues      []string
	Passed            bool
	Reason            string
	BranchDisposition string
	Selector          fingerprint.Selector
	ObservedAtMS      int64
	Final             bool
}

type ValidationMemberIdentity struct {
	BranchID string
	NodeID   string
}

type ValidationGroupTerminalObservation struct {
	GroupID         string
	TerminalReason  string
	WinningBranchID string
	ExpectedMembers []ValidationMemberIdentity
	ObservedAtMS    int64
}

// ValidationStateReader 是元素的可选功能。  现有的仅操作驱动程序保持源代码兼容；具有验证功能的驱动程序提供一种标准 DOM/ARIA 投影，而不会将框架类泄漏到域中。
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
	// 对于独立验证，GroupID 和 BranchID 为空。它们是执行身份，而不是持久的表达式模型，并让证据适配器将成员观察结果附加到其组 StepExecution。
	GroupID   string
	BranchID  string
	Target    fingerprint.NodeSpec
	Assertion ValidationAssertion
	MaxWait   time.Duration
	Stability time.Duration
}

func (v *ValidationNode) ID() string { return v.NodeID }

func (v *ValidationNode) Run(ctx context.Context, rt *Runtime) (runErr error) {
	if err := rt.waitBeforeStep(ctx); err != nil {
		return fmt.Errorf("validation %s: wait step interval: %w", v.NodeID, err)
	}
	execution := NewStepExecution(v.NodeID)
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseRunning); err != nil {
		return err
	}
	defer rt.releaseOccurrence(execution.nodeID, execution.occurrence)
	lifecycle, err := rt.beginLeafLifecycle(ctx, v.NodeID, "VALIDATION", execution.occurrence)
	if err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	defer func() { runErr = lifecycle.Complete(ctx, runErr) }()
	if err := transitionValidation(ctx, rt, execution, v.NodeID, PhaseValidating); err != nil {
		return validationFail(ctx, rt, execution, v.NodeID, err)
	}
	started := time.Now()
	validationErr := v.waitStable(ctx, rt)
	observationErr := rt.observeOperation(context.WithoutCancel(ctx), OperationObservation{RunID: rt.RunID, NodeID: v.NodeID, Operation: "validation", Attempt: 1, DurationMS: time.Since(started).Milliseconds(), Succeeded: validationErr == nil, ErrorKind: errorKind(validationErr)})
	if validationErr != nil {
		return validationFail(ctx, rt, execution, v.NodeID, errors.Join(validationErr, observationErr))
	}
	if observationErr != nil {
		return validationFail(ctx, rt, execution, v.NodeID, observationErr)
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
	var stableSince time.Time
	var lastActual string
	var lastActualValues []string
	observations := newValidationObservationRecorder()
	pollErr := rt.poller().Run(parent, maxWait, func(pollCtx context.Context) (bool, error) {
		var actualValues []string
		ok, actual, err := v.evaluateCollect(pollCtx, rt, &actualValues)
		lastActual = actual
		lastActualValues = append(lastActualValues[:0], actualValues...)
		if err != nil {
			if recordErr := observations.record(pollCtx, rt, v, false, actual, actualValues, "system_error", false); recordErr != nil {
				return false, recordErr
			}
			return false, err
		}
		if err := observations.record(pollCtx, rt, v, ok, actual, actualValues, validationReason(ok), false); err != nil {
			return false, err
		}
		if ok {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= stability {
				return true, observations.record(pollCtx, rt, v, true, actual, actualValues, "passed", true)
			}
		} else {
			stableSince = time.Time{}
		}
		return false, nil
	})
	if pollErr != nil {
		reason := "timeout"
		if errorKind(pollErr) != ErrorTimeout {
			reason = "system_error"
		}
		if err := observations.record(context.WithoutCancel(parent), rt, v, false, lastActual, lastActualValues, reason, true); err != nil {
			return err
		}
		actual := lastActual
		if validationEvidenceIsSensitive(v.Target, v.Assertion) {
			actual = "••••••••"
		}
		return fmt.Errorf("assertion was not continuously satisfied within %s (last actual %q): %w", maxWait, actual, pollErr)
	}
	return nil
}

// 评估恰好执行一轮读取/检查。  ValidationGroupNode 在派生分支结果之前为每个成员调用此方法，保留“同一轮 AND”不变量。
func (v *ValidationNode) evaluate(ctx context.Context, rt *Runtime) (bool, string, error) {
	return v.evaluateCollect(ctx, rt, nil)
}

func (v *ValidationNode) evaluateCollect(ctx context.Context, rt *Runtime, actualValues *[]string) (bool, string, error) {
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
		// 删除故意不“不可见”；调用者必须为该语义断言 not_exists。
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
	visible, err := rt.reader().Visible(ctx, el)
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
		text, err := rt.reader().Text(ctx, el)
		if err != nil {
			return false, "", err
		}
		return compareText(assertion, text)
	}
	if assertion.Kind == "attribute_equals" || assertion.Kind == "attribute_contains" {
		value, present, err := rt.reader().Attribute(ctx, el, assertion.Attribute)
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
		if actualValues != nil {
			*actualValues = append([]string(nil), state.SelectedTexts...)
		}
		return compareSet(assertion, state.SelectedTexts)
	default:
		return false, "", fmt.Errorf("unsupported validation assertion %q", assertion.Kind)
	}
}

// solvedAssertion 仅扩展持久断言合约允许的值。属性名称故意是静态的：允许在 DOM 属性名称中进行插值使得验证形状数据依赖，并且在工作区验证期间被拒绝。
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

// locate 应用与操作步骤相同的确定性修复决策。对于 not_exists 断言，适用的已治愈候选者是该元素仍然存在的证据，并且必须阻止误报；只有真正的 no_candidate 结果才会被视为缺席。
func (v *ValidationNode) locate(ctx context.Context, rt *Runtime) (Element, bool, error) {
	target := v.Target
	el, err := rt.locator().Locate(ctx, target)
	if err == nil {
		return el, false, nil
	}
	if !isExclusiveElementNotFound(err) {
		return nil, false, err
	}
	if rt.Healing == nil && rt.Healer == nil {
		return nil, true, nil
	}
	snapshot, err := rt.Driver.Snapshot(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("snapshot for healing: %w", err)
	}
	decision, err := rt.healingPort().Recover(ctx, target, snapshot)
	if err != nil {
		return nil, false, err
	}
	if err := decision.Validate(); err != nil {
		return nil, false, fmt.Errorf("invalid heal decision: %w", err)
	}
	if err := rt.recordHealSamples(ctx, HealSampleRecord{RunID: rt.RunID, NodeID: v.NodeID, SpecID: target.ID, OldSelector: firstSelector(target), Outcome: decision.Outcome, Samples: heal.SortSamples(decision.Samples(target.Fingerprint, rt.healingReviewCap()))}); err != nil {
		return nil, false, fmt.Errorf("record heal samples: %w", err)
	}
	if decision.Outcome == heal.OutcomeNoCandidate {
		if rt.Facts != nil {
			if err := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, firstSelector(target), decision); err != nil {
				return nil, false, fmt.Errorf("record heal decision: %w", err)
			}
		}
		return nil, true, nil
	}
	assessment, err := heal.Assess(target, decision, heal.ExecutionContext{PageURL: rt.PageURL, Origin: rt.Origin}, rt.HealingPolicy)
	if err != nil {
		return nil, false, fmt.Errorf("assess heal decision: %w", err)
	}
	if assessment.Disposition != heal.DispositionAllow {
		if assessment.Disposition == heal.DispositionBlock && decision.Outcome != heal.OutcomeNoCandidate {
			decision.Outcome = heal.OutcomeSafetyRejected
			decision.NeedsReview = false
		}
		if rt.Facts != nil {
			oldSelector := firstSelector(target)
			if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, oldSelector, decision); recordErr != nil {
				return nil, false, fmt.Errorf("record validation heal decision: %w", recordErr)
			}
		}
		return nil, false, fmt.Errorf("validation healing refused: %s", assessment.Explanation)
	}
	if decision.Outcome == heal.OutcomeNoCandidate || decision.Best == nil {
		if rt.Facts != nil {
			if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, firstSelector(target), decision); recordErr != nil {
				return nil, false, fmt.Errorf("record no-candidate heal decision: %w", recordErr)
			}
		}
		return nil, true, nil
	}
	healed := target
	healed.Selectors = append([]fingerprint.Selector{decision.Best.Selector}, healed.Selectors...)
	el, err = rt.Driver.Locate(ctx, healed)
	if err != nil {
		return nil, false, fmt.Errorf("re-locate after heal: %w", err)
	}
	if rt.Facts != nil {
		if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, v.NodeID, target.ID, firstSelector(target), decision); recordErr != nil {
			return nil, false, fmt.Errorf("record heal decision: %w", recordErr)
		}
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
		if len(expected) != len(actual) {
			return false, strings.Join(actual, ", "), nil
		}
		for index := range expected {
			if expected[index] != actual[index] {
				return false, strings.Join(actual, ", "), nil
			}
		}
		return true, strings.Join(actual, ", "), nil
	}
	available := make(map[string]int, len(actual))
	for _, value := range actual {
		available[value]++
	}
	for _, value := range expected {
		if available[value] == 0 {
			return false, strings.Join(actual, ", "), nil
		}
		available[value]--
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

func (g *ValidationGroupNode) Run(ctx context.Context, rt *Runtime) (runErr error) {
	for branchIndex, branch := range g.Branches {
		if branch.ID == "" {
			return fmt.Errorf("validation group %s: branch %d requires an ID", g.NodeID, branchIndex)
		}
		for memberIndex, member := range branch.Nodes {
			if member == nil {
				return fmt.Errorf("validation group %s: branch %s member %d is nil", g.NodeID, branch.ID, memberIndex)
			}
		}
	}
	if err := rt.waitBeforeStep(ctx); err != nil {
		return fmt.Errorf("validation group %s: wait step interval: %w", g.NodeID, err)
	}
	execution := NewStepExecution(g.NodeID)
	if err := transitionValidation(ctx, rt, execution, g.NodeID, PhaseRunning); err != nil {
		return err
	}
	defer rt.releaseOccurrence(execution.nodeID, execution.occurrence)
	lifecycle, err := rt.beginLeafLifecycle(ctx, g.NodeID, "VALIDATION_GROUP", execution.occurrence)
	if err != nil {
		return validationFail(ctx, rt, execution, g.NodeID, err)
	}
	defer func() { runErr = lifecycle.Complete(ctx, runErr) }()
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
	observations := newValidationObservationRecorder()
	stableSince := make([]time.Time, len(g.Branches))
	last := make(map[validationObservationKey]validationObservationState)
	pollErr := rt.poller().Run(parent, maxWait, func(ctx context.Context) (bool, error) {
		branchPassed := make([]bool, len(g.Branches))
		for i, branch := range g.Branches {
			branchPassed[i] = len(branch.Nodes) > 0
			for _, member := range branch.Nodes {
				canonical := *member
				canonical.GroupID = g.NodeID
				canonical.BranchID = branch.ID
				var actualValues []string
				ok, actual, err := canonical.evaluateCollect(ctx, rt, &actualValues)
				if err != nil {
					if actual != "" || actualValues != nil {
						key := validationObservationKey{groupID: g.NodeID, branchID: branch.ID, nodeID: canonical.NodeID}
						last[key] = validationObservationState{actual: actual, actualValues: append([]string(nil), actualValues...), reason: "system_error"}
					}
					if recordErr := observations.record(ctx, rt, &canonical, false, actual, actualValues, "system_error", false); recordErr != nil {
						return false, recordErr
					}
					return false, fmt.Errorf("branch %s: %w", branch.ID, err)
				}
				key := validationObservationKey{groupID: g.NodeID, branchID: branch.ID, nodeID: canonical.NodeID}
				last[key] = validationObservationState{passed: ok, actual: actual, actualValues: actualValues, reason: validationReason(ok)}
				if err := observations.record(ctx, rt, &canonical, ok, actual, actualValues, validationReason(ok), false); err != nil {
					return false, err
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
				winner := g.Branches[i].ID
				if err := observations.recordGroupFinal(ctx, rt, g, last, "passed", winner); err != nil {
					return false, err
				}
				return true, nil
			}
		}
		return false, nil
	})
	if pollErr != nil {
		ctx := context.WithoutCancel(parent)
		reason := validationTerminalReason(pollErr)
		if err := observations.recordGroupFinal(ctx, rt, g, last, reason, ""); err != nil {
			return err
		}
		return fmt.Errorf("no validation branch was continuously satisfied within %s: %w", maxWait, pollErr)
	}
	return nil
}

type validationObservationState struct {
	passed       bool
	actual       string
	actualValues []string
	reason       string
}

func validationObservationStatesEqual(left, right validationObservationState) bool {
	return left.passed == right.passed && left.actual == right.actual && left.reason == right.reason && slices.Equal(left.actualValues, right.actualValues)
}

type validationObservationKey struct {
	groupID  string
	branchID string
	nodeID   string
}

type validationObservationRecorder struct {
	last map[validationObservationKey]validationObservationState
}

func newValidationObservationRecorder() *validationObservationRecorder {
	return &validationObservationRecorder{last: make(map[validationObservationKey]validationObservationState)}
}

func (r *validationObservationRecorder) record(ctx context.Context, rt *Runtime, validation *ValidationNode,
	passed bool, actual string, actualValues []string, reason string, final bool) error {
	if rt.Facts == nil {
		return nil
	}
	state := validationObservationState{passed: passed, actual: actual, actualValues: append([]string(nil), actualValues...), reason: reason}
	key := validationObservationKey{groupID: validation.GroupID, branchID: validation.BranchID, nodeID: validation.NodeID}
	previous, seen := r.last[key]
	if !final && seen && validationObservationStatesEqual(previous, state) {
		return nil
	}
	r.last[key] = state
	spec := rt.effectiveSpec(validation.Target)
	selector := fingerprint.Selector{}
	if len(spec.Selectors) > 0 {
		selector = spec.Selectors[0]
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	assertion := validation.Assertion
	assertion.ExpectedValues = append([]string(nil), validation.Assertion.ExpectedValues...)
	if validationEvidenceIsSensitive(validation.Target, assertion) {
		actual = "••••••••"
		actualValues = nil
		if assertion.Expected != "" {
			assertion.Expected = "••••••••"
		}
		assertion.ExpectedValues = nil
	}
	return rt.Facts.StageValidationObservation(cleanupCtx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, ValidationObservation{
		NodeID: validation.NodeID, GroupID: validation.GroupID, BranchID: validation.BranchID,
		Assertion: assertion, Actual: actual, ActualValues: append([]string(nil), actualValues...), Passed: passed, Reason: reason,
		Selector: selector, ObservedAtMS: time.Now().UnixMilli(), Final: final,
	})
}

func (r *validationObservationRecorder) recordGroupFinal(ctx context.Context, rt *Runtime, group *ValidationGroupNode, last map[validationObservationKey]validationObservationState, terminalReason, winningBranchID string) error {
	members := make([]ValidationMemberIdentity, 0)
	seen := make(map[ValidationMemberIdentity]struct{})
	for _, branch := range group.Branches {
		disposition := branchDisposition(group.NodeID, branch, last, terminalReason, winningBranchID)
		for _, member := range branch.Nodes {
			identity := ValidationMemberIdentity{BranchID: branch.ID, NodeID: member.NodeID}
			if _, exists := seen[identity]; exists {
				continue
			}
			seen[identity] = struct{}{}
			members = append(members, identity)
			canonical := *member
			canonical.GroupID = group.NodeID
			canonical.BranchID = branch.ID
			key := validationObservationKey{groupID: group.NodeID, branchID: branch.ID, nodeID: member.NodeID}
			state, observed := last[key]
			if !observed {
				state.reason = terminalReason
			}
			reason := state.reason
			if terminalReason != "passed" {
				reason = terminalReason
			} else if state.passed {
				reason = "passed"
			}
			if err := r.recordWithDisposition(ctx, rt, &canonical, state.passed, state.actual, state.actualValues, reason, disposition); err != nil {
				return err
			}
		}
	}
	if rt.Facts == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	return rt.Facts.StageValidationGroupTerminal(cleanupCtx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, ValidationGroupTerminalObservation{
		GroupID: group.NodeID, TerminalReason: terminalReason, WinningBranchID: winningBranchID,
		ExpectedMembers: members, ObservedAtMS: time.Now().UnixMilli(),
	})
}

func (r *validationObservationRecorder) recordWithDisposition(ctx context.Context, rt *Runtime, validation *ValidationNode, passed bool, actual string, actualValues []string, reason, disposition string) error {
	if rt.Facts == nil {
		return nil
	}
	state := validationObservationState{passed: passed, actual: actual, actualValues: append([]string(nil), actualValues...), reason: reason}
	key := validationObservationKey{groupID: validation.GroupID, branchID: validation.BranchID, nodeID: validation.NodeID}
	r.last[key] = state
	spec := rt.effectiveSpec(validation.Target)
	selector := fingerprint.Selector{}
	if len(spec.Selectors) > 0 {
		selector = spec.Selectors[0]
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	assertion := validation.Assertion
	assertion.ExpectedValues = append([]string(nil), validation.Assertion.ExpectedValues...)
	if validationEvidenceIsSensitive(validation.Target, assertion) {
		actual = "••••••••"
		actualValues = nil
		if assertion.Expected != "" {
			assertion.Expected = "••••••••"
		}
		assertion.ExpectedValues = nil
	}
	return rt.Facts.StageValidationObservation(cleanupCtx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, ValidationObservation{
		NodeID: validation.NodeID, GroupID: validation.GroupID, BranchID: validation.BranchID,
		Assertion: assertion, Actual: actual, ActualValues: append([]string(nil), actualValues...), Passed: passed, Reason: reason, BranchDisposition: disposition,
		Selector: selector, ObservedAtMS: time.Now().UnixMilli(), Final: true,
	})
}

func branchDisposition(groupID string, branch ValidationBranch, last map[validationObservationKey]validationObservationState, terminalReason, winningBranchID string) string {
	if branch.ID == winningBranchID {
		return "won"
	}
	observed := len(branch.Nodes) > 0
	satisfied := observed
	for _, member := range branch.Nodes {
		state, exists := last[validationObservationKey{groupID: groupID, branchID: branch.ID, nodeID: member.NodeID}]
		observed = observed && exists
		satisfied = satisfied && exists && state.passed
	}
	if terminalReason == "passed" && satisfied {
		return "satisfied_not_winner"
	}
	if observed {
		return "not_satisfied"
	}
	return "not_observed"
}

func validationTerminalReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var classified *ClassifiedError
	if errors.As(err, &classified) && classified.Kind == ErrorTimeout {
		return "timeout"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "system_error"
}

func validationEvidenceIsSensitive(target fingerprint.NodeSpec, assertion ValidationAssertion) bool {
	if !strings.HasPrefix(assertion.Kind, "value_") && !strings.HasPrefix(assertion.Kind, "selected_set_") && assertion.Kind != "attribute_equals" && assertion.Kind != "attribute_contains" {
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
	if next == PhaseRunning {
		occurrence, err := rt.activeOccurrence(nodeID)
		if err != nil {
			return err
		}
		execution.occurrence = occurrence
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
