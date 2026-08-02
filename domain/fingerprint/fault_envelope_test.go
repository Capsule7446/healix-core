package fingerprint

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// envelopeKinds mirrors the Kind column of the registry's FINGERPRINT_* rows.
// Asserting through this table means a helper that hardcoded one Kind can no
// longer silently misclassify a code.
var envelopeKinds = map[fault.Code]fault.Kind{
	CodeSelectorInvalid:          fault.InvalidArgument,
	CodeElementTargetSpecInvalid: fault.InvalidArgument,
	CodeDescriptorInvalid:        fault.InvalidArgument,
	CodeFrameworkStackInvalid:    fault.InvalidArgument,
	CodeFrameworkDetectorFailed:  fault.Internal,
}

func violationKey(code fault.Code, field string) string {
	return string(code) + "@" + field
}

func violationKeys(violations []fault.Violation) []string {
	keys := make([]string, 0, len(violations))
	for _, violation := range violations {
		keys = append(keys, violationKey(violation.Code(), violation.Field()))
	}
	return keys
}

func requireEnvelope(t *testing.T, err error, wantCode fault.Code) fault.Descriptor {
	t.Helper()
	if err == nil {
		t.Fatal("Validate() accepted invalid input")
	}
	if !fault.IsCode(err, wantCode) {
		t.Fatalf("error = %v, want code %s", err, wantCode)
	}
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	wantKind, registered := envelopeKinds[wantCode]
	if !registered {
		t.Fatalf("code %s has no registered Kind in envelopeKinds", wantCode)
	}
	if descriptor.Kind() != wantKind {
		t.Fatalf("kind = %s, want %s", descriptor.Kind(), wantKind)
	}
	return descriptor
}

func requireViolation(t *testing.T, err error, wantCode, wantViolation fault.Code, wantField string) {
	t.Helper()
	descriptor := requireEnvelope(t, err, wantCode)
	for _, violation := range descriptor.Violations() {
		if violation.Code() == wantViolation && violation.Field() == wantField {
			return
		}
	}
	t.Fatalf("violations = [%s], want one with %s", strings.Join(violationKeys(descriptor.Violations()), ", "), violationKey(wantViolation, wantField))
}

func requireNoPublicLeak(t *testing.T, err error, secrets ...string) {
	t.Helper()
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	texts := []string{err.Error(), descriptor.Message()}
	for _, param := range descriptor.Params() {
		texts = append(texts, string(param.Key()), param.Value())
	}
	for _, violation := range descriptor.Violations() {
		texts = append(texts, violation.Field(), violation.Message())
		for _, param := range violation.Params() {
			texts = append(texts, string(param.Key()), param.Value())
		}
	}
	for _, secret := range secrets {
		for _, text := range texts {
			if strings.Contains(text, secret) {
				t.Fatalf("public fault text %q leaks %q", text, secret)
			}
		}
	}
}
