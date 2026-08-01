package scheduling

import (
	"context"
	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// CreateRunCommand is the sole authority for caller-supplied create data.
type CreateRunCommand struct {
	CommandID         string
	InstanceID        execution.InstanceID
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
	Run        execution.Instance
	Snapshot   execution.InstanceSnapshot
	EntryIDs   []execution.EntryID
	WasApplied bool
}

type StoredCreateRunResult struct {
	Run            execution.Instance
	Snapshot       execution.InstanceSnapshot
	SnapshotDigest string
	EntryIDs       []execution.EntryID
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
	Run           execution.Instance
	Snapshot      execution.InstanceSnapshot
	Entries       []execution.Entry
}

const CodeCreateInstanceCommandInvalid fault.Code = "EXECUTION_CREATE_INSTANCE_COMMAND_INVALID"

func createRunCommandInvalidError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.InvalidArgument,
		CodeCreateInstanceCommandInvalid,
		"create-instance command is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateInstanceCommandConflict fault.Code = "EXECUTION_CREATE_INSTANCE_COMMAND_CONFLICT"

func createRunCommandConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeCreateInstanceCommandConflict,
		"create-instance command conflicts with an existing request",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateInstanceSnapshotConflict fault.Code = "EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT"

func createRunSnapshotConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeCreateInstanceSnapshotConflict,
		"create-instance snapshot conflicts with the authoritative instance",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateInstanceAdapterContractViolation fault.Code = "EXECUTION_CREATE_INSTANCE_ADAPTER_CONTRACT_VIOLATION"

func createRunAdapterContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Internal,
		CodeCreateInstanceAdapterContractViolation,
		"create-instance adapter returned an invalid authoritative result",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateInstanceRetryable fault.Code = "EXECUTION_CREATE_INSTANCE_RETRYABLE"

func createRunRetryableError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Unavailable,
		CodeCreateInstanceRetryable,
		"create-instance outcome is temporarily unavailable",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeCreateInstanceCatalogGraphUnresolvable fault.Code = "EXECUTION_CREATE_INSTANCE_CATALOG_GRAPH_UNRESOLVABLE"

// classifyCatalogGraphFailure gives an unclassified catalog-graph failure its
// registered code, and lets an already-classified one through unchanged. The
// distinction matters as the domain packages migrate: once a nested validator
// starts returning its own code, wrapping it again here would bury that code
// under a second one and force the host to unwrap before it could classify.
func classifyCatalogGraphFailure(cause error) error {
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return createRunCatalogGraphUnresolvableError(cause)
}

func createRunCatalogGraphUnresolvableError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.FailedPrecondition,
		CodeCreateInstanceCatalogGraphUnresolvable,
		"create-instance catalog graph is unavailable or invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

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
