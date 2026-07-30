package node

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestClassifyNodeFaultPreservesStableDetails(t *testing.T) {
	cases := []struct {
		name     string
		cause    error
		wantKind fault.Kind
		wantCode fault.Code
	}{
		{name: "not found", cause: ErrElementNotFound, wantKind: fault.NotFound, wantCode: CodeElementNotFound},
		{name: "unknown", cause: errors.New("driver failed"), wantKind: fault.Internal, wantCode: CodeOperationFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			classified := classifyNodeFault(tc.cause)
			if kind, ok := fault.KindOf(classified); !ok || kind != tc.wantKind {
				t.Fatalf("kind=%q, found=%v", kind, ok)
			}
			if code, ok := fault.CodeOf(classified); !ok || code != tc.wantCode {
				t.Fatalf("code=%q, found=%v", code, ok)
			}
			if !errors.Is(classified, tc.cause) {
				t.Fatal("classification did not preserve original error")
			}
		})
	}
}

func TestExclusiveElementNotFoundRejectsMixedJoinedErrors(t *testing.T) {
	driverErr := errors.New("browser disconnected")
	transient := transientDriverFault(driverErr)
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "sentinel", err: ErrElementNotFound, want: true},
		{name: "wrapped", err: fmt.Errorf("all selectors failed: %w", ErrElementNotFound), want: true},
		{name: "joined not found", err: errors.Join(ErrElementNotFound, fmt.Errorf("fallback: %w", ErrElementNotFound)), want: true},
		{name: "mixed driver", err: errors.Join(ErrElementNotFound, driverErr), want: false},
		{name: "mixed transient fault", err: errors.Join(ErrElementNotFound, transient), want: false},
		{name: "driver", err: driverErr, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isExclusiveElementNotFound(tc.err); got != tc.want {
				t.Fatalf("isExclusiveElementNotFound() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRetryOnlyRetriesExplicitTransientFaultCode(t *testing.T) {
	attempts := 0
	err := Retry(RetryPolicy{Attempts: 3}, func() error {
		attempts++
		if attempts < 3 {
			return transientDriverFault(errors.New("temporary"))
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("retry result=%v attempts=%d", err, attempts)
	}

	attempts = 0
	err = Retry(RetryPolicy{Attempts: 3}, func() error {
		attempts++
		return ErrElementNotFound
	})
	if !errors.Is(err, ErrElementNotFound) || attempts != 1 {
		t.Fatalf("non-transient retry result=%v attempts=%d", err, attempts)
	}

	attempts = 0
	err = Retry(RetryPolicy{Attempts: 3}, func() error {
		attempts++
		return classifyNodeFault(context.Canceled)
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 || !fault.IsCode(err, CodeCanceled) {
		t.Fatalf("canceled retry result=%v attempts=%d", err, attempts)
	}
}
