package execution

import (
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/application/engine"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const (
	// CodeEntryCompletionStateInvalid covers a malformed observed state: an
	// unknown entry status or terminal intent, or a negative revision. The caller
	// read something the core vocabulary does not contain, so the remediation is
	// to repair the read, hence INVALID_ARGUMENT.
	CodeEntryCompletionStateInvalid fault.Code = "EXECUTION_ENTRY_COMPLETION_STATE_INVALID"
	// CodeEntryCompletionRevisionExhausted covers a state whose successor
	// revision core would refuse to write. Nothing about the argument is
	// malformed; the counter simply has no room left, hence OUT_OF_RANGE.
	CodeEntryCompletionRevisionExhausted fault.Code = "EXECUTION_ENTRY_COMPLETION_REVISION_EXHAUSTED"
	// CodeEntryCompletionNotRunning covers a well-formed state that is not
	// RUNNING. Only a running entry can be completed, and the caller must
	// re-read the entry before retrying, hence FAILED_PRECONDITION.
	CodeEntryCompletionNotRunning fault.Code = "EXECUTION_ENTRY_COMPLETION_NOT_RUNNING"
	// CodeEngineOutcomeInvalid covers an engine outcome outside the engine
	// vocabulary, or a failure code that is blank without being absent.
	CodeEngineOutcomeInvalid fault.Code = "EXECUTION_ENGINE_OUTCOME_INVALID"
)

// MaxExpectedEntryCompletionRevision is the largest value core will ever write
// into NextIntentRevision or NextCancellationGeneration.
//
// A counter already at or above it has no successor core may produce: the next
// value would be math.MaxInt64, and one further completion would wrap to
// MinInt64 — a value no adapter can compare-and-swap against, which silently
// turns the host's optimistic-concurrency check into no check at all. The
// completion is refused instead.
//
// The ceiling is checked per counter, and only for a counter the decision
// actually advances. A cancellation generation carried through unchanged has no
// successor to be exhausted, so a state sitting at the ceiling is still
// completable whenever the intent was not carried out.
const MaxExpectedEntryCompletionRevision int64 = math.MaxInt64 - 1

// mustEntryCompletionViolation builds a field-level reason. Construction can
// only fail on a malformed field name or code, both of which are compile-time
// constants here, so a failure is a programming error rather than a business
// one.
func mustEntryCompletionViolation(code fault.Code, field, message string) fault.Violation {
	violation, err := fault.NewViolation(code, field, message)
	if err != nil {
		panic(err)
	}
	return violation
}

func entryCompletionStateInvalidError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeEntryCompletionStateInvalid, "entry completion state is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func entryCompletionRevisionExhaustedError() error {
	err, constructionErr := fault.New(fault.OutOfRange, CodeEntryCompletionRevisionExhausted, "entry completion revision has no representable successor")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func entryCompletionNotRunningError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeEntryCompletionNotRunning, "entry is not running and cannot be completed")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func engineOutcomeInvalidError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeEngineOutcomeInvalid, "engine outcome is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// TerminalIntent is the core-owned vocabulary for "someone has asked this
// instance to stop". The host persists and compares these values but never
// invents one: ExecutionActionGateV1 keeps a single execution authority
// precisely by leaving the legal values here.
//
// TerminalIntentNone is a real value, not a zero placeholder — an entry that
// nobody asked to stop still has an intent to record, and a blank intent is
// rejected rather than read as "none".
type TerminalIntent string

const (
	TerminalIntentNone   TerminalIntent = "NONE"
	TerminalIntentCancel TerminalIntent = "CANCEL"
	TerminalIntentAbort  TerminalIntent = "ABORT"
)

// Validate reports whether the intent is one core defined. Hosts call it when
// they read an intent back out of storage, before feeding it to a decision.
func (intent TerminalIntent) Validate() error {
	switch intent {
	case TerminalIntentNone, TerminalIntentCancel, TerminalIntentAbort:
		return nil
	default:
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "terminalIntent", "terminal intent is not one core defined"))
	}
}

