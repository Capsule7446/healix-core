package execution

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestInvocationEdgeKeyDoesNotCollideLikeDelimitedStrings(t *testing.T) {
	left := InvocationEdgeKey{ParentPath: "parent", StepID: "step\x00tail"}
	right := InvocationEdgeKey{ParentPath: "parent\x00step", StepID: "tail"}
	if left == right {
		t.Fatal("distinct concrete invocation edges compare equal")
	}
	edges := map[InvocationEdgeKey]string{left: "left", right: "right"}
	if len(edges) != 2 || edges[left] != "left" || edges[right] != "right" {
		t.Fatalf("edge map collided: %#v", edges)
	}
}

func snapshotWithTwoConcreteReferenceEdges(t *testing.T) RunSnapshotInput {
	input := validRunSnapshotInput(t)
	root := &input.Plan.Workflows[0]
	root.Steps = []Step{{ID: "call", DisplayName: "Call", Kind: WorkflowReference, Reference: &Reference{WorkflowID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]parameter.Binding{"value": parameter.ParentReferenceBinding("count")}}}}
	child := WorkflowSnapshot{ID: "child", WorkflowID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1, Parameters: []Parameter{{Name: "value", DisplayName: "Value", Type: parameter.Number, Required: true}}, Steps: []Step{{ID: "wait-child", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}}
	input.Plan.Workflows = append(input.Plan.Workflows, child)
	input.Plan.References = []ReferenceResolution{{ParentVersionID: "workflow-v2", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1"}}
	item := input.TestTaskVersion.Items[0]
	item.ID = "item-2"
	item.SequenceNumber = 2
	input.TestTaskVersion.Items = append(input.TestTaskVersion.Items, item)
	entry := input.Plan.Entries[0]
	entry.ExecutionID = "entry-2"
	entry.TestTaskItemID = "item-2"
	entry.SequenceNumber = 2
	number, _ := parameter.NewNumberValue("2")
	entry.Parameters.ID = "scope-root-2"
	entry.Parameters.Values = cloneParameterValues(entry.Parameters.Values)
	entry.Parameters.Values["count"] = number
	input.Plan.Entries = append(input.Plan.Entries, entry)
	input.Invocations[0].Bindings = map[string]parameter.Binding{}
	root2 := InvocationScopeSnapshot{Path: "entry-2", WorkflowID: "workflow-1", WorkflowVersionID: "workflow-v2", Values: cloneParameterValues(entry.Parameters.Values), Bindings: map[string]parameter.Binding{}}
	child1 := InvocationScopeSnapshot{Path: "entry-1/call", ParentPath: "entry-1", ParentVersionID: "workflow-v2", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1", Values: map[string]parameter.Value{"value": input.Invocations[0].Values["count"]}, Bindings: cloneBindings(root.Steps[0].Reference.ParameterBindings)}
	child2 := InvocationScopeSnapshot{Path: "entry-2/call", ParentPath: "entry-2", ParentVersionID: "workflow-v2", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1", Values: map[string]parameter.Value{"value": number}, Bindings: cloneBindings(root.Steps[0].Reference.ParameterBindings)}
	input.Invocations = append(input.Invocations, root2, child1, child2)
	return input
}

func TestRunSnapshotUsesConcreteParentPathForRepeatedReferenceEdges(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	if _, err := SealRunSnapshot(input); err != nil {
		t.Fatal(err)
	}
	input = snapshotWithTwoConcreteReferenceEdges(t)
	input.Invocations = input.Invocations[:len(input.Invocations)-1]
	if _, err := SealRunSnapshot(input); err == nil {
		t.Fatal("missing edge accepted")
	}
	input = snapshotWithTwoConcreteReferenceEdges(t)
	input.Invocations = append(input.Invocations, input.Invocations[2])
	input.Invocations[len(input.Invocations)-1].Path = "entry-1/call-duplicate"
	if _, err := SealRunSnapshot(input); err == nil {
		t.Fatal("duplicate concrete edge accepted")
	}
}

func TestRunSnapshotRequiresCompleteConcreteBindingsAndChildValues(t *testing.T) {
	tests := []func(*RunSnapshotInput){func(v *RunSnapshotInput) { v.Invocations[2].Bindings = map[string]parameter.Binding{} }, func(v *RunSnapshotInput) { delete(v.Invocations[2].Values, "value") }, func(v *RunSnapshotInput) { v.Invocations[2].Values["extra"] = parameter.TextValue("x") }}
	for _, mutate := range tests {
		input := snapshotWithTwoConcreteReferenceEdges(t)
		mutate(&input)
		if _, err := SealRunSnapshot(input); err == nil {
			t.Fatal("binding/value divergence accepted")
		}
	}
}

func TestRunSnapshotPermitsConcreteBindingDifferentFromAuthoringMetadata(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	input.Invocations[2].Bindings["value"] = parameter.LiteralBinding(input.Invocations[2].Values["value"])
	if _, err := SealRunSnapshot(input); err != nil {
		t.Fatal(err)
	}
}
