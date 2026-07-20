package node

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPollerPreservesParentCancellationClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (Poller{Interval: time.Millisecond}).Run(ctx, time.Second, func(context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v does not preserve cancellation", err)
	}
	if errorKind(err) != ErrorContextClosed {
		t.Fatalf("kind=%s want=%s", errorKind(err), ErrorContextClosed)
	}
}

func TestPollerClassifiesOwnDeadlineAsTimeout(t *testing.T) {
	err := (Poller{Interval: time.Millisecond}).Run(context.Background(), 3*time.Millisecond, func(context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v does not preserve deadline", err)
	}
	if errorKind(err) != ErrorTimeout {
		t.Fatalf("kind=%s want=%s", errorKind(err), ErrorTimeout)
	}
}
