package node

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

type bindingCaptureNode struct {
	name string
	got  parameter.Value
}

func (n *bindingCaptureNode) ID() string { return "capture" }
func (n *bindingCaptureNode) Run(_ context.Context, rt *Runtime) error {
	n.got = rt.Parameters()[n.name]
	return nil
}

func TestRepeatNestedWorkflowEventsHaveDistinctOccurrences(t *testing.T) {
	facts := &testFacts{}
	runtime := &Runtime{Facts: facts, Scratchpad: map[string]any{}}
	call := &WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "child", Children: []Node{&WorkflowNode{NodeID: "leaf"}}}}
	repeat := &RepeatNode{NodeID: "repeat", Times: 2, Children: []Node{call}}
	if err := repeat.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	occurrences := map[string][]int{}
	for _, event := range facts.events {
		if event.Phase == PhaseRunning {
			occurrences[event.NodeID] = append(occurrences[event.NodeID], event.Occurrence)
		}
	}
	for _, nodeID := range []string{"call", "child", "leaf"} {
		got := occurrences[nodeID]
		if len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Fatalf("%s running occurrences = %v, want [1 2]", nodeID, got)
		}
	}
}

func TestWorkflowCallAppliesAndRestoresTypedParameterScope(t *testing.T) {
	capture := &bindingCaptureNode{name: "child_region"}
	call := &WorkflowCallNode{Target: &WorkflowNode{NodeID: "child", Children: []Node{capture}}, Bindings: map[string]parameter.Binding{"child_region": parameter.LiteralBinding(parameter.TextValue("east"))}, Values: map[string]parameter.Value{"child_region": parameter.TextValue("east")}, Constraints: map[string]parameter.Constraint{"child_region": {Type: parameter.Text}}}
	runtime := &Runtime{Scratchpad: map[string]any{"child_region": "extraction"}, parameterScope: map[string]parameter.Value{"parent": parameter.TextValue("outer")}}
	if err := call.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if !capture.got.Equal(parameter.TextValue("east")) {
		t.Fatalf("child value = %#v", capture.got)
	}
	if !runtime.Parameters()["parent"].Equal(parameter.TextValue("outer")) {
		t.Fatal("parent parameter scope was not restored")
	}
	if runtime.Scratchpad["child_region"] != "extraction" {
		t.Fatal("extraction scratchpad was modified")
	}
}

func TestWorkflowCallShadowsSameNamedParentParameter(t *testing.T) {
	capture := &bindingCaptureNode{name: "region"}
	call := &WorkflowCallNode{NodeID: "ref", Target: &WorkflowNode{NodeID: "child", Children: []Node{capture}}, Bindings: map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.TextValue(""))}, Values: map[string]parameter.Value{"region": parameter.TextValue("")}, Constraints: map[string]parameter.Constraint{"region": {Type: parameter.Text}}}
	runtime := &Runtime{parameterScope: map[string]parameter.Value{"region": parameter.TextValue("parent")}}
	if err := call.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if !capture.got.Equal(parameter.TextValue("")) {
		t.Fatalf("child value = %#v", capture.got)
	}
	if !runtime.Parameters()["region"].Equal(parameter.TextValue("parent")) {
		t.Fatal("parent scope was not restored")
	}
}
