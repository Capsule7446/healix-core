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

var (
	ErrRunCommandConflict      = errors.New("run command identity conflict")
	ErrRunIdentityConflict     = errors.New("run identity conflict")
	ErrRunRevisionConflict     = errors.New("run revision conflict")
	ErrRunStatusConflict       = errors.New("run status conflict")
	ErrQueueRevisionConflict   = errors.New("queue revision conflict")
	ErrQueueMembershipConflict = errors.New("queue membership conflict")
	ErrRunAdapterContract      = errors.New("run command adapter contract violation")
)

type CommandConflictError struct{ CommandID string }

func (e *CommandConflictError) Error() string        { return "run command identity conflict: " + e.CommandID }
func (e *CommandConflictError) Is(target error) bool { return target == ErrRunCommandConflict }

type RunIdentityConflictError struct{ RunID string }

func (e *RunIdentityConflictError) Error() string        { return "run identity conflict: " + e.RunID }
func (e *RunIdentityConflictError) Is(target error) bool { return target == ErrRunIdentityConflict }

type RunRevisionConflictError struct {
	RunID            string
	Expected, Actual int64
}

func (e *RunRevisionConflictError) Error() string {
	return fmt.Sprintf("run revision conflict: %s expected %d actual %d", e.RunID, e.Expected, e.Actual)
}
func (e *RunRevisionConflictError) Is(target error) bool { return target == ErrRunRevisionConflict }

type RunStatusConflictError struct {
	RunID            string
	Expected, Actual domainexecution.RunStatus
}

func (e *RunStatusConflictError) Error() string {
	return fmt.Sprintf("run status conflict: %s expected %s actual %s", e.RunID, e.Expected, e.Actual)
}
func (e *RunStatusConflictError) Is(target error) bool { return target == ErrRunStatusConflict }

type QueueRevisionConflictError struct {
	ScopeID          string
	Expected, Actual int64
}

func (e *QueueRevisionConflictError) Error() string {
	return fmt.Sprintf("queue revision conflict: %s expected %d actual %d", e.ScopeID, e.Expected, e.Actual)
}
func (e *QueueRevisionConflictError) Is(target error) bool { return target == ErrQueueRevisionConflict }

type QueueMembershipConflictError struct{ ScopeID string }

func (e *QueueMembershipConflictError) Error() string {
	return "queue membership conflict: " + e.ScopeID
}
func (e *QueueMembershipConflictError) Is(target error) bool {
	return target == ErrQueueMembershipConflict
}

type RunAdapterContractError struct {
	Operation string
	Cause     error
}

func (e *RunAdapterContractError) Error() string {
	return fmt.Sprintf("run command adapter contract violation: %s: %v", e.Operation, e.Cause)
}
func (e *RunAdapterContractError) Unwrap() error        { return e.Cause }
func (e *RunAdapterContractError) Is(target error) bool { return target == ErrRunAdapterContract }

const CodeRunSignalRetryable fault.Code = "EXECUTION_RUN_SIGNAL_RETRYABLE"

func runSignalRetryableError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Unavailable,
		CodeRunSignalRetryable,
		"execution cancellation signal must be retried",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

type CancelRunCommand struct {
	CommandID, RunID     string
	ExpectedStatus       domainexecution.RunStatus
	ExpectedRevision, At int64
}
type AbortRunCommand struct {
	CommandID, RunID     string
	ExpectedRevision, At int64
	Fence                domainexecution.WorkerFence
}
type ReorderQueueCommand struct {
	CommandID, ScopeID string
	ExpectedRevision   int64
	RunIDs             []string
}

