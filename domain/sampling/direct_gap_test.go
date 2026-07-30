package sampling

import (
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
)

func TestUpdateDraftStepDirectNestedReplacementAndRejection(t *testing.T) {
	tests := []struct {
		name, id    string
		replacement automation.FlowFragmentStep
		assert      func(*testing.T, TemporarySamplingWorkflow)
	}{
		{"root", "a", automation.FlowFragmentStep{ID: "a", DisplayName: "root replaced", Kind: automation.StepAction, ElementTargetID: "node-b"}, func(t *testing.T, w TemporarySamplingWorkflow) {
			if w.Steps[0].DisplayName != "root replaced" || !reflect.DeepEqual(w.Nodes[1].StepIDs, []string{"a", "b"}) {
				t.Fatalf("unexpected root update: %#v", w)
			}
		}},
		{"repeat child", "b", automation.FlowFragmentStep{ID: "b", DisplayName: "nested replaced", Kind: automation.StepAction, ElementTargetID: "node-a"}, func(t *testing.T, w TemporarySamplingWorkflow) {
			if w.Steps[1].Children[0].DisplayName != "nested replaced" || !reflect.DeepEqual(w.Nodes[0].StepIDs, []string{"a", "b"}) {
				t.Fatalf("unexpected repeat update: %#v", w)
			}
		}},
		{"validation branch", "c", automation.FlowFragmentStep{ID: "c", DisplayName: "branch replaced", Kind: automation.StepValidation, ElementTargetID: "node-b"}, func(t *testing.T, w TemporarySamplingWorkflow) {
			if w.Steps[2].ValidationGroup.Branches[0].Steps[0].DisplayName != "branch replaced" {
				t.Fatalf("unexpected branch update: %#v", w)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := draftFixture()
			before := draftFixture()
			got, err := UpdateDraftStep(source, tt.replacement)
			if err != nil {
				t.Fatal(err)
			}
			tt.assert(t, got)
			if !reflect.DeepEqual(source, before) {
				t.Fatal("source mutated")
			}
		})
	}
	for _, tt := range []struct {
		name string
		step automation.FlowFragmentStep
	}{{"empty id", automation.FlowFragmentStep{}}, {"unknown", automation.FlowFragmentStep{ID: "missing", DisplayName: "x", Kind: automation.StepAction, ElementTargetID: "node-a"}}, {"unknown node is atomic", automation.FlowFragmentStep{ID: "a", DisplayName: "x", Kind: automation.StepAction, ElementTargetID: "missing"}}, {"duplicate replacement is atomic", automation.FlowFragmentStep{ID: "a", DisplayName: "x", Kind: automation.StepRepeat, Children: []automation.FlowFragmentStep{{ID: "b", DisplayName: "duplicate", Kind: automation.StepAction, ElementTargetID: "node-a"}}}}} {
		t.Run(tt.name, func(t *testing.T) {
			source := draftFixture()
			before := draftFixture()
			got, err := UpdateDraftStep(source, tt.step)
			if err == nil {
				t.Fatalf("accepted invalid update: %#v", got)
			}
			if !reflect.DeepEqual(source, before) {
				t.Fatal("failed update mutated source")
			}
		})
	}
}

func TestRebuildTemporaryNodeReferencesDirectNestedStaleUnknownAndAtomicity(t *testing.T) {
	workflow := draftFixture()
	for i := range workflow.Nodes {
		workflow.Nodes[i].StepIDs = []string{"stale"}
	}
	if err := RebuildTemporaryNodeReferences(&workflow); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"node-a": {"a"}, "node-b": {"b"}, "node-c": {"c"}, "unused": nil}
	for _, node := range workflow.Nodes {
		if !reflect.DeepEqual(node.StepIDs, want[node.ID]) {
			t.Errorf("%s StepIDs=%v want %v", node.ID, node.StepIDs, want[node.ID])
		}
	}

	if err := RebuildTemporaryNodeReferences(nil); err == nil {
		t.Fatal("nil workflow accepted")
	}
	invalid := draftFixture()
	invalid.Nodes[0].StepIDs = []string{"preserve-a"}
	invalid.Nodes[1].StepIDs = []string{"preserve-b"}
	invalid.Steps[1].Children[0].ElementTargetID = "missing"
	beforeStepIDs := make([][]string, len(invalid.Nodes))
	for index := range invalid.Nodes {
		beforeStepIDs[index] = append([]string(nil), invalid.Nodes[index].StepIDs...)
	}
	if err := RebuildTemporaryNodeReferences(&invalid); err == nil {
		t.Fatal("unknown nested node accepted")
	}
	for index := range invalid.Nodes {
		if !reflect.DeepEqual(invalid.Nodes[index].StepIDs, beforeStepIDs[index]) {
			t.Fatalf("failed rebuild changed node %q references: got %v want %v", invalid.Nodes[index].ID, invalid.Nodes[index].StepIDs, beforeStepIDs[index])
		}
	}
}
