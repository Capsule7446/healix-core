package execution

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

// Go randomises map iteration, so a validator that ranges a map and returns on
// the first offending entry reports a different failure on a different run for
// the same input. 3e56ba2 already fixed one of these in domain/evidence and
// recorded why it matters: a stable code is only worth having if the cause
// underneath it is a function of the input.
//
// Repeating the call is the only assertion that catches it. One call always
// passes, whichever entry it happened to pick.
const determinismRuns = 200

func TestValidateSnapshotValuesReportsTheSameUnknownKeyEveryRun(t *testing.T) {
	definitions := []Parameter{{Name: "known", DisplayName: "Known", Type: parameter.Text, Required: true}}
	values := map[string]parameter.Value{
		"known": parameter.TextValue("value"),
		"alpha": parameter.TextValue("a"),
		"beta":  parameter.TextValue("b"),
		"gamma": parameter.TextValue("c"),
	}

	first := ""
	for run := 0; run < determinismRuns; run++ {
		err := validateSnapshotValues(definitions, values)
		if err == nil {
			t.Fatalf("run %d accepted three unknown parameters", run)
		}
		if run == 0 {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("run %d reported %q, run 0 reported %q; the reported key depends on map iteration order",
				run, err.Error(), first)
		}
	}
	// Sorted order makes the choice predictable rather than merely stable.
	if !strings.Contains(first, `"alpha"`) {
		t.Fatalf("reported %q, want the lexicographically first unknown key", first)
	}
}

func TestValidateBindingsReportsTheSameOffendingBindingEveryRun(t *testing.T) {
	parents := []Parameter{{Name: "parent", Type: parameter.Text}}
	child := []Parameter{{Name: "child", Type: parameter.Text}}
	// Three unknown binding names, so which one is reported is decided purely by
	// iteration order unless the walk is ordered.
	bindings := map[string]parameter.Binding{
		"zulu":  parameter.LiteralBinding(parameter.TextValue("z")),
		"alpha": parameter.LiteralBinding(parameter.TextValue("a")),
		"mike":  parameter.LiteralBinding(parameter.TextValue("m")),
	}

	first := ""
	for run := 0; run < determinismRuns; run++ {
		err := validateBindings(parents, child, bindings)
		if err == nil {
			t.Fatalf("run %d accepted three unknown bindings", run)
		}
		if run == 0 {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("run %d reported %q, run 0 reported %q; the reported binding depends on map iteration order",
				run, err.Error(), first)
		}
	}
	if !strings.Contains(first, `"alpha"`) {
		t.Fatalf("reported %q, want the lexicographically first unknown binding", first)
	}
}

// Two bindings failing for DIFFERENT reasons is the worse case: before the walk
// was ordered, the same input could come back with a different kind of failure,
// not merely a different name in the same sentence.
func TestValidateBindingsPicksTheSameFailureKindEveryRun(t *testing.T) {
	parents := []Parameter{{Name: "parent", Type: parameter.Text}}
	child := []Parameter{
		{Name: "alpha", Type: parameter.Text},
		{Name: "beta", Type: parameter.Text},
	}
	bindings := map[string]parameter.Binding{
		"alpha":   parameter.ParentReferenceBinding("absent"),
		"beta":    parameter.ParentReferenceBinding("absent"),
		"unknown": parameter.LiteralBinding(parameter.TextValue("x")),
	}

	first := ""
	for run := 0; run < determinismRuns; run++ {
		err := validateBindings(parents, child, bindings)
		if err == nil {
			t.Fatalf("run %d accepted a mix of invalid bindings", run)
		}
		if run == 0 {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("run %d reported %q, run 0 reported %q; both the binding and the failure kind move with iteration order",
				run, err.Error(), first)
		}
	}
}
