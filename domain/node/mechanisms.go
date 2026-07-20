package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// Locator centralizes single-page resolution and the runtime selector overlay.
type Locator interface {
	Locate(context.Context, fingerprint.NodeSpec) (Element, error)
}

// Reader centralizes browser-neutral element reads used by waits and assertions.
type Reader interface {
	Exists(context.Context, Element) (bool, error)
	Visible(context.Context, Element) (bool, error)
	Text(context.Context, Element) (string, error)
	Attribute(context.Context, Element, string) (string, bool, error)
}

// Poller centralizes bounded condition polling. The condition returns satisfied,
// observation, and error; observation is retained for timeout diagnostics.
type Poller struct {
	Interval time.Duration
}

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
			if errors.Is(err, ErrElementNotFound) {
				lastErr = err
			} else if IsTransient(err) {
				lastErr = err
			} else {
				return err
			}
		}
		select {
		case <-ticker.C:
		case <-pollCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("poll timeout after %s: %w", timeout, &ClassifiedError{Kind: ErrorTimeout, Operation: "poll", Err: errors.Join(lastErr, pollCtx.Err())})
			}
			return fmt.Errorf("poll timeout after %s: %w", timeout, &ClassifiedError{Kind: ErrorTimeout, Operation: "poll", Err: pollCtx.Err()})
		}
	}
}

// OperationRunner applies the finite retry policy to one browser operation.
type OperationRunner struct {
	Policy RetryPolicy
}

func (r OperationRunner) Run(operation func() error) (int, error) {
	return RetryWithAttempts(r.Policy, operation)
}

type elementReader struct{}

func (elementReader) Exists(ctx context.Context, element Element) (bool, error) {
	return element.Exists(ctx)
}
func (elementReader) Visible(ctx context.Context, element Element) (bool, error) {
	return element.Visible(ctx)
}
func (elementReader) Text(ctx context.Context, element Element) (string, error) {
	return element.Text(ctx)
}
func (elementReader) Attribute(ctx context.Context, element Element, name string) (string, bool, error) {
	return element.Attribute(ctx, name)
}

func (rt *Runtime) locator() Locator {
	return runtimeLocator{runtime: rt}
}

func (rt *Runtime) reader() Reader                   { return elementReader{} }
func (rt *Runtime) poller() Poller                   { return Poller{Interval: waitPollInterval} }
func (rt *Runtime) operationRunner() OperationRunner { return OperationRunner{Policy: rt.RetryPolicy} }

type runtimeLocator struct{ runtime *Runtime }

func (l runtimeLocator) Locate(ctx context.Context, spec fingerprint.NodeSpec) (Element, error) {
	if l.runtime == nil || l.runtime.Driver == nil {
		return nil, errors.New("node: locator driver is required")
	}
	return l.runtime.Driver.Locate(ctx, l.runtime.effectiveSpec(spec))
}

func IsTransient(err error) bool {
	var classified *ClassifiedError
	return errors.As(err, &classified) && classified.Kind == ErrorTransientDriver
}
