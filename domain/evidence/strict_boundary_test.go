package evidence

import (
	"math"
	"reflect"
	"testing"
)

func TestExportedValidatorsStrictBoundaries(t *testing.T) {
	for _, confidence := range []float64{-1, math.NaN(), math.Inf(-1), math.Inf(1), 1.0001} {
		if ValidateConfidence(confidence) == nil {
			t.Errorf("confidence %v accepted", confidence)
		}
	}
	for _, confidence := range []float64{0, 1} {
		if err := ValidateConfidence(confidence); err != nil {
			t.Errorf("confidence %v rejected: %v", confidence, err)
		}
	}
	for _, tt := range []struct {
		hash  string
		band  DecisionBand
		valid bool
	}{
		{"", DecisionUnknown, true}, {" \t\n", DecisionUnknown, true}, {"哈希", DecisionApplied, true},
		{"", DecisionApplied, false}, {"hash", DecisionUnknown, false}, {"hash", DecisionBand("UNKNOWN_ENUM"), false},
	} {
		if (ValidateDecisionBand(tt.hash, tt.band) == nil) != tt.valid {
			t.Errorf("ValidateDecisionBand(%q,%q) validity mismatch", tt.hash, tt.band)
		}
	}
	for _, phase := range []Phase{"", "UNKNOWN", PhaseSucceeded, PhaseFailed, PhaseCanceled, PhaseAborted} {
		want := phase == PhaseSucceeded || phase == PhaseFailed || phase == PhaseCanceled || phase == PhaseAborted
		if phase.IsTerminal() != want {
			t.Errorf("Phase(%q).IsTerminal() mismatch", phase)
		}
	}
}

func TestValidationValuesNilEmptyDuplicatesAndImmutability(t *testing.T) {
	for _, values := range [][]string{nil, {}, {""}, {"重复", "重复"}, {"\n", "你好"}} {
		original := append([]string(nil), values...)
		value := CollectionValidationValue(values)
		if err := value.Validate(); err != nil {
			t.Fatalf("CollectionValidationValue(%v): %v", values, err)
		}
		if len(values) > 0 {
			values[0] = "mutated"
		}
		got, ok := value.CollectionValue()
		if !ok || len(got) != len(original) || (len(got) > 0 && !reflect.DeepEqual(got, original)) {
			t.Fatalf("got %v, want %v", got, original)
		}
		if len(got) > 0 {
			got[0] = "mutated-again"
			again, _ := value.CollectionValue()
			if !reflect.DeepEqual(again, original) {
				t.Fatal("accessor exposed internal slice")
			}
		}
	}
	if (ValidationValue{}).Validate() == nil {
		t.Fatal("zero enum accepted")
	}
	if (ValidationValue{Kind: ValidationValueKind("unknown")}).Validate() == nil {
		t.Fatal("unknown enum accepted")
	}
}

func TestValidationGroupExpectedMembersImmutable(t *testing.T) {
	members := []ValidationMemberIdentity{{BranchID: "分支", ElementTargetID: "节点"}}
	value := NewValidationGroupTerminalObservation("id", mustInstanceID("run"), mustEntryID("execution"), mustStepExecutionID("step"), "group", ValidationTerminalPassed, "分支", members, 1)
	members[0].ElementTargetID = "mutated"
	got := value.ExpectedMembers()
	if got[0].ElementTargetID != "节点" {
		t.Fatal("constructor aliased input")
	}
	got[0].ElementTargetID = "mutated-again"
	if value.ExpectedMembers()[0].ElementTargetID != "节点" {
		t.Fatal("accessor exposed internal slice")
	}
}

func TestStepFactRejectsWhitespaceIdentity(t *testing.T) {
	fact := StepFact{ID: " ", InstanceID: mustInstanceID("run"), ExecutionID: mustEntryID("execution"), StepExecution: mustStepExecutionID("step"), Phase: PhaseSucceeded, ObservedAt: 1}
	if err := fact.Validate(); err == nil {
		t.Fatal("whitespace-only identity accepted")
	}
}
