package scheduling

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type coordinatorPortStub struct {
	claim                Claim
	found                bool
	claimErr, releaseErr error
	worker               string
	at                   int64
	released             Claim
	releaseContextErr    error
}

func (s *coordinatorPortStub) ClaimNext(_ context.Context, worker string, at int64) (Claim, bool, error) {
	s.worker, s.at = worker, at
	return s.claim, s.found, s.claimErr
}
func (s *coordinatorPortStub) Release(ctx context.Context, claim Claim) error {
	s.released, s.releaseContextErr = claim, ctx.Err()
	return s.releaseErr
}

type statePortStub struct {
	claim  Claim
	states []EntryState
	err    error
}

func (s *statePortStub) LoadEntryStates(_ context.Context, claim Claim) ([]EntryState, error) {
	s.claim = claim
	return s.states, s.err
}

type writerPortStub struct {
	claim    Claim
	decision Decision
	at       int64
	result   ApplyDecisionResult
	err      error
}

func (s *writerPortStub) ApplyDecision(_ context.Context, claim Claim, decision Decision, at int64) (ApplyDecisionResult, error) {
	s.claim, s.decision, s.at = claim, decision, at
	return s.result, s.err
}

func TestCoordinatorExportedBoundaryAndReleaseSemantics(t *testing.T) {
	plan := sealedCoordinatorPlan(t)
	claim := Claim{Snapshot: plan, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "token"}}
	releaseFailure, stateFailure := errors.New("release"), errors.New("states")

	claims := &coordinatorPortStub{claim: claim, found: true, releaseErr: releaseFailure}
	states := &statePortStub{err: stateFailure}
	_, err := NewCoordinator(claims, states, &writerPortStub{}).ProcessNext(context.Background(), "worker-x", 42)
	if !errors.Is(err, stateFailure) || !errors.Is(err, releaseFailure) {
		t.Fatalf("joined error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	claims = &coordinatorPortStub{claim: claim, found: true}
	states = &statePortStub{states: []EntryState{{ExecutionID: "execution-1", Status: execution.ExecutionPending}}}
	writer := &writerPortStub{result: ApplyDecisionResult{Fence: claim.Fence, Applied: true}}
	claimed, err := NewCoordinator(claims, states, writer).ProcessNext(ctx, "worker-x", 42)
	if err != nil || !claimed || claims.releaseContextErr != nil {
		t.Fatalf("claimed/error/release context = %v/%v/%v", claimed, err, claims.releaseContextErr)
	}
	if claims.worker != "worker-x" || claims.at != 42 || !reflect.DeepEqual(claims.released, claim) || !reflect.DeepEqual(states.claim, claim) || !reflect.DeepEqual(writer.claim, claim) || writer.at != 42 {
		t.Fatalf("ports did not receive exact arguments")
	}

	var nilClaims *coordinatorPortStub
	for _, tc := range []struct {
		name string
		c    Coordinator
	}{
		{"nil claims", NewCoordinator(nil, states, writer)}, {"typed nil claims", NewCoordinator(nilClaims, states, writer)},
		{"nil states", NewCoordinator(claims, nil, writer)}, {"nil writer", NewCoordinator(claims, states, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() != nil {
					t.Fatal("panicked")
				}
			}()
			if _, err := tc.c.ProcessNext(context.Background(), "w", 1); err == nil {
				t.Fatal("expected dependency error")
			}
		})
	}
}

func validCancelCommand() CancelRunCommand {
	return CancelRunCommand{CommandID: "c", RunID: "run", ExpectedStatus: execution.Running, ExpectedRevision: 1, At: 2}
}
func validAbortCommand() AbortRunCommand {
	return AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "token"}}
}

