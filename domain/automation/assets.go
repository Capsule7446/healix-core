// Package automation 定义持久化版本化自动化资产及其发布规则。
package automation

import (
	"errors"
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"

	"github.com/Capsule7446/healix-core/domain/weburl"
)

// Properties 保存资产的键值属性；复制操作返回独立映射。
type Properties map[string]string

// EnvironmentVariables 保存环境作用域公开的类型化参数值。
// 调用方保留传入映射的所有权，操作返回后可以修改；操作不会修改映射本身。
// 操作读取或复制映射期间不得并发写入，调用方须在外部同步访问。
type EnvironmentVariables map[string]parameter.Value

// Clone 返回独立拥有的映射并复制每个值，包括 MULTI_SELECT 切片。
// nil 接收值规范化为空的非 nil 映射；返回值不与接收值共享可变引用。
// 调用方须避免 Clone 执行期间并发写入接收值。
func (v EnvironmentVariables) Clone() EnvironmentVariables {
	out := make(EnvironmentVariables, len(v))
	for name, value := range v {
		out[name] = value.Clone()
	}
	return out
}

// Validate 校验每个变量名和类型化值，不取得或修改映射所有权。
// 调用方须避免校验期间并发写入；该方法返回内部普通 Go 错误。
// Environment.Validate 将该错误转换为 AUTOMATION_ENVIRONMENT_INVALID 聚合违规，
// 因此变量名不会写入公共错误文本。
func (v EnvironmentVariables) Validate() error {
	// 按变量名排序后校验，使违规顺序不依赖映射迭代顺序。
	names := make([]string, 0, len(v))
	for name := range v {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := parameter.ValidateName(name); err != nil {
			return fmt.Errorf("environment variable name: %w", err)
		}
		if err := v[name].Validate(); err != nil {
			return fmt.Errorf("environment variable: %w", err)
		}
	}
	return nil
}

// isElementWaitKind 判断等待类型是否属于元素等待枚举。
func isElementWaitKind(kind string) bool {
	return kind == "element" || kind == "element_visible" || kind == "element_invisible"
}

// Clone 返回属性映射的独立副本，不与接收值共享底层映射。
func (p Properties) Clone() Properties {
	out := make(Properties, len(p))
	for key, value := range p {
		out[key] = value
	}
	return out
}

// Validate 校验属性键不为空，失败时返回普通错误。
func (p Properties) Validate() error {
	for key := range p {
		if strings.TrimSpace(key) == "" {
			return errors.New("property key is required")
		}
	}
	return nil
}

// VersionSource 标识发布版本的来源。
type VersionSource string

const (
	// SourceManual 表示手工创建的版本。
	SourceManual VersionSource = "MANUAL"
	// SourceSampling 表示采样创建的版本。
	SourceSampling VersionSource = "SAMPLING"
	// SourceAutoHeal 表示自动修复创建的版本。
	SourceAutoHeal VersionSource = "AUTO_HEAL"
)

// Validate 校验来源是否属于支持的枚举值；失败时返回普通错误，由聚合边界负责归类。
func (s VersionSource) Validate() error {
	switch s {
	case SourceManual, SourceSampling, SourceAutoHeal:
		return nil
	default:
		return errors.New("unsupported version source")
	}
}

// ElementTarget 表示元素目标的稳定元数据和当前版本指针。
type ElementTarget struct {
	ID          string
	DisplayName string
	Properties  Properties
	// Deprecated: 文件夹层级移交宿主，这个反向引用随之退役。它从未被校验到 FolderForest 上，
	// 也从未进入冻结快照；见 docs/contracts/retirement-plan.md。
	FolderID         string
	CurrentVersionID string
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
	Revision         Revision
}

// ElementTargetVersion 表示元素目标的一份不可变版本内容。
type ElementTargetVersion struct {
	ID              string
	ElementTargetID string
	VersionNumber   int
	PageURL         string
	Origin          string
	Selectors       []fingerprint.Selector
	Fingerprint     fingerprint.Fingerprint
	Source          VersionSource
	CreatedAt       int64
	DeletedAt       int64
}

// ElementTargetAggregate 持有元素目标元数据、当前版本及完整版本历史。
type ElementTargetAggregate struct {
	ElementTarget ElementTarget
	Current       ElementTargetVersion
	Versions      []ElementTargetVersion
}

