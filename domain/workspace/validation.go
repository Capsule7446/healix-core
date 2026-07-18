package workspace

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	validationMinWaitMS      = 1_000
	validationMaxWaitMS      = 60_000
	validationMinStabilityMS = 200
	validationMaxStabilityMS = 5_000
	validationMaxBranches    = 5
	validationMaxBranchSteps = 10
	validationMaxGroupSteps  = 20
)

// ValidationAssertionKind is a framework-independent statement evaluated
// against one exactly versioned Node. Framework adapters convert DOM/ARIA
// details to these meanings; no framework-specific selector or class belongs
// here.
type ValidationAssertionKind string

const (
	ValidationExists                ValidationAssertionKind = "exists"
	ValidationNotExists             ValidationAssertionKind = "not_exists"
	ValidationVisible               ValidationAssertionKind = "visible"
	ValidationNotVisible            ValidationAssertionKind = "not_visible"
	ValidationTextEquals            ValidationAssertionKind = "text_equals"
	ValidationTextContains          ValidationAssertionKind = "text_contains"
	ValidationTextMatches           ValidationAssertionKind = "text_matches"
	ValidationValueEquals           ValidationAssertionKind = "value_equals"
	ValidationValueContains         ValidationAssertionKind = "value_contains"
	ValidationValueMatches          ValidationAssertionKind = "value_matches"
	ValidationValueNotEmpty         ValidationAssertionKind = "value_not_empty"
	ValidationEnabled               ValidationAssertionKind = "enabled"
	ValidationDisabled              ValidationAssertionKind = "disabled"
	ValidationChecked               ValidationAssertionKind = "checked"
	ValidationUnchecked             ValidationAssertionKind = "unchecked"
	ValidationMixed                 ValidationAssertionKind = "mixed"
	ValidationSelected              ValidationAssertionKind = "selected"
	ValidationUnselected            ValidationAssertionKind = "unselected"
	ValidationPressed               ValidationAssertionKind = "pressed"
	ValidationUnpressed             ValidationAssertionKind = "unpressed"
	ValidationSelectedTextEquals    ValidationAssertionKind = "selected_text_equals"
	ValidationSelectedTextContains  ValidationAssertionKind = "selected_text_contains"
	ValidationSelectedValueEquals   ValidationAssertionKind = "selected_value_equals"
	ValidationSelectedValueContains ValidationAssertionKind = "selected_value_contains"
	ValidationSelectedSetEquals     ValidationAssertionKind = "selected_set_equals"
	ValidationSelectedSetContains   ValidationAssertionKind = "selected_set_contains"
	ValidationAttributeEquals       ValidationAssertionKind = "attribute_equals"
	ValidationAttributeContains     ValidationAssertionKind = "attribute_contains"
)

// ValidationAssertion is intentionally singular. A Validation step represents
// exactly one statement; callers express conjunction/disjunction with a
// ValidationGroup rather than embedding assertion trees in a node.
type ValidationAssertion struct {
	Kind     ValidationAssertionKind
	Expected string
	// ExpectedValues is used only by selected_set_* and represents an unordered
	// collection. An empty slice is meaningful: it asserts no item is selected.
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
}

// Normalized removes fields that have no meaning for the selected assertion
// kind. Adapters use it when a user switches kinds or when a browser sampler
// provides a semantic recommendation, while Validate remains strict for raw
// domain inputs.
func (a ValidationAssertion) Normalized() ValidationAssertion {
	a.Kind = ValidationAssertionKind(strings.TrimSpace(string(a.Kind)))
	switch a.Kind {
	case ValidationExists, ValidationNotExists, ValidationVisible, ValidationNotVisible,
		ValidationValueNotEmpty, ValidationEnabled, ValidationDisabled, ValidationChecked,
		ValidationUnchecked, ValidationMixed, ValidationSelected, ValidationUnselected,
		ValidationPressed, ValidationUnpressed:
		return ValidationAssertion{Kind: a.Kind}
	case ValidationTextMatches, ValidationValueMatches:
		a.ExpectedValues = nil
		a.Attribute = ""
		a.IgnoreCase = false
	case ValidationSelectedSetEquals, ValidationSelectedSetContains:
		a.Expected = ""
		a.Attribute = ""
		a.IgnoreCase = false
	case ValidationAttributeEquals, ValidationAttributeContains:
		a.ExpectedValues = nil
	case ValidationTextEquals, ValidationTextContains, ValidationValueEquals, ValidationValueContains,
		ValidationSelectedTextEquals, ValidationSelectedTextContains, ValidationSelectedValueEquals,
		ValidationSelectedValueContains:
		a.ExpectedValues = nil
		a.Attribute = ""
	}
	return a
}

