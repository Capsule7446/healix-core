package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/interpolation"
)

// Node 是 workflow 的 step 树的执行单元——一个封闭的判别联合：
// *StepNode、*WaitNode、*RepeatNode、*WorkflowNode。
type Node interface {
	ID() string
	Run(ctx context.Context, rt *Runtime) error
}

// ActionKind 是 Step 在其定位到的元素上执行的动作类型。
type ActionKind string

const (
	ActionClick    ActionKind = "click"
	ActionInput    ActionKind = "input"
	ActionSelect   ActionKind = "select" // 按可见文本选中 <select> 的 option
	ActionHover    ActionKind = "hover"
	ActionNavigate ActionKind = "navigate"
	ActionPress    ActionKind = "press"
	ActionNoop     ActionKind = "noop" // 只定位（+断言），不做交互
	// ActionExtract 读取定位到的元素文本，存入 Scratchpad 中以 Value 命名的
	// 变量，供后续 step 用 ${name} 引用——跨站点流程（A 站取单号、B 站处理）
	// 的数据交接依赖它。
	ActionExtract ActionKind = "extract"
)

// Action 是 Step 唯一的副作用。input/navigate 的 Value 支持 ${var} 插值，
// 引用 Scratchpad 中的变量。
type Action struct {
	Kind   ActionKind
	Value  string   // 输入文本、navigate 的 URL，或 extract 的目标变量名
	Values []string // select 多选值；为空时退化使用 Value
}

// StepNode 通过 Target 定位一个元素并执行 Action；业务验证由独立 ValidationNode 表达。
// ActionNavigate 会完全跳过 Target/定位——它作用于整个页面，而非某个元素。
type StepNode struct {
	NodeID   string
	Target   fingerprint.NodeSpec
	Action   Action
	Timeout  time.Duration
	Optional bool
}

func (s *StepNode) ID() string { return s.NodeID }

