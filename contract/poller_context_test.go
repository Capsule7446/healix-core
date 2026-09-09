package contract_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/node"
)

type consumerPollReadError struct {
	cause error
}

func (e *consumerPollReadError) Error() string { return "consumer read stopped" }
func (e *consumerPollReadError) Unwrap() error { return e.cause }

func TestConsumerPollerClassifiesContextErrorReturnedByCondition(t *testing.T) {
	tests := []struct {
		name      string
		parent    func(*testing.T) (context.Context, context.CancelFunc)
		stop      func(context.CancelFunc)
		wantCode  fault.Code
		wantKind  fault.Kind
		wantErr   error
		readCause error
	}{
		{
			name: "own timeout",
			parent: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				return context.WithCancel(t.Context())
			},
			stop: func(context.CancelFunc) {}, wantCode: node.CodeTimeout,
			wantKind: fault.DeadlineExceeded, wantErr: context.DeadlineExceeded,
		},
		{
			name: "parent cancellation",
			parent: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				return context.WithCancel(t.Context())
			},
			stop: func(cancel context.CancelFunc) { cancel() }, wantCode: node.CodeCanceled,
			wantKind: fault.Canceled, wantErr: context.Canceled,
		},
		{
			name: "parent deadline",
			parent: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				return context.WithTimeout(t.Context(), 100*time.Millisecond)
			},
			stop: func(context.CancelFunc) {}, wantCode: node.CodeTimeout,
			wantKind: fault.DeadlineExceeded, wantErr: context.DeadlineExceeded,
		},
		{
			name: "parent deadline with another cancellation cause",
			parent: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				return context.WithTimeout(t.Context(), 100*time.Millisecond)
			},
			stop: func(context.CancelFunc) {}, wantCode: node.CodeTimeout,
			wantKind: fault.DeadlineExceeded, wantErr: context.DeadlineExceeded,
			readCause: context.Canceled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				parent, cancel := test.parent(t)
				t.Cleanup(cancel)
				var readErr *consumerPollReadError
				calls := 0
				err := (node.Poller{Interval: time.Millisecond}).Run(parent, time.Second, func(ctx context.Context) (bool, error) {
					calls++
					test.stop(cancel)
					<-ctx.Done()
					readErr = &consumerPollReadError{cause: errors.Join(test.readCause, ctx.Err())}
					return false, readErr
				})
				assertConsumerPollFault(t, err, test.wantCode, test.wantKind)
				var recovered *consumerPollReadError
				if calls != 1 || !errors.Is(err, test.wantErr) || !errors.Is(err, readErr) || !errors.As(err, &recovered) || recovered != readErr {
					t.Errorf("Poller.Run(%s) calls = %d, error = %v; want one call retaining the read and context causes", test.name, calls, err)
				}
				if test.readCause != nil && !errors.Is(err, test.readCause) {
					t.Errorf("Poller.Run(%s) error = %v, want additional read cause %v", test.name, err, test.readCause)
				}
			})
		})
	}
}

func TestConsumerPollerTimeoutKeepsEarlierRetryAndFinalReadCauses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		earlier := node.NewElementNotFoundError()
		var last *consumerPollReadError
		calls := 0
		err := (node.Poller{Interval: time.Millisecond}).Run(t.Context(), time.Second, func(ctx context.Context) (bool, error) {
			calls++
			if calls == 1 {
				return false, earlier
			}
			<-ctx.Done()
			last = &consumerPollReadError{cause: ctx.Err()}
			return false, last
		})
		assertConsumerPollFault(t, err, node.CodeTimeout, fault.DeadlineExceeded)
		if calls != 2 || !errors.Is(err, earlier) || !errors.Is(err, last) || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Poller.Run(retry then blocked read) calls = %d, error = %v; want two calls retaining all causes", calls, err)
		}
	})
}

func TestConsumerPollerPreservesIndependentReadErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		independent := fmt.Errorf("driver deadline: %w", context.DeadlineExceeded)
		err := (node.Poller{}).Run(t.Context(), time.Second, func(ctx context.Context) (bool, error) {
			if ctx.Err() != nil {
				t.Errorf("Poller.Run live context error = %v, want nil", ctx.Err())
			}
			return false, independent
		})
		if err != independent {
			t.Errorf("Poller.Run(independent deadline) error = %v, want the original %v", err, independent)
		}
	})
}

func TestConsumerPollerDoesNotReplaceIndependentErrorWhenParentCancels(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	independent := errors.New("independent driver failure")
	err := (node.Poller{}).Run(parent, time.Second, func(context.Context) (bool, error) {
		cancel()
		return false, independent
	})
	if err != independent {
		t.Errorf("Poller.Run(canceled parent with independent error) error = %v, want the original %v", err, independent)
	}
}

func TestConsumerPollerPreservesClassifiedContextCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var classified error
		err := (node.Poller{}).Run(t.Context(), time.Second, func(ctx context.Context) (bool, error) {
			<-ctx.Done()
			var constructionErr error
			classified, constructionErr = fault.Wrap(ctx.Err(), fault.Internal, node.CodeOperationFailed, "classified driver failure")
			if constructionErr != nil {
				t.Fatalf("construct consumer driver fault: %v", constructionErr)
			}
			return false, classified
		})
		assertConsumerPollFault(t, err, node.CodeOperationFailed, fault.Internal)
		if err != classified || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Poller.Run(classified context cause) error = %v, want the unchanged driver fault", err)
		}
	})
}

func assertConsumerPollFault(t *testing.T, err error, wantCode fault.Code, wantKind fault.Kind) {
	t.Helper()
	code, hasCode := fault.CodeOf(err)
	kind, hasKind := fault.KindOf(err)
	got := struct {
		Code    fault.Code
		Kind    fault.Kind
		HasCode bool
		HasKind bool
	}{code, kind, hasCode, hasKind}
	want := struct {
		Code    fault.Code
		Kind    fault.Kind
		HasCode bool
		HasKind bool
	}{wantCode, wantKind, true, true}
	if got != want {
		t.Errorf("Poller.Run fault = %+v, want %+v; error = %v", got, want, err)
	}
}
