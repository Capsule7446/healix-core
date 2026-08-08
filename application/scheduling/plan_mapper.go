package scheduling

import (
	"errors"
	"fmt"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// buildExecutionPlanInput 汇总构建执行计划草稿所需的实例、已解析发布物和入口输入。
type buildExecutionPlanInput struct {
	InstanceID  execution.InstanceID
	Publication automation.ResolvedExecutionFlow
	Entries     []executionEntryInput
}

// executionEntryInput 描述发布物中一个测试任务入口及其已解析参数快照。
type executionEntryInput struct {
	EntryID                           execution.EntryID
	TestTaskItemID                    string
	SequenceNumber                    int
	FlowFragmentID, WorkflowVersionID string
	ParameterSnapshot                 parameterSnapshotInput
}

// parameterSnapshotInput 携带入口参数快照的存在性、身份、模式版本和值副本。
type parameterSnapshotInput struct {
	IsPresent     bool
	ID            string
	SchemaVersion int
	Values        map[string]parameter.Value
}

// buildExecutionDraft 校验发布物和入口映射，将宿主提供的数据深复制为框架无关的执行计划草稿。
func buildExecutionDraft(input buildExecutionPlanInput) (execution.PlanSnapshot, error) {
	if err := input.Publication.Validate(); err != nil {
		// ResolvedExecutionFlow.Validate() 只返回 nil 或自身已分类的 fault；在此包装为未编码
		// 的 "invalid publication: %w" 会把该分类埋在边界之后，导致未注册错误码成为可见结果。
		return execution.PlanSnapshot{}, err
	}
	if input.InstanceID.Validate() != nil {
		return execution.PlanSnapshot{}, errors.New("run id is required")
	}
	if err := validateEntries(input); err != nil {
		return execution.PlanSnapshot{}, err
	}
	if err := rejectUnmappedParameters(input); err != nil {
		return execution.PlanSnapshot{}, err
	}
	workflows, err := mapWorkflows(input.Publication.Workflows)
	if err != nil {
		return execution.PlanSnapshot{}, err
	}
	references := mapReferences(input.Publication.References)
	if err := applyReferenceResolutions(workflows, references); err != nil {
		return execution.PlanSnapshot{}, err
	}
	policy, err := mapFailurePolicy(input.Publication.Version.FailurePolicy)
	if err != nil {
		return execution.PlanSnapshot{}, err
	}
	entries := make([]execution.Entry, len(input.Entries))
	for i, item := range input.Entries {
		entry := execution.Entry{ID: item.EntryID, TestTaskItemID: item.TestTaskItemID, SequenceNumber: item.SequenceNumber, FlowFragmentID: item.FlowFragmentID, WorkflowVersionID: item.WorkflowVersionID}
		if item.ParameterSnapshot.IsPresent {
			entry.Parameters = execution.ParameterSnapshot{ID: item.ParameterSnapshot.ID, SchemaVersion: item.ParameterSnapshot.SchemaVersion, WorkflowVersionID: item.WorkflowVersionID, Values: cloneParameterValues(item.ParameterSnapshot.Values)}
		}
		entries[i] = entry
	}
	draft := execution.PlanSnapshot{InstanceID: input.InstanceID, FailurePolicy: policy, Entries: entries, Workflows: workflows, Nodes: mapNodes(input.Publication.Nodes), References: references}
	if err := draft.Validate(); err != nil {
		return execution.PlanSnapshot{}, fmt.Errorf("validate execution draft: %w", err)
	}
	return draft, nil
}

// validateEntries 校验入口数量、顺序、身份唯一性以及固定/最新版本解析来源。
func validateEntries(input buildExecutionPlanInput) error {
	items := input.Publication.Version.Items
	if len(input.Entries) != len(items) {
		return errors.New("entry count must equal publication item count")
	}
	type dependencyKey struct {
		workflowID string
		versionID  string
	}
	deps := make(map[dependencyKey]automation.FlowFragmentDependencySnapshot, len(input.Publication.Workflows))
	for _, dependency := range input.Publication.Workflows {
		deps[dependencyKey{workflowID: dependency.FlowFragment.ID, versionID: dependency.Version.ID}] = dependency
	}
	seen := map[execution.EntryID]bool{}
	for i, e := range input.Entries {
		item := items[i]
		if e.TestTaskItemID != item.ID || e.SequenceNumber != item.SequenceNumber || e.FlowFragmentID != item.FlowFragmentID {
			return fmt.Errorf("entry %q does not match task item %q", e.EntryID, item.ID)
		}
		if seen[e.EntryID] {
			return fmt.Errorf("duplicate execution id %q", e.EntryID)
		}
		seen[e.EntryID] = true
		dependency, ok := deps[dependencyKey{workflowID: e.FlowFragmentID, versionID: e.WorkflowVersionID}]
		if !ok {
			return fmt.Errorf("entry %q workflow version is unresolved", e.EntryID)
		}
		if item.VersionPolicy == automation.FlowFragmentVersionFixed && item.WorkflowVersionID != e.WorkflowVersionID {
			return fmt.Errorf("entry %q fixed version mismatch", e.EntryID)
		}
		if item.VersionPolicy == automation.FlowFragmentVersionLatest && !dependency.ResolvedFromLatest {
			return fmt.Errorf("entry %q latest version resolution is missing provenance", e.EntryID)
		}
	}
	return nil
}

// rejectUnmappedParameters 要求定义了参数的工作流入口携带完整且已解析的参数快照。
func rejectUnmappedParameters(input buildExecutionPlanInput) error {
	definitions := make(map[string][]automation.ParameterDefinition, len(input.Publication.Workflows))
	for _, dependency := range input.Publication.Workflows {
		definitions[dependency.Version.ID] = dependency.Version.Definition.Parameters
	}
	for index, entry := range input.Entries {
		item := input.Publication.Version.Items[index]
		definitionsForEntry := definitions[entry.WorkflowVersionID]
		if len(definitionsForEntry) == 0 {
			continue
		}
		if !entry.ParameterSnapshot.IsPresent || entry.ParameterSnapshot.ID == "" || entry.ParameterSnapshot.SchemaVersion < 1 {
			return fmt.Errorf("workflow execution %q requires a parameter snapshot", entry.EntryID)
		}
		if err := validateResolvedParameterValues(definitionsForEntry, entry.ParameterSnapshot.Values); err != nil {
			return fmt.Errorf("test task item %q parameter snapshot: %w", item.ID, err)
		}
	}
	return nil
}

// validateResolvedParameterValues 解析并比较参数值，拒绝缺少定义项的快照。
func validateResolvedParameterValues(definitions []automation.ParameterDefinition, values map[string]parameter.Value) error {
	resolved, err := automation.ResolveParameterValues(definitions, values)
	if err != nil {
		return err
	}
	if !equalParameterValues(resolved, values) {
		return errors.New("parameter snapshot is incomplete")
	}
	return nil
}

// mapFailurePolicy 将自动化上下文的失败策略映射为执行上下文策略。
func mapFailurePolicy(p automation.FailurePolicy) (execution.FailurePolicy, error) {
	switch p {
	case automation.FailurePolicyStopOnFailure:
		return execution.FailurePolicyStopOnFailure, nil
	case automation.FailurePolicyContinueOnFailure:
		return execution.FailurePolicyContinueOnFailure, nil
	default:
		return "", fmt.Errorf("unsupported failure policy %q", p)
	}
}

// mapWorkflows 将已解析工作流依赖映射为执行快照，并深复制参数和步骤数据。
func mapWorkflows(items []automation.FlowFragmentDependencySnapshot) ([]execution.WorkflowSnapshot, error) {
	r := make([]execution.WorkflowSnapshot, len(items))
	for i, item := range items {
		p, err := mapParameters(item.Version.Definition.Parameters)
		if err != nil {
			return nil, fmt.Errorf("workflow %q parameters: %w", item.Version.ID, err)
		}
		r[i] = execution.WorkflowSnapshot{ID: item.FlowFragment.ID, VersionID: item.Version.ID, FlowFragmentID: item.Version.FlowFragmentID, DisplayName: item.FlowFragment.DisplayName, VersionNumber: item.Version.VersionNumber, Parameters: p, Steps: mapSteps(item.Version.Definition.Steps)}
	}
	return r, nil
}

// mapParameters 校验并映射参数定义，同时复制可变选项切片以隔离所有权。
func mapParameters(items []automation.ParameterDefinition) ([]execution.Parameter, error) {
	r := make([]execution.Parameter, len(items))
	for i, item := range items {
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("parameter %q: %w", item.Name, err)
		}
		r[i] = execution.Parameter{Name: item.Name, DisplayName: item.DisplayName, Description: item.Description, Type: item.Type, Required: item.Required, Default: item.Default, Options: append([]string(nil), item.Options...)}
	}
	return r, nil
}