// EngineOutcome is what one entry's engine run observed, as a domain value.
//
// Result is engine.RunProgram's own report, carried through unchanged rather
// than re-encoded: a second parallel enum would be a second place for the two
// vocabularies to drift. FailureCode is the classified fault code of whatever
// error accompanied the run, empty when there was none; it is recorded for the
// audit trail and deliberately never steers the terminal status.
type EngineOutcome struct {
	Result      engine.EntryResult
	FailureCode fault.Code
}

// NotStartedEngineOutcome is the outcome of an entry whose engine never ran —
// a stale fence, a refused authorization, a browser that could not be created.
// It is a valid, decidable outcome rather than an absence, so every failed
// entry still has a terminal state to commit and a lease to release.
func NotStartedEngineOutcome() EngineOutcome {
	return EngineOutcome{Result: engine.EntryResult{
		ExecutionOutcome: engine.ExecutionNotStarted,
		RecordingOutcome: engine.RecordingDisabled,
		TimelineOutcome:  engine.TimelineDisabled,
	}}
}

// InterruptedEngineOutcome is the outcome of an entry whose run was never
// observed to completion — the host process died while the engine was running,
// and recovery found the entry still RUNNING with a claim nobody holds.
//
// It exists because NotStartedEngineOutcome was the only construction recovery
// could reach, and using it there is not merely lossy but false: "not started"
// asserts the engine is known not to have begun, while an orphan may well have
// run to completion and taken its result down with the process. Both once
// terminated as FAILED with nothing to tell them apart afterwards, which for a
// product sold on its evidence chain makes failure-rate statistics and
// heal-candidate selection read a crash as a business failure.
func InterruptedEngineOutcome() EngineOutcome {
	return EngineOutcome{Result: engine.EntryResult{
		ExecutionOutcome: engine.ExecutionInterrupted,
		RecordingOutcome: engine.RecordingDisabled,
		TimelineOutcome:  engine.TimelineDisabled,
	}}
}

// Validate reports whether every field is inside the engine vocabulary.
func (outcome EngineOutcome) Validate() error {
	switch outcome.Result.ExecutionOutcome {
	case engine.OutcomeSucceeded, engine.OutcomeFailed, engine.OutcomeCanceled, engine.ExecutionNotStarted, engine.ExecutionInterrupted:
	default:
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "result.executionOutcome", "execution outcome is not one the engine reports"))
	}
	switch outcome.Result.RecordingOutcome {
	case engine.RecordingDisabled, engine.RecordingSucceeded, engine.RecordingStartFailed, engine.RecordingStopFailed:
	default:
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "result.recordingOutcome", "recording outcome is not one the engine reports"))
	}
	switch outcome.Result.TimelineOutcome {
	case engine.TimelineDisabled, engine.TimelineComplete, engine.TimelineStartFailed, engine.TimelineFinishFailed:
	default:
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "result.timelineOutcome", "timeline outcome is not one the engine reports"))
	}
	// A code that is present but blank is a lost classification, not "no
	// failure": it would be recorded as an empty audit field nobody can trace.
	if outcome.FailureCode != "" && strings.TrimSpace(string(outcome.FailureCode)) == "" {
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "failureCode", "failure code is present but blank"))
	}
	return nil
}

// EntryCompletionState is what the host observed about one entry immediately
// before completing it. It is the whole decision basis: anything absent here is
// deliberately absent.
//
// AbortPendingCommandID is one such deliberate absence. A pending abort command
// is idempotency identity, not a terminal intent, and keeping it out of this
// struct makes "mistake a pending abort for an effective intent" structurally
// impossible rather than merely discouraged. It travels on
// CompleteEntryCommand, where it belongs.
type EntryCompletionState struct {
	EntryStatus            domainexecution.EntryStatus
	TerminalIntent         TerminalIntent
	TerminalIntentRevision int64
	CancellationGeneration int64
}

