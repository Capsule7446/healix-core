package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func cancelRunCommandInvalidError(cause error) error {
	return newRunCommandWrappedFault(cause, fault.InvalidArgument, CodeCancelInstanceCommandInvalid, "cancel instance command is invalid")
}

func abortRunCommandInvalidError(cause error) error {
	return newRunCommandWrappedFault(cause, fault.InvalidArgument, CodeAbortInstanceCommandInvalid, "abort instance command is invalid")
}

func reorderQueueCommandInvalidError(cause error) error {
	return newRunCommandWrappedFault(cause, fault.InvalidArgument, CodeReorderQueueCommandInvalid, "reorder queue command is invalid")
}

func runCommandConflictError() error {
	return newRunCommandFault(fault.Conflict, CodeInstanceCommandIdentityConflict, "instance command identity conflicts with an existing request")
}

func runIdentityConflictError() error {
	return newRunCommandFault(fault.Conflict, CodeInstanceIdentityConflict, "instance identity conflicts with the authoritative state")
}

func runRevisionConflictError() error {
	return newRunCommandFault(fault.Conflict, CodeInstanceRevisionConflict, "instance revision conflicts with current state")
}

func runStatusConflictError() error {
	return newRunCommandFault(fault.Conflict, CodeInstanceStatusConflict, "instance status conflicts with current state")
}

func queueRevisionConflictError() error {
	return newRunCommandFault(fault.Conflict, CodeQueueRevisionConflict, "queue revision conflicts with current state")
}

func queueMembershipConflictError() error {
	return newRunCommandFault(fault.Conflict, CodeQueueMembershipConflict, "queue membership conflicts with the authoritative state")
}

func runAdapterContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.Internal, CodeInstanceAdapterContractViolation, "instance command adapter returned an invalid authoritative result")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func newRunCommandWrappedFault(cause error, kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func newRunCommandFault(kind fault.Kind, code fault.Code, message string) error {
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

type CancelRunCommand struct {
	CommandID            string
	InstanceID           domainexecution.InstanceID
	ExpectedStatus       domainexecution.InstanceStatus
	ExpectedRevision, At int64
}
type AbortRunCommand struct {
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

type RunCommandResult struct {
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

// RunCommandStore must canonicalize CommandID payloads and execute each method
// atomically. Cancel removes a queued Run, or transitions a running Run and
// invalidates its fence. Abort accepts only RUNNING, transitions to ABORTED and
// invalidates the supplied fence before returning. The returned result is the
// authoritative committed/replayed value, including after an unknown commit.
type RunCommandStore interface {
	Cancel(context.Context, CancelRunCommand) (RunCommandResult, error)
	Abort(context.Context, AbortRunCommand) (RunCommandResult, error)
}

// QueueCommandStore atomically applies a queue revision CAS. InstanceIDs must be the
// full exact permutation of all currently unclaimed QUEUED Runs in ScopeID; the
// store rejects duplicates, omissions, foreign, claimed, or nonqueued members.
type QueueCommandStore interface {
	Reorder(context.Context, ReorderQueueCommand) (ReorderQueueResult, error)
}

type RunCancellationSignaler interface {
	SignalRunCancellation(context.Context, domainexecution.InstanceID) error
}

type CancelRunService struct {
	store    RunCommandStore
	signaler RunCancellationSignaler
}

func NewCancelRunService(store RunCommandStore, signaler RunCancellationSignaler) CancelRunService {
	return CancelRunService{store: store, signaler: signaler}
}
func (s CancelRunService) CancelRun(ctx context.Context, command CancelRunCommand) (RunCommandResult, error) {
	if err := validateCancel(command); err != nil {
		return RunCommandResult{}, err
	}
	if isNilPort(s.store) {
		return RunCommandResult{}, schedulingDependencyRequiredError()
	}
	result, err := s.store.Cancel(ctx, command)
	if err != nil {
		return RunCommandResult{}, classifySchedulingAdapterFailure(err)
	}
	if err := validateRunResult(command.InstanceID, domainexecution.Canceled, command.ExpectedRevision, result); err != nil {
		// validateRunResult already returns
		// EXECUTION_INSTANCE_COMMAND_ADAPTER_CONTRACT_VIOLATION.
		return RunCommandResult{}, err
	}
	shouldSignal := command.ExpectedStatus == domainexecution.Running
	if result.SignalRequired != shouldSignal {
		return RunCommandResult{}, runAdapterContractViolationError(errors.New("unexpected SignalRequired value"))
	}
	return signalIfRequired(ctx, s.signaler, result)
}

type AbortRunService struct {
	store    RunCommandStore
	signaler RunCancellationSignaler
}

func NewAbortRunService(store RunCommandStore, signaler RunCancellationSignaler) AbortRunService {
	return AbortRunService{store: store, signaler: signaler}
}
func (s AbortRunService) AbortRun(ctx context.Context, command AbortRunCommand) (RunCommandResult, error) {
	if err := validateAbort(command); err != nil {
		return RunCommandResult{}, err
	}
	if isNilPort(s.store) {
		return RunCommandResult{}, schedulingDependencyRequiredError()
	}
	result, err := s.store.Abort(ctx, command)
	if err != nil {
		return RunCommandResult{}, classifySchedulingAdapterFailure(err)
	}
	if err := validateRunResult(command.InstanceID, domainexecution.Aborted, command.ExpectedRevision, result); err != nil {
		return RunCommandResult{}, err
	}
	if !result.SignalRequired {
		return RunCommandResult{}, runAdapterContractViolationError(errors.New("abort must require cancellation signal"))
	}
	return signalIfRequired(ctx, s.signaler, result)
}

func signalIfRequired(ctx context.Context, signaler RunCancellationSignaler, result RunCommandResult) (RunCommandResult, error) {
	if !result.SignalRequired {
		return result, nil
	}
	if isNilPort(signaler) {
		return result, runSignalRetryableError(errors.New("cancellation signaler is unavailable"))
	}
	if err := signaler.SignalRunCancellation(ctx, result.Run.ID); err != nil {
		return result, runSignalRetryableError(err)
	}
	return result, nil
}

func validateCancel(command CancelRunCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || command.InstanceID.Validate() != nil || command.ExpectedRevision < 0 || command.At <= 0 || (command.ExpectedStatus != domainexecution.Queued && command.ExpectedStatus != domainexecution.Running) {
		return cancelRunCommandInvalidError(nil)
	}
	return nil
}

func validateAbort(command AbortRunCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || command.InstanceID.Validate() != nil || command.ExpectedRevision < 0 || command.At <= 0 || command.Fence.InstanceID != command.InstanceID {
		return abortRunCommandInvalidError(nil)
	}
	if err := command.Fence.Validate(); err != nil {
		return abortRunCommandInvalidError(err)
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
func validateRunResult(instanceID domainexecution.InstanceID, status domainexecution.InstanceStatus, expectedRevision int64, result RunCommandResult) error {
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

func canonicalDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode command digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func CancelRunRequestDigest(command CancelRunCommand) (string, error) {
	return canonicalDigest(command)
}

func AbortRunRequestDigest(command AbortRunCommand) (string, error) {
	return canonicalDigest(command)
}

func ReorderQueueRequestDigest(command ReorderQueueCommand) (string, error) {
	owned := command
	owned.InstanceIDs = append([]string(nil), command.InstanceIDs...)
	return canonicalDigest(owned)
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
