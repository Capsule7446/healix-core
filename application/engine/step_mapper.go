package engine

import (
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/workspace"
)

func toWorkspaceStep(step execution.Step) workspace.WorkflowStep {
	result := workspace.WorkflowStep{ID: step.ID, DisplayName: step.DisplayName, Kind: workspace.StepKind(step.Kind), CaptureScreenshot: step.CaptureScreenshot, Action: step.Action, NodeID: step.NodeID, NodeVersionID: step.NodeVersionID, Value: step.Value, Values: append([]string(nil), step.Values...), WaitKind: step.WaitKind, WaitMS: step.WaitMS, RepeatCount: step.RepeatCount, Optional: step.Optional}
	if step.Reference != nil {
		result.Reference = &workspace.WorkflowReference{WorkflowID: step.Reference.WorkflowID, WorkflowVersionID: step.Reference.WorkflowVersionID, ParameterBindings: cloneStrings(step.Reference.ParameterBindings)}
	}
	if step.Validation != nil {
		result.Validation = &workspace.ValidationConfig{Assertion: workspace.ValidationAssertion{Kind: workspace.ValidationAssertionKind(step.Validation.Kind), Expected: step.Validation.Expected, ExpectedValues: append([]string(nil), step.Validation.ExpectedValues...), Attribute: step.Validation.Attribute, IgnoreCase: step.Validation.IgnoreCase}, Wait: workspace.ValidationWait{MaxWaitMS: step.Validation.MaxWaitMS, StabilityMS: step.Validation.StabilityMS}}
	}
	if step.ValidationGroup != nil {
		group := &workspace.ValidationGroup{Wait: workspace.ValidationWait{MaxWaitMS: step.ValidationGroup.MaxWaitMS, StabilityMS: step.ValidationGroup.StabilityMS}}
		for _, branch := range step.ValidationGroup.Branches {
			mapped := workspace.ValidationBranch{ID: branch.ID, Name: branch.Name}
			for _, member := range branch.Steps {
				mapped.Steps = append(mapped.Steps, toWorkspaceStep(member))
			}
			group.Branches = append(group.Branches, mapped)
		}
		result.ValidationGroup = group
	}
	for _, child := range step.Children {
		result.Children = append(result.Children, toWorkspaceStep(child))
	}
	return result
}
