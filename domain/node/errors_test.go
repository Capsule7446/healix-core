package node

import (
	"errors"
	"fmt"
	"testing"
)

func TestClassifyErrorPreservesStableKinds(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want ErrorKind
	}{
		{name: "not found", err: ErrElementNotFound, want: ErrorNotFound},
		{name: "unknown", err: errors.New("driver failed"), want: ErrorUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			classified, ok := ClassifyError("locate", test.err).(*ClassifiedError)
			if !ok || classified.Kind != test.want {
				t.Fatalf("classified = %#v, want kind %q", classified, test.want)
			}
			if !errors.Is(classified, test.err) {
				t.Fatalf("classification did not preserve original error")
			}
		})
	}
}

func TestExclusiveElementNotFoundRejectsMixedJoinedErrors(t *testing.T) {
	driverErr := errors.New("browser disconnected")
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "sentinel", err: ErrElementNotFound, want: true},
		{name: "wrapped", err: fmt.Errorf("all selectors failed: %w", ErrElementNotFound), want: true},
		{name: "joined not found", err: errors.Join(ErrElementNotFound, fmt.Errorf("fallback: %w", ErrElementNotFound)), want: true},
		{name: "mixed joined", err: errors.Join(ErrElementNotFound, driverErr), want: false},
		{name: "driver", err: driverErr, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isExclusiveElementNotFound(test.err); got != test.want {
				t.Fatalf("isExclusiveElementNotFound() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClassifiedErrorIsNilSafe(t *testing.T) {
	cases := []struct {
		name string
		err  *ClassifiedError
		want string
	}{
		{name: "nil receiver", err: nil, want: "<nil>"},
		{name: "nil cause", err: &ClassifiedError{Kind: ErrorTimeout}, want: "timeout: unspecified error"},
		{name: "nil cause with operation", err: &ClassifiedError{Kind: ErrorTimeout, Operation: "wait"}, want: "wait (timeout): unspecified error"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("Error() = %q, want %q", got, test.want)
			}
		})
	}
	if got := (*ClassifiedError)(nil).Unwrap(); got != nil {
		t.Fatalf("nil Unwrap() = %v, want nil", got)
	}
}

func TestRetryOnlyRetriesExplicitTransientErrors(t *testing.T) {
	attempts := 0
	err := Retry(RetryPolicy{Attempts: 3}, func() error {
		attempts++
		if attempts < 3 {
			return TransientError("click", errors.New("temporary"))
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
}
