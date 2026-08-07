package conformancetest

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Capsule7446/healix-core/application/engine"
	execution "github.com/Capsule7446/healix-core/application/execution"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// CompletionFaultPoint names a place inside one entry completion where the
// suite asks a fixture to fail.
//
// The points are the write stages EntryCompletionTransaction declares atomic.
// A host that commits any of them outside the transaction is caught here by the
// rollback check rather than by a reviewer reading the adapter. The empty value
// means "no fault", which is what ClearFault restores.
type CompletionFaultPoint string

const (
	// CompletionFaultBeforeReplay fails inside LookupEntryCompletion, before it
	// reads anything. A host that had already made the completion visible by
	// then would leave the caller unable to tell a fresh request from a
	// half-applied one.
	CompletionFaultBeforeReplay CompletionFaultPoint = "BEFORE_REPLAY"
	// CompletionFaultAfterDecision fails once the intent has been validated and
	// before the first write.
	CompletionFaultAfterDecision CompletionFaultPoint = "AFTER_DECISION"
	// CompletionFaultAfterEntry fails once the entry's terminal status and the
	// two counters have been written.
	CompletionFaultAfterEntry CompletionFaultPoint = "AFTER_ENTRY"
	// CompletionFaultAfterFacts fails once the entry's terminal facts have been
	// written.
	CompletionFaultAfterFacts CompletionFaultPoint = "AFTER_FACTS"
	// CompletionFaultAfterEvidence fails once the run's evidence references
	// have been written.
	CompletionFaultAfterEvidence CompletionFaultPoint = "AFTER_EVIDENCE"
	// CompletionFaultAfterGate fails once the execution action gate has been
	// terminalized.
	CompletionFaultAfterGate CompletionFaultPoint = "AFTER_GATE"
	// CompletionFaultAfterOutbox fails once the outbox record has been written,
	// which is the last write before the idempotency receipt. It is the point
	// where a non-atomic host looks most convincingly finished.
	CompletionFaultAfterOutbox CompletionFaultPoint = "AFTER_OUTBOX"
)

// CompletionFaultPoints lists every point the suite injects, lookup first and
// then the write stages in commit order. Hosts may call it to drive their own
// tests over the same set instead of restating it.
func CompletionFaultPoints() []CompletionFaultPoint {
	return []CompletionFaultPoint{
		CompletionFaultBeforeReplay,
		CompletionFaultAfterDecision,
		CompletionFaultAfterEntry,
		CompletionFaultAfterFacts,
		CompletionFaultAfterEvidence,
		CompletionFaultAfterGate,
		CompletionFaultAfterOutbox,
	}
}

// CompletionSnapshot is everything one entry completion is allowed to change,
// read back as a single comparable value.
//
// The first four fields are the entry's own state and must end up exactly equal
// to the decision core produced. The counters report how many times the fixture
// performed each class of write; the suite compares them rather than fixing
// their magnitude, because how many rows one completion produces is a host's
// business but whether a rolled-back attempt left any behind is not.
type CompletionSnapshot struct {
	EntryStatus domainexecution.EntryStatus
	// TerminalCause is read back for the same reason EntryStatus is: D-18
	// exists so a crash-interrupted entry stays distinguishable from one that
	// ran and failed, and a host that decided the cause but never persisted it
	// would leave both as FAILED with the suite none the wiser.
	TerminalCause          execution.TerminalCause
	TerminalIntent         execution.TerminalIntent
	TerminalIntentRevision int64
	CancellationGeneration int64
	Completions            int
	TerminalFacts          int
	EvidenceRefs           int
	GateTerminalizations   int
	OutboxRecords          int
}

// CompletionFixture is one host adapter under test, plus the two things the
// suite needs that the port itself does not expose: a way to read committed
// state back, and a way to make the adapter fail at a chosen point.
//
// SetFault and ClearFault must be safe to call from the goroutine driving the
// test while no completion is in flight; the suite never changes a fault
// concurrently with a call it expects to observe that fault.
type CompletionFixture interface {
	execution.EntryCompletionTransaction
	// Fence is the worker authority this fixture accepts. A completion carrying
	// any other fence must be refused with CodeWorkerFenceStale.
	Fence() domainexecution.WorkerFence
	// EntryID is the entry this fixture holds.
	EntryID() domainexecution.EntryID
	// Snapshot reads back everything a completion may have changed.
	Snapshot() CompletionSnapshot
	// SetFault arms a failure at one point of the next completion.
	SetFault(CompletionFaultPoint)
	// ClearFault disarms whatever SetFault armed.
	ClearFault()
}

