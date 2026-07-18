package workspace

import (
	"reflect"
	"strings"
	"testing"
)

func validationKindsByContract() (plain, scalar, regex, set, attribute []ValidationAssertionKind) {
	plain = []ValidationAssertionKind{
		ValidationExists, ValidationNotExists, ValidationVisible, ValidationNotVisible,
		ValidationValueNotEmpty, ValidationEnabled, ValidationDisabled, ValidationChecked,
		ValidationUnchecked, ValidationMixed, ValidationSelected, ValidationUnselected,
		ValidationPressed, ValidationUnpressed,
	}
	scalar = []ValidationAssertionKind{
		ValidationTextEquals, ValidationTextContains, ValidationValueEquals, ValidationValueContains,
		ValidationSelectedTextEquals, ValidationSelectedTextContains,
		ValidationSelectedValueEquals, ValidationSelectedValueContains,
	}
	regex = []ValidationAssertionKind{ValidationTextMatches, ValidationValueMatches}
	set = []ValidationAssertionKind{ValidationSelectedSetEquals, ValidationSelectedSetContains}
	attribute = []ValidationAssertionKind{ValidationAttributeEquals, ValidationAttributeContains}
	return
}

func TestValidationAssertionValidateAcceptsEverySupportedKind(t *testing.T) {
	plain, scalar, regex, set, attribute := validationKindsByContract()
	seen := make(map[ValidationAssertionKind]bool)
	for _, kind := range plain {
		seen[kind] = true
		t.Run(string(kind), func(t *testing.T) {
			if err := (ValidationAssertion{Kind: kind}).Validate(); err != nil {
				t.Fatalf("plain assertion rejected: %v", err)
			}
		})
	}
	for _, kind := range scalar {
		seen[kind] = true
		t.Run(string(kind), func(t *testing.T) {
			if err := (ValidationAssertion{Kind: kind, Expected: "expected", IgnoreCase: true}).Validate(); err != nil {
				t.Fatalf("scalar assertion rejected: %v", err)
			}
		})
	}
	for _, kind := range regex {
		seen[kind] = true
		t.Run(string(kind), func(t *testing.T) {
			if err := (ValidationAssertion{Kind: kind, Expected: `^ready-[0-9]+$`}).Validate(); err != nil {
				t.Fatalf("regex assertion rejected: %v", err)
			}
		})
	}
	for _, kind := range set {
		seen[kind] = true
		t.Run(string(kind), func(t *testing.T) {
			if err := (ValidationAssertion{Kind: kind, ExpectedValues: []string{"east", "west"}}).Validate(); err != nil {
				t.Fatalf("set assertion rejected: %v", err)
			}
			if err := (ValidationAssertion{Kind: kind}).Validate(); err != nil {
				t.Fatalf("empty set assertion rejected: %v", err)
			}
		})
	}
	for _, kind := range attribute {
		seen[kind] = true
		t.Run(string(kind), func(t *testing.T) {
			if err := (ValidationAssertion{Kind: kind, Attribute: "data-state", Expected: "ready", IgnoreCase: true}).Validate(); err != nil {
				t.Fatalf("attribute assertion rejected: %v", err)
			}
		})
	}
	if len(seen) != 28 {
		t.Fatalf("covered assertion kinds = %d, want 28", len(seen))
	}
}

func TestValidationAssertionValidateRejectsIncompatibleFields(t *testing.T) {
	plain, scalar, regexKinds, setKinds, attributeKinds := validationKindsByContract()
	for _, kind := range plain {
		t.Run(string(kind), func(t *testing.T) {
			invalid := []ValidationAssertion{
				{Kind: kind, Expected: "x"},
				{Kind: kind, ExpectedValues: []string{"x"}},
				{Kind: kind, Attribute: "name"},
				{Kind: kind, IgnoreCase: true},
			}
			for _, assertion := range invalid {
				if err := assertion.Validate(); err == nil {
					t.Fatalf("plain assertion accepted incompatible fields: %#v", assertion)
				}
			}
		})
	}
	for _, kind := range scalar {
		t.Run(string(kind), func(t *testing.T) {
			for _, assertion := range []ValidationAssertion{
				{Kind: kind, ExpectedValues: []string{"x"}},
				{Kind: kind, Attribute: "name"},
			} {
				if err := assertion.Validate(); err == nil {
					t.Fatalf("scalar assertion accepted incompatible fields: %#v", assertion)
				}
			}
		})
	}
	for _, kind := range regexKinds {
		t.Run(string(kind), func(t *testing.T) {
			for _, assertion := range []ValidationAssertion{
				{Kind: kind, Expected: "x", ExpectedValues: []string{"x"}},
				{Kind: kind, Expected: "x", Attribute: "name"},
				{Kind: kind, Expected: "x", IgnoreCase: true},
			} {
				if err := assertion.Validate(); err == nil {
					t.Fatalf("regex assertion accepted incompatible fields: %#v", assertion)
				}
			}
		})
	}
	for _, kind := range setKinds {
		t.Run(string(kind), func(t *testing.T) {
			for _, assertion := range []ValidationAssertion{
				{Kind: kind, Expected: "x"},
				{Kind: kind, Attribute: "name"},
				{Kind: kind, IgnoreCase: true},
			} {
				if err := assertion.Validate(); err == nil {
					t.Fatalf("set assertion accepted incompatible fields: %#v", assertion)
				}
			}
		})
	}
	for _, kind := range attributeKinds {
		t.Run(string(kind), func(t *testing.T) {
			for _, assertion := range []ValidationAssertion{
				{Kind: kind, Expected: "x"},
				{Kind: kind, Attribute: "data-state", ExpectedValues: []string{"x"}},
				{Kind: kind, Attribute: "${attribute}", Expected: "x"},
			} {
				if err := assertion.Validate(); err == nil {
					t.Fatalf("attribute assertion accepted incompatible fields: %#v", assertion)
				}
			}
		})
	}
	if err := (ValidationAssertion{Kind: "eventually"}).Validate(); err == nil {
		t.Fatal("unknown validation kind was accepted")
	}
}

