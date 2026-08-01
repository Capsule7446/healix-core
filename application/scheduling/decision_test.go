package scheduling

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestDecideAdvanceSerialABC(t *testing.T) {
	tests := []struct {
		name        string
		policy      execution.FailurePolicy
		states      []EntryState
		next        string
		transitions []string
		cause       SkipCause
		final       execution.InstanceStatus
	}{
		{"initial selects A", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionPending, execution.ExecutionPending, execution.ExecutionPending), "a", nil, "", ""},
		{"running frontier waits", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionRunning, execution.ExecutionPending, execution.ExecutionPending), "", nil, "", ""},
		{"A success selects B", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionSucceeded, execution.ExecutionPending, execution.ExecutionPending), "b", nil, "", ""},
		{"ABC success finalizes", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionSucceeded, execution.ExecutionSucceeded, execution.ExecutionSucceeded), "", nil, "", execution.Succeeded},
		{"A failure skips BC", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionFailed, execution.ExecutionPending, execution.ExecutionPending), "", []string{"b", "c"}, SkipCausePriorFailure, execution.Failed},
		{"B failure skips C", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionSucceeded, execution.ExecutionFailed, execution.ExecutionPending), "", []string{"c"}, SkipCausePriorFailure, execution.Failed},
		{"A cancellation skips BC", execution.FailurePolicyContinueOnFailure, entryStates(execution.ExecutionCanceled, execution.ExecutionPending, execution.ExecutionPending), "", []string{"b", "c"}, SkipCausePriorCancellation, execution.Canceled},
		{"B cancellation skips C", execution.FailurePolicyContinueOnFailure, entryStates(execution.ExecutionSucceeded, execution.ExecutionCanceled, execution.ExecutionPending), "", []string{"c"}, SkipCausePriorCancellation, execution.Canceled},
		{"A abort skips BC", execution.FailurePolicyContinueOnFailure, entryStates(execution.ExecutionAborted, execution.ExecutionPending, execution.ExecutionPending), "", []string{"b", "c"}, SkipCausePriorAbort, execution.Aborted},
		{"B abort skips C", execution.FailurePolicyContinueOnFailure, entryStates(execution.ExecutionSucceeded, execution.ExecutionAborted, execution.ExecutionPending), "", []string{"c"}, SkipCausePriorAbort, execution.Aborted},
		{"failure continues", execution.FailurePolicyContinueOnFailure, entryStates(execution.ExecutionSucceeded, execution.ExecutionFailed, execution.ExecutionPending), "c", nil, "", ""},
		{"continued failure aggregates", execution.FailurePolicyContinueOnFailure, entryStates(execution.ExecutionSucceeded, execution.ExecutionFailed, execution.ExecutionSucceeded), "", nil, "", execution.Failed},
		{"persisted failure skips", execution.FailurePolicyStopOnFailure, causedStates(execution.ExecutionFailed, SkipCausePriorFailure), "", nil, SkipCausePriorFailure, execution.Failed},
		{"persisted cancellation skips", execution.FailurePolicyContinueOnFailure, causedStates(execution.ExecutionCanceled, SkipCausePriorCancellation), "", nil, SkipCausePriorCancellation, execution.Canceled},
		{"persisted abort skips", execution.FailurePolicyContinueOnFailure, causedStates(execution.ExecutionAborted, SkipCausePriorAbort), "", nil, SkipCausePriorAbort, execution.Aborted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := DecideAdvance(sealedPlan(t, test.policy), test.states)
			if err != nil {
				t.Fatal(err)
			}
			if decision.NextExecutionID.String() != test.next || len(decision.Transitions) != len(test.transitions) {
				t.Fatalf("decision = %#v", decision)
			}
			for i, id := range test.transitions {
				got := decision.Transitions[i]
				if got.ExecutionID.String() != id || got.From != execution.ExecutionPending || got.To != execution.ExecutionSkipped || got.Cause != test.cause {
					t.Fatalf("transition = %#v", got)
				}
			}
			if (decision.FinalStatus == nil) != (test.final == "") || decision.FinalStatus != nil && *decision.FinalStatus != test.final {
				t.Fatalf("final = %#v", decision.FinalStatus)
			}
		})
	}
}

