package scheduling

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
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
	claim := Claim{Snapshot: plan, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "winner"}}
	writer := &recordingDecisionWriter{result: &ApplyDecisionResult{Fence: execution.WorkerFence{RunID: "run", ClaimToken: "stale"}, Applied: true}}
	coordinator := NewCoordinator(fakeClaimSource{claim: claim, found: true}, fakeStateReader{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionPending}}}, writer)
	_, err := coordinator.ProcessNext(context.Background(), "worker", 10)
	var typed *execution.StaleWorkerFenceError
	if !errors.Is(err, execution.ErrStaleWorkerFence) || !errors.As(err, &typed) || typed.Fence != claim.Fence {
		t.Fatalf("stale result error=%v", err)
	}
}

func TestCoordinatorAppliesDecisionUnderClaim(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	writer := &recordingDecisionWriter{}
	released := 0
	coordinator := NewCoordinator(
		fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "claim-token"}}, found: true, released: &released},
		fakeStateReader{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionPending}}},
		writer,
	)

	claimed, err := coordinator.ProcessNext(context.Background(), "worker", 10)
	if err != nil || !claimed {
		t.Fatalf("claimed/error = %v/%v", claimed, err)
	}
	if len(writer.decisions) != 1 || writer.decisions[0].NextExecutionID != "execution-1" || released != 1 {
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
		{name: "running entry", claims: fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "token"}}, found: true}, states: fakeStateReader{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionRunning}}}, want: true},
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

func TestCoordinatorRejectsInvalidClaimAndPropagatesPortErrors(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	failure := errors.New("port failure")
	tests := []struct {
		name   string
		claims fakeClaimSource
		states fakeStateReader
		writer *recordingDecisionWriter
		want   error
	}{
		{name: "missing token", claims: fakeClaimSource{claim: Claim{Snapshot: plan}, found: true}, writer: &recordingDecisionWriter{}, want: ErrInvalidClaim},
		{name: "claim failure", claims: fakeClaimSource{err: failure}, writer: &recordingDecisionWriter{}, want: failure},
		{name: "state failure", claims: fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "token"}}, found: true}, states: fakeStateReader{err: failure}, writer: &recordingDecisionWriter{}, want: failure},
		{name: "write failure", claims: fakeClaimSource{claim: Claim{Snapshot: plan, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "token"}}, found: true}, states: fakeStateReader{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionPending}}}, writer: &recordingDecisionWriter{err: failure}, want: failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator := NewCoordinator(test.claims, test.states, test.writer)
			_, err := coordinator.ProcessNext(context.Background(), "worker", 10)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCoordinatorReportsDecisionAndReleaseFailures(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	claim := Claim{Snapshot: plan, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "token"}}
	decisionFailureState := []EntryState{{ExecutionID: "foreign", Status: execution.ExecutionPending}}
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
		if !strings.Contains(err.Error(), "decide run advance") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("release failure is returned after success", func(t *testing.T) {
		released := 0
		coordinator := NewCoordinator(
			fakeClaimSource{claim: claim, found: true, releaseErr: releaseFailure, released: &released},
			fakeStateReader{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionRunning}}},
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
		if !errors.Is(err, releaseFailure) || !strings.Contains(err.Error(), "decide run advance") {
			t.Fatalf("joined error = %v", err)
		}
	})
}

func sealedCoordinatorPlan(t *testing.T) execution.RunSnapshot {
	t.Helper()
	workflow := execution.WorkflowSnapshot{ID: "workflow", FlowFragmentID: "workflow", VersionID: "workflow-v1", DisplayName: "FlowFragment", VersionNumber: 1, Steps: []execution.Step{{ID: "noop", DisplayName: "Noop", Kind: execution.ActionStep, Action: "press", Value: "Enter"}}}
	draft := execution.Draft{RunID: "run", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-1", TestTaskItemID: "item-1", SequenceNumber: 1, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1"}}, Workflows: []execution.WorkflowSnapshot{workflow}}
	snapshot, err := execution.SealRunSnapshot(execution.RunSnapshotInput{SchemaVersion: execution.RunSnapshotSchemaV1, RunID: "run", ExecutionFlowID: "task", TestTaskVersionID: "task-v1", TestTaskVersionNumber: 1, ExecutionFlow: execution.TestTaskSnapshot{ID: "task"}, ExecutionFlowVersion: execution.ExecutionFlowVersionSnapshot{ID: "task-v1", ExecutionFlowID: "task", VersionNumber: 1, Items: []execution.ExecutionFlowVersionItemSnapshot{{ID: "item-1", TestTaskVersionID: "task-v1", SequenceNumber: 1, FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1"}}}, Plan: draft, Invocations: []execution.InvocationScopeSnapshot{{Path: "execution-1", FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", Values: map[string]parameter.Value{}}}, Environment: execution.EnvironmentSnapshot{ID: "env", Revision: 1, DisplayName: "Environment", BaseURL: "https://example.test", Properties: map[string]string{}}, FailurePolicy: execution.FailurePolicyStopOnFailure, ScreenshotPolicy: execution.ScreenshotPolicySnapshot{Version: execution.ScreenshotPolicyV1, Enabled: true, Destination: "artifacts"}, HealerPolicy: execution.DefaultHealerPolicySnapshot()})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
