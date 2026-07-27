package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestEnvironmentVariablesValidateAllValueTypes(t *testing.T) {
	number, err := parameter.NewNumberValue("1.25")
	if err != nil {
		t.Fatal(err)
	}
	variables := EnvironmentVariables{
		"text":    parameter.TextValue("value"),
		"number":  number,
		"boolean": parameter.BooleanValue(true),
		"single":  parameter.SingleSelectValue("east"),
		"multi":   parameter.MultiSelectValue([]string{"east", "west"}),
	}
	if err := variables.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, variables := range map[string]EnvironmentVariables{
		"blank name": {" ": parameter.TextValue("value")},
		"zero value": {"invalid": {}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := variables.Validate(); err == nil {
				t.Fatal("invalid environment variables accepted")
			}
		})
	}
	overLimit := EnvironmentVariables{"large": parameter.TextValue(strings.Repeat("x", parameter.MaxValueStringBytes+1))}
	if err := overLimit.Validate(); err == nil {
		t.Fatal("oversized environment value accepted")
	}
}

func TestEnvironmentVariablesValidateNameContract(t *testing.T) {
	tests := []struct {
		name         string
		variableName string
		wantError    bool
	}{
		{name: "malformed UTF-8", variableName: string([]byte{0xff}), wantError: true},
		{name: "control character", variableName: "region\nname", wantError: true},
		{name: "format control character", variableName: "region" + string(rune(0x202e)) + "name", wantError: true},
		{name: "over byte limit", variableName: strings.Repeat("x", parameter.MaxNameBytes+1), wantError: true},
		{name: "exact byte limit with multibyte rune", variableName: strings.Repeat("界", parameter.MaxNameBytes/3) + "x", wantError: false},
		{name: "surrounding whitespace preserved", variableName: " region ", wantError: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (EnvironmentVariables{test.variableName: parameter.TextValue("value")}).Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

func TestEnvironmentVariablesCloneNormalizesNil(t *testing.T) {
	var variables EnvironmentVariables
	cloned := variables.Clone()
	if cloned == nil || len(cloned) != 0 {
		t.Fatalf("Clone() = %#v, want empty non-nil map", cloned)
	}
}

func TestEnvironmentVariablesCloneOwnsMultiSelect(t *testing.T) {
	source := EnvironmentVariables{"regions": parameter.MultiSelectValue([]string{"east", "west"})}
	cloned := source.Clone()
	source["regions"] = parameter.MultiSelectValue([]string{"mutated"})
	if got := cloned["regions"].MultiSelect(); len(got) != 2 || got[0] != "east" {
		t.Fatalf("clone changed with source: %v", got)
	}
	cloned["regions"] = parameter.MultiSelectValue([]string{"changed"})
	if got := source["regions"].MultiSelect(); len(got) != 1 || got[0] != "mutated" {
		t.Fatalf("source changed with clone: %v", got)
	}
}
