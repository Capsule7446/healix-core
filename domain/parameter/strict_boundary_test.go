package parameter

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestValueStrictBoundariesAndImmutability(t *testing.T) {
	for _, input := range []string{"", " ", "\n", "NaN", "Inf", "1e100001", "1e-100001"} {
		if _, err := NewNumberValue(input); err == nil {
			t.Errorf("NewNumberValue(%q) accepted", input)
		}
	}
	for input, want := range map[string]string{"-1": "-1", "0": "0", "+1": "1", "01.00": "1", "1e1": "10"} {
		got, err := NewNumberValue(input)
		if err != nil || got.Number() != want {
			t.Errorf("NewNumberValue(%q) = %q, %v; want %q", input, got.Number(), err, want)
		}
	}
	items := []string{"", "重复", "重复"}
	value := MultiSelectValue(items)
	items[0] = "mutated"
	got := value.MultiSelect()
	if !reflect.DeepEqual(got, []string{"", "重复", "重复"}) {
		t.Fatalf("constructor aliased input: %v", got)
	}
	got[0] = "mutated"
	if value.MultiSelect()[0] != "" {
		t.Fatal("accessor exposed internal slice")
	}
	if count, _, _, ok := value.MultiSelectMetrics(); !ok || count != 3 {
		t.Fatalf("metrics = %d, %v", count, ok)
	}
	if _, _, _, ok := TextValue("x").MultiSelectMetrics(); ok {
		t.Fatal("text reported multi-select metrics")
	}
	_ = math.MaxInt
}

func TestConstraintStrictTypesOptionsAndBinding(t *testing.T) {
	for _, constraint := range []Constraint{{}, {Type: Type("UNKNOWN")}} {
		if err := constraint.Validate(Value{}); err == nil {
			t.Errorf("constraint %+v accepted", constraint)
		}
	}
	if err := (Constraint{Type: SingleSelect, Options: nil}).Validate(SingleSelectValue("")); err == nil {
		t.Fatal("nil options accepted select")
	}
	if err := (Constraint{Type: MultiSelect, Options: []string{"a"}}).Validate(MultiSelectValue([]string{"a", "a"})); err != nil {
		t.Fatalf("duplicates unexpectedly rejected: %v", err)
	}

	parents := map[string]Value{"名称": TextValue("值")}
	resolved, err := ParentReferenceBinding("名称").Resolve(parents)
	if err != nil || resolved.Text() != "值" {
		t.Fatalf("unicode binding: %v, %v", resolved, err)
	}
	for _, name := range []string{"", " ", "\n"} {
		if _, err := ParentReferenceBinding(name).Resolve(map[string]Value{name: TextValue("x")}); err == nil {
			t.Errorf("parent name %q accepted", name)
		}
	}
	large := strings.Repeat("x", MaxValueStringBytes+1)
	if err := TextValue(large).Validate(); err == nil {
		t.Fatal("oversized text accepted")
	}
}