func (a ValidationAssertion) Validate() error {
	switch a.Kind {
	case ValidationExists, ValidationNotExists, ValidationVisible, ValidationNotVisible,
		ValidationValueNotEmpty, ValidationEnabled, ValidationDisabled, ValidationChecked,
		ValidationUnchecked, ValidationMixed, ValidationSelected, ValidationUnselected,
		ValidationPressed, ValidationUnpressed:
		if a.Expected != "" || len(a.ExpectedValues) != 0 || a.Attribute != "" || a.IgnoreCase {
			return fmt.Errorf("validation %q does not accept comparison options", a.Kind)
		}
		return nil
	case ValidationTextEquals, ValidationTextContains, ValidationValueEquals, ValidationValueContains,
		ValidationSelectedTextEquals, ValidationSelectedTextContains, ValidationSelectedValueEquals,
		ValidationSelectedValueContains:
		if len(a.ExpectedValues) != 0 || a.Attribute != "" {
			return fmt.Errorf("validation %q accepts one scalar expected value", a.Kind)
		}
		return nil
	case ValidationTextMatches, ValidationValueMatches:
		if len(a.ExpectedValues) != 0 || a.Attribute != "" || a.IgnoreCase {
			return fmt.Errorf("validation %q accepts only a regular expression", a.Kind)
		}
		// A runtime expression is compiled only after its variables are expanded.
		// Compiling ${env.pattern} here would reject a valid persisted template.
		if !strings.Contains(a.Expected, "${") {
			if _, err := regexp.Compile(a.Expected); err != nil {
				return fmt.Errorf("validation %q has invalid regular expression: %w", a.Kind, err)
			}
		}
		return nil
	case ValidationSelectedSetEquals, ValidationSelectedSetContains:
		if a.Expected != "" || a.Attribute != "" || a.IgnoreCase {
			return fmt.Errorf("validation %q accepts only expected values", a.Kind)
		}
		return nil
	case ValidationAttributeEquals, ValidationAttributeContains:
		if strings.TrimSpace(a.Attribute) == "" {
			return errors.New("attribute validation requires an attribute name")
		}
		if len(a.ExpectedValues) != 0 {
			return fmt.Errorf("validation %q accepts one scalar expected value", a.Kind)
		}
		if strings.Contains(a.Attribute, "${") {
			return errors.New("attribute validation does not accept variable expressions")
		}
		return nil
	default:
		return fmt.Errorf("unsupported validation kind %q", a.Kind)
	}
}

// ValidationWait defines the bounded wait and continuous-stability windows for
// a standalone validation node or an entire validation group.
type ValidationWait struct {
	MaxWaitMS   int
	StabilityMS int
}

func (w ValidationWait) Validate() error {
	if w.MaxWaitMS < validationMinWaitMS || w.MaxWaitMS > validationMaxWaitMS {
		return fmt.Errorf("validation maximum wait must be %d-%dms", validationMinWaitMS, validationMaxWaitMS)
	}
	if w.StabilityMS < validationMinStabilityMS || w.StabilityMS > validationMaxStabilityMS {
		return fmt.Errorf("validation stability window must be %d-%dms", validationMinStabilityMS, validationMaxStabilityMS)
	}
	if w.StabilityMS >= w.MaxWaitMS {
		return errors.New("validation stability window must be shorter than maximum wait")
	}
	return nil
}

func (w ValidationWait) isZero() bool { return w.MaxWaitMS == 0 && w.StabilityMS == 0 }

// ValidationConfig belongs to a StepValidation. The wait is meaningful only
// for a standalone validation; group members inherit ValidationGroup.Wait.
type ValidationConfig struct {
	Assertion ValidationAssertion
	Wait      ValidationWait
	// Actual/SuggestedKinds are sampling-time editor hints, not executable
	// truth. They make the captured semantic recommendation inspectable while
	// keeping the persisted assertion itself independent of a UI framework.
	Actual         string
	SupportedKinds []ValidationAssertionKind
}

// ValidationBranch is one AND branch in the fixed (AND...) OR (AND...) group
// grammar. Steps are kept as WorkflowStep values so DTO mappers and the
// materializer can use one recursive schema, while aggregate validation
// prevents any other kind from entering a branch.
type ValidationBranch struct {
	ID    string
	Name  string
	Steps []WorkflowStep
}

// ValidationGroup is a one-level disjunction of AND branches. Its Wait is
// inherited by every member node; nested groups and action nodes are invalid.
type ValidationGroup struct {
	Wait     ValidationWait
	Branches []ValidationBranch
}

