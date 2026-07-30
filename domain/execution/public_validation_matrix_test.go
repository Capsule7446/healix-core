package execution

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestParameterValidateSelectAndNonSelectBoundaryMatrix(t *testing.T) {
	tests := []struct {
		name  string
		value Parameter
		want  string
	}{
		{name: "non select declares options", value: Parameter{Name: "value", DisplayName: "Value", Type: parameter.Text, Required: true, Options: []string{"forbidden"}}, want: "cannot declare options"},
		{name: "select has no options", value: Parameter{Name: "value", DisplayName: "Value", Type: parameter.SingleSelect, Required: true}, want: "requires options"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
	optional := Parameter{Name: "value", DisplayName: "Value", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("default"))}
	if err := optional.Validate(); err != nil {
		t.Fatalf("valid optional default rejected: %v", err)
	}
}

func TestDraftValidatePublicRuleMatrix(t *testing.T) {
	const nodeID = "00000000-0000-0000-0000-000000000001"
	base := func() Draft { return validDraftWithNodes(validNodeSnapshot(nodeID, "v1")) }
	tests := []struct {
		name   string
		build  func() Draft
		mutate func(*Draft)
		want   string
	}{
		{name: "invalid failure policy", build: base, mutate: func(value *Draft) { value.FailurePolicy = "UNKNOWN" }, want: "invalid failure policy"},
		{name: "missing entries", build: base, mutate: func(value *Draft) { value.Entries = nil }, want: "at least one entry"},
		{name: "entry sequence outside range", build: base, mutate: func(value *Draft) { value.Entries[0].SequenceNumber = 0 }, want: "outside contiguous range"},
		{name: "duplicate entry sequence", build: base, mutate: func(value *Draft) {
			second := value.Entries[0]
			second.ExecutionID, second.TestTaskItemID = "execution-two", "item-two"
			value.Entries = append(value.Entries, second)
		}, want: "duplicate execution entry sequence"},
		{name: "missing entry identity", build: base, mutate: func(value *Draft) { value.Entries[0].ExecutionID = " " }, want: "requires execution"},
		{name: "duplicate execution identity", build: base, mutate: func(value *Draft) {
			second := value.Entries[0]
			second.TestTaskItemID, second.SequenceNumber = "item-two", 2
			value.Entries = append(value.Entries, second)
		}, want: "duplicate execution entry"},
		{name: "duplicate task item identity", build: base, mutate: func(value *Draft) {
			second := value.Entries[0]
			second.ExecutionID, second.SequenceNumber = "execution-two", 2
			value.Entries = append(value.Entries, second)
		}, want: "duplicate test task item"},
		{name: "workflow version identity missing", build: base, mutate: func(value *Draft) { value.Workflows[0].VersionID = " " }, want: "empty version id"},
		{name: "duplicate workflow version", build: base, mutate: func(value *Draft) { value.Workflows = append(value.Workflows, value.Workflows[0]) }, want: "duplicate workflow version"},
		{name: "entry workflow missing", build: base, mutate: func(value *Draft) { value.Entries[0].WorkflowVersionID = "missing" }, want: "entry workflow version"},
		{name: "entry workflow owner mismatch", build: base, mutate: func(value *Draft) { value.Entries[0].FlowFragmentID = "other" }, want: "belongs to workflow"},
		{name: "parameterless workflow carries snapshot", build: base, mutate: func(value *Draft) { value.Entries[0].Parameters.ID = "scope" }, want: "requires an empty parameter snapshot"},
		{name: "parameter snapshot identity invalid", build: func() Draft { return validRunSnapshotInput(t).Plan }, mutate: func(value *Draft) { value.Entries[0].Parameters.ID = " " }, want: "parameter snapshot identity"},
		{name: "node identity missing", build: base, mutate: func(value *Draft) { value.Nodes[0].ElementTargetID = " " }, want: "node dependency requires"},
		{name: "node version has different owners", build: base, mutate: func(value *Draft) {
			value.Nodes = append(value.Nodes, validNodeSnapshot("00000000-0000-0000-0000-000000000002", "v1"))
		}, want: "owned by different nodes"},
		{name: "duplicate node dependency", build: base, mutate: func(value *Draft) { value.Nodes = append(value.Nodes, value.Nodes[0]) }, want: "duplicate node dependency"},
		{name: "duplicate workflow resolution", build: base, mutate: func(value *Draft) {
			resolution := ReferenceResolution{ParentVersionID: "workflow-v1", StepID: "unused", FlowFragmentID: "child", WorkflowVersionID: "child-v1"}
			value.References = []ReferenceResolution{resolution, resolution}
		}, want: "duplicate workflow resolution"},
		{name: "unowned workflow resolutions are sorted", build: base, mutate: func(value *Draft) {
			value.References = []ReferenceResolution{
				{ParentVersionID: "workflow-v1", StepID: "z", FlowFragmentID: "child", WorkflowVersionID: "child-v1"},
				{ParentVersionID: "workflow-v1", StepID: "a", FlowFragmentID: "child", WorkflowVersionID: "child-v1"},
			}
		}, want: "does not belong to a reference step"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := test.build()
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestSealCanonicalizesMultipleEntryOrder(t *testing.T) {
	draft := validDraftWithNodes(validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1"))
	first := draft.Entries[0]
	first.ExecutionID, first.TestTaskItemID, first.SequenceNumber = "execution-one", "item-one", 1
	second := first
	second.ExecutionID, second.TestTaskItemID, second.SequenceNumber = "execution-two", "item-two", 2
	draft.Entries = []WorkflowEntry{second, first}

	plan, err := Seal(draft)
	if err != nil {
		t.Fatal(err)
	}
	entries := plan.Entries()
	if entries[0].SequenceNumber != 1 || entries[1].SequenceNumber != 2 {
		t.Fatalf("sealed entry order = %#v", entries)
	}
}

func TestWorkflowSnapshotValidateRejectsMissingSteps(t *testing.T) {
	workflow := validWorkflowSnapshot()
	workflow.Steps = nil
	if err := workflow.Validate(); err == nil || !strings.Contains(err.Error(), "at least one step") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRunConstructionAndTransitionPublicErrorBoundaries(t *testing.T) {
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}
	base := Run{ID: "run-1", ExecutionFlowID: "task-1", TestTaskVersionID: "task-v3", EnvironmentID: "env-1", Status: Queued, QueuePosition: 0, CreatedAt: 10, QueuedAt: 10}
	mismatch := base
	mismatch.ID = "other"
	if _, err := NewRun(mismatch, snapshot); err == nil || !strings.Contains(err.Error(), "identity must match") {
		t.Fatalf("NewRun() error = %v", err)
	}
	queued, err := NewRun(base, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queued.Transition(Succeeded, 10); err == nil {
		t.Fatal("illegal queued to succeeded transition accepted")
	}
}