// Validate reports whether the observed state is inside the core vocabulary. It
// deliberately says nothing about whether the entry can be completed — a
// well-formed PENDING state is valid and undecidable, and those are different
// answers with different remediations.
func (state EntryCompletionState) Validate() error {
	switch state.EntryStatus {
	case domainexecution.EntryPending, domainexecution.EntryRunning, domainexecution.EntrySucceeded,
		domainexecution.EntryFailed, domainexecution.EntryCanceled, domainexecution.EntryAborted,
		domainexecution.EntrySkipped:
	default:
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "entryStatus", "entry status is not one core defined"))
	}
	if err := state.TerminalIntent.Validate(); err != nil {
		return err
	}
	if state.TerminalIntentRevision < 0 {
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "terminalIntentRevision", "terminal intent revision must not be negative"))
	}
	if state.CancellationGeneration < 0 {
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "cancellationGeneration", "cancellation generation must not be negative"))
	}
	return nil
}

// EntryCompletionDecision is the complete terminal answer for one entry.
//
// The Current* fields repeat what core was told it observed, so the host has an
// exact compare-and-swap predicate without re-reading. The Next* fields are the
// exact values the host must write; the host must never increment or infer
// either counter itself, which is what
// ValidateCompleteEntryIntentDigest mechanically enforces.
// TerminalCause reports how much of a run was observed before its entry
// reached a terminal status. It is the second of two independent axes, and
// keeping them apart is the whole of D-18: EntryStatus answers "what did this
// entry come to", which the terminal intent can decide, while TerminalCause
// answers "did anyone see it happen", which no intent can change.
//
// Without it, an entry that ran and failed its assertions, an entry whose
// browser could not be created, and an entry whose observer crashed all landed
// as FAILED with no field able to separate them once persisted.
type TerminalCause string

const (
	// TerminalCauseCompleted means the engine ran and reported a result. The
	// result may be a failure; what makes it completed is that it was observed.
	TerminalCauseCompleted TerminalCause = "COMPLETED"
	// TerminalCauseNotStarted means the engine is known never to have begun — a
	// stale fence, a refused authorization, a browser that could not be created.
	TerminalCauseNotStarted TerminalCause = "NOT_STARTED"
	// TerminalCauseInterrupted means the run was never observed to completion.
	// Recovery terminating an orphan RUNNING entry produces this; whether the
	// engine actually finished is unknown and unknowable.
	TerminalCauseInterrupted TerminalCause = "INTERRUPTED"
)

type EntryCompletionDecision struct {
	EntryStatus domainexecution.EntryStatus
	// TerminalCause travels with the status rather than being left for the host
	// to derive from the command, because the host persists this struct verbatim
	// as the authoritative terminal record. A field it had to recompute is a
	// field two hosts could compute differently.
	TerminalCause                 TerminalCause
	CurrentIntent                 TerminalIntent
	CurrentIntentRevision         int64
	CurrentCancellationGeneration int64
	NextIntent                    TerminalIntent
	NextIntentRevision            int64
	NextCancellationGeneration    int64
}

