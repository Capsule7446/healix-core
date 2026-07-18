package node

import (
	"context"
	"time"
)

type stepIntervalWait func(context.Context, time.Duration) error

type stepPacer struct {
	started bool
	wait    stepIntervalWait
}

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
