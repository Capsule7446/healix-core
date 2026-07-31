package node

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/interpolation"
	"github.com/Capsule7446/healix-core/domain/parameter"
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
	Target   fingerprint.ElementTargetSpec
	Action   Action
	Timeout  time.Duration
	Optional bool
}

func (s *StepNode) ID() string { return s.NodeID }

func validateNavigationURL(value string) error {
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return errors.New("control characters are not allowed")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("absolute URL is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.User != nil {
		return errors.New("userinfo is not allowed")
	}
	return nil
}

func (s *StepNode) Run(ctx context.Context, rt *Runtime) (runErr error) {
	if err := rt.waitBeforeStep(ctx); err != nil {
		return classifyNodeFault(err)
	}
	parentCtx := ctx
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	execution := NewStepExecution(s.NodeID)
	if err := s.transition(ctx, rt, execution, PhaseRunning); err != nil {
		return classifyStepPhaseTransitionInvalid(err)
	}
	defer rt.releaseOccurrence(execution.nodeID, execution.occurrence)
	lifecycle, err := rt.beginLeafLifecycle(ctx, s.NodeID, "STEP", execution.occurrence)
	if err != nil {
		return s.fail(ctx, parentCtx, rt, execution, err)
	}
	defer func() { runErr = lifecycle.Complete(parentCtx, runErr) }()

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
		if err := validateNavigationURL(action.Value); err != nil {
			return s.fail(ctx, parentCtx, rt, execution, wrapStepConfigurationInvalidError(err, mustViolation(fault.CodeFieldInvalid, "action.value", "navigation url is invalid")))
		}
		started := time.Now()
		attempts, err := rt.operationRunner().Run(func() error { return rt.Driver.Navigate(ctx, action.Value) })
		rt.observeOperationBestEffort(ctx, OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: string(action.Kind), Attempt: attempts, DurationMS: time.Since(started).Milliseconds(), Succeeded: err == nil, FaultKind: nodeFaultKind(err), FaultCode: nodeFaultCode(err)})
		if err != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: navigate failed: %w", s.NodeID, classifyNodeFault(err)))
		}
		return s.finish(ctx, parentCtx, rt, execution)
	}
	if action.Kind == ActionPress {
		started := time.Now()
		attempts, err := rt.operationRunner().Run(func() error { return rt.Driver.Press(ctx, action.Value) })
		rt.observeOperationBestEffort(ctx, OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: string(action.Kind), Attempt: attempts, DurationMS: time.Since(started).Milliseconds(), Succeeded: err == nil, FaultKind: nodeFaultKind(err), FaultCode: nodeFaultCode(err)})
		if err != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: press failed: %w", s.NodeID, classifyNodeFault(err)))
		}
		return s.finish(ctx, parentCtx, rt, execution)
	}

	target := s.Target
	healed := false
	var el Element
	locateStarted := time.Now()
	locateAttempts, err := rt.operationRunner().Run(func() error {
		var locateErr error
		el, locateErr = rt.locator().Locate(ctx, target)
		return locateErr
	})
	rt.observeOperationBestEffort(context.WithoutCancel(ctx), OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: "locate", Selector: firstSelector(target), Healed: false, Attempt: locateAttempts, DurationMS: time.Since(locateStarted).Milliseconds(), Succeeded: err == nil, FaultKind: nodeFaultKind(err), FaultCode: nodeFaultCode(err)})
	if err != nil {
		if !isExclusiveElementNotFound(err) {
			// Mirrors the navigate and press branches above, which already classify.
			return s.fail(ctx, parentCtx, rt, execution, classifyNodeFault(err))
		}
		if s.Optional {
			if err := s.transition(ctx, rt, execution, PhaseSucceeded); err != nil {
				return s.fail(ctx, parentCtx, rt, execution, classifyStepPhaseTransitionInvalid(err))
			}
			lifecycle.MarkSkipped()
			return nil
		}
		if rt.Healing == nil && rt.Healer == nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: locate failed and healing disabled: %w", s.NodeID, err))
		}
		if err := s.transition(ctx, rt, execution, PhaseHealing); err != nil {
			return s.fail(ctx, parentCtx, rt, execution, classifyStepPhaseTransitionInvalid(err))
		}
		el, err = s.heal(ctx, rt, target)
		if err != nil {
			return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: heal failed: %w", s.NodeID, err))
		}
		target = rt.effectiveSpec(target)
		healed = true
	}

	if err := s.transition(ctx, rt, execution, PhaseTransitioning); err != nil {
		return s.fail(ctx, parentCtx, rt, execution, classifyStepPhaseTransitionInvalid(err))
	}
	started := time.Now()
	attempts, actionErr := rt.runOperationWithAttempts(func() error { return applyAction(ctx, rt, el, action) })
	selector := fingerprint.Selector{}
	if len(target.Selectors) > 0 {
		selector = target.Selectors[0]
	}
	rt.observeOperationBestEffort(ctx, OperationObservation{RunID: rt.RunID, NodeID: s.NodeID, Operation: string(action.Kind), Selector: selector, Healed: healed, Attempt: attempts, DurationMS: time.Since(started).Milliseconds(), Succeeded: actionErr == nil, FaultKind: nodeFaultKind(actionErr), FaultCode: nodeFaultCode(actionErr)})
	if actionErr != nil {
		return s.fail(ctx, parentCtx, rt, execution, fmt.Errorf("node %s: action failed: %w", s.NodeID, classifyNodeFault(actionErr)))
	}

	return s.finish(ctx, parentCtx, rt, execution)
}

