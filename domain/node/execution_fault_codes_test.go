package node

import (
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// TestClassifyStepPhaseTransitionInvalidClassifiesOnlyUncodedFailures verifies
// the boundary-classification contract: nil stays nil, an already-classified
// fault (as produced by rt.emit's Facts-port failure path) passes through
// unchanged instead of being buried under a second code, and an uncoded
// failure is wrapped with the phase-transition code while the original detail
// survives only on the private cause.
func TestClassifyStepPhaseTransitionInvalidClassifiesOnlyUncodedFailures(t *testing.T) {
	if got := classifyStepPhaseTransitionInvalid(nil); got != nil {
		t.Fatalf("classifyStepPhaseTransitionInvalid(nil) = %v, want nil", got)
	}

	alreadyClassified := evidenceRecordFailedError(errors.New("facts unavailable"))
	passedThrough := classifyStepPhaseTransitionInvalid(alreadyClassified)
	if passedThrough != alreadyClassified {
		t.Fatalf("classifyStepPhaseTransitionInvalid() replaced an already-classified fault: got %v, want %v", passedThrough, alreadyClassified)
	}
	if !fault.IsCode(passedThrough, CodeEvidenceRecordFailed) {
		t.Fatal("pass-through fault lost its original code")
	}

	uncoded := errors.New("node step-execution without-active-occurrence detail")
	classified := classifyStepPhaseTransitionInvalid(uncoded)
	if !fault.IsCode(classified, CodeStepPhaseTransitionInvalid) {
		t.Fatalf("classified = %v, want code %s", classified, CodeStepPhaseTransitionInvalid)
	}
	descriptor, ok := fault.Describe(classified)
	if !ok || descriptor.Kind() != fault.FailedPrecondition || descriptor.Message() != "step phase transition is invalid" {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	if descriptor.Message() == uncoded.Error() {
		t.Fatal("public message leaked the private detail")
	}
	if cause := errors.Unwrap(classified); cause != uncoded {
		t.Fatalf("private cause = %v, want %v", cause, uncoded)
	}
}

func TestHealingRefusedErrorContract(t *testing.T) {
	cause := errors.New("healing refused: block: origin_mismatch")
	err := healingRefusedError(cause)
	if !fault.IsCode(err, CodeHealingRefused) {
		t.Fatalf("error = %v, want code %s", err, CodeHealingRefused)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.FailedPrecondition || descriptor.Message() != "healing was refused" {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	if got := errors.Unwrap(err); got != cause {
		t.Fatalf("private cause = %v, want %v", got, cause)
	}
}

func TestEvidenceRecordFailedErrorContract(t *testing.T) {
	cause := errors.New("facts port unavailable")
	err := evidenceRecordFailedError(cause)
	if !fault.IsCode(err, CodeEvidenceRecordFailed) {
		t.Fatalf("error = %v, want code %s", err, CodeEvidenceRecordFailed)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Kind() != fault.Unavailable || descriptor.Message() != "execution evidence could not be recorded" {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	if got := errors.Unwrap(err); got != cause {
		t.Fatalf("private cause = %v, want %v", got, cause)
	}
}

// TestStepConfigurationInvalidErrorCarriesViolationWithoutLeakingCause proves
// the envelope shape used across every EXECUTION_STEP_CONFIGURATION_INVALID
// site: the violation field/code are public, and — for the wrapping
// constructor — the rejected value stays reachable only through Unwrap.
func TestStepConfigurationInvalidErrorCarriesViolationWithoutLeakingCause(t *testing.T) {
	violation := mustViolation(fault.CodeFieldInvalid, "action.kind", "action kind is not supported")

	direct := stepConfigurationInvalidError(violation)
	if !fault.IsCode(direct, CodeStepConfigurationInvalid) {
		t.Fatalf("direct = %v, want code %s", direct, CodeStepConfigurationInvalid)
	}
	descriptor, ok := fault.Describe(direct)
	if !ok || descriptor.Kind() != fault.InvalidArgument || descriptor.Message() != "step configuration is invalid" {
		t.Fatalf("descriptor = %#v (ok=%v)", descriptor, ok)
	}
	if violations := descriptor.Violations(); len(violations) != 1 || violations[0].Field() != "action.kind" {
		t.Fatalf("violations = %#v", violations)
	}

	distinctiveDetail := errors.New("rejected action kind was double_click_and_hold_extended_sentinel")
	wrapped := wrapStepConfigurationInvalidError(distinctiveDetail, violation)
	if wrapDescriptor, ok := fault.Describe(wrapped); !ok || wrapDescriptor.Message() != "step configuration is invalid" {
		t.Fatalf("wrapped descriptor = %#v (ok=%v)", wrapDescriptor, ok)
	}
	if cause := errors.Unwrap(wrapped); cause != distinctiveDetail {
		t.Fatalf("private cause = %v, want %v", cause, distinctiveDetail)
	}
}
