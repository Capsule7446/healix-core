package node

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// WaitKind 是 WaitNode 的等待条件类型。
type WaitKind string

const (
	// WaitSleep 固定时长等待（duration）。
	WaitSleep WaitKind = "sleep"
	// WaitElement 轮询等待某个元素可被定位，直到成功或超时。
	WaitElement WaitKind = "element"
	// WaitElementVisible 等待元素存在且可见。
	WaitElementVisible WaitKind = "element_visible"
	// WaitElementInvisible 等待元素不可见或已从 DOM 移除。
	WaitElementInvisible WaitKind = "element_invisible"
	// WaitNetworkIdle 等待页面网络空闲，直到满足或超时。
	WaitNetworkIdle WaitKind = "network_idle"
)

// 时的默认上限。
const DefaultWaitTimeout = 10 * time.Second

// waitPollInterval 是 WaitElement 的轮询间隔。
const waitPollInterval = 100 * time.Millisecond

// WaitNode 暂停执行，直到条件满足：固定时长（sleep）、元素出现（element）
// 或网络空闲（network_idle）。条件等待有自己独立的 Timeout，与外层 step
// 超时无关——超时未满足条件即判失败。
type WaitNode struct {
	NodeID   string
	Kind     WaitKind
	Duration time.Duration        // WaitSleep：等待时长
	Target   fingerprint.NodeSpec // WaitElement：要等的元素
	Timeout  time.Duration        // WaitElement/WaitNetworkIdle：条件超时，0 用 DefaultWaitTimeout
}

func (w *WaitNode) ID() string { return w.NodeID }

func (w *WaitNode) Validate() error {
	switch w.Kind {
	case "", WaitSleep:
		if w.Duration < 0 || w.Timeout != 0 {
			return fmt.Errorf("invalid sleep wait configuration")
		}
	case WaitElement:
		if w.Duration != 0 || w.Timeout < 0 {
			return fmt.Errorf("invalid element wait configuration")
		}
	case WaitElementVisible, WaitElementInvisible:
		if w.Duration != 0 || w.Timeout < 0 {
			return fmt.Errorf("invalid visibility wait configuration")
		}
	case WaitNetworkIdle:
		if w.Duration != 0 || w.Timeout < 0 {
			return fmt.Errorf("invalid network idle wait configuration")
		}
	default:
		return fmt.Errorf("unknown wait kind %q", w.Kind)
	}
	return nil
}

