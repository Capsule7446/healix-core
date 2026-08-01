package scheduling

import (
	"errors"
	"fmt"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

type buildExecutionPlanInput struct {
	RunID       string
	Publication automation.ResolvedExecutionFlow
	Entries     []executionEntryInput
}
type executionEntryInput struct {
	ExecutionID, TestTaskItemID       string
	SequenceNumber                    int
	FlowFragmentID, WorkflowVersionID string
	ParameterSnapshot                 parameterSnapshotInput
}
type parameterSnapshotInput struct {
	IsPresent     bool
	ID            string
	SchemaVersion int
	Values        map[string]parameter.Value
}

func buildExecutionDraft(input buildExecutionPlanInput) (execution.PlanSnapshot, error) {
	if err := input.Publication.Validate(); err != nil {
		// ResolvedExecutionFlow.Validate() always returns nil or its own classified
		// fault; wrapping it in an uncoded "invalid publication: %w" buried that
		// classification behind an unpublished code at this boundary.
		return execution.PlanSnapshot{}, err
	}
	if input.RunID == "" {
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
		entry := execution.Entry{ExecutionID: item.ExecutionID, TestTaskItemID: item.TestTaskItemID, SequenceNumber: item.SequenceNumber, FlowFragmentID: item.FlowFragmentID, WorkflowVersionID: item.WorkflowVersionID}
		if item.ParameterSnapshot.IsPresent {
			entry.Parameters = execution.ParameterSnapshot{ID: item.ParameterSnapshot.ID, SchemaVersion: item.ParameterSnapshot.SchemaVersion, WorkflowVersionID: item.WorkflowVersionID, Values: cloneParameterValues(item.ParameterSnapshot.Values)}
		}
		entries[i] = entry
	}
	draft := execution.PlanSnapshot{RunID: input.RunID, FailurePolicy: policy, Entries: entries, Workflows: workflows, Nodes: mapNodes(input.Publication.Nodes), References: references}
	if err := draft.Validate(); err != nil {
		return execution.PlanSnapshot{}, fmt.Errorf("validate execution draft: %w", err)
	}
	return draft, nil
}
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
	seen := map[string]bool{}
	for i, e := range input.Entries {
		item := items[i]
		if e.TestTaskItemID != item.ID || e.SequenceNumber != item.SequenceNumber || e.FlowFragmentID != item.FlowFragmentID {
			return fmt.Errorf("entry %q does not match task item %q", e.ExecutionID, item.ID)
		}
		if seen[e.ExecutionID] {
			return fmt.Errorf("duplicate execution id %q", e.ExecutionID)
		}
		seen[e.ExecutionID] = true
		dependency, ok := deps[dependencyKey{workflowID: e.FlowFragmentID, versionID: e.WorkflowVersionID}]
		if !ok {
			return fmt.Errorf("entry %q workflow version is unresolved", e.ExecutionID)
		}
		if item.VersionPolicy == automation.FlowFragmentVersionFixed && item.WorkflowVersionID != e.WorkflowVersionID {
			return fmt.Errorf("entry %q fixed version mismatch", e.ExecutionID)
		}
		if item.VersionPolicy == automation.FlowFragmentVersionLatest && !dependency.ResolvedFromLatest {
			return fmt.Errorf("entry %q latest version resolution is missing provenance", e.ExecutionID)
		}
	}
	return nil
}
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
			return fmt.Errorf("workflow execution %q requires a parameter snapshot", entry.ExecutionID)
		}
		if err := validateResolvedParameterValues(definitionsForEntry, entry.ParameterSnapshot.Values); err != nil {
			return fmt.Errorf("test task item %q parameter snapshot: %w", item.ID, err)
		}
	}
	return nil
}
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
func mapNodes(items []automation.ElementTargetDependencySnapshot) []execution.NodeSnapshot {
	r := make([]execution.NodeSnapshot, len(items))
	for i, item := range items {
		r[i] = execution.NodeSnapshot{ElementTargetID: item.ElementTarget.ID, VersionID: item.Version.ID, DisplayName: item.ElementTarget.DisplayName, PageURL: item.Version.PageURL, Origin: item.Version.Origin, Selectors: append([]fingerprint.Selector(nil), item.Version.Selectors...), Fingerprint: item.Version.Fingerprint}
	}
	return r
}
func mapReferences(items []automation.FlowFragmentReferenceResolution) []execution.ReferenceResolution {
	r := make([]execution.ReferenceResolution, len(items))
	for i, item := range items {
		r[i] = execution.ReferenceResolution{ParentVersionID: item.ParentFlowFragmentVersionID, StepID: item.StepID, FlowFragmentID: item.FlowFragmentID, WorkflowVersionID: item.WorkflowVersionID, ResolvedFromLatest: item.ResolvedFromLatest}
	}
	return r
}
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
