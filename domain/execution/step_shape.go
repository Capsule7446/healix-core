package execution

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/interpolation"

	"github.com/Capsule7446/healix-core/domain/weburl"
)

// stepShapeBuilder accumulates the ordered violations for one workflow
// snapshot's step tree. Every check that reaches an already-classified fault
// from another context (interpolation, fingerprint, parameter) short-circuits
// the walk and lets that fault propagate unchanged instead of being buried
// under a generic violation: those contexts own their own contract, and a
// caller fixing an interpolation failure should not have to unwrap a second
// envelope to learn that. Every other internal shape check stays an ordinary
// Go error and is discarded once recorded as a violation, matching the
// discard pattern used across every other aggregate envelope in this
// contract.
type stepShapeBuilder struct {
	violations []fault.Violation
	classified error
}

func (b *stepShapeBuilder) done() bool {
	return b.classified != nil || atCap(b.violations)
}

func (b *stepShapeBuilder) violation(code fault.Code, field, message string) {
	if b.done() {
		return
	}
	b.violations = append(b.violations, mustViolation(code, field, message))
}

// absorb records cause as the short-circuiting classified fault when cause is
// already coded, and otherwise discards its detail in favor of one generic
// violation. cause's own text may carry identities or user-supplied values —
// discarding it is what keeps them out of public text.
func (b *stepShapeBuilder) absorb(cause error, code fault.Code, field, message string) {
	if cause == nil || b.done() {
		return
	}
	if _, ok := fault.CodeOf(cause); ok {
		b.classified = cause
		return
	}
	b.violation(code, field, message)
}

// Validate reports every step-shape failure through one aggregate envelope
// carrying ordered violations, or propagates an already-classified fault
// (interpolation, fingerprint, parameter) reached mid-walk unchanged. No step
// identity, parameter name, or enum value reaches public text: every failing
// check degrades into a generic violation at the recursive, unindexed "steps"
// field, matching the existing precedent for recursive walks that have no
// flat position to report.
func (w WorkflowSnapshot) Validate() error {
	builder := &stepShapeBuilder{}
	if strings.TrimSpace(w.FlowFragmentID) == "" || strings.TrimSpace(w.VersionID) == "" || (w.ID != "" && w.ID != w.FlowFragmentID) {
		builder.violation(fault.CodeFieldInvalid, "flowFragmentId", "workflow version does not belong to workflow")
	}
	if strings.TrimSpace(w.DisplayName) == "" {
		builder.violation(fault.CodeFieldRequired, "displayName", "display name is required")
	}
	if w.VersionNumber < 1 {
		builder.violation(fault.CodeFieldInvalid, "versionNumber", "version number must be positive")
	}
	switch {
	case len(w.Steps) == 0:
		builder.violation(fault.CodeFieldRequired, "steps", "workflow requires at least one step")
	default:
		if err := validateStepBounds(w.Steps); err != nil {
			builder.violation(fault.CodeFieldInvalid, "steps", "workflow step structure exceeds the allowed nesting depth or step count")
		} else {
			seen := make(map[string]struct{})
			validateStepsInto(builder, w.Steps, true, seen)
		}
	}
	seenParameterNames := make(map[string]struct{}, len(w.Parameters))
	for index, definition := range w.Parameters {
		if builder.done() {
			break
		}
		if err := definition.Validate(); err != nil {
			builder.violation(fault.CodeFieldInvalid, fieldIndex("parameters", index), "workflow parameter is invalid")
		}
		if _, duplicate := seenParameterNames[definition.Name]; duplicate {
			builder.violation(fault.CodeFieldDuplicate, fieldIndex("parameters", index), "workflow parameter name is duplicated")
		}
		seenParameterNames[definition.Name] = struct{}{}
	}
	if builder.classified != nil {
		return builder.classified
	}
	if len(builder.violations) != 0 {
		return stepShapeInvalidError(builder.violations)
	}
	return nil
}

