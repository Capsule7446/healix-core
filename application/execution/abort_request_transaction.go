package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// requestAbortDigestV1 is the immutable wire tag for the abort request digest.
// Changing these bytes changes every digest hosts have already persisted, which
// would turn a retried abort into a second apply. A new shape gets a new tag,
// never an edit to this one.
const requestAbortDigestV1 = "request-abort-v1"

const (
	// CodeRequestAbortCommandInvalid covers an abort request core cannot digest:
	// a malformed identity, fence, observed state or request. The caller must
	// repair the request, hence INVALID_ARGUMENT.
	CodeRequestAbortCommandInvalid fault.Code = "EXECUTION_REQUEST_ABORT_COMMAND_INVALID"
	// CodeRequestAbortDigestMismatch covers an intent whose digest or decision
	// does not follow from its own command. It is what stops a host from
	// substituting counters core never produced.
	CodeRequestAbortDigestMismatch fault.Code = "EXECUTION_REQUEST_ABORT_DIGEST_MISMATCH"
	// CodeRequestAbortUnavailable covers a service with no transaction behind
	// it. Nothing about the request is wrong, so it is UNAVAILABLE rather than
	// INVALID_ARGUMENT.
	CodeRequestAbortUnavailable fault.Code = "EXECUTION_REQUEST_ABORT_UNAVAILABLE"
	// CodeRequestAbortAdapterContractViolation covers an adapter that returned
	// an outcome the port forbids — an unknown status, a different identity, or
	// a decision it recomputed for itself. That is an implementation defect in
	// the adapter, not a business failure, hence INTERNAL.
	CodeRequestAbortAdapterContractViolation fault.Code = "EXECUTION_REQUEST_ABORT_ADAPTER_CONTRACT_VIOLATION"
	// CodeRequestAbortIdentityConflict is the code adapters raise when the entry
	// no longer holds the state the command claimed to observe, so the
	// compare-and-swap found a different writer had moved first.
	CodeRequestAbortIdentityConflict fault.Code = "EXECUTION_REQUEST_ABORT_IDENTITY_CONFLICT"
)

func requestAbortCommandInvalidError(cause error, violation fault.Violation) error {
	err, constructionErr := fault.Wrap(cause, fault.InvalidArgument, CodeRequestAbortCommandInvalid, "abort request command is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func requestAbortDigestMismatchError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeRequestAbortDigestMismatch, "abort request intent does not match its command", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// RequestAbortUnavailableError is what a service with no transaction returns.
// Hosts classify on it to tell a missing adapter from a rejected request.
func RequestAbortUnavailableError() error {
	err, constructionErr := fault.New(fault.Unavailable, CodeRequestAbortUnavailable, "abort request transaction is unavailable")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// RequestAbortIdentityConflictError is the fault an adapter returns when the
// entry no longer holds the state the command observed. Core exports the
// constructor so every adapter raises the same classified conflict instead of
// inventing one.
func RequestAbortIdentityConflictError() error {
	err, constructionErr := fault.New(fault.Conflict, CodeRequestAbortIdentityConflict, "entry state changed before the abort request was recorded")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func requestAbortContractViolationError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.Internal, CodeRequestAbortAdapterContractViolation, "abort request adapter violated the port contract", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// RequestAbortCommand is one request to stop a running entry, with everything
// needed to identify and decide it.
//
// The command carries no timestamp, for the same reason CompleteEntryCommand
// does not: a wall clock would change the digest on every retry, turning a
// crash-and-retry into a second apply. Hosts stamp request times themselves,
// outside the identity.
type RequestAbortCommand struct {
	EntryID domainexecution.EntryID
	Fence   domainexecution.WorkerFence
	State   EntryCompletionState
	Request AbortRequest
}

// Validate reports whether the command is well formed enough to digest. It says
// nothing about whether the abort can be requested — DecideAbortRequest answers
// that, and the two failures have different remediations.
func (command RequestAbortCommand) Validate() error {
	if err := command.EntryID.Validate(); err != nil {
		return requestAbortCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "entryId", "entry id is invalid"))
	}
	if err := command.Fence.Validate(); err != nil {
		return requestAbortCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "fence", "worker execution authority is invalid"))
	}
	if err := command.State.Validate(); err != nil {
		return requestAbortCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "state", "observed entry state is invalid"))
	}
	if err := command.Request.Validate(); err != nil {
		return requestAbortCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "request", "abort request is invalid"))
	}
	return nil
}