// mapSteps 递归映射步骤树，并复制切片、参数绑定和嵌套断言数据。
func mapSteps(items []automation.FlowFragmentStep) []execution.Step {
	r := make([]execution.Step, len(items))
	for i, item := range items {
		s := execution.Step{ID: item.ID, DisplayName: item.DisplayName, Kind: execution.StepKind(item.Kind), CaptureScreenshot: item.CaptureScreenshot, Action: item.Action, ElementTargetID: item.ElementTargetID, ElementTargetVersionID: item.ElementTargetVersionID, Value: item.Value, Values: append([]string(nil), item.Values...), WaitKind: item.WaitKind, WaitMS: item.WaitMS, RepeatCount: item.RepeatCount, Optional: item.Optional, Children: mapSteps(item.Children)}
		if item.Reference != nil {
			s.Reference = &execution.Reference{FlowFragmentID: item.Reference.FlowFragmentID, WorkflowVersionID: item.Reference.WorkflowVersionID, ParameterBindings: cloneParameterBindings(item.Reference.ParameterBindings)}
		}
		if item.Validation != nil {
			s.Validation = &execution.Validation{Kind: string(item.Validation.Assertion.Kind), Expected: item.Validation.Assertion.Expected, ExpectedValues: append([]string(nil), item.Validation.Assertion.ExpectedValues...), Attribute: item.Validation.Assertion.Attribute, IgnoreCase: item.Validation.Assertion.IgnoreCase, MaxWaitMS: item.Validation.Wait.MaxWaitMS, StabilityMS: item.Validation.Wait.StabilityMS}
		}
		if item.ValidationGroup != nil {
			g := &execution.ValidationGroup{MaxWaitMS: item.ValidationGroup.Wait.MaxWaitMS, StabilityMS: item.ValidationGroup.Wait.StabilityMS, Branches: make([]execution.ValidationBranch, len(item.ValidationGroup.Branches))}
			for j, b := range item.ValidationGroup.Branches {
				g.Branches[j] = execution.ValidationBranch{ID: b.ID, Name: b.Name, Steps: mapSteps(b.Steps)}
			}
			s.ValidationGroup = g
		}
		r[i] = s
	}
	return r
}

