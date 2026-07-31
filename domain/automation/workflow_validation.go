package automation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
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

// ValidationAssertionKind 是一种独立于框架的语句，针对一个精确版本化的 ElementTarget 进行评估。框架适配器将 DOM/ARIA 详细信息转换为这些含义；没有特定于框架的选择器或类属于这里。
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

// ValidationAssertion 故意是单一的。验证步骤仅代表一个语句；调用者使用 ValidationGroup 表达合取/析取，而不是在节点中嵌入断言树。
type ValidationAssertion struct {
	Kind     ValidationAssertionKind
	Expected string
	// ExpectedValues 仅由 selected_set_* 使用，表示无序集合。空切片是有意义的：它断言没有选择任何项目。
	ExpectedValues []string
	Attribute      string
	IgnoreCase     bool
}

// Normalized 规范化删除对所选断言类型没有意义的字段。当用户切换类型或浏览器采样器提供语义建议时，适配器会使用它，而验证对原始域输入仍然严格。
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
		// 运行时表达式仅在其变量展开后才进行编译。此处编译 ${env.pattern} 将拒绝有效的持久模板。
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

// ValidationWait 为独立验证节点或整个验证组定义有界等待和连续稳定性窗口。
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

// ValidationConfig 属于 StepValidation。等待仅对独立验证有意义；组成员继承ValidationGroup.Wait。
type ValidationConfig struct {
	Assertion ValidationAssertion
	Wait      ValidationWait
	// Actual/SuggestedKinds 是采样时间编辑器提示，而不是可执行的事实。它们使捕获的语义建议可检查，同时保持持久断言本身独立于 UI 框架。
	Actual         string
	SupportedKinds []ValidationAssertionKind
}

// ValidationBranch 是固定 (AND...) OR (AND...) 组语法中的一个 AND 分支。步骤保留为 FlowFragmentStep 值，以便 DTO 映射器和实现器可以使用一种递归模式，而聚合验证则阻止任何其他类型进入分支。
type ValidationBranch struct {
	ID    string
	Name  string
	Steps []FlowFragmentStep
}

// ValidationGroup 是 AND 分支的一级析取。它的Wait被每个成员节点继承；嵌套组和操作节点无效。
type ValidationGroup struct {
	Wait     ValidationWait
	Branches []ValidationBranch
}

// validateStandaloneValidationStep returns ordered violations rather than a
// joined message. Step and member display names are author-written content
// and never reach the violation text; the recursive step tree carries no flat
// index (existing precedent), so fields describe the aspect that failed.
func validateStandaloneValidationStep(step FlowFragmentStep) []fault.Violation {
	var violations []fault.Violation
	if step.Validation == nil {
		return []fault.Violation{mustViolation(fault.CodeFieldRequired, "steps.validation", "validation step requires validation configuration")}
	}
	if step.ValidationGroup != nil || step.Action != "" || step.Reference != nil ||
		step.Value != "" || len(step.Values) != 0 || step.WaitKind != "" || step.WaitMS != 0 ||
		step.RepeatCount != 0 || len(step.Children) != 0 || step.Optional {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validation", "validation step contains unsupported action or child configuration"))
	}
	if strings.TrimSpace(step.ElementTargetID) == "" || strings.TrimSpace(step.ElementTargetVersionID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.validation.elementTarget", "validation step requires an exact element target reference"))
	}
	if err := step.Validation.Assertion.Validate(); err != nil {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validation.assertion", "validation assertion is invalid"))
	}
	if err := step.Validation.Wait.Validate(); err != nil {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validation.wait", "validation wait is invalid"))
	}
	return violations
}

// validateValidationGroupStep mirrors validateStandaloneValidationStep for a
// validation-group root step and its branch members.
func validateValidationGroupStep(step FlowFragmentStep, seen map[string]struct{}) []fault.Violation {
	if step.ValidationGroup == nil {
		return []fault.Violation{mustViolation(fault.CodeFieldRequired, "steps.validationGroup", "validation group requires group configuration")}
	}
	var violations []fault.Violation
	if step.Validation != nil || step.Action != "" || step.Reference != nil ||
		step.ElementTargetID != "" || step.ElementTargetVersionID != "" || step.Value != "" || len(step.Values) != 0 ||
		step.WaitKind != "" || step.WaitMS != 0 || step.RepeatCount != 0 || len(step.Children) != 0 || step.Optional {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup", "validation group contains unsupported step configuration"))
	}
	group := step.ValidationGroup
	if err := group.Wait.Validate(); err != nil {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.wait", "validation group wait is invalid"))
	}
	if len(group.Branches) == 0 || len(group.Branches) > validationMaxBranches {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.branches", "validation group branch count is unsupported"))
	}
	branchIDs := make(map[string]struct{}, len(group.Branches))
	total := 0
	for _, branch := range group.Branches {
		if strings.TrimSpace(branch.ID) == "" || strings.TrimSpace(branch.Name) == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.validationGroup.branches", "validation group branch id and name are required"))
		}
		if _, exists := branchIDs[branch.ID]; exists && branch.ID != "" {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, "steps.validationGroup.branches", "validation group branch id is duplicated"))
		}
		branchIDs[branch.ID] = struct{}{}
		if len(branch.Steps) == 0 || len(branch.Steps) > validationMaxBranchSteps {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.branches", "validation group branch step count is unsupported"))
		}
		total += len(branch.Steps)
		for _, member := range branch.Steps {
			if strings.TrimSpace(member.ID) == "" || strings.TrimSpace(member.DisplayName) == "" {
				violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.validationGroup.branches.member", "validation group member step id and display name are required"))
			}
			if _, exists := seen[member.ID]; exists && member.ID != "" {
				violations = append(violations, mustViolation(fault.CodeFieldDuplicate, "steps.validationGroup.branches.member", "step id is duplicated"))
			}
			seen[member.ID] = struct{}{}
			if member.Kind != StepValidation {
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.branches.member", "validation group branch members must be validation steps"))
				continue
			}
			if member.Validation == nil {
				violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.validationGroup.branches.member", "validation group member requires validation configuration"))
				continue
			}
			if member.ValidationGroup != nil || member.Action != "" || member.Reference != nil ||
				member.Value != "" || len(member.Values) != 0 || member.WaitKind != "" || member.WaitMS != 0 ||
				member.RepeatCount != 0 || len(member.Children) != 0 || member.Optional {
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.branches.member", "validation group member contains unsupported action or child configuration"))
			}
			if strings.TrimSpace(member.ElementTargetID) == "" || strings.TrimSpace(member.ElementTargetVersionID) == "" {
				violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.validationGroup.branches.member.elementTarget", "validation group member requires an exact element target reference"))
			}
			if err := member.Validation.Assertion.Validate(); err != nil {
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.branches.member.assertion", "validation group member assertion is invalid"))
			}
			if !member.Validation.Wait.isZero() {
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.branches.member.wait", "validation group member must inherit the group wait"))
			}
		}
	}
	if total > validationMaxGroupSteps {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup.branches", "validation group exceeds its maximum total step count"))
	}
	return violations
}
