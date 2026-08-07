package execution

import (
	"strings"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

func abortRequestInvalidError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeAbortRequestInvalid, "abort request is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func abortRequestNotRunningError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeAbortRequestNotRunning, "entry is not running and cannot be asked to abort")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func abortRequestAlreadyAbortingError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeAbortRequestAlreadyAborting, "entry is already aborting")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// AbortRequest is the identity of one request to stop a running entry.
//
// It carries identity only. AbortPendingCommandID names the host command so a
// replay can be recognised and an audit row can be traced, and
// DecideAbortRequest is required to produce the same decision whichever
// identity it is given — the same separation D-12 draws by keeping the field
// off EntryCompletionState. The type exists as a parameter rather than being
// folded away because the decision's inputs are a contract: a later request
// attribute belongs here, where the host already passes a value, rather than in
// a second parameter added to every call site.
type AbortRequest struct {
	AbortPendingCommandID string
}

// Validate reports whether the request identity is one the host can persist and
// match later. Surrounding space is rejected rather than trimmed: the host
// stores this value and compares it verbatim, so silently changing it here
// would make core and host disagree about the key.
func (request AbortRequest) Validate() error {
	if request.AbortPendingCommandID == "" || strings.TrimSpace(request.AbortPendingCommandID) == "" {
		return abortRequestInvalidError(mustEntryCompletionViolation(fault.CodeFieldRequired, "abortPendingCommandId", "abort pending command id is required"))
	}
	if request.AbortPendingCommandID != strings.TrimSpace(request.AbortPendingCommandID) {
		return abortRequestInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "abortPendingCommandId", "abort pending command id must be normalized"))
	}
	return nil
}

// AbortRequestDecision is the complete answer for one abort request: an exact
// compare-and-swap predicate and the exact values to write.
//
// The Current* fields repeat what core was told it observed, so the host can
// match the pending-intent row without re-reading. The Next* fields are written
// verbatim; the host must never increment or infer either counter, which is the
// same rule ExecutionActionGateV1 relies on to keep one execution authority.
//
// There is no entry status here, and that absence is the point. Requesting an
// abort records an intent; it does not end anything. The terminal status stays
// the sole output of DecideEntryCompletion, so abort and ordinary completion
// converge on one terminal write path instead of two.
type AbortRequestDecision struct {
	CurrentIntent                 TerminalIntent
	CurrentIntentRevision         int64
	CurrentCancellationGeneration int64
	NextIntent                    TerminalIntent
	NextIntentRevision            int64
	NextCancellationGeneration    int64
}

// DecideAbortRequest answers what the terminal-intent counters become when
// someone asks a running entry to stop.
//
// It is a pure function of state and request: no Context, no port, no clock. A
// host may call it as a pre-check and get the answer the commit will use.
//
// Every combination has a determinate result or an explicit fault:
//
//   - A non-RUNNING entry is refused with CodeAbortRequestNotRunning. An entry
//     that already reached a terminal status has nothing left to stop.
//   - NONE and CANCEL both advance to ABORT. Abort is strictly stronger than
//     cancel — cancel stops the entries that have not begun, abort ends the one
//     in flight — so escalation is allowed rather than treated as a conflict.
//   - ABORT is refused with CodeAbortRequestAlreadyAborting. Nothing is left to
//     advance, and writing a no-op revision would be actively harmful: it moves
//     the compare-and-swap predicate out from under a completion that already
//     read the previous value, turning a duplicated click into a spurious
//     completion conflict.
//
// NextIntentRevision advances by exactly one, so the request writes a value
// distinguishable from every other write and a replay can be told from a fresh
// request.
//
// NextCancellationGeneration is carried through unchanged. D-12 spends a
// generation only when an intent is actually carried out — when a completion
// reaches CANCELED or ABORTED — and a request is not a carrying-out. Advancing
// it here as well would spend one generation twice for a single abort, and the
// scheduler reads generations to decide whether an instance may still advance.
// A generation already at MaxExpectedEntryCompletionRevision therefore does not
// block a request: it has no successor to be exhausted.
//
// Ordering: the decision this produces is written first, then the entry runs to
// its natural stopping point, then EntryCompletionTransaction.CompleteEntry
// terminates it and finalises the host's action gate in one transaction. This
// function must not be used to terminate an entry or to invalidate a claim.
func DecideAbortRequest(state EntryCompletionState, request AbortRequest) (AbortRequestDecision, error) {
	if err := state.Validate(); err != nil {
		return AbortRequestDecision{}, err
	}
	if err := request.Validate(); err != nil {
		return AbortRequestDecision{}, err
	}
	if state.EntryStatus != domainexecution.EntryRunning {
		return AbortRequestDecision{}, abortRequestNotRunningError()
	}
	if state.TerminalIntent == TerminalIntentAbort {
		return AbortRequestDecision{}, abortRequestAlreadyAbortingError()
	}
	// The request always writes a successor revision, so that counter is always
	// the one that has to be representable. The generation is not advanced here
	// and so cannot be exhausted by a request.
	if state.TerminalIntentRevision >= MaxExpectedEntryCompletionRevision {
		return AbortRequestDecision{}, entryCompletionRevisionExhaustedError()
	}
	return AbortRequestDecision{
		CurrentIntent:                 state.TerminalIntent,
		CurrentIntentRevision:         state.TerminalIntentRevision,
		CurrentCancellationGeneration: state.CancellationGeneration,
		NextIntent:                    TerminalIntentAbort,
		NextIntentRevision:            state.TerminalIntentRevision + 1,
		NextCancellationGeneration:    state.CancellationGeneration,
	}, nil
}
