package node

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestClassifyNodeFaultPreservesCauseAndCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind fault.Kind
		code fault.Code
	}{
		{name: "canceled", err: context.Canceled, kind: fault.Canceled, code: CodeCanceled},
		{name: "deadline", err: context.DeadlineExceeded, kind: fault.DeadlineExceeded, code: CodeTimeout},
		{name: "not found", err: fmt.Errorf("locate: %w", ErrElementNotFound), kind: fault.NotFound, code: CodeElementNotFound},
		{name: "unknown", err: errors.New("driver password=secret failed"), kind: fault.Internal, code: CodeOperationFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classified := classifyNodeFault(test.err)
			if !errors.Is(classified, test.err) {
				t.Fatal("classified fault did not preserve cause")
			}
			if kind, ok := fault.KindOf(classified); !ok || kind != test.kind {
				t.Fatalf("KindOf() = %q, %v", kind, ok)
			}
			if code, ok := fault.CodeOf(classified); !ok || code != test.code {
				t.Fatalf("CodeOf() = %q, %v", code, ok)
			}
			if descriptor, ok := fault.Describe(classified); !ok || descriptor.Message() == "driver password=secret failed" {
				t.Fatalf("Describe() = %#v, %v", descriptor, ok)
			}
		})
	}
}

func TestClassifyNodeFaultIsChainIdempotent(t *testing.T) {
	original := transientDriverFault(errors.New("temporary driver failure"))
	classified := classifyNodeFault(fmt.Errorf("outer: %w", original))
	if !fault.IsCode(classified, CodeTransientDriver) {
		t.Fatalf("CodeOf() = %q", classified)
	}
	if !errors.Is(classified, original) {
		t.Fatal("classified fault did not retain wrapped fault")
	}
}
