package scheduling

import (
	"context"
	"errors"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
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

const CodeCreateRunCommandInvalid fault.Code = "EXECUTION_CREATE_RUN_COMMAND_INVALID"

func createRunCommandInvalidError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.InvalidArgument,
		CodeCreateRunCommandInvalid,
		"create-run command is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateRunCommandConflict fault.Code = "EXECUTION_CREATE_RUN_COMMAND_CONFLICT"

func createRunCommandConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeCreateRunCommandConflict,
		"create-run command conflicts with an existing request",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateRunSnapshotConflict fault.Code = "EXECUTION_CREATE_RUN_SNAPSHOT_CONFLICT"

func createRunSnapshotConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeCreateRunSnapshotConflict,
		"create-run snapshot conflicts with the authoritative run",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateRunAdapterContractViolation fault.Code = "EXECUTION_CREATE_RUN_ADAPTER_CONTRACT_VIOLATION"

func createRunAdapterContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Internal,
		CodeCreateRunAdapterContractViolation,
		"create-run adapter returned an invalid authoritative result",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
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

var (
	ErrCreateRunCatalogGraph = errors.New("create-run catalog graph not found or invalid")
	ErrCreateRunRetryable    = errors.New("retryable create-run transaction or catalog conflict")
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