// RequestAbortDigest is the stable identity of one abort request.
//
// It is built field by field with length prefixes rather than by marshalling,
// so no field ordering, encoder version or optional-field convention can move
// the bytes underneath a digest a host already persisted. A request that cannot
// be validated has no digest: the function returns an empty string with the
// fault, so a refused command can never be recorded under a plausible-looking
// identity.
func RequestAbortDigest(command RequestAbortCommand) (string, error) {
	if err := command.Validate(); err != nil {
		return "", err
	}

	h := sha256.New()
	writeDigestString(h, requestAbortDigestV1)
	writeDigestString(h, command.EntryID.String())
	writeDigestString(h, command.Fence.InstanceID.String())
	writeDigestString(h, command.Fence.ClaimToken)
	writeDigestString(h, string(command.State.EntryStatus))
	writeDigestString(h, string(command.State.TerminalIntent))
	writeDigestUint64(h, uint64(command.State.TerminalIntentRevision))
	writeDigestUint64(h, uint64(command.State.CancellationGeneration))
	writeDigestString(h, command.Request.AbortPendingCommandID)
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// RequestAbortIntent is one abort request, its identity, and the decision core
// reached for it.
//
// Decision travels with the intent rather than being recomputed inside the
// adapter, because the host must never derive NextIntentRevision itself.
// ValidateRequestAbortIntentDigest turns that promise into a check the adapter
// runs on every apply.
type RequestAbortIntent struct {
	EntryID       domainexecution.EntryID
	RequestDigest string
	Command       RequestAbortCommand
	Decision      AbortRequestDecision
}

// ValidateRequestAbortIntentDigest re-derives both the digest and the decision
// from the intent's own command and reports any disagreement.
//
// Adapters must call it first in RequestAbort, before touching storage. It is
// the mechanical form of the two promises the contract rests on: that the
// identity a request is recorded under really is the identity of the request
// being applied, and that the counters being written are the ones core
// produced. Both checks are local — no Context, no ports, no ownership.
func ValidateRequestAbortIntentDigest(intent RequestAbortIntent) error {
	digest, err := RequestAbortDigest(intent.Command)
	if err != nil {
		return err
	}
	if intent.RequestDigest != digest {
		return requestAbortDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "requestDigest", "request digest does not match the command"))
	}
	if intent.EntryID != intent.Command.EntryID {
		return requestAbortDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "entryId", "intent identity does not match the command"))
	}
	decision, err := DecideAbortRequest(intent.Command.State, intent.Command.Request)
	if err != nil {
		return err
	}
	if intent.Decision != decision {
		return requestAbortDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "decision", "decision does not follow from the command"))
	}
	return nil
}

// RequestAbortStatus reports whether an abort request did the work or found it
// already done.
type RequestAbortStatus string

const (
	// RequestAbortApplied means this call recorded the pending intent.
	RequestAbortApplied RequestAbortStatus = "APPLIED"
	// RequestAbortReplayed means an identical request had already been applied
	// and this call changed nothing.
	RequestAbortReplayed RequestAbortStatus = "REPLAYED"
)

// RequestAbortOutcome is what one abort request produced. Decision is the
// decision that was recorded, so a replay returns the same answer the original
// apply did rather than a freshly computed one.
type RequestAbortOutcome struct {
	Status        RequestAbortStatus
	EntryID       domainexecution.EntryID
	RequestDigest string
	Decision      AbortRequestDecision
}

// AbortRequestTransaction is the port a host implements to record a pending
// abort intent.
//
// Atomicity boundary — everything below must land in ONE transaction, or none
// of it may land:
//
//   - the pending terminal intent, written as the Decision's Next* values under
//     a compare-and-swap on its Current* values;
//   - the abort command receipt keyed by Command.Request.AbortPendingCommandID;
//   - the idempotency receipt keyed by (EntryID, RequestDigest), written last so
//     a crash before it leaves the whole batch invisible and retryable.
//
// What must NOT happen here, and this is the whole point of D-17 being a
// separate contract: the entry's status is not changed, its facts are not
// written, the claim is not invalidated and the action gate is not
// terminalized. An abort request records that someone asked; it does not end
// anything. Terminating the entry stays the sole job of
// EntryCompletionTransaction, so abort and ordinary completion converge on one
// terminal write path rather than two that can disagree.
//
// Ordering — this transaction commits first, the entry then reaches its natural
// stopping point, and EntryCompletionTransaction.CompleteEntry terminates it
// while finalising the gate. A host that invalidated the claim here would leave
// that completion's authority compare-and-swap pointing at a stale row.
//
// Context is for cancellation and deadlines only; an implementation must not
// read values out of it. Neither method takes ownership of anything it is
// given, and the values it returns are owned by the caller.
type AbortRequestTransaction interface {
	// LookupAbortRequest reports the outcome already recorded for one request
	// digest. It must report RequestAbortReplayed on a hit, must not write, and
	// must return (zero, false, nil) when nothing is recorded.
	LookupAbortRequest(ctx context.Context, entryID domainexecution.EntryID, requestDigest string) (RequestAbortOutcome, bool, error)
	// RequestAbort records one pending abort intent atomically.
	//
	// It must call ValidateRequestAbortIntentDigest before touching storage,
	// must write the Decision's Next* fields verbatim without recomputing the
	// revision, and must return RequestAbortReplayed rather than applying twice
	// when it finds the receipt already present inside the transaction. It
	// returns RequestAbortIdentityConflictError when the entry no longer holds
	// the state the command observed.
	RequestAbort(ctx context.Context, intent RequestAbortIntent) (RequestAbortOutcome, error)
}