// ValidateFor 校验从元素目标历史中选定的确切不可变版本。
// 它临时将选定版本作为当前版本执行同一套内容规则，但不要求该版本仍是实时指针指向的版本，
// 因为历史版本可以被 REUSE 发布路径重新使用。
// 选定版本仍会检查选择器、指纹、版本号和来源，避免绕过聚合校验进入不可变发布内容。
func (v ElementTargetVersion) ValidateFor(target ElementTarget) error {
	selected := target
	selected.CurrentVersionID = v.ID
	return (ElementTargetAggregate{ElementTarget: selected, Current: v}).Validate()
}

// Validate 校验元素目标身份、当前版本、选择器、指纹和版本来源，并按字段顺序返回聚合错误。
func (a ElementTargetAggregate) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(a.ElementTarget.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "element target id is required"))
	}
	if strings.TrimSpace(a.ElementTarget.DisplayName) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "displayName", "display name is required"))
	}
	if a.ElementTarget.Properties.hasInvalidKey() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "properties", "property key is required"))
	}
	if strings.TrimSpace(a.Current.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "currentVersionId", "current version id is required"))
	}
	if a.ElementTarget.CurrentVersionID != a.Current.ID {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "currentVersionId", "current version pointer must match the current version"))
	}
	if a.Current.ElementTargetID != a.ElementTarget.ID {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "current.elementTargetId", "current version must belong to this element target"))
	}
	if a.Current.VersionNumber < 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "current.versionNumber", "version number must be at least 1"))
	}
	if len(a.Current.Selectors) == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "current.selectors", "at least one selector is required"))
	}
	for index, selector := range a.Current.Selectors {
		if err := selector.Validate(); err != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, fmt.Sprintf("current.selectors.%d", index), "selector is invalid"))
		}
	}
	if strings.TrimSpace(a.Current.Fingerprint.Tag) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "current.fingerprint.tag", "fingerprint tag is required"))
	}
	if a.Current.Fingerprint.Attributes == nil {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "current.fingerprint.attributes", "fingerprint attributes are required"))
	}
	// 指纹的兄弟索引和框架栈必须在发布边界再次校验，确保不可变版本不会保存无效内容。
	// 这些字段由采样输入携带，聚合校验不得因版本复用路径而跳过。
	if a.Current.Fingerprint.SiblingIndex < 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "current.fingerprint.siblingIndex", "fingerprint sibling index must not be negative"))
	}
	if err := a.Current.Fingerprint.Framework.Validate(); err != nil {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "current.fingerprint.framework", "fingerprint framework stack is invalid"))
	}
	if err := a.Current.Source.Validate(); err != nil {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "current.source", "version source is unsupported"))
	}
	if len(violations) > 0 {
		return elementTargetInvalidError(violations...)
	}
	return nil
}

// hasInvalidKey 判断属性映射中是否存在去除空白后为空的键。
func (p Properties) hasInvalidKey() bool {
	for key := range p {
		if strings.TrimSpace(key) == "" {
			return true
		}
	}
	return false
}

// Environment 表示可复用的环境配置及其生命周期字段。
type Environment struct {
	ID          string
	DisplayName string
	BaseURL     string
	Variables   EnvironmentVariables
	CreatedAt   int64
	UpdatedAt   int64
	DeletedAt   int64
	Revision    Revision
}

// Validate 校验环境身份、时间、基础 URL 和类型化变量，并在失败时返回聚合错误。
func (e Environment) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(e.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "environment id is required"))
	}
	if strings.TrimSpace(e.DisplayName) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "displayName", "display name is required"))
	}
	// 基础 URL 的协议、凭据和绝对地址约束分别报告，便于调用方定位字段原因。
	// 所有分支同时拒绝不符合 weburl 共享规则的控制字符和地址形状。
	if baseURL := strings.TrimSpace(e.BaseURL); baseURL != "" {
		switch weburl.Check(baseURL) {
		case weburl.Accepted:
		case weburl.RejectScheme:
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "baseUrl", "base URL must use HTTP or HTTPS"))
		case weburl.RejectUserinfo:
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "baseUrl", "base URL cannot contain credentials"))
		default:
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "baseUrl", "base URL must be an absolute HTTP(S) URL"))
		}
	}
	if err := e.Variables.Validate(); err != nil {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "variables", "environment variables are invalid"))
	}
	if len(violations) > 0 {
		return environmentInvalidError(violations...)
	}
	return nil
}