// CompletionFactory builds one fresh fixture holding one entry in the supplied
// state.
//
// It must be deterministic in its identities: every fixture it returns reports
// the same Fence and EntryID, and two fixtures built from the same state start
// with identical snapshots. The suite builds a clean run and a crash-then-retry
// run from two fixtures and compares them, which is only meaningful when those
// two promises hold.
type CompletionFactory func(t *testing.T, state execution.EntryCompletionState) CompletionFixture

// RunEntryCompletion runs the entry completion conformance suite against one
// host adapter.
//
// It exercises EntryCompletionTransaction through EntryCompletionService,
// because that is the only supported way to reach the port and a host that
// passes only when driven directly has not been tested on the path it will
// actually run. Every subtest builds its own fixture, so a failure leaves no
// residue for the next one.
func RunEntryCompletion(t *testing.T, factory CompletionFactory) {
	t.Helper()

	t.Run("applies-once-then-replays", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "")
		decision := mustCompletionDecision(t, command)

		applied, err := service.Complete(context.Background(), command)
		if err != nil {
			t.Fatalf("Complete() = %v, want nil", err)
		}
		if applied.Status != execution.CompleteEntryApplied {
			t.Fatalf("Complete() status = %s, want %s", applied.Status, execution.CompleteEntryApplied)
		}
		if applied.EntryID != fixture.EntryID() {
			t.Fatalf("Complete() entry = %s, want %s", applied.EntryID, fixture.EntryID())
		}
		if applied.Decision != decision {
			t.Fatalf("Complete() decision = %#v, want %#v", applied.Decision, decision)
		}
		after := fixture.Snapshot()
		assertCompletionCommitted(t, before, after, decision)

		replayed, err := service.Complete(context.Background(), command)
		if err != nil {
			t.Fatalf("replayed Complete() = %v, want nil", err)
		}
		if replayed.Status != execution.CompleteEntryReplayed {
			t.Fatalf("replayed status = %s, want %s", replayed.Status, execution.CompleteEntryReplayed)
		}
		if replayed.Decision != decision || replayed.RequestDigest != applied.RequestDigest {
			t.Fatalf("replay = %#v, want the recorded outcome %#v", replayed, applied)
		}
		if got := fixture.Snapshot(); got != after {
			t.Fatalf("replay changed state: before=%#v after=%#v", after, got)
		}
	})

	t.Run("lookup-of-an-unknown-request-finds-nothing-and-writes-nothing", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "")
		digest, err := execution.CompleteEntryRequestDigest(command)
		if err != nil {
			t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
		}

		recorded, found, err := fixture.LookupEntryCompletion(context.Background(), fixture.EntryID(), digest)
		if err != nil {
			t.Fatalf("LookupEntryCompletion() = %v, want nil", err)
		}
		if found {
			t.Fatalf("LookupEntryCompletion() found %#v for a request never applied", recorded)
		}
		if recorded != (execution.CompleteEntryOutcome{}) {
			t.Fatalf("LookupEntryCompletion() miss returned %#v, want the zero outcome", recorded)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("lookup changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("stale-fence-writes-nothing", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "")
		command.Fence.ClaimToken += "-stale"

		if _, err := service.Complete(context.Background(), command); !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) {
			t.Fatalf("stale fence error = %v, want code %s", err, domainexecution.CodeWorkerFenceStale)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("stale fence changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("stale-observed-state-conflicts-and-writes-nothing", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		// Still RUNNING and still decidable, so the request reaches the adapter
		// and is refused by the compare-and-swap rather than by core.
		drifted := state
		drifted.TerminalIntentRevision++
		command := completionCommand(fixture, drifted, completionFailedOutcome(), "")

		if _, err := service.Complete(context.Background(), command); !fault.IsCode(err, execution.CodeCompleteEntryIdentityConflict) {
			t.Fatalf("stale state error = %v, want code %s", err, execution.CodeCompleteEntryIdentityConflict)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("stale state changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("undecidable-request-leaves-no-trace", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentNone)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		pending := state
		pending.EntryStatus = domainexecution.EntryPending
		command := completionCommand(fixture, pending, completionFailedOutcome(), "")

		if _, err := service.Complete(context.Background(), command); !fault.IsCode(err, execution.CodeEntryCompletionNotRunning) {
			t.Fatalf("undecidable request error = %v, want code %s", err, execution.CodeEntryCompletionNotRunning)
		}
		if got := fixture.Snapshot(); got != before {
			t.Fatalf("undecidable request changed state: before=%#v after=%#v", before, got)
		}
	})

	t.Run("forged-intent-is-refused-and-writes-nothing", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			forge func(execution.CompleteEntryIntent) execution.CompleteEntryIntent
		}{
			{"digest-does-not-match-the-command", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.RequestDigest = "sha256:" + strings.Repeat("0", 64)
				return intent
			}},
			{"host-computed-the-next-intent-revision", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.Decision.NextIntentRevision++
				return intent
			}},
			{"host-computed-the-next-cancellation-generation", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.Decision.NextCancellationGeneration++
				return intent
			}},
			{"host-chose-the-terminal-status", func(intent execution.CompleteEntryIntent) execution.CompleteEntryIntent {
				intent.Decision.EntryStatus = domainexecution.EntryCanceled
				return intent
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				state := completionRunningState(execution.TerminalIntentNone)
				fixture := factory(t, state)
				before := fixture.Snapshot()
				command := completionCommand(fixture, state, completionFailedOutcome(), "abort-command-1")
				digest, err := execution.CompleteEntryRequestDigest(command)
				if err != nil {
					t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
				}
				intent := test.forge(execution.CompleteEntryIntent{
					EntryID:       fixture.EntryID(),
					RequestDigest: digest,
					Command:       command,
					Decision:      mustCompletionDecision(t, command),
				})

				if _, err := fixture.CompleteEntry(context.Background(), intent); !fault.IsCode(err, execution.CodeCompleteEntryDigestMismatch) {
					t.Fatalf("forged intent error = %v, want code %s", err, execution.CodeCompleteEntryDigestMismatch)
				}
				if got := fixture.Snapshot(); got != before {
					t.Fatalf("forged intent changed state: before=%#v after=%#v", before, got)
				}
			})
		}
	})

	t.Run("faults-roll-back-and-the-retry-equals-one-clean-run", func(t *testing.T) {
		for _, point := range CompletionFaultPoints() {
			t.Run(string(point), func(t *testing.T) {
				state := completionRunningState(execution.TerminalIntentCancel)
				outcome := completionFailedOutcome()

				clean := factory(t, state)
				cleanCommand := completionCommand(clean, state, outcome, "abort-command-1")
				if _, err := execution.NewEntryCompletionService(clean).Complete(context.Background(), cleanCommand); err != nil {
					t.Fatalf("clean completion = %v, want nil", err)
				}
				want := clean.Snapshot()

				faulted := factory(t, state)
				service := execution.NewEntryCompletionService(faulted)
				command := completionCommand(faulted, state, outcome, "abort-command-1")
				before := faulted.Snapshot()

				faulted.SetFault(point)
				if _, err := service.Complete(context.Background(), command); err == nil {
					t.Fatalf("completion faulted at %s returned nil error", point)
				}
				if got := faulted.Snapshot(); got != before {
					t.Fatalf("fault %s changed state: before=%#v after=%#v", point, before, got)
				}

				faulted.ClearFault()
				retried, err := service.Complete(context.Background(), command)
				if err != nil {
					t.Fatalf("retry after %s = %v, want nil", point, err)
				}
				if retried.Status != execution.CompleteEntryApplied {
					t.Fatalf("retry after %s status = %s, want %s", point, retried.Status, execution.CompleteEntryApplied)
				}
				if got := faulted.Snapshot(); got != want {
					t.Fatalf("retry after %s is not equivalent to one clean run: got=%#v want=%#v", point, got, want)
				}

				replayed, err := service.Complete(context.Background(), command)
				if err != nil {
					t.Fatalf("replay after %s = %v, want nil", point, err)
				}
				if replayed.Status != execution.CompleteEntryReplayed {
					t.Fatalf("replay after %s status = %s, want %s", point, replayed.Status, execution.CompleteEntryReplayed)
				}
				if replayed.Decision != retried.Decision {
					t.Fatalf("replay after %s decision = %#v, want %#v", point, replayed.Decision, retried.Decision)
				}
				if got := faulted.Snapshot(); got != want {
					t.Fatalf("replay after %s changed state: before=%#v after=%#v", point, want, got)
				}
			})
		}
	})

	t.Run("concurrent-identical-requests-apply-once", func(t *testing.T) {
		state := completionRunningState(execution.TerminalIntentAbort)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionFailedOutcome(), "abort-command-1")
		decision := mustCompletionDecision(t, command)

		const workers = 4
		start := make(chan struct{})
		results := make(chan execution.CompleteEntryOutcome, workers)
		failures := make(chan error, workers)
		var wait sync.WaitGroup
		wait.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wait.Done()
				<-start
				result, err := service.Complete(context.Background(), command)
				results <- result
				failures <- err
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		close(failures)

		// Every caller must succeed: an identical request is by definition a
		// replay, and a host that reported a conflict instead would send a
		// worker back to re-read an entry that already holds the answer.
		for err := range failures {
			if err != nil {
				t.Fatalf("concurrent completion = %v, want nil", err)
			}
		}
		applied := 0
		for result := range results {
			switch result.Status {
			case execution.CompleteEntryApplied:
				applied++
			case execution.CompleteEntryReplayed:
			default:
				t.Fatalf("concurrent completion status = %s", result.Status)
			}
			if result.Decision != decision {
				t.Fatalf("concurrent completion decision = %#v, want %#v", result.Decision, decision)
			}
		}
		if applied != 1 {
			t.Fatalf("concurrent completions applied %d times, want 1", applied)
		}
		assertCompletionCommitted(t, before, fixture.Snapshot(), decision)
	})

	t.Run("engine-success-under-a-cancel-intent-commits-succeeded", func(t *testing.T) {
		// 裁决一 at the persistence boundary: the engine finished, so the
		// external side effects already landed and the record must say so. The
		// cancel is not lost — it rides into the next revision, where
		// DecideAdvance is what actually stops the instance.
		state := completionRunningState(execution.TerminalIntentCancel)
		fixture := factory(t, state)
		service := execution.NewEntryCompletionService(fixture)
		before := fixture.Snapshot()
		command := completionCommand(fixture, state, completionSucceededOutcome(), "abort-command-1")

		if _, err := service.Complete(context.Background(), command); err != nil {
			t.Fatalf("Complete() = %v, want nil", err)
		}
		after := fixture.Snapshot()
		if after.EntryStatus != domainexecution.EntrySucceeded {
			t.Fatalf("entry status = %s, want %s", after.EntryStatus, domainexecution.EntrySucceeded)
		}
		if after.TerminalIntent != execution.TerminalIntentCancel {
			t.Fatalf("terminal intent = %s, want the observed %s", after.TerminalIntent, execution.TerminalIntentCancel)
		}
		if after.TerminalIntentRevision != before.TerminalIntentRevision+1 {
			t.Fatalf("terminal intent revision = %d, want %d", after.TerminalIntentRevision, before.TerminalIntentRevision+1)
		}
		if after.CancellationGeneration != before.CancellationGeneration {
			t.Fatalf("cancellation generation = %d, want the unchanged %d", after.CancellationGeneration, before.CancellationGeneration)
		}
	})

	t.Run("abort-command-identity-does-not-change-what-is-committed", func(t *testing.T) {
		// 裁决二: the pending abort command is idempotency and audit identity,
		// so it must move the request digest and must not move the decision.
		state := completionRunningState(execution.TerminalIntentCancel)
		withoutAbort := factory(t, state)
		withAbort := factory(t, state)
		if withoutAbort.Fence() != withAbort.Fence() || withoutAbort.EntryID() != withAbort.EntryID() {
			t.Fatalf("factory returned differing identities: %#v/%s and %#v/%s",
				withoutAbort.Fence(), withoutAbort.EntryID(), withAbort.Fence(), withAbort.EntryID())
		}
		commandWithout := completionCommand(withoutAbort, state, completionFailedOutcome(), "")
		commandWith := completionCommand(withAbort, state, completionFailedOutcome(), "abort-command-1")

		digestWithout, err := execution.CompleteEntryRequestDigest(commandWithout)
		if err != nil {
			t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
		}
		digestWith, err := execution.CompleteEntryRequestDigest(commandWith)
		if err != nil {
			t.Fatalf("CompleteEntryRequestDigest() = %v, want nil", err)
		}
		if digestWithout == digestWith {
			t.Fatal("commands naming different abort commands share a request digest")
		}

		resultWithout, err := execution.NewEntryCompletionService(withoutAbort).Complete(context.Background(), commandWithout)
		if err != nil {
			t.Fatalf("Complete() without a pending abort = %v, want nil", err)
		}
		resultWith, err := execution.NewEntryCompletionService(withAbort).Complete(context.Background(), commandWith)
		if err != nil {
			t.Fatalf("Complete() with a pending abort = %v, want nil", err)
		}
		if resultWithout.Decision != resultWith.Decision {
			t.Fatalf("abort identity changed the decision: %#v and %#v", resultWithout.Decision, resultWith.Decision)
		}
		if withoutAbort.Snapshot() != withAbort.Snapshot() {
			t.Fatalf("abort identity changed what was committed: %#v and %#v", withoutAbort.Snapshot(), withAbort.Snapshot())
		}
	})
}

