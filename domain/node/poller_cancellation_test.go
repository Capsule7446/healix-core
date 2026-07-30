package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestPollerPreservesParentCancellationFault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (Poller{Interval: time.Millisecond}).Run(ctx, time.Second, func(context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v does not preserve cancellation", err)
	}
	if !fault.IsCode(err, CodeCanceled) {
		t.Fatalf("code=%q want=%q", nodeFaultCode(err), CodeCanceled)
	}
}

func TestPollerClassifiesOwnDeadlineAsTimeout(t *testing.T) {
	err := (Poller{Interval: time.Millisecond}).Run(context.Background(), 3*time.Millisecond, func(context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v does not preserve deadline", err)
	}
	if !fault.IsCode(err, CodeTimeout) {
		t.Fatalf("code=%q want=%q", nodeFaultCode(err), CodeTimeout)
	}
}

func TestPollerTimeoutRetainsTransientCauseWithTimeoutCode(t *testing.T) {
	driverCause := errors.New("driver temporarily unavailable")
	err := (Poller{Interval: time.Millisecond}).Run(context.Background(), 3*time.Millisecond, func(context.Context) (bool, error) {
		return false, transientDriverFault(driverCause)
	})
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, driverCause) {
		t.Fatalf("error=%v did not retain timeout and driver causes", err)
	}
	if !fault.IsCode(err, CodeTimeout) {
		t.Fatalf("code=%q want=%q", nodeFaultCode(err), CodeTimeout)
	}
}