// heal 在 s.Target 中所有选择器都解析失败后被调用。它向纯算法 Healer
// 请求一个 Decision，无论结果如何都会把这次尝试记录为执行事实，并在出现可用候选时，
// 把它提到 Target 选择器列表最前面，再通过 Driver 重新定位——
// 这样调用方拿到的始终是一个普通的 Element。
func (s *StepNode) heal(ctx context.Context, rt *Runtime, target fingerprint.ElementTargetSpec) (Element, error) {
	snap, err := rt.Driver.Snapshot(ctx)
	if err != nil {
		return nil, classifyNodeFault(err)
	}

	decision, err := rt.healingPort().Recover(ctx, target, snap)
	if err != nil {
		return nil, err
	}
	if err := decision.Validate(); err != nil {
		return nil, classifyNodeFault(err)
	}
	if err := rt.recordHealSamples(ctx, HealSampleRecord{RunID: rt.RunID, NodeID: s.NodeID, SpecID: target.ID, OldSelector: firstSelector(target), Outcome: decision.Outcome, Samples: heal.SortSamples(decision.Samples(target.Fingerprint, rt.healingReviewCap()))}); err != nil {
		return nil, evidenceRecordFailedError(err)
	}
	assessment, err := heal.Assess(target, decision, heal.ExecutionContext{PageURL: rt.PageURL, Origin: rt.Origin}, rt.HealingPolicy)
	if err != nil {
		return nil, classifyNodeFault(err)
	}
	if assessment.Disposition != heal.DispositionAllow {
		if assessment.Disposition == heal.DispositionBlock && decision.Outcome == heal.OutcomeNoCandidate {
			if rt.Facts != nil {
				if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, s.NodeID, target.ID, firstSelector(target), decision); recordErr != nil {
					return nil, evidenceRecordFailedError(recordErr)
				}
			}
			return nil, healingRefusedError(fmt.Errorf("no heal candidate reached review_cap: %s", assessment.Explanation))
		}
		if assessment.Disposition == heal.DispositionBlock {
			decision.Outcome = heal.OutcomeSafetyRejected
			decision.NeedsReview = false
		}
		if rt.Facts != nil {
			oldSelector := firstSelector(target)
			if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, s.NodeID, target.ID, oldSelector, decision); recordErr != nil {
				return nil, evidenceRecordFailedError(recordErr)
			}
		}
		return nil, healingRefusedError(fmt.Errorf("healing refused: %s", assessment.Explanation))
	}
	if decision.Outcome == heal.OutcomeNoCandidate || decision.Best == nil {
		if rt.Facts != nil {
			if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, s.NodeID, target.ID, firstSelector(target), decision); recordErr != nil {
				return nil, evidenceRecordFailedError(recordErr)
			}
		}
		return nil, healingRefusedError(errors.New("no heal candidate reached review_cap"))
	}

	healedSpec := target
	healedSpec.Selectors = append([]fingerprint.Selector{decision.Best.Selector}, healedSpec.Selectors...)

	el, err := rt.Driver.Locate(ctx, healedSpec)
	if err != nil {
		return nil, fmt.Errorf("re-locate after heal: %w", err)
	}

	if rt.Facts != nil {
		if recordErr := rt.Facts.StageHealDecision(ctx, domainexecution.WorkerFence{RunID: rt.RunID, ClaimToken: rt.ClaimToken}, s.NodeID, target.ID, firstSelector(target), decision); recordErr != nil {
			return nil, evidenceRecordFailedError(recordErr)
		}
	}
	rt.setSelectorOverlay(healedSpec)

	return el, nil
}

