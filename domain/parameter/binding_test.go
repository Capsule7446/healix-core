package parameter

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
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

// All three variants share one code on purpose: the caller declared the binding
// and owns the parent scope, so a per-reason i18n key would tell it nothing it
// cannot already read from its own input.
func TestBindingRejectsInvalidVariantsAndMissingParents(t *testing.T) {
	const secretParent = "absent-parent-8f21"
	// The three used to share one code, which told a caller holding a malformed
	// binding to go fix the surrounding scope. A binding the caller never built
	// usably is theirs to correct; only a well-formed reference into a scope that
	// lacks the value is a precondition on something else.
	tests := []struct {
		name     string
		binding  Binding
		parent   map[string]Value
		wantCode fault.Code
		wantKind fault.Kind
	}{
		{name: "zero", binding: Binding{}, wantCode: CodeBindingInvalid, wantKind: fault.InvalidArgument},
		{name: "blank reference", binding: ParentReferenceBinding(""), wantCode: CodeBindingInvalid, wantKind: fault.InvalidArgument},
		{name: "missing reference", binding: ParentReferenceBinding(secretParent), parent: map[string]Value{}, wantCode: CodeBindingUnresolvable, wantKind: fault.FailedPrecondition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.binding.Resolve(test.parent)
			if !value.Equal(Value{}) {
				t.Fatalf("Resolve() returned %#v on failure", value)
			}
			if !fault.IsCode(err, test.wantCode) {
				t.Fatalf("error = %v, want code %s", err, test.wantCode)
			}
			descriptor, ok := fault.Describe(err)
			if !ok || descriptor.Kind() != test.wantKind || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
				t.Fatalf("descriptor = %#v (ok=%v), want kind %s", descriptor, ok, test.wantKind)
			}
			if strings.Contains(err.Error(), secretParent) {
				t.Fatalf("public error leaked the parent parameter name: %q", err)
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
