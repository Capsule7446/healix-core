package conformancetest

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/application/execution"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// AbortFaultPoint names one place a host adapter can be made to fail while
// recording a pending abort intent.
//
// The points are the write stages AbortRequestTransaction declares atomic. A
// host that commits any of them outside the transaction is caught here by the
// rollback check rather than by a reviewer reading the adapter. The empty value
// means "no fault", which is what ClearFault restores.
type AbortFaultPoint string

const (
	// AbortFaultBeforeReplay fails inside LookupAbortRequest, before it reads
	// anything. A host that had already made the request visible by then would
	// leave the caller unable to tell a fresh request from a half-applied one.
	AbortFaultBeforeReplay AbortFaultPoint = "BEFORE_REPLAY"
	// AbortFaultAfterDecision fails once the intent has been validated and
	// before the first write.
	AbortFaultAfterDecision AbortFaultPoint = "AFTER_DECISION"
	// AbortFaultAfterIntent fails once the pending terminal intent has been
	// written under its compare-and-swap.
	AbortFaultAfterIntent AbortFaultPoint = "AFTER_INTENT"
	// AbortFaultAfterReceipt fails once the abort command receipt has been
	// written, which is the last write before the idempotency receipt. It is the
	// point where a non-atomic host looks most convincingly finished.
	AbortFaultAfterReceipt AbortFaultPoint = "AFTER_RECEIPT"
)

// AbortFaultPoints lists every point the suite injects, lookup first and then
// the write stages in commit order. Hosts may call it to drive their own tests
// over the same set instead of restating it.
func AbortFaultPoints() []AbortFaultPoint {
	return []AbortFaultPoint{
		AbortFaultBeforeReplay,
		AbortFaultAfterDecision,
		AbortFaultAfterIntent,
		AbortFaultAfterReceipt,
	}
}

// AbortSnapshot is everything one abort request is allowed to change, read back
// as a single comparable value.
//
// EntryStatus is present precisely because a request must never change it. The
// counters report how many rows each class of write produced; the suite compares
// them rather than fixing their magnitude, because how many rows one request
// produces is a host's business but whether a rolled-back attempt left any
// behind is not.
type AbortSnapshot struct {
	EntryStatus            domainexecution.EntryStatus
	TerminalIntent         execution.TerminalIntent
	TerminalIntentRevision int64
	CancellationGeneration int64
	PendingIntents         int
	CommandReceipts        int
	// IdempotencyReceipts is the third write the port declares atomic, and it
	// needs reading back for two reasons the other two counters cannot cover: a
	// receipt committed outside the transaction makes a crashed attempt look
	// applied on retry, and a receipt appended again on every replay grows
	// without bound while the recorded outcome stays correct.
	IdempotencyReceipts int
}

// AbortFixture is one host adapter under test, plus the two things the suite
// needs that the port itself does not expose: a way to read committed state
// back, and a way to make the adapter fail at a chosen point.
//
// SetFault and ClearFault must be safe to call from the goroutine driving the
// test while no request is in flight; the suite never changes a fault
// concurrently with a call it expects to observe that fault.
type AbortFixture interface {
	execution.AbortRequestTransaction
	// Fence is the worker authority this fixture accepts.
	Fence() domainexecution.WorkerFence
	// EntryID is the entry this fixture holds.
	EntryID() domainexecution.EntryID
	// Snapshot reads back everything an abort request may have changed.
	Snapshot() AbortSnapshot
	// SetFault arms a failure at one point of the next request.
	SetFault(AbortFaultPoint)
	// ClearFault disarms whatever SetFault armed.
	ClearFault()
}

// AbortFactory builds one fresh fixture holding one running entry in the
// supplied state.
//
// It must be deterministic in its identities: every fixture it returns reports
// the same Fence and EntryID, and two fixtures built from the same state start
// with identical snapshots. The suite builds a clean run and a crash-then-retry
// run from two fixtures and compares them, which is only meaningful when those
// two promises hold.
type AbortFactory func(t *testing.T, state execution.EntryCompletionState) AbortFixture