func fieldIndex(prefix string, index int) string {
	return prefix + "." + strconv.Itoa(index)
}

func validateStepsInto(b *stepShapeBuilder, steps []Step, root bool, seen map[string]struct{}) {
	for _, step := range steps {
		if b.done() {
			return
		}
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.DisplayName) == "" {
			b.violation(fault.CodeFieldRequired, "steps", "step id and display name are required")
		}
		if _, exists := seen[step.ID]; exists {
			b.violation(fault.CodeFieldDuplicate, "steps", "step id is duplicated")
		}
		seen[step.ID] = struct{}{}
		if step.Kind != ActionStep && step.Optional {
			b.violation(fault.CodeFieldInvalid, "steps", "only an action step may be optional")
		}
		switch step.Kind {
		case ActionStep:
			validateActionInto(b, step)
		case WaitStep:
			validateWaitInto(b, step)
		case RepeatStep:
			validateRepeatInto(b, step)
			validateStepsInto(b, step.Children, false, seen)
		case FlowFragmentReference:
			validateReferenceInto(b, step)
		case ValidationStep:
			if !root {
				b.violation(fault.CodeFieldInvalid, "steps", "a validation step must be a root step or validation-group member")
			}
			validateValidationStepInto(b, step, false)
		case ValidationGroupStep:
			if !root {
				b.violation(fault.CodeFieldInvalid, "steps", "a validation group must be a root step")
			}
			validateValidationGroupInto(b, step, seen)
		default:
			b.violation(fault.CodeFieldInvalid, "steps", "step kind is unsupported")
		}
	}
}

func validateActionInto(b *stepShapeBuilder, s Step) {
	if s.Validation != nil || s.ValidationGroup != nil || s.Reference != nil || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 {
		b.violation(fault.CodeFieldInvalid, "steps", "an action step contains unsupported step configuration")
	}
	switch s.Action {
	case "click", "input", "select", "hover", "navigate", "press", "noop", "extract":
	default:
		b.violation(fault.CodeFieldInvalid, "steps", "action step action is unsupported")
	}
	if s.Action != "navigate" && s.Action != "press" && strings.TrimSpace(s.ElementTargetID) == "" {
		b.violation(fault.CodeFieldRequired, "steps", "action step requires a node")
	}
	if strings.TrimSpace(s.ElementTargetID) != "" && strings.TrimSpace(s.ElementTargetVersionID) == "" {
		b.violation(fault.CodeFieldRequired, "steps", "action step requires an exact node version")
	}
	if (s.Action == "navigate" || s.Action == "press" || s.Action == "extract") && strings.TrimSpace(s.Value) == "" {
		b.violation(fault.CodeFieldRequired, "steps", "action step requires a value")
	}
	if s.Action == "navigate" && strings.TrimSpace(s.Value) != "" {
		b.absorb(validateSealedNavigationURL(s.Value), fault.CodeFieldInvalid, "steps", "action step navigate URL is invalid")
	}
	if s.Action == "navigate" || s.Action == "input" || s.Action == "select" || s.Action == "press" {
		for _, value := range append([]string{s.Value}, s.Values...) {
			if b.done() {
				return
			}
			_, err := interpolation.Names(value)
			b.absorb(err, fault.CodeFieldInvalid, "steps", "action step value is invalid")
		}
	}
	if s.Action == "select" && strings.TrimSpace(s.Value) == "" && len(s.Values) == 0 {
		b.violation(fault.CodeFieldRequired, "steps", "select action requires at least one value")
	}
}

