package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/parameter"
)

// Go randomises map iteration, so a validator that ranges a map and returns on
// the first offending entry names a different field on a different run for the
// same input. Repeating the call is the only assertion that catches it: one
// call always passes, whichever entry it happened to pick.
const determinismRuns = 200

func TestEnvironmentVariablesReportTheSameBadNameEveryRun(t *testing.T) {
	// Built from a rune rather than written inline: a literal control byte in
	// the source is invisible, so nobody notices when an edit drops it and the
	// fixture quietly stops being invalid.
	control := string(rune(1))
	// One bad NAME and one bad VALUE, deliberately. Three bad names would not
	// prove anything: parameter.ValidateName's error carries no parameter, and
	// the wrapper adds none, so three of them produce byte-identical strings and
	// the stability loop compares a constant with itself. That is what the first
	// version of this test did, and deleting the sort it guards left it green.
	//
	// A bad name and a bad value take different branches with different
	// prefixes, so which one is reported is observable — which is the only way
	// the ordering can be asserted at all.
	variables := EnvironmentVariables{
		"alpha" + control: parameter.TextValue("fine"),
		"zulu":            parameter.Value{},
	}

	first := ""
	for run := 0; run < determinismRuns; run++ {
		err := variables.Validate()
		if err == nil {
			t.Fatalf("run %d accepted a malformed name and a malformed value", run)
		}
		if run == 0 {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("run %d reported %q, run 0 reported %q; which failure is reported depends on map iteration order",
				run, err.Error(), first)
		}
	}
	// Sorted order makes the choice predictable rather than merely stable:
	// "alpha" sorts before "zulu", so the name branch must win every time.
	if !strings.HasPrefix(first, "environment variable name:") {
		t.Fatalf("reported %q, want the lexicographically first offender, which is the bad name", first)
	}
}

// Environment.Validate folds the variable failure into one fixed violation, so
// the public contract never moved. This pins that it stays that way: the
// nondeterminism was only ever in the private cause, and the fix must not have
// promoted a variable name into public text.
func TestEnvironmentValidateKeepsVariableNamesOutOfPublicText(t *testing.T) {
	environment := Environment{
		ID: "env-1", DisplayName: "Environment", BaseURL: "https://example.test",
		Variables: EnvironmentVariables{"bad" + string(rune(1)) + "name": parameter.TextValue("x")},
		Revision:  1,
	}
	err := environment.Validate()
	if err == nil {
		t.Fatal("a malformed variable name was accepted")
	}
	if strings.Contains(err.Error(), "bad") {
		t.Fatalf("public text names the offending variable: %q", err.Error())
	}
}
