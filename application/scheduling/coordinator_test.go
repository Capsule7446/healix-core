package scheduling

import (
	"context"
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
)

type fakeClaimSource struct {
	claim    Claim
	found    bool
	err      error
	released *int
}

func (f fakeClaimSource) ClaimNext(context.Context, string, int64) (Claim, bool, error) {
	return f.claim, f.found, f.err
}

func (f fakeClaimSource) Release(context.Context, Claim) error {
	if f.released != nil {
		*f.released++
	}
	return nil
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
	err       error
}

func (f *recordingDecisionWriter) ApplyDecision(_ context.Context, _ Claim, decision Decision, _ int64) error {
	f.decisions = append(f.decisions, decision)
	return f.err
}

func TestCoordinatorAppliesDecisionUnderClaim(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	writer := &recordingDecisionWriter{}
	released := 0
	coordinator := NewCoordinator(
		fakeClaimSource{claim: Claim{Plan: plan, Token: "claim-token"}, found: true, released: &released},
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
		{name: "running entry", claims: fakeClaimSource{claim: Claim{Plan: plan, Token: "token"}, found: true}, states: fakeStateReader{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionRunning}}}, want: true},
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
		{name: "missing token", claims: fakeClaimSource{claim: Claim{Plan: plan}, found: true}, writer: &recordingDecisionWriter{}, want: ErrInvalidClaim},
		{name: "claim failure", claims: fakeClaimSource{err: failure}, writer: &recordingDecisionWriter{}, want: failure},
		{name: "state failure", claims: fakeClaimSource{claim: Claim{Plan: plan, Token: "token"}, found: true}, states: fakeStateReader{err: failure}, writer: &recordingDecisionWriter{}, want: failure},
		{name: "write failure", claims: fakeClaimSource{claim: Claim{Plan: plan, Token: "token"}, found: true}, states: fakeStateReader{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionPending}}}, writer: &recordingDecisionWriter{err: failure}, want: failure},
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

func sealedCoordinatorPlan(t *testing.T) execution.Plan {
	t.Helper()
	workflow := execution.WorkflowSnapshot{ID: "workflow", WorkflowID: "workflow", VersionID: "workflow-v1", DisplayName: "Workflow", VersionNumber: 1, Steps: []execution.Step{{ID: "noop", DisplayName: "Noop", Kind: execution.ActionStep, Action: "press", Value: "Enter"}}}
	plan, err := execution.Seal(execution.Draft{RunID: "run", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-1", TestTaskItemID: "item-1", SequenceNumber: 1, WorkflowID: "workflow", WorkflowVersionID: "workflow-v1"}}, Workflows: []execution.WorkflowSnapshot{workflow}})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
