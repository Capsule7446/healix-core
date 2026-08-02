package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"strings"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const (
	CodeInstanceCommandIdentityConflict  fault.Code = "EXECUTION_INSTANCE_COMMAND_IDENTITY_CONFLICT"
	CodeInstanceIdentityConflict         fault.Code = "EXECUTION_INSTANCE_IDENTITY_CONFLICT"
	CodeInstanceRevisionConflict         fault.Code = "EXECUTION_INSTANCE_REVISION_CONFLICT"
	CodeInstanceStatusConflict           fault.Code = "EXECUTION_INSTANCE_STATUS_CONFLICT"
	CodeQueueRevisionConflict            fault.Code = "EXECUTION_QUEUE_REVISION_CONFLICT"
	CodeQueueMembershipConflict          fault.Code = "EXECUTION_QUEUE_MEMBERSHIP_CONFLICT"
	CodeInstanceAdapterContractViolation fault.Code = "EXECUTION_INSTANCE_COMMAND_ADAPTER_CONTRACT_VIOLATION"
	CodeCancelInstanceCommandInvalid     fault.Code = "EXECUTION_CANCEL_INSTANCE_COMMAND_INVALID"
	CodeAbortInstanceCommandInvalid      fault.Code = "EXECUTION_ABORT_INSTANCE_COMMAND_INVALID"
	CodeReorderQueueCommandInvalid       fault.Code = "EXECUTION_REORDER_QUEUE_COMMAND_INVALID"
)

func cancelInstanceCommandInvalidError(cause error) error {
	return newInstanceCommandWrappedFault(cause, fault.InvalidArgument, CodeCancelInstanceCommandInvalid, "cancel instance command is invalid")
}

func abortInstanceCommandInvalidError(cause error) error {
	return newInstanceCommandWrappedFault(cause, fault.InvalidArgument, CodeAbortInstanceCommandInvalid, "abort instance command is invalid")
}

func reorderQueueCommandInvalidError(cause error) error {
	return newInstanceCommandWrappedFault(cause, fault.InvalidArgument, CodeReorderQueueCommandInvalid, "reorder queue command is invalid")
}

func runCommandConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceCommandIdentityConflict, "instance command identity conflicts with an existing request")
}

func runIdentityConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceIdentityConflict, "instance identity conflicts with the authoritative state")
}

func runRevisionConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceRevisionConflict, "instance revision conflicts with current state")
}

func runStatusConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceStatusConflict, "instance status conflicts with current state")
}

func queueRevisionConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeQueueRevisionConflict, "queue revision conflicts with current state")
}

func queueMembershipConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeQueueMembershipConflict, "queue membership conflicts with the authoritative state")
}

func runAdapterContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.Internal, CodeInstanceAdapterContractViolation, "instance command adapter returned an invalid authoritative result")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func newInstanceCommandWrappedFault(cause error, kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func newInstanceCommandFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeInstanceSignalRetryable fault.Code = "EXECUTION_INSTANCE_SIGNAL_RETRYABLE"

func runSignalRetryableError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Unavailable,
		CodeInstanceSignalRetryable,
		"execution cancellation signal must be retried",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

type CancelInstanceCommand struct {
	CommandID            string
	InstanceID           domainexecution.InstanceID
	ExpectedStatus       domainexecution.InstanceStatus
	ExpectedRevision, At int64
}
type AbortInstanceCommand struct {
	CommandID            string
	InstanceID           domainexecution.InstanceID
	ExpectedRevision, At int64
	Fence                domainexecution.WorkerFence
}
type ReorderQueueCommand struct {
	CommandID, ScopeID string
	ExpectedRevision   int64
	InstanceIDs        []string
}

type InstanceCommandResult struct {
	Run            domainexecution.Instance
	Revision       int64
	WasApplied     bool
	SignalRequired bool
}
type ReorderQueueResult struct {
	ScopeID     string
	Revision    int64
	InstanceIDs []string
	WasApplied  bool
}

// InstanceCommandStore must canonicalize CommandID payloads and execute each method
// atomically. Cancel removes a queued Run, or transitions a running Run and
// invalidates its fence. Abort accepts only RUNNING, transitions to ABORTED and
// invalidates the supplied fence before returning. The returned result is the
// authoritative committed/replayed value, including after an unknown commit.
type InstanceCommandStore interface {
	Cancel(context.Context, CancelInstanceCommand) (InstanceCommandResult, error)
	Abort(context.Context, AbortInstanceCommand) (InstanceCommandResult, error)
}

// QueueCommandStore atomically applies a queue revision CAS. InstanceIDs must be the
// full exact permutation of all currently unclaimed QUEUED Runs in ScopeID; the
// store rejects duplicates, omissions, foreign, claimed, or nonqueued members.
type QueueCommandStore interface {
	Reorder(context.Context, ReorderQueueCommand) (ReorderQueueResult, error)
}

type InstanceCancellationSignaler interface {
	SignalInstanceCancellation(context.Context, domainexecution.InstanceID) error
}

type CancelInstanceService struct {
	store    InstanceCommandStore
	signaler InstanceCancellationSignaler
}

func NewCancelInstanceService(store InstanceCommandStore, signaler InstanceCancellationSignaler) CancelInstanceService {
	return CancelInstanceService{store: store, signaler: signaler}
}
func (s CancelInstanceService) CancelInstance(ctx context.Context, command CancelInstanceCommand) (InstanceCommandResult, error) {
	if err := validateCancel(command); err != nil {
		return InstanceCommandResult{}, err
	}
	if isNilPort(s.store) {
		return InstanceCommandResult{}, schedulingDependencyRequiredError()
	}
	result, err := s.store.Cancel(ctx, command)
	if err != nil {
		return InstanceCommandResult{}, classifySchedulingAdapterFailure(err)
	}
	if err := validateInstanceResult(command.InstanceID, domainexecution.Canceled, command.ExpectedRevision, result); err != nil {
		// validateInstanceResult already returns
		// EXECUTION_INSTANCE_COMMAND_ADAPTER_CONTRACT_VIOLATION.
		return InstanceCommandResult{}, err
	}
	shouldSignal := command.ExpectedStatus == domainexecution.Running
	if result.SignalRequired != shouldSignal {
		return InstanceCommandResult{}, runAdapterContractViolationError(errors.New("unexpected SignalRequired value"))
	}
	return signalIfRequired(ctx, s.signaler, result)
}

type AbortInstanceService struct {
	store    InstanceCommandStore
	signaler InstanceCancellationSignaler
}

func NewAbortInstanceService(store InstanceCommandStore, signaler InstanceCancellationSignaler) AbortInstanceService {
	return AbortInstanceService{store: store, signaler: signaler}
}
func (s AbortInstanceService) AbortInstance(ctx context.Context, command AbortInstanceCommand) (InstanceCommandResult, error) {
	if err := validateAbort(command); err != nil {
		return InstanceCommandResult{}, err
	}
	if isNilPort(s.store) {
		return InstanceCommandResult{}, schedulingDependencyRequiredError()
	}
	result, err := s.store.Abort(ctx, command)
	if err != nil {
		return InstanceCommandResult{}, classifySchedulingAdapterFailure(err)
	}
	if err := validateInstanceResult(command.InstanceID, domainexecution.Aborted, command.ExpectedRevision, result); err != nil {
		return InstanceCommandResult{}, err
	}
	if !result.SignalRequired {
		return InstanceCommandResult{}, runAdapterContractViolationError(errors.New("abort must require cancellation signal"))
	}
	return signalIfRequired(ctx, s.signaler, result)
}

func signalIfRequired(ctx context.Context, signaler InstanceCancellationSignaler, result InstanceCommandResult) (InstanceCommandResult, error) {
	if !result.SignalRequired {
		return result, nil
	}
	if isNilPort(signaler) {
		return result, runSignalRetryableError(errors.New("cancellation signaler is unavailable"))
	}
	if err := signaler.SignalInstanceCancellation(ctx, result.Run.ID); err != nil {
		return result, runSignalRetryableError(err)
	}
	return result, nil
}

func validateCancel(command CancelInstanceCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || command.InstanceID.Validate() != nil || command.ExpectedRevision < 0 || command.At <= 0 || (command.ExpectedStatus != domainexecution.Queued && command.ExpectedStatus != domainexecution.Running) {
		return cancelInstanceCommandInvalidError(nil)
	}
	return nil
}

func validateAbort(command AbortInstanceCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || command.InstanceID.Validate() != nil || command.ExpectedRevision < 0 || command.At <= 0 || command.Fence.InstanceID != command.InstanceID {
		return abortInstanceCommandInvalidError(nil)
	}
	if err := command.Fence.Validate(); err != nil {
		return abortInstanceCommandInvalidError(err)
	}
	return nil
}

type ReorderQueueService struct{ store QueueCommandStore }

func NewReorderQueueService(store QueueCommandStore) ReorderQueueService {
	return ReorderQueueService{store: store}
}
func (s ReorderQueueService) ReorderQueue(ctx context.Context, command ReorderQueueCommand) (ReorderQueueResult, error) {
	if err := validateReorder(command); err != nil {
		return ReorderQueueResult{}, err
	}
	if isNilPort(s.store) {
		return ReorderQueueResult{}, schedulingDependencyRequiredError()
	}
	ownedCommand := command
	ownedCommand.InstanceIDs = append([]string(nil), command.InstanceIDs...)
	result, err := s.store.Reorder(ctx, ownedCommand)
	if err != nil {
		return ReorderQueueResult{}, classifySchedulingAdapterFailure(err)
	}
	if result.ScopeID != command.ScopeID || result.Revision != command.ExpectedRevision+1 || len(result.InstanceIDs) != len(command.InstanceIDs) {
		return ReorderQueueResult{}, runAdapterContractViolationError(errors.New("reorder result identity, revision, or membership count is invalid"))
	}
	for index := range command.InstanceIDs {
		if result.InstanceIDs[index] != command.InstanceIDs[index] {
			return ReorderQueueResult{}, runAdapterContractViolationError(errors.New("reorder result membership order is invalid"))
		}
	}
	result.InstanceIDs = append([]string(nil), result.InstanceIDs...)
	return result, nil
}
func validateInstanceResult(instanceID domainexecution.InstanceID, status domainexecution.InstanceStatus, expectedRevision int64, result InstanceCommandResult) error {
	if result.Run.ID != instanceID {
		return runAdapterContractViolationError(runIdentityConflictError())
	}
	if result.Run.Status != status {
		return runAdapterContractViolationError(runStatusConflictError())
	}
	if result.Revision != expectedRevision+1 {
		return runAdapterContractViolationError(runRevisionConflictError())
	}
	if err := domainexecution.ValidateInstance(result.Run); err != nil {
		return runAdapterContractViolationError(err)
	}
	return nil
}

// These digests are written field by field rather than through json.Marshal.
// The reflective route silently dropped every field it could not see: an
// execution coordinate is a struct whose only field is unexported, so
// json.Marshal encoded InstanceID as {} and two cancellations of *different*
// instances sharing a command id produced the same digest. Nothing failed —
// the replay check simply stopped being able to tell them apart.
//
// These three strings are wire tags, not Go names. They are the domain
// separation prefix of a stored idempotency digest, so a later rename must not
// touch them. They were introduced by the field-by-field rewrite below, which
// changed the digest of every cancel, abort, and reorder record stored before
// it — see docs/contracts/digest-wire-tags.md for that break and its status.
//
// Writing the fields out also makes the digest independent of Go names, so a
// later rename cannot move a value that idempotency records are keyed on.
const (
	cancelInstanceRequestDigestV1 = "cancel-instance-request-v1"
	abortInstanceRequestDigestV1  = "abort-instance-request-v1"
	reorderQueueRequestDigestV1   = "reorder-queue-request-v1"
)

func finishDigest(h hash.Hash) (string, error) {
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func CancelInstanceRequestDigest(command CancelInstanceCommand) (string, error) {
	h := sha256.New()
	writeDigestString(h, cancelInstanceRequestDigestV1)
	writeDigestString(h, command.CommandID)
	writeDigestString(h, command.InstanceID.String())
	writeDigestString(h, string(command.ExpectedStatus))
	writeDigestUint64(h, uint64(command.ExpectedRevision))
	writeDigestUint64(h, uint64(command.At))
	return finishDigest(h)
}

func AbortInstanceRequestDigest(command AbortInstanceCommand) (string, error) {
	h := sha256.New()
	writeDigestString(h, abortInstanceRequestDigestV1)
	writeDigestString(h, command.CommandID)
	writeDigestString(h, command.InstanceID.String())
	writeDigestUint64(h, uint64(command.ExpectedRevision))
	writeDigestUint64(h, uint64(command.At))
	// The fence identifies which worker is allowed to abort, so two aborts that
	// differ only by fence are different requests.
	writeDigestString(h, command.Fence.InstanceID.String())
	writeDigestString(h, command.Fence.ClaimToken)
	return finishDigest(h)
}

func ReorderQueueRequestDigest(command ReorderQueueCommand) (string, error) {
	h := sha256.New()
	writeDigestString(h, reorderQueueRequestDigestV1)
	writeDigestString(h, command.CommandID)
	writeDigestString(h, command.ScopeID)
	writeDigestUint64(h, uint64(command.ExpectedRevision))
	writeDigestUint64(h, uint64(len(command.InstanceIDs)))
	for _, id := range command.InstanceIDs {
		writeDigestString(h, id)
	}
	return finishDigest(h)
}

func validateReorder(command ReorderQueueCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.ScopeID) == "" || command.ExpectedRevision < 0 || len(command.InstanceIDs) == 0 {
		return reorderQueueCommandInvalidError(nil)
	}
	seen := make(map[string]struct{}, len(command.InstanceIDs))
	for _, id := range command.InstanceIDs {
		if strings.TrimSpace(id) == "" {
			return reorderQueueCommandInvalidError(nil)
		}
		if _, ok := seen[id]; ok {
			return queueMembershipConflictError()
		}
		seen[id] = struct{}{}
	}
	return nil
}
