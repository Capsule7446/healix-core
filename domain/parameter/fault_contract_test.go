package parameter

import (
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func requireCode(t *testing.T, err error, wantCode fault.Code, wantKind fault.Kind) fault.Descriptor {
	t.Helper()
	if !fault.IsCode(err, wantCode) {
		t.Fatalf("error = %v, want code %s", err, wantCode)
	}
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	if descriptor.Kind() != wantKind {
		t.Fatalf("kind = %s, want %s", descriptor.Kind(), wantKind)
	}
	// None of these codes is an aggregate input code, so the registry forbids them
	// from carrying violations.
	if len(descriptor.Violations()) != 0 || len(descriptor.Params()) != 0 {
		t.Fatalf("descriptor carries details: %#v", descriptor)
	}
	return descriptor
}

func requireNoLeak(t *testing.T, err error, secrets ...string) {
	t.Helper()
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	for _, secret := range secrets {
		for _, text := range []string{err.Error(), descriptor.Message()} {
			if strings.Contains(text, secret) {
				t.Fatalf("public fault text %q leaks %q", text, secret)
			}
		}
	}
}

// NewNumberValue is a host-facing constructor with no callers inside Core, and it
// used to return the caller's raw input inside the error text.
func TestNewNumberValueRejectsWithoutEchoingInput(t *testing.T) {
	const secretInput = "0x1f-not-a-number-8f21"
	value, err := NewNumberValue(secretInput)
	if !value.Equal(Value{}) {
		t.Fatalf("NewNumberValue() returned %#v on failure", value)
	}
	requireCode(t, err, CodeValueInvalid, fault.InvalidArgument)
	requireNoLeak(t, err, secretInput)
	// The canonicalisation failure stays reachable as a private cause, and the
	// rejected input is absent from it too, because hosts may log causes.
	cause := errors.Unwrap(err)
	if cause == nil {
		t.Fatal("canonicalisation cause was discarded")
	}
	if strings.Contains(cause.Error(), secretInput) {
		t.Fatalf("private cause %q leaks the rejected input", cause)
	}
}

func TestNewNumberValueRejectsOversizedInputWithSameCode(t *testing.T) {
	_, err := NewNumberValue(strings.Repeat("9", MaxValueStringBytes+1))
	// Over-size shares PARAMETER_VALUE_INVALID rather than getting OUT_OF_RANGE:
	// the remediation is the same, supply a different value.
	requireCode(t, err, CodeValueInvalid, fault.InvalidArgument)
}

func TestValidateNameRejectsWithoutEchoingName(t *testing.T) {
	const secretName = "name-8f21"
	tests := []struct {
		name  string
		input string
	}{
		{name: "blank", input: " \t\n"},
		{name: "control character", input: secretName + string(rune(7))},
		{name: "format character", input: secretName + string(rune(0x200D))},
		{name: "invalid utf8", input: secretName + "\xff"},
		{name: "over byte limit", input: strings.Repeat("n", MaxNameBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateName(test.input)
			requireCode(t, err, CodeNameInvalid, fault.InvalidArgument)
			requireNoLeak(t, err, secretName, "8f21")
		})
	}
}

func TestValueValidateRejectsUnsupportedTypeWithoutEchoingIt(t *testing.T) {
	const secretType = "SECRET_TYPE_8F21"
	err := Value{kind: Type(secretType)}.Validate()
	requireCode(t, err, CodeValueInvalid, fault.InvalidArgument)
	requireNoLeak(t, err, secretType)
}

func TestConstraintValidateRejectsWithoutEchoingTypesOrOptions(t *testing.T) {
	const secretOption = "option-8f21"
	err := Constraint{Type: SingleSelect, Options: []string{secretOption}}.Validate(SingleSelectValue("absent-8f21"))
	requireCode(t, err, CodeConstraintUnsatisfied, fault.InvalidArgument)
	requireNoLeak(t, err, secretOption, "absent-8f21")

	err = Constraint{Type: Text}.Validate(BooleanValue(true))
	requireCode(t, err, CodeConstraintUnsatisfied, fault.InvalidArgument)
	requireNoLeak(t, err, string(Text), string(Boolean))
}

// A malformed value keeps its own code on the way out. Re-wrapping it as a
// constraint failure would force the host to unwrap before it could tell that the
// value itself, not the constraint, was the problem.
func TestConstraintValidatePropagatesValueCodeUnchanged(t *testing.T) {
	err := Constraint{Type: Text}.Validate(Value{})
	requireCode(t, err, CodeValueInvalid, fault.InvalidArgument)
	if fault.IsCode(err, CodeConstraintUnsatisfied) {
		t.Fatalf("value failure was re-labelled as a constraint failure: %v", err)
	}
}

// The sign byte was added after the size checks, so a negative number whose
// unsigned body was exactly at the limit came back one byte over — accepted by
// the constructor, then rejected by that same value's own Validate.
func TestNewNumberValueNeverReturnsAValueItsOwnValidatorRejects(t *testing.T) {
	for _, input := range []string{"-1e65535", "1e65535", "-1e65534", "1e65534"} {
		t.Run(input, func(t *testing.T) {
			value, err := NewNumberValue(input)
			if err != nil {
				requireCode(t, err, CodeValueInvalid, fault.InvalidArgument)
				return
			}
			if len(value.Number()) > MaxValueStringBytes {
				t.Fatalf("constructor returned %d bytes, over the %d limit", len(value.Number()), MaxValueStringBytes)
			}
			if validateErr := value.Validate(); validateErr != nil {
				t.Fatalf("constructor accepted a value its own Validate rejects: %v", validateErr)
			}
		})
	}
}