// RunAbortRequest runs the abort request conformance suite against one host
// adapter.
//
// It exercises AbortRequestTransaction through AbortRequestService, because
// that is the only supported way to reach the port and a host that passes only
// when driven directly has not been tested on the path it will actually run.
// Every subtest builds its own fixture, so a failure leaves no residue for the
// next one.
func RunAbortRequest(t *testing.T, factory AbortFactory) {
	t.Helper()

	t.Run("applies-once-then-replays", func(t *testing.T) {
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewAbortRequestService(fixture)
		command := abortCommand(fixture, state, "command-1")
		decision := mustAbortDecision(t, command)

		applied, err := service.Request(context.Background(), command)
		if err != nil {
			t.Fatalf("Request() = %v, want nil", err)
		}
		if applied.Status != execution.RequestAbortApplied {
			t.Fatalf("Request() status = %s, want %s", applied.Status, execution.RequestAbortApplied)
		}
		if applied.Decision != decision {
			t.Fatalf("Request() decision = %+v, want %+v", applied.Decision, decision)
		}
		after := fixture.Snapshot()
		assertAbortRecorded(t, after, decision)

		replayed, err := service.Request(context.Background(), command)
		if err != nil {
			t.Fatalf("replayed Request() = %v, want nil", err)
		}
		if replayed.Status != execution.RequestAbortReplayed {
			t.Fatalf("replayed status = %s, want %s", replayed.Status, execution.RequestAbortReplayed)
		}
		if replayed.Decision != decision {
			t.Fatalf("replayed decision = %+v, want the recorded %+v", replayed.Decision, decision)
		}
		if again := fixture.Snapshot(); again != after {
			t.Fatalf("replay changed committed state: %+v -> %+v", after, again)
		}
	})

	t.Run("request-leaves-the-entry-running", func(t *testing.T) {
		// This is the whole reason D-17 is a separate contract from D-12. An
		// adapter that terminated the entry here would give the instance two
		// terminal write paths that can disagree, and would strand the
		// completion's authority compare-and-swap on a row it no longer matches.
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		command := abortCommand(fixture, state, "command-1")

		if _, err := execution.NewAbortRequestService(fixture).Request(context.Background(), command); err != nil {
			t.Fatalf("Request() = %v, want nil", err)
		}
		after := fixture.Snapshot()
		if after.EntryStatus != before.EntryStatus {
			t.Fatalf("entry status moved from %q to %q; an abort request records intent and ends nothing", before.EntryStatus, after.EntryStatus)
		}
		if after.EntryStatus != domainexecution.EntryRunning {
			t.Fatalf("entry status = %q, want it still %q", after.EntryStatus, domainexecution.EntryRunning)
		}
		if after.CancellationGeneration != before.CancellationGeneration {
			t.Fatalf("cancellation generation moved from %d to %d; a request does not spend one", before.CancellationGeneration, after.CancellationGeneration)
		}
	})

	t.Run("escalates-a-cancel-and-refuses-a-second-abort", func(t *testing.T) {
		escalating := abortRunningState(execution.TerminalIntentCancel)
		fixture := factory(t, escalating)
		command := abortCommand(fixture, escalating, "command-1")
		applied, err := execution.NewAbortRequestService(fixture).Request(context.Background(), command)
		if err != nil {
			t.Fatalf("escalating Request() = %v, want nil", err)
		}
		if applied.Decision.NextIntent != execution.TerminalIntentAbort {
			t.Fatalf("next intent = %q, want %q", applied.Decision.NextIntent, execution.TerminalIntentAbort)
		}

		aborting := abortRunningState(execution.TerminalIntentAbort)
		repeat := factory(t, aborting)
		before := repeat.Snapshot()
		_, err = execution.NewAbortRequestService(repeat).Request(context.Background(), abortCommand(repeat, aborting, "command-2"))
		if !fault.IsCode(err, execution.CodeAbortRequestAlreadyAborting) {
			t.Fatalf("error = %v, want %s", err, execution.CodeAbortRequestAlreadyAborting)
		}
		if after := repeat.Snapshot(); after != before {
			t.Fatalf("refused request changed committed state: %+v -> %+v", before, after)
		}
	})

	t.Run("faults-roll-back-all-observable-state", func(t *testing.T) {
		for _, point := range AbortFaultPoints() {
			t.Run(string(point), func(t *testing.T) {
				state := abortRunningState(execution.TerminalIntentNone)
				fixture := factory(t, state)
				service := execution.NewAbortRequestService(fixture)
				command := abortCommand(fixture, state, "command-1")
				decision := mustAbortDecision(t, command)
				before := fixture.Snapshot()

				fixture.SetFault(point)
				if _, err := service.Request(context.Background(), command); err == nil {
					t.Fatalf("Request() succeeded with a fault armed at %s", point)
				}
				if crashed := fixture.Snapshot(); crashed != before {
					t.Fatalf("fault at %s left state behind: %+v -> %+v", point, before, crashed)
				}

				// A retry after the crash must be indistinguishable from a first
				// clean attempt: same decision, same committed state.
				fixture.ClearFault()
				retried, err := service.Request(context.Background(), command)
				if err != nil {
					t.Fatalf("retry after %s = %v, want nil", point, err)
				}
				if retried.Status != execution.RequestAbortApplied && retried.Status != execution.RequestAbortReplayed {
					t.Fatalf("retry after %s status = %s", point, retried.Status)
				}
				if retried.Decision != decision {
					t.Fatalf("retry after %s decision = %+v, want %+v", point, retried.Decision, decision)
				}
				assertAbortRecorded(t, fixture.Snapshot(), decision)
			})
		}
	})

	// The next two subtests are deliberately separate. A stale fence and a stale
	// observed state are both "somebody else moved first", but the caller reacts
	// oppositely: a lost claim means stop, a moved state means re-read and
	// rebuild. An adapter that answered both with one code — or with an
	// unclassified storage error — would leave the host unable to tell them
	// apart, so each asserts its own code rather than merely "an error".
	t.Run("stale-fence-writes-nothing", func(t *testing.T) {
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		stale := abortCommand(fixture, state, "command-1")
		stale.Fence.ClaimToken += "-stale"

		_, err := execution.NewAbortRequestService(fixture).Request(context.Background(), stale)
		if !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
			t.Fatalf("stale fence error = %v, want code %s", err, domainexecution.CodeWorkerFenceStale)
		}
		if after := fixture.Snapshot(); after != before {
			t.Fatalf("stale fence changed committed state: %+v -> %+v", before, after)
		}
	})

	t.Run("stale-observed-state-conflicts-and-writes-nothing", func(t *testing.T) {
		state := abortRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		// The command is well formed and its fence is current; it simply
		// observed a revision the entry has already moved past.
		stale := abortCommand(fixture, state, "command-1")
		stale.State.TerminalIntentRevision = state.TerminalIntentRevision + 1

		_, err := execution.NewAbortRequestService(fixture).Request(context.Background(), stale)
		if !fault.IsCode(err, execution.CodeRequestAbortIdentityConflict) {
			t.Fatalf("stale state error = %v, want code %s", err, execution.CodeRequestAbortIdentityConflict)
		}
		if after := fixture.Snapshot(); after != before {
			t.Fatalf("stale state changed committed state: %+v -> %+v", before, after)
		}
	})
}

