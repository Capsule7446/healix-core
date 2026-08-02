package sampling

import (
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestDraftCommandsRejectBoundaryIndexesAndMissingTargetsWithoutMutation(t *testing.T) {
	tests := []struct {
		name     string
		run      func(UnpublishedFlowFragment) (UnpublishedFlowFragment, error)
		wantCode fault.Code
	}{
		{
			name: "insert index below zero",
			run: func(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
				return InsertUnpublishedFlowFragmentStep(workflow, FlowFragmentStepContainer{}, -1, automation.FlowFragmentStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, ElementTargetID: "node-a"})
			},
			wantCode: CodeDraftIndexOutOfRange,
		},
		{
			name: "insert index above length",
			run: func(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
				return InsertUnpublishedFlowFragmentStep(workflow, FlowFragmentStepContainer{}, len(workflow.Steps)+1, automation.FlowFragmentStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, ElementTargetID: "node-a"})
			},
			wantCode: CodeDraftIndexOutOfRange,
		},
		{
			name: "delete missing step",
			run: func(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
				return DeleteUnpublishedFlowFragmentStep(workflow, "missing")
			},
			wantCode: CodeDraftStepNotFound,
		},
		{
			name: "move missing step",
			run: func(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
				return MoveUnpublishedFlowFragmentStep(workflow, "missing", FlowFragmentStepContainer{}, 0)
			},
			wantCode: CodeDraftStepNotFound,
		},
		{
			name: "move to invalid destination index",
			run: func(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
				return MoveUnpublishedFlowFragmentStep(workflow, "a", FlowFragmentStepContainer{ParentStepID: "repeat"}, 2)
			},
			wantCode: CodeDraftIndexOutOfRange,
		},
		{
			name: "reorder missing container",
			run: func(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
				return ReorderUnpublishedFlowFragmentSteps(workflow, FlowFragmentStepContainer{ParentStepID: "missing"}, nil)
			},
			wantCode: CodeDraftStepNotFound,
		},
		{
			name: "delete missing node",
			run: func(workflow UnpublishedFlowFragment) (UnpublishedFlowFragment, error) {
				return DeleteUnpublishedElementTarget(workflow, "missing")
			},
			wantCode: CodeDraftElementTargetNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			before := draftFixture()
			got, err := test.run(workflow)
			requireEnvelope(t, err, test.wantCode)
			requireNoPublicLeak(t, err, "missing", "node-a")
			if !reflect.DeepEqual(got, UnpublishedFlowFragment{}) {
				t.Fatalf("rejected command returned partial workflow: %#v", got)
			}
			if !reflect.DeepEqual(workflow, before) {
				t.Fatal("rejected command mutated source workflow")
			}
		})
	}
}

func TestDeleteUnpublishedFlowFragmentStepRemovesValidationBranchMember(t *testing.T) {
	workflow := draftFixture()
	got, err := DeleteUnpublishedFlowFragmentStep(workflow, "c")
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
		container FlowFragmentStepContainer
		wantCode  fault.Code
		wantField string
	}{
		{name: "root cannot select branch", container: FlowFragmentStepContainer{BranchID: "branch"}, wantCode: CodeDraftInvalid, wantField: "container.branchId"},
		{name: "missing parent", container: FlowFragmentStepContainer{ParentStepID: "missing"}, wantCode: CodeDraftStepNotFound},
		{name: "action cannot contain children", container: FlowFragmentStepContainer{ParentStepID: "a"}, wantCode: CodeDraftInvalid, wantField: "container"},
		{name: "repeat cannot select branch", container: FlowFragmentStepContainer{ParentStepID: "repeat", BranchID: "branch"}, wantCode: CodeDraftInvalid, wantField: "container"},
		{name: "validation group branch must exist", container: FlowFragmentStepContainer{ParentStepID: "group", BranchID: "missing"}, wantCode: CodeDraftInvalid, wantField: "container"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			before := draftFixture()
			got, err := InsertUnpublishedFlowFragmentStep(workflow, test.container, 0, automation.FlowFragmentStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, ElementTargetID: "node-a"})
			if test.wantField == "" {
				requireEnvelope(t, err, test.wantCode)
			} else {
				requireViolation(t, err, test.wantCode, fault.CodeFieldMismatch, test.wantField)
			}
			// "branch" is not usable as a sentinel here: the field path container.branchId
			// legitimately contains it, and a field name is not caller input.
			requireNoPublicLeak(t, err, "missing")
			if !reflect.DeepEqual(got, UnpublishedFlowFragment{}) || !reflect.DeepEqual(workflow, before) {
				t.Fatalf("rejected container changed state: got=%#v source=%#v", got, workflow)
			}
		})
	}
}

