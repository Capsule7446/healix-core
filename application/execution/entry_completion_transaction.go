package execution

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// completeEntryRequestDigestV1 是入口完成请求摘要的不可变线格式标签。修改这些字节会改变宿主已持久化
// 的所有摘要，使所有进行中的完成请求变成第二次应用。新增形状必须使用新标签，不得编辑此标签。
const completeEntryRequestDigestV1 = "complete-entry-request-v1"

// completeEntryCommandInvalidError 将完成命令校验失败包装为带字段违规明细的调用方错误。
func completeEntryCommandInvalidError(cause error, violation fault.Violation) error {
	err, constructionErr := fault.Wrap(cause, fault.InvalidArgument, CodeCompleteEntryCommandInvalid, "complete entry command is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// completeEntryDigestMismatchError 构造完成意图与命令不一致的错误。
func completeEntryDigestMismatchError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeCompleteEntryDigestMismatch, "complete entry intent does not match its command", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompleteEntryUnavailableError 构造未接入入口完成事务时的错误。宿主组合根无法提供适配器时使用它，
// 使双方以同一方式报告缺失依赖。
func CompleteEntryUnavailableError() error {
	err, constructionErr := fault.New(fault.Unavailable, CodeCompleteEntryUnavailable, "entry completion transaction is unavailable")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompleteEntryIdentityConflictError 构造入口不再持有命令观测状态时的错误。适配器在 CompleteEntry
// 的 CAS 未匹配任何行时返回它，表示其他写入者先到达，调用方必须重新读取后重试。
func CompleteEntryIdentityConflictError() error {
	err, constructionErr := fault.New(fault.Conflict, CodeCompleteEntryIdentityConflict, "entry completion observed a stale state")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// completeEntryContractViolationError 构造适配器违反入口完成端口契约的内部错误。
func completeEntryContractViolationError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.Internal, CodeCompleteEntryAdapterContractViolation, "entry completion adapter violated its contract", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// CompleteEntryCommand 表示一次结束入口的请求。
//
// State 和 Outcome 是完整决策依据；Core 仅根据它们推导终态答案。AbortPendingCommandID 是仍在等待
// 收据的中止命令身份，放在此处而不放在 State 中：它是幂等和审计身份，而非决策依据；宿主在提交时
// 需要它填充 action gate 的 CommandID，并在同一事务中写入中止命令收据。没有待处理中止时为空。
//
// 命令不携带时间戳。墙上时钟会使每次重试的摘要变化，将崩溃重试变成第二次应用；摘要正是为防止
// 这一点而存在。宿主在身份之外自行记录完成时间。
type CompleteEntryCommand struct {
	EntryID               domainexecution.EntryID
	Fence                 domainexecution.WorkerFence
	State                 EntryCompletionState
	Outcome               EngineOutcome
	AbortPendingCommandID string
}

// Validate 校验命令是否具有可生成摘要的有效形状；它不判断入口是否可以完成，该问题由
// DecideEntryCompletion 回答，两类失败的处理方式不同。
func (command CompleteEntryCommand) Validate() error {
	if err := command.EntryID.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "entryId", "entry id is invalid"))
	}
	if err := command.Fence.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "fence", "worker execution authority is invalid"))
	}
	if err := command.State.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "state", "observed entry state is invalid"))
	}
	if err := command.Outcome.Validate(); err != nil {
		return completeEntryCommandInvalidError(err, mustEntryCompletionViolation(fault.CodeFieldInvalid, "outcome", "engine outcome is invalid"))
	}
	// 缺省合法；存在但为空白不合法。空身份会作为独立请求生成摘要，却无法指向任何人能找到的命令。
	if command.AbortPendingCommandID != "" && strings.TrimSpace(command.AbortPendingCommandID) == "" {
		return completeEntryCommandInvalidError(nil, mustEntryCompletionViolation(fault.CodeFieldInvalid, "abortPendingCommandId", "abort pending command id is present but blank"))
	}
	return nil
}