// validateSealedNavigationURL rejects control characters, disallows
// interpolation inside the scheme or authority, and otherwise requires an
// absolute HTTP(S) URL without embedded credentials. It returns the
// interpolation package's own coded fault unwrapped so a caller does not have
// to unwrap a second envelope to learn the expression was malformed.
func validateSealedNavigationURL(value string) error {
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return errors.New("control characters are not allowed")
	}
	names, err := interpolation.Names(value)
	if err != nil {
		return err
	}
	authorityEnd := len(value)
	if scheme := strings.Index(value, "://"); scheme >= 0 {
		authorityEnd = scheme + 3
		if slash := strings.IndexAny(value[authorityEnd:], "/?#"); slash >= 0 {
			authorityEnd += slash
		}
	}
	if strings.Contains(value[:authorityEnd], "${") {
		return errors.New("interpolation is not allowed in URL scheme or authority")
	}
	parseable := value
	for _, name := range names {
		parseable = strings.ReplaceAll(parseable, "${"+name+"}", "placeholder")
	}
	// The host requirement used to be skipped whenever the URL contained any
	// interpolation, which let `https:///${path}` through with no host at all
	// while the same URL without the variable was refused. Interpolation is
	// already banned in the authority above, so by this point the authority is
	// literal in every candidate and its host can always be checked.
	if rejection := weburl.Check(parseable); rejection != weburl.Accepted {
		return fmt.Errorf("navigation URL rejected: %s", rejection)
	}
	return nil
}

func validateWaitInto(b *stepShapeBuilder, s Step) {
	element := s.WaitKind == "element" || s.WaitKind == "element_visible" || s.WaitKind == "element_invisible"
	if s.Validation != nil || s.ValidationGroup != nil || s.Action != "" || s.Value != "" || len(s.Values) != 0 || s.RepeatCount != 0 || s.Reference != nil || len(s.Children) != 0 || (!element && (s.ElementTargetID != "" || s.ElementTargetVersionID != "")) {
		b.violation(fault.CodeFieldInvalid, "steps", "a wait step contains unsupported step configuration")
	}
	switch s.WaitKind {
	case "", "sleep":
		if s.WaitMS <= 0 || s.WaitMS > MaxWaitMS {
			b.violation(fault.CodeFieldInvalid, "steps", "a fixed wait duration is out of range")
		}
	case "element", "element_visible", "element_invisible":
		if strings.TrimSpace(s.ElementTargetID) == "" {
			b.violation(fault.CodeFieldRequired, "steps", "an element wait requires a node")
		}
		if strings.TrimSpace(s.ElementTargetVersionID) == "" {
			b.violation(fault.CodeFieldRequired, "steps", "an element wait requires an exact node version")
		}
		if s.WaitMS < 0 || s.WaitMS > MaxWaitMS {
			b.violation(fault.CodeFieldInvalid, "steps", "a wait timeout is out of range")
		}
	case "network_idle":
		if s.WaitMS < 0 || s.WaitMS > MaxWaitMS {
			b.violation(fault.CodeFieldInvalid, "steps", "a wait timeout is out of range")
		}
	default:
		b.violation(fault.CodeFieldInvalid, "steps", "wait kind is unsupported")
	}
}

func validateRepeatInto(b *stepShapeBuilder, s Step) {
	if s.Validation != nil || s.ValidationGroup != nil || s.Action != "" || s.ElementTargetID != "" || s.ElementTargetVersionID != "" || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.Reference != nil {
		b.violation(fault.CodeFieldInvalid, "steps", "a repeat step contains unsupported step configuration")
	}
	if s.RepeatCount < 1 || len(s.Children) == 0 {
		b.violation(fault.CodeFieldRequired, "steps", "a repeat step requires a count and children")
	} else if s.RepeatCount > MaxRepeatCount {
		b.violation(fault.CodeFieldInvalid, "steps", "repeat count exceeds the allowed maximum")
	}
}

