package conformancetest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/application/execution"
	"github.com/Capsule7446/healix-core/application/execution/conformancetest"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

// abortReferenceFixture is a correct in-memory AbortRequestTransaction.
//
// It exists so the suite is proven against an implementation known to satisfy
// the contract. A conformance suite that has never gone green is indistinguishable
// from one that cannot: the first host to run it could not tell a defect in its
// adapter from a defect in the suite.
//
// It stages every write into locals and commits them in one assignment at the
// end, which is how an in-memory fixture models "one transaction or none".
type abortReferenceFixture struct {
	entryID domainexecution.EntryID
	fence   domainexecution.WorkerFence

	entryStatus            domainexecution.EntryStatus
	terminalIntent         execution.TerminalIntent
	terminalIntentRevision int64
	cancellationGeneration int64
	pendingIntents         int
	commandReceipts        int
	idempotencyReceipts    int

	receipts map[string]execution.RequestAbortOutcome
	fault    conformancetest.AbortFaultPoint
}

func newAbortReferenceFixture(_ *testing.T, state execution.EntryCompletionState) conformancetest.AbortFixture {
	return &abortReferenceFixture{
		entryID:                mustEntryID("entry-1"),
		fence:                  domainexecution.WorkerFence{InstanceID: mustInstanceID("run-1"), ClaimToken: "claim-1"},
		entryStatus:            state.EntryStatus,
		terminalIntent:         state.TerminalIntent,
		terminalIntentRevision: state.TerminalIntentRevision,
		cancellationGeneration: state.CancellationGeneration,
		receipts:               map[string]execution.RequestAbortOutcome{},
	}
}

func (f *abortReferenceFixture) Fence() domainexecution.WorkerFence { return f.fence }
func (f *abortReferenceFixture) EntryID() domainexecution.EntryID   { return f.entryID }

func (f *abortReferenceFixture) Snapshot() conformancetest.AbortSnapshot {
	return conformancetest.AbortSnapshot{
		EntryStatus:            f.entryStatus,
		TerminalIntent:         f.terminalIntent,
		TerminalIntentRevision: f.terminalIntentRevision,
		CancellationGeneration: f.cancellationGeneration,
		PendingIntents:         f.pendingIntents,
		CommandReceipts:        f.commandReceipts,
		IdempotencyReceipts:    f.idempotencyReceipts,
	}
}

func (f *abortReferenceFixture) SetFault(point conformancetest.AbortFaultPoint) { f.fault = point }
func (f *abortReferenceFixture) ClearFault()                                    { f.fault = "" }

func (f *abortReferenceFixture) failAt(point conformancetest.AbortFaultPoint) error {
	if f.fault == point {
		return errors.New("injected fault at " + string(point))
	}
	return nil
}

func (f *abortReferenceFixture) LookupAbortRequest(_ context.Context, entryID domainexecution.EntryID, digest string) (execution.RequestAbortOutcome, bool, error) {
	if err := f.failAt(conformancetest.AbortFaultBeforeReplay); err != nil {
		return execution.RequestAbortOutcome{}, false, err
	}
	if entryID != f.entryID {
		return execution.RequestAbortOutcome{}, false, execution.RequestAbortIdentityConflictError()
	}
	recorded, found := f.receipts[digest]
	return recorded, found, nil
}

func (f *abortReferenceFixture) RequestAbort(_ context.Context, intent execution.RequestAbortIntent) (execution.RequestAbortOutcome, error) {
	// The contract requires this check first, before storage is touched.
	if err := execution.ValidateRequestAbortIntentDigest(intent); err != nil {
		return execution.RequestAbortOutcome{}, err
	}
	if err := f.failAt(conformancetest.AbortFaultAfterDecision); err != nil {
		return execution.RequestAbortOutcome{}, err
	}
	if intent.Command.Fence != f.fence {
		return execution.RequestAbortOutcome{}, domainexecution.NewStaleWorkerFenceError()
	}
	if recorded, found := f.receipts[intent.RequestDigest]; found {
		return recorded, nil
	}
	// Compare-and-swap on the exact predicate core supplied.
	if f.terminalIntent != intent.Decision.CurrentIntent ||
		f.terminalIntentRevision != intent.Decision.CurrentIntentRevision ||
		f.cancellationGeneration != intent.Decision.CurrentCancellationGeneration {
		return execution.RequestAbortOutcome{}, execution.RequestAbortIdentityConflictError()
	}

	intents, receipts := f.pendingIntents+1, f.commandReceipts
	if err := f.failAt(conformancetest.AbortFaultAfterIntent); err != nil {
		return execution.RequestAbortOutcome{}, err
	}
	receipts++
	if err := f.failAt(conformancetest.AbortFaultAfterReceipt); err != nil {
		return execution.RequestAbortOutcome{}, err
	}

	outcome := execution.RequestAbortOutcome{
		Status:        execution.RequestAbortApplied,
		EntryID:       intent.EntryID,
		RequestDigest: intent.RequestDigest,
		Decision:      intent.Decision,
	}
	// Commit: the counters are written verbatim from the decision, the entry's
	// own status is deliberately untouched, and the idempotency receipt lands
	// last so a crash before it leaves the batch invisible.
	f.terminalIntent = intent.Decision.NextIntent
	f.terminalIntentRevision = intent.Decision.NextIntentRevision
	f.cancellationGeneration = intent.Decision.NextCancellationGeneration
	f.pendingIntents, f.commandReceipts = intents, receipts
	f.idempotencyReceipts++
	replay := outcome
	replay.Status = execution.RequestAbortReplayed
	f.receipts[intent.RequestDigest] = replay
	return outcome, nil
}

// TestAbortRequestReferenceImplementationPassesTheSuite proves the suite is
// satisfiable, and doubles as the regression that keeps it satisfiable when the
// contract next changes.
func TestAbortRequestReferenceImplementationPassesTheSuite(t *testing.T) {
	conformancetest.RunAbortRequest(t, newAbortReferenceFixture)
}