// CompleteEntryRequestDigest 生成一次入口完成请求的稳定身份摘要。
//
// 摘要逐字段写入长度前缀，而不是整体序列化，因此字段顺序、编码器版本或可选字段约定不会改变
// 宿主已持久化摘要对应的字节。无法校验的请求没有摘要：函数返回空字符串及错误，遭拒命令不会
// 以看似合理的身份被记录。
func CompleteEntryRequestDigest(command CompleteEntryCommand) (string, error) {
	if err := command.Validate(); err != nil {
		return "", err
	}

	h := sha256.New()
	writeDigestString(h, completeEntryRequestDigestV1)
	writeDigestString(h, command.EntryID.String())
	writeDigestString(h, command.Fence.InstanceID.String())
	writeDigestString(h, command.Fence.ClaimToken)
	writeDigestString(h, string(command.State.EntryStatus))
	writeDigestString(h, string(command.State.TerminalIntent))
	writeDigestUint64(h, uint64(command.State.TerminalIntentRevision))
	writeDigestUint64(h, uint64(command.State.CancellationGeneration))
	writeDigestString(h, string(command.Outcome.Result.ExecutionOutcome))
	writeDigestString(h, string(command.Outcome.Result.RecordingOutcome))
	writeDigestString(h, string(command.Outcome.Result.TimelineOutcome))
	writeDigestString(h, string(command.Outcome.FailureCode))
	writeDigestString(h, command.AbortPendingCommandID)
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// CompleteEntryIntent 携带一次完成请求、其身份以及 Core 为其得出的决策。
//
// 决策随意图传递，而不是在适配器内部重新计算，因为宿主绝不能自行推导 NextIntentRevision 或
// NextCancellationGeneration。ValidateCompleteEntryIntentDigest 将这一保证转化为适配器每次应用前
// 执行的检查。
type CompleteEntryIntent struct {
	EntryID       domainexecution.EntryID
	RequestDigest string
	Command       CompleteEntryCommand
	Decision      EntryCompletionDecision
}

// ValidateCompleteEntryIntentDigest 根据意图自身命令重新计算摘要和决策，并报告任何不一致。
//
// 适配器必须在 CompleteEntry 中首先调用它，再接触存储。这是契约两项保证的机械化实现：记录完成
// 所使用的身份必须确实属于正在应用的请求，待写入计数器必须是 Core 产生的计数器。两项检查均为
// 本地操作，不使用 Context、端口或所有权。
func ValidateCompleteEntryIntentDigest(intent CompleteEntryIntent) error {
	digest, err := CompleteEntryRequestDigest(intent.Command)
	if err != nil {
		return err
	}
	if intent.RequestDigest != digest {
		return completeEntryDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "requestDigest", "request digest does not match the command"))
	}
	if intent.EntryID != intent.Command.EntryID {
		return completeEntryDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "entryId", "intent identity does not match the command"))
	}
	decision, err := DecideEntryCompletion(intent.Command.State, intent.Command.Outcome)
	if err != nil {
		return err
	}
	if intent.Decision != decision {
		return completeEntryDigestMismatchError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "decision", "decision does not follow from the command"))
	}
	return nil
}

// CompleteEntryStatus 表示完成请求是执行了写入还是发现写入已存在。
type CompleteEntryStatus string

const (
	// CompleteEntryApplied 表示本次调用执行了完成。
	CompleteEntryApplied CompleteEntryStatus = "APPLIED"
	// CompleteEntryReplayed 表示相同请求已应用，本次调用未改变任何内容。
	CompleteEntryReplayed CompleteEntryStatus = "REPLAYED"
)

// CompleteEntryOutcome 保存一次完成尝试的结果。Decision 是实际记录的决策，因此重放返回原始应用
// 使用的同一终态答案，而不是重新计算的答案。
type CompleteEntryOutcome struct {
	Status        CompleteEntryStatus
	EntryID       domainexecution.EntryID
	RequestDigest string
	Decision      EntryCompletionDecision
}

// EntryCompletionTransaction 是宿主实现的端口，用于结束入口。
//
// 原子性边界——以下所有内容必须在一个事务中全部落地，否则一项也不得落地：
//
//   - 入口终态及 Decision 中的两个计数器；
//   - 入口终态事实；
//   - 同一运行产生的证据引用；
//   - execution action gate 的终态化：使用 Command.AbortPendingCommandID 作为 gate 的 CommandID，
//     并将 Decision.Next* 对作为精确写入值；
//   - Command.AbortPendingCommandID 存在时的中止命令收据；
//   - 宣布完成的 outbox 记录；
//   - 以 (EntryID, RequestDigest) 为键的幂等收据，最后写入，使事务批次在崩溃时整体保持不可见且可重试。
//
// 宿主在该边界外执行的任何操作都必须能从边界内内容重新推导，例如上传录制、通知 UI、释放 OS 句柄。
// 任何会让完成看起来已结束的写入——任何后续读取者可能误认为已提交终态的内容——都属于边界内。
//
// 顺序是：完成必须先提交，调度再询问 DecideAdvance 是否可以运行下一个入口。DecideAdvance 读取本
// 事务写入的计数器；若先运行它，会读取终态之前的状态，让已取消实例再启动一个入口。
//
// Context 仅用于取消和截止时间；实现不得从中读取业务值。两个方法都不取得传入值的所有权，
// 返回值归调用方所有。
type EntryCompletionTransaction interface {
	// LookupEntryCompletion 按请求摘要查询已记录的完成结果。命中时必须返回 CompleteEntryReplayed，
	// 不得写入；未记录时必须返回 (zero, false, nil)。
	LookupEntryCompletion(ctx context.Context, entryID domainexecution.EntryID, requestDigest string) (CompleteEntryOutcome, bool, error)
	// CompleteEntry 原子应用一次完成。
	//
	// 它必须在接触存储前调用 ValidateCompleteEntryIntentDigest，必须原样写入 Decision 字段而不重新
	// 计算任一计数器；在事务内发现收据已存在时必须返回 CompleteEntryReplayed，不能重复应用。
	// 入口不再持有命令观测状态时，返回 CompleteEntryIdentityConflictError。
	CompleteEntry(ctx context.Context, intent CompleteEntryIntent) (CompleteEntryOutcome, error)
}