func TestValidationAssertionRegexTemplateAndCompilationContract(t *testing.T) {
	for _, kind := range []ValidationAssertionKind{ValidationTextMatches, ValidationValueMatches} {
		t.Run(string(kind), func(t *testing.T) {
			if err := (ValidationAssertion{Kind: kind, Expected: "["}).Validate(); err == nil || !strings.Contains(err.Error(), "invalid regular expression") {
				t.Fatalf("invalid static regex error = %v", err)
			}
			if err := (ValidationAssertion{Kind: kind, Expected: "${env.pattern}"}).Validate(); err != nil {
				t.Fatalf("runtime regex template rejected before expansion: %v", err)
			}
		})
	}
}

func TestValidationAssertionNormalizedMatrix(t *testing.T) {
	plain, scalar, regexKinds, setKinds, attributeKinds := validationKindsByContract()
	allFields := func(kind ValidationAssertionKind) ValidationAssertion {
		return ValidationAssertion{Kind: kind, Expected: "expected", ExpectedValues: []string{"one", "two"}, Attribute: "data-state", IgnoreCase: true}
	}
	for _, kind := range plain {
		t.Run(string(kind), func(t *testing.T) {
			got := allFields(kind).Normalized()
			if !reflect.DeepEqual(got, ValidationAssertion{Kind: kind}) {
				t.Fatalf("normalized = %#v", got)
			}
		})
	}
	for _, kind := range scalar {
		t.Run(string(kind), func(t *testing.T) {
			got := allFields(kind).Normalized()
			want := ValidationAssertion{Kind: kind, Expected: "expected", IgnoreCase: true}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("normalized = %#v, want %#v", got, want)
			}
		})
	}
	for _, kind := range regexKinds {
		t.Run(string(kind), func(t *testing.T) {
			got := allFields(kind).Normalized()
			want := ValidationAssertion{Kind: kind, Expected: "expected"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("normalized = %#v, want %#v", got, want)
			}
		})
	}
	for _, kind := range setKinds {
		t.Run(string(kind), func(t *testing.T) {
			got := allFields(kind).Normalized()
			want := ValidationAssertion{Kind: kind, ExpectedValues: []string{"one", "two"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("normalized = %#v, want %#v", got, want)
			}
		})
	}
	for _, kind := range attributeKinds {
		t.Run(string(kind), func(t *testing.T) {
			got := allFields(kind).Normalized()
			want := ValidationAssertion{Kind: kind, Expected: "expected", Attribute: "data-state", IgnoreCase: true}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("normalized = %#v, want %#v", got, want)
			}
		})
	}
	trimmed := (ValidationAssertion{Kind: "  visible  ", Expected: "stale"}).Normalized()
	if !reflect.DeepEqual(trimmed, ValidationAssertion{Kind: ValidationVisible}) {
		t.Fatalf("kind whitespace was not normalized: %#v", trimmed)
	}
}

func TestValidationWaitValidateBoundaryMatrix(t *testing.T) {
	for _, wait := range []ValidationWait{
		{MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinStabilityMS},
		{MaxWaitMS: validationMaxWaitMS, StabilityMS: validationMaxStabilityMS},
		{MaxWaitMS: validationMaxStabilityMS + 1, StabilityMS: validationMaxStabilityMS},
	} {
		if err := wait.Validate(); err != nil {
			t.Fatalf("valid boundary wait %#v rejected: %v", wait, err)
		}
	}
	tests := []struct {
		name string
		wait ValidationWait
		want string
	}{
		{name: "maximum below minimum", wait: ValidationWait{MaxWaitMS: validationMinWaitMS - 1, StabilityMS: validationMinStabilityMS}, want: "maximum wait"},
		{name: "maximum above maximum", wait: ValidationWait{MaxWaitMS: validationMaxWaitMS + 1, StabilityMS: validationMinStabilityMS}, want: "maximum wait"},
		{name: "stability below minimum", wait: ValidationWait{MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinStabilityMS - 1}, want: "stability window"},
		{name: "stability above maximum", wait: ValidationWait{MaxWaitMS: validationMaxWaitMS, StabilityMS: validationMaxStabilityMS + 1}, want: "stability window"},
		{name: "stability equals maximum wait", wait: ValidationWait{MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinWaitMS}, want: "shorter than"},
		{name: "stability exceeds maximum wait", wait: ValidationWait{MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinWaitMS + 1}, want: "shorter than"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.wait.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}