// ParameterDefinition 定义流程片段参数的名称、类型、约束和默认值。
type ParameterDefinition struct {
	Name        string
	DisplayName string
	Description string
	Type        parameter.Type
	Required    bool
	Default     parameter.OptionalValue
	Options     []string
}

// Validate 校验参数定义及默认值，并将结构错误归入 AUTOMATION_FLOW_FRAGMENT_INVALID。
// 选项、类型和默认值不写入公共文本；值自身的 PARAMETER_* 错误保持原码。
func (p ParameterDefinition) Validate() error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.DisplayName) == "" {
		return flowFragmentInvalidError(mustViolation(fault.CodeFieldRequired, "definition.name", "parameter name and display name are required"))
	}
	switch p.Type {
	case parameter.Text, parameter.Number, parameter.Boolean:
		if len(p.Options) != 0 {
			return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "definition.options", "non-select parameter cannot declare options"))
		}
	case parameter.SingleSelect, parameter.MultiSelect:
		if len(p.Options) == 0 {
			return flowFragmentInvalidError(mustViolation(fault.CodeFieldRequired, "definition.options", "select parameter requires options"))
		}
		seen := make(map[string]struct{}, len(p.Options))
		for _, option := range p.Options {
			if strings.TrimSpace(option) == "" {
				return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "definition.options", "select parameter options cannot be empty"))
			}
			if _, exists := seen[option]; exists {
				return flowFragmentInvalidError(mustViolation(fault.CodeFieldDuplicate, "definition.options", "select parameter has a duplicate option"))
			}
			seen[option] = struct{}{}
		}
	default:
		return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "definition.type", "unsupported parameter type"))
	}
	value, present := p.Default.Value()
	if p.Required && present {
		return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "definition.default", "required parameter cannot declare a default"))
	}
	if !p.Required && !present {
		return flowFragmentInvalidError(mustViolation(fault.CodeFieldRequired, "definition.default", "optional parameter requires a default"))
	}
	if present {
		return p.ValidateValue(value)
	}
	return nil
}

// ValidateValue 校验给定值是否符合参数定义，并将结构不匹配归入流程片段错误。
// 值自身的 Validate 错误保持原错误码并直接返回，不嵌套或改写错误。
func (p ParameterDefinition) ValidateValue(value parameter.Value) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Type() != p.Type {
		return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "definition.type", "parameter value type does not match its declaration"))
	}
	allowed := func(candidate string) bool {
		for _, option := range p.Options {
			if option == candidate {
				return true
			}
		}
		return false
	}
	switch p.Type {
	case parameter.SingleSelect:
		if !allowed(value.SingleSelect()) {
			return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "value", "single-select value is not an allowed option"))
		}
	case parameter.MultiSelect:
		seen := make(map[string]struct{}, len(value.MultiSelect()))
		for _, selected := range value.MultiSelect() {
			if !allowed(selected) {
				return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "value", "multi-select value contains an unknown option"))
			}
			if _, duplicate := seen[selected]; duplicate {
				return flowFragmentInvalidError(mustViolation(fault.CodeFieldDuplicate, "value", "multi-select value contains a duplicate option"))
			}
			seen[selected] = struct{}{}
		}
	}
	return nil
}

// StepKind 标识流程步骤的种类。
type StepKind string

const (
	// StepAction 表示动作步骤。
	StepAction StepKind = "ACTION"
	// StepWait 表示等待步骤。
	StepWait StepKind = "WAIT"
	// StepRepeat 表示重复步骤。
	StepRepeat StepKind = "REPEAT"
	// StepFlowFragmentRef 表示流程片段引用步骤。
	StepFlowFragmentRef StepKind = "WORKFLOW_REF"
	// StepValidation 表示验证步骤。
	StepValidation StepKind = "VALIDATION"
	// StepValidationGroup 表示验证组步骤。
	StepValidationGroup StepKind = "VALIDATION_GROUP"
)

// FlowFragmentReference 描述流程片段引用及其参数绑定。
type FlowFragmentReference struct {
	FlowFragmentID    string
	WorkflowVersionID string
	LatestPublished   bool
	ParameterBindings map[string]parameter.Binding
}

