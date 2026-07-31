package node

import (
	"context"
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestRetryWithAttemptsBoundaryMatrix(t *testing.T) {
	cases := []struct {
		name     string
		policy   RetryPolicy
		failures int
		wantCall int
		wantErr  bool
		wantCode fault.Code
	}{
		{name: "zero attempts normalizes to one", policy: RetryPolicy{}, failures: 1, wantCall: 1, wantErr: true, wantCode: CodeTransientDriver},
		{name: "negative attempts normalizes to one", policy: RetryPolicy{Attempts: -2}, failures: 1, wantCall: 1, wantErr: true, wantCode: CodeTransientDriver},
		{name: "success on first", policy: RetryPolicy{Attempts: 3}, failures: 0, wantCall: 1},
		{name: "success on final", policy: RetryPolicy{Attempts: 3}, failures: 2, wantCall: 3},
		{name: "exhausted", policy: RetryPolicy{Attempts: 3}, failures: 3, wantCall: 3, wantErr: true, wantCode: CodeTransientDriver},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			gotCalls, err := RetryWithAttempts(tc.policy, func() error {
				calls++
				if calls <= tc.failures {
					return transientDriverFault(errors.New("temporary"))
				}
				return nil
			})
			if calls != tc.wantCall || gotCalls != tc.wantCall || (err != nil) != tc.wantErr {
				t.Fatalf("calls=%d reported=%d err=%v", calls, gotCalls, err)
			}
			if tc.wantErr && !fault.IsCode(err, tc.wantCode) {
				t.Fatalf("code=%q", nodeFaultCode(err))
			}
		})
	}
}

func TestRetryWithAttemptsRejectsNonTransientFaults(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "plain error", err: errors.New("driver failed")},
		{name: "not found", err: classifyNodeFault(NewElementNotFoundError())},
		{name: "canceled", err: classifyNodeFault(context.Canceled)},
		{name: "timeout", err: classifyNodeFault(context.DeadlineExceeded)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := RetryWithAttempts(RetryPolicy{Attempts: 3}, func() error {
				calls++
				return tc.err
			})
			if calls != 1 || !errors.Is(err, tc.err) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
