package contract_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestExternalConsumerCanUseSafeFaultContract(t *testing.T) {
	wrapped, err := fault.Wrap(context.DeadlineExceeded, fault.DeadlineExceeded, "EXECUTION_EXTERNAL_TIMEOUT", "execution timed out")
	if err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}

	consumerError := fmt.Errorf("consumer boundary: %w", wrapped)
	if !errors.Is(consumerError, context.DeadlineExceeded) || !errors.Is(consumerError, fault.Code("EXECUTION_EXTERNAL_TIMEOUT")) {
		t.Fatal("external consumer lost causal or code identity")
	}
	if code, ok := fault.CodeOf(consumerError); !ok || code != "EXECUTION_EXTERNAL_TIMEOUT" {
		t.Fatalf("CodeOf() = %q, %v", code, ok)
	}
	if descriptor, ok := fault.Describe(consumerError); !ok || descriptor.Message() != "execution timed out" {
		t.Fatalf("Describe() = %#v, %v", descriptor, ok)
	}
}

func TestExternalConsumerHandlesUnknownAndTypedNilFaultsSafely(t *testing.T) {
	var typedNil *fault.Error
	var typedNilError error = typedNil
	unknown := errors.New("driver token=secret")

	for _, err := range []error{nil, typedNilError, unknown, errors.Join(unknown, typedNilError)} {
		if code, ok := fault.CodeOf(err); ok || code != "" {
			t.Fatalf("CodeOf(%v) = %q, %v", err, code, ok)
		}
		if descriptor, ok := fault.Describe(err); ok || descriptor.Message() != "" {
			t.Fatalf("Describe(%v) unexpectedly exposed details: %#v, %v", err, descriptor, ok)
		}
		if fault.IsCode(err, "EXECUTION_EXTERNAL_TIMEOUT") {
			t.Fatalf("IsCode(%v) unexpectedly matched", err)
		}
	}
}