// EntryCompletionService 将已校验的完成请求转换为恰好提交一次的终态。
//
// 它是访问 EntryCompletionTransaction 的唯一支持路径：先决策再写入，使不可决策请求不会到达存储；
// 同时将适配器返回的每个结果与产生它的请求逐一校验。使用 NewEntryCompletionService 构造。
// 零值不含事务，每次调用都返回 CodeCompleteEntryUnavailable，而不会 panic。
type EntryCompletionService struct {
	transaction EntryCompletionTransaction
}

// NewEntryCompletionService 将服务连接到一个事务端口。
//
// 它接受 nil 或 typed-nil 事务，不在构造时拒绝。由配置映射装配服务的组合根应在真正需要缺失
// 适配器的调用处失败并指出适配器，而不是启动时在没有请求上下文的情况下失败。服务不拥有事务，
// 也不会关闭事务。
func NewEntryCompletionService(transaction EntryCompletionTransaction) EntryCompletionService {
	return EntryCompletionService{transaction: transaction}
}

// Complete 恰好完成一次入口。
//
// 顺序是有意设计的：先生成摘要（同时校验命令），再决策，再查找已有收据，最后应用。先决策再接触
// 适配器意味着 Core 会拒绝的请求——非 RUNNING 入口、修订耗尽、命令形状错误——不会产生存储往返，
// 也不会留下部分痕迹。重放相同命令会返回 CompleteEntryReplayed 的已记录结果，不应用任何变更。
//
// Context 原样传给适配器。返回结果归调用方所有；拒绝调用时与错误一同返回零结果。
func (s EntryCompletionService) Complete(ctx context.Context, command CompleteEntryCommand) (CompleteEntryOutcome, error) {
	if isNilInterfaceValue(s.transaction) {
		return CompleteEntryOutcome{}, CompleteEntryUnavailableError()
	}
	digest, err := CompleteEntryRequestDigest(command)
	if err != nil {
		return CompleteEntryOutcome{}, err
	}
	decision, err := DecideEntryCompletion(command.State, command.Outcome)
	if err != nil {
		return CompleteEntryOutcome{}, err
	}

	recorded, found, err := s.transaction.LookupEntryCompletion(ctx, command.EntryID, digest)
	if err != nil {
		return CompleteEntryOutcome{}, fmt.Errorf("lookup entry completion: %w", err)
	}
	if found {
		if recorded.Status != CompleteEntryReplayed {
			return CompleteEntryOutcome{}, completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "status", "a recorded completion must be reported as replayed"))
		}
		if err := validateCompleteEntryOutcome(recorded, command.EntryID, digest, decision); err != nil {
			return CompleteEntryOutcome{}, err
		}
		return recorded, nil
	}

	applied, err := s.transaction.CompleteEntry(ctx, CompleteEntryIntent{
		EntryID:       command.EntryID,
		RequestDigest: digest,
		Command:       command,
		Decision:      decision,
	})
	if err != nil {
		return CompleteEntryOutcome{}, fmt.Errorf("complete entry: %w", err)
	}
	// APPLIED 和 REPLAYED 在此处都合法：并发工作线程可能在查询与应用之间提交了相同请求，
	// 适配器在事务内发现自身收据时也正是在执行契约要求的行为。
	if applied.Status != CompleteEntryApplied && applied.Status != CompleteEntryReplayed {
		return CompleteEntryOutcome{}, completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "status", "completion status is not one core defined"))
	}
	if err := validateCompleteEntryOutcome(applied, command.EntryID, digest, decision); err != nil {
		return CompleteEntryOutcome{}, err
	}
	return applied, nil
}

// validateCompleteEntryOutcome 强制适配器结果与传入请求一致。结果若指向不同入口、不同请求，
// 或携带适配器自行重新计算的决策，属于调用方不得据此行动的缺陷，因此直接拒绝而不带警告返回。
func validateCompleteEntryOutcome(outcome CompleteEntryOutcome, entryID domainexecution.EntryID, digest string, decision EntryCompletionDecision) error {
	if outcome.EntryID != entryID {
		return completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "entryId", "outcome names a different entry"))
	}
	if outcome.RequestDigest != digest {
		return completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "requestDigest", "outcome names a different request"))
	}
	if outcome.Decision != decision {
		return completeEntryContractViolationError(mustEntryCompletionViolation(fault.CodeFieldMismatch, "decision", "outcome carries a decision core did not make"))
	}
	return nil
}

// writeDigestString 以长度前缀写入摘要字符串。
func writeDigestString(h hash.Hash, value string) {
	writeDigestUint64(h, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}

// writeDigestUint64 以固定的大端字节序写入摘要整数。
func writeDigestUint64(h hash.Hash, value uint64) {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], value)
	_, _ = h.Write(buffer[:])
}
