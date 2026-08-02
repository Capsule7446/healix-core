// 包节点定义了由主机适配器实现的可执行工作流节点、生命周期状态机和运行时端口。
package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

const terminalEventTimeout = 5 * time.Second

const operationObservationTimeout = 5 * time.Second

// NewElementNotFoundError is the Driver contract's explicit business signal that
// every selector in an ElementTargetSpec has been exhausted. Cancellation,
// invalid selectors, and browser failures remain distinguishable.
func NewElementNotFoundError() error {
	return mustWrapNodeFault(
		errors.New("element lookup exhausted"),
		fault.NotFound,
		CodeElementNotFound,
		"element was not found",
	)
}

// Program 程序是一棵按惯例不可变的可执行树加上为一个 WorkflowExecution 捕获的确切 ElementTargetSpec 索引。编译器每次执行都会构建一个新的程序；运行时覆盖永远不会改变规格。
type Program struct {
	Root  Node
	Specs map[string]fingerprint.ElementTargetSpec
}

// Phase 是某个 step 在 RUNNING -> [HEALING] -> TRANSITIONING ->
// [VALIDATING] -> SUCCEEDED/FAILED 状态机中所处的位置。
type Phase string

const (
	PhaseRunning       Phase = "RUNNING"
	PhaseHealing       Phase = "HEALING"
	PhaseTransitioning Phase = "TRANSITIONING"
	PhaseValidating    Phase = "VALIDATING"
	PhaseSucceeded     Phase = "SUCCEEDED"
	PhaseFailed        Phase = "FAILED"
	PhaseCanceled      Phase = "CANCELED"
)

// StepExecution 保存一次 StepNode.Run 的当前阶段，并拒绝状态机之外的转换。
// Repeat 再次运行同一个 StepNode 时会创建新的执行实例，因此终态不会泄漏到
// 下一轮迭代。
type StepExecution struct {
	nodeID     string
	phase      Phase
	occurrence int
}

func NewStepExecution(nodeID string) *StepExecution {
	return &StepExecution{nodeID: nodeID}
}

func (e *StepExecution) Phase() Phase { return e.phase }

var stepPhaseTransitions = map[Phase]map[Phase]struct{}{
	"": {
		PhaseRunning: {},
	},
	PhaseRunning: {
		PhaseHealing: {}, PhaseTransitioning: {}, PhaseValidating: {},
		PhaseSucceeded: {}, PhaseFailed: {}, PhaseCanceled: {},
	},
	PhaseHealing: {
		PhaseTransitioning: {}, PhaseFailed: {}, PhaseCanceled: {},
	},
	PhaseTransitioning: {
		PhaseValidating: {}, PhaseSucceeded: {}, PhaseFailed: {}, PhaseCanceled: {},
	},
	PhaseValidating: {
		PhaseSucceeded: {}, PhaseFailed: {}, PhaseCanceled: {},
	},
}

func (e *StepExecution) CanTransition(next Phase) error {
	if e == nil {
		return stepPhaseTransitionInvalidError(errors.New("node: nil step execution"))
	}
	// ValidatePhaseTransition already returns a fully classified fault; return it
	// unchanged rather than discarding its detail to build a second one — the
	// contract forbids wrapping an already-coded fault in another fault.
	return ValidatePhaseTransition(e.phase, next)
}

// ValidatePhaseTransition 向持久性适配器公开相同的域保护，因此部分或重复写入无法产生不可能的 StepExecution 历史记录。
func ValidatePhaseTransition(current, next Phase) error {
	if _, ok := stepPhaseTransitions[current][next]; !ok {
		return stepPhaseTransitionInvalidError(fmt.Errorf("invalid step phase transition %q -> %q", current, next))
	}
	return nil
}

func (e *StepExecution) Transition(next Phase) error {
	if err := e.CanTransition(next); err != nil {
		return err
	}
	e.phase = next
	return nil
}

// Event describes one runtime phase transition.
type Event struct {
	InstanceID domainexecution.InstanceID
	NodeID     string
	Occurrence int
	Phase      Phase
	Payload    map[string]any
}

