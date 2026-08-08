package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// requestAbortDigestV1 是中止请求摘要的不可变线格式标签。修改这些字节会改变宿主已持久化的所有摘要，
// 使重试的中止变成第二次应用。新增形状必须使用新标签，不得编辑此标签。
const requestAbortDigestV1 = "request-abort-v1"

// requestAbortCommandInvalidError 将命令校验失败包装为带字段违规明细的调用方错误。
func requestAbortCommandInvalidError(cause error, violation fault.Violation) error {
	err, constructionErr := fault.Wrap(cause, fault.InvalidArgument, CodeRequestAbortCommandInvalid, "abort request command is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// requestAbortDigestMismatchError 构造请求意图与命令不一致的错误。
func requestAbortDigestMismatchError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeRequestAbortDigestMismatch, "abort request intent does not match its command", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// RequestAbortUnavailableError 构造事务缺失或不可用时服务返回的错误；宿主据此区分适配器缺失与请求被拒绝。
func RequestAbortUnavailableError() error {
	err, constructionErr := fault.New(fault.Unavailable, CodeRequestAbortUnavailable, "abort request transaction is unavailable")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// RequestAbortIdentityConflictError 构造入口已不再持有命令所观测状态时适配器应返回的错误。
// Core 导出此构造器，使所有适配器使用相同分类的冲突错误。
func RequestAbortIdentityConflictError() error {
	err, constructionErr := fault.New(fault.Conflict, CodeRequestAbortIdentityConflict, "entry state changed before the abort request was recorded")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// requestAbortContractViolationError 构造适配器违反中止请求端口契约的内部错误。
func requestAbortContractViolationError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.Internal, CodeRequestAbortAdapterContractViolation, "abort request adapter violated the port contract", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// RequestAbortCommand 表示一次停止运行中入口的请求，携带身份识别和决策所需的全部信息。
//
// 命令不携带时间戳，原因与 CompleteEntryCommand 相同：墙上时钟会使每次重试的摘要变化，将崩溃重试
// 变成第二次应用。宿主在身份之外自行记录请求时间。
type RequestAbortCommand struct {
	EntryID domainexecution.EntryID
	Fence   domainexecution.WorkerFence
	State   EntryCompletionState
	Request AbortRequest
}

// Validate 校验命令是否具有可生成摘要的有效形状；它不判断是否可以请求中止，该问题由
// DecideAbortRequest 回答，两类失败的处理方式不同。
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

// RequestAbortDigest 生成一次中止请求的稳定身份摘要。
//
// 摘要逐字段写入长度前缀，而不是整体序列化，因此字段顺序、编码器版本或可选字段约定不会改变
// 宿主已持久化摘要对应的字节。无法校验的请求没有摘要：函数返回空字符串及错误，遭拒命令不会
// 以看似合理的身份被记录。
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

// RequestAbortIntent 携带一次中止请求、其身份以及 Core 为其得出的决策。
//
// 决策随意图传递，而不是在适配器内部重新计算，因为宿主绝不能自行推导 NextIntentRevision。
// ValidateRequestAbortIntentDigest 将这一保证转化为适配器每次应用前执行的检查。
type RequestAbortIntent struct {
	EntryID       domainexecution.EntryID
	RequestDigest string
	Command       RequestAbortCommand
	Decision      AbortRequestDecision
}

// ValidateRequestAbortIntentDigest 根据意图自身命令重新计算摘要和决策，并报告任何不一致。
//
// 适配器必须在 RequestAbort 中首先调用它，再接触存储。这是契约两项保证的机械化实现：记录请求
// 所使用的身份必须确实属于正在应用的请求，待写入计数器必须是 Core 产生的计数器。两项检查均为
// 本地操作，不使用 Context、端口或所有权。
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

// RequestAbortStatus 表示中止请求是完成了写入还是发现写入已存在。
type RequestAbortStatus string

const (
	// RequestAbortApplied 表示本次调用记录了待处理中止意图。
	RequestAbortApplied RequestAbortStatus = "APPLIED"
	// RequestAbortReplayed 表示相同请求已应用，本次调用未改变任何内容。
	RequestAbortReplayed RequestAbortStatus = "REPLAYED"
)

// RequestAbortOutcome 保存一次中止请求产生的结果。Decision 是实际记录的决策，因此重放返回原始
// 应用使用的同一答案，而不是重新计算的答案。
type RequestAbortOutcome struct {
	Status        RequestAbortStatus
	EntryID       domainexecution.EntryID
	RequestDigest string
	Decision      AbortRequestDecision
}

// AbortRequestTransaction 是宿主实现的端口，用于记录待处理中止意图。
//
// 原子性边界——以下所有内容必须在一个事务中全部落地，否则一项也不得落地：
//
//   - 待处理终态意图：以 Decision.Next* 值写入，并对其 Current* 值执行 CAS；
//   - 以 Command.Request.AbortPendingCommandID 为键的中止命令收据；
//   - 以 (EntryID, RequestDigest) 为键的幂等收据，最后写入，使事务批次在崩溃时整体保持不可见且可重试。
//
// 此处不得发生以下操作，这正是 D-17 作为独立契约的全部意义：不改变入口状态，不写入事实，
// 不使领取失效，也不将 action gate 终态化。中止请求只记录有人提出请求，不会结束任何事项。
// 终止入口仍是 EntryCompletionTransaction 的唯一职责，使中止与普通完成汇聚到一条终态写入路径。
//
// 顺序是：本事务先提交，入口随后运行到自然停止点，EntryCompletionTransaction.CompleteEntry 再在
// 完成 gate 的同时终止入口。宿主若在此处使领取失效，完成操作的权威 CAS 将指向过期行。
//
// Context 仅用于取消和截止时间；实现不得从中读取业务值。两个方法都不取得传入值的所有权，
// 返回值归调用方所有。
type AbortRequestTransaction interface {
	// LookupAbortRequest 按请求摘要查询已记录的中止结果。命中时必须返回 RequestAbortReplayed，
	// 不得写入；未记录时必须返回 (zero, false, nil)。
	LookupAbortRequest(ctx context.Context, entryID domainexecution.EntryID, requestDigest string) (RequestAbortOutcome, bool, error)
	// RequestAbort 原子记录一个待处理中止意图。
	//
	// 它必须在接触存储前调用 ValidateRequestAbortIntentDigest，必须原样写入 Decision.Next* 字段而
	// 不重新计算修订，并且在事务内发现收据已存在时必须返回 RequestAbortReplayed，不能重复应用。
	//
	// 两类拒绝彼此不同，不得合并，因为调用方的处理方式不同：
	//
	//   - 非本工作线程持有的 fence 以 domainexecution.CodeWorkerFenceStale 拒绝。工作线程已失去领取，
	//     必须停止而不是重试。
	//   - 入口已不再持有命令观测状态时，以 RequestAbortIdentityConflictError 拒绝。领取仍然有效，
	//     调用方重新读取并重建命令。
	//
	// 适配器若对任一情况返回未分类存储错误，宿主将无法区分“放弃”与“重新读取并重试”。
	RequestAbort(ctx context.Context, intent RequestAbortIntent) (RequestAbortOutcome, error)
}

// AbortRequestService 将已校验的中止请求转换为恰好记录一次的待处理中止意图。
//
// 它是访问 AbortRequestTransaction 的唯一支持路径：先决策再写入，使不可决策请求不会到达存储；
// 同时将适配器返回的每个结果与产生它的请求逐一校验。使用 NewAbortRequestService 构造。
// 零值不含事务，每次调用都返回 CodeRequestAbortUnavailable，而不会 panic。
type AbortRequestService struct {
	transaction AbortRequestTransaction
}

// NewAbortRequestService 将服务连接到一个事务端口。
//
// 它接受 nil 或 typed-nil 事务，不在构造时拒绝。由配置映射装配服务的组合根应在真正需要缺失
// 适配器的调用处失败并指出适配器，而不是启动时在没有请求上下文的情况下失败。服务不拥有事务，
// 也不会关闭事务。
func NewAbortRequestService(transaction AbortRequestTransaction) AbortRequestService {
	return AbortRequestService{transaction: transaction}
}

// Request 恰好记录一次待处理中止意图。
//
// 顺序与 EntryCompletionService.Complete 对应：先生成摘要（同时校验命令），再决策，再查找已有
// 收据，最后应用。先决策再接触适配器意味着 Core 会拒绝的请求——非 RUNNING 入口、正在中止的入口、
// 修订耗尽——不会产生存储往返，也不会留下部分痕迹。重放相同命令会返回 RequestAbortReplayed
// 的已记录结果，不应用任何变更。
//
// Context 原样传给适配器。返回结果归调用方所有；拒绝调用时与错误一同返回零结果。
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
	// APPLIED 和 REPLAYED 在此处都合法：并发工作线程可能在查询与应用之间提交了相同请求，
	// 适配器在事务内发现自身收据时也正是在执行契约要求的行为。
	if applied.Status != RequestAbortApplied && applied.Status != RequestAbortReplayed {
		return RequestAbortOutcome{}, requestAbortContractViolationError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "status", "abort request status is not one core defined"))
	}
	if err := validateRequestAbortOutcome(applied, command.EntryID, digest, decision); err != nil {
		return RequestAbortOutcome{}, err
	}
	return applied, nil
}

// validateRequestAbortOutcome 强制适配器结果与传入请求一致。结果若指向不同入口、不同请求，
// 或携带适配器自行重新计算的决策，属于调用方不得据此行动的缺陷，因此直接拒绝而不带警告返回。
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
