package parameter

import (
	"strings"
	"testing"
)

func TestLiteralBindingPreservesTypedValueAndClonesCollections(t *testing.T) {
	items := []string{"north,east", "south"}
	binding := LiteralBinding(MultiSelectValue(items))
	items[0] = "mutated"

	value, ok := binding.Literal()
	if !ok || value.Type() != MultiSelect || value.MultiSelect()[0] != "north,east" {
		t.Fatalf("literal = %#v/%v", value, ok)
	}
	copy := value.MultiSelect()
	copy[0] = "changed"
	resolved, err := binding.Clone().Resolve(nil)
	if err != nil || resolved.MultiSelect()[0] != "north,east" {
		t.Fatalf("resolved = %#v, error = %v", resolved, err)
	}
	if _, ok := binding.ParentName(); ok || binding.Kind() != LiteralBindingKind {
		t.Fatal("literal exposed as parent reference")
	}
}

func TestParentReferenceBindingResolvesOnlyNamedTypedParent(t *testing.T) {
	binding := ParentReferenceBinding("count")
	number, err := NewNumberValue("01.20")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := binding.Resolve(map[string]Value{"count": number})
	if err != nil || resolved.Number() != "1.2" {
		t.Fatalf("resolved = %#v, error = %v", resolved, err)
	}
	name, ok := binding.ParentName()
	if !ok || name != "count" || binding.Kind() != ParentReferenceBindingKind {
		t.Fatalf("reference = %q/%v/%q", name, ok, binding.Kind())
	}
	if _, ok := binding.Literal(); ok {
		t.Fatal("reference exposed as literal")
	}
}

func TestBindingRejectsInvalidVariantsAndMissingParents(t *testing.T) {
	tests := []struct {
		name    string
		binding Binding
		parent  map[string]Value
		want    string
	}{
		{name: "zero", binding: Binding{}, want: "unsupported"},
		{name: "blank reference", binding: ParentReferenceBinding(""), want: "name is required"},
		{name: "missing reference", binding: ParentReferenceBinding("absent"), parent: map[string]Value{}, want: "is missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.binding.Resolve(test.parent); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestConstraintValidatesExactTypeAndSelectMembership(t *testing.T) {
	tests := []struct {
		name       string
		constraint Constraint
		value      Value
		wantError  bool
	}{
		{"text", Constraint{Type: Text}, TextValue("${literal}"), false},
		{"type mismatch", Constraint{Type: Text}, BooleanValue(true), true},
		{"single accepted", Constraint{Type: SingleSelect, Options: []string{"east"}}, SingleSelectValue("east"), false},
		{"single rejected", Constraint{Type: SingleSelect, Options: []string{"east"}}, SingleSelectValue("west"), true},
		{"multi accepted with comma", Constraint{Type: MultiSelect, Options: []string{"north,east", "south"}}, MultiSelectValue([]string{"north,east", "south"}), false},
		{"multi rejected", Constraint{Type: MultiSelect, Options: []string{"south"}}, MultiSelectValue([]string{"north"}), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.constraint.Validate(test.value)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
