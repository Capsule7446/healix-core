package sampling

import (
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func draftFixture() TemporarySamplingWorkflow {
	return TemporarySamplingWorkflow{
		ID: "workflow", DisplayName: "workflow", Properties: automation.Properties{},
		Steps: []automation.WorkflowStep{
			{ID: "a", DisplayName: "a", Kind: automation.StepAction, NodeID: "node-a"},
			{ID: "repeat", DisplayName: "repeat", Kind: automation.StepRepeat, Children: []automation.WorkflowStep{{ID: "b", DisplayName: "b", Kind: automation.StepAction, NodeID: "node-b"}}},
			{ID: "group", DisplayName: "group", Kind: automation.StepValidationGroup, ValidationGroup: &automation.ValidationGroup{Branches: []automation.ValidationBranch{{ID: "branch", Name: "branch", Steps: []automation.WorkflowStep{{ID: "c", DisplayName: "c", Kind: automation.StepValidation, NodeID: "node-c"}}}}}},
		},
		Nodes: []TemporarySamplingNode{
			{ID: "node-a", Properties: automation.Properties{}, Selectors: []fingerprint.Selector{}, StepIDs: []string{"a"}},
			{ID: "node-b", Properties: automation.Properties{}, Selectors: []fingerprint.Selector{}, StepIDs: []string{"b"}},
			{ID: "node-c", Properties: automation.Properties{}, Selectors: []fingerprint.Selector{}, StepIDs: []string{"c"}},
			{ID: "unused", Properties: automation.Properties{}},
		},
		ValidationCapturedActionIDs: []string{"a", "b"},
	}
}

func TestDraftCommandsInsertMoveReorderAndDeleteImmutably(t *testing.T) {
	original := draftFixture()
	before := draftFixture()
	inserted, err := InsertDraftStep(original, StepContainer{ParentStepID: "repeat"}, 1, automation.WorkflowStep{ID: "d", DisplayName: "d", Kind: automation.StepAction, NodeID: "node-a"})
	if err != nil || len(inserted.Steps[1].Children) != 2 || inserted.Nodes[0].StepIDs[1] != "d" {
		t.Fatalf("insert = %#v, %v", inserted, err)
	}
	moved, err := MoveDraftStep(inserted, "d", StepContainer{ParentStepID: "group", BranchID: "branch"}, 1)
	if err != nil || moved.Steps[2].ValidationGroup.Branches[0].Steps[1].ID != "d" {
		t.Fatalf("move = %#v, %v", moved, err)
	}
	reordered, err := ReorderDraftSteps(moved, StepContainer{}, []string{"group", "repeat", "a"})
	if err != nil || reordered.Steps[0].ID != "group" {
		t.Fatalf("reorder = %#v, %v", reordered, err)
	}
	deleted, err := DeleteDraftStep(reordered, "a")
	if err != nil || len(deleted.ValidationCapturedActionIDs) != 1 || len(deleted.Nodes[0].StepIDs) != 1 {
		t.Fatalf("delete = %#v, %v", deleted, err)
	}
	if !reflect.DeepEqual(original, before) {
		t.Fatal("draft commands mutated source")
	}
}

func TestMoveDraftStepUsesFinalPositionWithinContainer(t *testing.T) {
	workflow := draftFixture()
	workflow.Steps = []automation.WorkflowStep{
		{ID: "a", DisplayName: "a", Kind: automation.StepAction, NodeID: "node-a"},
		{ID: "b", DisplayName: "b", Kind: automation.StepAction, NodeID: "node-b"},
		{ID: "c", DisplayName: "c", Kind: automation.StepValidation, NodeID: "node-c"},
	}
	for _, test := range []struct {
		name  string
		step  string
		index int
		want  []string
	}{
		{name: "first to end", step: "a", index: 2, want: []string{"b", "c", "a"}},
		{name: "first to middle", step: "a", index: 1, want: []string{"b", "a", "c"}},
		{name: "last to start", step: "c", index: 0, want: []string{"c", "a", "b"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			moved, err := MoveDraftStep(workflow, test.step, StepContainer{}, test.index)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(moved.Steps))
			for index, step := range moved.Steps {
				got[index] = step.ID
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("steps = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDraftCommandsRejectInvalidIdentitiesAndReferences(t *testing.T) {
	workflow := draftFixture()
	if _, err := InsertDraftStep(workflow, StepContainer{}, 0, automation.WorkflowStep{ID: "a", DisplayName: "duplicate", Kind: automation.StepAction, NodeID: "node-a"}); err == nil {
		t.Fatal("duplicate step was accepted")
	}
	if _, err := InsertDraftStep(workflow, StepContainer{}, 0, automation.WorkflowStep{ID: "unknown", DisplayName: "unknown", Kind: automation.StepAction, NodeID: "missing"}); err == nil {
		t.Fatal("unknown node reference was accepted")
	}
	if _, err := DeleteDraftNode(workflow, "node-a"); err == nil {
		t.Fatal("referenced node deletion was accepted")
	}
	if _, err := ReorderDraftSteps(workflow, StepContainer{}, []string{"a", "repeat"}); err == nil {
		t.Fatal("partial reorder was accepted")
	}
	if _, err := InsertDraftStep(workflow, StepContainer{ParentStepID: "a"}, 0, automation.WorkflowStep{ID: "child", DisplayName: "child", Kind: automation.StepAction, NodeID: "node-a"}); err == nil {
		t.Fatal("action child container was accepted")
	}
}

func TestDeleteDraftNodeRemovesUnreferencedNode(t *testing.T) {
	workflow := draftFixture()
	got, err := DeleteDraftNode(workflow, "unused")
	if err != nil || len(got.Nodes) != len(workflow.Nodes)-1 {
		t.Fatalf("delete node = %#v, %v", got, err)
	}
}
