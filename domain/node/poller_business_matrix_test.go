package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestPollerBusinessMatrix(t *testing.T) {
	cases := []struct {
		name      string
		condition func(*int) func(context.Context) (bool, error)
		parent    func() context.Context
		wantErr   bool
		wantCode  fault.Code
		wantCalls int
	}{
		{name: "immediate success", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return true, nil }
		}, wantCalls: 1},
		{name: "transient then success", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) {
				*c++
				if *c == 1 {
					return false, transientDriverFault(errors.New("temporary"))
				}
				return true, nil
			}
		}, wantCalls: 2},
		{name: "permanent error", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return false, errors.New("invalid") }
		}, wantErr: true, wantCalls: 1},
		{name: "timeout false condition", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return false, nil }
		}, wantErr: true, wantCode: CodeTimeout},
		{name: "parent cancellation", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return false, nil }
		}, parent: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, wantErr: true, wantCode: CodeCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			parent := context.Background()
			if tc.parent != nil {
				parent = tc.parent()
			}
			err := (Poller{Interval: time.Millisecond}).Run(parent, 5*time.Millisecond, tc.condition(&calls))
			if (err != nil) != tc.wantErr {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if tc.wantCalls > 0 && calls != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", calls, tc.wantCalls)
			}
			if tc.wantCode != "" && !fault.IsCode(err, tc.wantCode) {
				t.Fatalf("code=%q err=%v", nodeFaultCode(err), err)
			}
		})
	}
}

func TestPollerDoesNotRetryMixedTransientFailures(t *testing.T) {
	transient := transientDriverFault(errors.New("temporary"))
	permanent := errors.New("invalid")
	calls := 0
	err := (Poller{Interval: time.Millisecond}).Run(context.Background(), time.Second, func(context.Context) (bool, error) {
		calls++
		return false, errors.Join(transient, permanent)
	})
	if calls != 1 || !errors.Is(err, permanent) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
