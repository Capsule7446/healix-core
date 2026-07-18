package node

import (
	"context"
	"strings"
	"testing"
)

type bindingCaptureNode struct {
	name string
	got  string
}

func (n *bindingCaptureNode) ID() string { return "capture" }

func (n *bindingCaptureNode) Run(_ context.Context, rt *Runtime) error {
	n.got, _ = rt.Scratchpad[n.name].(string)
	return nil
}

func TestWorkflowCallAppliesAndRestoresParameterScope(t *testing.T) {
	capture := &bindingCaptureNode{name: "child_region"}
	call := &WorkflowCallNode{
		Target:   &WorkflowNode{NodeID: "child", Children: []Node{capture}},
		Bindings: map[string]string{"child_region": "${parent_region}"},
	}
	runtime := &Runtime{Scratchpad: map[string]any{"parent_region": "east", "child_region": "outer", "params.child_region": "outer-prefixed"}}
	if err := call.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if capture.got != "east" {
		t.Fatalf("child value = %q, want east", capture.got)
	}
	if got := runtime.Scratchpad["child_region"]; got != "outer" {
		t.Fatalf("restored child value = %v, want outer", got)
	}
	if got := runtime.Scratchpad["params.child_region"]; got != "outer-prefixed" {
		t.Fatalf("restored prefixed child value = %v, want outer-prefixed", got)
	}
}

func TestWorkflowCallRejectsMissingParentParameter(t *testing.T) {
	call := &WorkflowCallNode{Target: &WorkflowNode{NodeID: "child"},
		Bindings: map[string]string{"child_region": "${missing}"}}
	err := call.Run(context.Background(), &Runtime{Scratchpad: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("Run error = %v, want missing parameter", err)
	}
}

func TestWorkflowCallShadowsSameNamedOptionalParentParameter(t *testing.T) {
	capture := &bindingCaptureNode{name: "region"}
	call := &WorkflowCallNode{NodeID: "ref", Target: &WorkflowNode{NodeID: "child", Children: []Node{capture}},
		Bindings: map[string]string{"region": ""}}
	runtime := &Runtime{Scratchpad: map[string]any{"region": "parent", "params.region": "parent"}}
	if err := call.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if capture.got != "" {
		t.Fatalf("child optional value = %q, want empty child scope", capture.got)
	}
	if runtime.Scratchpad["region"] != "parent" || runtime.Scratchpad["params.region"] != "parent" {
		t.Fatal("parent parameter scope was not restored")
	}
}