func (w *WaitNode) Run(ctx context.Context, rt *Runtime) (runErr error) {
	if err := w.Validate(); err != nil {
		return fmt.Errorf("wait %s: validate: %w", w.NodeID, err)
	}
	if err := rt.waitBeforeStep(ctx); err != nil {
		return fmt.Errorf("wait %s: wait step interval: %w", w.NodeID, err)
	}
	occurrence, err := rt.beginOccurrence(ctx, w.NodeID)
	if err != nil {
		return fmt.Errorf("wait %s: enter running phase: %w", w.NodeID, err)
	}
	defer rt.releaseOccurrence(w.NodeID, occurrence)
	lifecycle, err := rt.beginLeafLifecycle(ctx, w.NodeID, "WAIT", occurrence)
	if err != nil {
		if emitErr := rt.emitTerminal(ctx, w.NodeID, failurePhase(ctx)); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	defer func() { runErr = lifecycle.Complete(ctx, runErr) }()

	started := time.Now()
	err = nil
	switch w.Kind {
	case WaitSleep, "":
		err = w.sleep(ctx)
	case WaitElement:
		err = w.waitElement(ctx, rt, false, false)
	case WaitElementVisible:
		err = w.waitElement(ctx, rt, true, false)
	case WaitElementInvisible:
		err = w.waitElement(ctx, rt, false, true)
	case WaitNetworkIdle:
		err = w.waitNetworkIdle(ctx, rt)
	default:
		err = fmt.Errorf("unknown wait kind %q", w.Kind)
	}

	rt.observeOperationBestEffort(ctx, OperationObservation{RunID: rt.RunID, NodeID: w.NodeID, Operation: string(w.Kind), Attempt: 1, DurationMS: time.Since(started).Milliseconds(), Succeeded: err == nil, FaultKind: nodeFaultKind(err), FaultCode: nodeFaultCode(err)})
	if err != nil {
		if emitErr := rt.emitTerminal(ctx, w.NodeID, failurePhase(ctx)); emitErr != nil {
			return errors.Join(fmt.Errorf("wait %s: %w", w.NodeID, err), emitErr)
		}
		return fmt.Errorf("wait %s: %w", w.NodeID, err)
	}
	if err := rt.emit(ctx, w.NodeID, PhaseSucceeded); err != nil {
		return fmt.Errorf("wait %s: enter succeeded phase: %w", w.NodeID, err)
	}
	return nil
}

func (w *WaitNode) sleep(ctx context.Context) error {
	select {
	case <-time.After(w.Duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *WaitNode) timeout() time.Duration {
	if w.Timeout > 0 {
		return w.Timeout
	}
	return DefaultWaitTimeout
}

func (w *WaitNode) waitElement(ctx context.Context, rt *Runtime, requireVisible, requireInvisible bool) error {
	return rt.poller().Run(ctx, w.timeout(), func(pollCtx context.Context) (bool, error) {
		el, err := rt.locator().Locate(pollCtx, w.Target)
		if err != nil {
			if errors.Is(err, ErrElementNotFound) && requireInvisible {
				return true, nil
			}
			return false, err
		}
		if !requireVisible && !requireInvisible {
			return true, nil
		}
		visible, visibleErr := rt.reader().Visible(pollCtx, el)
		if visibleErr != nil {
			return false, visibleErr
		}
		if (requireVisible && visible) || (requireInvisible && !visible) {
			return true, nil
		}
		return false, nil
	})
}

func (w *WaitNode) waitNetworkIdle(ctx context.Context, rt *Runtime) error {
	ctx, cancel := context.WithTimeout(ctx, w.timeout())
	defer cancel()
	if err := rt.Driver.WaitNetworkIdle(ctx); err != nil {
		return fmt.Errorf("network not idle within %s: %w", w.timeout(), err)
	}
	return nil
}

// RepeatNode 按顺序把 Children 运行 Times 次，一旦某个子节点出错就中止。
type RepeatNode struct {
	NodeID   string
	Times    int
	Children []Node
}

func (r *RepeatNode) ID() string { return r.NodeID }

func (r *RepeatNode) Run(ctx context.Context, rt *Runtime) error {
	if r.Times < 0 {
		return fmt.Errorf("repeat %s: times cannot be negative", r.NodeID)
	}
	for i, child := range r.Children {
		if child == nil {
			return fmt.Errorf("repeat %s: child %d is nil", r.NodeID, i)
		}
	}
	occurrence, err := rt.beginOccurrence(ctx, r.NodeID)
	if err != nil {
		return fmt.Errorf("repeat %s: enter running phase: %w", r.NodeID, err)
	}
	defer rt.releaseOccurrence(r.NodeID, occurrence)
	for i := 0; i < r.Times; i++ {
		for _, c := range r.Children {
			if err := c.Run(ctx, rt); err != nil {
				if emitErr := rt.emitTerminal(ctx, r.NodeID, failurePhase(ctx)); emitErr != nil {
					return errors.Join(fmt.Errorf("repeat %s iteration %d: %w", r.NodeID, i, err), emitErr)
				}
				return fmt.Errorf("repeat %s iteration %d: %w", r.NodeID, i, err)
			}
		}
	}
	if err := rt.emit(ctx, r.NodeID, PhaseSucceeded); err != nil {
		return fmt.Errorf("repeat %s: enter succeeded phase: %w", r.NodeID, err)
	}
	return nil
}

// WorkflowNode 按顺序运行 Children。Application 编译器从锁定的 Workspace
// 版本快照构造它；它也是 FlowFragment 相互引用时被引用的不可变执行单元。
// FlowFragment 按顺序执行；跨 FlowFragment 调度由应用层负责。
type WorkflowNode struct {
	NodeID             string
	Children           []Node
	OwnsParameterScope bool
	Parameters         map[string]parameter.Value
}

func (w *WorkflowNode) ID() string { return w.NodeID }

func (w *WorkflowNode) Run(ctx context.Context, rt *Runtime) error {
	for i, child := range w.Children {
		if child == nil {
			return fmt.Errorf("workflow %s: child %d is nil", w.NodeID, i)
		}
	}
	previous := rt.parameterScope
	if w.OwnsParameterScope {
		rt.parameterScope = cloneParameterScope(w.Parameters)
		if rt.parameterScope == nil {
			rt.parameterScope = map[string]parameter.Value{}
		}
		defer func() { rt.parameterScope = previous }()
	}
	occurrence, err := rt.beginOccurrence(ctx, w.NodeID)
	if err != nil {
		return fmt.Errorf("workflow %s: enter running phase: %w", w.NodeID, err)
	}
	defer rt.releaseOccurrence(w.NodeID, occurrence)
	for _, c := range w.Children {
		if err := c.Run(ctx, rt); err != nil {
			if emitErr := rt.emitTerminal(ctx, w.NodeID, failurePhase(ctx)); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}
	}
	if err := rt.emit(ctx, w.NodeID, PhaseSucceeded); err != nil {
		return fmt.Errorf("workflow %s: enter succeeded phase: %w", w.NodeID, err)
	}
	return nil
}

// WorkflowCallNode 在子工作流调用期间应用参考边参数绑定。当每个调用接收一个隔离的参数范围时，引用的不可变 WorkflowNode 保持可重用。
type WorkflowCallNode struct {
	NodeID      string
	Target      *WorkflowNode
	Bindings    map[string]parameter.Binding
	Values      map[string]parameter.Value
	Constraints map[string]parameter.Constraint
}

func (w *WorkflowCallNode) ID() string {
	if w.NodeID != "" {
		return w.NodeID
	}
	if w.Target == nil {
		return ""
	}
	return w.Target.ID()
}

func (w *WorkflowCallNode) Run(ctx context.Context, rt *Runtime) error {
	if w.Target == nil {
		return errors.New("workflow call target is required")
	}
	id := w.ID()
	occurrence, err := rt.beginOccurrence(ctx, id)
	if err != nil {
		return fmt.Errorf("workflow call %s: enter running phase: %w", id, err)
	}
	defer rt.releaseOccurrence(id, occurrence)
	err = w.runTarget(ctx, rt)
	if err != nil {
		if emitErr := rt.emitTerminal(ctx, id, failurePhase(ctx)); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	if err := rt.emit(ctx, id, PhaseSucceeded); err != nil {
		return fmt.Errorf("workflow call %s: enter succeeded phase: %w", id, err)
	}
	return nil
}

func (w *WorkflowCallNode) runTarget(ctx context.Context, rt *Runtime) error {
	resolved := make(map[string]parameter.Value, len(w.Values))
	for name, value := range w.Values {
		constraint, exists := w.Constraints[name]
		if !exists {
			return fmt.Errorf("workflow %s parameter %s has no schema", w.Target.ID(), name)
		}
		if err := constraint.Validate(value); err != nil {
			return fmt.Errorf("workflow %s parameter %s: %w", w.Target.ID(), name, err)
		}
		resolved[name] = value.Clone()
	}
	for name := range w.Constraints {
		if _, exists := resolved[name]; !exists {
			return fmt.Errorf("workflow %s parameter %s is missing", w.Target.ID(), name)
		}
	}
	for name, value := range rt.parameterScope {
		if strings.HasPrefix(name, "env.") {
			resolved[name] = value.Clone()
		}
	}
	previous := rt.parameterScope
	rt.parameterScope = resolved
	defer func() { rt.parameterScope = previous }()
	return w.Target.Run(ctx, rt)
}

func cloneParameterScope(source map[string]parameter.Value) map[string]parameter.Value {
	if source == nil {
		return nil
	}
	result := make(map[string]parameter.Value, len(source))
	for name, value := range source {
		result[name] = value.Clone()
	}
	return result
}
