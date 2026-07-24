package parameter

import (
	"strings"
	"testing"
)

func TestMultiSelectMetricsInspectWithoutExposingPayload(t *testing.T) {
	source := []string{"a,b", "c"}
	value := MultiSelectValue(source)
	count, totalBytes, maxItemBytes, ok := value.MultiSelectMetrics()
	if !ok || count != 2 || totalBytes != 4 || maxItemBytes != 3 {
		t.Fatalf("metrics = %d/%d/%d/%v", count, totalBytes, maxItemBytes, ok)
	}
	source[0] = "mutated"
	if value.MultiSelect()[0] != "a,b" {
		t.Fatal("metrics API weakened immutable ownership")
	}
	if _, _, _, ok := TextValue("x").MultiSelectMetrics(); ok {
		t.Fatal("non-multi value reported multi metrics")
	}
}

func TestValueAccessorsPreserveEachPublicType(t *testing.T) {
	number, err := NewNumberValue("01.20")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		value Value
		kind  Type
		check func(Value) bool
	}{
		{"text", TextValue("hello"), Text, func(value Value) bool { return value.Text() == "hello" }},
		{"number", number, Number, func(value Value) bool { return value.Number() == "1.2" }},
		{"boolean", BooleanValue(true), Boolean, func(value Value) bool { return value.Boolean() }},
		{"single select", SingleSelectValue("east"), SingleSelect, func(value Value) bool { return value.SingleSelect() == "east" }},
		{"multi select", MultiSelectValue([]string{"north,east", "south"}), MultiSelect, func(value Value) bool {
			items := value.MultiSelect()
			return len(items) == 2 && items[0] == "north,east" && items[1] == "south"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.value.Type() != test.kind || !test.check(test.value) {
				t.Fatalf("value/type = %#v/%q", test.value, test.value.Type())
			}
		})
	}
}

func TestOptionalValueDistinguishesAbsentFromPresentZeroValues(t *testing.T) {
	var absent OptionalValue
	if absent.IsPresent() {
		t.Fatal("zero optional value reported present")
	}
	if value, present := absent.Value(); present || value.Type() != "" {
		t.Fatalf("absent Value() = %#v/%v", value, present)
	}
	for _, value := range []Value{TextValue(""), BooleanValue(false), MultiSelectValue(nil)} {
		optional := PresentValue(value)
		got, present := optional.Value()
		if !optional.IsPresent() || !present || !got.Equal(value) {
			t.Fatalf("present zero value lost: %#v/%v", got, present)
		}
	}
}

func TestValueValidateAcceptsEveryKindAndRejectsInvalidValues(t *testing.T) {
	number, err := NewNumberValue("1.25")
	if err != nil {
		t.Fatal(err)
	}
	valid := []Value{TextValue(""), number, BooleanValue(false), SingleSelectValue("east"), MultiSelectValue([]string{"east"})}
	for _, value := range valid {
		if err := value.Validate(); err != nil {
			t.Fatalf("Validate(%q) = %v", value.Type(), err)
		}
	}
	invalid := []Value{{}, {kind: Type("DATE")}, {kind: Number, text: "01"}, {kind: Number, text: "not-number"}}
	for _, value := range invalid {
		if err := value.Validate(); err == nil {
			t.Fatalf("Validate(%#v) accepted invalid value", value)
		}
	}
}

func TestValueEqualDetectsSemanticDifferences(t *testing.T) {
	tests := []struct {
		name        string
		left, right Value
	}{
		{"kind", TextValue("east"), SingleSelectValue("east")},
		{"text", TextValue("east"), TextValue("west")},
		{"boolean", BooleanValue(true), BooleanValue(false)},
		{"multi length", MultiSelectValue([]string{"east"}), MultiSelectValue([]string{"east", "west"})},
		{"multi element", MultiSelectValue([]string{"east", "west"}), MultiSelectValue([]string{"east", "north"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.left.Equal(test.right) || test.right.Equal(test.left) {
				t.Fatalf("different values compared equal: %#v/%#v", test.left, test.right)
			}
		})
	}
	if !MultiSelectValue([]string{"east", "west"}).Equal(MultiSelectValue([]string{"east", "west"})) {
		t.Fatal("equivalent multi-select values did not compare equal")
	}
}

func TestNumberCanonicalization(t *testing.T) {
	tests := map[string]string{"001.2300e+2": "123", "100": "100", "-0.00": "0", "1e-2": "0.01"}
	for input, expected := range tests {
		value, err := NewNumberValue(input)
		if err != nil {
			t.Fatalf("NewNumberValue(%q): %v", input, err)
		}
		if value.Number() != expected {
			t.Fatalf("NewNumberValue(%q) = %q, want %q", input, value.Number(), expected)
		}
	}
	for _, invalid := range []string{"", "NaN", "Inf", "1.2.3", "1e100001"} {
		if _, err := NewNumberValue(invalid); err == nil {
			t.Fatalf("NewNumberValue(%q) accepted invalid value", invalid)
		}
	}
}

func TestNumberRejectsOversizedInputBeforeCanonicalization(t *testing.T) {
	oversizedMantissa := strings.Repeat("9", MaxValueStringBytes+1)
	if _, err := NewNumberValue(oversizedMantissa); err == nil {
		t.Fatal("oversized mantissa accepted")
	}
	if _, err := NewNumberValue("1e65536"); err == nil {
		t.Fatal("oversized canonical output accepted")
	}
}

func TestValueAcceptsStringsAtExactLimit(t *testing.T) {
	maximum := strings.Repeat("x", MaxValueStringBytes)
	for _, value := range []Value{
		TextValue(maximum),
		SingleSelectValue(maximum),
		MultiSelectValue([]string{"valid", maximum}),
	} {
		if err := value.Validate(); err != nil {
			t.Fatalf("Validate(%s) rejected exact-limit string: %v", value.Type(), err)
		}
	}
}

func TestValueRejectsOversizedStrings(t *testing.T) {
	oversized := strings.Repeat("x", MaxValueStringBytes+1)
	for _, value := range []Value{
		TextValue(oversized),
		SingleSelectValue(oversized),
		MultiSelectValue([]string{"valid", oversized}),
	} {
		if err := value.Validate(); err == nil {
			t.Fatalf("Validate(%s) accepted oversized string", value.Type())
		}
	}
}

func TestClosedValuesCloneAndValidate(t *testing.T) {
	values := []Value{TextValue(""), BooleanValue(true), SingleSelectValue("east"), MultiSelectValue([]string{"east", "west"})}
	for _, value := range values {
		if err := value.Validate(); err != nil {
			t.Fatalf("Validate(%s): %v", value.Type(), err)
		}
	}
	multi := values[3]
	copy := multi.MultiSelect()
	copy[0] = "mutated"
	if multi.MultiSelect()[0] != "east" {
		t.Fatal("MultiSelect exposed mutable storage")
	}
	optional := PresentValue(multi)
	got, present := optional.Value()
	if !present || !got.Equal(multi) {
		t.Fatal("optional value lost presence or value")
	}
}
