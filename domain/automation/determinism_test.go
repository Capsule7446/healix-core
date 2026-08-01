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
	// Three names carrying a control character, which parameter.ValidateName
	// rejects. Which one is reported is decided purely by iteration order unless
	// the walk is ordered.
	// Built from a rune rather than written inline: a literal control byte in
	// the source is invisible, so nobody notices when an edit drops it and the
	// fixture quietly stops being invalid.
	control := string(rune(1))
	variables := EnvironmentVariables{
		"zulu" + control:  parameter.TextValue("z"),
		"alpha" + control: parameter.TextValue("a"),
		"mike" + control:  parameter.TextValue("m"),
	}

	first := ""
	for run := 0; run < determinismRuns; run++ {
		err := variables.Validate()
		if err == nil {
			t.Fatalf("run %d accepted three malformed variable names", run)
		}
		if run == 0 {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("run %d reported %q, run 0 reported %q; the reported variable depends on map iteration order",
				run, err.Error(), first)
		}
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
