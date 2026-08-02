package fault

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// This file closes the measured gaps in the fault kernel's own coverage. Two
// of them are worth naming because they are easy to misread from a coverage
// percentage alone:
//
//   - Kind() and Message() were missing their *non-nil* branch, not their nil
//     branch. The nil branch is exercised by TestFaultHandlesNilAndUnknownErrors;
//     what no in-package test did was read a populated fault back through its
//     own accessors, because every other package reads faults through KindOf
//     and Describe instead.
//   - Format was missing its nil branch, not its %q branch. %q is already
//     covered by the verb sweep in TestFormattingNeverReachesThePrivateCause.
//
// The remaining gaps are the validators' defensive arms. They are reachable
// from outside the package through zero-valued Param and Violation literals,
// which is exactly the input a caller produces by forgetting to check the
// error from NewParam.

func TestCodeSatisfiesErrorWithItsOwnString(t *testing.T) {
	code := Code("EXECUTION_CODE_AS_ERROR")

	// Code implements error so that errors.Is(err, someCode) can match a
	// fault by code alone. That makes Error() reachable by any host that
	// formats the sentinel it just matched against.
	var asError error = code
	if asError.Error() != string(code) {
		t.Fatalf("Code.Error() = %q, want %q", asError.Error(), string(code))
	}
	if fmt.Sprintf("%v", asError) != string(code) {
		t.Fatalf("formatted Code = %q, want %q", fmt.Sprintf("%v", asError), string(code))
	}
}

func TestParamAccessorsReturnWhatWasConstructed(t *testing.T) {
	param, err := NewParam("fieldName", "safe-value")
	if err != nil {
		t.Fatal(err)
	}

	if param.Key() != ParamKey("fieldName") {
		t.Fatalf("Key() = %q", param.Key())
	}
	if param.Value() != "safe-value" {
		t.Fatalf("Value() = %q", param.Value())
	}

	var zero Param
	if zero.Key() != "" || zero.Value() != "" {
		t.Fatalf("zero Param = (%q, %q), want empty", zero.Key(), zero.Value())
	}
}

func TestPopulatedFaultReadsBackThroughItsOwnAccessors(t *testing.T) {
	param, err := NewParam("field", "value")
	if err != nil {
		t.Fatal(err)
	}
	violation, err := NewViolation(CodeFieldRequired, "step.kind", "step kind is required")
	if err != nil {
		t.Fatal(err)
	}
	built, err := New(InvalidArgument, "EXECUTION_ACCESSOR_READBACK", "safe message", WithParams(param), WithViolations(violation))
	if err != nil {
		t.Fatal(err)
	}

	if built.Kind() != InvalidArgument {
		t.Fatalf("Kind() = %q, want %q", built.Kind(), InvalidArgument)
	}
	if built.Code() != Code("EXECUTION_ACCESSOR_READBACK") {
		t.Fatalf("Code() = %q", built.Code())
	}
	if built.Message() != "safe message" {
		t.Fatalf("Message() = %q", built.Message())
	}
	if params := built.Params(); len(params) != 1 || params[0] != param {
		t.Fatalf("Params() = %#v", params)
	}
	if violations := built.Violations(); len(violations) != 1 || violations[0].Code() != CodeFieldRequired {
		t.Fatalf("Violations() = %#v", violations)
	}
}

func TestNilFaultCollectionAccessorsReturnNil(t *testing.T) {
	var nilFault *Error

	if nilFault.Params() != nil {
		t.Fatalf("Params() on nil = %#v, want nil", nilFault.Params())
	}
	if nilFault.Violations() != nil {
		t.Fatalf("Violations() on nil = %#v, want nil", nilFault.Violations())
	}
}

// TestFormatOfANilFaultStaysPrintable matters because Format exists to keep
// %#v and %+v off the private cause. A nil fault reaching a log line through
// those verbs must still render, not panic — a panic inside a log statement
// is how a diagnostic path takes down the thing it was diagnosing.
func TestFormatOfANilFaultStaysPrintable(t *testing.T) {
	var nilFault *Error

	for _, verb := range []string{"%v", "%s", "%q", "%+v", "%#v"} {
		rendered := fmt.Sprintf(verb, nilFault)
		if !strings.Contains(rendered, "<nil>") {
			t.Fatalf("Sprintf(%q, nil fault) = %q, want it to mention <nil>", verb, rendered)
		}
	}
}

// TestIsCodeRejectsAnUnusableCode stops IsCode from becoming an accidental
// wildcard. errors.Is against a malformed Code would compare against a value
// no valid fault can hold, so the honest answer is false — but only because
// the guard runs first, not by luck.
func TestIsCodeRejectsAnUnusableCode(t *testing.T) {
	real, err := New(NotFound, "EXECUTION_ISCODE_GUARD", "safe message")
	if err != nil {
		t.Fatal(err)
	}

	for _, malformed := range []Code{"", "lowercase", "AB", "WITH SPACE", "TRAILING_"} {
		if IsCode(real, malformed) {
			t.Fatalf("IsCode matched the malformed code %q", malformed)
		}
		if IsCode(nil, malformed) {
			t.Fatalf("IsCode(nil, %q) matched", malformed)
		}
	}
	if !IsCode(real, "EXECUTION_ISCODE_GUARD") {
		t.Fatal("IsCode failed to match the fault's own code")
	}
}

func TestNewParamRejectsOversizeValue(t *testing.T) {
	if _, err := NewParam("field", strings.Repeat("a", maxParamValueLen)); err != nil {
		t.Fatalf("a value at the limit was rejected: %v", err)
	}
	if _, err := NewParam("field", strings.Repeat("a", maxParamValueLen+1)); err == nil {
		t.Fatal("a value one byte over the limit was accepted")
	}
}

