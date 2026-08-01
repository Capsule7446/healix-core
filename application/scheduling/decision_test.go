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
		{"initial selects A", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryPending, execution.EntryPending, execution.EntryPending), "a", []string{"a"}, "", ""},
		{"running frontier waits", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryRunning, execution.EntryPending, execution.EntryPending), "", nil, "", ""},
		{"A success selects B", execution.FailurePolicyStopOnFailure, entryStates(execution.EntrySucceeded, execution.EntryPending, execution.EntryPending), "b", []string{"b"}, "", ""},
		{"ABC success finalizes", execution.FailurePolicyStopOnFailure, entryStates(execution.EntrySucceeded, execution.EntrySucceeded, execution.EntrySucceeded), "", nil, "", execution.Succeeded},
		{"A failure skips BC", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryFailed, execution.EntryPending, execution.EntryPending), "", []string{"b", "c"}, SkipCausePriorFailure, execution.Failed},
		{"B failure skips C", execution.FailurePolicyStopOnFailure, entryStates(execution.EntrySucceeded, execution.EntryFailed, execution.EntryPending), "", []string{"c"}, SkipCausePriorFailure, execution.Failed},
		{"A cancellation skips BC", execution.FailurePolicyContinueOnFailure, entryStates(execution.EntryCanceled, execution.EntryPending, execution.EntryPending), "", []string{"b", "c"}, SkipCausePriorCancellation, execution.Canceled},
		{"B cancellation skips C", execution.FailurePolicyContinueOnFailure, entryStates(execution.EntrySucceeded, execution.EntryCanceled, execution.EntryPending), "", []string{"c"}, SkipCausePriorCancellation, execution.Canceled},
		{"A abort skips BC", execution.FailurePolicyContinueOnFailure, entryStates(execution.EntryAborted, execution.EntryPending, execution.EntryPending), "", []string{"b", "c"}, SkipCausePriorAbort, execution.Aborted},
		{"B abort skips C", execution.FailurePolicyContinueOnFailure, entryStates(execution.EntrySucceeded, execution.EntryAborted, execution.EntryPending), "", []string{"c"}, SkipCausePriorAbort, execution.Aborted},
		{"failure continues", execution.FailurePolicyContinueOnFailure, entryStates(execution.EntrySucceeded, execution.EntryFailed, execution.EntryPending), "c", []string{"c"}, "", ""},
		{"continued failure aggregates", execution.FailurePolicyContinueOnFailure, entryStates(execution.EntrySucceeded, execution.EntryFailed, execution.EntrySucceeded), "", nil, "", execution.Failed},
		{"persisted failure skips", execution.FailurePolicyStopOnFailure, causedStates(execution.EntryFailed, SkipCausePriorFailure), "", nil, SkipCausePriorFailure, execution.Failed},
		{"persisted cancellation skips", execution.FailurePolicyContinueOnFailure, causedStates(execution.EntryCanceled, SkipCausePriorCancellation), "", nil, SkipCausePriorCancellation, execution.Canceled},
		{"persisted abort skips", execution.FailurePolicyContinueOnFailure, causedStates(execution.EntryAborted, SkipCausePriorAbort), "", nil, SkipCausePriorAbort, execution.Aborted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := DecideAdvance(sealedPlan(t, test.policy), test.states)
			if err != nil {
				t.Fatal(err)
			}
			if decision.NextEntryID.String() != test.next || len(decision.Transitions) != len(test.transitions) {
				t.Fatalf("decision = %#v", decision)
			}
			for i, id := range test.transitions {
				got := decision.Transitions[i]
				if got.EntryID.String() != id || got.From != execution.EntryPending {
					t.Fatalf("transition %d = %#v", i, got)
				}
				switch got.To {
				case execution.EntrySkipped:
					if got.Cause != test.cause {
						t.Fatalf("transition %d cause = %s", i, got.Cause)
					}
				case execution.EntryRunning:
				default:
					t.Fatalf("transition %d unexpected To = %s", i, got.To)
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
		{"missing", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryPending, execution.EntryPending)},
		{"duplicate identity", execution.FailurePolicyStopOnFailure, []EntryState{{EntryID: mustEntryID("a"), Status: execution.EntryPending}, {EntryID: mustEntryID("a"), Status: execution.EntryPending}, {EntryID: mustEntryID("c"), Status: execution.EntryPending}}},
		{"unknown identity", execution.FailurePolicyStopOnFailure, []EntryState{{EntryID: mustEntryID("a"), Status: execution.EntryPending}, {EntryID: mustEntryID("b"), Status: execution.EntryPending}, {EntryID: mustEntryID("x"), Status: execution.EntryPending}}},
		{"unknown status", execution.FailurePolicyStopOnFailure, entryStates(execution.EntrySucceeded, "UNKNOWN", execution.EntryPending)},
		{"pending before running", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryPending, execution.EntryRunning, execution.EntryPending)},
		{"terminal after pending", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryPending, execution.EntrySucceeded, execution.EntryPending)},
		{"terminal after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryFailed, execution.EntrySucceeded, execution.EntryPending)},
		{"failed after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryFailed, execution.EntryFailed, execution.EntryPending)},
		{"canceled after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryFailed, execution.EntryCanceled, execution.EntryPending)},
		{"aborted after stop", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryFailed, execution.EntryAborted, execution.EntryPending)},
		{"skipped before cause", execution.FailurePolicyStopOnFailure, []EntryState{{EntryID: mustEntryID("a"), Status: execution.EntrySkipped, SkipCause: SkipCausePriorFailure}, {EntryID: mustEntryID("b"), Status: execution.EntryPending}, {EntryID: mustEntryID("c"), Status: execution.EntryPending}}},
		{"all skipped", execution.FailurePolicyStopOnFailure, []EntryState{{mustEntryID("a"), execution.EntrySkipped, SkipCausePriorFailure}, {mustEntryID("b"), execution.EntrySkipped, SkipCausePriorFailure}, {mustEntryID("c"), execution.EntrySkipped, SkipCausePriorFailure}}},
		{"cause mismatch", execution.FailurePolicyStopOnFailure, []EntryState{{EntryID: mustEntryID("a"), Status: execution.EntryFailed}, {mustEntryID("b"), execution.EntrySkipped, SkipCausePriorAbort}, {mustEntryID("c"), execution.EntrySkipped, SkipCausePriorFailure}}},
		{"missing skip cause", execution.FailurePolicyStopOnFailure, entryStates(execution.EntryFailed, execution.EntrySkipped, execution.EntrySkipped)},
		{"cause on non-skipped", execution.FailurePolicyStopOnFailure, []EntryState{{mustEntryID("a"), execution.EntryFailed, SkipCausePriorFailure}, {EntryID: mustEntryID("b"), Status: execution.EntryPending}, {EntryID: mustEntryID("c"), Status: execution.EntryPending}}},
		{"continue arbitrary skipped", execution.FailurePolicyContinueOnFailure, []EntryState{{EntryID: mustEntryID("a"), Status: execution.EntrySucceeded}, {mustEntryID("b"), execution.EntrySkipped, SkipCausePriorFailure}, {mustEntryID("c"), execution.EntrySkipped, SkipCausePriorFailure}}},
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

func entryStates(values ...execution.EntryStatus) []EntryState {
	ids := []string{"a", "b", "c"}
	result := make([]EntryState, len(values))
	for i, status := range values {
		result[i] = EntryState{EntryID: mustEntryID(ids[i]), Status: status}
	}
	return result
}
func causedStates(first execution.EntryStatus, cause SkipCause) []EntryState {
	return []EntryState{{EntryID: mustEntryID("a"), Status: first}, {EntryID: mustEntryID("b"), Status: execution.EntrySkipped, SkipCause: cause}, {EntryID: mustEntryID("c"), Status: execution.EntrySkipped, SkipCause: cause}}
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
		SchemaVersion: execution.InstanceSnapshotSchemaV1, InstanceID: mustInstanceID("run"), ExecutionFlowID: "task", TestTaskVersionID: "task-v1", TestTaskVersionNumber: 1,
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