func (s *StepNode) Run(ctx context.Context, rt *Runtime) error {
	if err := rt.waitBeforeStep(ctx); err != nil {
		return fmt.Errorf("node %s: wait step interval: %w", s.NodeID, err)
	}
	parentCtx := ctx
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	execution := NewStepExecution(s.NodeID)
	if err := s.transition(ctx, rt, execution, PhaseRunning); err != nil {
		return fmt.Errorf("node %s: enter running phase: %w", s.NodeID, err)
	}

	action := s.Action
	action.Values = append([]string(nil), s.Action.Values...)
	if action.Kind == ActionNavigate || action.Kind == ActionInput || action.Kind == ActionSelect || action.Kind == ActionPress {
		variables := runtimeVariables{rt: rt}
		v, err := interpolation.Expand(action.Value, variables)
		if err != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: %w", s.NodeID, err))
		}
		action.Value = v
		for i, value := range action.Values {
			expanded, err := interpolation.Expand(value, variables)
			if err != nil {
				return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: %w", s.NodeID, err))
			}
			action.Values[i] = expanded
		}
	}

	if action.Kind == ActionNavigate {
		started := time.Now()
		attempts, err := rt.operationRunner().Run(func() error { return rt.Driver.Navigate(ctx, action.Value) })
		observationErr := rt.observeOperation(context.WithoutCancel(ctx), OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: string(action.Kind), Attempt: attempts, DurationMS: time.Since(started).Milliseconds(), Succeeded: err == nil, ErrorKind: errorKind(err)})
		if err != nil {
			return s.fail(ctx, parentCtx, rt, execution, errors.Join(fmt.Errorf("node %s: navigate failed: %w", s.NodeID, ClassifyError("navigate", err)), observationErr))
		}
		if observationErr != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: record navigate observation: %w", s.NodeID, observationErr))
		}
		return s.finish(ctx, parentCtx, rt, execution)
	}
	if action.Kind == ActionPress {
		started := time.Now()
		attempts, err := rt.operationRunner().Run(func() error { return rt.Driver.Press(ctx, action.Value) })
		observationErr := rt.observeOperation(context.WithoutCancel(ctx), OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: string(action.Kind), Attempt: attempts, DurationMS: time.Since(started).Milliseconds(), Succeeded: err == nil, ErrorKind: errorKind(err)})
		if err != nil {
			return s.fail(ctx, parentCtx, rt, execution, errors.Join(fmt.Errorf("node %s: press failed: %w", s.NodeID, ClassifyError("press", err)), observationErr))
		}
		if observationErr != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: record press observation: %w", s.NodeID, observationErr))
		}
		return s.finish(ctx, parentCtx, rt, execution)
	}

	target := rt.effectiveSpec(s.Target)
	healed := false
	var el Element
	locateStarted := time.Now()
	locateAttempts, err := rt.operationRunner().Run(func() error {
		var locateErr error
		el, locateErr = rt.locator().Locate(ctx, target)
		return locateErr
	})
	rt.observeOperationBestEffort(context.WithoutCancel(ctx), OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: "locate", Selector: firstSelector(target), Healed: false, Attempt: locateAttempts, DurationMS: time.Since(locateStarted).Milliseconds(), Succeeded: err == nil, ErrorKind: errorKind(err)})
	if err != nil {
		if !errors.Is(err, ErrElementNotFound) {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: locate failed: %w", s.NodeID, err))
		}
		if s.Optional {
			if err := s.transition(ctx, rt, execution, PhaseSucceeded); err != nil {
				return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: skip optional step: %w", s.NodeID, err))
			}
			return nil
		}
		if rt.Healer == nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: locate failed and healing disabled: %w", s.NodeID, err))
		}
		if err := s.transition(ctx, rt, execution, PhaseHealing); err != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: enter healing phase: %w", s.NodeID, err))
		}
		el, err = s.heal(ctx, rt, target)
		if err != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: heal failed: %w", s.NodeID, err))
		}
		target = rt.effectiveSpec(target)
		healed = true
	}

	if err := s.transition(ctx, rt, execution, PhaseTransitioning); err != nil {
		return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: enter transitioning phase: %w", s.NodeID, err))
	}
	started := time.Now()
	attempts, actionErr := rt.runOperationWithAttempts(func() error { return applyAction(ctx, rt, el, action) })
	selector := fingerprint.Selector{}
	if len(target.Selectors) > 0 {
		selector = target.Selectors[0]
	}
	observationErr := rt.observeOperation(context.WithoutCancel(ctx), OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: string(action.Kind), Selector: selector, Healed: healed, Attempt: attempts, DurationMS: time.Since(started).Milliseconds(), Succeeded: actionErr == nil, ErrorKind: errorKind(actionErr)})
	if actionErr != nil {
		return s.fail(ctx, parentCtx, rt, execution, errors.Join(fmt.Errorf("node %s: action failed: %w", s.NodeID, ClassifyError(string(action.Kind), actionErr)), observationErr))
	}
	if observationErr != nil {
		return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: record action observation: %w", s.NodeID, observationErr))
	}

	return s.finish(ctx, parentCtx, rt, execution)
}

// heal 在 s.Target 中所有选择器都解析失败后被调用。它向纯算法 Healer
// 请求一个 Decision，无论结果如何都会把这次尝试记录为执行事实，并在出现可用候选时，
// 把它提到 Target 选择器列表最前面，再通过 Driver 重新定位——
// 这样调用方拿到的始终是一个普通的 Element。
func (s *StepNode) heal(ctx context.Context, rt *Runtime, target fingerprint.NodeSpec) (Element, error) {
	snap, err := rt.Driver.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot for healing: %w", err)
	}

	decision, err := rt.Healer.Heal(ctx, target, snap)
	if err != nil {
		return nil, err
	}
	if err := decision.Validate(); err != nil {
		return nil, fmt.Errorf("invalid heal decision: %w", err)
	}
	assessment, err := heal.Assess(target, decision, heal.ExecutionContext{PageURL: rt.PageURL, Origin: rt.Origin}, rt.HealingPolicy)
	if err != nil {
		return nil, fmt.Errorf("assess heal decision: %w", err)
	}
	if assessment.Disposition != heal.DispositionAllow {
		if assessment.Disposition == heal.DispositionBlock && decision.Outcome == heal.OutcomeNoCandidate {
			return nil, fmt.Errorf("no heal candidate reached review_cap: %s", assessment.Explanation)
		}
		if assessment.Disposition == heal.DispositionBlock {
			decision.Outcome = heal.OutcomeSafetyRejected
			decision.NeedsReview = false
		}
		if rt.Facts != nil {
			oldSelector := firstSelector(target)
			if recordErr := rt.Facts.RecordHealDecision(ctx, rt.RunID, s.NodeID, target.ID, oldSelector, decision); recordErr != nil {
				return nil, fmt.Errorf("record heal decision: %w", recordErr)
			}
		}
		return nil, fmt.Errorf("healing refused: %s", assessment.Explanation)
	}
	if rt.Facts != nil {
		oldSelector := firstSelector(target)
		if recordErr := rt.Facts.RecordHealDecision(ctx, rt.RunID, s.NodeID, target.ID, oldSelector, decision); recordErr != nil {
			return nil, fmt.Errorf("record heal decision: %w", recordErr)
		}
	}

	if decision.Outcome == heal.OutcomeNoCandidate || decision.Best == nil {
		return nil, fmt.Errorf("no heal candidate reached review_cap")
	}

	healedSpec := target
	healedSpec.Selectors = append([]fingerprint.Selector{decision.Best.Selector}, healedSpec.Selectors...)

	el, err := rt.Driver.Locate(ctx, healedSpec)
	if err != nil {
		return nil, fmt.Errorf("re-locate after heal: %w", err)
	}

	rt.setSelectorOverlay(healedSpec)

	return el, nil
}