func TestDecideAdvanceRejectsMalformedStateVectors(t *testing.T) {
	tests := []struct {
		name   string
		policy execution.FailurePolicy
		states []EntryState
	}{
		{"missing", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionPending, execution.ExecutionPending)},
		{"duplicate identity", execution.FailurePolicyStopOnFailure, []EntryState{{ExecutionID: mustEntryID("a"), Status: execution.ExecutionPending}, {ExecutionID: mustEntryID("a"), Status: execution.ExecutionPending}, {ExecutionID: mustEntryID("c"), Status: execution.ExecutionPending}}},
		{"unknown identity", execution.FailurePolicyStopOnFailure, []EntryState{{ExecutionID: mustEntryID("a"), Status: execution.ExecutionPending}, {ExecutionID: mustEntryID("b"), Status: execution.ExecutionPending}, {ExecutionID: mustEntryID("x"), Status: execution.ExecutionPending}}},
		{"unknown status", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionSucceeded, "UNKNOWN", execution.ExecutionPending)},
		{"pending before running", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionPending, execution.ExecutionRunning, execution.ExecutionPending)},
		{"terminal after pending", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionPending, execution.ExecutionSucceeded, execution.ExecutionPending)},
		{"terminal after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionFailed, execution.ExecutionSucceeded, execution.ExecutionPending)},
		{"failed after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionFailed, execution.ExecutionFailed, execution.ExecutionPending)},
		{"canceled after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionFailed, execution.ExecutionCanceled, execution.ExecutionPending)},
		{"aborted after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionFailed, execution.ExecutionAborted, execution.ExecutionPending)},
		{"skipped before cause", execution.FailurePolicyStopOnFailure, []EntryState{{ExecutionID: mustEntryID("a"), Status: execution.ExecutionSkipped, SkipCause: SkipCausePriorFailure}, {ExecutionID: mustEntryID("b"), Status: execution.ExecutionPending}, {ExecutionID: mustEntryID("c"), Status: execution.ExecutionPending}}},
		{"all skipped", execution.FailurePolicyStopOnFailure, []EntryState{{mustEntryID("a"), execution.ExecutionSkipped, SkipCausePriorFailure}, {mustEntryID("b"), execution.ExecutionSkipped, SkipCausePriorFailure}, {mustEntryID("c"), execution.ExecutionSkipped, SkipCausePriorFailure}}},
		{"cause mismatch", execution.FailurePolicyStopOnFailure, []EntryState{{ExecutionID: mustEntryID("a"), Status: execution.ExecutionFailed}, {mustEntryID("b"), execution.ExecutionSkipped, SkipCausePriorAbort}, {mustEntryID("c"), execution.ExecutionSkipped, SkipCausePriorFailure}}},
		{"missing skip cause", execution.FailurePolicyStopOnFailure, entryStates(execution.ExecutionFailed, execution.ExecutionSkipped, execution.ExecutionSkipped)},
		{"cause on non-skipped", execution.FailurePolicyStopOnFailure, []EntryState{{mustEntryID("a"), execution.ExecutionFailed, SkipCausePriorFailure}, {ExecutionID: mustEntryID("b"), Status: execution.ExecutionPending}, {ExecutionID: mustEntryID("c"), Status: execution.ExecutionPending}}},
		{"continue arbitrary skipped", execution.FailurePolicyContinueOnFailure, []EntryState{{ExecutionID: mustEntryID("a"), Status: execution.ExecutionSucceeded}, {mustEntryID("b"), execution.ExecutionSkipped, SkipCausePriorFailure}, {mustEntryID("c"), execution.ExecutionSkipped, SkipCausePriorFailure}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecideAdvance(sealedPlan(t, test.policy), test.states)
			if !fault.IsCode(err, CodeEntryStatesInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func entryStates(values ...execution.ExecutionStatus) []EntryState {
	ids := []string{"a", "b", "c"}
	result := make([]EntryState, len(values))
	for i, status := range values {
		result[i] = EntryState{ExecutionID: mustEntryID(ids[i]), Status: status}
	}
	return result
}
func causedStates(first execution.ExecutionStatus, cause SkipCause) []EntryState {
	return []EntryState{{ExecutionID: mustEntryID("a"), Status: first}, {ExecutionID: mustEntryID("b"), Status: execution.ExecutionSkipped, SkipCause: cause}, {ExecutionID: mustEntryID("c"), Status: execution.ExecutionSkipped, SkipCause: cause}}
}
func sealedPlan(t *testing.T, policy execution.FailurePolicy) execution.InstanceSnapshot {
	t.Helper()
	draft := planDraft(policy)
	items := make([]execution.ExecutionFlowVersionItemSnapshot, len(draft.Entries))
	invocations := make([]execution.InvocationScopeSnapshot, len(draft.Entries))
	for index, entry := range draft.Entries {
		items[index] = execution.ExecutionFlowVersionItemSnapshot{ID: entry.TestTaskItemID, TestTaskVersionID: "task-v1", SequenceNumber: entry.SequenceNumber, FlowFragmentID: entry.FlowFragmentID, WorkflowVersionID: entry.WorkflowVersionID}
		invocations[index] = execution.InvocationScopeSnapshot{Path: execution.RootInvocationPath(entry.ID), FlowFragmentID: entry.FlowFragmentID, WorkflowVersionID: entry.WorkflowVersionID, Values: map[string]parameter.Value{}}
	}
	snapshot, err := execution.SealInstanceSnapshot(execution.InstanceSnapshotInput{
		SchemaVersion: execution.RunSnapshotSchemaV1, InstanceID: mustInstanceID("run"), ExecutionFlowID: "task", TestTaskVersionID: "task-v1", TestTaskVersionNumber: 1,
		ExecutionFlow: execution.TestTaskSnapshot{ID: "task"}, ExecutionFlowVersion: execution.ExecutionFlowVersionSnapshot{ID: "task-v1", ExecutionFlowID: "task", VersionNumber: 1, Items: items},
		Plan: draft, Invocations: invocations, Environment: execution.EnvironmentSnapshot{ID: "env", Revision: 1, DisplayName: "Environment", BaseURL: "https://example.test", Properties: map[string]string{}}, FailurePolicy: policy,
		ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func planDraft(policy execution.FailurePolicy) execution.PlanSnapshot {
	workflow := execution.WorkflowSnapshot{ID: "workflow", VersionID: "workflow-v1", FlowFragmentID: "workflow", DisplayName: "FlowFragment", VersionNumber: 1, Steps: []execution.Step{{ID: "wait", DisplayName: "Wait", Kind: execution.WaitStep, WaitKind: "sleep", WaitMS: 1}}}
	return execution.PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: policy, Entries: []execution.Entry{{ID: mustEntryID("a"), TestTaskItemID: "item-a", SequenceNumber: 1, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1"}, {ID: mustEntryID("b"), TestTaskItemID: "item-b", SequenceNumber: 2, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1"}, {ID: mustEntryID("c"), TestTaskItemID: "item-c", SequenceNumber: 3, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1"}}, Workflows: []execution.WorkflowSnapshot{workflow}}
}
