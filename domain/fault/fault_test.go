package fault

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const testCode Code = "EXECUTION_TEST_FAILURE"

func TestNewRejectsInvalidPublicValues(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		code    Code
		message string
	}{
		{name: "unknown kind", kind: Kind("UNKNOWN"), code: testCode, message: "safe message"},
		{name: "invalid code", kind: InvalidArgument, code: "lowercase", message: "safe message"},
		{name: "empty message", kind: InvalidArgument, code: testCode, message: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.kind, test.code, test.message); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func TestFaultPreservesCauseWithoutDisclosingIt(t *testing.T) {
	cause := errors.New("driver password=top-secret failed")
	err, newErr := Wrap(cause, Unavailable, testCode, "execution dependency is unavailable")
	if newErr != nil {
		t.Fatalf("Wrap() error = %v", newErr)
	}

	if got := err.Error(); got != "EXECUTION_TEST_FAILURE: execution dependency is unavailable" {
		t.Fatalf("Error() = %q", got)
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("Error() disclosed cause: %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is() did not reach cause")
	}
	if !errors.Is(err, testCode) {
		t.Fatal("errors.Is() did not match code")
	}

	descriptor, ok := Describe(fmt.Errorf("application: %w", err))
	if !ok {
		t.Fatal("Describe() did not find wrapped fault")
	}
	if descriptor.Code() != testCode || descriptor.Kind() != Unavailable {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	if strings.Contains(descriptor.Message(), "top-secret") {
		t.Fatalf("Describe() disclosed cause: %q", descriptor.Message())
	}
}

func TestFaultQueriesTraverseWrappingAndJoin(t *testing.T) {
	canceled, err := Wrap(context.Canceled, Canceled, "EXECUTION_OPERATION_CANCELED", "operation was canceled")
	if err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}
	other := errors.New("other")
	joined := fmt.Errorf("outer: %w", errors.Join(other, canceled))

	if code, ok := CodeOf(joined); !ok || code != canceled.Code() {
		t.Fatalf("CodeOf() = %q, %v", code, ok)
	}
	if kind, ok := KindOf(joined); !ok || kind != Canceled {
		t.Fatalf("KindOf() = %q, %v", kind, ok)
	}
	if !IsCode(joined, canceled.Code()) || IsCode(joined, testCode) {
		t.Fatal("IsCode() returned incorrect result")
	}
	if !errors.Is(joined, context.Canceled) {
		t.Fatal("errors.Is() did not preserve context cause")
	}
	var extracted *Error
	if !errors.As(joined, &extracted) || extracted != canceled {
		t.Fatal("errors.As() did not extract fault")
	}
}

func TestFaultDefensivelyCopiesParamsAndViolations(t *testing.T) {
	param, err := NewParam("field", "displayName")
	if err != nil {
		t.Fatalf("NewParam() error = %v", err)
	}
	violation, err := NewViolation("AUTOMATION_NAME_REQUIRED", "displayName", "display name is required", param)
	if err != nil {
		t.Fatalf("NewViolation() error = %v", err)
	}
	fault, err := New(InvalidArgument, "AUTOMATION_INVALID", "automation input is invalid", WithParams(param), WithViolations(violation))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	params := fault.Params()
	params[0] = Param{}
	violations := fault.Violations()
	violations[0] = Violation{}

	if got := fault.Params()[0]; got != param {
		t.Fatalf("Params() leaked ownership: %#v", got)
	}
	if got := fault.Violations()[0]; got.Code() != violation.Code() || got.Field() != violation.Field() || got.Message() != violation.Message() || !reflect.DeepEqual(got.Params(), violation.Params()) {
		t.Fatalf("Violations() leaked ownership: %#v", got)
	}

	descriptor, ok := Describe(fault)
	if !ok || descriptor.Params()[0] != param || descriptor.Violations()[0].Code() != violation.Code() || descriptor.Violations()[0].Field() != violation.Field() || !reflect.DeepEqual(descriptor.Violations()[0].Params(), violation.Params()) {
		t.Fatalf("Describe() = %#v, %v", descriptor, ok)
	}
}

func TestFaultHandlesNilAndUnknownErrors(t *testing.T) {
	var nilFault *Error
	if nilFault.Error() != "<nil>" || nilFault.Unwrap() != nil || nilFault.Code() != "" || nilFault.Kind() != "" || nilFault.Message() != "" {
		t.Fatal("nil fault did not return safe zero values")
	}
	var typedNil error = nilFault
	for _, err := range []error{nil, typedNil, errors.New("driver token=secret")} {
		if _, ok := CodeOf(err); ok {
			t.Fatalf("CodeOf(%v) unexpectedly found a fault", err)
		}
		if _, ok := KindOf(err); ok {
			t.Fatalf("KindOf(%v) unexpectedly found a fault", err)
		}
		if IsCode(err, testCode) {
			t.Fatalf("IsCode(%v) unexpectedly matched", err)
		}
		if _, ok := Describe(err); ok {
			t.Fatalf("Describe(%v) unexpectedly found a fault", err)
		}
	}
}

func TestFaultRejectsInvalidOptionsAndViolationShapes(t *testing.T) {
	if _, err := New(Internal, testCode, "safe\ninternal failure"); err == nil {
		t.Fatal("New() accepted a control-character message")
	}
	if _, err := New(Internal, testCode, "safe internal failure", WithViolations(Violation{})); err == nil {
		t.Fatal("New() accepted an invalid violation")
	}
	if _, err := Wrap(nil, Internal, testCode, "safe internal failure"); err != nil {
		t.Fatalf("Wrap(nil) error = %v", err)
	}
	if _, err := New(Internal, testCode, "safe internal failure", nil); err == nil {
		t.Fatal("New() accepted nil option")
	}
	if _, err := NewParam("BadKey", "value"); err == nil {
		t.Fatal("NewParam() accepted invalid key")
	}
	if _, err := NewViolation("bad", "field", "safe message"); err == nil {
		t.Fatal("NewViolation() accepted invalid code")
	}
	if _, err := NewViolation("AUTOMATION_INVALID", "BadField", "safe message"); err == nil {
		t.Fatal("NewViolation() accepted invalid field")
	}
}

func TestFaultValidationErrorsDoNotReflectRejectedValues(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		make   func(string) error
	}{
		{name: "kind", secret: "secret-kind", make: func(secret string) error { _, err := New(Kind(secret), testCode, "safe message"); return err }},
		{name: "code", secret: "secret-code", make: func(secret string) error { _, err := New(Internal, Code(secret), "safe message"); return err }},
		{name: "parameter key", secret: "secret-key", make: func(secret string) error { _, err := NewParam(ParamKey(secret), "safe value"); return err }},
		{name: "violation field", secret: "secret-field", make: func(secret string) error { _, err := NewViolation(testCode, secret, "safe message"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.make(test.secret)
			if err == nil || strings.Contains(err.Error(), test.secret) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestFaultRejectsMaliciousTextAtPublicBoundaries(t *testing.T) {
	tests := []struct {
		name string
		make func() error
	}{
		{name: "parameter null byte", make: func() error { _, err := NewParam("field", "safe\x00secret"); return err }},
		{name: "message null byte", make: func() error { _, err := New(Internal, testCode, "safe\x00secret"); return err }},
		{name: "message unicode line separator", make: func() error { _, err := New(Internal, testCode, "safe secret"); return err }},
		{name: "violation message unicode paragraph separator", make: func() error { _, err := NewViolation(testCode, "field", "safe secret"); return err }},
		{name: "message invalid UTF-8", make: func() error { _, err := New(Internal, testCode, string([]byte{'s', 0xff})); return err }},
		{name: "parameter bidi override", make: func() error { _, err := NewParam("field", "safe‮secret"); return err }},
		{name: "message bidi isolate", make: func() error { _, err := New(Internal, testCode, "safe⁦secret"); return err }},
		{name: "violation message zero-width format", make: func() error { _, err := NewViolation(testCode, "field", "safe​secret"); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.make(); err == nil {
				t.Fatal("accepted unsafe public text")
			}
		})
	}
}

// A cause may hold a token, a URL, or page content — that is what Unwrap exists
// to gate. Default struct formatting reflects over unexported fields, so %#v and
// %+v published exactly that to any host logging the error.
func TestFormattingNeverReachesThePrivateCause(t *testing.T) {
	cause := fmt.Errorf("carrier: %w", &formattedCause{token: "token-8f21"})

	err, constructionErr := Wrap(cause, Internal, Code("KERNEL_FORMAT_PROBE"), "operation could not be completed")
	if constructionErr != nil {
		t.Fatalf("construct: %v", constructionErr)
	}

	for _, format := range []string{"%v", "%s", "%q", "%+v", "%#v"} {
		rendered := fmt.Sprintf(format, err)
		if strings.Contains(rendered, "token-8f21") {
			t.Fatalf("%s rendered the private cause: %s", format, rendered)
		}
		if !strings.Contains(rendered, "KERNEL_FORMAT_PROBE") {
			t.Fatalf("%s lost the code: %s", format, rendered)
		}
	}
	// The cause is still reachable for a caller that deliberately asks.
	if !errors.Is(err, cause) {
		t.Fatal("Unwrap no longer reaches the cause")
	}
}

type formattedCause struct{ token string }

func (c *formattedCause) Error() string { return "private detail " + c.token }

// Over-cap violations used to fail construction. Every context builds violations
// from untrusted input and constructs through a must-helper, so a hostile payload
// became a panic instead of a classified rejection.
func TestOverCapViolationsTruncateInsteadOfFailingConstruction(t *testing.T) {
	violations := make([]Violation, 0, MaxViolations+10)
	for index := 0; index < MaxViolations+10; index++ {
		violation, err := NewViolation(CodeFieldInvalid, fmt.Sprintf("items.%d", index), "item is invalid")
		if err != nil {
			t.Fatalf("build violation %d: %v", index, err)
		}
		violations = append(violations, violation)
	}

	err, constructionErr := New(InvalidArgument, Code("KERNEL_CAP_PROBE"), "input is invalid", WithViolations(violations...))
	if constructionErr != nil {
		t.Fatalf("construction failed instead of truncating: %v", constructionErr)
	}
	kept := err.Violations()
	if len(kept) != MaxViolations {
		t.Fatalf("kept %d violations, want the cap %d", len(kept), MaxViolations)
	}
	// The prefix is kept, so the report degrades deterministically rather than
	// becoming an arbitrary subset.
	for index, violation := range kept {
		if want := fmt.Sprintf("items.%d", index); violation.Field() != want {
			t.Fatalf("violation %d is %q, want the leading prefix entry %q", index, violation.Field(), want)
		}
	}
}

// "safe English" was enforced by naming two separators by hand, which let every
// other invisible or reordering character through.
func TestPublicTextRejectsInvisibleAndReorderingCharacters(t *testing.T) {
	rejected := map[string]string{
		"no-break space":     "safe\u00A0text",
		"ogham space":        "safe\u1680text",
		"en quad":            "safe\u2000text",
		"narrow no-break":    "safe\u202Ftext",
		"ideographic space":  "safe\u3000text",
		"hangul filler":      "\u3164",
		"combining marks":    "A\u0300\u0301\u0302",
		"variation selector": "safe\uFE0Ftext",
		"right-to-left mark": "safe\u200Ftext",
		"zero width joiner":  "safe\u200Dtext",
	}
	for name, text := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, err := New(InvalidArgument, Code("KERNEL_TEXT_PROBE"), text); err == nil {
				t.Fatalf("message %q was accepted as safe public text", text)
			}
			if _, err := NewParam("detail", text); err == nil {
				t.Fatalf("param value %q was accepted as safe public text", text)
			}
		})
	}
	if _, err := New(InvalidArgument, Code("KERNEL_TEXT_PROBE"), "plain ascii message (with punctuation) 123"); err != nil {
		t.Fatalf("ordinary safe English was rejected: %v", err)
	}
}
