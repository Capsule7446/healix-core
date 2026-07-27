package node

import (
	"context"
	"errors"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"testing"
)

type parameterCaptureNode struct{ got map[string]parameter.Value }

func (n *parameterCaptureNode) ID() string { return "capture" }
func (n *parameterCaptureNode) Run(_ context.Context, rt *Runtime) error {
	n.got = rt.Parameters()
	return nil
}

type failingParameterCaptureNode struct{ parameterCaptureNode }

func (n *failingParameterCaptureNode) Run(ctx context.Context, rt *Runtime) error {
	_ = n.parameterCaptureNode.Run(ctx, rt)
	return errors.New("stop")
}
func TestParameterlessRootClearsAndRestoresStaleScope(t *testing.T) {
	c := &parameterCaptureNode{}
	rt := &Runtime{parameterScope: map[string]parameter.Value{"stale": parameter.TextValue("old")}}
	root := &WorkflowNode{NodeID: "root", OwnsParameterScope: true, Parameters: map[string]parameter.Value{}, Children: []Node{c}}
	if err := root.Run(context.Background(), rt); err != nil {
		t.Fatal(err)
	}
	if len(c.got) != 0 {
		t.Fatalf("stale scope: %#v", c.got)
	}
	if !rt.Parameters()["stale"].Equal(parameter.TextValue("old")) {
		t.Fatal("scope not restored")
	}
}
func TestRootWorkflowInstallsAndRestoresTypedScopeOnSuccessAndFailure(t *testing.T) {
	n, _ := parameter.NewNumberValue("2.50")
	values := map[string]parameter.Value{"text": parameter.TextValue("hello"), "enabled": parameter.BooleanValue(true), "count": n, "regions": parameter.MultiSelectValue([]string{"a,b", "c"})}
	for _, fail := range []bool{false, true} {
		c := &parameterCaptureNode{}
		var child Node = c
		if fail {
			f := &failingParameterCaptureNode{}
			child = f
			c = &f.parameterCaptureNode
		}
		rt := &Runtime{parameterScope: map[string]parameter.Value{"outer": parameter.TextValue("kept")}}
		err := (&WorkflowNode{NodeID: "root", OwnsParameterScope: true, Parameters: values, Children: []Node{child}}).Run(context.Background(), rt)
		if (err != nil) != fail {
			t.Fatalf("error=%v", err)
		}
		if c.got["count"].Number() != "2.5" || len(c.got["regions"].MultiSelect()) != 2 {
			t.Fatalf("scope=%#v", c.got)
		}
		if !rt.Parameters()["outer"].Equal(parameter.TextValue("kept")) {
			t.Fatal("leaked")
		}
	}
}
func TestWorkflowCallExecutesFullyResolvedInvocationDefaultsWithoutBindings(t *testing.T) {
	number, err := parameter.NewNumberValue("-1234567890.2500")
	if err != nil {
		t.Fatal(err)
	}
	capture := &parameterCaptureNode{}
	values := map[string]parameter.Value{
		"number":  number,
		"enabled": parameter.BooleanValue(true),
		"regions": parameter.MultiSelectValue([]string{"北,区", "south"}),
	}
	call := &WorkflowCallNode{
		NodeID: "call", Target: &WorkflowNode{NodeID: "child", Children: []Node{capture}},
		Bindings: map[string]parameter.Binding{}, Values: values,
		Constraints: map[string]parameter.Constraint{
			"number": {Type: parameter.Number}, "enabled": {Type: parameter.Boolean},
			"regions": {Type: parameter.MultiSelect, Options: []string{"北,区", "south"}},
		},
	}
	if err := call.Run(context.Background(), &Runtime{}); err != nil {
		t.Fatal(err)
	}
	if capture.got["number"].Number() != "-1234567890.25" || !capture.got["enabled"].Boolean() || len(capture.got["regions"].MultiSelect()) != 2 {
		t.Fatalf("resolved default scope lost typed values: %#v", capture.got)
	}
}

