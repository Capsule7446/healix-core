package contract_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/interpolation"
	"github.com/Capsule7446/healix-core/domain/parameter"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

// Codes are only usable if a package outside the module graph can name them. The
// registry guard in architecture/ verifies each code's Kind, safe message, and
// exportedness; what it cannot verify is that every owning context is importable
// from a consumer. domain/evidence in particular had no consumer-side import, so
// its whole family would have gone unexercised from outside.
func TestEveryCodeFamilyIsReachableFromAConsumer(t *testing.T) {
	families := map[string][]fault.Code{
		"EXECUTION": {
			domainexecution.CodeWorkerFenceStale,
		},
		"AUTOMATION": {
			domainautomation.CodeExecutionFlowInvalid,
			domainautomation.CodeRevisionExhausted,
		},
		"SAMPLING": {
			sampling.CodeSessionInputInvalid,
			sampling.CodeDraftIndexOutOfRange,
			sampling.CodeInternal,
		},
		"EVIDENCE": {
			evidence.CodeStepTransitionCommitInvalid,
			evidence.CodeCommitFactLimitExceeded,
			evidence.CodeStepProgressEventInvalid,
			evidence.CodeStepFactInvalid,
			evidence.CodeHealObservationInvalid,
			evidence.CodeValidationObservationInvalid,
			evidence.CodeValidationGroupObservationInvalid,
		},
		"FINGERPRINT": {
			fingerprint.CodeSelectorInvalid,
			fingerprint.CodeElementTargetSpecInvalid,
			fingerprint.CodeDescriptorInvalid,
			fingerprint.CodeFrameworkStackInvalid,
			fingerprint.CodeFrameworkDetectorFailed,
		},
		"PARAMETER": {
			parameter.CodeNameInvalid,
			parameter.CodeValueInvalid,
			parameter.CodeConstraintUnsatisfied,
			parameter.CodeBindingUnresolvable,
		},
		"INTERPOLATION": {
			interpolation.CodeResolverRequired,
		},
		"VALIDATION": {
			fault.CodeFieldRequired,
			fault.CodeFieldInvalid,
			fault.CodeFieldDuplicate,
			fault.CodeFieldMismatch,
		},
	}

	for prefix, codes := range families {
		for _, code := range codes {
			if code == "" {
				t.Errorf("%s family exposes an empty code constant", prefix)
				continue
			}
			// A code carries its own prefix; a consumer routing on prefix must be able
			// to rely on that rather than on which package it came from.
			if got := string(code); len(got) <= len(prefix) || got[:len(prefix)] != prefix || got[len(prefix)] != '_' {
				t.Errorf("%s is exposed under the %s family but does not carry that prefix", code, prefix)
			}
			// errors.Is must match on the code alone, with no access to the producer.
			wrapped, constructionErr := fault.New(fault.InvalidArgument, code, "consumer reachability probe")
			if constructionErr != nil {
				t.Errorf("%s cannot be constructed by a consumer: %v", code, constructionErr)
				continue
			}
			if !errors.Is(fmt.Errorf("consumer boundary: %w", wrapped), code) {
				t.Errorf("%s does not survive consumer-side wrapping", code)
			}
		}
	}
}

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
