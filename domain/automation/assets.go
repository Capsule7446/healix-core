// Package automation defines durable versioned automation assets and publication rules.
package automation

import (
	"errors"
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"net/url"
	"sort"
	"strings"

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
func (v EnvironmentVariables) Validate() error {
	for name, value := range v {
		if err := parameter.ValidateName(name); err != nil {
			return fmt.Errorf("environment variable name: %w", err)
		}
		if err := value.Validate(); err != nil {
			return fmt.Errorf("environment variable %q: %w", name, err)
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

func (s VersionSource) Validate() error {
	switch s {
	case SourceManual, SourceSampling, SourceAutoHeal:
		return nil
	default:
		return fmt.Errorf("unsupported version source %q", s)
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
	var problems []string
	if strings.TrimSpace(a.ElementTarget.ID) == "" {
		problems = append(problems, "node id is required")
	}
	if strings.TrimSpace(a.ElementTarget.DisplayName) == "" {
		problems = append(problems, "display name is required")
	}
	if err := a.ElementTarget.Properties.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if strings.TrimSpace(a.Current.ID) == "" {
		problems = append(problems, "node version id is required")
	}
	if a.ElementTarget.CurrentVersionID != a.Current.ID {
		problems = append(problems, "node current version pointer must match current version")
	}
	if a.Current.ElementTargetID != a.ElementTarget.ID {
		problems = append(problems, "node version must belong to node")
	}
	if a.Current.VersionNumber < 1 {
		problems = append(problems, "version number must be >= 1")
	}
	if len(a.Current.Selectors) == 0 {
		problems = append(problems, "at least one selector is required")
	}
	for index, selector := range a.Current.Selectors {
		if err := selector.Validate(); err != nil {
			problems = append(problems, fmt.Sprintf("selector %d: %v", index+1, err))
		}
	}
	if strings.TrimSpace(a.Current.Fingerprint.Tag) == "" {
		problems = append(problems, "fingerprint tag is required")
	}
	if a.Current.Fingerprint.Attributes == nil {
		problems = append(problems, "fingerprint attributes are required")
	}
	if err := a.Current.Source.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
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
	var problems []string
	if strings.TrimSpace(e.ID) == "" {
		problems = append(problems, "environment id is required")
	}
	if strings.TrimSpace(e.DisplayName) == "" {
		problems = append(problems, "display name is required")
	}
	if baseURL := strings.TrimSpace(e.BaseURL); baseURL != "" {
		parsed, err := url.ParseRequestURI(baseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			problems = append(problems, "base URL must be an absolute HTTP(S) URL")
		} else if parsed.Scheme != "http" && parsed.Scheme != "https" {
			problems = append(problems, "base URL must use HTTP or HTTPS")
		} else if parsed.User != nil {
			problems = append(problems, "base URL cannot contain credentials")
		}
	}
	if err := e.Variables.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
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

func (p ParameterDefinition) Validate() error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.DisplayName) == "" {
		return errors.New("parameter name and display name are required")
	}
	switch p.Type {
	case parameter.Text, parameter.Number, parameter.Boolean:
		if len(p.Options) != 0 {
			return errors.New("non-select parameter cannot declare options")
		}
	case parameter.SingleSelect, parameter.MultiSelect:
		if len(p.Options) == 0 {
			return errors.New("select parameter requires options")
		}
		seen := make(map[string]struct{}, len(p.Options))
		for _, option := range p.Options {
			if strings.TrimSpace(option) == "" {
				return errors.New("select parameter options cannot be empty")
			}
			if _, exists := seen[option]; exists {
				return fmt.Errorf("select parameter has duplicate option %q", option)
			}
			seen[option] = struct{}{}
		}
	default:
		return fmt.Errorf("unsupported parameter type %q", p.Type)
	}
	value, present := p.Default.Value()
	if p.Required && present {
		return errors.New("required parameter cannot declare a default")
	}
	if !p.Required && !present {
		return errors.New("optional parameter requires a default")
	}
	if present {
		return p.ValidateValue(value)
	}
	return nil
}

func (p ParameterDefinition) ValidateValue(value parameter.Value) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Type() != p.Type {
		return fmt.Errorf("expected %s value, got %s", p.Type, value.Type())
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
			return errors.New("single-select value is not an allowed option")
		}
	case parameter.MultiSelect:
		seen := make(map[string]struct{}, len(value.MultiSelect()))
		for _, selected := range value.MultiSelect() {
			if !allowed(selected) {
				return errors.New("multi-select value contains an unknown option")
			}
			if _, duplicate := seen[selected]; duplicate {
				return errors.New("multi-select value contains a duplicate option")
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
	var problems []string
	if err := validateWorkflowStepBounds(a.Current.Definition.Steps); err != nil {
		return err
	}
	if strings.TrimSpace(a.FlowFragment.ID) == "" {
		problems = append(problems, "workflow id is required")
	}
	if strings.TrimSpace(a.FlowFragment.DisplayName) == "" {
		problems = append(problems, "display name is required")
	}
	if err := a.FlowFragment.Properties.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if a.Current.FlowFragmentID != a.FlowFragment.ID || strings.TrimSpace(a.Current.ID) == "" {
		problems = append(problems, "workflow version must belong to workflow")
	}
	if a.FlowFragment.CurrentVersionID != a.Current.ID {
		problems = append(problems, "workflow current version pointer must match current version")
	}
	if a.Current.VersionNumber < 1 {
		problems = append(problems, "version number must be >= 1")
	}
	seen := make(map[string]struct{})
	if len(a.Current.Definition.Steps) == 0 {
		problems = append(problems, "workflow requires at least one step")
	}
	var validateSteps func([]FlowFragmentStep, bool)
	validateSteps = func(steps []FlowFragmentStep, root bool) {
		for _, step := range steps {
			if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.DisplayName) == "" {
				problems = append(problems, "step id and display name are required")
			}
			if _, exists := seen[step.ID]; exists {
				problems = append(problems, fmt.Sprintf("duplicate step id %q", step.ID))
			}
			seen[step.ID] = struct{}{}
			if step.Kind != StepAction && step.Optional {
				problems = append(problems, fmt.Sprintf("step %q only ACTION can be optional", step.DisplayName))
			}
			switch step.Kind {
			case StepAction:
				if step.Validation != nil || step.ValidationGroup != nil {
					problems = append(problems, fmt.Sprintf("step %q ACTION cannot carry validation configuration", step.DisplayName))
				}
				if step.Reference != nil || step.WaitKind != "" || step.WaitMS != 0 ||
					step.RepeatCount != 0 || len(step.Children) != 0 {
					problems = append(problems, fmt.Sprintf("step %q ACTION contains unsupported step configuration", step.DisplayName))
				}
				if !supportedAction(step.Action) {
					problems = append(problems, fmt.Sprintf("step %q has unsupported action %q", step.DisplayName, step.Action))
				}
				if step.Action != "navigate" && step.Action != "press" && strings.TrimSpace(step.ElementTargetID) == "" {
					problems = append(problems, fmt.Sprintf("step %q requires a node", step.DisplayName))
				}
				if strings.TrimSpace(step.ElementTargetID) != "" && strings.TrimSpace(step.ElementTargetVersionID) == "" {
					problems = append(problems, fmt.Sprintf("step %q requires an exact node version", step.DisplayName))
				}
				if (step.Action == "navigate" || step.Action == "press" || step.Action == "extract") && strings.TrimSpace(step.Value) == "" {
					problems = append(problems, fmt.Sprintf("step %q action %s requires a value", step.DisplayName, step.Action))
				}
				if step.Action == "select" && strings.TrimSpace(step.Value) == "" && len(step.Values) == 0 {
					problems = append(problems, fmt.Sprintf("step %q select requires at least one value", step.DisplayName))
				}
			case StepWait:
				if step.Validation != nil || step.ValidationGroup != nil {
					problems = append(problems, fmt.Sprintf("step %q WAIT cannot carry validation configuration", step.DisplayName))
				}
				if step.Action != "" || step.Value != "" || len(step.Values) != 0 || step.RepeatCount != 0 ||
					step.Reference != nil || len(step.Children) != 0 ||
					(!isElementWaitKind(step.WaitKind) && (step.ElementTargetID != "" || step.ElementTargetVersionID != "")) {
					problems = append(problems, fmt.Sprintf("step %q WAIT contains unsupported step configuration", step.DisplayName))
				}
				switch step.WaitKind {
				case "", "sleep":
					if step.WaitMS <= 0 {
						problems = append(problems, fmt.Sprintf("step %q fixed wait must be > 0", step.DisplayName))
					}
				case "element", "element_visible", "element_invisible":
					if strings.TrimSpace(step.ElementTargetID) == "" {
						problems = append(problems, fmt.Sprintf("step %q element wait requires a node", step.DisplayName))
					}
					if strings.TrimSpace(step.ElementTargetVersionID) == "" {
						problems = append(problems, fmt.Sprintf("step %q element wait requires an exact node version", step.DisplayName))
					}
					if step.WaitMS < 0 {
						problems = append(problems, fmt.Sprintf("step %q timeout must be >= 0", step.DisplayName))
					}
				case "network_idle":
					if step.WaitMS < 0 {
						problems = append(problems, fmt.Sprintf("step %q timeout must be >= 0", step.DisplayName))
					}
				default:
					problems = append(problems, fmt.Sprintf("step %q has unsupported wait kind %q", step.DisplayName, step.WaitKind))
				}
			case StepRepeat:
				if step.Validation != nil || step.ValidationGroup != nil {
					problems = append(problems, fmt.Sprintf("step %q REPEAT cannot carry validation configuration", step.DisplayName))
				}
				if step.Action != "" || step.ElementTargetID != "" || step.ElementTargetVersionID != "" || step.Value != "" ||
					len(step.Values) != 0 || step.WaitKind != "" || step.WaitMS != 0 || step.Reference != nil {
					problems = append(problems, fmt.Sprintf("step %q REPEAT contains unsupported step configuration", step.DisplayName))
				}
				if step.RepeatCount < 1 || len(step.Children) == 0 {
					problems = append(problems, fmt.Sprintf("step %q repeat requires count and children", step.DisplayName))
				}
				validateSteps(step.Children, false)
			case StepFlowFragmentRef:
				if step.Validation != nil || step.ValidationGroup != nil {
					problems = append(problems, fmt.Sprintf("step %q WORKFLOW_REF cannot carry validation configuration", step.DisplayName))
				}
				if step.Action != "" || step.ElementTargetID != "" || step.ElementTargetVersionID != "" || step.Value != "" ||
					len(step.Values) != 0 || step.WaitKind != "" || step.WaitMS != 0 ||
					step.RepeatCount != 0 || len(step.Children) != 0 {
					problems = append(problems, fmt.Sprintf("step %q WORKFLOW_REF contains unsupported step configuration", step.DisplayName))
				}
				if step.Reference == nil || strings.TrimSpace(step.Reference.FlowFragmentID) == "" {
					problems = append(problems, fmt.Sprintf("step %q requires a workflow reference", step.DisplayName))
				} else if step.Reference.LatestPublished && strings.TrimSpace(step.Reference.WorkflowVersionID) != "" {
					problems = append(problems, fmt.Sprintf("step %q latest workflow reference cannot persist a version", step.DisplayName))
				} else if !step.Reference.LatestPublished && strings.TrimSpace(step.Reference.WorkflowVersionID) == "" {
					problems = append(problems, fmt.Sprintf("step %q fixed workflow reference requires a version", step.DisplayName))
				}
			case StepValidation:
				if !root {
					problems = append(problems, fmt.Sprintf("validation step %q must be a root step or validation-group member", step.DisplayName))
				}
				problems = append(problems, validateStandaloneValidationStep(step)...)
			case StepValidationGroup:
				if !root {
					problems = append(problems, fmt.Sprintf("validation group %q must be a root step", step.DisplayName))
				}
				problems = append(problems, validateValidationGroupStep(step, seen)...)
			default:
				problems = append(problems, fmt.Sprintf("step %q has unsupported kind %q", step.DisplayName, step.Kind))
			}
		}
	}
	validateSteps(a.Current.Definition.Steps, true)
	parameterNames := make([]string, 0, len(a.Current.Definition.Parameters))
	for _, parameter := range a.Current.Definition.Parameters {
		if err := parameter.Validate(); err != nil {
			problems = append(problems, fmt.Sprintf("parameter %q: %v", parameter.Name, err))
		}
		parameterNames = append(parameterNames, parameter.Name)
	}
	sort.Strings(parameterNames)
	for index := 1; index < len(parameterNames); index++ {
		if parameterNames[index] == parameterNames[index-1] {
			problems = append(problems, fmt.Sprintf("duplicate parameter %q", parameterNames[index]))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
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
		return fmt.Errorf("unsupported workflow version policy %q", p)
	}
}