// OperationObservation is an optional, framework-neutral execution fact.
type OperationObservation struct {
	InstanceID domainexecution.InstanceID
	EntryID    domainexecution.EntryID
	Occurrence int
	NodeID     string
	Operation  string
	Selector   fingerprint.Selector
	Healed     bool
	Attempt    int
	DurationMS int64
	Succeeded  bool
	FaultKind  fault.Kind
	FaultCode  fault.Code
}

// OperationObserver can be implemented alongside ExecutionSink without changing
// existing adapters. Implementations must be safe for the single Runtime caller.
type OperationObserver interface {
	RecordOperation(context.Context, OperationObservation) error
}

// Element 是一个已定位、可查询、可交互的 DOM 节点，是对任意浏览器
// 自动化 Driver 的领域层抽象。
type Element interface {
	Exists(ctx context.Context) (bool, error)
	Visible(ctx context.Context) (bool, error)
	Text(ctx context.Context) (string, error)
	Attribute(ctx context.Context, name string) (string, bool, error)
	Click(ctx context.Context) error
	Input(ctx context.Context, text string) error
	// Select 按可见文本选中一个或多个 option。
	Select(ctx context.Context, option string, more ...string) error
	// Hover 把鼠标移到元素上（触发 mouseover/mouseenter）。
	Hover(ctx context.Context) error
	// WaitStable 会阻塞，直到元素的位置/尺寸停止变化后才尝试执行动作
	// 避免在动画进行中点击移动目标。
	WaitStable(ctx context.Context) error
}

// Driver 按优先级顺序将 ElementTargetSpec 的选择器与实时页面比对解析、
// 执行导航，并为自愈提供 DOMSnapshot。
type Driver interface {
	Navigate(ctx context.Context, url string) error
	Press(ctx context.Context, key string) error
	Locate(ctx context.Context, spec fingerprint.ElementTargetSpec) (Element, error)
	Snapshot(ctx context.Context) (heal.DOMSnapshot, error)
	// WaitNetworkIdle 阻塞到页面网络空闲；超时通过 ctx 控制
	// （WaitNode 的条件超时）。
	WaitNetworkIdle(ctx context.Context) error
}

// Recorder 是框架无关的会话录制端口，由宿主提供适配器。
// Runtime 上的 Recorder 为 nil 表示"录屏关闭"，Start/Stop 也就不会被调用。
type Recorder interface {
	Start(ctx context.Context, instanceID domainexecution.InstanceID) (RecordingTimeline, error)
	Stop(ctx context.Context, retain bool) error
}

// HealSampleObserver receives the complete deterministic candidate sample for replay.
type HealSampleObserver interface {
	RecordHealSamples(context.Context, HealSampleRecord) error
}

type HealSampleRecord struct {
	InstanceID  domainexecution.InstanceID
	NodeID      string
	SpecID      string
	OldSelector fingerprint.Selector
	Outcome     heal.Outcome
	Samples     []heal.CandidateSample
}

type TerminalCommit struct {
	Event Event
}

// ExecutionSink stages terminal-associated facts and publishes them only when
// CommitTerminal atomically commits the terminal event under the same fence.
type ExecutionSink interface {
	RecordProgress(context.Context, domainexecution.WorkerFence, Event) error
	StageHealDecision(context.Context, domainexecution.WorkerFence, string, string, fingerprint.Selector, heal.Decision) error
	StageValidationObservation(context.Context, domainexecution.WorkerFence, ValidationObservation) error
	StageValidationGroupTerminal(context.Context, domainexecution.WorkerFence, ValidationGroupTerminalObservation) error
	CommitTerminal(context.Context, domainexecution.WorkerFence, TerminalCommit) error
}

