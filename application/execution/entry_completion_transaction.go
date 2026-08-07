package execution

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// completeEntryRequestDigestV1 is the immutable wire tag for the entry
// completion request digest. Changing these bytes changes every digest hosts
// have already persisted, which would turn every in-flight completion into a
// second apply. A new shape gets a new tag, never an edit to this one.
const completeEntryRequestDigestV1 = "complete-entry-request-v1"

func completeEntryCommandInvalidError(cause error, violation fault.Violation) error {
	err, constructionErr := fault.Wrap(cause, fault.InvalidArgument, CodeCompleteEntryCommandInvalid, "complete entry command is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func completeEntryDigestMismatchError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeCompleteEntryDigestMismatch, "complete entry intent does not match its command", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompleteEntryUnavailableError reports that no entry completion transaction is
// wired. Hosts construct it when their own composition root cannot supply an
// adapter, so a missing dependency is reported the same way from either side.
func CompleteEntryUnavailableError() error {
	err, constructionErr := fault.New(fault.Unavailable, CodeCompleteEntryUnavailable, "entry completion transaction is unavailable")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompleteEntryIdentityConflictError reports that the entry no longer holds the
// state the command observed. Adapters return it from CompleteEntry when their
// compare-and-swap matches no row: another writer reached the entry first, and
// the caller must re-read before retrying.
func CompleteEntryIdentityConflictError() error {
	err, constructionErr := fault.New(fault.Conflict, CodeCompleteEntryIdentityConflict, "entry completion observed a stale state")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func completeEntryContractViolationError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.Internal, CodeCompleteEntryAdapterContractViolation, "entry completion adapter violated its contract", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompleteEntryCommand is one request to end one entry.
//
// State and Outcome are the whole decision basis; core derives the terminal
// answer from them and nothing else. AbortPendingCommandID is the identity of
// an abort command still awaiting its receipt, carried here and deliberately
// not on State: it is idempotency and audit identity, never decision basis, and
// the host needs it at commit time to fill the action gate's CommandID and
// write the abort command's receipt inside the same transaction. It is empty
// when no abort is pending.
//
// The command carries no timestamp. A wall clock would change the digest on
// every retry, which would turn a crash-and-retry into a second apply — exactly
// what the digest exists to prevent. Hosts stamp completion times themselves,
// outside the identity.
type CompleteEntryCommand struct {
	EntryID               domainexecution.EntryID
	Fence                 domainexecution.WorkerFence
	State                 EntryCompletionState
	Outcome               EngineOutcome
	AbortPendingCommandID string
}

// Validate reports whether the command is well formed enough to digest. It says
// nothing about whether the entry can be completed — DecideEntryCompletion
// answers that, and the two failures have different remediations.
func (command CompleteEntryCommand) Validate() error {
	if err := command.EntryID.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "entryId", "entry id is invalid"))
	}
	if err := command.Fence.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "fence", "worker execution authority is invalid"))
	}
	if err := command.State.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "state", "observed entry state is invalid"))
	}
	if err := command.Outcome.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "outcome", "engine outcome is invalid"))
	}
	// Absent is legal; present but blank is not. A blank identity would digest
	// as its own distinct request while naming no command anyone can find.
	if command.AbortPendingCommandID != "" && strings.TrimSpace(command.AbortPendingCommandID) == "" {
		return completeEntryCommandInvalidError(nil, mustEntryCompletionViolation(fault.CodeFieldInvalid, "abortPendingCommandId", "abort pending command id is present but blank"))
	}
	return nil
}

