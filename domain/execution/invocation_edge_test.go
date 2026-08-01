package execution

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestInvocationEdgeKeyDoesNotCollideLikeDelimitedStrings(t *testing.T) {
	left := InvocationEdgeKey{ParentPath: mustInvocationPath("parent"), StepID: "step\x00tail"}
	right := InvocationEdgeKey{ParentPath: mustInvocationPath("parent\x00step"), StepID: "tail"}
	if left == right {
		t.Fatal("distinct concrete invocation edges compare equal")
	}
	edges := map[InvocationEdgeKey]string{left: "left", right: "right"}
	if len(edges) != 2 || edges[left] != "left" || edges[right] != "right" {
		t.Fatalf("edge map collided: %#v", edges)
	}
}

func snapshotWithTwoConcreteReferenceEdges(t *testing.T) InstanceSnapshotInput {
	input := validRunSnapshotInput(t)
	root := &input.Plan.Workflows[0]
	root.Steps = []Step{{ID: "call", DisplayName: "Call", Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]parameter.Binding{"value": parameter.ParentReferenceBinding("count")}}}}
	child := WorkflowSnapshot{ID: "child", FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1, Parameters: []Parameter{{Name: "value", DisplayName: "Value", Type: parameter.Number, Required: true}}, Steps: []Step{{ID: "call-grandchild", DisplayName: "Call grandchild", Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: "grandchild", WorkflowVersionID: "grandchild-v1"}}}}
	grandchild := WorkflowSnapshot{ID: "grandchild", FlowFragmentID: "grandchild", VersionID: "grandchild-v1", DisplayName: "Grandchild", VersionNumber: 1, Steps: []Step{{ID: "wait-grandchild", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: 1}}}
	input.Plan.Workflows = append(input.Plan.Workflows, child, grandchild)
	input.Plan.References = []ReferenceResolution{
		{ParentVersionID: "workflow-v2", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1"},
		{ParentVersionID: "child-v1", StepID: "call-grandchild", FlowFragmentID: "grandchild", WorkflowVersionID: "grandchild-v1"},
	}
	item := input.ExecutionFlowVersion.Items[0]
	item.ID = "item-2"
	item.SequenceNumber = 2
	input.ExecutionFlowVersion.Items = append(input.ExecutionFlowVersion.Items, item)
	entry := input.Plan.Entries[0]
	entry.ID = mustEntryID("entry-2")
	entry.TestTaskItemID = "item-2"
	entry.SequenceNumber = 2
	number, _ := parameter.NewNumberValue("2")
	entry.Parameters.ID = "scope-root-2"
	entry.Parameters.Values = cloneParameterValues(entry.Parameters.Values)
	entry.Parameters.Values["count"] = number
	input.Plan.Entries = append(input.Plan.Entries, entry)
	input.Invocations[0].Bindings = map[string]parameter.Binding{}
	root2 := InvocationScopeSnapshot{Path: mustInvocationPath("entry-2"), FlowFragmentID: "workflow-1", WorkflowVersionID: "workflow-v2", Values: cloneParameterValues(entry.Parameters.Values), Bindings: map[string]parameter.Binding{}}
	child1 := InvocationScopeSnapshot{Path: mustInvocationPath("entry-1/4:call"), ParentPath: mustInvocationPath("entry-1"), ParentVersionID: "workflow-v2", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1", Values: map[string]parameter.Value{"value": input.Invocations[0].Values["count"]}, Bindings: cloneBindings(root.Steps[0].Reference.ParameterBindings)}
	child2 := InvocationScopeSnapshot{Path: mustInvocationPath("entry-2/4:call"), ParentPath: mustInvocationPath("entry-2"), ParentVersionID: "workflow-v2", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1", Values: map[string]parameter.Value{"value": number}, Bindings: cloneBindings(root.Steps[0].Reference.ParameterBindings)}
	grandchild1 := InvocationScopeSnapshot{Path: mustInvocationPath("entry-1/4:call/15:call-grandchild"), ParentPath: child1.Path, ParentVersionID: "child-v1", StepID: "call-grandchild", FlowFragmentID: "grandchild", WorkflowVersionID: "grandchild-v1", Values: map[string]parameter.Value{}, Bindings: map[string]parameter.Binding{}}
	grandchild2 := InvocationScopeSnapshot{Path: mustInvocationPath("entry-2/4:call/15:call-grandchild"), ParentPath: child2.Path, ParentVersionID: "child-v1", StepID: "call-grandchild", FlowFragmentID: "grandchild", WorkflowVersionID: "grandchild-v1", Values: map[string]parameter.Value{}, Bindings: map[string]parameter.Binding{}}
	input.Invocations = append(input.Invocations, root2, child1, child2, grandchild1, grandchild2)
	return input
}

func TestRunSnapshotRequiresCanonicalChildInvocationPath(t *testing.T) {
	const stepID = "call/阶段:一"

	canonicalInput := snapshotWithTwoConcreteReferenceEdges(t)
	canonicalInput.Plan.Workflows[0].Steps[0].ID = stepID
	canonicalInput.Plan.References[0].StepID = stepID
	for index := range canonicalInput.Invocations {
		invocation := &canonicalInput.Invocations[index]
		if invocation.ParentVersionID != "workflow-v2" {
			continue
		}
		oldPath := invocation.Path
		invocation.StepID = stepID
		// Building the canonical form through Child rather than by hand keeps this
		// case asserting what the validator accepts, not what the test remembers
		// the encoding to be.
		childPath, childErr := invocation.ParentPath.Child(stepID)
		if childErr != nil {
			t.Fatalf("canonical child path: %v", childErr)
		}
		invocation.Path = childPath
		for descendantIndex := range canonicalInput.Invocations {
			descendant := &canonicalInput.Invocations[descendantIndex]
			if descendant.ParentPath == oldPath {
				descendant.ParentPath = invocation.Path
				descendant.Path = mustInvocationPath(invocation.Path.String() + "/15:call-grandchild")
			}
		}
	}
	if _, err := SealInstanceSnapshot(canonicalInput); err != nil {
		t.Fatalf("canonical child invocation path rejected: %v", err)
	}

	forgedInput := snapshotWithTwoConcreteReferenceEdges(t)
	child := &forgedInput.Invocations[2]
	oldChildPath := child.Path
	child.Path = mustInvocationPath("forged-unique-child-path")
	for index := range forgedInput.Invocations {
		grandchild := &forgedInput.Invocations[index]
		if grandchild.ParentPath == oldChildPath {
			grandchild.ParentPath = child.Path
			grandchild.Path = mustInvocationPath(child.Path.String() + "/15:call-grandchild")
		}
	}
	sealed, err := SealInstanceSnapshot(forgedInput)
	requireCreateInstanceSnapshotRejection(t, err, "path is not canonical")
	if sealed.Digest() != "" {
		t.Fatalf("failed seal returned digest %q", sealed.Digest())
	}
}

func TestRunSnapshotUsesConcreteParentPathForRepeatedReferenceEdges(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	if _, err := SealInstanceSnapshot(input); err != nil {
		t.Fatal(err)
	}
	input = snapshotWithTwoConcreteReferenceEdges(t)
	input.Invocations = input.Invocations[:len(input.Invocations)-1]
	if _, err := SealInstanceSnapshot(input); err == nil {
		t.Fatal("missing edge accepted")
	}
	input = snapshotWithTwoConcreteReferenceEdges(t)
	input.Invocations = append(input.Invocations, input.Invocations[2])
	// The segment length has to match the step id it declares, or the path is
	// rejected for being malformed before the duplicate edge is ever reached.
	input.Invocations[len(input.Invocations)-1].Path = mustInvocationPath("entry-1/14:call-duplicate")
	if _, err := SealInstanceSnapshot(input); err == nil {
		t.Fatal("duplicate concrete edge accepted")
	}
}

func TestRunSnapshotRequiresCompleteConcreteBindingsAndChildValues(t *testing.T) {
	tests := []func(*InstanceSnapshotInput){func(v *InstanceSnapshotInput) { v.Invocations[2].Bindings = map[string]parameter.Binding{} }, func(v *InstanceSnapshotInput) { delete(v.Invocations[2].Values, "value") }, func(v *InstanceSnapshotInput) { v.Invocations[2].Values["extra"] = parameter.TextValue("x") }}
	for _, mutate := range tests {
		input := snapshotWithTwoConcreteReferenceEdges(t)
		mutate(&input)
		if _, err := SealInstanceSnapshot(input); err == nil {
			t.Fatal("binding/value divergence accepted")
		}
	}
}

func TestRunSnapshotPermitsConcreteBindingDifferentFromAuthoringMetadata(t *testing.T) {
	input := snapshotWithTwoConcreteReferenceEdges(t)
	input.Invocations[2].Bindings["value"] = parameter.LiteralBinding(input.Invocations[2].Values["value"])
	if _, err := SealInstanceSnapshot(input); err != nil {
		t.Fatal(err)
	}
}
