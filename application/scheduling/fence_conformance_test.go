package scheduling

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type fencedSchedulingStore struct {
	mu       sync.Mutex
	snapshot execution.InstanceSnapshot
	active   execution.WorkerFence
	claims   uint64
	starts   int
}

func (s *fencedSchedulingStore) ClaimNext(_ context.Context, worker string, _ int64) (Claim, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active.ClaimToken != "" {
		return Claim{}, false, nil
	}
	s.claims++
	s.active = execution.WorkerFence{RunID: s.snapshot.RunID(), ClaimToken: fmt.Sprintf("%s-%d", worker, s.claims)}
	return Claim{Snapshot: s.snapshot, Fence: s.active}, true, nil
}

func (s *fencedSchedulingStore) Release(_ context.Context, claim Claim) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if claim.Fence != s.active {
		return execution.NewStaleWorkerFenceError()
	}
	s.active = execution.WorkerFence{}
	return nil
}

func (s *fencedSchedulingStore) ApplyDecision(_ context.Context, claim Claim, decision Decision, _ int64) (ApplyDecisionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if claim.Fence != s.active {
		return ApplyDecisionResult{}, execution.NewStaleWorkerFenceError()
	}
	if decision.NextExecutionID != (execution.EntryID{}) {
		s.starts++
	}
	return ApplyDecisionResult{Fence: claim.Fence, Applied: true}, nil
}

func TestClaimAndDecisionFenceConformance(t *testing.T) {
	store := &fencedSchedulingStore{snapshot: sealedCoordinatorPlan(t)}
	claims := make(chan Claim, 2)
	var wg sync.WaitGroup
	for _, worker := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			claim, found, err := store.ClaimNext(context.Background(), worker, 1)
			if err != nil {
				t.Errorf("claim: %v", err)
			}
			if found {
				claims <- claim
			}
		}(worker)
	}
	wg.Wait()
	close(claims)
	var winner Claim
	count := 0
	for claim := range claims {
		winner, count = claim, count+1
	}
	if count != 1 {
		t.Fatalf("claim winners=%d", count)
	}
	stale := winner
	stale.Fence.ClaimToken = "stale"
	decision := Decision{NextExecutionID: mustEntryID("execution-1")}
	if _, err := store.ApplyDecision(context.Background(), stale, decision, 2); !fault.IsCode(err, execution.CodeWorkerFenceStale) {
		t.Fatalf("stale decision error=%v", err)
	}
	if _, err := store.ApplyDecision(context.Background(), winner, decision, 2); err != nil {
		t.Fatal(err)
	}
	if store.starts != 1 {
		t.Fatalf("successor starts=%d", store.starts)
	}
	if err := store.Release(context.Background(), stale); !fault.IsCode(err, execution.CodeWorkerFenceStale) {
		t.Fatalf("stale release error=%v", err)
	}
	if err := store.Release(context.Background(), winner); err != nil {
		t.Fatal(err)
	}
	reacquired, found, err := store.ClaimNext(context.Background(), "worker-a", 3)
	if err != nil || !found {
		t.Fatalf("claim after release=%v/%v", found, err)
	}
	if reacquired.Fence.ClaimToken == winner.Fence.ClaimToken {
		t.Fatal("same worker reacquisition reused claim token")
	}
	if _, err := store.ApplyDecision(context.Background(), winner, decision, 4); !fault.IsCode(err, execution.CodeWorkerFenceStale) {
		t.Fatalf("released ABA fence applied decision: %v", err)
	}
	if err := store.Release(context.Background(), winner); !fault.IsCode(err, execution.CodeWorkerFenceStale) {
		t.Fatalf("released ABA fence released new claim: %v", err)
	}
	if _, err := store.ApplyDecision(context.Background(), reacquired, decision, 4); err != nil {
		t.Fatal(err)
	}
}
