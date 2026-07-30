package scheduling

import (
	"context"
	"errors"
	"github.com/Capsule7446/healix-core/domain/parameter"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
)

// CreateRunCommand is the sole authority for caller-supplied create data.
type CreateRunCommand struct {
	CommandID         string
	RunID             string
	ExecutionFlowID   string
	TestTaskVersionID string
	EnvironmentID     string
	Entries           map[string]map[string]parameter.Value
	FailurePolicy     execution.FailurePolicy
	CreatedAt         int64
	ScreenshotPolicy  execution.ScreenshotPolicySnapshot
	HealerPolicy      execution.HealerPolicySnapshot
}

// ResolvedCreateRun contains only assets read from one consistent catalog view.
type ResolvedCreateRun struct {
	Plan        automation.ResolvedExecutionFlow
	Environment automation.Environment
	Invocations []execution.InvocationScopeSnapshot
}

type CreateRunResult struct {
	Run        execution.Run
	Snapshot   execution.RunSnapshot
	EntryIDs   []string
	WasApplied bool
}

type StoredCreateRunResult struct {
	Run            execution.Run
	Snapshot       execution.RunSnapshot
	SnapshotDigest string
	EntryIDs       []string
}

type StoredCreateRunCommand struct {
	CommandID     string
	RequestDigest string
	Result        StoredCreateRunResult
}

type InsertCreateRunStatus string

const (
	InsertCreateRunApplied  InsertCreateRunStatus = "APPLIED"
	InsertCreateRunReplayed InsertCreateRunStatus = "REPLAYED"
)

type InsertCreateRunOutcome struct {
	Status        InsertCreateRunStatus
	CommandID     string
	RequestDigest string
	Result        StoredCreateRunResult
}

type CreateRunIntent struct {
	CommandID     string
	RequestDigest string
	Run           execution.Run
	Snapshot      execution.RunSnapshot
	Entries       []execution.WorkflowEntry
}

type CreateRunAdapterContractError struct {
	Operation string
	Reason    string
}

func (e *CreateRunAdapterContractError) Error() string {
	return "create-run adapter contract violation: " + e.Operation + ": " + e.Reason
}
func (e *CreateRunAdapterContractError) Is(target error) bool {
	return target == ErrCreateRunAdapterContract
}

type CreateRunCatalogGraphError struct {
	Operation string
	Cause     error
}

func (e *CreateRunCatalogGraphError) Error() string {
	if e.Cause == nil {
		return "create-run catalog graph not found or invalid: " + e.Operation
	}
	return "create-run catalog graph not found or invalid: " + e.Operation + ": " + e.Cause.Error()
}
func (e *CreateRunCatalogGraphError) Is(target error) bool { return target == ErrCreateRunCatalogGraph }
func (e *CreateRunCatalogGraphError) Unwrap() error        { return e.Cause }

type CreateRunRetryableError struct {
	Operation string
	Cause     error
}

func (e *CreateRunRetryableError) Error() string {
	if e.Cause == nil {
		return "retryable create-run transaction or catalog conflict: " + e.Operation
	}
	return "retryable create-run transaction or catalog conflict: " + e.Operation + ": " + e.Cause.Error()
}
func (e *CreateRunRetryableError) Is(target error) bool { return target == ErrCreateRunRetryable }
func (e *CreateRunRetryableError) Unwrap() error        { return e.Cause }

type CreateRunCommandConflictError struct{ CommandID string }

func (e *CreateRunCommandConflictError) Error() string {
	return "create-run command identity conflict: " + e.CommandID
}
func (e *CreateRunCommandConflictError) Is(target error) bool {
	return target == ErrCreateRunCommandConflict
}

type CreateRunSnapshotConflictError struct{ RunID string }

func (e *CreateRunSnapshotConflictError) Error() string {
	return "immutable run identity or snapshot conflict: " + e.RunID
}
func (e *CreateRunSnapshotConflictError) Is(target error) bool {
	return target == ErrCreateRunSnapshotConflict
}

var (
	ErrInvalidCreateRunCommand   = errors.New("invalid create-run command")
	ErrCreateRunCatalogGraph     = errors.New("create-run catalog graph not found or invalid")
	ErrCreateRunRetryable        = errors.New("retryable create-run transaction or catalog conflict")
	ErrCreateRunCommandConflict  = errors.New("create-run command identity conflict")
	ErrCreateRunSnapshotConflict = errors.New("immutable run identity or snapshot conflict")
	ErrCreateRunAdapterContract  = errors.New("create-run adapter contract violation")
)

type CreateRunStore interface {
	InTransaction(context.Context, func(CreateRunTx) error) error
}

type CreateRunTx interface {
	FindCommand(context.Context, string) (StoredCreateRunCommand, bool, error)
	// ResolveCreateRun must resolve task, recursive workflow LATEST/current pointers,
	// nodes, environment, bindings, defaults, and concrete invocations from one
	// transaction view. It must validate cycles, missing pointers, and execution
	// depth/invocation/reference/value limits before returning.
	ResolveCreateRun(context.Context, CreateRunCommand) (ResolvedCreateRun, error)
	// InsertCreateRun atomically stores the command, queued Run, exact entries,
	// sealed snapshot, and queue membership/order. Its outcome is authoritative.
	InsertCreateRun(context.Context, CreateRunIntent) (InsertCreateRunOutcome, error)
}