func TestRunCommandServicesExportedInvalidAndDependencyMatrix(t *testing.T) {
	cancelInvalid := []CancelRunCommand{{}, {CommandID: "c", RunID: "run", ExpectedStatus: execution.Running, ExpectedRevision: -1, At: 2}, {CommandID: "c", RunID: "run", ExpectedStatus: execution.Running, At: 0}, {CommandID: "c", RunID: "run", ExpectedStatus: execution.Succeeded, At: 2}}
	abortInvalid := []AbortRunCommand{{}, {CommandID: "a", RunID: "run", ExpectedRevision: -1, At: 2, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "t"}}, {CommandID: "a", RunID: "run", At: 0, Fence: execution.WorkerFence{RunID: "run", ClaimToken: "t"}}, {CommandID: "a", RunID: "run", At: 2, Fence: execution.WorkerFence{RunID: "other", ClaimToken: "t"}}, {CommandID: "a", RunID: "run", At: 2, Fence: execution.WorkerFence{RunID: "run"}}}
	for _, command := range cancelInvalid {
		if result, err := NewCancelRunService(nil, nil).CancelRun(context.Background(), command); err == nil || result != (RunCommandResult{}) {
			t.Fatalf("cancel invalid result/error=%#v/%v", result, err)
		}
	}
	for _, command := range abortInvalid {
		if result, err := NewAbortRunService(nil, nil).AbortRun(context.Background(), command); err == nil || result != (RunCommandResult{}) {
			t.Fatalf("abort invalid result/error=%#v/%v", result, err)
		}
	}
	var typedNilStore *commandStoreStub
	for _, call := range []func() error{
		func() error {
			_, err := NewCancelRunService(nil, nil).CancelRun(context.Background(), validCancelCommand())
			return err
		},
		func() error {
			_, err := NewCancelRunService(typedNilStore, nil).CancelRun(context.Background(), validCancelCommand())
			return err
		},
		func() error {
			_, err := NewAbortRunService(nil, nil).AbortRun(context.Background(), validAbortCommand())
			return err
		},
		func() error {
			_, err := NewAbortRunService(typedNilStore, nil).AbortRun(context.Background(), validAbortCommand())
			return err
		},
	} {
		func() {
			defer func() {
				if recover() != nil {
					t.Fatal("nil dependency panicked")
				}
			}()
			err := call()
			if !fault.IsCode(err, CodeSchedulingDependencyRequired) {
				t.Fatalf("dependency error = %v", err)
			}
		}()
	}
}

func TestCancelSignalFailureReturnsCommittedResult(t *testing.T) {
	store := &commandStoreStub{cancelResult: RunCommandResult{Run: validCommandRun(t, execution.Canceled), Revision: 2, WasApplied: true, SignalRequired: true}}
	result, err := NewCancelRunService(store, signalStub{store: store, err: errors.New("down")}).CancelRun(context.Background(), validCancelCommand())
	if !fault.IsCode(err, CodeRunSignalRetryable) || !result.WasApplied {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
}

func TestReorderQueueExportedBoundaryAndOwnership(t *testing.T) {
	invalid := []ReorderQueueCommand{{}, {CommandID: "r", ScopeID: "s", ExpectedRevision: -1, RunIDs: []string{"a"}}, {CommandID: "r", ScopeID: "s", RunIDs: nil}, {CommandID: "r", ScopeID: "s", RunIDs: []string{" "}}}
	for _, command := range invalid {
		if result, err := NewReorderQueueService(nil).ReorderQueue(context.Background(), command); err == nil || !reflect.DeepEqual(result, ReorderQueueResult{}) {
			t.Fatalf("invalid result/error=%#v/%v", result, err)
		}
	}
	var typedNil *queueStoreStub
	for _, store := range []QueueCommandStore{nil, typedNil} {
		func() {
			defer func() {
				if recover() != nil {
					t.Fatal("nil store panicked")
				}
			}()
			if _, err := NewReorderQueueService(store).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r", ScopeID: "s", RunIDs: []string{"a"}}); err == nil {
				t.Fatal("expected error")
			}
		}()
	}
	failure := errors.New("store")
	result, err := NewReorderQueueService(&queueStoreStub{result: ReorderQueueResult{ScopeID: "bad"}, err: failure}).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r", ScopeID: "s", RunIDs: []string{"a"}})
	if !errors.Is(err, failure) || !reflect.DeepEqual(result, ReorderQueueResult{}) {
		t.Fatalf("store result/error=%#v/%v", result, err)
	}
	input := []string{"b", "a"}
	store := &queueStoreStub{result: ReorderQueueResult{ScopeID: "s", Revision: 1, RunIDs: []string{"b", "a"}, WasApplied: true}}
	result, err = NewReorderQueueService(store).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r", ScopeID: "s", RunIDs: input})
	input[0] = "changed"
	store.seen.RunIDs[1] = "changed"
	if err != nil || !reflect.DeepEqual(result.RunIDs, []string{"b", "a"}) {
		t.Fatalf("owned result/error=%#v/%v", result, err)
	}
}
