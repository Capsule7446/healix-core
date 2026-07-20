package node

import (
	"errors"
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