// FlowFragmentStep 表示流程片段中的一个步骤及其子步骤。
type FlowFragmentStep struct {
	ID          string
	DisplayName string
	Kind        StepKind
	// CaptureScreenshot 属于不可变的 FlowFragmentVersion 定义。它仅表达用户的意图；浏览器捕获和文件输出保留在主机执行基础设施中。
	CaptureScreenshot bool
	Action            string
	ElementTargetID   string
	// ElementTargetVersionID 是不可变的 FlowFragmentVersion 定义的一部分。  它故意位于 ElementTargetID 旁边，因为单次运行可能合法地包含同一稳定节点的两个版本。
	ElementTargetVersionID string
	// Value 和 Values 保存普通字面量或插值后的流程输入。
	Value       string
	Values      []string
	WaitKind    string
	WaitMS      int
	RepeatCount int
	Optional    bool
	// 验证是一流验证步骤的一个语义断言。验证从不执行任何操作并拥有其等待策略。
	Validation *ValidationConfig
	// 仅当 Kind 为 StepValidationGroup 时才会填充 ValidationGroup。其分支是一级OR；每个分支都包含与 AND 组合的验证步骤。
	ValidationGroup *ValidationGroup
	Reference       *FlowFragmentReference
	Children        []FlowFragmentStep
}

// FlowFragmentContent 保存流程片段步骤树和参数定义。
type FlowFragmentContent struct {
	Steps      []FlowFragmentStep
	Parameters []ParameterDefinition
}

// FlowFragment 表示流程片段的稳定元数据和当前版本指针。
type FlowFragment struct {
	ID          string
	DisplayName string
	Properties  Properties
	// Deprecated: 文件夹层级移交宿主，这个反向引用随之退役。它从未被校验到 FolderForest 上，
	// 也从未进入冻结快照；见 docs/contracts/retirement-plan.md。
	FolderID         string
	CurrentVersionID string
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
	Revision         Revision
}

// FlowFragmentVersion 表示流程片段的一份不可变版本内容。
type FlowFragmentVersion struct {
	ID             string
	FlowFragmentID string
	VersionNumber  int
	Definition     FlowFragmentContent
	CreatedAt      int64
	DeletedAt      int64
}

// FlowFragmentAggregate 持有流程片段元数据、当前版本及完整版本历史。
type FlowFragmentAggregate struct {
	FlowFragment FlowFragment
	Current      FlowFragmentVersion
	Versions     []FlowFragmentVersion
}

// ValidateFor 校验从流程片段历史中选定的确切不可变版本；它不要求所选历史版本仍是稳定资产的当前版本。
func (v FlowFragmentVersion) ValidateFor(workflow FlowFragment) error {
	selected := workflow
	selected.CurrentVersionID = v.ID
	return (FlowFragmentAggregate{FlowFragment: selected, Current: v}).Validate()
}

const (
	maxWorkflowStepDepth = 64
	maxWorkflowStepCount = 10_000
)

type workflowStepFrame struct {
	steps []FlowFragmentStep
	depth int
}

// validateWorkflowStepBounds 校验步骤树的最大深度和总数量。
func validateWorkflowStepBounds(steps []FlowFragmentStep) error {
	stack := []workflowStepFrame{{steps: steps, depth: 1}}
	count := 0
	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if frame.depth > maxWorkflowStepDepth {
			return fmt.Errorf("workflow exceeds maximum nesting depth of %d", maxWorkflowStepDepth)
		}
		if len(frame.steps) > maxWorkflowStepCount-count {
			return fmt.Errorf("workflow exceeds maximum step count of %d", maxWorkflowStepCount)
		}
		count += len(frame.steps)
		for index := len(frame.steps) - 1; index >= 0; index-- {
			if len(frame.steps[index].Children) > 0 {
				stack = append(stack, workflowStepFrame{steps: frame.steps[index].Children, depth: frame.depth + 1})
			}
		}
	}
	return nil
}