// assertCompletionCommitted holds a fixture to the decision it was given.
//
// The four state fields must equal the decision exactly, because they are the
// values core produced and the host promised to write verbatim. Completions
// must advance by one, because that is the whole point of the idempotency
// receipt. The remaining counters are only required not to go backwards: how
// many rows one completion writes is a host's own business, but a completion
// that erased evidence already recorded would not be.
func assertCompletionCommitted(t *testing.T, before, after CompletionSnapshot, decision execution.EntryCompletionDecision) {
	t.Helper()
	if after.EntryStatus != decision.EntryStatus {
		t.Fatalf("entry status = %s, want %s", after.EntryStatus, decision.EntryStatus)
	}
	if after.TerminalCause != decision.TerminalCause {
		t.Fatalf("terminal cause = %s, want %s", after.TerminalCause, decision.TerminalCause)
	}
	if after.TerminalIntent != decision.NextIntent {
		t.Fatalf("terminal intent = %s, want %s", after.TerminalIntent, decision.NextIntent)
	}
	if after.TerminalIntentRevision != decision.NextIntentRevision {
		t.Fatalf("terminal intent revision = %d, want %d", after.TerminalIntentRevision, decision.NextIntentRevision)
	}
	if after.CancellationGeneration != decision.NextCancellationGeneration {
		t.Fatalf("cancellation generation = %d, want %d", after.CancellationGeneration, decision.NextCancellationGeneration)
	}
	if after.Completions != before.Completions+1 {
		t.Fatalf("completions = %d, want %d", after.Completions, before.Completions+1)
	}
	if after.TerminalFacts < before.TerminalFacts ||
		after.EvidenceRefs < before.EvidenceRefs ||
		after.GateTerminalizations < before.GateTerminalizations ||
		after.OutboxRecords < before.OutboxRecords {
		t.Fatalf("a completion discarded writes it had already recorded: before=%#v after=%#v", before, after)
	}
}

