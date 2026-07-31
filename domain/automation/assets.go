// Package automation defines durable versioned automation assets and publication rules.
package automation

import (
	"errors"
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"net/url"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type Properties map[string]string

// EnvironmentVariables contains the typed values exposed through the env. scope.
// Callers retain ownership of maps passed to domain operations and may modify them
// after the operation returns. They must not modify a map concurrently while a
// domain operation is reading or cloning it; synchronize such access externally.
type EnvironmentVariables map[string]parameter.Value

// Clone returns an independently owned map and clones every value, including
// MULTI_SELECT slices. A nil receiver is normalized to an empty, non-nil map.
// The caller must prevent concurrent writes to the receiver while Clone runs.
func (v EnvironmentVariables) Clone() EnvironmentVariables {
	out := make(EnvironmentVariables, len(v))
	for name, value := range v {
		out[name] = value.Clone()
	}
	return out
}

// Validate checks every variable name and typed value without taking ownership
// or modifying the map. The caller must prevent concurrent writes while it runs.
// It stays an internal, ordinary Go error: it is reachable only through
// Environment.Validate, which owns AUTOMATION_ENVIRONMENT_INVALID and degrades
// this failure into its own violation, so the variable name never reaches
// public text.
func (v EnvironmentVariables) Validate() error {
	for name, value := range v {
		if err := parameter.ValidateName(name); err != nil {
			return fmt.Errorf("environment variable name: %w", err)
		}
		if err := value.Validate(); err != nil {
			return fmt.Errorf("environment variable: %w", err)
		}
	}
	return nil
}

func isElementWaitKind(kind string) bool {
	return kind == "element" || kind == "element_visible" || kind == "element_invisible"
}

func (p Properties) Clone() Properties {
	out := make(Properties, len(p))
	for key, value := range p {
		out[key] = value
	}
	return out
}

func (p Properties) Validate() error {
	for key := range p {
		if strings.TrimSpace(key) == "" {
			return errors.New("property key is required")
		}
	}
	return nil
}

type VersionSource string

const (
	SourceManual   VersionSource = "MANUAL"
	SourceSampling VersionSource = "SAMPLING"
	SourceAutoHeal VersionSource = "AUTO_HEAL"
)

// Validate stays an internal, ordinary Go error: it is reachable only through
// ElementTargetAggregate.Validate, which owns AUTOMATION_ELEMENT_TARGET_INVALID
// and degrades this failure into its own violation, so the out-of-range enum
// value never reaches public text. Direct callers still get a stable substring.
func (s VersionSource) Validate() error {
	switch s {
	case SourceManual, SourceSampling, SourceAutoHeal:
		return nil
	default:
		return errors.New("unsupported version source")
	}
}

type ElementTarget struct {
	ID               string
	DisplayName      string
	Properties       Properties
	FolderID         string
	CurrentVersionID string
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
	Revision         Revision
}

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
	RunUsageCount   int
}

type ElementTargetAggregate struct {
	ElementTarget ElementTarget
	Current       ElementTargetVersion
	Versions      []ElementTargetVersion
}

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
	if err := a.Current.Source.Validate(); err != nil {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "current.source", "version source is unsupported"))
	}
	if len(violations) > 0 {
		return elementTargetInvalidError(violations...)
	}
	return nil
}

// hasInvalidKey reports whether any property key is blank once trimmed. The
// result does not depend on which key is iterated first, so ranging the map
// here does not make the caller's violation order a function of map order.
func (p Properties) hasInvalidKey() bool {
	for key := range p {
		if strings.TrimSpace(key) == "" {
			return true
		}
	}
	return false
}

type Environment struct {
	ID          string
	DisplayName string
	BaseURL     string
	Variables   EnvironmentVariables
	CreatedAt   int64
	UpdatedAt   int64
	DeletedAt   int64
	LastUsedAt  int64
	RunCount    int
	Revision    Revision
}

func (e Environment) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(e.ID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "id", "environment id is required"))
	}
	if strings.TrimSpace(e.DisplayName) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "displayName", "display name is required"))
	}
	if baseURL := strings.TrimSpace(e.BaseURL); baseURL != "" {
		parsed, err := url.ParseRequestURI(baseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "baseUrl", "base URL must be an absolute HTTP(S) URL"))
		} else if parsed.Scheme != "http" && parsed.Scheme != "https" {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "baseUrl", "base URL must use HTTP or HTTPS"))
		} else if parsed.User != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "baseUrl", "base URL cannot contain credentials"))
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

type ParameterDefinition struct {
	Name        string
	DisplayName string
	Description string
	Type        parameter.Type
	Required    bool
	Default     parameter.OptionalValue
	Options     []string
}

// Validate returns an AUTOMATION_FLOW_FRAGMENT_INVALID envelope: parameter
// definitions are flow fragment content, so they carry their own code's
// single-violation short circuits rather than a separate family. Option and
// type values never reach public text; a failing default degrades through
// ValidateValue, which keeps a parameter.Value's own PARAMETER_* code intact.
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

// ValidateValue returns an AUTOMATION_FLOW_FRAGMENT_INVALID envelope for a
// structural mismatch between the value and its declaration. A failing
// value.Validate() keeps its own PARAMETER_* code and passes through unwrapped.
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

type StepKind string

const (
	StepAction          StepKind = "ACTION"
	StepWait            StepKind = "WAIT"
	StepRepeat          StepKind = "REPEAT"
	StepFlowFragmentRef StepKind = "WORKFLOW_REF"
	StepValidation      StepKind = "VALIDATION"
	StepValidationGroup StepKind = "VALIDATION_GROUP"
)

type FlowFragmentReference struct {
	FlowFragmentID    string
	WorkflowVersionID string
	LatestPublished   bool
	ParameterBindings map[string]parameter.Binding
}

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
	// Value and Values are ordinary literal/interpolated workflow input.
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

type FlowFragmentContent struct {
	Steps      []FlowFragmentStep
	Parameters []ParameterDefinition
}

type FlowFragment struct {
	ID               string
	DisplayName      string
	Properties       Properties
	FolderID         string
	CurrentVersionID string
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
	Revision         Revision
}

type FlowFragmentVersion struct {
	ID             string
	FlowFragmentID string
	VersionNumber  int
	Definition     FlowFragmentContent
	CreatedAt      int64
	DeletedAt      int64
	RunUsageCount  int
}

type FlowFragmentAggregate struct {
	FlowFragment FlowFragment
	Current      FlowFragmentVersion
	Versions     []FlowFragmentVersion
}

// ValidateFor 验证从运行快照中选择的确切不可变版本。它故意不声明所选的历史版本仍然是稳定资产的当前版本。
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
				// A parameter default's own PARAMETER_* fault keeps its code rather
				// than being restated as a generic definition failure.
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

func supportedAction(action string) bool {
	switch action {
	case "click", "input", "select", "hover", "navigate", "press", "noop", "extract":
		return true
	default:
		return false
	}
}

type FlowFragmentVersionPolicy string

const (
	FlowFragmentVersionFixed  FlowFragmentVersionPolicy = "FIXED"
	FlowFragmentVersionLatest FlowFragmentVersionPolicy = "LATEST"
)

func (p FlowFragmentVersionPolicy) Validate() error {
	switch p {
	case FlowFragmentVersionFixed, FlowFragmentVersionLatest:
		return nil
	default:
		return errors.New("unsupported workflow version policy")
	}
}
