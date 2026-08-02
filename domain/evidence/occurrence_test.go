package evidence

import "testing"

// TestEveryValidatingCoordinateCarrierRejectsNonPositiveOccurrence pins the rule
// that Occurrence carries. Two of the three coordinate components are value
// types that cannot hold a meaningless value; this one is a bare int, so the
// only thing standing between a zero and the evidence store is each carrier's
// Validate.
//
// StepPhaseEvent and HealCandidateReset are absent on purpose: neither declares
// a Validate, because neither reaches a store on its own. StepTransitionCommit
// owns them and checks their occurrence against the event's, which
// TestStepTransitionCommitValidatesAtomicFactIdentity already covers.
func TestEveryValidatingCoordinateCarrierRejectsNonPositiveOccurrence(t *testing.T) {
	carriers := []struct {
		name     string
		validate func(occurrence int) error
	}{
		{"StepProgressEvent", func(occurrence int) error {
			event := validProgressEvent(ProgressRunning)
			event.Occurrence = occurrence
			return event.Validate()
		}},
		{"StepFact", func(occurrence int) error {
			fact := StepFact{ID: "fact", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"),
				StepExecutionID: mustStepExecutionID("step"), Occurrence: occurrence, Phase: PhaseSucceeded, ObservedAt: 1}
			return fact.Validate()
		}},
		{"HealObservation", func(occurrence int) error {
			observation := HealObservation{ID: "observation", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"),
				StepExecutionID: mustStepExecutionID("step"), Occurrence: occurrence, ElementTargetID: "node",
				BaseNodeVersionID: "node-v1", DecisionBand: DecisionUnknown, ObservedAt: 1}
			return observation.Validate()
		}},
		{"ValidationObservation", func(occurrence int) error {
			observation := ValidationObservation{ID: "observation", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"),
				StepExecutionID: mustStepExecutionID("step"), Occurrence: occurrence,
				ValidationStepID: "validation", ElementTargetID: "node", ElementTargetVersionID: "node-v1",
				AssertionKind: "visible", Expected: AbsentValidationValue(), Actual: AbsentValidationValue(),
				Reason: "passed", HealReviewStatus: "not_attempted", ObservedAt: 1}
			return observation.Validate()
		}},
		{"ValidationProgressObservation", func(occurrence int) error {
			observation := validValidationProgressObservation()
			observation.Occurrence = occurrence
			return observation.Validate()
		}},
		{"ValidationGroupTerminalObservation", func(occurrence int) error {
			return NewValidationGroupTerminalObservation(
				"terminal", mustInstanceID("run"), mustEntryID("execution"), mustStepExecutionID("step"), occurrence,
				"group", ValidationTerminalPassed, "branch",
				[]ValidationMemberIdentity{{BranchID: "branch", ElementTargetID: "node"}}, 1,
			).Validate()
		}},
	}
	for _, carrier := range carriers {
		t.Run(carrier.name, func(t *testing.T) {
			if err := carrier.validate(1); err != nil {
				t.Fatalf("the first occurrence was rejected: %v", err)
			}
			for _, occurrence := range []int{0, -1} {
				if err := carrier.validate(occurrence); err == nil {
					t.Errorf("occurrence %d was accepted", occurrence)
				}
			}
		})
	}
}