func TestWorkflowCallKeepsTypedEnvironmentAvailableInNestedScope(t *testing.T) {
	number, err := parameter.NewNumberValue("2.500")
	if err != nil {
		t.Fatal(err)
	}
	capture := &parameterCaptureNode{}
	call := &WorkflowCallNode{
		NodeID:      "call",
		Target:      &WorkflowNode{NodeID: "child", Children: []Node{capture}},
		Values:      map[string]parameter.Value{"child": parameter.TextValue("value")},
		Constraints: map[string]parameter.Constraint{"child": {Type: parameter.Text}},
	}
	environment := map[string]parameter.Value{
		"env.text": parameter.TextValue("east"), "env.number": number,
		"env.boolean": parameter.BooleanValue(true), "env.single": parameter.SingleSelectValue("primary"),
		"env.multi": parameter.MultiSelectValue([]string{"east", "west"}),
	}
	rootCapture := &parameterCaptureNode{}
	root := &WorkflowNode{NodeID: "root", OwnsParameterScope: true, Parameters: environment, Children: []Node{rootCapture, call}}
	if err := root.Run(context.Background(), &Runtime{}); err != nil {
		t.Fatal(err)
	}
	for key, want := range environment {
		if got := rootCapture.got[key]; !got.Equal(want) {
			t.Fatalf("root %s = %#v, want %#v", key, got, want)
		}
		if got := capture.got[key]; !got.Equal(want) {
			t.Fatalf("nested %s = %#v, want %#v", key, got, want)
		}
	}
}

func TestWorkflowCallResolvesTypedParentReferencesWithoutScratchpadFallback(t *testing.T) {
	n, _ := parameter.NewNumberValue("1.20")
	c := &parameterCaptureNode{}
	bindings := map[string]parameter.Binding{"text": parameter.ParentReferenceBinding("ptext"), "number": parameter.ParentReferenceBinding("pnumber"), "boolean": parameter.ParentReferenceBinding("pboolean"), "single": parameter.ParentReferenceBinding("psingle"), "multi": parameter.ParentReferenceBinding("pmulti"), "literal": parameter.LiteralBinding(parameter.TextValue("${x}"))}
	constraints := map[string]parameter.Constraint{"text": {Type: parameter.Text}, "number": {Type: parameter.Number}, "boolean": {Type: parameter.Boolean}, "single": {Type: parameter.SingleSelect, Options: []string{"east"}}, "multi": {Type: parameter.MultiSelect, Options: []string{"a,b", "c"}}, "literal": {Type: parameter.Text}}
	values := map[string]parameter.Value{"text": parameter.TextValue("hello"), "number": n, "boolean": parameter.BooleanValue(true), "single": parameter.SingleSelectValue("east"), "multi": parameter.MultiSelectValue([]string{"a,b", "c"}), "literal": parameter.TextValue("${x}")}
	call := &WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "child", Children: []Node{c}}, Bindings: bindings, Values: values, Constraints: constraints}
	rt := &Runtime{parameterScope: map[string]parameter.Value{"ptext": parameter.TextValue("hello"), "pnumber": n, "pboolean": parameter.BooleanValue(true), "psingle": parameter.SingleSelectValue("east"), "pmulti": parameter.MultiSelectValue([]string{"a,b", "c"}), "x": parameter.TextValue("expanded")}, Scratchpad: map[string]any{"ptext": "contamination"}}
	if err := call.Run(context.Background(), rt); err != nil {
		t.Fatal(err)
	}
	if c.got["literal"].Text() != "${x}" || c.got["number"].Number() != "1.2" || len(c.got["multi"].MultiSelect()) != 2 {
		t.Fatalf("scope=%#v", c.got)
	}
	if rt.Scratchpad["ptext"] != "contamination" {
		t.Fatal("scratchpad changed")
	}
	if rt.Parameters()["ptext"].Text() != "hello" {
		t.Fatal("parent not restored")
	}
}
func TestWorkflowCallRejectsMissingAndMismatchedResolvedValues(t *testing.T) {
	tests := []struct {
		name       string
		values     map[string]parameter.Value
		constraint parameter.Constraint
	}{{"missing", nil, parameter.Constraint{Type: parameter.Text}}, {"type", map[string]parameter.Value{"value": parameter.BooleanValue(true)}, parameter.Constraint{Type: parameter.Text}}, {"option", map[string]parameter.Value{"value": parameter.SingleSelectValue("west")}, parameter.Constraint{Type: parameter.SingleSelect, Options: []string{"east"}}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &WorkflowCallNode{Target: &WorkflowNode{NodeID: "child"}, Values: tt.values, Constraints: map[string]parameter.Constraint{"value": tt.constraint}}
			if err := call.Run(context.Background(), &Runtime{}); err == nil {
				t.Fatal("invalid resolved value accepted")
			}
		})
	}
}
