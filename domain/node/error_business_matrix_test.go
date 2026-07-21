package node

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryWithAttemptsBoundaryMatrix(t *testing.T) {
	cases := []struct {
		name     string
		policy   RetryPolicy
		failures int
		wantCall int
		wantErr  bool
		wantKind ErrorKind
	}{
		{name: "zero attempts normalizes to one", policy: RetryPolicy{}, failures: 1, wantCall: 1, wantErr: true, wantKind: ErrorTransientDriver},
		{name: "negative attempts normalizes to one", policy: RetryPolicy{Attempts: -2}, failures: 1, wantCall: 1, wantErr: true, wantKind: ErrorTransientDriver},
		{name: "success on first", policy: RetryPolicy{Attempts: 3}, failures: 0, wantCall: 1},
		{name: "success on final", policy: RetryPolicy{Attempts: 3}, failures: 2, wantCall: 3},
		{name: "exhausted", policy: RetryPolicy{Attempts: 3}, failures: 3, wantCall: 3, wantErr: true, wantKind: ErrorTransientDriver},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			gotCalls, err := RetryWithAttempts(tc.policy, func() error {
				calls++
				if calls <= tc.failures {
					return TransientError("operation", errors.New("temporary"))
				}
				return nil
			})
			if calls != tc.wantCall || gotCalls != tc.wantCall || (err != nil) != tc.wantErr {
				t.Fatalf("calls=%d reported=%d err=%v", calls, gotCalls, err)
			}
			if tc.wantErr && errorKind(err) != tc.wantKind {
				t.Fatalf("kind=%s", errorKind(err))
			}
		})
	}
}

func TestClassifyErrorBoundaryMatrix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, deadlineCancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer deadlineCancel()
	<-deadline.Done()
	cases := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{name: "nil", err: nil, want: ""},
		{name: "canceled", err: ctx.Err(), want: ErrorContextClosed},
		{name: "deadline", err: deadline.Err(), want: ErrorTimeout},
		{name: "wrapped not found", err: errors.Join(errors.New("context"), ErrElementNotFound), want: ErrorNotFound},
		{name: "already classified", err: TransientError("driver", errors.New("retry")), want: ErrorTransientDriver},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if errorKind(ClassifyError("test", tc.err)) != tc.want {
				t.Fatalf("kind=%s", errorKind(ClassifyError("test", tc.err)))
			}
		})
	}
}
