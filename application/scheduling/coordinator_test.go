package scheduling

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

type fakeClaimSource struct {
	claim      Claim
	found      bool
	err        error
	releaseErr error
	released   *int
}

func (f fakeClaimSource) ClaimNext(context.Context, string, int64) (Claim, bool, error) {
	return f.claim, f.found, f.err
}

func (f fakeClaimSource) Release(context.Context, Claim) error {
	if f.released != nil {
		*f.released++
	}
	return f.releaseErr
}

type fakeStateReader struct {
	states []EntryState
	err    error
}

func (f fakeStateReader) LoadEntryStates(context.Context, Claim) ([]EntryState, error) {
	return f.states, f.err
}

type recordingDecisionWriter struct {
	decisions []Decision
	result    *ApplyDecisionResult
	err       error
}

func (f *recordingDecisionWriter) ApplyDecision(_ context.Context, claim Claim, decision Decision, _ int64) (ApplyDecisionResult, error) {
	f.decisions = append(f.decisions, decision)
	if f.result != nil {
		return *f.result, f.err
	}
	return ApplyDecisionResult{Fence: claim.Fence, Applied: f.err == nil}, f.err
}

func TestCoordinatorFailsClosedOnStaleDecisionResult(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	claim := Claim{Snapshot: plan, Fence: execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "winner"}}
	writer := &recordingDecisionWriter{result: &ApplyDecisionResult{Fence: execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "stale"}, Applied: true}}
	coordinator := NewCoordinator(fakeClaimSource{claim: claim, found: true}, fakeStateReader{states: []EntryState{{EntryID: mustEntryID("execution-1"), Status: execution.EntryPending}}}, writer)
	_, err := coordinator.ProcessNext(context.Background(), "worker", 10)
	if !fault.IsCode(err, execution.CodeWorkerFenceStale) || strings.Contains(err.Error(), claim.Fence.InstanceID.String()) || strings.Contains(err.Error(), claim.Fence.ClaimToken) {
		t.Fatalf("stale result error=%v", err)
	}
}

func TestCoordinatorAppliesDecisionUnderClaim(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	writer := &recordingDecisionWriter{}
	released := 0
	coordinator := NewCoordinator(
		fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim-token"}}, found: true, released: &released},
		fakeStateReader{states: []EntryState{{EntryID: mustEntryID("execution-1"), Status: execution.EntryPending}}},
		writer,
	)

	claimed, err := coordinator.ProcessNext(context.Background(), "worker", 10)
	if err != nil || !claimed {
		t.Fatalf("claimed/error = %v/%v", claimed, err)
	}
	if len(writer.decisions) != 1 || writer.decisions[0].NextEntryID != mustEntryID("execution-1") || released != 1 {
		t.Fatalf("decisions/released = %#v/%d", writer.decisions, released)
	}
}

func TestCoordinatorDoesNotWriteWithoutClaimOrAdvance(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	tests := []struct {
		name   string
		claims fakeClaimSource
		states fakeStateReader
		want   bool
	}{
		{name: "empty queue", claims: fakeClaimSource{}},
		{name: "running entry", claims: fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "token"}}, found: true}, states: fakeStateReader{states: []EntryState{{EntryID: mustEntryID("execution-1"), Status: execution.EntryRunning}}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &recordingDecisionWriter{}
			coordinator := NewCoordinator(test.claims, test.states, writer)
			claimed, err := coordinator.ProcessNext(context.Background(), "worker", 10)
			if err != nil || claimed != test.want || len(writer.decisions) != 0 {
				t.Fatalf("claimed/error/writes = %v/%v/%d", claimed, err, len(writer.decisions))
			}
		})
	}
}

func TestSchedulingClaimInvalidErrorExposesSafeStableContract(t *testing.T) {
	err := schedulingClaimInvalidError()
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Code() != CodeSchedulingClaimInvalid || descriptor.Kind() != fault.FailedPrecondition || descriptor.Message() != "scheduling claim is invalid" || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
		t.Fatalf("descriptor = %#v, ok = %v", descriptor, ok)
	}
	for _, sensitive := range []string{"run-sensitive-id", "claim-sensitive-token", "sha256:secret"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestCoordinatorRejectsInvalidClaimAndPropagatesPortErrors(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	failure := errors.New("port failure")
	tests := []struct {
		name     string
		claims   fakeClaimSource
		states   fakeStateReader
		writer   *recordingDecisionWriter
		wantCode fault.Code
		want     error
	}{
		{name: "missing token", claims: fakeClaimSource{claim: Claim{Snapshot: plan}, found: true}, writer: &recordingDecisionWriter{}, wantCode: CodeSchedulingClaimInvalid},
		{name: "claim failure", claims: fakeClaimSource{err: failure}, writer: &recordingDecisionWriter{}, wantCode: CodeSchedulingAdapterUnavailable, want: failure},
		{name: "state failure", claims: fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "token"}}, found: true}, states: fakeStateReader{err: failure}, writer: &recordingDecisionWriter{}, wantCode: CodeSchedulingAdapterUnavailable, want: failure},
		{name: "write failure", claims: fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "token"}}, found: true}, states: fakeStateReader{states: []EntryState{{EntryID: mustEntryID("execution-1"), Status: execution.EntryPending}}}, writer: &recordingDecisionWriter{err: failure}, wantCode: CodeSchedulingAdapterUnavailable, want: failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator := NewCoordinator(test.claims, test.states, test.writer)
			_, err := coordinator.ProcessNext(context.Background(), "worker", 10)
			if test.wantCode != "" && !fault.IsCode(err, test.wantCode) {
				t.Fatalf("error = %v, want code %v", err, test.wantCode)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if test.wantCode == CodeSchedulingAdapterUnavailable {
				descriptor, ok := fault.Describe(err)
				if !ok || strings.Contains(descriptor.Message(), "port failure") {
					t.Fatalf("public message = %#v (ok=%v), must not carry the port detail", descriptor, ok)
				}
			}
		})
	}
}

