package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"math"
	"strings"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const (
	// CodeInstanceCommandIdentityConflict 表示命令 ID 已对应不同的实例请求。
	CodeInstanceCommandIdentityConflict fault.Code = "EXECUTION_INSTANCE_COMMAND_IDENTITY_CONFLICT"
	// CodeInstanceIdentityConflict 表示返回实例 ID 与请求权威状态不一致。
	CodeInstanceIdentityConflict fault.Code = "EXECUTION_INSTANCE_IDENTITY_CONFLICT"
	// CodeInstanceRevisionConflict 表示实例结果修订号不符合 ExpectedRevision+1。
	CodeInstanceRevisionConflict fault.Code = "EXECUTION_INSTANCE_REVISION_CONFLICT"
	// CodeInstanceStatusConflict 表示实例结果状态不符合命令要求。
	CodeInstanceStatusConflict fault.Code = "EXECUTION_INSTANCE_STATUS_CONFLICT"
	// CodeQueueRevisionConflict 表示队列结果修订号不符合请求的 CAS 后继值。
	CodeQueueRevisionConflict fault.Code = "EXECUTION_QUEUE_REVISION_CONFLICT"
	// CodeQueueMembershipConflict 表示重排请求包含重复、遗漏或不属于队列的成员。
	CodeQueueMembershipConflict fault.Code = "EXECUTION_QUEUE_MEMBERSHIP_CONFLICT"
	// CodeInstanceAdapterContractViolation 表示实例命令适配器返回了不合法的权威结果。
	CodeInstanceAdapterContractViolation fault.Code = "EXECUTION_INSTANCE_COMMAND_ADAPTER_CONTRACT_VIOLATION"
	// CodeCancelInstanceCommandInvalid 表示取消实例命令的字段或修订号无效。
	CodeCancelInstanceCommandInvalid fault.Code = "EXECUTION_CANCEL_INSTANCE_COMMAND_INVALID"
	// CodeAbortInstanceCommandInvalid 表示中止实例命令的字段、修订号或 fence 无效。
	CodeAbortInstanceCommandInvalid fault.Code = "EXECUTION_ABORT_INSTANCE_COMMAND_INVALID"
	// CodeReorderQueueCommandInvalid 表示队列重排命令的字段或修订号无效。
	CodeReorderQueueCommandInvalid fault.Code = "EXECUTION_REORDER_QUEUE_COMMAND_INVALID"
)

// MaxExpectedRevision 是命令允许声明已观察到的最大修订号。
//
// 每条命令的结果都必须校验为 ExpectedRevision+1。若允许 MaxInt64，该加法会
// 溢出为 MinInt64（适配器不可能返回的值），乐观并发检查便失去作用：命令先执行、
// 存储先变更，随后写入才被报告为适配器契约违规，而不是它本应得到的无效参数错误。
// 保留顶值可使后继值运算始终可表示，因此上限是 MaxInt64-1 而不是 MaxInt64。
const MaxExpectedRevision int64 = math.MaxInt64 - 1

// representableRevision 判断修订号是否非负且其后继值仍可表示。
func representableRevision(revision int64) bool {
	return revision >= 0 && revision <= MaxExpectedRevision
}

// cancelInstanceCommandInvalidError 构造取消实例命令无效的错误。
func cancelInstanceCommandInvalidError(cause error) error {
	return newInstanceCommandWrappedFault(cause, fault.InvalidArgument, CodeCancelInstanceCommandInvalid, "cancel instance command is invalid")
}

// abortInstanceCommandInvalidError 构造中止实例命令无效的错误。
func abortInstanceCommandInvalidError(cause error) error {
	return newInstanceCommandWrappedFault(cause, fault.InvalidArgument, CodeAbortInstanceCommandInvalid, "abort instance command is invalid")
}

// reorderQueueCommandInvalidError 构造队列重排命令无效的错误。
func reorderQueueCommandInvalidError(cause error) error {
	return newInstanceCommandWrappedFault(cause, fault.InvalidArgument, CodeReorderQueueCommandInvalid, "reorder queue command is invalid")
}

// runCommandConflictError 构造实例命令身份冲突错误。
func runCommandConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceCommandIdentityConflict, "instance command identity conflicts with an existing request")
}

// runIdentityConflictError 构造实例 ID 与权威状态不一致的错误。
func runIdentityConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceIdentityConflict, "instance identity conflicts with the authoritative state")
}

// runRevisionConflictError 构造实例修订号冲突错误。
func runRevisionConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceRevisionConflict, "instance revision conflicts with current state")
}

// runStatusConflictError 构造实例状态冲突错误。
func runStatusConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeInstanceStatusConflict, "instance status conflicts with current state")
}

// queueRevisionConflictError 构造队列修订号冲突错误。
func queueRevisionConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeQueueRevisionConflict, "queue revision conflicts with current state")
}

// queueMembershipConflictError 构造队列成员关系冲突错误。
func queueMembershipConflictError() error {
	return newInstanceCommandFault(fault.Conflict, CodeQueueMembershipConflict, "queue membership conflicts with the authoritative state")
}

