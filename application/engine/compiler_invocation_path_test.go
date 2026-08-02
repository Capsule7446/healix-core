package engine

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/node"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// TestCompilePlanStampsEveryStepMetadataWithItsCallSite pins the delivery half of
// the evidence coordinate. Declaring InvocationPath on StepMetadata was never the
// contract; filling it with the invocation scope that owns the step is. Two
// sibling calls to the same child workflow are what give this teeth: a compiler
// that stamped every entry with the entry root would satisfy a mere non-empty
// check while losing the exact thing the coordinate exists to name.
func TestCompilePlanStampsEveryStepMetadataWithItsCallSite(t *testing.T) {
	child := execution.WorkflowSnapshot{FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1,
		Parameters: []execution.Parameter{{Name: "region", DisplayName: "Region", Type: parameter.Text, Required: true}},
		Steps:      []execution.Step{{ID: "click", DisplayName: "Click", Kind: execution.ActionStep, Action: "click", ElementTargetID: compilerNodeID, ElementTargetVersionID: compilerNodeV1}}}
	call := func(id string) execution.Step {
		return execution.Step{ID: id, DisplayName: id, Kind: execution.FlowFragmentReference,
			Reference: &execution.Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1",
				ParameterBindings: map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.TextValue("east"))}}}
	}
	plan := execution.PlanSnapshot{InstanceID: mustInstanceID("exec"), FailurePolicy: execution.FailurePolicyStopOnFailure,
		Entries: []execution.Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "root", WorkflowVersionID: "root-v1"}},
		Workflows: []execution.WorkflowSnapshot{
			{FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "Root", VersionNumber: 1, Steps: []execution.Step{call("first"), call("second")}}, child},
		Nodes: []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "submit")},
		References: []execution.ReferenceResolution{
			{ParentVersionID: "root-v1", StepID: "first", FlowFragmentID: "child", WorkflowVersionID: "child-v1"},
			{ParentVersionID: "root-v1", StepID: "second", FlowFragmentID: "child", WorkflowVersionID: "child-v1"},
		}}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	for runtimeID, metadata := range compiled.Metadata {
		if metadata.InvocationPath == (execution.InvocationPath{}) {
			t.Errorf("metadata %q carries no invocation path", runtimeID)
		}
	}

	root := compiled.program.Root.(*node.WorkflowNode)
	rootPath := execution.RootInvocationPath(mustEntryID("execution-entry"))
	first := root.Children[0].(*node.WorkflowCallNode).Target
	second := root.Children[1].(*node.WorkflowCallNode).Target
	firstClick := compiled.Metadata[first.Children[0].ID()].InvocationPath
	secondClick := compiled.Metadata[second.Children[0].ID()].InvocationPath
	if firstClick == secondClick {
		t.Fatalf("both invocations of the same child report call site %q", firstClick)
	}
	if firstClick == rootPath || secondClick == rootPath {
		t.Fatalf("a nested step reports the entry root as its call site: %q and %q", firstClick, secondClick)
	}
	// The call step itself belongs to the scope that issues it, not to the scope it
	// opens. Getting this backwards would attribute a caller's evidence to the callee.
	if got := compiled.Metadata[root.Children[0].ID()].InvocationPath; got != rootPath {
		t.Fatalf("the calling step left its own scope: %q", got)
	}
	if got := compiled.Metadata[first.ID()].InvocationPath; got == rootPath {
		t.Fatalf("the called workflow stayed in the caller's scope: %q", got)
	}
}
