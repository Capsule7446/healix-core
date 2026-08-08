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

// stepShapeBuilder 收集一个工作流快照步骤树的有序违规项。遇到插值、指纹或参数上下文
// 已分类的 fault 时立即停止并原样传播；其他内部形状错误记录为通用违规，避免私有详情
// 进入公开文本。
type stepShapeBuilder struct {
	violations []fault.Violation
	classified error
}

// done 判断构建器是否已遇到分类错误或达到违规封套上限。
func (b *stepShapeBuilder) done() bool {
	return b.classified != nil || atCap(b.violations)
}

// violation 在构建器未结束时追加一个安全字段违规。
func (b *stepShapeBuilder) violation(code fault.Code, field, message string) {
	if b.done() {
		return
	}
	b.violations = append(b.violations, mustViolation(code, field, message))
}

// absorb 遇到已编码 cause 时记录为短路分类错误，否则丢弃其详情并追加一个通用违规。
// cause 文本可能携带身份或用户输入，丢弃它才能避免进入公开文本。
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

// Validate 通过一个携带有序违规项的聚合封套报告步骤形状失败，或原样传播遍历中遇到的
// 插值、指纹和参数领域 fault。步骤身份、参数名和枚举值不会进入公开文本，递归检查统一
// 降级到未索引的 steps 字段。
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

// fieldIndex 将集合索引拼接为逻辑字段路径。
func fieldIndex(prefix string, index int) string {
	return prefix + "." + strconv.Itoa(index)
}

// validateStepsInto 递归校验步骤树，维护步骤 ID 唯一性并按根/子步骤上下文分派检查。
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

// validateActionInto 校验动作步骤的配置互斥、动作种类、节点身份、值和导航 URL。
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

// validateSealedNavigationURL 拒绝控制字符和 scheme/authority 中的插值，并要求无凭据的
// 绝对 HTTP(S) URL；插值包的已编码错误会原样返回，避免被第二层封套掩盖。
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
	// scheme/authority 已禁止插值，因此此处候选 URL 的 authority 始终是字面值，可以统一
	// 检查主机是否存在。
	if rejection := weburl.Check(parseable); rejection != weburl.Accepted {
		return fmt.Errorf("navigation URL rejected: %s", rejection)
	}
	return nil
}

// validateWaitInto 校验等待步骤的配置、等待种类、节点身份和时间范围。
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

// validateRepeatInto 校验重复步骤的配置互斥、次数上限和子步骤要求。
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

// validateReferenceInto 校验 WORKFLOW_REF 步骤形状，并按排序后的参数绑定名遍历，保证违规
// 顺序只由输入决定。
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

// validateValidationStepInto 校验单项验证步骤及其作为根步骤或分组成员时的等待语义。
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

// validateValidationGroupInto 校验验证组配置、分支唯一性、成员步骤和总步骤上限。
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

// Validate 校验验证配置的种类、插值表达式、期望值、属性和等待窗口；不回显 Kind、Expected
// 或 Attribute 的调用方内容，并原样返回插值包的已分类错误。
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

// validateValidationWait 校验验证最大等待和稳定窗口的范围及先后关系。
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