func firstSelector(spec fingerprint.NodeSpec) fingerprint.Selector {
	if len(spec.Selectors) == 0 {
		return fingerprint.Selector{}
	}
	return spec.Selectors[0]
}

func applyAction(ctx context.Context, rt *Runtime, el Element, a Action) error {
	if err := el.WaitStable(ctx); err != nil {
		return fmt.Errorf("wait stable: %w", err)
	}
	switch a.Kind {
	case ActionClick:
		return el.Click(ctx)
	case ActionInput:
		return el.Input(ctx, a.Value)
	case ActionSelect:
		values := append([]string(nil), a.Values...)
		if len(values) == 0 {
			values = []string{a.Value}
		}
		if len(values) == 0 || values[0] == "" {
			return fmt.Errorf("select action requires value")
		}
		return el.Select(ctx, values[0], values[1:]...)
	case ActionHover:
		return el.Hover(ctx)
	case ActionExtract:
		if a.Value == "" {
			return fmt.Errorf("extract action requires a variable name in value")
		}
		txt, err := el.Text(ctx)
		if err != nil {
			return fmt.Errorf("extract text: %w", err)
		}
		if rt.Scratchpad == nil {
			rt.Scratchpad = map[string]any{}
		}
		rt.Scratchpad[a.Value] = txt
		return nil
	case ActionNoop, "":
		return nil
	default:
		return fmt.Errorf("unknown action kind %q", a.Kind)
	}
}

func (s *StepNode) finish(ctx, parentCtx context.Context, rt *Runtime, execution *StepExecution) error {
	if err := s.transition(ctx, rt, execution, PhaseSucceeded); err != nil {
		return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: enter succeeded phase: %w", s.NodeID, err))
	}
	return nil
}

func (s *StepNode) transition(ctx context.Context, rt *Runtime, execution *StepExecution, next Phase) error {
	if err := execution.CanTransition(next); err != nil {
		return err
	}
	if err := rt.emit(ctx, s.NodeID, next); err != nil {
		return err
	}
	return execution.Transition(next)
}

func (s *StepNode) fail(ctx, parentCtx context.Context, rt *Runtime, execution *StepExecution, cause error) error {
	terminal := PhaseFailed
	if parentCtx.Err() != nil {
		terminal = PhaseCanceled
	}
	if err := execution.CanTransition(terminal); err != nil {
		return errors.Join(cause, fmt.Errorf("node %s: enter %s phase: %w", s.NodeID, terminal, err))
	}
	if err := rt.emitTerminal(ctx, s.NodeID, terminal); err != nil {
		return errors.Join(cause, fmt.Errorf("node %s: enter %s phase: %w", s.NodeID, terminal, err))
	}
	if err := execution.Transition(terminal); err != nil {
		return errors.Join(cause, fmt.Errorf("node %s: enter %s phase: %w", s.NodeID, terminal, err))
	}
	return cause
}

type runtimeVariables struct{ rt *Runtime }

func (v runtimeVariables) Variable(name string) (string, bool) {
	value, ok := v.rt.Scratchpad[name]
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	return s, ok
}
