package node

import (
	"context"
	"time"
)

// stepIntervalWait 等待一个步骤间隔或响应上下文取消。
type stepIntervalWait func(context.Context, time.Duration) error

// stepPacer 确保首个叶步骤立即执行，后续叶步骤之间遵守间隔。
type stepPacer struct {
	started bool
	wait    stepIntervalWait
}

// before 在首个步骤放行，之后按 interval 等待；非正 interval 不产生等待。
func (p *stepPacer) before(ctx context.Context, interval time.Duration) error {
	if !p.started {
		p.started = true
		return nil
	}
	if interval <= 0 {
		return nil
	}
	wait := p.wait
	if wait == nil {
		wait = waitForStepInterval
	}
	return wait(ctx, interval)
}

// waitForStepInterval 使用可取消计时器等待指定间隔。
func waitForStepInterval(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
