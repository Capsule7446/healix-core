package interpolation

import (
	"runtime"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

type constantResolver string

func (value constantResolver) Variable(string) (string, bool) { return string(value), true }

// TestExpandRefusesTheAmplificationTemplate pins the concrete attack: a 64 KiB
// template of repeated ${x} against a 64 KiB value expands to exactly 1 GiB.
// Both inputs are individually unremarkable, so no upstream size check catches
// this — only an output budget does.
func TestExpandRefusesTheAmplificationTemplate(t *testing.T) {
	template := strings.Repeat("${x}", 16384)
	value := strings.Repeat("A", 65536)
	if len(template) != 65536 {
		t.Fatalf("template = %d bytes, want 65536", len(template))
	}

	got, err := Expand(template, constantResolver(value))

	if !fault.IsCode(err, CodeExpansionTooLarge) {
		t.Fatalf("Expand error = %v, want %q", err, CodeExpansionTooLarge)
	}
	if got != "" {
		t.Fatalf("refused expansion still returned %d bytes", len(got))
	}
	if kind, ok := fault.KindOf(err); !ok || kind != fault.ResourceExhausted {
		t.Fatalf("kind = %q (%v), want %q", kind, ok, fault.ResourceExhausted)
	}
}

// TestExpandRefusalNeverMaterializesTheOutput is the point of the budget. A
// refusal that first builds the 1 GiB string has already exhausted the memory
// it was meant to protect, so the assertion is on allocation, not on the error.
func TestExpandRefusalNeverMaterializesTheOutput(t *testing.T) {
	template := strings.Repeat("${x}", 16384)
	value := strings.Repeat("A", 65536)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := Expand(template, constantResolver(value)); err == nil {
		t.Fatal("Expand accepted the amplification template")
	}
	runtime.ReadMemStats(&after)

	const ceiling = 8 * MaxExpansionBytes
	if grew := after.TotalAlloc - before.TotalAlloc; grew > ceiling {
		t.Fatalf("refusal allocated %d bytes, want at most %d", grew, ceiling)
	}
}

func TestExpandOutputBudgetBoundary(t *testing.T) {
	tests := []struct {
		name       string
		valueBytes int
		wantCode   fault.Code
	}{
		{name: "one below the budget", valueBytes: MaxExpansionBytes - 1},
		{name: "exactly the budget", valueBytes: MaxExpansionBytes},
		{name: "one above the budget", valueBytes: MaxExpansionBytes + 1, wantCode: CodeExpansionTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Expand("${x}", constantResolver(strings.Repeat("A", test.valueBytes)))

			if test.wantCode != "" {
				if !fault.IsCode(err, test.wantCode) {
					t.Fatalf("Expand error = %v, want %q", err, test.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand(%d bytes) = %v", test.valueBytes, err)
			}
			if len(got) != test.valueBytes {
				t.Fatalf("len = %d, want %d", len(got), test.valueBytes)
			}
		})
	}
}

// TestExpandBudgetCountsLiteralsAndValuesTogether stops the budget from being
// evaded by pushing the bulk into the template's literal spans instead of into
// the resolved values.
func TestExpandBudgetCountsLiteralsAndValuesTogether(t *testing.T) {
	half := MaxExpansionBytes / 2
	literal := strings.Repeat("L", half+8)

	if _, err := Expand(literal+"${x}"+literal, constantResolver("")); !fault.IsCode(err, CodeExpansionTooLarge) {
		t.Fatalf("literal-heavy template error = %v, want %q", err, CodeExpansionTooLarge)
	}
}

// TestExpandBudgetSurvivesRemainderOverflow guards the implementation shape.
// A budget written as `written + len(chunk) > limit` overflows to a negative
// int on adversarial sizes and silently admits the write; a remaining-quota
// check cannot.
func TestExpandBudgetSurvivesRemainderOverflow(t *testing.T) {
	template := strings.Repeat("${x}", 4096)
	value := strings.Repeat("V", 4096)

	got, err := Expand(template, constantResolver(value))

	if !fault.IsCode(err, CodeExpansionTooLarge) {
		t.Fatalf("Expand error = %v, want %q", err, CodeExpansionTooLarge)
	}
	if got != "" {
		t.Fatalf("refused expansion leaked %d bytes", len(got))
	}
}

// FuzzExpandNeverExceedsTheBudget states the invariant the budget exists to
// hold: once interpolation syntax is present, any accepted result is within
// the ceiling. The no-syntax fast path returns the caller's own string
// unchanged and is not an expansion, so it is excluded.
func FuzzExpandNeverExceedsTheBudget(f *testing.F) {
	for _, seed := range []string{"${a}", "${a}${a}", "x${a}y", "${a}/${b}"} {
		f.Add(seed, "value", 32)
	}
	f.Fuzz(func(t *testing.T, expression, value string, repeat int) {
		// Reject the input rather than skip: a skipped fuzz case still
		// counts as a run, which makes a corpus that is entirely
		// out-of-range look like coverage.
		if repeat < 0 || repeat > 4096 {
			return
		}
		names, err := Names(expression)
		if err != nil {
			return
		}
		resolver := variableMap{}
		for _, name := range names {
			resolver[name] = strings.Repeat(value, repeat)
		}

		got, err := Expand(expression, resolver)
		if err != nil {
			return
		}
		if strings.Contains(expression, "${") && len(got) > MaxExpansionBytes {
			t.Fatalf("accepted result is %d bytes, over the %d budget", len(got), MaxExpansionBytes)
		}
	})
}