// validateReferenceInto validates a WORKFLOW_REF step's shape. Parameter
// binding names are walked in sorted order rather than map order, because
// violation order must be a function of the input alone.
func validateReferenceInto(b *stepShapeBuilder, s Step) {
	r := s.Reference
	if s.Validation != nil || s.ValidationGroup != nil || s.Action != "" || s.ElementTargetID != "" || s.ElementTargetVersionID != "" || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 {
		b.violation(fault.CodeFieldInvalid, "steps", "a workflow reference step contains unsupported step configuration")
	}
	if r == nil || strings.TrimSpace(r.FlowFragmentID) == "" {
		b.violation(fault.CodeFieldRequired, "steps", "a workflow reference step requires a workflow reference")
	}
	if r == nil {
		return
	}
	names := make([]string, 0, len(r.ParameterBindings))
	for name := range r.ParameterBindings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if b.done() {
			return
		}
		binding := r.ParameterBindings[name]
		if strings.TrimSpace(name) == "" {
			b.violation(fault.CodeFieldInvalid, "steps", "a workflow reference has an empty parameter binding name")
		}
		if _, err := binding.Resolve(nil); err != nil {
			if _, isReference := binding.ParentName(); !isReference {
				b.absorb(err, fault.CodeFieldInvalid, "steps", "a workflow reference parameter binding is invalid")
			}
		}
	}
}

func validateValidationStepInto(b *stepShapeBuilder, s Step, member bool) {
	if s.Validation == nil {
		b.violation(fault.CodeFieldRequired, "steps", "a validation step requires validation configuration")
		return
	}
	if s.ValidationGroup != nil || s.Action != "" || s.Reference != nil || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 || s.Optional {
		b.violation(fault.CodeFieldInvalid, "steps", "a validation step contains unsupported action or child configuration")
	}
	if strings.TrimSpace(s.ElementTargetID) == "" || strings.TrimSpace(s.ElementTargetVersionID) == "" {
		b.violation(fault.CodeFieldRequired, "steps", "a validation step requires an exact node reference")
	}
	b.absorb(s.Validation.Validate(!member), fault.CodeFieldInvalid, "steps", "validation configuration is invalid")
}

func validateValidationGroupInto(b *stepShapeBuilder, s Step, seen map[string]struct{}) {
	g := s.ValidationGroup
	if g == nil {
		b.violation(fault.CodeFieldRequired, "steps", "a validation group requires group configuration")
		return
	}
	if s.Validation != nil || s.Action != "" || s.Reference != nil || s.ElementTargetID != "" || s.ElementTargetVersionID != "" || s.Value != "" || len(s.Values) != 0 || s.WaitKind != "" || s.WaitMS != 0 || s.RepeatCount != 0 || len(s.Children) != 0 || s.Optional {
		b.violation(fault.CodeFieldInvalid, "steps", "a validation group contains unsupported step configuration")
	}
	b.absorb(validateValidationWait(g.MaxWaitMS, g.StabilityMS), fault.CodeFieldInvalid, "steps", "validation group wait configuration is invalid")
	if len(g.Branches) == 0 || len(g.Branches) > validationMaxBranches {
		b.violation(fault.CodeFieldInvalid, "steps", "a validation group requires a supported number of branches")
	}
	branchIDs, total := map[string]struct{}{}, 0
	for _, branch := range g.Branches {
		if b.done() {
			return
		}
		if strings.TrimSpace(branch.ID) == "" || strings.TrimSpace(branch.Name) == "" {
			b.violation(fault.CodeFieldRequired, "steps", "a validation group branch requires an id and a name")
		}
		if _, ok := branchIDs[branch.ID]; ok && branch.ID != "" {
			b.violation(fault.CodeFieldDuplicate, "steps", "a validation group branch id is duplicated")
		}
		branchIDs[branch.ID] = struct{}{}
		if len(branch.Steps) == 0 || len(branch.Steps) > validationMaxBranchSteps {
			b.violation(fault.CodeFieldInvalid, "steps", "a validation group branch requires a supported number of validation steps")
		}
		total += len(branch.Steps)
		for _, member := range branch.Steps {
			if b.done() {
				return
			}
			if _, ok := seen[member.ID]; ok {
				b.violation(fault.CodeFieldDuplicate, "steps", "a step id is duplicated")
			}
			seen[member.ID] = struct{}{}
			if strings.TrimSpace(member.ID) == "" || strings.TrimSpace(member.DisplayName) == "" {
				b.violation(fault.CodeFieldRequired, "steps", "a validation group member requires an id and a display name")
			}
			if member.Kind != ValidationStep {
				b.violation(fault.CodeFieldInvalid, "steps", "a validation group branch only accepts validation steps")
				continue
			}
			validateValidationStepInto(b, member, true)
		}
	}
	if total > validationMaxGroupSteps {
		b.violation(fault.CodeFieldInvalid, "steps", "a validation group exceeds the maximum number of validation steps")
	}
}

