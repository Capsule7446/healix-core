package node

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPollerBusinessMatrix(t *testing.T) {
	cases := []struct {
		name      string
		condition func(*int) func(context.Context) (bool, error)
		parent    func() context.Context
		wantErr   bool
		wantKind  ErrorKind
		wantCalls int
	}{
		{name: "immediate success", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return true, nil }
		}, wantCalls: 1},
		{name: "transient then success", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) {
				*c++
				if *c == 1 {
					return false, TransientError("poll", errors.New("temporary"))
				}
				return true, nil
			}
		}, wantCalls: 2},
		{name: "permanent error", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return false, errors.New("invalid") }
		}, wantErr: true, wantKind: ErrorUnknown, wantCalls: 1},
		{name: "timeout false condition", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return false, nil }
		}, wantErr: true, wantKind: ErrorTimeout},
		{name: "parent cancellation", condition: func(c *int) func(context.Context) (bool, error) {
			return func(context.Context) (bool, error) { *c++; return false, nil }
		}, parent: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, wantErr: true, wantKind: ErrorContextClosed},
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
			if tc.wantErr && errorKind(err) != tc.wantKind {
				t.Fatalf("kind=%s err=%v", errorKind(err), err)
			}
		})
	}
}