// Runtime 是贯穿整棵 Node 树的、每次运行专属的执行上下文。
// Runtime、Driver、Page 和 Element 端口在当前版本均要求由单个顺序执行器访问；
// 并发调度、资源池和跨页面生命周期属于延期能力。
type Runtime struct {
	InstanceID domainexecution.InstanceID
	EntryID    domainexecution.EntryID
	ClaimToken string
	PageURL    string
	Origin     string
	// StepInterval 控制可执行叶步骤之间的最小暂停时间。第一个叶子步骤立即开始；容器节点和验证组成员不消耗额外的时间间隔。
	StepInterval time.Duration
	// Specs 按 ID 索引每个 StepNode 的 ElementTargetSpec，使断言可以引用
	// 该 step 自身目标以外的其他元素。
	Specs map[string]fingerprint.ElementTargetSpec
	// SelectorOverlay 是本次 run 内按 ElementTargetSpec ID 保存的 healed selector 列表。
	// 编译出的 Specs/StepNode 保持不变，同一 spec 的后续 step、repeat 和断言
	// 都通过 effectiveSpec 读取该 overlay。
	SelectorOverlay      map[string][]fingerprint.Selector
	Driver               Driver
	Healer               heal.Healer // nil = 关闭自愈
	Healing              HealingPort
	Recorder             Recorder      // nil = 关闭录屏
	Facts                ExecutionSink // nil = 不输出执行事实
	Timeline             RecordingTimeline
	StepTimeline         StepTimelineSink
	CompletionChain      *NodeCompletionChain
	ReadOnlyBrowser      ReadOnlyBrowser
	CompletionObserver   NodeCompletionObserver
	OperationObserver    OperationObserver
	HealSamples          HealSampleObserver
	RetryPolicy          RetryPolicy
	HealingPolicy        heal.SafetyPolicy
	HealingReviewCap     float64
	Scratchpad           map[string]any
	parameterScope       map[string]parameter.Value
	leafExecutionStarted bool
	pacer                stepPacer
	occurrences          map[string]int
	activeOccurrences    map[string][]int
}

func (rt *Runtime) Parameters() map[string]parameter.Value {
	if rt == nil || rt.parameterScope == nil {
		return nil
	}
	result := make(map[string]parameter.Value, len(rt.parameterScope))
	for name, value := range rt.parameterScope {
		result[name] = value.Clone()
	}
	return result
}

func (rt *Runtime) LeafExecutionStarted() bool {
	return rt != nil && rt.leafExecutionStarted
}

func (rt *Runtime) observeOperation(ctx context.Context, observation OperationObservation) error {
	if rt.OperationObserver == nil {
		return nil
	}
	return rt.OperationObserver.RecordOperation(ctx, observation)
}

func (rt *Runtime) observeOperationBestEffort(ctx context.Context, observation OperationObservation) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), operationObservationTimeout)
	defer cancel()
	_ = rt.observeOperation(cleanupCtx, observation)
}

func (rt *Runtime) healingPort() HealingPort {
	if rt.Healing != nil {
		return rt.Healing
	}
	return adaptHealer(rt.Healer)
}

func (rt *Runtime) healingReviewCap() float64 {
	if rt.HealingReviewCap > 0 {
		return rt.HealingReviewCap
	}
	return 0.60
}

func (rt *Runtime) recordHealSamples(ctx context.Context, record HealSampleRecord) error {
	if rt == nil || rt.HealSamples == nil {
		return nil
	}
	return rt.HealSamples.RecordHealSamples(ctx, record)
}

func (rt *Runtime) waitBeforeStep(ctx context.Context) error {
	return rt.pacer.before(ctx, rt.StepInterval)
}

func (rt *Runtime) runOperation(operation func() error) error {
	return Retry(rt.RetryPolicy, operation)
}

func (rt *Runtime) runOperationWithAttempts(operation func() error) (int, error) {
	return RetryWithAttempts(rt.RetryPolicy, operation)
}

func (rt *Runtime) activeOccurrence(nodeID string) (int, error) {
	stack := rt.activeOccurrences[nodeID]
	if len(stack) == 0 {
		return 0, fmt.Errorf("node %s without active occurrence", nodeID)
	}
	return stack[len(stack)-1], nil
}

// mustActiveOccurrence returns the active occurrence for nodeID, or 0 if
// none is active. Observations are best-effort and must not break execution
// when the occurrence stack is empty (e.g. during terminal cleanup).
func (rt *Runtime) mustActiveOccurrence(nodeID string) int {
	occurrence, err := rt.activeOccurrence(nodeID)
	if err != nil {
		return 0
	}
	return occurrence
}

