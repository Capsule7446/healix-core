package scheduling

import (
	"context"
	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// CreateInstanceCommand 是调用方提供创建数据的唯一权威来源。
type CreateInstanceCommand struct {
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

// ResolvedCreateInstance 仅包含从同一一致目录视图读取的资产。
type ResolvedCreateInstance struct {
	Plan        automation.ResolvedExecutionFlow
	Environment automation.Environment
	Invocations []execution.InvocationScopeSnapshot
}

// CreateInstanceResult 保存创建实例返回的运行、封存快照、入口 ID 和是否本次应用。
type CreateInstanceResult struct {
	Run        execution.Instance
	Snapshot   execution.InstanceSnapshot
	EntryIDs   []execution.EntryID
	WasApplied bool
}

// StoredCreateInstanceResult 保存事务中持久化的运行、快照摘要和入口 ID。
type StoredCreateInstanceResult struct {
	Run            execution.Instance
	Snapshot       execution.InstanceSnapshot
	SnapshotDigest string
	EntryIDs       []execution.EntryID
}

// StoredCreateInstanceCommand 保存命令 ID、请求摘要及权威创建结果。
type StoredCreateInstanceCommand struct {
	CommandID     string
	RequestDigest string
	Result        StoredCreateInstanceResult
}

// InsertCreateInstanceStatus 表示实例插入是首次应用还是幂等重放。
type InsertCreateInstanceStatus string

const (
	// InsertCreateInstanceApplied 表示本次事务首次插入实例。
	InsertCreateInstanceApplied InsertCreateInstanceStatus = "APPLIED"
	// InsertCreateInstanceReplayed 表示相同请求已插入，本次未改变状态。
	InsertCreateInstanceReplayed InsertCreateInstanceStatus = "REPLAYED"
)

// InsertCreateInstanceOutcome 保存插入状态、命令身份、请求摘要和权威结果。
type InsertCreateInstanceOutcome struct {
	Status        InsertCreateInstanceStatus
	CommandID     string
	RequestDigest string
	Result        StoredCreateInstanceResult
}

// CreateInstanceIntent 携带原子插入实例所需的命令身份、运行、封存快照和入口列表。
type CreateInstanceIntent struct {
	CommandID     string
	RequestDigest string
	Run           execution.Instance
	Snapshot      execution.InstanceSnapshot
	Entries       []execution.Entry
}

// CodeCreateInstanceCommandInvalid 表示创建命令形状或预算无效。
const CodeCreateInstanceCommandInvalid fault.Code = "EXECUTION_CREATE_INSTANCE_COMMAND_INVALID"

// createInstanceCommandInvalidError 构造创建命令无效的调用方错误。
func createInstanceCommandInvalidError(cause error) error {
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

// CodeCreateInstanceCommandConflict 表示命令 ID 已对应不同请求摘要。
const CodeCreateInstanceCommandConflict fault.Code = "EXECUTION_CREATE_INSTANCE_COMMAND_CONFLICT"

// createInstanceCommandConflictError 构造命令身份冲突错误。
func createInstanceCommandConflictError() error {
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

// CodeCreateInstanceSnapshotConflict 表示创建快照与权威实例状态冲突。
const CodeCreateInstanceSnapshotConflict fault.Code = "EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT"

// createInstanceSnapshotConflictError 构造创建快照冲突错误。
func createInstanceSnapshotConflictError() error {
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

// CodeCreateInstanceAdapterContractViolation 表示适配器返回非法权威创建结果。
const CodeCreateInstanceAdapterContractViolation fault.Code = "EXECUTION_CREATE_INSTANCE_ADAPTER_CONTRACT_VIOLATION"

// createInstanceAdapterContractViolationError 构造适配器契约违规内部错误。
func createInstanceAdapterContractViolationError(cause error) error {
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

// CodeCreateInstanceRetryable 表示创建结果暂时不可用，可安全重试。
const CodeCreateInstanceRetryable fault.Code = "EXECUTION_CREATE_INSTANCE_RETRYABLE"

// createInstanceRetryableError 构造可重试的暂时不可用错误。
func createInstanceRetryableError(cause error) error {
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

// CodeCreateInstanceCatalogGraphUnresolvable 表示目录图无法解析或不可用。
const CodeCreateInstanceCatalogGraphUnresolvable fault.Code = "EXECUTION_CREATE_INSTANCE_CATALOG_GRAPH_UNRESOLVABLE"

// classifyCatalogGraphFailure 为未分类目录图失败补上注册错误码，并让已分类错误原样通过，避免嵌套
// 校验器的错误码被第二层包装掩盖。
func classifyCatalogGraphFailure(cause error) error {
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return createInstanceCatalogGraphUnresolvableError(cause)
}

// createInstanceCatalogGraphUnresolvableError 构造目录图不可解析的前置条件错误。
func createInstanceCatalogGraphUnresolvableError(cause error) error {
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

// CreateInstanceStore 定义围绕创建实例事务执行操作的存储端口。
type CreateInstanceStore interface {
	// InTransaction 在单一事务中执行创建实例读写。
	InTransaction(context.Context, func(CreateInstanceTx) error) error
}

// CreateInstanceTx 定义查找命令、解析一致目录视图和原子插入实例的事务端口。
type CreateInstanceTx interface {
	// FindCommand 按命令 ID 查找已存储命令；未找到时返回 found=false。
	FindCommand(context.Context, string) (StoredCreateInstanceCommand, bool, error)
	// ResolveCreateInstance 必须从同一事务视图解析任务、递归工作流 LATEST/current 指针、节点、环境、
	// 绑定、默认值和具体调用；返回前必须校验循环、缺失指针及执行深度/调用/引用/值限制。
	ResolveCreateInstance(context.Context, CreateInstanceCommand) (ResolvedCreateInstance, error)
	// InsertCreateInstance 原子存储命令、排队 Run、精确入口、封存快照及队列成员关系/顺序；返回结果
	// 是权威结果。
	InsertCreateInstance(context.Context, CreateInstanceIntent) (InsertCreateInstanceOutcome, error)
}
