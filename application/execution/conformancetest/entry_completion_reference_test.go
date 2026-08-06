package conformancetest_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	execution "github.com/Capsule7446/healix-core/application/execution"
	"github.com/Capsule7446/healix-core/application/execution/conformancetest"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

// completionReferenceState is everything one entry completion is allowed to
// touch. The suite reads it back through Snapshot, so a write that escapes the
// staged copy below is immediately visible as a fault-point regression.
type completionReferenceState struct {
	entryStatus            domainexecution.EntryStatus
	terminalIntent         execution.TerminalIntent
	terminalIntentRevision int64
	cancellationGeneration int64
	completions            int
	terminalFacts          int
	evidenceRefs           int
	gateTerminalizations   int
	outboxRecords          int
	receipts               map[string]execution.CompleteEntryOutcome
}

type completionReferenceFixture struct {
	mu      sync.Mutex
	fence   domainexecution.WorkerFence
	entryID domainexecution.EntryID
	state   completionReferenceState
	fault   conformancetest.CompletionFaultPoint
}

func newCompletionReferenceFixture(_ *testing.T, state execution.EntryCompletionState) conformancetest.CompletionFixture {
	return &completionReferenceFixture{
		fence:   domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim"},
		entryID: mustEntryID("execution"),
		state: completionReferenceState{
			entryStatus:            state.EntryStatus,
			terminalIntent:         state.TerminalIntent,
			terminalIntentRevision: state.TerminalIntentRevision,
			cancellationGeneration: state.CancellationGeneration,
			receipts:               map[string]execution.CompleteEntryOutcome{},
		},
	}
}

func cloneCompletionState(source completionReferenceState) completionReferenceState {
	clone := source
	clone.receipts = make(map[string]execution.CompleteEntryOutcome, len(source.receipts))
	for key, value := range source.receipts {
		clone.receipts[key] = value
	}
	return clone
}

func completionReceiptKey(entryID domainexecution.EntryID, digest string) string {
	return entryID.String() + "|" + digest
}

func completionFaultError(point conformancetest.CompletionFaultPoint) error {
	return fmt.Errorf("injected reference fault at %s", point)
}

func (f *completionReferenceFixture) Fence() domainexecution.WorkerFence { return f.fence }

func (f *completionReferenceFixture) EntryID() domainexecution.EntryID { return f.entryID }

func (f *completionReferenceFixture) Snapshot() conformancetest.CompletionSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return conformancetest.CompletionSnapshot{
		EntryStatus:            f.state.entryStatus,
		TerminalIntent:         f.state.terminalIntent,
		TerminalIntentRevision: f.state.terminalIntentRevision,
		CancellationGeneration: f.state.cancellationGeneration,
		Completions:            f.state.completions,
		TerminalFacts:          f.state.terminalFacts,
		EvidenceRefs:           f.state.evidenceRefs,
		GateTerminalizations:   f.state.gateTerminalizations,
		OutboxRecords:          f.state.outboxRecords,
	}
}

func (f *completionReferenceFixture) SetFault(point conformancetest.CompletionFaultPoint) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fault = point
}

func (f *completionReferenceFixture) ClearFault() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fault = ""
}

func (f *completionReferenceFixture) observedState() execution.EntryCompletionState {
	return execution.EntryCompletionState{
		EntryStatus:            f.state.entryStatus,
		TerminalIntent:         f.state.terminalIntent,
		TerminalIntentRevision: f.state.terminalIntentRevision,
		CancellationGeneration: f.state.cancellationGeneration,
	}
}

func (f *completionReferenceFixture) LookupEntryCompletion(_ context.Context, entryID domainexecution.EntryID, digest string) (execution.CompleteEntryOutcome, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Armed before the read, not after: a host that crashes here has not yet
	// told anyone the request was already applied.
	if f.fault == conformancetest.CompletionFaultBeforeReplay {
		return execution.CompleteEntryOutcome{}, false, completionFaultError(f.fault)
	}
	record, ok := f.state.receipts[completionReceiptKey(entryID, digest)]
	if !ok {
		return execution.CompleteEntryOutcome{}, false, nil
	}
	record.Status = execution.CompleteEntryReplayed
	return record, true, nil
}

func (f *completionReferenceFixture) CompleteEntry(_ context.Context, intent execution.CompleteEntryIntent) (execution.CompleteEntryOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := execution.ValidateCompleteEntryIntentDigest(intent); err != nil {
		return execution.CompleteEntryOutcome{}, err
	}
	if intent.Command.Fence != f.fence {
		return execution.CompleteEntryOutcome{}, domainexecution.NewStaleWorkerFenceError()
	}
	key := completionReceiptKey(intent.EntryID, intent.RequestDigest)
	if recorded, ok := f.state.receipts[key]; ok {
		recorded.Status = execution.CompleteEntryReplayed
		return recorded, nil
	}
	if intent.Command.State != f.observedState() {
		return execution.CompleteEntryOutcome{}, execution.CompleteEntryIdentityConflictError()
	}

	staged := cloneCompletionState(f.state)
	outcome := execution.CompleteEntryOutcome{
		Status:        execution.CompleteEntryApplied,
		EntryID:       intent.EntryID,
		RequestDigest: intent.RequestDigest,
		Decision:      intent.Decision,
	}
	// The write order the contract declares atomic. Every stage lands in the
	// staged copy, so an injected fault discards the whole batch.
	stages := []struct {
		point conformancetest.CompletionFaultPoint
		apply func()
	}{
		{conformancetest.CompletionFaultAfterDecision, func() {}},
		{conformancetest.CompletionFaultAfterEntry, func() {
			staged.entryStatus = intent.Decision.EntryStatus
			staged.terminalIntent = intent.Decision.NextIntent
			staged.terminalIntentRevision = intent.Decision.NextIntentRevision
			staged.cancellationGeneration = intent.Decision.NextCancellationGeneration
			staged.completions++
		}},
		{conformancetest.CompletionFaultAfterFacts, func() { staged.terminalFacts++ }},
		{conformancetest.CompletionFaultAfterEvidence, func() { staged.evidenceRefs++ }},
		{conformancetest.CompletionFaultAfterGate, func() { staged.gateTerminalizations++ }},
		{conformancetest.CompletionFaultAfterOutbox, func() { staged.outboxRecords++ }},
	}
	for _, stage := range stages {
		stage.apply()
		if f.fault == stage.point {
			return execution.CompleteEntryOutcome{}, completionFaultError(stage.point)
		}
	}
	staged.receipts[key] = outcome
	f.state = staged
	return outcome, nil
}

func TestReferenceEntryCompletionTransactionConformance(t *testing.T) {
	conformancetest.RunEntryCompletion(t, newCompletionReferenceFixture)
}

func TestCompletionReferenceStateCloneIsIndependent(t *testing.T) {
	source := completionReferenceState{receipts: map[string]execution.CompleteEntryOutcome{
		"entry|sha256:one": {Status: execution.CompleteEntryApplied},
	}}
	clone := cloneCompletionState(source)
	clone.receipts["entry|sha256:two"] = execution.CompleteEntryOutcome{Status: execution.CompleteEntryApplied}
	if len(source.receipts) != 1 {
		t.Fatalf("clone shares the receipt map, source now has %d entries", len(source.receipts))
	}
	if reflect.DeepEqual(source.receipts, clone.receipts) {
		t.Fatal("clone and source receipts must be separate maps")
	}
}