// releaseOccurrence releases only the occurrence owned by one invocation. It is
// idempotent and never pops a nested same-ID frame by position alone.
func (rt *Runtime) releaseOccurrence(nodeID string, occurrence int) {
	stack := rt.activeOccurrences[nodeID]
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] != occurrence {
			continue
		}
		stack = append(stack[:i], stack[i+1:]...)
		if len(stack) == 0 {
			delete(rt.activeOccurrences, nodeID)
		} else {
			rt.activeOccurrences[nodeID] = stack
		}
		return
	}
}

func (rt *Runtime) beginOccurrence(ctx context.Context, nodeID string) (int, error) {
	if err := rt.emit(ctx, nodeID, PhaseRunning); err != nil {
		return 0, err
	}
	return rt.activeOccurrence(nodeID)
}

func (rt *Runtime) emit(ctx context.Context, nodeID string, phase Phase) error {
	if rt.occurrences == nil {
		rt.occurrences = make(map[string]int)
		rt.activeOccurrences = make(map[string][]int)
	}
	stack := rt.activeOccurrences[nodeID]
	occurrence := 0
	if phase == PhaseRunning {
		occurrence = rt.occurrences[nodeID] + 1
	} else {
		if len(stack) == 0 {
			return fmt.Errorf("emit execution event %s/%s without active occurrence", nodeID, phase)
		}
		occurrence = stack[len(stack)-1]
	}
	evt := Event{InstanceID: rt.InstanceID, NodeID: nodeID, Occurrence: occurrence, Phase: phase}
	fence := domainexecution.WorkerFence{InstanceID: rt.InstanceID, ClaimToken: rt.ClaimToken}
	var recordErr error
	if rt.Facts != nil {
		if phase == PhaseSucceeded || phase == PhaseFailed || phase == PhaseCanceled {
			recordErr = rt.Facts.CommitTerminal(ctx, fence, TerminalCommit{Event: evt})
		} else {
			recordErr = rt.Facts.RecordProgress(ctx, fence, evt)
		}
	}
	if phase == PhaseRunning {
		if recordErr != nil {
			return evidenceRecordFailedError(recordErr)
		}
		rt.occurrences[nodeID] = occurrence
		rt.activeOccurrences[nodeID] = append(stack, occurrence)
		return nil
	}
	if phase == PhaseSucceeded || phase == PhaseFailed || phase == PhaseCanceled {
		if recordErr == nil {
			rt.releaseOccurrence(nodeID, occurrence)
		}
	}
	if recordErr != nil {
		return evidenceRecordFailedError(recordErr)
	}
	return nil
}

// emitTerminal 为终端审核事件提供有界清理上下文。步骤超时或进程信号在发出 FAILED 之前取消执行上下文；重用该上下文将使持久性尝试确定性失败，并使节点没有终止事件。
func (rt *Runtime) emitTerminal(ctx context.Context, nodeID string, phase Phase) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
	defer cancel()
	return rt.emit(cleanupCtx, nodeID, phase)
}

func failurePhase(ctx context.Context) Phase {
	if ctx.Err() != nil {
		return PhaseCanceled
	}
	return PhaseFailed
}

func (rt *Runtime) effectiveSpec(base fingerprint.ElementTargetSpec) fingerprint.ElementTargetSpec {
	spec := base
	if canonical, ok := rt.Specs[base.ID]; ok {
		spec = canonical
	}
	if selectors, ok := rt.SelectorOverlay[spec.ID]; ok {
		spec.Selectors = append([]fingerprint.Selector(nil), selectors...)
	}
	return spec
}

func (rt *Runtime) specByID(id string) (fingerprint.ElementTargetSpec, bool) {
	spec, ok := rt.Specs[id]
	if !ok {
		return fingerprint.ElementTargetSpec{}, false
	}
	return rt.effectiveSpec(spec), true
}

func (rt *Runtime) setSelectorOverlay(spec fingerprint.ElementTargetSpec) {
	if rt.SelectorOverlay == nil {
		rt.SelectorOverlay = make(map[string][]fingerprint.Selector)
	}
	rt.SelectorOverlay[spec.ID] = append([]fingerprint.Selector(nil), spec.Selectors...)
}
