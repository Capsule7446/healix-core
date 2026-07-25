package automation

import "github.com/Capsule7446/healix-core/domain/parameter"

import "testing"

func TestParameterValueConstructorsAndCanonicalNumber(t *testing.T) {
	number, err := parameter.NewNumberValue("001.2300e+2")
	if err != nil {
		t.Fatal(err)
	}
	if number.Type() != parameter.Number || number.Number() != "123" {
		t.Fatalf("unexpected canonical number: %#v", number)
	}
	hundred, err := parameter.NewNumberValue("100")
	if err != nil || hundred.Number() != "100" {
		t.Fatalf("integer trailing zero canonicalization = %q, %v", hundred.Number(), err)
	}
	if _, err := parameter.NewNumberValue("NaN"); err == nil {
		t.Fatal("expected non-decimal number rejection")
	}
	multi := parameter.MultiSelectValue([]string{"north", "south"})
	values := multi.MultiSelect()
	values[0] = "mutated"
	if multi.MultiSelect()[0] != "north" {
		t.Fatal("multi-select value leaked mutable storage")
	}
}

func TestParameterDefinitionDefaultPresenceAndTypeValidation(t *testing.T) {
	tests := []struct {
		name       string
		definition ParameterDefinition
		wantError  bool
	}{
		{"optional missing default", ParameterDefinition{Name: "region", DisplayName: "Region", Type: parameter.Text}, true},
		{"optional empty text default", ParameterDefinition{Name: "region", DisplayName: "Region", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue(""))}, false},
		{"required has default", ParameterDefinition{Name: "region", DisplayName: "Region", Type: parameter.Text, Required: true, Default: parameter.PresentValue(parameter.TextValue("x"))}, true},
		{"wrong typed default", ParameterDefinition{Name: "enabled", DisplayName: "Enabled", Type: parameter.Boolean, Default: parameter.PresentValue(parameter.TextValue("true"))}, true},
		{"invalid select default", ParameterDefinition{Name: "region", DisplayName: "Region", Type: parameter.SingleSelect, Options: []string{"east"}, Default: parameter.PresentValue(parameter.SingleSelectValue("west"))}, true},
		{"duplicate multi default", ParameterDefinition{Name: "regions", DisplayName: "Regions", Type: parameter.MultiSelect, Options: []string{"east"}, Default: parameter.PresentValue(parameter.MultiSelectValue([]string{"east", "east"}))}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.definition.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestResolveParameterValuesRejectsUnknownMissingAndMismatch(t *testing.T) {
	definitions := []ParameterDefinition{
		{Name: "required", DisplayName: "Required", Type: parameter.Boolean, Required: true},
		{Name: "optional", DisplayName: "Optional", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue(""))},
	}
	if _, err := ResolveParameterValues(definitions, map[string]parameter.Value{}); err == nil {
		t.Fatal("expected missing required error")
	}
	if _, err := ResolveParameterValues(definitions, map[string]parameter.Value{"required": parameter.BooleanValue(true), "unknown": parameter.TextValue("x")}); err == nil {
		t.Fatal("expected unknown parameter error")
	}
	if _, err := ResolveParameterValues(definitions, map[string]parameter.Value{"required": parameter.TextValue("true")}); err == nil {
		t.Fatal("expected type mismatch")
	}
	resolved, err := ResolveParameterValues(definitions, map[string]parameter.Value{"required": parameter.BooleanValue(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved["optional"].Equal(parameter.TextValue("")) {
		t.Fatal("default was not resolved")
	}
}