// AbortRequestService turns a validated abort request into exactly one recorded
// pending intent.
//
// It is the only supported way to reach AbortRequestTransaction: it decides
// before it writes, so an undecidable request never reaches storage, and it
// checks every outcome the adapter returns against the request that produced
// it. Construct it with NewAbortRequestService. The zero value has no
// transaction and refuses every call with CodeRequestAbortUnavailable rather
// than panicking.
type AbortRequestService struct {
	transaction AbortRequestTransaction
}

// NewAbortRequestService wires a service to one transaction.
//
// It accepts a nil or typed-nil transaction instead of refusing at construction
// time, because a composition root that assembles services from a config map
// should fail on the call that needs the missing adapter, naming it, rather
// than at start-up with no request to point at. The service does not own the
// transaction and never closes it.
func NewAbortRequestService(transaction AbortRequestTransaction) AbortRequestService {
	return AbortRequestService{transaction: transaction}
}

// Request records one pending abort intent, exactly once.
//
// The order mirrors EntryCompletionService.Complete: digest (which validates
// the command), then decide, then look for an existing receipt, then apply.
// Deciding before touching the adapter means a request core would refuse — a
// non-running entry, an abort already in flight, an exhausted revision — costs
// no storage round trip and leaves no partial trace. Replaying the same command
// returns the recorded outcome with RequestAbortReplayed and applies nothing.
//
// Context is passed to the adapter untouched. The returned outcome is owned by
// the caller; a refused call returns the zero outcome alongside the fault.
func (s AbortRequestService) Request(ctx context.Context, command RequestAbortCommand) (RequestAbortOutcome, error) {
	if isNilInterfaceValue(s.transaction) {
		return RequestAbortOutcome{}, RequestAbortUnavailableError()
	}
	digest, err := RequestAbortDigest(command)
	if err != nil {
		return RequestAbortOutcome{}, err
	}
	decision, err := DecideAbortRequest(command.State, command.Request)
	if err != nil {
		return RequestAbortOutcome{}, err
	}

	recorded, found, err := s.transaction.LookupAbortRequest(ctx, command.EntryID, digest)
	if err != nil {
		return RequestAbortOutcome{}, fmt.Errorf("lookup abort request: %w", err)
	}
	if found {
		if recorded.Status != RequestAbortReplayed {
			return RequestAbortOutcome{}, requestAbortContractViolationError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "status", "a recorded abort request must be reported as replayed"))
		}
		if err := validateRequestAbortOutcome(recorded, command.EntryID, digest, decision); err != nil {
			return RequestAbortOutcome{}, err
		}
		return recorded, nil
	}

	applied, err := s.transaction.RequestAbort(ctx, RequestAbortIntent{
		EntryID:       command.EntryID,
		RequestDigest: digest,
		Command:       command,
		Decision:      decision,
	})
	if err != nil {
		return RequestAbortOutcome{}, fmt.Errorf("request abort: %w", err)
	}
	// APPLIED and REPLAYED are both legal here: a concurrent worker may have
	// committed the identical request between the lookup and the apply, and an
	// adapter that discovers its own receipt inside the transaction is doing
	// exactly what the contract asks.
	if applied.Status != RequestAbortApplied && applied.Status != RequestAbortReplayed {
		return RequestAbortOutcome{}, requestAbortContractViolationError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "status", "abort request status is not one core defined"))
	}
	if err := validateRequestAbortOutcome(applied, command.EntryID, digest, decision); err != nil {
		return RequestAbortOutcome{}, err
	}
	return applied, nil
}

// validateRequestAbortOutcome holds the adapter to the request it was given. An
// outcome that names a different entry, a different request, or a decision the
// adapter recomputed is a defect the caller must not act on, so it is refused
// rather than returned with a warning.
func validateRequestAbortOutcome(outcome RequestAbortOutcome, entryID domainexecution.EntryID, digest string, decision AbortRequestDecision) error {
	if outcome.EntryID != entryID {
		return requestAbortContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "entryId", "outcome names a different entry"))
	}
	if outcome.RequestDigest != digest {
		return requestAbortContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "requestDigest", "outcome names a different request"))
	}
	if outcome.Decision != decision {
		return requestAbortContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "decision", "outcome carries a decision core did not make"))
	}
	return nil
}
