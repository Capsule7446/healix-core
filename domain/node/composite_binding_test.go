package node

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
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

// TestWorkflowCallResolveBindingsReportsFirstViolationDeterministically closes
// a map-iteration nondeterminism defect: resolveBindings used to range over
// w.Values directly and return on the first problem it found, so which
// binding was reported when more than one was broken depended on Go's
// randomized map order. Sorting names first means "aaa" (missing its value)
// always outranks "ccc" (a constraint violation) alphabetically, regardless of
// iteration order — repeating the build from scratch each time is what would
// expose the old bug, since each fresh map literal gets its own random seed.
func TestWorkflowCallResolveBindingsReportsFirstViolationDeterministically(t *testing.T) {
	build := func() *WorkflowCallNode {
		return &WorkflowCallNode{
			NodeID: "call",
			Target: &WorkflowNode{NodeID: "child"},
			Values: map[string]parameter.Value{
				"bbb": parameter.TextValue("value"),
				"ccc": parameter.BooleanValue(true),
			},
			Constraints: map[string]parameter.Constraint{
				"aaa": {Type: parameter.Text},
				"ccc": {Type: parameter.Text},
			},
		}
	}
	for attempt := 0; attempt < 50; attempt++ {
		_, err := build().resolveBindings()
		if err == nil {
			t.Fatalf("attempt %d: expected a binding violation", attempt)
		}
		descriptor, ok := fault.Describe(err)
		if !ok {
			t.Fatalf("attempt %d: error = %v is not a classified fault", attempt, err)
		}
		violations := descriptor.Violations()
		if len(violations) != 1 {
			t.Fatalf("attempt %d: violations = %#v, want exactly one", attempt, violations)
		}
		if got := violations[0].Code(); got != fault.CodeFieldRequired {
			t.Fatalf("attempt %d: violation code = %s, want %s (the alphabetically-first name's missing binding)", attempt, got, fault.CodeFieldRequired)
		}
	}
}