type RunCommandResult struct {
	Run            domainexecution.Run
	Revision       int64
	WasApplied     bool
	SignalRequired bool
}
type ReorderQueueResult struct {
	ScopeID    string
	Revision   int64
	RunIDs     []string
	WasApplied bool
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

// QueueCommandStore atomically applies a queue revision CAS. RunIDs must be the
// full exact permutation of all currently unclaimed QUEUED Runs in ScopeID; the
// store rejects duplicates, omissions, foreign, claimed, or nonqueued members.
type QueueCommandStore interface {
	Reorder(context.Context, ReorderQueueCommand) (ReorderQueueResult, error)
}

type RunCancellationSignaler interface {
	SignalRunCancellation(context.Context, string) error
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
		return RunCommandResult{}, fmt.Errorf("cancel run transaction: %w", err)
	}
	if err := validateRunResult(command.RunID, domainexecution.Canceled, command.ExpectedRevision, result); err != nil {
		return RunCommandResult{}, fmt.Errorf("cancel run authoritative result: %w", err)
	}
	shouldSignal := command.ExpectedStatus == domainexecution.Running
	if result.SignalRequired != shouldSignal {
		return RunCommandResult{}, &RunAdapterContractError{Operation: "validate Host cancellation requirement", Cause: errors.New("unexpected SignalRequired value")}
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
	if strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.RunID) == "" || command.ExpectedRevision < 0 || command.At <= 0 || command.Fence.RunID != command.RunID {
		return RunCommandResult{}, errors.New("invalid abort run command")
	}
	if err := command.Fence.Validate(); err != nil {
		return RunCommandResult{}, fmt.Errorf("invalid abort run command: %w", err)
	}
	if isNilPort(s.store) {
		return RunCommandResult{}, schedulingDependencyRequiredError()
	}
	result, err := s.store.Abort(ctx, command)
	if err != nil {
		return RunCommandResult{}, fmt.Errorf("abort run transaction: %w", err)
	}
	if err := validateRunResult(command.RunID, domainexecution.Aborted, command.ExpectedRevision, result); err != nil {
		return RunCommandResult{}, fmt.Errorf("abort run authoritative result: %w", err)
	}
	if !result.SignalRequired {
		return RunCommandResult{}, &RunAdapterContractError{Operation: "validate Host cancellation requirement", Cause: errors.New("abort must require cancellation signal")}
	}
	return signalIfRequired(ctx, s.signaler, result)
}

func signalIfRequired(ctx context.Context, signaler RunCancellationSignaler, result RunCommandResult) (RunCommandResult, error) {
	if !result.SignalRequired {
		return result, nil
	}
	if signaler == nil {
		return result, runSignalRetryableError(errors.New("cancellation signaler is unavailable"))
	}
	if err := signaler.SignalRunCancellation(ctx, result.Run.ID); err != nil {
		return result, runSignalRetryableError(err)
	}
	return result, nil
}

func validateCancel(command CancelRunCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.RunID) == "" || command.ExpectedRevision < 0 || command.At <= 0 || (command.ExpectedStatus != domainexecution.Queued && command.ExpectedStatus != domainexecution.Running) {
		return errors.New("invalid cancel run command")
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
	ownedCommand.RunIDs = append([]string(nil), command.RunIDs...)
	result, err := s.store.Reorder(ctx, ownedCommand)
	if err != nil {
		return ReorderQueueResult{}, fmt.Errorf("reorder queue transaction: %w", err)
	}
	if result.ScopeID != command.ScopeID || result.Revision != command.ExpectedRevision+1 || len(result.RunIDs) != len(command.RunIDs) {
		return ReorderQueueResult{}, errors.New("invalid reorder queue authoritative result")
	}
	for index := range command.RunIDs {
		if result.RunIDs[index] != command.RunIDs[index] {
			return ReorderQueueResult{}, errors.New("invalid reorder queue authoritative result")
		}
	}
	result.RunIDs = append([]string(nil), result.RunIDs...)
	return result, nil
}
func validateRunResult(runID string, status domainexecution.RunStatus, expectedRevision int64, result RunCommandResult) error {
	if result.Run.ID != runID {
		return &RunAdapterContractError{Operation: "validate run identity", Cause: &RunIdentityConflictError{RunID: runID}}
	}
	if result.Run.Status != status {
		return &RunAdapterContractError{Operation: "validate run status", Cause: &RunStatusConflictError{RunID: runID, Expected: status, Actual: result.Run.Status}}
	}
	if result.Revision != expectedRevision+1 {
		return &RunAdapterContractError{Operation: "validate run revision", Cause: &RunRevisionConflictError{RunID: runID, Expected: expectedRevision + 1, Actual: result.Revision}}
	}
	if err := domainexecution.ValidateRun(result.Run); err != nil {
		return &RunAdapterContractError{Operation: "validate run lifecycle", Cause: err}
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
	owned.RunIDs = append([]string(nil), command.RunIDs...)
	return canonicalDigest(owned)
}

func validateReorder(command ReorderQueueCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.ScopeID) == "" || command.ExpectedRevision < 0 || len(command.RunIDs) == 0 {
		return errors.New("invalid reorder queue command")
	}
	seen := make(map[string]struct{}, len(command.RunIDs))
	for _, id := range command.RunIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("invalid reorder queue command")
		}
		if _, ok := seen[id]; ok {
			return &QueueMembershipConflictError{ScopeID: command.ScopeID}
		}
		seen[id] = struct{}{}
	}
	return nil
}