// DecideEntryCompletion answers what terminal state one entry reached, and what
// the instance's terminal-intent counters become.
//
// It is a pure function of state and outcome: the same pair always yields the
// same decision, so a host may call it standalone as a pre-check and get the
// same answer the commit will use. It touches no port, takes no Context, and
// owns no resources.
//
// Every combination of engine outcome, terminal intent and starting status has
// a determinate result or an explicit fault — there is no combination the host
// is left to decide:
//
//   - A non-RUNNING entry is refused with CodeEntryCompletionNotRunning.
//   - 裁决一：引擎跑完时事实压过意图。An engine that reached SUCCEEDED means the
//     external side effects already landed — a form was submitted, an order was
//     placed — and a cancel cannot roll those back. Recording CANCELED would
//     contradict the evidence chain the same run produced. The entry is
//     SUCCEEDED, and the intent still travels intact into the Next* fields so
//     DecideAdvance stops the *next* entry, which is where a cancel actually
//     takes effect.
//   - Otherwise the intent names the terminal status: NONE→FAILED,
//     CANCEL→CANCELED, ABORT→ABORTED. An engine CANCELED or NOT_STARTED under no
//     intent is a FAILED entry, not a fault: the entry is already RUNNING, and
//     refusing to give it a terminal state would strand it there with an
//     unreleasable lease.
//   - Recording and timeline outcomes never change the terminal status. A
//     recorder that failed to stop degrades the evidence, not the run.
//
// NextIntent always equals CurrentIntent: completing an entry observes an
// intent, it never changes one. NextIntentRevision always advances by exactly
// one, so every completion writes a distinct value and a replayed commit can be
// told apart from a fresh one. NextCancellationGeneration advances only when
// the intent was actually carried out — that is, when the terminal status is
// CANCELED or ABORTED — so a generation is never spent on an intent that lost
// to a finished run, and a generation at MaxExpectedEntryCompletionRevision
// blocks only the completions that would have had to advance it.
//
// Ordering: this decision must be reached, and committed through
// EntryCompletionTransaction, before scheduling asks DecideAdvance whether the
// next entry may run. DecideAdvance reads the counters this decision produces;
// calling it first would read a pre-terminal state and let a canceled instance
// start one more entry.
func DecideEntryCompletion(state EntryCompletionState, outcome EngineOutcome) (EntryCompletionDecision, error) {
	if err := state.Validate(); err != nil {
		return EntryCompletionDecision{}, err
	}
	if err := outcome.Validate(); err != nil {
		return EntryCompletionDecision{}, err
	}
	if state.EntryStatus != domainexecution.EntryRunning {
		return EntryCompletionDecision{}, entryCompletionNotRunningError()
	}
	// The intent revision advances on every completion, so it is always the
	// decision's own successor that has to be representable.
	if state.TerminalIntentRevision >= MaxExpectedEntryCompletionRevision {
		return EntryCompletionDecision{}, entryCompletionRevisionExhaustedError()
	}

	status := decideTerminalEntryStatus(state.TerminalIntent, outcome.Result.ExecutionOutcome)
	nextGeneration := state.CancellationGeneration
	if status == domainexecution.EntryCanceled || status == domainexecution.EntryAborted {
		// Only a generation this decision actually spends needs a successor. A
		// generation carried through unchanged has nothing to overflow, and
		// refusing it would strand a finished entry in RUNNING with a lease
		// nobody can release — the exact outcome this contract exists to prevent.
		if state.CancellationGeneration >= MaxExpectedEntryCompletionRevision {
			return EntryCompletionDecision{}, entryCompletionRevisionExhaustedError()
		}
		nextGeneration++
	}
	return EntryCompletionDecision{
		EntryStatus:                   status,
		TerminalCause:                 terminalCauseOf(outcome.Result.ExecutionOutcome),
		CurrentIntent:                 state.TerminalIntent,
		CurrentIntentRevision:         state.TerminalIntentRevision,
		CurrentCancellationGeneration: state.CancellationGeneration,
		NextIntent:                    state.TerminalIntent,
		NextIntentRevision:            state.TerminalIntentRevision + 1,
		NextCancellationGeneration:    nextGeneration,
	}, nil
}

// terminalCauseOf reads the observation axis out of the engine outcome. It is
// total over the validated vocabulary, and deliberately takes no intent: an
// intent can change which terminal status an unfinished entry reaches, but
// nothing about an intent changes whether the run was seen.
func terminalCauseOf(executed engine.ExecutionOutcome) TerminalCause {
	switch executed {
	case engine.ExecutionNotStarted:
		return TerminalCauseNotStarted
	case engine.ExecutionInterrupted:
		return TerminalCauseInterrupted
	default:
		// SUCCEEDED, FAILED and CANCELED are all reports from a run that
		// happened. The trailing case is exhaustive over the validated
		// vocabulary rather than a catch-all: EngineOutcome.Validate has already
		// refused anything outside it.
		return TerminalCauseCompleted
	}
}

// decideTerminalEntryStatus is total over the validated vocabulary: both
// switches are exhaustive, and the trailing returns are unreachable rather than
// catch-alls that would silently absorb a new engine constant.
func decideTerminalEntryStatus(intent TerminalIntent, executed engine.ExecutionOutcome) domainexecution.EntryStatus {
	if executed == engine.OutcomeSucceeded {
		return domainexecution.EntrySucceeded
	}
	switch intent {
	case TerminalIntentCancel:
		return domainexecution.EntryCanceled
	case TerminalIntentAbort:
		return domainexecution.EntryAborted
	default:
		return domainexecution.EntryFailed
	}
}
