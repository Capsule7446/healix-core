package scheduling

import (
	"errors"
	"fmt"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type BuildExecutionPlanInput struct {
	RunID           string
	Publication     automation.TestTaskVersionPlan
	Entries         []ExecutionEntryInput
	ParameterScopes []ParameterScopeInput
}
type ExecutionEntryInput struct {
	ExecutionID, TestTaskItemID   string
	SequenceNumber                int
	WorkflowID, WorkflowVersionID string
	ParameterSnapshot             ParameterSnapshotInput
}
type ParameterScopeInput struct {
	TestTaskItemID, Path, WorkflowID, WorkflowVersionID string
	Values                                              map[string]any
}
type ParameterSnapshotInput struct {
	IsPresent bool
	ID        string
	Values    map[string]any
}

func BuildExecutionPlan(input BuildExecutionPlanInput) (execution.Plan, error) {
	if err := input.Publication.Validate(); err != nil {
		return execution.Plan{}, fmt.Errorf("invalid publication: %w", err)
	}
	if input.RunID == "" {
		return execution.Plan{}, errors.New("run id is required")
	}
	if err := validateEntries(input); err != nil {
		return execution.Plan{}, err
	}
	if err := rejectUnmappedParameters(input); err != nil {
		return execution.Plan{}, err
	}
	workflows, err := mapWorkflows(input.Publication.Workflows)
	if err != nil {
		return execution.Plan{}, err
	}
	policy, err := mapFailurePolicy(input.Publication.Version.FailurePolicy)
	if err != nil {
		return execution.Plan{}, err
	}
	entries := make([]execution.WorkflowEntry, len(input.Entries))
	for i, item := range input.Entries {
		entries[i] = execution.WorkflowEntry{ExecutionID: item.ExecutionID, TestTaskItemID: item.TestTaskItemID, SequenceNumber: item.SequenceNumber, WorkflowID: item.WorkflowID, WorkflowVersionID: item.WorkflowVersionID}
	}
	plan, err := execution.Seal(execution.Draft{RunID: input.RunID, FailurePolicy: policy, Entries: entries, Workflows: workflows, Nodes: mapNodes(input.Publication.Nodes), References: mapReferences(input.Publication.References)})
	if err != nil {
		return execution.Plan{}, fmt.Errorf("seal execution plan: %w", err)
	}
	return plan, nil
}
func validateEntries(input BuildExecutionPlanInput) error {
	items := input.Publication.Version.Items
	if len(input.Entries) != len(items) {
		return errors.New("entry count must equal publication item count")
	}
	type dependencyKey struct {
		workflowID string
		versionID  string
	}
	deps := make(map[dependencyKey]automation.WorkflowDependencySnapshot, len(input.Publication.Workflows))
	for _, dependency := range input.Publication.Workflows {
		deps[dependencyKey{workflowID: dependency.Workflow.ID, versionID: dependency.Version.ID}] = dependency
	}
	seen := map[string]bool{}
	for i, e := range input.Entries {
		item := items[i]
		if e.TestTaskItemID != item.ID || e.SequenceNumber != item.SequenceNumber || e.WorkflowID != item.WorkflowID {
			return fmt.Errorf("entry %q does not match task item %q", e.ExecutionID, item.ID)
		}
		if seen[e.ExecutionID] {
			return fmt.Errorf("duplicate execution id %q", e.ExecutionID)
		}
		seen[e.ExecutionID] = true
		dependency, ok := deps[dependencyKey{workflowID: e.WorkflowID, versionID: e.WorkflowVersionID}]
		if !ok {
			return fmt.Errorf("entry %q workflow version is unresolved", e.ExecutionID)
		}
		if item.VersionPolicy == automation.WorkflowVersionFixed && item.WorkflowVersionID != e.WorkflowVersionID {
			return fmt.Errorf("entry %q fixed version mismatch", e.ExecutionID)
		}
		if item.VersionPolicy == automation.WorkflowVersionLatest && (!dependency.ResolvedFromLatest || dependency.Workflow.CurrentVersionID != e.WorkflowVersionID) {
			return fmt.Errorf("entry %q latest version mismatch", e.ExecutionID)
		}
	}
	return nil
}
func rejectUnmappedParameters(input BuildExecutionPlanInput) error {
	if len(input.ParameterScopes) > 0 {
		return errors.New("run parameter scopes cannot be mapped losslessly")
	}
	for _, i := range input.Publication.Version.Items {
		if len(i.Parameters) > 0 {
			return fmt.Errorf("test task item %q parameter values cannot be mapped losslessly", i.ID)
		}
	}
	for _, e := range input.Entries {
		if e.ParameterSnapshot.IsPresent || e.ParameterSnapshot.ID != "" || len(e.ParameterSnapshot.Values) > 0 {
			return fmt.Errorf("workflow execution %q parameter snapshot cannot be mapped losslessly", e.ExecutionID)
		}
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
func mapWorkflows(items []automation.WorkflowDependencySnapshot) ([]execution.WorkflowSnapshot, error) {
	r := make([]execution.WorkflowSnapshot, len(items))
	for i, item := range items {
		p, err := mapParameters(item.Version.Definition.Parameters)
		if err != nil {
			return nil, fmt.Errorf("workflow %q parameters: %w", item.Version.ID, err)
		}
		r[i] = execution.WorkflowSnapshot{ID: item.Workflow.ID, VersionID: item.Version.ID, WorkflowID: item.Version.WorkflowID, DisplayName: item.Workflow.DisplayName, VersionNumber: item.Version.VersionNumber, Parameters: p, Steps: mapSteps(item.Version.Definition.Steps)}
	}
	return r, nil
}
func mapParameters(items []automation.ParameterDefinition) ([]execution.Parameter, error) {
	r := make([]execution.Parameter, len(items))
	for i, item := range items {
		if item.Type != automation.ParameterText || item.Required || len(item.Options) != 0 {
			return nil, fmt.Errorf("parameter %q has unsupported semantics that cannot be mapped losslessly", item.Name)
		}
		r[i] = execution.Parameter{Name: item.Name, DefaultValue: item.DefaultValue}
	}
	return r, nil
}
func mapSteps(items []automation.WorkflowStep) []execution.Step {
	r := make([]execution.Step, len(items))
	for i, item := range items {
		s := execution.Step{ID: item.ID, DisplayName: item.DisplayName, Kind: execution.StepKind(item.Kind), CaptureScreenshot: item.CaptureScreenshot, Action: item.Action, NodeID: item.NodeID, NodeVersionID: item.NodeVersionID, Value: item.Value, Values: append([]string(nil), item.Values...), WaitKind: item.WaitKind, WaitMS: item.WaitMS, RepeatCount: item.RepeatCount, Optional: item.Optional, Children: mapSteps(item.Children)}
		if item.Reference != nil {
			s.Reference = &execution.Reference{WorkflowID: item.Reference.WorkflowID, WorkflowVersionID: item.Reference.WorkflowVersionID, ParameterBindings: cloneStringMap(item.Reference.ParameterBindings)}
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
func mapNodes(items []automation.NodeDependencySnapshot) []execution.NodeSnapshot {
	r := make([]execution.NodeSnapshot, len(items))
	for i, item := range items {
		r[i] = execution.NodeSnapshot{NodeID: item.Node.ID, VersionID: item.Version.ID, DisplayName: item.Node.DisplayName, PageURL: item.Version.PageURL, Origin: item.Version.Origin, Selectors: append([]fingerprint.Selector(nil), item.Version.Selectors...), Fingerprint: item.Version.Fingerprint}
	}
	return r
}
func mapReferences(items []automation.WorkflowReferenceResolution) []execution.ReferenceResolution {
	r := make([]execution.ReferenceResolution, len(items))
	for i, item := range items {
		r[i] = execution.ReferenceResolution{ParentVersionID: item.ParentWorkflowVersionID, StepID: item.StepID, WorkflowID: item.WorkflowID, WorkflowVersionID: item.WorkflowVersionID}
	}
	return r
}
func cloneStringMap(s map[string]string) map[string]string {
	if s == nil {
		return nil
	}
	r := make(map[string]string, len(s))
	for k, v := range s {
		r[k] = v
	}
	return r
}