// CompleteEntryRequestDigest is the stable identity of one completion request.
//
// It is built field by field with length prefixes rather than by marshalling,
// so no field ordering, encoder version or optional-field convention can move
// the bytes underneath a digest a host already persisted. A request that cannot
// be validated has no digest: the function returns an empty string with the
// fault, so a refused command can never be recorded under a plausible-looking
// identity.
func CompleteEntryRequestDigest(command CompleteEntryCommand) (string, error) {
	if err := command.Validate(); err != nil {
		return "", err
	}

	h := sha256.New()
	writeDigestString(h, completeEntryRequestDigestV1)
	writeDigestString(h, command.EntryID.String())
	writeDigestString(h, command.Fence.InstanceID.String())
	writeDigestString(h, command.Fence.ClaimToken)
	writeDigestString(h, string(command.State.EntryStatus))
	writeDigestString(h, string(command.State.TerminalIntent))
	writeDigestUint64(h, uint64(command.State.TerminalIntentRevision))
	writeDigestUint64(h, uint64(command.State.CancellationGeneration))
	writeDigestString(h, string(command.Outcome.Result.ExecutionOutcome))
	writeDigestString(h, string(command.Outcome.Result.RecordingOutcome))
	writeDigestString(h, string(command.Outcome.Result.TimelineOutcome))
	writeDigestString(h, string(command.Outcome.FailureCode))
	writeDigestString(h, command.AbortPendingCommandID)
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// CompleteEntryIntent is one completion request, its identity, and the decision
// core reached for it.
//
// Decision travels with the intent rather than being recomputed inside the
// adapter, because the host must never derive NextIntentRevision or
// NextCancellationGeneration itself. ValidateCompleteEntryIntentDigest turns
// that promise into a check the adapter runs on every apply.
type CompleteEntryIntent struct {
	EntryID       domainexecution.EntryID
	RequestDigest string
	Command       CompleteEntryCommand
	Decision      EntryCompletionDecision
}

// ValidateCompleteEntryIntentDigest re-derives both the digest and the decision
// from the intent's own command and reports any disagreement.
//
// Adapters must call it first in CompleteEntry, before touching storage. It is
// the mechanical form of the two promises the contract rests on: that the
// identity a completion is recorded under really is the identity of the request
// being applied, and that the counters being written are the ones core
// produced. Both checks are local — no Context, no ports, no ownership.
func ValidateCompleteEntryIntentDigest(intent CompleteEntryIntent) error {
	digest, err := CompleteEntryRequestDigest(intent.Command)
	if err != nil {
		return err
	}
	if intent.RequestDigest != digest {
		return completeEntryDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "requestDigest", "request digest does not match the command"))
	}
	if intent.EntryID != intent.Command.EntryID {
		return completeEntryDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "entryId", "intent identity does not match the command"))
	}
	decision, err := DecideEntryCompletion(intent.Command.State, intent.Command.Outcome)
	if err != nil {
		return err
	}
	if intent.Decision != decision {
		return completeEntryDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "decision", "decision does not follow from the command"))
	}
	return nil
}

// CompleteEntryStatus reports whether a completion did the work or found it
// already done.
type CompleteEntryStatus string

const (
	// CompleteEntryApplied means this call performed the completion.
	CompleteEntryApplied CompleteEntryStatus = "APPLIED"
	// CompleteEntryReplayed means an identical request had already been applied
	// and this call changed nothing.
	CompleteEntryReplayed CompleteEntryStatus = "REPLAYED"
)

// CompleteEntryOutcome is what one completion attempt produced. Decision is the
// decision that was recorded, so a replay returns the same terminal answer the
// original apply did rather than a freshly computed one.
type CompleteEntryOutcome struct {
	Status        CompleteEntryStatus
	EntryID       domainexecution.EntryID
	RequestDigest string
	Decision      EntryCompletionDecision
}

// EntryCompletionTransaction is the port a host implements to end an entry.
//
// Atomicity boundary — everything below must land in ONE transaction, or none
// of it may land:
//
//   - the entry's terminal status and the two counters from Decision;
//   - the entry's terminal facts;
//   - the evidence references produced by the same run;
//   - the terminalization of the execution action gate, using
//     Command.AbortPendingCommandID as the gate's CommandID and the Decision's
//     Next* pair as the exact values written;
//   - the abort command receipt, when Command.AbortPendingCommandID is present;
//   - the outbox record announcing the completion;
//   - the idempotency receipt keyed by (EntryID, RequestDigest), written last so
//     a crash before it leaves the whole batch invisible and retryable.
//
// Anything a host does outside that boundary must be re-derivable from what is
// inside it: uploading a recording, notifying a UI, releasing an OS handle.
// Writes that make the completion look done — anything a later reader could
// mistake for a committed terminal state — belong inside.
//
// Ordering — the completion must be committed BEFORE scheduling asks
// DecideAdvance whether the next entry may run. DecideAdvance reads the
// counters this transaction writes; running it first would read a pre-terminal
// state and let a canceled instance start one more entry.
//
// Context is for cancellation and deadlines only; an implementation must not
// read values out of it. Neither method takes ownership of anything it is
// given, and the values it returns are owned by the caller.
type EntryCompletionTransaction interface {
	// LookupEntryCompletion reports the outcome already recorded for one
	// request digest. It must report CompleteEntryReplayed on a hit, must not
	// write, and must return (zero, false, nil) when nothing is recorded.
	LookupEntryCompletion(ctx context.Context, entryID domainexecution.EntryID, requestDigest string) (CompleteEntryOutcome, bool, error)
	// CompleteEntry applies one completion atomically.
	//
	// It must call ValidateCompleteEntryIntentDigest before touching storage,
	// must write the Decision's fields verbatim without recomputing either
	// counter, and must return CompleteEntryReplayed rather than applying twice
	// when it finds the receipt already present inside the transaction. It
	// returns CompleteEntryIdentityConflictError when the entry no longer holds
	// the state the command observed.
	CompleteEntry(ctx context.Context, intent CompleteEntryIntent) (CompleteEntryOutcome, error)
}

