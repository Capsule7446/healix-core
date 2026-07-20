package node

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOperationRunnerUsesFiniteTransientRetries(t *testing.T) {
	attempts := 0
	runner := OperationRunner{Policy: RetryPolicy{Attempts: 3}}
	got, err := runner.Run(func() error {
		attempts++
		if attempts < 3 {
			return TransientError("locate", errors.New("temporary"))
		}
		return nil
	})
	if err != nil || got != 3 {
		t.Fatalf("attempts=%d err=%v", got, err)
	}
}

func TestPollerContinuesAfterTransientError(t *testing.T) {
	calls := 0
	err := (Poller{Interval: time.Millisecond}).Run(context.Background(), time.Second, func(context.Context) (bool, error) {
		calls++
		if calls < 2 {
			return false, TransientError("poll", errors.New("temporary"))
		}
		return true, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestPollerDoesNotRetryPermanentError(t *testing.T) {
	calls := 0
	permanent := errors.New("invalid selector")
	err := (Poller{Interval: time.Millisecond}).Run(context.Background(), time.Second, func(context.Context) (bool, error) {
		calls++
		return false, permanent
	})
	if !errors.Is(err, permanent) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