func TestNewViolationRejectsUnusableParams(t *testing.T) {
	// A zero Param is what a caller holds after ignoring NewParam's error, so
	// the violation constructor has to re-check rather than trust it.
	if _, err := NewViolation(CodeFieldInvalid, "step.kind", "step kind is invalid", Param{}); err == nil {
		t.Fatal("a violation accepted a zero-valued parameter")
	}
}

func TestValidateMessageRejectsOversizeMessage(t *testing.T) {
	if err := validateMessage(strings.Repeat("a", maxMessageLength)); err != nil {
		t.Fatalf("a message at the limit was rejected: %v", err)
	}
	if err := validateMessage(strings.Repeat("a", maxMessageLength+1)); err == nil {
		t.Fatal("a message one byte over the limit was accepted")
	}
}

// TestValidateParamsMatrix covers each defensive arm separately so a single
// broad rejection cannot stand in for all of them.
func TestValidateParamsMatrix(t *testing.T) {
	safe := func(key ParamKey, value string) Param { return Param{key: key, value: value} }
	tests := []struct {
		name       string
		params     []Param
		wantReject bool
	}{
		{name: "empty", params: nil},
		{name: "at the count limit", params: countedParams(maxParams)},
		{name: "over the count limit", params: countedParams(maxParams + 1), wantReject: true},
		{name: "valid", params: []Param{safe("field", "value")}},
		{name: "zero valued", params: []Param{{}}, wantReject: true},
		{name: "uppercase key", params: []Param{safe("Field", "value")}, wantReject: true},
		{name: "key with underscore", params: []Param{safe("field_name", "value")}, wantReject: true},
		{name: "value at the limit", params: []Param{safe("field", strings.Repeat("a", maxParamValueLen))}},
		{name: "value over the limit", params: []Param{safe("field", strings.Repeat("a", maxParamValueLen+1))}, wantReject: true},
		{name: "value with a control character", params: []Param{safe("field", "line\nbreak")}, wantReject: true},
		{name: "value with non-ascii", params: []Param{safe("field", "值")}, wantReject: true},
		{name: "duplicate keys", params: []Param{safe("field", "a"), safe("field", "b")}, wantReject: true},
		{name: "distinct keys", params: []Param{safe("field", "a"), safe("other", "b")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateParams(test.params)

			if test.wantReject && err == nil {
				t.Fatal("validateParams accepted an unusable parameter set")
			}
			if !test.wantReject && err != nil {
				t.Fatalf("validateParams rejected a usable set: %v", err)
			}
		})
	}
}

func TestValidateViolationsMatrix(t *testing.T) {
	valid := Violation{code: CodeFieldRequired, field: "step.kind", message: "step kind is required"}
	tests := []struct {
		name       string
		violations []Violation
		wantReject bool
	}{
		{name: "empty", violations: nil},
		{name: "valid", violations: []Violation{valid}},
		{name: "at the count limit", violations: repeatViolation(valid, MaxViolations)},
		// construct truncates before calling validateViolations, so this arm
		// is defence in depth for any future caller that does not.
		{name: "over the count limit", violations: repeatViolation(valid, MaxViolations+1), wantReject: true},
		{name: "zero valued", violations: []Violation{{}}, wantReject: true},
		{name: "invalid code", violations: []Violation{{code: "lower", field: "step.kind", message: "m"}}, wantReject: true},
		{name: "invalid field", violations: []Violation{{code: CodeFieldRequired, field: "Step.Kind", message: "m"}}, wantReject: true},
		{name: "empty message", violations: []Violation{{code: CodeFieldRequired, field: "step.kind"}}, wantReject: true},
		{name: "unsafe message", violations: []Violation{{code: CodeFieldRequired, field: "step.kind", message: "bad\x00"}}, wantReject: true},
		{name: "unusable param", violations: []Violation{{code: CodeFieldRequired, field: "step.kind", message: "m", params: []Param{{}}}}, wantReject: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateViolations(test.violations)

			if test.wantReject && err == nil {
				t.Fatal("validateViolations accepted an unusable violation set")
			}
			if !test.wantReject && err != nil {
				t.Fatalf("validateViolations rejected a usable set: %v", err)
			}
		})
	}
}

// TestConstructRejectsAFailingOption keeps the option loop's error arm honest.
// Option is exported but faultOptions is not, so only this package can build a
// failing option — which is precisely why nothing outside it can cover this.
func TestConstructRejectsAFailingOption(t *testing.T) {
	sentinel := errors.New("option refused")
	failing := Option(func(*faultOptions) error { return sentinel })

	_, err := New(Internal, "EXECUTION_OPTION_FAILURE", "safe message", failing)

	if !errors.Is(err, sentinel) {
		t.Fatalf("New() error = %v, want the option's own error", err)
	}
}

func TestConstructRejectsUnusableParams(t *testing.T) {
	if _, err := New(Internal, "EXECUTION_PARAM_FAILURE", "safe message", WithParams(Param{})); err == nil {
		t.Fatal("New accepted a zero-valued parameter")
	}
	if _, err := Wrap(errors.New("cause"), Internal, "EXECUTION_PARAM_FAILURE", "safe message", WithParams(Param{})); err == nil {
		t.Fatal("Wrap accepted a zero-valued parameter")
	}
}

func countedParams(count int) []Param {
	params := make([]Param, count)
	for index := range params {
		params[index] = Param{key: ParamKey(fmt.Sprintf("field%d", index)), value: "value"}
	}
	return params
}

func repeatViolation(violation Violation, count int) []Violation {
	violations := make([]Violation, count)
	for index := range violations {
		violations[index] = violation
	}
	return violations
}
