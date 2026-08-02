package scheduling

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// The command validator ranges the entry map and returns on its first offender,
// and its two branches return different KINDS of failure: a malformed item id is
// an uncoded error, a bad parameter value carries the parameter package's own
// code. The outer envelope makes both CodeCreateInstanceCommandInvalid, so
// CodeOf and the rendered message never moved and no existing test could see
// this.
//
// fault.IsCode is errors.Is, and it walks the whole chain. So with one offender
// of each kind present, IsCode(err, parameter.CodeValueInvalid) answered
// differently on different runs for byte-identical input — a host that branches
// on a deeper code routes the same request two ways. The fault package
// documents that exact hazard.
//
// One call can never show this; it always picks one of the two.
const commandDeterminismRuns = 200

func TestCreateInstanceCommandRejectionCarriesTheSameChainEveryRun(t *testing.T) {
	build := func() CreateInstanceCommand {
		command := validCreateInstanceCommand()
		// "  bad id  " is malformed; "zzz-item" holds a value that fails its own
		// validation. Sorted, the malformed id is visited first every time.
		command.Entries = map[string]map[string]parameter.Value{
			"  bad id  ": {"region": parameter.TextValue("north")},
			"zzz-item":   {"region": parameter.Value{}},
		}
		return command
	}

	first := validateCreateInstanceCommand(build())
	if first == nil {
		t.Fatal("a command carrying a malformed item id and an invalid value was accepted")
	}
	firstNested := fault.IsCode(first, parameter.CodeValueInvalid)

	for run := 0; run < commandDeterminismRuns; run++ {
		err := validateCreateInstanceCommand(build())
		if err == nil {
			t.Fatalf("run %d accepted the command", run)
		}
		if !fault.IsCode(err, CodeCreateInstanceCommandInvalid) {
			t.Fatalf("run %d lost the envelope code: %v", run, err)
		}
		if nested := fault.IsCode(err, parameter.CodeValueInvalid); nested != firstNested {
			t.Fatalf("run %d reports IsCode(PARAMETER_VALUE_INVALID)=%v, run 0 reported %v; "+
				"which offender is found depends on map iteration order",
				run, nested, firstNested)
		}
	}

	// Sorted order makes the choice predictable, not merely stable: the malformed
	// id sorts first, so the uncoded branch must win and the parameter code must
	// never appear in the chain.
	if firstNested {
		t.Fatal("the parameter code reached the chain; the lexicographically first offender is the malformed item id")
	}
}