// Validate stays an ordinary Go error: it is reached only as an internal
// shape check, either absorbed generically by the step-shape envelope above
// or discarded by a direct caller that only needs to know whether the
// configuration is acceptable. It never echoes Kind, Expected, or Attribute:
// Kind is a caller-supplied enum that may fall outside the closed set when
// invalid, and Expected/Attribute may carry interpolated parameter values.
// An interpolation failure is returned unwrapped so an already-classified
// fault is never buried under a second one.
func (v Validation) Validate(waitRequired bool) error {
	if _, err := interpolation.Names(v.Expected); err != nil {
		return err
	}
	for _, value := range v.ExpectedValues {
		if _, err := interpolation.Names(value); err != nil {
			return err
		}
	}
	switch v.Kind {
	case "exists", "not_exists", "visible", "not_visible", "value_not_empty", "enabled", "disabled", "checked", "unchecked", "mixed", "selected", "unselected", "pressed", "unpressed":
		if v.Expected != "" || len(v.ExpectedValues) != 0 || v.Attribute != "" || v.IgnoreCase {
			return errors.New("validation does not accept comparison options")
		}
	case "text_equals", "text_contains", "value_equals", "value_contains", "selected_text_equals", "selected_text_contains", "selected_value_equals", "selected_value_contains":
		if len(v.ExpectedValues) != 0 || v.Attribute != "" {
			return errors.New("validation accepts one scalar expected value")
		}
	case "text_matches", "value_matches":
		if len(v.ExpectedValues) != 0 || v.Attribute != "" || v.IgnoreCase {
			return errors.New("validation accepts only a regular expression")
		}
		if !strings.Contains(v.Expected, "${") {
			if _, err := regexp.Compile(v.Expected); err != nil {
				return errors.New("validation has an invalid regular expression")
			}
		}
	case "selected_set_equals", "selected_set_contains":
		if v.Expected != "" || v.Attribute != "" || v.IgnoreCase {
			return errors.New("validation accepts only expected values")
		}
	case "attribute_equals", "attribute_contains":
		if strings.TrimSpace(v.Attribute) == "" {
			return errors.New("attribute validation requires an attribute name")
		}
		if len(v.ExpectedValues) != 0 {
			return errors.New("validation accepts one scalar expected value")
		}
		if strings.Contains(v.Attribute, "${") {
			return errors.New("attribute validation does not accept variable expressions")
		}
	default:
		return errors.New("unsupported validation kind")
	}
	if waitRequired {
		return validateValidationWait(v.MaxWaitMS, v.StabilityMS)
	}
	if v.MaxWaitMS != 0 || v.StabilityMS != 0 {
		return errors.New("validation group member must inherit the group wait")
	}
	return nil
}

func validateValidationWait(maxWait, stability int) error {
	if maxWait < validationMinWaitMS || maxWait > validationMaxWaitMS {
		return errors.New("validation maximum wait is out of range")
	}
	if stability < validationMinStabilityMS || stability > validationMaxStabilityMS {
		return errors.New("validation stability window is out of range")
	}
	if stability >= maxWait {
		return errors.New("validation stability window must be shorter than maximum wait")
	}
	return nil
}
