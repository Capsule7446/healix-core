package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// Locator 集中处理单页面元素定位及运行时选择器覆盖。
type Locator interface {
	// Locate 按元素目标规格定位元素。
	Locate(context.Context, fingerprint.ElementTargetSpec) (Element, error)
}

// Reader 集中提供等待和断言使用的浏览器无关元素读取。
type Reader interface {
	// Exists 判断元素是否存在。
	Exists(context.Context, Element) (bool, error)
	// Visible 判断元素是否可见。
	Visible(context.Context, Element) (bool, error)
	// Text 返回元素文本。
	Text(context.Context, Element) (string, error)
	// Attribute 返回元素属性值及是否存在标志。
	Attribute(context.Context, Element, string) (string, bool, error)
}

// Poller 集中执行有界条件轮询，临时未找到或瞬态驱动错误会保留到超时诊断。
type Poller struct {
	Interval time.Duration
}

// Run 在给定超时内轮询 condition，并分类取消、超时和可重试错误。
func (p Poller) Run(ctx context.Context, timeout time.Duration, condition func(context.Context) (bool, error)) error {
	if timeout <= 0 {
		timeout = DefaultWaitTimeout
	}
	if p.Interval <= 0 {
		p.Interval = waitPollInterval
	}
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		satisfied, err := condition(pollCtx)
		if err == nil && satisfied {
			return nil
		}
		if err != nil {
			if fault.IsCode(err, CodeElementNotFound) || isExclusiveTransientDriverFault(err) {
				lastErr = err
			} else {
				return err
			}
		}
		select {
		case <-ticker.C:
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return fmt.Errorf("poll canceled: %w", classifyNodeFault(ctx.Err()))
			}
			if lastErr != nil {
				cause := errors.Join(lastErr, pollCtx.Err())
				return fmt.Errorf("poll timeout after %s: %w", timeout, mustWrapNodeFault(cause, fault.DeadlineExceeded, CodeTimeout, "node operation timed out"))
			}
			return fmt.Errorf("poll timeout after %s: %w", timeout, mustWrapNodeFault(pollCtx.Err(), fault.DeadlineExceeded, CodeTimeout, "node operation timed out"))
		}
	}
}

// OperationRunner 将有限重试策略应用于一次浏览器操作。
type OperationRunner struct {
	Policy RetryPolicy
}

// Run 按策略执行操作并返回尝试次数及最终错误。
func (r OperationRunner) Run(operation func() error) (int, error) {
	return RetryWithAttempts(r.Policy, operation)
}

// elementReader 将 Element 方法适配为 Reader 端口。
type elementReader struct{}

// Exists 委托 Element.Exists。
func (elementReader) Exists(ctx context.Context, element Element) (bool, error) {
	return element.Exists(ctx)
}

// Visible 委托 Element.Visible。
func (elementReader) Visible(ctx context.Context, element Element) (bool, error) {
	return element.Visible(ctx)
}

// Text 委托 Element.Text。
func (elementReader) Text(ctx context.Context, element Element) (string, error) {
	return element.Text(ctx)
}

// Attribute 委托 Element.Attribute。
func (elementReader) Attribute(ctx context.Context, element Element, name string) (string, bool, error) {
	return element.Attribute(ctx, name)
}

// locator 返回绑定当前 Runtime 的定位器适配器。
func (rt *Runtime) locator() Locator {
	return runtimeLocator{runtime: rt}
}

// reader 返回浏览器无关元素读取适配器。
func (rt *Runtime) reader() Reader { return elementReader{} }

// poller 返回使用 node 默认轮询间隔的 Poller。
func (rt *Runtime) poller() Poller { return Poller{Interval: waitPollInterval} }

// operationRunner 返回使用 Runtime 重试策略的操作执行器。
func (rt *Runtime) operationRunner() OperationRunner { return OperationRunner{Policy: rt.RetryPolicy} }

// runtimeLocator 将 Runtime 驱动和有效规格适配为 Locator。
type runtimeLocator struct{ runtime *Runtime }

// Locate 校验驱动依赖后委托 Runtime 的有效元素规格执行定位。
func (l runtimeLocator) Locate(ctx context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
	if l.runtime == nil || l.runtime.Driver == nil {
		// 缺失驱动与缺失工作流调用目标具有相同修复方式，统一归入步骤配置错误并记录一个违规。
		return nil, stepConfigurationInvalidError(mustViolation(fault.CodeFieldRequired, "driver", "node driver is required"))
	}
	return l.runtime.Driver.Locate(ctx, l.runtime.effectiveSpec(spec))
}
