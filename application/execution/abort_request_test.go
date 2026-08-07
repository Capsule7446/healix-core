package execution

import (
	"reflect"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

func abortRunningState(intent TerminalIntent, revision, generation int64) EntryCompletionState {
	return EntryCompletionState{
		EntryStatus:            domainexecution.EntryRunning,
		TerminalIntent:         intent,
		TerminalIntentRevision: revision,
		CancellationGeneration: generation,
	}
}

// TestDecideAbortRequestAdvancesOnlyTheIntentRevision is the core matrix. Every
// legal starting intent has a determinate answer, and the answer is checked
// field by field rather than by spot-checking the intent: the host writes all
// six values verbatim, so a wrong counter is as damaging as a wrong intent.
func TestDecideAbortRequestAdvancesOnlyTheIntentRevision(t *testing.T) {
	for _, test := range []struct {
		name    string
		initial TerminalIntent
	}{
		{"from none", TerminalIntentNone},
		{"from cancel", TerminalIntentCancel},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := abortRunningState(test.initial, 7, 3)
			decision, err := DecideAbortRequest(state, AbortRequest{AbortPendingCommandID: "command-1"})
			if err != nil {
				t.Fatalf("DecideAbortRequest() = %v", err)
			}
			want := AbortRequestDecision{
				CurrentIntent:                 test.initial,
				CurrentIntentRevision:         7,
				CurrentCancellationGeneration: 3,
				NextIntent:                    TerminalIntentAbort,
				NextIntentRevision:            8,
				// Requesting an abort is not carrying one out. The generation is
				// spent by the completion that actually reaches ABORTED; advancing
				// it here too would spend it twice.
				NextCancellationGeneration: 3,
			}
			if !reflect.DeepEqual(decision, want) {
				t.Fatalf("decision = %+v, want %+v", decision, want)
			}
		})
	}
}

// TestDecideAbortRequestRefusesAnAbortAlreadyInFlight pins the ruling that a
// repeat request is a conflict rather than an idempotent hit. Advancing the
// revision on a no-op would move the compare-and-swap predicate out from under
// a completion that already read the old value, turning a duplicated user click
// into a spurious completion conflict.
func TestDecideAbortRequestRefusesAnAbortAlreadyInFlight(t *testing.T) {
	state := abortRunningState(TerminalIntentAbort, 4, 1)
	decision, err := DecideAbortRequest(state, AbortRequest{AbortPendingCommandID: "command-1"})
	if !fault.IsCode(err, CodeAbortRequestAlreadyAborting) {
		t.Fatalf("error = %v, want %s", err, CodeAbortRequestAlreadyAborting)
	}
	if !reflect.DeepEqual(decision, AbortRequestDecision{}) {
		t.Fatalf("refused request returned %+v, want the zero decision", decision)
	}
}

// TestDecideAbortRequestRefusesEveryNonRunningStatus covers the whole status
// vocabulary rather than one sample, so a status added later has no silent
// default.
func TestDecideAbortRequestRefusesEveryNonRunningStatus(t *testing.T) {
	for _, status := range []domainexecution.EntryStatus{
		domainexecution.EntryPending, domainexecution.EntrySucceeded, domainexecution.EntryFailed,
		domainexecution.EntryCanceled, domainexecution.EntryAborted, domainexecution.EntrySkipped,
	} {
		t.Run(string(status), func(t *testing.T) {
			state := abortRunningState(TerminalIntentNone, 1, 0)
			state.EntryStatus = status
			decision, err := DecideAbortRequest(state, AbortRequest{AbortPendingCommandID: "command-1"})
			if !fault.IsCode(err, CodeAbortRequestNotRunning) {
				t.Fatalf("error = %v, want %s", err, CodeAbortRequestNotRunning)
			}
			if !reflect.DeepEqual(decision, AbortRequestDecision{}) {
				t.Fatalf("refused request returned %+v, want the zero decision", decision)
			}
		})
	}
}