func validateStandaloneValidationStep(step WorkflowStep) []string {
	var problems []string
	if step.Validation == nil {
		return []string{fmt.Sprintf("validation step %q requires validation configuration", step.DisplayName)}
	}
	if step.ValidationGroup != nil || step.Action != "" || step.Reference != nil ||
		step.Value != "" || len(step.Values) != 0 || step.WaitKind != "" || step.WaitMS != 0 ||
		step.RepeatCount != 0 || len(step.Children) != 0 || step.Optional {
		problems = append(problems, fmt.Sprintf("validation step %q contains unsupported action or child configuration", step.DisplayName))
	}
	if strings.TrimSpace(step.NodeID) == "" || strings.TrimSpace(step.NodeVersionID) == "" {
		problems = append(problems, fmt.Sprintf("validation step %q requires an exact node reference", step.DisplayName))
	}
	if err := step.Validation.Assertion.Validate(); err != nil {
		problems = append(problems, fmt.Sprintf("validation step %q assertion: %v", step.DisplayName, err))
	}
	if err := step.Validation.Wait.Validate(); err != nil {
		problems = append(problems, fmt.Sprintf("validation step %q wait: %v", step.DisplayName, err))
	}
	return problems
}

func validateValidationGroupStep(step WorkflowStep, seen map[string]struct{}) []string {
	var problems []string
	if step.ValidationGroup == nil {
		return []string{fmt.Sprintf("validation group %q requires group configuration", step.DisplayName)}
	}
	if step.Validation != nil || step.Action != "" || step.Reference != nil ||
		step.NodeID != "" || step.NodeVersionID != "" || step.Value != "" || len(step.Values) != 0 ||
		step.WaitKind != "" || step.WaitMS != 0 || step.RepeatCount != 0 || len(step.Children) != 0 || step.Optional {
		problems = append(problems, fmt.Sprintf("validation group %q contains unsupported step configuration", step.DisplayName))
	}
	group := step.ValidationGroup
	if err := group.Wait.Validate(); err != nil {
		problems = append(problems, fmt.Sprintf("validation group %q wait: %v", step.DisplayName, err))
	}
	if len(group.Branches) == 0 || len(group.Branches) > validationMaxBranches {
		problems = append(problems, fmt.Sprintf("validation group %q requires 1-%d branches", step.DisplayName, validationMaxBranches))
	}
	branchIDs := make(map[string]struct{}, len(group.Branches))
	total := 0
	for _, branch := range group.Branches {
		if strings.TrimSpace(branch.ID) == "" || strings.TrimSpace(branch.Name) == "" {
			problems = append(problems, fmt.Sprintf("validation group %q branch id and name are required", step.DisplayName))
		}
		if _, exists := branchIDs[branch.ID]; exists && branch.ID != "" {
			problems = append(problems, fmt.Sprintf("validation group %q has duplicate branch id %q", step.DisplayName, branch.ID))
		}
		branchIDs[branch.ID] = struct{}{}
		if len(branch.Steps) == 0 || len(branch.Steps) > validationMaxBranchSteps {
			problems = append(problems, fmt.Sprintf("validation group %q branch %q requires 1-%d validation steps", step.DisplayName, branch.Name, validationMaxBranchSteps))
		}
		total += len(branch.Steps)
		for _, member := range branch.Steps {
			if strings.TrimSpace(member.ID) == "" || strings.TrimSpace(member.DisplayName) == "" {
				problems = append(problems, fmt.Sprintf("validation group %q member step id and display name are required", step.DisplayName))
			}
			if _, exists := seen[member.ID]; exists && member.ID != "" {
				problems = append(problems, fmt.Sprintf("duplicate step id %q", member.ID))
			}
			seen[member.ID] = struct{}{}
			if member.Kind != StepValidation {
				problems = append(problems, fmt.Sprintf("validation group %q branch %q only accepts VALIDATION steps", step.DisplayName, branch.Name))
				continue
			}
			if member.Validation == nil {
				problems = append(problems, fmt.Sprintf("validation group %q member %q requires validation configuration", step.DisplayName, member.DisplayName))
				continue
			}
			if member.ValidationGroup != nil || member.Action != "" || member.Reference != nil ||
				member.Value != "" || len(member.Values) != 0 || member.WaitKind != "" || member.WaitMS != 0 ||
				member.RepeatCount != 0 || len(member.Children) != 0 || member.Optional {
				problems = append(problems, fmt.Sprintf("validation group %q member %q contains unsupported action or child configuration", step.DisplayName, member.DisplayName))
			}
			if strings.TrimSpace(member.NodeID) == "" || strings.TrimSpace(member.NodeVersionID) == "" {
				problems = append(problems, fmt.Sprintf("validation group %q member %q requires an exact node reference", step.DisplayName, member.DisplayName))
			}
			if err := member.Validation.Assertion.Validate(); err != nil {
				problems = append(problems, fmt.Sprintf("validation group %q member %q assertion: %v", step.DisplayName, member.DisplayName, err))
			}
			if !member.Validation.Wait.isZero() {
				problems = append(problems, fmt.Sprintf("validation group %q member %q must inherit the group wait", step.DisplayName, member.DisplayName))
			}
		}
	}
	if total > validationMaxGroupSteps {
		problems = append(problems, fmt.Sprintf("validation group %q has %d validation steps; maximum is %d", step.DisplayName, total, validationMaxGroupSteps))
	}
	return problems
}
