package execution

import (
	"strings"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// abortRequestInvalidError 构造带字段违规明细的无效中止请求错误。
func abortRequestInvalidError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeAbortRequestInvalid, "abort request is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// abortRequestNotRunningError 构造入口不处于 RUNNING 状态时的前置条件错误。
func abortRequestNotRunningError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeAbortRequestNotRunning, "entry is not running and cannot be asked to abort")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// abortRequestAlreadyAbortingError 构造入口已有 ABORT 意图时的前置条件错误。
func abortRequestAlreadyAbortingError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeAbortRequestAlreadyAborting, "entry is already aborting")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// AbortRequest 表示一次请求停止运行中入口的身份。
//
// 它只携带身份。AbortPendingCommandID 标识宿主命令，用于识别重放并追踪审计记录；
// DecideAbortRequest 无论接收到哪一个身份，都必须给出相同决策。D-12 通过将该字段排除在
// EntryCompletionState 之外保持同样的分离。该类型作为参数存在，是因为决策输入属于契约：
// 后续请求属性应放在宿主已经传入值的此处，而不是添加到每个调用点的第二个参数中。
type AbortRequest struct {
	AbortPendingCommandID string
}

// Validate 校验请求身份是否可由宿主持久化并在后续匹配。首尾空白会被拒绝而非裁剪：宿主按原值
// 存储并比较，若此处静默修改，Core 与宿主会对键值产生分歧。
func (request AbortRequest) Validate() error {
	if request.AbortPendingCommandID == "" || strings.TrimSpace(request.AbortPendingCommandID) == "" {
		return abortRequestInvalidError(mustEntryCompletionViolation(fault.CodeFieldRequired, "abortPendingCommandId", "abort pending command id is required"))
	}
	if request.AbortPendingCommandID != strings.TrimSpace(request.AbortPendingCommandID) {
		return abortRequestInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "abortPendingCommandId", "abort pending command id must be normalized"))
	}
	return nil
}

// AbortRequestDecision 是一次中止请求的完整决策：包含精确的 CAS 谓词和待写入值。
//
// Current* 字段重复 Core 获知的观测值，使宿主无需重新读取即可匹配待处理中止意图行。
// Next* 字段必须原样写入；宿主不得递增或推导任一计数器，这是 ExecutionActionGateV1 保持
// 单一执行权威所依赖的同一规则。
//
// 这里没有入口状态，这一缺失是有意的。请求中止只记录意图，不会结束任何事项。终态仍由
// DecideEntryCompletion 唯一产生，因此中止和普通完成会汇聚到同一终态写入路径。
type AbortRequestDecision struct {
	CurrentIntent                 TerminalIntent
	CurrentIntentRevision         int64
	CurrentCancellationGeneration int64
	NextIntent                    TerminalIntent
	NextIntentRevision            int64
	NextCancellationGeneration    int64
}

// DecideAbortRequest 计算有人请求停止运行中入口时终态意图计数器应变为何值。
//
// 它是状态和请求的纯函数：不使用 Context、端口或时钟。宿主可以将其作为预检查调用，并获得
// 提交时将使用的同一答案。
//
// 每种组合都有确定结果或明确错误：
//
//   - 非 RUNNING 入口以 CodeAbortRequestNotRunning 拒绝；已达到终态的入口没有可停止的内容。
//   - NONE 和 CANCEL 都推进到 ABORT。ABORT 严格强于 CANCEL：CANCEL 停止尚未开始的入口，
//     ABORT 结束正在执行的入口，因此允许升级而不视为冲突。
//   - ABORT 以 CodeAbortRequestAlreadyAborting 拒绝。没有计数器需要推进，写入无操作修订
//     反而有害：它会使已读取旧值的完成操作失去 CAS 谓词，令重复点击伪装成完成冲突。
//
// NextIntentRevision 恰好递增一，使请求写入的值可与其他写入区分，并可将重放与新请求区分。
//
// NextCancellationGeneration 原样传递。D-12 仅在意图实际执行时消耗 generation，即完成达到
// CANCELED 或 ABORTED 时；请求本身不执行意图。此处同时递增会让一次中止消耗两次 generation，
// 而调度器正是读取 generation 判断实例能否继续推进。因此已达到 MaxExpectedEntryCompletionRevision
// 的 generation 不会阻止请求：请求不需要为它产生后继值。
//
// 顺序是：先写入本函数产生的决策，再让入口运行到自然停止点，最后由
// EntryCompletionTransaction.CompleteEntry 在一个事务中终止入口并完成宿主 action gate。
// 不得使用本函数终止入口或使领取失效。
func DecideAbortRequest(state EntryCompletionState, request AbortRequest) (AbortRequestDecision, error) {
	if err := state.Validate(); err != nil {
		return AbortRequestDecision{}, err
	}
	if err := request.Validate(); err != nil {
		return AbortRequestDecision{}, err
	}
	if state.EntryStatus != domainexecution.EntryRunning {
		return AbortRequestDecision{}, abortRequestNotRunningError()
	}
	if state.TerminalIntent == TerminalIntentAbort {
		return AbortRequestDecision{}, abortRequestAlreadyAbortingError()
	}
	// 请求始终写入意图修订的后继值，因此需要可表示的始终是该计数器；此处不推进 generation，
	// 请求不会使其耗尽。
	if state.TerminalIntentRevision >= MaxExpectedEntryCompletionRevision {
		return AbortRequestDecision{}, entryCompletionRevisionExhaustedError()
	}
	return AbortRequestDecision{
		CurrentIntent:                 state.TerminalIntent,
		CurrentIntentRevision:         state.TerminalIntentRevision,
		CurrentCancellationGeneration: state.CancellationGeneration,
		NextIntent:                    TerminalIntentAbort,
		NextIntentRevision:            state.TerminalIntentRevision + 1,
		NextCancellationGeneration:    state.CancellationGeneration,
	}, nil
}
