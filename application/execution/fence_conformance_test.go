package execution

import (
	"context"
	"sync"
	"testing"

	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type fencedExecutionStore struct {
	mu       sync.Mutex
	active   domainexecution.WorkerFence
	progress int
	terminal int
}

func (s *fencedExecutionStore) check(fence domainexecution.WorkerFence) error {
	if fence != s.active {
		return domainexecution.NewStaleWorkerFenceError()
	}
	return nil
}

func (s *fencedExecutionStore) RecordStepProgress(_ context.Context, fence domainexecution.WorkerFence, _ evidence.StepProgressEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(fence); err != nil {
		return err
	}
	s.progress++
	return nil
}

func (s *fencedExecutionStore) RecordValidationProgress(_ context.Context, fence domainexecution.WorkerFence, _ evidence.ValidationProgressObservation) error {
	return s.RecordStepProgress(context.Background(), fence, evidence.StepProgressEvent{})
}

func (s *fencedExecutionStore) CommitStepTransition(_ context.Context, fence domainexecution.WorkerFence, _ evidence.StepTransitionCommit, _ HealGovernancePlanner) (evidence.StepTransitionCommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(fence); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	s.terminal++
	return evidence.StepTransitionCommitResult{}, nil
}

func TestExecutionWritesRejectStaleWorkerFence(t *testing.T) {
	winner := domainexecution.WorkerFence{RunID: "run", ClaimToken: "winner"}
	stale := domainexecution.WorkerFence{RunID: "run", ClaimToken: "stale"}
	store := &fencedExecutionStore{active: winner}
	if err := store.RecordStepProgress(context.Background(), stale, evidence.StepProgressEvent{}); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
		t.Fatalf("stale progress error=%v", err)
	}
	if _, err := store.CommitStepTransition(context.Background(), stale, evidence.StepTransitionCommit{}, NewDefaultHealGovernancePlanner()); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
		t.Fatalf("stale terminal error=%v", err)
	}
	if err := store.RecordStepProgress(context.Background(), winner, evidence.StepProgressEvent{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitStepTransition(context.Background(), winner, evidence.StepTransitionCommit{}, NewDefaultHealGovernancePlanner()); err != nil {
		t.Fatal(err)
	}
	if store.progress != 1 || store.terminal != 1 {
		t.Fatalf("writes=%d/%d", store.progress, store.terminal)
	}
	reacquired := domainexecution.WorkerFence{RunID: "run", ClaimToken: "winner-next-acquisition"}
	store.active = reacquired
	if err := store.RecordStepProgress(context.Background(), winner, evidence.StepProgressEvent{}); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
		t.Fatalf("released ABA progress error=%v", err)
	}
	if _, err := store.CommitStepTransition(context.Background(), winner, evidence.StepTransitionCommit{}, NewDefaultHealGovernancePlanner()); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
		t.Fatalf("released ABA terminal error=%v", err)
	}
	if err := store.RecordStepProgress(context.Background(), reacquired, evidence.StepProgressEvent{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitStepTransition(context.Background(), reacquired, evidence.StepTransitionCommit{}, NewDefaultHealGovernancePlanner()); err != nil {
		t.Fatal(err)
	}
}

var _ ProgressWriter = (*fencedExecutionStore)(nil)
var _ StepTransitionTransaction = (*fencedExecutionStore)(nil)
