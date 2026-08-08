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

// terminalEventTimeout 是终态事件持久化使用的独立清理超时。
const terminalEventTimeout = 5 * time.Second

// operationObservationTimeout 是尽力记录操作观测使用的独立超时。
const operationObservationTimeout = 5 * time.Second

// NewElementNotFoundError 构造表示目标所有选择器均已耗尽的 Driver 业务错误；取消、无效
// 选择器和浏览器失败保持可区分。
func NewElementNotFoundError() error {
	return mustWrapNodeFault(
		errors.New("element lookup exhausted"),
		fault.NotFound,
		CodeElementNotFound,
		"element was not found",
	)
}

// Program 是按约定不可变的可执行节点树及本次执行使用的 ElementTargetSpec 索引。
type Program struct {
	Root  Node
	Specs map[string]fingerprint.ElementTargetSpec
}

// Phase 表示步骤在运行、自愈、迁移、验证或终止状态机中的位置。
type Phase string

const (
	// PhaseRunning 表示步骤正在运行。
	PhaseRunning Phase = "RUNNING"
	// PhaseHealing 表示步骤正在自愈。
	PhaseHealing Phase = "HEALING"
	// PhaseTransitioning 表示步骤正在迁移。
	PhaseTransitioning Phase = "TRANSITIONING"
	// PhaseValidating 表示步骤正在验证。
	PhaseValidating Phase = "VALIDATING"
	// PhaseSucceeded 表示步骤成功终止。
	PhaseSucceeded Phase = "SUCCEEDED"
	// PhaseFailed 表示步骤失败终止。
	PhaseFailed Phase = "FAILED"
	// PhaseCanceled 表示步骤因取消终止。
	PhaseCanceled Phase = "CANCELED"
)

// StepExecution 保存一次 StepNode.Run 的当前阶段，并拒绝状态机之外的转换。
// Repeat 再次运行同一个 StepNode 时会创建新的执行实例，因此终态不会泄漏到
// 下一轮迭代。
// StepExecution 保存一次节点执行的当前阶段和 Occurrence。
type StepExecution struct {
	nodeID     string
	phase      Phase
	occurrence int
}

// NewStepExecution 创建处于初始阶段、绑定 nodeID 的步骤执行状态。
func NewStepExecution(nodeID string) *StepExecution {
	return &StepExecution{nodeID: nodeID}
}

// Phase 返回步骤执行当前阶段；nil 接收者会触发与其他字段访问相同的运行时语义。
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

// CanTransition 校验当前步骤执行是否允许迁移到 next，并传播已分类阶段错误。
func (e *StepExecution) CanTransition(next Phase) error {
	if e == nil {
		return stepPhaseTransitionInvalidError(errors.New("node: nil step execution"))
	}
	// ValidatePhaseTransition 已返回完整分类错误，原样传播以避免嵌套已编码 fault。
	return ValidatePhaseTransition(e.phase, next)
}

// ValidatePhaseTransition 校验阶段迁移图，供持久化适配器复用同一领域保护。
func ValidatePhaseTransition(current, next Phase) error {
	if _, ok := stepPhaseTransitions[current][next]; !ok {
		return stepPhaseTransitionInvalidError(fmt.Errorf("invalid step phase transition %q -> %q", current, next))
	}
	return nil
}

// Transition 校验并应用阶段迁移；失败时不修改当前阶段。
func (e *StepExecution) Transition(next Phase) error {
	if err := e.CanTransition(next); err != nil {
		return err
	}
	e.phase = next
	return nil
}

// Event 记录节点执行阶段事件及其实例、节点和 Occurrence 身份。
type Event struct {
	InstanceID domainexecution.InstanceID
	NodeID     string
	Occurrence int
	Phase      Phase
}

// OperationObservation 是可选的框架无关操作事实。
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

// OperationObserver 记录操作观测；实现需支持单个 Runtime 调用方的访问约束。
type OperationObserver interface {
	// RecordOperation 记录一次操作观测。
	RecordOperation(context.Context, OperationObservation) error
}

// Element 是一个已定位、可查询、可交互的 DOM 节点，是对任意浏览器
// 自动化 Driver 的领域层抽象。
type Element interface {
	// Exists 判断元素是否存在。
	Exists(ctx context.Context) (bool, error)
	// Visible 判断元素是否可见。
	Visible(ctx context.Context) (bool, error)
	// Text 返回元素文本。
	Text(ctx context.Context) (string, error)
	// Attribute 返回属性值及是否存在标志。
	Attribute(ctx context.Context, name string) (string, bool, error)
	// Click 点击元素。
	Click(ctx context.Context) error
	// Input 向元素输入文本。
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
	// Navigate 导航到 URL。
	Navigate(ctx context.Context, url string) error
	// Press 向页面发送按键。
	Press(ctx context.Context, key string) error
	// Locate 按目标规格定位元素。
	Locate(ctx context.Context, spec fingerprint.ElementTargetSpec) (Element, error)
	// Snapshot 返回用于自愈的 DOM 快照。
	Snapshot(ctx context.Context) (heal.DOMSnapshot, error)
	// WaitNetworkIdle 阻塞到页面网络空闲；超时通过 ctx 控制
	// （WaitNode 的条件超时）。
	WaitNetworkIdle(ctx context.Context) error
}