// assertAbortRecorded holds a committed request to the decision core produced:
// the two counters must be the Next* pair verbatim, and exactly one row of each
// class must exist however many attempts it took.
func assertAbortRecorded(t *testing.T, snapshot AbortSnapshot, decision execution.AbortRequestDecision) {
	t.Helper()
	if snapshot.TerminalIntent != decision.NextIntent {
		t.Fatalf("terminal intent = %q, want %q", snapshot.TerminalIntent, decision.NextIntent)
	}
	if snapshot.TerminalIntentRevision != decision.NextIntentRevision {
		t.Fatalf("intent revision = %d, want %d", snapshot.TerminalIntentRevision, decision.NextIntentRevision)
	}
	if snapshot.CancellationGeneration != decision.NextCancellationGeneration {
		t.Fatalf("cancellation generation = %d, want %d", snapshot.CancellationGeneration, decision.NextCancellationGeneration)
	}
	if snapshot.PendingIntents != 1 {
		t.Fatalf("pending intents = %d, want exactly 1", snapshot.PendingIntents)
	}
	if snapshot.CommandReceipts != 1 {
		t.Fatalf("command receipts = %d, want exactly 1", snapshot.CommandReceipts)
	}
	if snapshot.IdempotencyReceipts != 1 {
		t.Fatalf("idempotency receipts = %d, want exactly 1 however many attempts it took", snapshot.IdempotencyReceipts)
	}
}

func abortRunningState(intent execution.TerminalIntent) execution.EntryCompletionState {
	return execution.EntryCompletionState{
		EntryStatus:            domainexecution.EntryRunning,
		TerminalIntent:         intent,
		TerminalIntentRevision: 1,
		CancellationGeneration: 0,
	}
}

func abortCommand(fixture AbortFixture, state execution.EntryCompletionState, commandID string) execution.RequestAbortCommand {
	return execution.RequestAbortCommand{
		EntryID: fixture.EntryID(),
		Fence:   fixture.Fence(),
		State:   state,
		Request: execution.AbortRequest{AbortPendingCommandID: commandID},
	}
}

func mustAbortDecision(t *testing.T, command execution.RequestAbortCommand) execution.AbortRequestDecision {
	t.Helper()
	decision, err := execution.DecideAbortRequest(command.State, command.Request)
	if err != nil {
		t.Fatalf("DecideAbortRequest() = %v", err)
	}
	return decision
}
