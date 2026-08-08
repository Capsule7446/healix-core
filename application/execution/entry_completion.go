package execution

import (
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/application/engine"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// MaxExpectedEntryCompletionRevision 是 Core 写入 NextIntentRevision 或 NextCancellationGeneration
// 的最大值。
//
// 已达到或超过该值的计数器没有 Core 可产生的后继值：下一个值会是 math.MaxInt64，再完成一次会
// 溢出到 MinInt64；适配器无法对该值执行 CAS，宿主的乐观并发检查会静默变成完全不检查。因此拒绝完成。
//
// 上限按计数器分别检查，且仅检查决策实际推进的计数器。原样传递的 cancellation generation 不需要
// 后继值，因此意图未执行时，即使状态处于上限仍可完成。
const MaxExpectedEntryCompletionRevision int64 = math.MaxInt64 - 1

// mustEntryCompletionViolation 构造字段级原因。这里的字段名和错误码均为编译期常量，构造失败只
// 可能表示程序错误，而非业务错误。
func mustEntryCompletionViolation(code fault.Code, field, message string) fault.Violation {
	violation, err := fault.NewViolation(code, field, message)
	if err != nil {
		panic(err)
	}
	return violation
}

// entryCompletionStateInvalidError 构造带字段违规明细的无效完成状态错误。
func entryCompletionStateInvalidError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeEntryCompletionStateInvalid, "entry completion state is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// entryCompletionRevisionExhaustedError 构造不存在可表示后继修订时的范围错误。
func entryCompletionRevisionExhaustedError() error {
	err, constructionErr := fault.New(fault.OutOfRange, CodeEntryCompletionRevisionExhausted, "entry completion revision has no representable successor")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// entryCompletionNotRunningError 构造入口不处于 RUNNING 状态时的完成前置条件错误。
func entryCompletionNotRunningError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeEntryCompletionNotRunning, "entry is not running and cannot be completed")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// engineOutcomeInvalidError 构造带字段违规明细的无效引擎结果错误。
func engineOutcomeInvalidError(violation fault.Violation) error {
	err, constructionErr := fault.New(fault.InvalidArgument, CodeEngineOutcomeInvalid, "engine outcome is invalid", fault.WithViolations(violation))
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// TerminalIntent 是 Core 所有的“有人请求停止此实例”词汇。宿主持久化并比较这些值，但不自行发明值；
// ExecutionActionGateV1 通过将合法值保留在此处来维护单一执行权威。
//
// TerminalIntentNone 是真实值而非零占位：即使没有人请求停止，入口仍有要记录的意图；空意图会被拒绝，
// 不会被解释为“none”。
type TerminalIntent string

const (
	// TerminalIntentNone 表示当前没有取消或中止请求。
	TerminalIntentNone TerminalIntent = "NONE"
	// TerminalIntentCancel 表示请求取消尚未完成的执行。
	TerminalIntentCancel TerminalIntent = "CANCEL"
	// TerminalIntentAbort 表示请求中止正在执行的入口。
	TerminalIntentAbort TerminalIntent = "ABORT"
)

// Validate 校验意图是否属于 Core 定义的词汇。宿主从存储读回意图后，应在将其交给决策前调用。
func (intent TerminalIntent) Validate() error {
	switch intent {
	case TerminalIntentNone, TerminalIntentCancel, TerminalIntentAbort:
		return nil
	default:
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "terminalIntent", "terminal intent is not one core defined"))
	}
}

// EngineOutcome 保存一次入口引擎运行观测到的领域结果。
//
// Result 是 engine.RunProgram 自身的报告，原样传递而不重新编码；维护第二套枚举会为两套词汇产生
// 漂移的第二个位置。FailureCode 是运行错误（若有）的已分类错误码；无错误时为空。它用于审计链，
// 且有意不参与终态决定。
type EngineOutcome struct {
	Result      engine.EntryResult
	FailureCode fault.Code
}