// runAdapterContractViolationError 将适配器返回结果包装为契约违规错误。
func runAdapterContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.Internal, CodeInstanceAdapterContractViolation, "instance command adapter returned an invalid authoritative result")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// newInstanceCommandWrappedFault 创建带原因链的实例命令领域错误。
func newInstanceCommandWrappedFault(cause error, kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// newInstanceCommandFault 创建不带原因的实例命令领域错误。
func newInstanceCommandFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CodeInstanceSignalRetryable 表示取消信号发送失败，调用方应重试而不是撤销已提交状态。
const CodeInstanceSignalRetryable fault.Code = "EXECUTION_INSTANCE_SIGNAL_RETRYABLE"

// runSignalRetryableError 构造取消信号需要重试的不可用错误。
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

// CancelInstanceCommand 携带取消实例所需的幂等命令身份、预期状态、CAS 修订号和调用时间。
type CancelInstanceCommand struct {
	CommandID            string
	InstanceID           domainexecution.InstanceID
	ExpectedStatus       domainexecution.InstanceStatus
	ExpectedRevision, At int64
}

// AbortInstanceCommand 携带中止实例所需的幂等命令身份、CAS 修订号、调用时间和 worker fence。
type AbortInstanceCommand struct {
	CommandID            string
	InstanceID           domainexecution.InstanceID
	ExpectedRevision, At int64
	Fence                domainexecution.WorkerFence
}

// ReorderQueueCommand 携带队列范围、CAS 修订号以及完整的未领取队列成员排列。
type ReorderQueueCommand struct {
	CommandID, ScopeID string
	ExpectedRevision   int64
	InstanceIDs        []string
}

// InstanceCommandResult 保存实例命令提交后的权威实例、修订号、应用标记和是否需要发送取消信号。
type InstanceCommandResult struct {
	Run            domainexecution.Instance
	Revision       int64
	WasApplied     bool
	SignalRequired bool
}

// ReorderQueueResult 保存队列重排提交后的范围、修订号、成员排列和应用标记。
type ReorderQueueResult struct {
	ScopeID     string
	Revision    int64
	InstanceIDs []string
	WasApplied  bool
}

// InstanceCommandStore 必须规范化 CommandID 负载并原子执行每个方法。Cancel 移除排队
// Run，或迁移运行中 Run 并使其 fence 失效；Abort 仅接受 RUNNING，迁移为 ABORTED 并在
// 返回前使提供的 fence 失效。返回值必须是权威的已提交或重放结果，即使提交结果未知也如此。
type InstanceCommandStore interface {
	// Cancel 原子地取消排队实例，或迁移运行中实例并使其 fence 失效；返回权威提交或重放结果。
	Cancel(context.Context, CancelInstanceCommand) (InstanceCommandResult, error)
	// Abort 仅接受运行中实例，将其迁移为 ABORTED，并使提供的 fence 失效后返回权威结果。
	Abort(context.Context, AbortInstanceCommand) (InstanceCommandResult, error)
}

// QueueCommandStore 原子应用队列修订号 CAS。InstanceIDs 必须是 ScopeID 中当前所有未领取
// QUEUED Run 的完整精确排列；存储必须拒绝重复、遗漏、外部、已领取或非排队成员。
type QueueCommandStore interface {
	// Reorder 以队列修订号 CAS 原子应用完整的未领取 QUEUED 实例排列。
	Reorder(context.Context, ReorderQueueCommand) (ReorderQueueResult, error)
}

// InstanceCancellationSignaler 将需要取消的实例通知宿主执行器。
type InstanceCancellationSignaler interface {
	// SignalInstanceCancellation 请求宿主向指定实例发送取消信号。
	SignalInstanceCancellation(context.Context, domainexecution.InstanceID) error
}

// CancelInstanceService 校验并执行取消实例命令，再按权威结果发送必要的取消信号。
type CancelInstanceService struct {
	store    InstanceCommandStore
	signaler InstanceCancellationSignaler
}

// NewCancelInstanceService 创建使用给定存储端口和取消信号端口的取消服务。
func NewCancelInstanceService(store InstanceCommandStore, signaler InstanceCancellationSignaler) CancelInstanceService {
	return CancelInstanceService{store: store, signaler: signaler}
}

// CancelInstance 校验命令、调用原子存储操作，并验证返回结果的实例身份、状态、修订号和信号标记。
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
		// validateInstanceResult 已返回 EXECUTION_INSTANCE_COMMAND_ADAPTER_CONTRACT_VIOLATION。
		return InstanceCommandResult{}, err
	}
	shouldSignal := command.ExpectedStatus == domainexecution.Running
	if result.SignalRequired != shouldSignal {
		return InstanceCommandResult{}, runAdapterContractViolationError(errors.New("unexpected SignalRequired value"))
	}
	return signalIfRequired(ctx, s.signaler, result)
}

