package evidence

import "testing"

func TestHealObservationUsesEvidenceOwnedDecisionBand(t *testing.T) {
	observation := HealObservation{ID: "observation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step", NodeID: "node", BaseNodeVersionID: "version", Confidence: 0.8, DecisionBand: DecisionUnknown, ObservedAt: 1}
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	observation.CandidateHash = "candidate"
	if err := observation.Validate(); err == nil {
		t.Fatal("expected candidate decision band rejection")
	}
}

func TestValidationObservationRejectsUnknownReviewStatus(t *testing.T) {
	observation := ValidationObservation{ID: "validation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step", ValidationStepID: "validation-step", NodeID: "node", NodeVersionID: "version", AssertionKind: "visible", Reason: "final", HealReviewStatus: "unknown", ObservedAt: 1}
	if err := observation.Validate(); err == nil {
		t.Fatal("expected review status rejection")
	}
}