func TestCoordinatorReleasesInvalidClaimAndJoinsReleaseFailure(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	releaseFailure := errors.New("release failed")
	released := 0
	coordinator := NewCoordinator(
		fakeClaimSource{
			claim:      Claim{Snapshot: plan},
			found:      true,
			releaseErr: releaseFailure,
			released:   &released,
		},
		fakeStateReader{},
		&recordingDecisionWriter{},
	)

	claimed, err := coordinator.ProcessNext(context.Background(), "worker", 10)
	if !claimed || released != 1 || !fault.IsCode(err, CodeSchedulingClaimInvalid) || !errors.Is(err, releaseFailure) {
		t.Fatalf("claimed/released/error = %v/%d/%v", claimed, released, err)
	}
}

func TestCoordinatorReportsDecisionAndReleaseFailures(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	claim := Claim{Snapshot: plan, Fence: execution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "token"}}
	decisionFailureState := []EntryState{{EntryID: mustEntryID("foreign"), Status: execution.EntryPending}}
	releaseFailure := errors.New("release failed")

	t.Run("decision failure still releases", func(t *testing.T) {
		released := 0
		coordinator := NewCoordinator(
			fakeClaimSource{claim: claim, found: true, released: &released},
			fakeStateReader{states: decisionFailureState},
			&recordingDecisionWriter{},
		)
		claimed, err := coordinator.ProcessNext(context.Background(), "worker", 10)
		if !claimed || err == nil || released != 1 {
			t.Fatalf("claimed/error/released = %v/%v/%d", claimed, err, released)
		}
		// The decision failure now arrives as its own classified fault instead of
		// behind an uncoded "decide run advance" wrapper.
		if !fault.IsCode(err, CodeEntryStatesInvalid) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("release failure is returned after success", func(t *testing.T) {
		released := 0
		coordinator := NewCoordinator(
			fakeClaimSource{claim: claim, found: true, releaseErr: releaseFailure, released: &released},
			fakeStateReader{states: []EntryState{{EntryID: mustEntryID("execution-1"), Status: execution.EntryRunning}}},
			&recordingDecisionWriter{},
		)
		claimed, err := coordinator.ProcessNext(context.Background(), "worker", 10)
		if !claimed || !errors.Is(err, releaseFailure) || released != 1 {
			t.Fatalf("claimed/error/released = %v/%v/%d", claimed, err, released)
		}
	})

	t.Run("decision and release failures are joined", func(t *testing.T) {
		coordinator := NewCoordinator(
			fakeClaimSource{claim: claim, found: true, releaseErr: releaseFailure},
			fakeStateReader{states: decisionFailureState},
			&recordingDecisionWriter{},
		)
		_, err := coordinator.ProcessNext(context.Background(), "worker", 10)
		// Joining must preserve both: the release adapter's own error and the
		// decision's classification.
		if !errors.Is(err, releaseFailure) || !fault.IsCode(err, CodeEntryStatesInvalid) {
			t.Fatalf("joined error = %v", err)
		}
	})
}

func sealedCoordinatorPlan(t *testing.T) execution.InstanceSnapshot {
	t.Helper()
	workflow := execution.WorkflowSnapshot{ID: "workflow", FlowFragmentID: "workflow", VersionID: "workflow-v1", DisplayName: "FlowFragment", VersionNumber: 1, Steps: []execution.Step{{ID: "noop", DisplayName: "Noop", Kind: execution.ActionStep, Action: "press", Value: "Enter"}}}
	draft := execution.PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.Entry{{ID: mustEntryID("execution-1"), TestTaskItemID: "item-1", SequenceNumber: 1, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1"}}, Workflows: []execution.WorkflowSnapshot{workflow}}
	snapshot, err := execution.SealInstanceSnapshot(execution.InstanceSnapshotInput{SchemaVersion: execution.InstanceSnapshotSchemaV1, InstanceID: mustInstanceID("run"), ExecutionFlowID: "task", TestTaskVersionID: "task-v1", TestTaskVersionNumber: 1, ExecutionFlow: execution.TestTaskSnapshot{ID: "task"}, ExecutionFlowVersion: execution.ExecutionFlowVersionSnapshot{ID: "task-v1", ExecutionFlowID: "task", VersionNumber: 1, Items: []execution.ExecutionFlowVersionItemSnapshot{{ID: "item-1", TestTaskVersionID: "task-v1", SequenceNumber: 1, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1"}}}, Plan: draft, Invocations: []execution.InvocationScopeSnapshot{{Path: mustInvocationPath("execution-1"), FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", Values: map[string]parameter.Value{}}}, Environment: execution.EnvironmentSnapshot{ID: "env", Revision: 1, DisplayName: "Environment", BaseURL: "https://example.test", Properties: map[string]string{}}, FailurePolicy: execution.FailurePolicyStopOnFailure, ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot()})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