// mapNodes 将元素目标依赖映射为执行节点快照，并复制选择器和指纹值。
func mapNodes(items []automation.ElementTargetDependencySnapshot) []execution.NodeSnapshot {
	r := make([]execution.NodeSnapshot, len(items))
	for i, item := range items {
		r[i] = execution.NodeSnapshot{ElementTargetID: item.ElementTarget.ID, VersionID: item.Version.ID, DisplayName: item.ElementTarget.DisplayName, PageURL: item.Version.PageURL, Origin: item.Version.Origin, Selectors: append([]fingerprint.Selector(nil), item.Version.Selectors...), Fingerprint: item.Version.Fingerprint.Clone()}
	}
	return r
}

// mapReferences 将工作流引用解析结果映射为执行引用解析快照。
func mapReferences(items []automation.FlowFragmentReferenceResolution) []execution.ReferenceResolution {
	r := make([]execution.ReferenceResolution, len(items))
	for i, item := range items {
		r[i] = execution.ReferenceResolution{ParentVersionID: item.ParentFlowFragmentVersionID, StepID: item.StepID, FlowFragmentID: item.FlowFragmentID, WorkflowVersionID: item.WorkflowVersionID, ResolvedFromLatest: item.ResolvedFromLatest}
	}
	return r
}

// applyReferenceResolutions 按父版本和步骤 ID 回填引用的已解析工作流版本，并递归处理子步骤。
func applyReferenceResolutions(workflows []execution.WorkflowSnapshot, resolutions []execution.ReferenceResolution) error {
	type referenceKey struct{ parentVersionID, stepID string }
	byKey := make(map[referenceKey]execution.ReferenceResolution, len(resolutions))
	for _, resolution := range resolutions {
		byKey[referenceKey{resolution.ParentVersionID, resolution.StepID}] = resolution
	}
	var walk func(string, []execution.Step) error
	walk = func(parentVersionID string, steps []execution.Step) error {
		for index := range steps {
			if steps[index].Reference != nil {
				resolution, exists := byKey[referenceKey{parentVersionID, steps[index].ID}]
				if !exists {
					return fmt.Errorf("workflow reference %q resolution is missing", steps[index].ID)
				}
				steps[index].Reference.WorkflowVersionID = resolution.WorkflowVersionID
			}
			if err := walk(parentVersionID, steps[index].Children); err != nil {
				return err
			}
		}
		return nil
	}
	for index := range workflows {
		if err := walk(workflows[index].VersionID, workflows[index].Steps); err != nil {
			return err
		}
	}
	return nil
}

// cloneParameterBindings 深复制参数绑定映射；nil 输入保持 nil，非 nil 输入由结果独占所有权。
func cloneParameterBindings(source map[string]parameter.Binding) map[string]parameter.Binding {
	if source == nil {
		return nil
	}
	result := make(map[string]parameter.Binding, len(source))
	for name, binding := range source {
		result[name] = binding.Clone()
	}
	return result
}

// cloneParameterValues 深复制参数值映射；nil 输入保持 nil，避免计划快照与调用方共享可变值。
func cloneParameterValues(source map[string]parameter.Value) map[string]parameter.Value {
	if source == nil {
		return nil
	}
	result := make(map[string]parameter.Value, len(source))
	for key, value := range source {
		result[key] = value.Clone()
	}
	return result
}

// equalParameterValues 比较两个参数值映射的键集合和值内容是否完全相等。
func equalParameterValues(left, right map[string]parameter.Value) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, exists := right[key]
		if !exists || !value.Equal(other) {
			return false
		}
	}
	return true
}