// PageLocation 表示浏览器查询时的实时 URL 和来源。
type PageLocation struct {
	URL    string
	Origin string
}

// PageLocator 查询浏览器实时位置，用于自愈时确认来源安全边界。
type PageLocator interface {
	// CurrentLocation 返回浏览器当前页面位置。
	CurrentLocation(ctx context.Context) (PageLocation, error)
}

// Recorder 是框架无关的会话录制端口，由宿主提供适配器。
// Runtime 上的 Recorder 为 nil 表示"录屏关闭"，Start/Stop 也就不会被调用。
type Recorder interface {
	// Start 开始录制并返回时间线。
	Start(ctx context.Context, instanceID domainexecution.InstanceID) (RecordingTimeline, error)
	// Stop 停止录制，并按 retain 决定是否保留产物。
	Stop(ctx context.Context, retain bool) error
}

// HealSampleObserver 接收可重放的完整确定性候选样本。
type HealSampleObserver interface {
	// RecordHealSamples 记录一次自愈候选样本。
	RecordHealSamples(context.Context, HealSampleRecord) error
}

// HealSampleRecord 保存实例、节点、规格、旧选择器、自愈结果和候选样本。
type HealSampleRecord struct {
	InstanceID  domainexecution.InstanceID
	NodeID      string
	SpecID      string
	OldSelector fingerprint.Selector
	Outcome     heal.Outcome
	Samples     []heal.CandidateSample
}

// TerminalCommit 携带需要与同一 fence 原子提交的终态事件。
type TerminalCommit struct {
	Event Event
}

// ExecutionSink 暂存终态相关事实，并仅在同一 fence 下 CommitTerminal 原子提交时发布。
type ExecutionSink interface {
	// RecordProgress 记录非终态进度事件。
	RecordProgress(context.Context, domainexecution.WorkerFence, Event) error
	// StageHealDecision 暂存自愈决策及其选择器。
	StageHealDecision(context.Context, domainexecution.WorkerFence, string, string, fingerprint.Selector, heal.Decision) error
	// StageValidationObservation 暂存验证观测。
	StageValidationObservation(context.Context, domainexecution.WorkerFence, ValidationObservation) error
	// StageValidationGroupTerminal 暂存验证组终态观测。
	StageValidationGroupTerminal(context.Context, domainexecution.WorkerFence, ValidationGroupTerminalObservation) error
	// CommitTerminal 在 fence 下原子提交终态事件及已暂存事实。
	CommitTerminal(context.Context, domainexecution.WorkerFence, TerminalCommit) error
}

// Runtime 是贯穿整棵 Node 树的、每次运行专属的执行上下文。
// Runtime、Driver、Page 和 Element 端口在当前版本均要求由单个顺序执行器访问；
// 并发调度、资源池和跨页面生命周期属于延期能力。
type Runtime struct {
	InstanceID domainexecution.InstanceID
	EntryID    domainexecution.EntryID
	ClaimToken string
	// StepInterval 控制可执行叶步骤之间的最小暂停时间。第一个叶子步骤立即开始；容器节点和验证组成员不消耗额外的时间间隔。
	StepInterval time.Duration
	// Specs 按 ID 索引每个 StepNode 的 ElementTargetSpec，使断言可以引用
	// 该 step 自身目标以外的其他元素。
	Specs map[string]fingerprint.ElementTargetSpec
	// SelectorOverlay 是本次 run 内按 ElementTargetSpec ID 保存的 healed selector 列表。
	// 编译出的 Specs/StepNode 保持不变，同一 spec 的后续 step、repeat 和断言
	// 都通过 effectiveSpec 读取该 overlay。
	SelectorOverlay map[string][]fingerprint.Selector
	Driver          Driver
	// PageLocator 供自愈安全评估读取实时页面位置；nil 表示无法确认位置，
	// 自愈一律拒绝。
	PageLocator          PageLocator
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

// Parameters 返回参数作用域的深拷贝；nil Runtime 或 nil 作用域返回 nil。
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

// LeafExecutionStarted 判断是否已有叶节点开始执行。
func (rt *Runtime) LeafExecutionStarted() bool {
	return rt != nil && rt.leafExecutionStarted
}

// observeOperation 将操作观测提交到可选端口。
func (rt *Runtime) observeOperation(ctx context.Context, observation OperationObservation) error {
	if rt.OperationObserver == nil {
		return nil
	}
	return rt.OperationObserver.RecordOperation(ctx, observation)
}

// observeOperationBestEffort 在独立超时上下文中尽力记录操作观测，忽略记录错误。
func (rt *Runtime) observeOperationBestEffort(ctx context.Context, observation OperationObservation) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), operationObservationTimeout)
	defer cancel()
	_ = rt.observeOperation(cleanupCtx, observation)
}

