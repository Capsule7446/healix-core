package parameter

import (
	"strings"
	"testing"
)

func TestValidateNameContract(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{name: "blank", value: " \t", wantError: true},
		{name: "malformed UTF-8", value: string([]byte{0xff}), wantError: true},
		{name: "ASCII control", value: "region\nname", wantError: true},
		{name: "Unicode control", value: "region" + string(rune(0x85)) + "name", wantError: true},
		{name: "Unicode format control", value: "region" + string(rune(0x202e)) + "name", wantError: true},
		{name: "zero width format control", value: "region" + string(rune(0x200b)) + "name", wantError: true},
		{name: "over byte limit", value: strings.Repeat("x", MaxNameBytes+1), wantError: true},
		{name: "exact byte limit", value: strings.Repeat("界", MaxNameBytes/3) + "x"},
		{name: "surrounding whitespace preserved", value: " region "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateName(test.value)
			if (err != nil) != test.wantError {
				t.Fatalf("ValidateName() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}
