package scheduling

import (
	"context"
	"math"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// Every command in this file is checked against ExpectedRevision+1 once the
// adapter answers. At MaxInt64 that sum wraps to MinInt64, which no honest
// adapter can return — so a command that should have been rejected as
// nonsensical instead reaches the store, mutates state, and only then fails as
// an "adapter contract violation". The bound has to be enforced before the
// port call, and these tests assert the port is never reached.

// revisionBoundStore fails loudly rather than answering, because reaching it
// at all is the defect under test.
type revisionBoundStore struct{ calls int }

func (s *revisionBoundStore) Cancel(context.Context, CancelInstanceCommand) (InstanceCommandResult, error) {
	s.calls++
	return InstanceCommandResult{}, nil
}
func (s *revisionBoundStore) Abort(context.Context, AbortInstanceCommand) (InstanceCommandResult, error) {
	s.calls++
	return InstanceCommandResult{}, nil
}
func (s *revisionBoundStore) Reorder(context.Context, ReorderQueueCommand) (ReorderQueueResult, error) {
	s.calls++
	return ReorderQueueResult{}, nil
}

func revisionBoundInstanceID(t *testing.T) domainexecution.InstanceID {
	t.Helper()
	id, err := domainexecution.NewInstanceID("instance-revision-bound")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInstanceCommandsRejectUnrepresentableExpectedRevision(t *testing.T) {
	instanceID := revisionBoundInstanceID(t)
	tests := []struct {
		name     string
		revision int64
		reject   bool
	}{
		{name: "zero", revision: 0},
		{name: "one", revision: 1},
		{name: "largest representable successor", revision: MaxExpectedRevision},
		{name: "successor overflows", revision: math.MaxInt64, reject: true},
		{name: "negative", revision: -1, reject: true},
		{name: "most negative", revision: math.MinInt64, reject: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("cancel", func(t *testing.T) {
				store := &revisionBoundStore{}
				_, err := NewCancelInstanceService(store, nil).CancelInstance(context.Background(), CancelInstanceCommand{
					CommandID: "command", InstanceID: instanceID, ExpectedStatus: domainexecution.Queued,
					ExpectedRevision: test.revision, At: 1,
				})
				assertRevisionBound(t, test.reject, CodeCancelInstanceCommandInvalid, err, store.calls)
			})
			t.Run("abort", func(t *testing.T) {
				store := &revisionBoundStore{}
				_, err := NewAbortInstanceService(store, nil).AbortInstance(context.Background(), AbortInstanceCommand{
					CommandID: "command", InstanceID: instanceID, ExpectedRevision: test.revision, At: 1,
					Fence: domainexecution.WorkerFence{InstanceID: instanceID, ClaimToken: "claim"},
				})
				assertRevisionBound(t, test.reject, CodeAbortInstanceCommandInvalid, err, store.calls)
			})
			t.Run("reorder", func(t *testing.T) {
				store := &revisionBoundStore{}
				_, err := NewReorderQueueService(store).ReorderQueue(context.Background(), ReorderQueueCommand{
					CommandID: "command", ScopeID: "scope", ExpectedRevision: test.revision,
					InstanceIDs: []string{"instance-a"},
				})
				assertRevisionBound(t, test.reject, CodeReorderQueueCommandInvalid, err, store.calls)
			})
		})
	}
}

// assertRevisionBound checks the two things that matter: an out-of-range
// revision is refused with the command's own invalid-argument code, and the
// store was never asked. An in-range revision must get past validation — what
// happens afterwards is the adapter contract's business, not this bound's.
func assertRevisionBound(t *testing.T, reject bool, code fault.Code, err error, storeCalls int) {
	t.Helper()
	if reject {
		if !fault.IsCode(err, code) {
			t.Fatalf("error = %v, want %q", err, code)
		}
		if storeCalls != 0 {
			t.Fatalf("an out-of-range revision still reached the store %d times", storeCalls)
		}
		return
	}
	if fault.IsCode(err, code) {
		t.Fatalf("an in-range revision was rejected as an invalid command: %v", err)
	}
	if storeCalls != 1 {
		t.Fatalf("store calls = %d, want the in-range command to reach the store once", storeCalls)
	}
}

// TestMaxExpectedRevisionSuccessorIsRepresentable states the property the
// constant exists for, so a future edit to the bound cannot quietly reopen the
// wrap.
func TestMaxExpectedRevisionSuccessorIsRepresentable(t *testing.T) {
	if MaxExpectedRevision >= math.MaxInt64 {
		t.Fatalf("MaxExpectedRevision = %d leaves no room for the +1 every result check performs", int64(MaxExpectedRevision))
	}
	if MaxExpectedRevision+1 <= 0 {
		t.Fatalf("MaxExpectedRevision+1 = %d wrapped", MaxExpectedRevision+1)
	}
}