// currentLocation 查询实时页面位置；端口缺失、查询失败或空结果均返回零上下文，使自愈拒绝。
func (rt *Runtime) currentLocation(ctx context.Context) heal.ExecutionContext {
	if rt == nil || rt.PageLocator == nil {
		return heal.ExecutionContext{}
	}
	location, err := rt.PageLocator.CurrentLocation(ctx)
	if err != nil {
		return heal.ExecutionContext{}
	}
	return heal.ExecutionContext{PageURL: location.URL, Origin: location.Origin}
}

// healingPort 返回显式 HealingPort，未设置时适配 Healer。
func (rt *Runtime) healingPort() HealingPort {
	if rt.Healing != nil {
		return rt.Healing
	}
	return adaptHealer(rt.Healer)
}

// healingReviewCap 返回有效审核阈值，未设置时使用 0.60。
func (rt *Runtime) healingReviewCap() float64 {
	if rt.HealingReviewCap > 0 {
		return rt.HealingReviewCap
	}
	return 0.60
}

// recordHealSamples 将自愈样本提交到可选端口；端口缺失时视为无需记录。
func (rt *Runtime) recordHealSamples(ctx context.Context, record HealSampleRecord) error {
	if rt == nil || rt.HealSamples == nil {
		return nil
	}
	return rt.HealSamples.RecordHealSamples(ctx, record)
}

// waitBeforeStep 应用叶步骤之间的节奏等待。
func (rt *Runtime) waitBeforeStep(ctx context.Context) error {
	return rt.pacer.before(ctx, rt.StepInterval)
}

// runOperation 按 Runtime 重试策略执行一次操作。
func (rt *Runtime) runOperation(operation func() error) error {
	return Retry(rt.RetryPolicy, operation)
}

// runOperationWithAttempts 按 Runtime 重试策略执行操作并返回尝试次数。
func (rt *Runtime) runOperationWithAttempts(operation func() error) (int, error) {
	return RetryWithAttempts(rt.RetryPolicy, operation)
}

// activeOccurrence 返回节点当前调用栈顶的 Occurrence；没有活动帧时返回错误。
func (rt *Runtime) activeOccurrence(nodeID string) (int, error) {
	stack := rt.activeOccurrences[nodeID]
	if len(stack) == 0 {
		return 0, fmt.Errorf("node %s without active occurrence", nodeID)
	}
	return stack[len(stack)-1], nil
}

// mustActiveOccurrence 返回节点活动 Occurrence；无活动帧时返回 0，供尽力观测使用而不打断执行。
func (rt *Runtime) mustActiveOccurrence(nodeID string) int {
	occurrence, err := rt.activeOccurrence(nodeID)
	if err != nil {
		return 0
	}
	return occurrence
}

// releaseOccurrence 仅释放指定调用拥有的 Occurrence，具备幂等性且不会按位置误弹出嵌套同 ID 帧。
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

// beginOccurrence 记录节点 RUNNING 事件并返回新建的活动 Occurrence。
func (rt *Runtime) beginOccurrence(ctx context.Context, nodeID string) (int, error) {
	if err := rt.emit(ctx, nodeID, PhaseRunning); err != nil {
		return 0, err
	}
	return rt.activeOccurrence(nodeID)
}

// emit 记录节点阶段事件，并在成功终态提交后释放对应 Occurrence；事实端口错误会分类返回。
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

// failurePhase 根据上下文是否取消选择 FAILED 或 CANCELED 终态。
func failurePhase(ctx context.Context) Phase {
	if ctx.Err() != nil {
		return PhaseCanceled
	}
	return PhaseFailed
}

// effectiveSpec 以规范规格为基准应用本次运行的 selector overlay，并复制选择器切片。
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

// specByID 按 ID 获取规格并应用当前运行的 selector overlay。
func (rt *Runtime) specByID(id string) (fingerprint.ElementTargetSpec, bool) {
	spec, ok := rt.Specs[id]
	if !ok {
		return fingerprint.ElementTargetSpec{}, false
	}
	return rt.effectiveSpec(spec), true
}

// promoteSelector 将自愈选择器置于列表首位，保留其余有序回退项并去除相同选择器重复项。
func promoteSelector(base fingerprint.ElementTargetSpec, healed fingerprint.Selector) fingerprint.ElementTargetSpec {
	selectors := make([]fingerprint.Selector, 0, len(base.Selectors)+1)
	selectors = append(selectors, healed)
	for _, selector := range base.Selectors {
		if selector != healed {
			selectors = append(selectors, selector)
		}
	}
	spec := base
	spec.Selectors = selectors
	return spec
}

// setSelectorOverlay 保存规格选择器的副本，供本次 Runtime 后续定位使用。
func (rt *Runtime) setSelectorOverlay(spec fingerprint.ElementTargetSpec) {
	if rt.SelectorOverlay == nil {
		rt.SelectorOverlay = make(map[string][]fingerprint.Selector)
	}
	rt.SelectorOverlay[spec.ID] = append([]fingerprint.Selector(nil), spec.Selectors...)
}
