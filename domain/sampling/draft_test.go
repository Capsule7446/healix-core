package sampling

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func draftFixture() TemporarySamplingWorkflow {
	return TemporarySamplingWorkflow{
		ID: "workflow", DisplayName: "workflow", Properties: automation.Properties{},
		Steps: []automation.FlowFragmentStep{
			{ID: "a", DisplayName: "a", Kind: automation.StepAction, NodeID: "node-a"},
			{ID: "repeat", DisplayName: "repeat", Kind: automation.StepRepeat, Children: []automation.FlowFragmentStep{{ID: "b", DisplayName: "b", Kind: automation.StepAction, NodeID: "node-b"}}},
			{ID: "group", DisplayName: "group", Kind: automation.StepValidationGroup, ValidationGroup: &automation.ValidationGroup{Branches: []automation.ValidationBranch{{ID: "branch", Name: "branch", Steps: []automation.FlowFragmentStep{{ID: "c", DisplayName: "c", Kind: automation.StepValidation, NodeID: "node-c"}}}}}},
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
	inserted, err := InsertDraftStep(original, StepContainer{ParentStepID: "repeat"}, 1, automation.FlowFragmentStep{ID: "d", DisplayName: "d", Kind: automation.StepAction, NodeID: "node-a"})
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
	workflow.Steps = []automation.FlowFragmentStep{
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

func TestUpdateDraftStepCoversEveryNestedContainerAndRebuildsReferences(t *testing.T) {
	tests := []struct {
		name   string
		stepID string
		got    func(TemporarySamplingWorkflow) automation.FlowFragmentStep
	}{
		{
			name:   "root step",
			stepID: "a",
			got:    func(workflow TemporarySamplingWorkflow) automation.FlowFragmentStep { return workflow.Steps[0] },
		},
		{
			name:   "repeat child",
			stepID: "b",
			got: func(workflow TemporarySamplingWorkflow) automation.FlowFragmentStep {
				return workflow.Steps[1].Children[0]
			},
		},
		{
			name:   "validation branch member",
			stepID: "c",
			got: func(workflow TemporarySamplingWorkflow) automation.FlowFragmentStep {
				return workflow.Steps[2].ValidationGroup.Branches[0].Steps[0]
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			original := draftFixture()
			before := draftFixture()
			replacement := test.got(original)
			replacement.DisplayName = "updated " + test.stepID
			replacement.NodeID = "unused"
			replacement.Values = []string{"owned"}

			updated, err := UpdateDraftStep(original, replacement)
			if err != nil {
				t.Fatal(err)
			}
			got := test.got(updated)
			if got.DisplayName != replacement.DisplayName || got.NodeID != "unused" || !reflect.DeepEqual(got.Values, []string{"owned"}) {
				t.Fatalf("updated step = %#v", got)
			}
			if !reflect.DeepEqual(original, before) {
				t.Fatal("update mutated source workflow")
			}
			replacement.Values[0] = "caller mutation"
			if test.got(updated).Values[0] != "owned" {
				t.Fatal("updated workflow aliases replacement input")
			}

			replacementNodeFound := false
			for _, node := range updated.Nodes {
				contains := false
				for _, stepID := range node.StepIDs {
					contains = contains || stepID == test.stepID
				}
				if node.ID == "unused" {
					replacementNodeFound = true
					if !contains {
						t.Fatalf("replacement node references = %v, want %q", node.StepIDs, test.stepID)
					}
				} else if contains {
					t.Fatalf("stale node %q still references %q", node.ID, test.stepID)
				}
			}
			if !replacementNodeFound {
				t.Fatal("replacement node was removed")
			}
		})
	}
}

func TestUpdateDraftStepRejectsInvalidIdentityAndReferenceWithoutMutation(t *testing.T) {
	tests := []struct {
		name        string
		replacement automation.FlowFragmentStep
		wantError   string
	}{
		{
			name:        "blank id",
			replacement: automation.FlowFragmentStep{ID: " \t\n", DisplayName: "blank", Kind: automation.StepAction},
			wantError:   "id is required",
		},
		{
			name:        "unknown id",
			replacement: automation.FlowFragmentStep{ID: "missing", DisplayName: "missing", Kind: automation.StepAction},
			wantError:   "was not found",
		},
		{
			name:        "unknown node",
			replacement: automation.FlowFragmentStep{ID: "a", DisplayName: "invalid", Kind: automation.StepAction, NodeID: "missing"},
			wantError:   "unknown temporary node",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			before := draftFixture()
			got, err := UpdateDraftStep(workflow, test.replacement)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("UpdateDraftStep() error = %v, want containing %q", err, test.wantError)
			}
			if !reflect.DeepEqual(got, TemporarySamplingWorkflow{}) {
				t.Fatalf("rejected update returned partial workflow: %#v", got)
			}
			if !reflect.DeepEqual(workflow, before) {
				t.Fatal("rejected update mutated source workflow")
			}
		})
	}
}

func TestReorderDraftStepsAcceptsEveryExactPermutation(t *testing.T) {
	original := draftFixture()
	before := draftFixture()
	for _, orderedIDs := range permutations([]string{"a", "repeat", "group"}) {
		name := strings.Join(orderedIDs, "-")
		t.Run(name, func(t *testing.T) {
			reordered, err := ReorderDraftSteps(original, StepContainer{}, orderedIDs)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(reordered.Steps))
			for index, step := range reordered.Steps {
				got[index] = step.ID
			}
			if !reflect.DeepEqual(got, orderedIDs) {
				t.Fatalf("steps = %v, want %v", got, orderedIDs)
			}
		})
	}
	if !reflect.DeepEqual(original, before) {
		t.Fatal("reorder permutations mutated source workflow")
	}
}

func TestReorderDraftStepsRejectsNonPermutations(t *testing.T) {
	tests := []struct {
		name       string
		orderedIDs []string
	}{
		{name: "nil", orderedIDs: nil},
		{name: "empty", orderedIDs: []string{}},
		{name: "omission", orderedIDs: []string{"a", "repeat"}},
		{name: "extra", orderedIDs: []string{"a", "repeat", "group", "extra"}},
		{name: "duplicate", orderedIDs: []string{"a", "repeat", "repeat"}},
		{name: "unknown", orderedIDs: []string{"a", "repeat", "missing"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			before := draftFixture()
			got, err := ReorderDraftSteps(workflow, StepContainer{}, test.orderedIDs)
			if err == nil || !strings.Contains(err.Error(), "exact step permutation") {
				t.Fatalf("ReorderDraftSteps() error = %v", err)
			}
			if !reflect.DeepEqual(got, TemporarySamplingWorkflow{}) {
				t.Fatalf("rejected reorder returned partial workflow: %#v", got)
			}
			if !reflect.DeepEqual(workflow, before) {
				t.Fatal("rejected reorder mutated source workflow")
			}
		})
	}
}

func permutations(values []string) [][]string {
	if len(values) == 0 {
		return [][]string{{}}
	}
	var result [][]string
	for index, value := range values {
		rest := append([]string(nil), values[:index]...)
		rest = append(rest, values[index+1:]...)
		for _, suffix := range permutations(rest) {
			permutation := append([]string{value}, suffix...)
			result = append(result, permutation)
		}
	}
	return result
}

func TestDraftCommandsRejectInvalidIdentitiesAndReferences(t *testing.T) {
	workflow := draftFixture()
	if _, err := InsertDraftStep(workflow, StepContainer{}, 0, automation.FlowFragmentStep{ID: "a", DisplayName: "duplicate", Kind: automation.StepAction, NodeID: "node-a"}); err == nil {
		t.Fatal("duplicate step was accepted")
	}
	if _, err := InsertDraftStep(workflow, StepContainer{}, 0, automation.FlowFragmentStep{ID: "unknown", DisplayName: "unknown", Kind: automation.StepAction, NodeID: "missing"}); err == nil {
		t.Fatal("unknown node reference was accepted")
	}
	if _, err := DeleteDraftNode(workflow, "node-a"); err == nil {
		t.Fatal("referenced node deletion was accepted")
	}
	if _, err := ReorderDraftSteps(workflow, StepContainer{}, []string{"a", "repeat"}); err == nil {
		t.Fatal("partial reorder was accepted")
	}
	if _, err := InsertDraftStep(workflow, StepContainer{ParentStepID: "a"}, 0, automation.FlowFragmentStep{ID: "child", DisplayName: "child", Kind: automation.StepAction, NodeID: "node-a"}); err == nil {
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
