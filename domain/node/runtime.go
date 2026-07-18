// 包节点定义了由主机适配器实现的可执行工作流节点、生命周期状态机和运行时端口。
package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

const terminalEventTimeout = 5 * time.Second

// ErrElementNotFound 是 Driver 合约的显式业务信号，表明 NodeSpec 的每个定位器均已耗尽。取消、格式错误的选择器和浏览器故障必须保持可区分的错误。
var ErrElementNotFound = errors.New("node: element not found")

// Program：程序是一棵按惯例不可变的可执行树加上为一个 WorkflowExecution 捕获的确切 NodeSpec 索引。编译器每次执行都会构建一个新的程序；运行时覆盖永远不会改变规格。
type Program struct {
	Root  Node
	Specs map[string]fingerprint.NodeSpec
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
	nodeID string
	phase  Phase
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
		return fmt.Errorf("node: nil step execution")
	}
	if err := ValidatePhaseTransition(e.phase, next); err != nil {
		return fmt.Errorf("node %s: invalid step phase transition %q -> %q", e.nodeID, e.phase, next)
	}
	return nil
}

// ValidatePhaseTransition 向持久性适配器公开相同的域保护，因此部分或重复写入无法产生不可能的 StepExecution 历史记录。
func ValidatePhaseTransition(current, next Phase) error {
	if _, ok := stepPhaseTransitions[current][next]; !ok {
		return fmt.Errorf("invalid step phase transition %q -> %q", current, next)
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

// Event 是一次阶段转换，由 ExecutionSink.RecordEvent 持久化。
type Event struct {
	RunID   string
	NodeID  string
	Phase   Phase
	Payload map[string]any
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

// Driver 按优先级顺序将 NodeSpec 的选择器与实时页面比对解析、
// 执行导航，并为自愈提供 DOMSnapshot。
type Driver interface {
	Navigate(ctx context.Context, url string) error
	Press(ctx context.Context, key string) error
	Locate(ctx context.Context, spec fingerprint.NodeSpec) (Element, error)
	Snapshot(ctx context.Context) (heal.DOMSnapshot, error)
	// WaitNetworkIdle 阻塞到页面网络空闲；超时通过 ctx 控制
	// （WaitNode 的条件超时）。
	WaitNetworkIdle(ctx context.Context) error
}

// Recorder 是框架无关的会话录制端口，由宿主提供适配器。
// Runtime 上的 Recorder 为 nil 表示"录屏关闭"，Start/Stop 也就不会被调用。
type Recorder interface {
	Start(ctx context.Context, runID string) error
	Stop(ctx context.Context, retain bool) error
}

// ExecutionSink 是执行期间产生的阶段、验证与自愈事实端口。
//
// 注意两个 ID 空间：nodeID 是执行树中 StepNode 的 ID；specID 属于
// fingerprint.NodeSpec（workspace 上下文中即稳定 Node 的 ID）。两者同为
// string，实现方不得互换。
type ExecutionSink interface {
	RecordEvent(ctx context.Context, evt Event) error
	RecordHealDecision(ctx context.Context, runID, nodeID, specID string, oldSelector fingerprint.Selector, decision heal.Decision) error
	RecordValidationObservation(ctx context.Context, runID string, observation ValidationObservation) error
}

// Runtime 是贯穿整棵 Node 树的、每次运行专属的执行上下文。
type Runtime struct {
	RunID string
	// StepInterval 控制可执行叶步骤之间的最小暂停时间。第一个叶子步骤立即开始；容器节点和验证组成员不消耗额外的时间间隔。
	StepInterval time.Duration
	// Specs 按 ID 索引每个 StepNode 的 NodeSpec，使断言可以引用
	// 该 step 自身目标以外的其他元素。
	Specs map[string]fingerprint.NodeSpec
	// SelectorOverlay 是本次 run 内按 NodeSpec ID 保存的 healed selector 列表。
	// 编译出的 Specs/StepNode 保持不变，同一 spec 的后续 step、repeat 和断言
	// 都通过 effectiveSpec 读取该 overlay。
	SelectorOverlay map[string][]fingerprint.Selector
	Driver          Driver
	Healer          heal.Healer   // nil = 关闭自愈
	Recorder        Recorder      // nil = 关闭录屏
	Facts           ExecutionSink // nil = 不输出执行事实
	Scratchpad      map[string]any
	pacer           stepPacer
}

func (rt *Runtime) waitBeforeStep(ctx context.Context) error {
	return rt.pacer.before(ctx, rt.StepInterval)
}

func (rt *Runtime) emit(ctx context.Context, nodeID string, phase Phase) error {
	evt := Event{RunID: rt.RunID, NodeID: nodeID, Phase: phase}
	if rt.Facts != nil {
		if err := rt.Facts.RecordEvent(ctx, evt); err != nil {
			return fmt.Errorf("record execution event %s/%s: %w", nodeID, phase, err)
		}
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

func (rt *Runtime) effectiveSpec(base fingerprint.NodeSpec) fingerprint.NodeSpec {
	spec := base
	if canonical, ok := rt.Specs[base.ID]; ok {
		spec = canonical
	}
	if selectors, ok := rt.SelectorOverlay[spec.ID]; ok {
		spec.Selectors = append([]fingerprint.Selector(nil), selectors...)
	}
	return spec
}

func (rt *Runtime) specByID(id string) (fingerprint.NodeSpec, bool) {
	spec, ok := rt.Specs[id]
	if !ok {
		return fingerprint.NodeSpec{}, false
	}
	return rt.effectiveSpec(spec), true
}

func (rt *Runtime) setSelectorOverlay(spec fingerprint.NodeSpec) {
	if rt.SelectorOverlay == nil {
		rt.SelectorOverlay = make(map[string][]fingerprint.Selector)
	}
	rt.SelectorOverlay[spec.ID] = append([]fingerprint.Selector(nil), spec.Selectors...)
}