func firstSelector(spec fingerprint.ElementTargetSpec) fingerprint.Selector {
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
			return stepConfigurationInvalidError(mustViolation(fault.CodeFieldRequired, "action.values", "select action requires a value"))
		}
		return el.Select(ctx, values[0], values[1:]...)
	case ActionHover:
		return el.Hover(ctx)
	case ActionExtract:
		if a.Value == "" {
			return stepConfigurationInvalidError(mustViolation(fault.CodeFieldRequired, "action.value", "extract action requires a variable name"))
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
		return wrapStepConfigurationInvalidError(fmt.Errorf("unknown action kind %q", a.Kind), mustViolation(fault.CodeFieldInvalid, "action.kind", "action kind is not supported"))
	}
}

func (s *StepNode) finish(ctx, parentCtx context.Context, rt *Runtime, execution *StepExecution) error {
	if err := s.transition(ctx, rt, execution, PhaseSucceeded); err != nil {
		return s.fail(ctx, parentCtx, rt, execution, classifyStepPhaseTransitionInvalid(err))
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
	if next == PhaseRunning {
		occurrence, err := rt.activeOccurrence(s.NodeID)
		if err != nil {
			return err
		}
		execution.occurrence = occurrence
	}
	return execution.Transition(next)
}

func (s *StepNode) fail(ctx, parentCtx context.Context, rt *Runtime, execution *StepExecution, cause error) error {
	terminal := PhaseFailed
	if parentCtx.Err() != nil {
		terminal = PhaseCanceled
	}
	if err := execution.CanTransition(terminal); err != nil {
		return errors.Join(cause, classifyStepPhaseTransitionInvalid(err))
	}
	if err := rt.emitTerminal(ctx, s.NodeID, terminal); err != nil {
		return errors.Join(cause, classifyStepPhaseTransitionInvalid(err))
	}
	if err := execution.Transition(terminal); err != nil {
		return errors.Join(cause, classifyStepPhaseTransitionInvalid(err))
	}
	return cause
}

type runtimeVariables struct{ rt *Runtime }

func (v runtimeVariables) Variable(name string) (string, bool) {
	parameterName := name
	if strings.HasPrefix(name, "params.") {
		parameterName = strings.TrimPrefix(name, "params.")
	}
	if value, ok := v.rt.parameterScope[parameterName]; ok {
		switch value.Type() {
		case parameter.Text:
			return value.Text(), true
		case parameter.Number:
			return value.Number(), true
		case parameter.Boolean:
			return strconv.FormatBool(value.Boolean()), true
		case parameter.SingleSelect:
			return value.SingleSelect(), true
		case parameter.MultiSelect:
			return "", false
		}
	}
	value, ok := v.rt.Scratchpad[name]
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	return s, ok
}
