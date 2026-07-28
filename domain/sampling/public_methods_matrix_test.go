package sampling

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
)

func TestDraftCommandsRejectBoundaryIndexesAndMissingTargetsWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		run  func(TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error)
		want string
	}{
		{
			name: "insert index below zero",
			run: func(workflow TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error) {
				return InsertDraftStep(workflow, StepContainer{}, -1, automation.WorkflowStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, NodeID: "node-a"})
			},
			want: "out of range",
		},
		{
			name: "insert index above length",
			run: func(workflow TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error) {
				return InsertDraftStep(workflow, StepContainer{}, len(workflow.Steps)+1, automation.WorkflowStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, NodeID: "node-a"})
			},
			want: "out of range",
		},
		{
			name: "delete missing step",
			run: func(workflow TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error) {
				return DeleteDraftStep(workflow, "missing")
			},
			want: "was not found",
		},
		{
			name: "move missing step",
			run: func(workflow TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error) {
				return MoveDraftStep(workflow, "missing", StepContainer{}, 0)
			},
			want: "was not found",
		},
		{
			name: "move to invalid destination index",
			run: func(workflow TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error) {
				return MoveDraftStep(workflow, "a", StepContainer{ParentStepID: "repeat"}, 2)
			},
			want: "out of range",
		},
		{
			name: "reorder missing container",
			run: func(workflow TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error) {
				return ReorderDraftSteps(workflow, StepContainer{ParentStepID: "missing"}, nil)
			},
			want: "container",
		},
		{
			name: "delete missing node",
			run: func(workflow TemporarySamplingWorkflow) (TemporarySamplingWorkflow, error) {
				return DeleteDraftNode(workflow, "missing")
			},
			want: "was not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			before := draftFixture()
			got, err := test.run(workflow)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
			if !reflect.DeepEqual(got, TemporarySamplingWorkflow{}) {
				t.Fatalf("rejected command returned partial workflow: %#v", got)
			}
			if !reflect.DeepEqual(workflow, before) {
				t.Fatal("rejected command mutated source workflow")
			}
		})
	}
}

func TestDeleteDraftStepRemovesValidationBranchMember(t *testing.T) {
	workflow := draftFixture()
	got, err := DeleteDraftStep(workflow, "c")
	if err != nil {
		t.Fatal(err)
	}
	if steps := got.Steps[2].ValidationGroup.Branches[0].Steps; len(steps) != 0 {
		t.Fatalf("validation branch steps = %v, want empty", steps)
	}
	if len(got.Nodes[2].StepIDs) != 0 {
		t.Fatalf("node-c StepIDs = %v, want empty", got.Nodes[2].StepIDs)
	}
	if len(workflow.Steps[2].ValidationGroup.Branches[0].Steps) != 1 {
		t.Fatal("delete mutated source workflow")
	}
}

func TestDraftContainerSelectionRejectsImpossibleBusinessShapes(t *testing.T) {
	tests := []struct {
		name      string
		container StepContainer
	}{
		{name: "root cannot select branch", container: StepContainer{BranchID: "branch"}},
		{name: "missing parent", container: StepContainer{ParentStepID: "missing"}},
		{name: "action cannot contain children", container: StepContainer{ParentStepID: "a"}},
		{name: "repeat cannot select branch", container: StepContainer{ParentStepID: "repeat", BranchID: "branch"}},
		{name: "validation group branch must exist", container: StepContainer{ParentStepID: "group", BranchID: "missing"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			before := draftFixture()
			got, err := InsertDraftStep(workflow, test.container, 0, automation.WorkflowStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, NodeID: "node-a"})
			if err == nil || !strings.Contains(err.Error(), "container") {
				t.Fatalf("container error = %v", err)
			}
			if !reflect.DeepEqual(got, TemporarySamplingWorkflow{}) || !reflect.DeepEqual(workflow, before) {
				t.Fatalf("rejected container changed state: got=%#v source=%#v", got, workflow)
			}
		})
	}
}

func TestDraftCommandsRejectMalformedWorkflowIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TemporarySamplingWorkflow)
		want   string
	}{
		{name: "blank workflow id", mutate: func(workflow *TemporarySamplingWorkflow) { workflow.ID = " \t" }, want: "workflow id"},
		{name: "blank node id", mutate: func(workflow *TemporarySamplingWorkflow) { workflow.Nodes[0].ID = " \n" }, want: "node id"},
		{name: "duplicate node id", mutate: func(workflow *TemporarySamplingWorkflow) { workflow.Nodes[1].ID = workflow.Nodes[0].ID }, want: "duplicate temporary sampling node"},
		{name: "blank nested step id", mutate: func(workflow *TemporarySamplingWorkflow) { workflow.Steps[1].Children[0].ID = " " }, want: "step id"},
		{name: "duplicate nested step id", mutate: func(workflow *TemporarySamplingWorkflow) { workflow.Steps[1].Children[0].ID = workflow.Steps[0].ID }, want: "duplicate temporary sampling step"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			test.mutate(&workflow)
			before := draftFixture()
			test.mutate(&before)
			got, err := InsertDraftStep(workflow, StepContainer{}, len(workflow.Steps), automation.WorkflowStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, NodeID: "node-c"})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("identity error = %v, want containing %q", err, test.want)
			}
			if !reflect.DeepEqual(got, TemporarySamplingWorkflow{}) || !reflect.DeepEqual(workflow, before) {
				t.Fatalf("rejected identity changed state: got=%#v source=%#v", got, workflow)
			}
		})
	}
}

func TestRebuildTemporaryNodeReferencesDerivesNestedProjectionInEncounterOrder(t *testing.T) {
	workflow := draftFixture()
	workflow.Steps = append(workflow.Steps,
		automation.WorkflowStep{ID: "a-second", DisplayName: "a second", Kind: automation.StepAction, NodeID: "node-a"},
		automation.WorkflowStep{ID: "wait", DisplayName: "wait", Kind: automation.StepWait},
	)
	for index := range workflow.Nodes {
		workflow.Nodes[index].StepIDs = []string{"stale"}
	}

	if err := RebuildTemporaryNodeReferences(&workflow); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"node-a": {"a", "a-second"},
		"node-b": {"b"},
		"node-c": {"c"},
		"unused": nil,
	}
	for _, node := range workflow.Nodes {
		if !reflect.DeepEqual(node.StepIDs, want[node.ID]) {
			t.Fatalf("node %q StepIDs = %v, want %v", node.ID, node.StepIDs, want[node.ID])
		}
	}
}

func TestRebuildTemporaryNodeReferencesRejectsNilAndUnknownNode(t *testing.T) {
	if err := RebuildTemporaryNodeReferences(nil); err == nil || !strings.Contains(err.Error(), "workflow is required") {
		t.Fatalf("nil workflow error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*TemporarySamplingWorkflow)
		stepID string
	}{
		{name: "repeat child", mutate: func(workflow *TemporarySamplingWorkflow) { workflow.Steps[1].Children[0].NodeID = "missing" }, stepID: "b"},
		{name: "validation branch", mutate: func(workflow *TemporarySamplingWorkflow) {
			workflow.Steps[2].ValidationGroup.Branches[0].Steps[0].NodeID = "missing"
		}, stepID: "c"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			test.mutate(&workflow)
			if err := RebuildTemporaryNodeReferences(&workflow); err == nil ||
				!strings.Contains(err.Error(), "step "+test.stepID) || !strings.Contains(err.Error(), "missing") {
				t.Fatalf("unknown node error = %v", err)
			}
		})
	}
}