// TestDecideAbortRequestIgnoresTheCommandIdentity mirrors D-12's ruling two for
// the request side: the pending command id is idempotency identity, never a
// decision input. Two different identities over the same state must produce
// byte-identical decisions, or the host could reach a different terminal
// authority by retrying under a new command id.
func TestDecideAbortRequestIgnoresTheCommandIdentity(t *testing.T) {
	state := abortRunningState(TerminalIntentCancel, 11, 5)
	first, err := DecideAbortRequest(state, AbortRequest{AbortPendingCommandID: "command-a"})
	if err != nil {
		t.Fatalf("first DecideAbortRequest() = %v", err)
	}
	second, err := DecideAbortRequest(state, AbortRequest{AbortPendingCommandID: "command-b"})
	if err != nil {
		t.Fatalf("second DecideAbortRequest() = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("command identity changed the decision: %+v vs %+v", first, second)
	}
}

// TestDecideAbortRequestRejectsMalformedInput keeps the request side as strict
// as the completion side. A blank command id would reach the host's idempotency
// row as an untraceable key.
func TestDecideAbortRequestRejectsMalformedInput(t *testing.T) {
	for _, test := range []struct {
		name    string
		state   EntryCompletionState
		request AbortRequest
		code    fault.Code
	}{
		{"blank command id", abortRunningState(TerminalIntentNone, 1, 0), AbortRequest{AbortPendingCommandID: " \t "}, CodeAbortRequestInvalid},
		{"absent command id", abortRunningState(TerminalIntentNone, 1, 0), AbortRequest{}, CodeAbortRequestInvalid},
		{"unpadded command id", abortRunningState(TerminalIntentNone, 1, 0), AbortRequest{AbortPendingCommandID: " command-1"}, CodeAbortRequestInvalid},
		{"unknown intent", abortRunningState(TerminalIntent("SHRUG"), 1, 0), AbortRequest{AbortPendingCommandID: "command-1"}, CodeEntryCompletionStateInvalid},
		{"negative revision", abortRunningState(TerminalIntentNone, -1, 0), AbortRequest{AbortPendingCommandID: "command-1"}, CodeEntryCompletionStateInvalid},
		{"negative generation", abortRunningState(TerminalIntentNone, 1, -1), AbortRequest{AbortPendingCommandID: "command-1"}, CodeEntryCompletionStateInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision, err := DecideAbortRequest(test.state, test.request)
			if !fault.IsCode(err, test.code) {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			if !reflect.DeepEqual(decision, AbortRequestDecision{}) {
				t.Fatalf("rejected request returned %+v, want the zero decision", decision)
			}
		})
	}
}

// TestDecideAbortRequestRefusesAnExhaustedRevision reuses the completion
// ceiling: the request writes a successor revision, so it is subject to the
// same representability limit. The generation is untouched here, so a
// generation sitting at the ceiling must not block the request.
func TestDecideAbortRequestRefusesAnExhaustedRevision(t *testing.T) {
	exhausted := abortRunningState(TerminalIntentNone, MaxExpectedEntryCompletionRevision, 0)
	if _, err := DecideAbortRequest(exhausted, AbortRequest{AbortPendingCommandID: "command-1"}); !fault.IsCode(err, CodeEntryCompletionRevisionExhausted) {
		t.Fatalf("error = %v, want %s", err, CodeEntryCompletionRevisionExhausted)
	}

	// A generation at the ceiling has no successor to be exhausted, because the
	// request never advances it.
	spentGeneration := abortRunningState(TerminalIntentNone, 1, MaxExpectedEntryCompletionRevision)
	decision, err := DecideAbortRequest(spentGeneration, AbortRequest{AbortPendingCommandID: "command-1"})
	if err != nil {
		t.Fatalf("DecideAbortRequest() with a spent generation = %v", err)
	}
	if decision.NextCancellationGeneration != MaxExpectedEntryCompletionRevision {
		t.Fatalf("generation = %d, want it carried through unchanged", decision.NextCancellationGeneration)
	}
}

// TestDecideAbortRequestFeedsTheCompletionItPrecedes is the seam D-17 exists
// for: the request produces a pending intent, and the completion that follows
// turns it into the terminal status. Testing them apart would leave the join
// unverified, which is exactly where the host's two SQLite writes meet.
func TestDecideAbortRequestFeedsTheCompletionItPrecedes(t *testing.T) {
	state := abortRunningState(TerminalIntentNone, 2, 0)
	request, err := DecideAbortRequest(state, AbortRequest{AbortPendingCommandID: "command-1"})
	if err != nil {
		t.Fatalf("DecideAbortRequest() = %v", err)
	}

	// The host writes the request decision, then re-reads before completing.
	afterRequest := EntryCompletionState{
		EntryStatus:            domainexecution.EntryRunning,
		TerminalIntent:         request.NextIntent,
		TerminalIntentRevision: request.NextIntentRevision,
		CancellationGeneration: request.NextCancellationGeneration,
	}
	completion, err := DecideEntryCompletion(afterRequest, NotStartedEngineOutcome())
	if err != nil {
		t.Fatalf("DecideEntryCompletion() = %v", err)
	}
	if completion.EntryStatus != domainexecution.EntryAborted {
		t.Fatalf("terminal status = %q, want %q", completion.EntryStatus, domainexecution.EntryAborted)
	}
	// The generation the request declined to spend is spent here, exactly once.
	if completion.NextCancellationGeneration != state.CancellationGeneration+1 {
		t.Fatalf("generation = %d, want exactly one advance across request + completion", completion.NextCancellationGeneration)
	}
}