// AbortInstanceService 校验并执行中止实例命令，再发送必需的取消信号。
type AbortInstanceService struct {
	store    InstanceCommandStore
	signaler InstanceCancellationSignaler
}

// NewAbortInstanceService 创建使用给定存储端口和取消信号端口的中止服务。
func NewAbortInstanceService(store InstanceCommandStore, signaler InstanceCancellationSignaler) AbortInstanceService {
	return AbortInstanceService{store: store, signaler: signaler}
}

// AbortInstance 校验命令、调用原子存储操作，并验证返回结果必须是 ABORTED 且要求发送信号。
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

// signalIfRequired 在权威结果要求时发送取消信号；信号端口缺失或失败均返回可重试错误。
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

// validateCancel 校验取消命令身份、实例 ID、可表示修订号、调用时间和预期状态。
func validateCancel(command CancelInstanceCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || command.InstanceID.Validate() != nil || !representableRevision(command.ExpectedRevision) || command.At <= 0 || (command.ExpectedStatus != domainexecution.Queued && command.ExpectedStatus != domainexecution.Running) {
		return cancelInstanceCommandInvalidError(nil)
	}
	return nil
}

// validateAbort 校验中止命令身份、实例 ID、可表示修订号、调用时间及 fence 归属和内容。
func validateAbort(command AbortInstanceCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || command.InstanceID.Validate() != nil || !representableRevision(command.ExpectedRevision) || command.At <= 0 || command.Fence.InstanceID != command.InstanceID {
		return abortInstanceCommandInvalidError(nil)
	}
	if err := command.Fence.Validate(); err != nil {
		return abortInstanceCommandInvalidError(err)
	}
	return nil
}

// ReorderQueueService 校验并执行队列成员排列命令。
type ReorderQueueService struct{ store QueueCommandStore }

// NewReorderQueueService 创建使用给定队列存储端口的重排服务。
func NewReorderQueueService(store QueueCommandStore) ReorderQueueService {
	return ReorderQueueService{store: store}
}

// ReorderQueue 校验命令、复制成员排列后执行队列 CAS，并验证返回范围、修订号及顺序。
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

// validateInstanceResult 验证适配器返回的实例身份、目标状态、CAS 后继修订号和领域不变量。
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

// 以下摘要按字段写入，而不是通过 json.Marshal 生成。反射路径会静默丢弃不可见字段：
// 执行坐标是唯一字段未导出的结构体，因此 json.Marshal 会把 InstanceID 编码为 {}，
// 共享同一命令 ID 的两个不同实例取消请求会得到相同摘要。过程没有报错，只是重放检查
// 失去了区分它们的能力。
//
// 这三个字符串是线协议标签，不是 Go 名称；它们是持久化幂等摘要的域分隔前缀，后续
// 重命名不得修改。它们由下面的逐字段重写引入，并改变了此前存储的每条取消、中止和
// 重排记录的摘要；该破坏及其状态见 docs/contracts/digest-wire-tags.md。
//
// 逐字段写入还使摘要独立于 Go 名称，因此后续重命名不会改变幂等记录所使用的键值。
const (
	// cancelInstanceRequestDigestV1 是取消实例请求摘要的稳定线协议标签。
	cancelInstanceRequestDigestV1 = "cancel-instance-request-v1"
	// abortInstanceRequestDigestV1 是中止实例请求摘要的稳定线协议标签。
	abortInstanceRequestDigestV1 = "abort-instance-request-v1"
	// reorderQueueRequestDigestV1 是队列重排请求摘要的稳定线协议标签。
	reorderQueueRequestDigestV1 = "reorder-queue-request-v1"
)

// finishDigest 将已写入字段的哈希值编码为带 sha256 前缀的摘要字符串。
func finishDigest(h hash.Hash) (string, error) {
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// CancelInstanceRequestDigest 按稳定字段顺序计算取消实例命令的幂等请求摘要。
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

// AbortInstanceRequestDigest 按稳定字段顺序计算中止实例命令的幂等请求摘要，并纳入 fence 身份。
func AbortInstanceRequestDigest(command AbortInstanceCommand) (string, error) {
	h := sha256.New()
	writeDigestString(h, abortInstanceRequestDigestV1)
	writeDigestString(h, command.CommandID)
	writeDigestString(h, command.InstanceID.String())
	writeDigestUint64(h, uint64(command.ExpectedRevision))
	writeDigestUint64(h, uint64(command.At))
	// fence 标识允许中止的 worker，因此仅 fence 不同的两个中止请求也是不同请求。
	writeDigestString(h, command.Fence.InstanceID.String())
	writeDigestString(h, command.Fence.ClaimToken)
	return finishDigest(h)
}

// ReorderQueueRequestDigest 按稳定字段顺序计算队列重排命令的幂等请求摘要及成员顺序。
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

// validateReorder 校验重排命令字段，并拒绝成员 ID 为空或重复的请求。
func validateReorder(command ReorderQueueCommand) error {
	if strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.ScopeID) == "" || !representableRevision(command.ExpectedRevision) || len(command.InstanceIDs) == 0 {
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
