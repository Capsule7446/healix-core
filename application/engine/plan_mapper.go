package engine

import (
	"fmt"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/workspace"
)

// CompilePlan is the new execution-context entry point. It accepts only the
// execution vocabulary; legacy workspace plans are translated at the boundary.
func CompilePlan(plan execution.Plan) (CompiledExecution, error) {
	legacy, err := legacyPlan(plan)
	if err != nil {
		return CompiledExecution{}, err
	}
	return CompileExecution(legacy, workspace.WorkflowExecutionPlan{ID: plan.RunID, WorkflowVersionID: plan.RootVersionID})
}

func legacyPlan(plan execution.Plan) (workspace.TestTaskRunPlan, error) {
	if plan.RunID == "" || plan.RootVersionID == "" {
		return workspace.TestTaskRunPlan{}, fmt.Errorf("execution plan requires run and root version identities")
	}
	legacy := workspace.TestTaskRunPlan{}
	for _, workflow := range plan.Workflows {
		steps := make([]workspace.WorkflowStep, 0, len(workflow.Steps))
		for _, step := range workflow.Steps {
			steps = append(steps, toWorkspaceStep(step))
		}
		legacy.Workflows = append(legacy.Workflows, workspace.WorkflowDependencySnapshot{Workflow: workspace.Workflow{ID: workflow.WorkflowID, DisplayName: workflow.DisplayName}, Version: workspace.WorkflowVersion{ID: workflow.VersionID, WorkflowID: workflow.WorkflowID, VersionNumber: workflow.VersionNumber, Definition: workspace.WorkflowDefinition{Steps: steps}}})
	}
	for _, node := range plan.Nodes {
		legacy.Nodes = append(legacy.Nodes, workspace.NodeDependencySnapshot{Node: workspace.Node{ID: node.NodeID, DisplayName: node.DisplayName, CurrentVersionID: node.VersionID}, Version: workspace.NodeVersion{ID: node.VersionID, NodeID: node.NodeID, PageURL: node.PageURL, Origin: node.Origin, Selectors: append([]fingerprint.Selector(nil), node.Selectors...), Fingerprint: node.Fingerprint}})
	}
	for _, ref := range plan.References {
		legacy.References = append(legacy.References, workspace.WorkflowReferenceResolution{ParentWorkflowVersionID: ref.ParentVersionID, StepID: ref.StepID, WorkflowID: ref.WorkflowID, WorkflowVersionID: ref.WorkflowVersionID})
	}
	return legacy, nil
}
