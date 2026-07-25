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

func TestValidationValueDistinguishesAbsentScalarCollectionAndRedacted(t *testing.T) {
	values := []ValidationValue{
		AbsentValidationValue(),
		ScalarValidationValue(""),
		CollectionValidationValue([]string{}),
		RedactedValidationValue(),
	}
	for _, value := range values {
		if err := value.Validate(); err != nil {
			t.Fatalf("value %#v: %v", value, err)
		}
	}
	for left := range values {
		for right := left + 1; right < len(values); right++ {
			if values[left].Equal(values[right]) {
				t.Fatalf("values %#v and %#v are indistinguishable", values[left], values[right])
			}
		}
	}
	input := []string{"a, b", "", "a, b", "\x1f"}
	value := CollectionValidationValue(input)
	input[0] = "mutated"
	got, ok := value.CollectionValue()
	if !ok || got[0] != "a, b" {
		t.Fatalf("collection aliases constructor input: %#v", got)
	}
	got[0] = "mutated"
	again, _ := value.CollectionValue()
	if again[0] != "a, b" {
		t.Fatalf("collection aliases accessor output: %#v", again)
	}
}

func TestValidationValueRejectsInvalidKindFieldCombinations(t *testing.T) {
	tests := []ValidationValue{
		{},
		{Kind: ValidationValueKind("unknown")},
		{Kind: ValidationValueAbsent, Scalar: "x"},
		{Kind: ValidationValueScalar, Scalar: "x", collection: []string{}},
		{Kind: ValidationValueCollection, Scalar: "x", collection: []string{"y"}},
		{Kind: ValidationValueRedacted, collection: []string{}},
	}
	for _, value := range tests {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid value accepted: %#v", value)
		}
	}
}

func TestValidationGroupTerminalObservationRequiresConsistentWinnerAndReason(t *testing.T) {
	base := NewValidationGroupTerminalObservation(
		"terminal", "run", "execution", "step", "group", ValidationTerminalPassed, "branch",
		[]ValidationMemberIdentity{{BranchID: "branch", NodeID: "node"}}, 1,
	)
	if err := base.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	tests := []ValidationGroupTerminalObservation{
		func() ValidationGroupTerminalObservation { value := base; value.WinningBranchID = ""; return value }(),
		func() ValidationGroupTerminalObservation {
			value := base
			value.TerminalReason = ValidationTerminalTimeout
			return value
		}(),
		func() ValidationGroupTerminalObservation {
			value := base
			value.TerminalReason = ValidationTerminalReason("unknown")
			value.WinningBranchID = ""
			return value
		}(),
	}
	for _, observation := range tests {
		if err := observation.Validate(); err == nil {
			t.Fatalf("invalid terminal accepted: %#v", observation)
		}
	}
	for _, reason := range []ValidationTerminalReason{ValidationTerminalTimeout, ValidationTerminalCanceled, ValidationTerminalSystemError} {
		observation := base
		observation.TerminalReason = reason
		observation.WinningBranchID = ""
		if err := observation.Validate(); err != nil {
			t.Fatalf("reason %s: %v", reason, err)
		}
	}
	missingWinner := base
	missingWinner.WinningBranchID = "other"
	if err := missingWinner.Validate(); err == nil {
		t.Fatal("passed group accepted a winner outside expected members")
	}
	empty := NewValidationGroupTerminalObservation("terminal", "run", "execution", "step", "group", ValidationTerminalPassed, "branch", nil, 1)
	if err := empty.Validate(); err == nil {
		t.Fatal("group without expected members was accepted")
	}
}

func TestValidationObservationRejectsUnknownReviewStatus(t *testing.T) {
	observation := ValidationObservation{ID: "validation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step", ValidationStepID: "validation-step", NodeID: "node", NodeVersionID: "version", AssertionKind: "visible", Expected: AbsentValidationValue(), Actual: AbsentValidationValue(), Reason: "final", HealReviewStatus: "unknown", ObservedAt: 1}
	if err := observation.Validate(); err == nil {
		t.Fatal("expected review status rejection")
	}
}