// Validate 校验流程片段元数据、步骤树、参数定义和当前版本，并在失败时返回聚合错误。
func (a FlowFragmentAggregate) Validate() error {
	if err := validateWorkflowStepBounds(a.Current.Definition.Steps); err != nil {
		return flowFragmentInvalidError(mustViolation(fault.CodeFieldInvalid, "steps", "flow fragment step tree exceeds its nesting or count limit"))
	}
	var violations []fault.Violation
	if strings.TrimSpace(a.FlowFragment.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "flow fragment id is required"))
	}
	if strings.TrimSpace(a.FlowFragment.DisplayName) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "displayName", "display name is required"))
	}
	if a.FlowFragment.Properties.hasInvalidKey() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "properties", "property key is required"))
	}
	if a.Current.FlowFragmentID != a.FlowFragment.ID || strings.TrimSpace(a.Current.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "current", "flow fragment version must belong to this flow fragment"))
	}
	if a.FlowFragment.CurrentVersionID != a.Current.ID {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "currentVersionId", "current version pointer must match the current version"))
	}
	if a.Current.VersionNumber < 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "current.versionNumber", "version number must be at least 1"))
	}
	seen := make(map[string]struct{})
	if len(a.Current.Definition.Steps) == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps", "flow fragment requires at least one step"))
	}
	var validateSteps func([]FlowFragmentStep, bool)
	validateSteps = func(steps []FlowFragmentStep, root bool) {
		for _, step := range steps {
			if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.DisplayName) == "" {
				violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.identity", "step id and display name are required"))
			}
			if _, exists := seen[step.ID]; exists {
				violations = append(violations, mustViolation(fault.CodeFieldDuplicate, "steps.identity", "step id is duplicated"))
			}
			seen[step.ID] = struct{}{}
			if step.Kind != StepAction && step.Optional {
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.optional", "only an action step can be optional"))
			}
			switch step.Kind {
			case StepAction:
				if step.Validation != nil || step.ValidationGroup != nil {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.action", "an action step cannot carry validation configuration"))
				}
				if step.Reference != nil || step.WaitKind != "" || step.WaitMS != 0 ||
					step.RepeatCount != 0 || len(step.Children) != 0 {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.action", "an action step contains unsupported step configuration"))
				}
				if !supportedAction(step.Action) {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.action", "step action is not supported"))
				}
				if step.Action != "navigate" && step.Action != "press" && strings.TrimSpace(step.ElementTargetID) == "" {
					violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.action.elementTargetId", "step action requires an element target"))
				}
				if strings.TrimSpace(step.ElementTargetID) != "" && strings.TrimSpace(step.ElementTargetVersionID) == "" {
					violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.action.elementTargetVersionId", "step action requires an exact element target version"))
				}
				// navigate 和 press 可以省略元素目标；引用解析、发布、依赖闭包、快照校验和编译均要求非空 ElementTargetID。
				// 因此没有元素目标的步骤不得携带版本 ID；等待、重复和流程片段引用步骤同样拒绝该组合。
				if strings.TrimSpace(step.ElementTargetID) == "" && strings.TrimSpace(step.ElementTargetVersionID) != "" {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.action.elementTargetVersionId", "a step action without an element target cannot carry a version"))
				}
				if (step.Action == "navigate" || step.Action == "press" || step.Action == "extract") && strings.TrimSpace(step.Value) == "" {
					violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.action.value", "step action requires a value"))
				}
				if step.Action == "select" && strings.TrimSpace(step.Value) == "" && len(step.Values) == 0 {
					violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.action.values", "select action requires at least one value"))
				}
			case StepWait:
				if step.Validation != nil || step.ValidationGroup != nil {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.wait", "a wait step cannot carry validation configuration"))
				}
				if step.Action != "" || step.Value != "" || len(step.Values) != 0 || step.RepeatCount != 0 ||
					step.Reference != nil || len(step.Children) != 0 ||
					(!isElementWaitKind(step.WaitKind) && (step.ElementTargetID != "" || step.ElementTargetVersionID != "")) {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.wait", "a wait step contains unsupported step configuration"))
				}
				switch step.WaitKind {
				case "", "sleep":
					if step.WaitMS <= 0 {
						violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.wait.waitMs", "fixed wait duration must be positive"))
					}
				case "element", "element_visible", "element_invisible":
					if strings.TrimSpace(step.ElementTargetID) == "" {
						violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.wait.elementTargetId", "element wait requires an element target"))
					}
					if strings.TrimSpace(step.ElementTargetVersionID) == "" {
						violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.wait.elementTargetVersionId", "element wait requires an exact element target version"))
					}
					if step.WaitMS < 0 {
						violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.wait.waitMs", "wait timeout must not be negative"))
					}
				case "network_idle":
					if step.WaitMS < 0 {
						violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.wait.waitMs", "wait timeout must not be negative"))
					}
				default:
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.wait.waitKind", "wait kind is not supported"))
				}
			case StepRepeat:
				if step.Validation != nil || step.ValidationGroup != nil {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.repeat", "a repeat step cannot carry validation configuration"))
				}
				if step.Action != "" || step.ElementTargetID != "" || step.ElementTargetVersionID != "" || step.Value != "" ||
					len(step.Values) != 0 || step.WaitKind != "" || step.WaitMS != 0 || step.Reference != nil {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.repeat", "a repeat step contains unsupported step configuration"))
				}
				if step.RepeatCount < 1 || len(step.Children) == 0 {
					violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.repeat", "a repeat step requires a positive count and at least one child"))
				}
				validateSteps(step.Children, false)
			case StepFlowFragmentRef:
				if step.Validation != nil || step.ValidationGroup != nil {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.reference", "a flow fragment reference step cannot carry validation configuration"))
				}
				if step.Action != "" || step.ElementTargetID != "" || step.ElementTargetVersionID != "" || step.Value != "" ||
					len(step.Values) != 0 || step.WaitKind != "" || step.WaitMS != 0 ||
					step.RepeatCount != 0 || len(step.Children) != 0 {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.reference", "a flow fragment reference step contains unsupported step configuration"))
				}
				if step.Reference == nil || strings.TrimSpace(step.Reference.FlowFragmentID) == "" {
					violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.reference.flowFragmentId", "a flow fragment reference requires a target"))
				} else if step.Reference.LatestPublished && strings.TrimSpace(step.Reference.WorkflowVersionID) != "" {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.reference.workflowVersionId", "a latest-published reference cannot pin a version"))
				} else if !step.Reference.LatestPublished && strings.TrimSpace(step.Reference.WorkflowVersionID) == "" {
					violations = append(violations, mustViolation(fault.CodeFieldRequired, "steps.reference.workflowVersionId", "a fixed reference requires a version"))
				}
			case StepValidation:
				if !root {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validation", "a validation step must be a root step or a validation-group member"))
				}
				violations = append(violations, validateStandaloneValidationStep(step)...)
			case StepValidationGroup:
				if !root {
					violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.validationGroup", "a validation group must be a root step"))
				}
				violations = append(violations, validateValidationGroupStep(step, seen)...)
			default:
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, "steps.kind", "step kind is not supported"))
			}
		}
	}
	validateSteps(a.Current.Definition.Steps, true)
	parameterNames := make([]string, 0, len(a.Current.Definition.Parameters))
	for index, definition := range a.Current.Definition.Parameters {
		if err := definition.Validate(); err != nil {
			if _, classified := fault.CodeOf(err); classified && !fault.IsCode(err, CodeFlowFragmentInvalid) {
				// 参数默认值自身的 PARAMETER_* 错误保持原码，不改写为泛化的定义错误，
				// 调用方可以据此区分值校验失败和定义结构失败。
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, fmt.Sprintf("definition.parameters.%d", index), "parameter definition value is incompatible with its declaration"))
			} else {
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, fmt.Sprintf("definition.parameters.%d", index), "parameter definition is invalid"))
			}
		}
		parameterNames = append(parameterNames, definition.Name)
	}
	sort.Strings(parameterNames)
	for index := 1; index < len(parameterNames); index++ {
		if parameterNames[index] == parameterNames[index-1] {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, "definition.parameters", "a parameter name is duplicated"))
		}
	}
	if len(violations) > 0 {
		return flowFragmentInvalidError(violations...)
	}
	return nil
}

// supportedAction 判断动作名称是否属于当前支持的动作集合。
func supportedAction(action string) bool {
	switch action {
	case "click", "input", "select", "hover", "navigate", "press", "noop", "extract":
		return true
	default:
		return false
	}
}

// FlowFragmentVersionPolicy 标识流程片段引用的版本选择策略。
type FlowFragmentVersionPolicy string

const (
	// FlowFragmentVersionFixed 表示固定版本策略。
	FlowFragmentVersionFixed FlowFragmentVersionPolicy = "FIXED"
	// FlowFragmentVersionLatest 表示最新版本策略。
	FlowFragmentVersionLatest FlowFragmentVersionPolicy = "LATEST"
)

// Validate 校验版本选择策略是否属于支持的枚举值。
func (p FlowFragmentVersionPolicy) Validate() error {
	switch p {
	case FlowFragmentVersionFixed, FlowFragmentVersionLatest:
		return nil
	default:
		return errors.New("unsupported workflow version policy")
	}
}