// NotStartedEngineOutcome 构造引擎从未运行的入口结果，例如 fence 过期、授权被拒或浏览器无法创建。
// 这是有效且可决策的结果而非缺失值，使每个失败入口仍有可提交的终态和可释放的租约。
func NotStartedEngineOutcome() EngineOutcome {
	return EngineOutcome{Result: engine.EntryResult{
		ExecutionOutcome: engine.ExecutionNotStarted,
		RecordingOutcome: engine.RecordingDisabled,
		TimelineOutcome:  engine.TimelineDisabled,
	}}
}

// InterruptedEngineOutcome 构造运行未被观察到完成的入口结果：引擎运行期间宿主进程终止，恢复发现
// 入口仍为 RUNNING 且没有任何人持有领取。
//
// 恢复路径在处理孤立入口时必须使用此结果，而不能使用 NotStartedEngineOutcome：后者表示引擎已知
// 未开始，孤立入口可能已运行完成并随进程终止带走结果，因此将其作为未开始会产生事实错误。若两者
// 都终止为 FAILED，持久化后无法区分；依赖证据链的产品会将崩溃误读为业务失败，进而污染失败率统计
// 和自愈候选选择。
func InterruptedEngineOutcome() EngineOutcome {
	return EngineOutcome{Result: engine.EntryResult{
		ExecutionOutcome: engine.ExecutionInterrupted,
		// 每个轴都表达同一事实：未知。此处写入 DISABLED 会在更低层重复本构造器要阻止的错误：
		// 观测器终止的入口可能正在录制，并留下宿主之后发现却无法与“录制已关闭”记录对应的部分文件。
		RecordingOutcome: engine.RecordingUnobserved,
		TimelineOutcome:  engine.TimelineUnobserved,
	}}
}

// Validate 校验所有字段是否属于引擎词汇。
func (outcome EngineOutcome) Validate() error {
	switch outcome.Result.ExecutionOutcome {
	case engine.OutcomeSucceeded, engine.OutcomeFailed, engine.OutcomeCanceled, engine.ExecutionNotStarted, engine.ExecutionInterrupted:
	default:
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "result.executionOutcome", "execution outcome is not one the engine reports"))
	}
	switch outcome.Result.RecordingOutcome {
	case engine.RecordingDisabled, engine.RecordingSucceeded, engine.RecordingStartFailed, engine.RecordingStopFailed, engine.RecordingUnobserved:
	default:
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "result.recordingOutcome", "recording outcome is not one the engine reports"))
	}
	switch outcome.Result.TimelineOutcome {
	case engine.TimelineDisabled, engine.TimelineComplete, engine.TimelineStartFailed, engine.TimelineFinishFailed, engine.TimelineUnobserved:
	default:
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "result.timelineOutcome", "timeline outcome is not one the engine reports"))
	}
	// 已存在但为空白的错误码表示分类丢失，而不是“无失败”：它会以无人可追踪的空审计字段被记录。
	if outcome.FailureCode != "" && strings.TrimSpace(string(outcome.FailureCode)) == "" {
		return engineOutcomeInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "failureCode", "failure code is present but blank"))
	}
	return nil
}

// EntryCompletionState 保存宿主在完成入口前立即观测到的状态，是完整决策依据；未包含的内容均为有意
// 缺省。
//
// AbortPendingCommandID 就是这种有意缺省的字段。待处理中止命令是幂等身份，而非终态意图；将其排除
// 在此结构之外，可在结构层面阻止“把待处理中止误认为有效意图”，而不只是依赖约定。它随
// CompleteEntryCommand 传递，归属在那里。
type EntryCompletionState struct {
	EntryStatus            domainexecution.EntryStatus
	TerminalIntent         TerminalIntent
	TerminalIntentRevision int64
	CancellationGeneration int64
}