func TestDraftCommandsRejectMalformedWorkflowIdentity(t *testing.T) {
	tests := []struct {
		name          string
		mutate        func(*UnpublishedFlowFragment)
		wantViolation fault.Code
		wantField     string
	}{
		{name: "blank workflow id", mutate: func(workflow *UnpublishedFlowFragment) { workflow.ID = " \t" }, wantViolation: fault.CodeFieldRequired, wantField: "id"},
		{name: "blank node id", mutate: func(workflow *UnpublishedFlowFragment) { workflow.Nodes[0].ID = " \n" }, wantViolation: fault.CodeFieldRequired, wantField: "elementTargets.0.id"},
		{name: "duplicate node id", mutate: func(workflow *UnpublishedFlowFragment) { workflow.Nodes[1].ID = workflow.Nodes[0].ID }, wantViolation: fault.CodeFieldDuplicate, wantField: "elementTargets.1.id"},
		{name: "blank nested step id", mutate: func(workflow *UnpublishedFlowFragment) { workflow.Steps[1].Children[0].ID = " " }, wantViolation: fault.CodeFieldRequired, wantField: "steps.id"},
		{name: "duplicate nested step id", mutate: func(workflow *UnpublishedFlowFragment) { workflow.Steps[1].Children[0].ID = workflow.Steps[0].ID }, wantViolation: fault.CodeFieldDuplicate, wantField: "steps.id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			test.mutate(&workflow)
			before := draftFixture()
			test.mutate(&before)
			got, err := InsertUnpublishedFlowFragmentStep(workflow, FlowFragmentStepContainer{}, len(workflow.Steps), automation.FlowFragmentStep{ID: "new", DisplayName: "new", Kind: automation.StepAction, ElementTargetID: "node-c"})
			requireViolation(t, err, CodeDraftInvalid, test.wantViolation, test.wantField)
			requireNoPublicLeak(t, err, "node-a", "node-b", "node-c")
			if !reflect.DeepEqual(got, UnpublishedFlowFragment{}) || !reflect.DeepEqual(workflow, before) {
				t.Fatalf("rejected identity changed state: got=%#v source=%#v", got, workflow)
			}
		})
	}
}

func TestRebuildUnpublishedElementTargetReferencesDerivesNestedProjectionInEncounterOrder(t *testing.T) {
	workflow := draftFixture()
	workflow.Steps = append(workflow.Steps,
		automation.FlowFragmentStep{ID: "a-second", DisplayName: "a second", Kind: automation.StepAction, ElementTargetID: "node-a"},
		automation.FlowFragmentStep{ID: "wait", DisplayName: "wait", Kind: automation.StepWait},
	)
	for index := range workflow.Nodes {
		workflow.Nodes[index].StepIDs = []string{"stale"}
	}

	if err := RebuildUnpublishedElementTargetReferences(&workflow); err != nil {
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

func TestRebuildUnpublishedElementTargetReferencesRejectsNilAndUnknownNode(t *testing.T) {
	// A nil draft is a caller code defect, not a business input failure.
	requireEnvelope(t, RebuildUnpublishedElementTargetReferences(nil), CodeInternal)

	tests := []struct {
		name   string
		mutate func(*UnpublishedFlowFragment)
		stepID string
	}{
		{name: "repeat child", mutate: func(workflow *UnpublishedFlowFragment) { workflow.Steps[1].Children[0].ElementTargetID = "missing" }, stepID: "b"},
		{name: "validation branch", mutate: func(workflow *UnpublishedFlowFragment) {
			workflow.Steps[2].ValidationGroup.Branches[0].Steps[0].ElementTargetID = "missing"
		}, stepID: "c"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := draftFixture()
			test.mutate(&workflow)
			err := RebuildUnpublishedElementTargetReferences(&workflow)
			requireViolation(t, err, CodeWorkspaceInvalid, fault.CodeFieldMismatch, "steps.elementTargetId")
			// Both the step id and the temporary element target id were echoed before.
			// Only the latter is asserted: this fixture's step ids are single characters,
			// which are contained in almost any text and prove nothing as sentinels.
			requireNoPublicLeak(t, err, "missing")
		})
	}
}
