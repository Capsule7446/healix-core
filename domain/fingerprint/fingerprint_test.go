package fingerprint

import (
	"strings"
	"testing"
)

func TestSelectorValidate(t *testing.T) {
	valid := Selector{Type: SelectorCSS, Value: "#submit", Priority: 0}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid selector: %v", err)
	}
	for name, selector := range map[string]Selector{
		"type":     {Type: "shadow", Value: "#submit"},
		"value":    {Type: SelectorCSS},
		"priority": {Type: SelectorCSS, Value: "#submit", Priority: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := selector.Validate(); err == nil {
				t.Fatal("invalid selector unexpectedly passed")
			}
		})
	}
}

func TestSelectorValidateBusinessMatrix(t *testing.T) {
	for _, selectorType := range []SelectorType{SelectorRole, SelectorTestID, SelectorCSS, SelectorXPath, SelectorText} {
		t.Run("valid/"+string(selectorType), func(t *testing.T) {
			if err := (Selector{Type: selectorType, Value: "locator", Priority: 1}).Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, test := range []struct {
		name     string
		selector Selector
		want     string
	}{
		{name: "empty type", selector: Selector{Value: "locator"}, want: "unsupported type"},
		{name: "unknown type", selector: Selector{Type: "shadow", Value: "locator"}, want: "unsupported type"},
		{name: "empty value", selector: Selector{Type: SelectorCSS}, want: "value is required"},
		{name: "whitespace value", selector: Selector{Type: SelectorCSS, Value: " \t"}, want: "value is required"},
		{name: "negative priority", selector: Selector{Type: SelectorCSS, Value: "#submit", Priority: -1}, want: "priority"},
	} {
		t.Run("invalid/"+test.name, func(t *testing.T) {
			if err := test.selector.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want containing %q", err, test.want)
			}
		})
	}
}

func TestNodeSpecValidateInvariantMatrix(t *testing.T) {
	valid := func() ElementTargetSpec {
		return ElementTargetSpec{UUID: "123e4567-e89b-42d3-a456-426614174000", ID: "login.submit",
			Selectors:   []Selector{{Type: SelectorCSS, Value: "#submit"}},
			Fingerprint: Fingerprint{Tag: "button", Attributes: map[string]string{}, SiblingIndex: 0}}
	}
	tests := []struct {
		name   string
		mutate func(*ElementTargetSpec)
		want   string
	}{
		{name: "bad uuid", mutate: func(spec *ElementTargetSpec) { spec.UUID = "bad" }, want: "canonical UUID"},
		{name: "blank id", mutate: func(spec *ElementTargetSpec) { spec.ID = "  " }, want: "id is required"},
		{name: "no selectors", mutate: func(spec *ElementTargetSpec) { spec.Selectors = nil }, want: "at least 1"},
		{name: "bad selector", mutate: func(spec *ElementTargetSpec) { spec.Selectors[0].Priority = -1 }, want: "selectors[0]"},
		{name: "blank tag", mutate: func(spec *ElementTargetSpec) { spec.Fingerprint.Tag = " " }, want: "tag is required"},
		{name: "nil attributes", mutate: func(spec *ElementTargetSpec) { spec.Fingerprint.Attributes = nil }, want: "attributes is required"},
		{name: "negative sibling", mutate: func(spec *ElementTargetSpec) { spec.Fingerprint.SiblingIndex = -1 }, want: "sibling_index"},
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := valid()
			test.mutate(&spec)
			if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want containing %q", err, test.want)
			}
		})
	}
}

func FuzzSelectorValidateNeverPanics(f *testing.F) {
	f.Add("css", "#submit", 0)
	f.Add("shadow", "", -1)
	f.Fuzz(func(t *testing.T, selectorType, value string, priority int) {
		_ = (Selector{Type: SelectorType(selectorType), Value: value, Priority: priority}).Validate()
	})
}

func TestNodeSpecValidate(t *testing.T) {
	valid := ElementTargetSpec{
		ID:          "login.submit",
		Selectors:   []Selector{{Type: SelectorRole, Value: "button[name=Login]"}},
		Fingerprint: Fingerprint{Tag: "button", Attributes: map[string]string{}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid node spec: %v", err)
	}
	valid.ID = ""
	valid.Selectors = nil
	valid.Fingerprint.Attributes = nil
	if err := valid.Validate(); err == nil {
		t.Fatal("invalid node spec unexpectedly passed")
	}
}

func TestNodeSpecValidateOptionalUUID(t *testing.T) {
	spec := ElementTargetSpec{
		UUID: "not-a-uuid", ID: "login.submit",
		Selectors:   []Selector{{Type: SelectorCSS, Value: "#submit"}},
		Fingerprint: Fingerprint{Tag: "button", Attributes: map[string]string{}},
	}
	if err := spec.Validate(); err == nil {
		t.Fatal("invalid UUID unexpectedly passed")
	}
	spec.UUID = "123e4567-e89b-42d3-a456-426614174000"
	if err := spec.Validate(); err != nil {
		t.Fatalf("valid UUID: %v", err)
	}
}