// Validate 校验观测状态是否属于 Core 词汇。它有意不判断入口是否可以完成：形状有效的 PENDING 状态
// 是有效但不可决策的状态，两者答案不同，处理方式也不同。
func (state EntryCompletionState) Validate() error {
	switch state.EntryStatus {
	case domainexecution.EntryPending, domainexecution.EntryRunning, domainexecution.EntrySucceeded,
		domainexecution.EntryFailed, domainexecution.EntryCanceled, domainexecution.EntryAborted,
		domainexecution.EntrySkipped:
	default:
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "entryStatus", "entry status is not one core defined"))
	}
	if err := state.TerminalIntent.Validate(); err != nil {
		return err
	}
	if state.TerminalIntentRevision < 0 {
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "terminalIntentRevision", "terminal intent revision must not be negative"))
	}
	if state.CancellationGeneration < 0 {
		return entryCompletionStateInvalidError(mustEntryCompletionViolation(fault.CodeFieldInvalid, "cancellationGeneration", "cancellation generation must not be negative"))
	}
	return nil
}

// TerminalCause 表示入口达到终态前，有多少运行过程被观测到；它是独立于终态状态的第二条轴。
//
// D-18 的核心是保持两条轴分离：EntryStatus 回答“入口最终变成什么”，可由终态意图决定；
// TerminalCause 回答“是否有人看到它发生”，任何意图都不能改变。
//
// 没有该字段，运行后断言失败、浏览器无法创建以及观测器崩溃的入口都会落为 FAILED，持久化后无
// 字段区分三者。
type TerminalCause string

const (
	// TerminalCauseCompleted 表示引擎运行并报告结果；结果可以是失败，关键在于运行被观测到。
	TerminalCauseCompleted TerminalCause = "COMPLETED"
	// TerminalCauseNotStarted 表示已知引擎从未开始，例如 fence 过期、授权被拒或浏览器无法创建。
	TerminalCauseNotStarted TerminalCause = "NOT_STARTED"
	// TerminalCauseInterrupted 表示运行未被观测到完成；恢复终止孤立 RUNNING 入口时产生此值，
	// 引擎是否实际完成未知且无法得知。
	TerminalCauseInterrupted TerminalCause = "INTERRUPTED"
)

// EntryCompletionDecision 保存一次入口完成的完整终态答案及终态意图计数器 CAS 值。
type EntryCompletionDecision struct {
	EntryStatus domainexecution.EntryStatus
	// TerminalCause 随状态传递，而不是留给宿主从命令推导，因为宿主会将此结构原样持久化为权威终态
	// 记录。凡是需要宿主重新计算的字段，都可能被两个宿主计算出不同结果。
	TerminalCause                 TerminalCause
	CurrentIntent                 TerminalIntent
	CurrentIntentRevision         int64
	CurrentCancellationGeneration int64
	NextIntent                    TerminalIntent
	NextIntentRevision            int64
	NextCancellationGeneration    int64
}