// EntryCompletionService turns a validated completion request into exactly one
// committed terminal state.
//
// It is the only supported way to reach EntryCompletionTransaction: it decides
// before it writes, so an undecidable request never reaches storage, and it
// checks every outcome the adapter returns against the request that produced
// it. Construct it with NewEntryCompletionService. The zero value has no
// transaction and refuses every call with CodeCompleteEntryUnavailable rather
// than panicking.
type EntryCompletionService struct {
	transaction EntryCompletionTransaction
}

// NewEntryCompletionService wires a service to one transaction.
//
// It accepts a nil or typed-nil transaction instead of refusing at construction
// time, because a composition root that assembles services from a config map
// should fail on the call that needs the missing adapter, naming it, rather
// than at start-up with no request to point at. The service does not own the
// transaction and never closes it.
func NewEntryCompletionService(transaction EntryCompletionTransaction) EntryCompletionService {
	return EntryCompletionService{transaction: transaction}
}

// Complete ends one entry, exactly once.
//
// The order is deliberate: digest (which validates the command), then decide,
// then look for an existing receipt, then apply. Deciding before touching the
// adapter means a request core would refuse — a non-running entry, an exhausted
// revision, a malformed command — costs no storage round trip and leaves no
// partial trace. Replaying the same command returns the recorded outcome with
// CompleteEntryReplayed and applies nothing.
//
// Context is passed to the adapter untouched. The returned outcome is owned by
// the caller; a refused call returns the zero outcome alongside the fault.
func (s EntryCompletionService) Complete(ctx context.Context, command CompleteEntryCommand) (CompleteEntryOutcome, error) {
	if isNilInterfaceValue(s.transaction) {
		return CompleteEntryOutcome{}, CompleteEntryUnavailableError()
	}
	digest, err := CompleteEntryRequestDigest(command)
	if err != nil {
		return CompleteEntryOutcome{}, err
	}
	decision, err := DecideEntryCompletion(command.State, command.Outcome)
	if err != nil {
		return CompleteEntryOutcome{}, err
	}

	recorded, found, err := s.transaction.LookupEntryCompletion(ctx, command.EntryID, digest)
	if err != nil {
		return CompleteEntryOutcome{}, fmt.Errorf("lookup entry completion: %w", err)
	}
	if found {
		if recorded.Status != CompleteEntryReplayed {
			return CompleteEntryOutcome{}, completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "status", "a recorded completion must be reported as replayed"))
		}
		if err := validateCompleteEntryOutcome(recorded, command.EntryID, digest, decision); err != nil {
			return CompleteEntryOutcome{}, err
		}
		return recorded, nil
	}

	applied, err := s.transaction.CompleteEntry(ctx, CompleteEntryIntent{
		EntryID:       command.EntryID,
		RequestDigest: digest,
		Command:       command,
		Decision:      decision,
	})
	if err != nil {
		return CompleteEntryOutcome{}, fmt.Errorf("complete entry: %w", err)
	}
	// APPLIED and REPLAYED are both legal here: a concurrent worker may have
	// committed the identical request between the lookup and the apply, and an
	// adapter that discovers its own receipt inside the transaction is doing
	// exactly what the contract asks.
	if applied.Status != CompleteEntryApplied && applied.Status != CompleteEntryReplayed {
		return CompleteEntryOutcome{}, completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "status", "completion status is not one core defined"))
	}
	if err := validateCompleteEntryOutcome(applied, command.EntryID, digest, decision); err != nil {
		return CompleteEntryOutcome{}, err
	}
	return applied, nil
}

// validateCompleteEntryOutcome holds the adapter to the request it was given.
// An outcome that names a different entry, a different request, or a decision
// the adapter recomputed is a defect the caller must not act on, so it is
// refused rather than returned with a warning.
func validateCompleteEntryOutcome(outcome CompleteEntryOutcome, entryID domainexecution.EntryID, digest string, decision EntryCompletionDecision) error {
	if outcome.EntryID != entryID {
		return completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "entryId", "outcome names a different entry"))
	}
	if outcome.RequestDigest != digest {
		return completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "requestDigest", "outcome names a different request"))
	}
	if outcome.Decision != decision {
		return completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "decision", "outcome carries a decision core did not make"))
	}
	return nil
}

func writeDigestString(h hash.Hash, value string) {
	writeDigestUint64(h, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}

func writeDigestUint64(h hash.Hash, value uint64) {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], value)
	_, _ = h.Write(buffer[:])
}