// completionRunningState is the only state a completion can start from. The
// revision and generation are distinct non-zero values so a host that returned
// one where the other belongs is visible in the failure message.
func completionRunningState(intent execution.TerminalIntent) execution.EntryCompletionState {
	return execution.EntryCompletionState{
		EntryStatus:            domainexecution.EntryRunning,
		TerminalIntent:         intent,
		TerminalIntentRevision: 7,
		CancellationGeneration: 3,
	}
}

func completionCommand(fixture CompletionFixture, state execution.EntryCompletionState, outcome execution.EngineOutcome, abortPendingCommandID string) execution.CompleteEntryCommand {
	return execution.CompleteEntryCommand{
		EntryID:               fixture.EntryID(),
		Fence:                 fixture.Fence(),
		State:                 state,
		Outcome:               outcome,
		AbortPendingCommandID: abortPendingCommandID,
	}
}

// completionSucceededOutcome is a run that finished, evidence and all.
func completionSucceededOutcome() execution.EngineOutcome {
	return execution.EngineOutcome{Result: engine.EntryResult{
		ExecutionOutcome: engine.OutcomeSucceeded,
		RecordingOutcome: engine.RecordingSucceeded,
		TimelineOutcome:  engine.TimelineComplete,
	}}
}

// completionFailedOutcome is a run that failed with degraded evidence, which is
// the shape most likely to expose a host that lets recording quality leak into
// the terminal status.
func completionFailedOutcome() execution.EngineOutcome {
	return execution.EngineOutcome{
		Result: engine.EntryResult{
			ExecutionOutcome: engine.OutcomeFailed,
			RecordingOutcome: engine.RecordingStopFailed,
			TimelineOutcome:  engine.TimelineComplete,
		},
		FailureCode: execution.CodeSchedulingAdapterUnavailable,
	}
}

// mustCompletionDecision is the answer the suite compares a host against. A
// command the suite itself built is decidable by construction, so a failure
// here is a defect in this file rather than in the adapter under test.
func mustCompletionDecision(t *testing.T, command execution.CompleteEntryCommand) execution.EntryCompletionDecision {
	t.Helper()
	decision, err := execution.DecideEntryCompletion(command.State, command.Outcome)
	if err != nil {
		t.Fatalf("DecideEntryCompletion() = %v, want nil", err)
	}
	return decision
}