// DecideEntryCompletion 计算一个入口达到的终态，以及实例终态意图计数器应变为何值。
//
// 它是状态与结果的纯函数：相同输入对始终产生相同决策，宿主可以独立调用作预检查，并获得提交时
// 使用的同一答案。它不接触端口，不接收 Context，也不拥有资源。
//
// 引擎结果、终态意图和起始状态的每种组合都有确定结果或明确错误，宿主无需自行决定：
//
//   - 非 RUNNING 入口以 CodeEntryCompletionNotRunning 拒绝。
//   - 裁决一：引擎完成运行时，事实优先于意图。达到 SUCCEEDED 表示外部副作用已经发生——表单已提交、
//     订单已下单——取消无法回滚这些副作用。记录 CANCELED 会与同一运行产生的证据链矛盾。入口
//     记录为 SUCCEEDED，同时将意图完整带入 Next* 字段，使 DecideAdvance 停止下一个入口，取消
//     才会在那里真正生效。
//   - 其他情况由意图指定终态：NONE→FAILED、CANCEL→CANCELED、ABORT→ABORTED。无意图时引擎
//     返回 CANCELED 或 NOT_STARTED 仍得到 FAILED，而不是错误：入口已经处于 RUNNING，拒绝给它
//     终态会使其滞留并留下无法释放的租约。
//   - 录制和时间线结果永远不改变终态。录制器停止失败会降低证据质量，而不是改变运行结果。
//
// NextIntent 始终等于 CurrentIntent：完成入口只观测意图，不改变意图。NextIntentRevision 始终恰好
// 递增一，使每次完成写入不同值，并可区分重放提交与新提交。NextCancellationGeneration 仅在意图
// 实际执行时推进，即终态为 CANCELED 或 ABORTED 时；意图若因运行完成而未执行，generation 不会被
// 消耗。达到 MaxExpectedEntryCompletionRevision 的 generation 仅阻止需要推进它的完成。
//
// 顺序是：先得出本决策并通过 EntryCompletionTransaction 提交，调度再询问 DecideAdvance 是否可以
// 运行下一个入口。DecideAdvance 读取本决策产生的计数器；若先调用它，会读取终态之前的状态，让已
// 取消实例再启动一个入口。
func DecideEntryCompletion(state EntryCompletionState, outcome EngineOutcome) (EntryCompletionDecision, error) {
	if err := state.Validate(); err != nil {
		return EntryCompletionDecision{}, err
	}
	if err := outcome.Validate(); err != nil {
		return EntryCompletionDecision{}, err
	}
	if state.EntryStatus != domainexecution.EntryRunning {
		return EntryCompletionDecision{}, entryCompletionNotRunningError()
	}
	// 每次完成都会推进意图修订，因此始终必须能表示决策自身的后继值。
	if state.TerminalIntentRevision >= MaxExpectedEntryCompletionRevision {
		return EntryCompletionDecision{}, entryCompletionRevisionExhaustedError()
	}

	status := decideTerminalEntryStatus(state.TerminalIntent, outcome.Result.ExecutionOutcome)
	nextGeneration := state.CancellationGeneration
	if status == domainexecution.EntryCanceled || status == domainexecution.EntryAborted {
		// 仅本决策实际消耗的 generation 需要后继值。原样传递的 generation 不会溢出；拒绝当前完成
		// 会使已结束入口滞留在 RUNNING，并留下无人可释放的租约，这正是本契约要防止的结果。
		if state.CancellationGeneration >= MaxExpectedEntryCompletionRevision {
			return EntryCompletionDecision{}, entryCompletionRevisionExhaustedError()
		}
		nextGeneration++
	}
	return EntryCompletionDecision{
		EntryStatus:                   status,
		TerminalCause:                 terminalCauseOf(outcome.Result.ExecutionOutcome),
		CurrentIntent:                 state.TerminalIntent,
		CurrentIntentRevision:         state.TerminalIntentRevision,
		CurrentCancellationGeneration: state.CancellationGeneration,
		NextIntent:                    state.TerminalIntent,
		NextIntentRevision:            state.TerminalIntentRevision + 1,
		NextCancellationGeneration:    nextGeneration,
	}, nil
}

// terminalCauseOf 从引擎结果读取观测轴。它覆盖已校验的完整词汇，且有意不接收意图：意图可以改变
// 未完成入口达到的终态，但不能改变运行是否被观测到。
func terminalCauseOf(executed engine.ExecutionOutcome) TerminalCause {
	switch executed {
	case engine.ExecutionNotStarted:
		return TerminalCauseNotStarted
	case engine.ExecutionInterrupted:
		return TerminalCauseInterrupted
	default:
		// SUCCEEDED、FAILED 和 CANCELED 都表示运行已发生。尾部分支覆盖已校验词汇，而不是吞掉未知值的
		// catch-all：EngineOutcome.Validate 已拒绝词汇之外的值。
		return TerminalCauseCompleted
	}
}

// decideTerminalEntryStatus 覆盖已校验词汇：两个 switch 均为穷尽分支，尾部返回是不可达路径，
// 不会静默吸收新增的引擎常量。
func decideTerminalEntryStatus(intent TerminalIntent, executed engine.ExecutionOutcome) domainexecution.EntryStatus {
	if executed == engine.OutcomeSucceeded {
		return domainexecution.EntrySucceeded
	}
	switch intent {
	case TerminalIntentCancel:
		return domainexecution.EntryCanceled
	case TerminalIntentAbort:
		return domainexecution.EntryAborted
	default:
		return domainexecution.EntryFailed
	}
}
