package execution

import (
	"testing"

	"github.com/Capsule7446/healix-core/application/engine"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// TestInterruptedOutcomeIsDistinctFromNotStarted is the D-18 gap. Before this,
// a host recovering an orphan RUNNING entry after a crash had only
// NotStartedEngineOutcome to offer, which is not merely lossy — it is a false
// statement. "Not started" means the engine is known not to have begun; an
// orphan means the engine may well have run and nobody survived to observe it.
// Both landed in SQLite as FAILED, and no field could tell them apart later.
func TestInterruptedOutcomeIsDistinctFromNotStarted(t *testing.T) {
	interrupted := InterruptedEngineOutcome()
	if err := interrupted.Validate(); err != nil {
		t.Fatalf("InterruptedEngineOutcome() is not valid: %v", err)
	}
	if interrupted.Result.ExecutionOutcome != engine.ExecutionInterrupted {
		t.Fatalf("execution outcome = %q, want %q", interrupted.Result.ExecutionOutcome, engine.ExecutionInterrupted)
	}
	if interrupted == NotStartedEngineOutcome() {
		t.Fatal("an interrupted run must not be spelled the same as one that never started")
	}
}

// TestTerminalCauseSeparatesObservationFromResult is the answer to D-18 §7.2:
// core takes option (b). The seven-state entry machine is unchanged — FAILED
// stays the honest terminal status for an entry that neither succeeded nor was
// asked to stop — and the distinction the host needs rides alongside it on the
// decision the host already persists verbatim.
func TestTerminalCauseSeparatesObservationFromResult(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome EngineOutcome
		status  domainexecution.EntryStatus
		cause   TerminalCause
	}{
		{"engine succeeded", completedOutcome(engine.OutcomeSucceeded), domainexecution.EntrySucceeded, TerminalCauseCompleted},
		{"engine failed", completedOutcome(engine.OutcomeFailed), domainexecution.EntryFailed, TerminalCauseCompleted},
		{"engine canceled", completedOutcome(engine.OutcomeCanceled), domainexecution.EntryFailed, TerminalCauseCompleted},
		{"never started", NotStartedEngineOutcome(), domainexecution.EntryFailed, TerminalCauseNotStarted},
		{"never observed", InterruptedEngineOutcome(), domainexecution.EntryFailed, TerminalCauseInterrupted},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := EntryCompletionState{
				EntryStatus:            domainexecution.EntryRunning,
				TerminalIntent:         TerminalIntentNone,
				TerminalIntentRevision: 1,
			}
			decision, err := DecideEntryCompletion(state, test.outcome)
			if err != nil {
				t.Fatalf("DecideEntryCompletion() = %v", err)
			}
			if decision.EntryStatus != test.status {
				t.Fatalf("entry status = %q, want %q", decision.EntryStatus, test.status)
			}
			if decision.TerminalCause != test.cause {
				t.Fatalf("terminal cause = %q, want %q", decision.TerminalCause, test.cause)
			}
		})
	}
}

// TestTerminalCauseIsIndependentOfTheIntent keeps the two axes separate. The
// intent decides which terminal status an unfinished entry reaches; the cause
// reports how much of the run was observed. Collapsing them would put the host
// back where D-18 found it — unable to tell an aborted-but-observed entry from
// an aborted-because-we-crashed one.
func TestTerminalCauseIsIndependentOfTheIntent(t *testing.T) {
	for _, intent := range []TerminalIntent{TerminalIntentNone, TerminalIntentCancel, TerminalIntentAbort} {
		for _, test := range []struct {
			outcome EngineOutcome
			cause   TerminalCause
		}{
			{NotStartedEngineOutcome(), TerminalCauseNotStarted},
			{InterruptedEngineOutcome(), TerminalCauseInterrupted},
			{completedOutcome(engine.OutcomeFailed), TerminalCauseCompleted},
		} {
			t.Run(string(intent)+"/"+string(test.cause), func(t *testing.T) {
				state := EntryCompletionState{
					EntryStatus:            domainexecution.EntryRunning,
					TerminalIntent:         intent,
					TerminalIntentRevision: 1,
				}
				decision, err := DecideEntryCompletion(state, test.outcome)
				if err != nil {
					t.Fatalf("DecideEntryCompletion() = %v", err)
				}
				if decision.TerminalCause != test.cause {
					t.Fatalf("intent %q changed the cause to %q, want %q", intent, decision.TerminalCause, test.cause)
				}
			})
		}
	}
}

// TestSucceededRunIsAlwaysReportedAsCompleted guards the one cell where the two
// axes could be made to disagree: 裁决一 lets a finished engine outrank a cancel
// intent, and the cause must say the run was observed rather than inheriting
// anything from the intent that lost.
func TestSucceededRunIsAlwaysReportedAsCompleted(t *testing.T) {
	for _, intent := range []TerminalIntent{TerminalIntentNone, TerminalIntentCancel, TerminalIntentAbort} {
		state := EntryCompletionState{
			EntryStatus:            domainexecution.EntryRunning,
			TerminalIntent:         intent,
			TerminalIntentRevision: 1,
		}
		decision, err := DecideEntryCompletion(state, completedOutcome(engine.OutcomeSucceeded))
		if err != nil {
			t.Fatalf("DecideEntryCompletion() = %v", err)
		}
		if decision.EntryStatus != domainexecution.EntrySucceeded || decision.TerminalCause != TerminalCauseCompleted {
			t.Fatalf("intent %q: status=%q cause=%q", intent, decision.EntryStatus, decision.TerminalCause)
		}
	}
}

// TestEngineOutcomeStillRejectsWhatTheEngineCannotReport keeps the new constant
// from widening the gate: only the vocabulary application/engine actually
// produces is accepted.
func TestEngineOutcomeStillRejectsWhatTheEngineCannotReport(t *testing.T) {
	outcome := InterruptedEngineOutcome()
	outcome.Result.ExecutionOutcome = engine.ExecutionOutcome("SHRUG")
	if err := outcome.Validate(); !fault.IsCode(err, CodeEngineOutcomeInvalid) {
		t.Fatalf("error = %v, want %s", err, CodeEngineOutcomeInvalid)
	}
}

func completedOutcome(result engine.ExecutionOutcome) EngineOutcome {
	return EngineOutcome{Result: engine.EntryResult{
		ExecutionOutcome: result,
		RecordingOutcome: engine.RecordingDisabled,
		TimelineOutcome:  engine.TimelineDisabled,
	}}
}
