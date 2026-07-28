package parameter

import (
	"strings"
	"testing"
)

func TestLiteralBindingRejectsInvalidClosedValueBeforeResolution(t *testing.T) {
	for _, binding := range []Binding{
		LiteralBinding(Value{}),
		LiteralBinding(TextValue(strings.Repeat("x", MaxValueStringBytes+1))),
	} {
		if _, err := binding.Resolve(nil); err == nil {
			t.Fatalf("Resolve() accepted invalid literal %#v", binding)
		}
	}
}

func FuzzNumberCanonicalizationIsIdempotentAndValid(f *testing.F) {
	for _, seed := range []string{
		"0",
		"-0.00",
		"001.2300e+2",
		"1e-100000",
		"1e100000",
		"1e-100001",
		"1e100001",
		"NaN",
		"+🚀",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		value, err := NewNumberValue(input)
		if err != nil {
			return
		}
		if err := value.Validate(); err != nil {
			t.Fatalf("accepted NUMBER %q produced invalid value %q: %v", input, value.Number(), err)
		}
		if len(value.Number()) > MaxValueStringBytes {
			t.Fatalf("accepted NUMBER %q exceeded output limit: %d", input, len(value.Number()))
		}
		again, err := NewNumberValue(value.Number())
		if err != nil || !again.Equal(value) {
			t.Fatalf("canonicalization is not idempotent: input=%q first=%q second=%q err=%v", input, value.Number(), again.Number(), err)
		}
		if value.Number() == "-0" {
			t.Fatalf("canonical zero retained a negative sign: %q", value.Number())
		}
	})
}
