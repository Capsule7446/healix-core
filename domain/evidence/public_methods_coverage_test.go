package evidence

import (
	"math"
	"strings"
	"testing"
)

func validValidationProgressObservation() ValidationProgressObservation {
	return ValidationProgressObservation{
		ID: "observation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step",
		ValidationStepID: "validation", NodeID: "node", NodeVersionID: "node-v1",
		AssertionKind: "visible", Expected: AbsentValidationValue(), Actual: AbsentValidationValue(),
		Reason: "passed", HealReviewStatus: "not_attempted", ObservedAt: 1,
	}
}

func TestValidationProgressObservationValidateRuleMatrix(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*ValidationProgressObservation)
		wantError string
	}{
		{name: "valid"},
		{name: "grouped progress", mutate: func(value *ValidationProgressObservation) { value.GroupID, value.BranchID = "group", "branch" }},
		{name: "auto applied", mutate: func(value *ValidationProgressObservation) {
			value.HealReviewStatus, value.HealConfidence = "auto_applied", 1
		}},
		{name: "review pending", mutate: func(value *ValidationProgressObservation) {
			value.HealReviewStatus, value.HealConfidence = "review_pending", 0.5
		}},
		{name: "no candidate", mutate: func(value *ValidationProgressObservation) { value.HealReviewStatus = "no_candidate" }},
		{name: "missing identity", mutate: func(value *ValidationProgressObservation) { value.ID = "" }, wantError: "requires identity and reason"},
		{name: "missing reason", mutate: func(value *ValidationProgressObservation) { value.Reason = "" }, wantError: "requires identity and reason"},
		{name: "timestamp lower boundary", mutate: func(value *ValidationProgressObservation) { value.ObservedAt = 0 }, wantError: "requires positive time"},
		{name: "invalid expected value", mutate: func(value *ValidationProgressObservation) { value.Expected = ValidationValue{} }, wantError: "validation expected value"},
		{name: "invalid actual value", mutate: func(value *ValidationProgressObservation) { value.Actual = ValidationValue{} }, wantError: "validation actual value"},
		{name: "group without branch", mutate: func(value *ValidationProgressObservation) { value.GroupID = "group" }, wantError: "present together"},
		{name: "branch without group", mutate: func(value *ValidationProgressObservation) { value.BranchID = "branch" }, wantError: "present together"},
		{name: "confidence below boundary", mutate: func(value *ValidationProgressObservation) { value.HealConfidence = -0.0001 }, wantError: "confidence"},
		{name: "confidence above boundary", mutate: func(value *ValidationProgressObservation) { value.HealConfidence = 1.0001 }, wantError: "confidence"},
		{name: "confidence NaN", mutate: func(value *ValidationProgressObservation) { value.HealConfidence = math.NaN() }, wantError: "confidence"},
		{name: "unsupported review status", mutate: func(value *ValidationProgressObservation) { value.HealReviewStatus = "UNKNOWN" }, wantError: "unsupported heal review status"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validValidationProgressObservation()
			if test.mutate != nil {
				test.mutate(&value)
			}
			err := value.Validate()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestHealObservationValidateBusinessBoundaryMatrix(t *testing.T) {
	valid := HealObservation{
		ID: "observation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step",
		NodeID: "node", BaseNodeVersionID: "node-v1", ObservedAt: 1,
		DecisionBand: DecisionUnknown,
	}
	tests := []struct {
		name      string
		mutate    func(*HealObservation)
		wantError string
	}{
		{name: "no candidate at lower confidence boundary"},
		{name: "applied candidate at upper confidence boundary", mutate: func(value *HealObservation) {
			value.CandidateHash, value.DecisionBand, value.Confidence = "candidate", DecisionApplied, 1
		}},
		{name: "below cap candidate", mutate: func(value *HealObservation) {
			value.CandidateHash, value.DecisionBand, value.Confidence = "candidate", DecisionBelowCap, 0.5
		}},
		{name: "missing identity", mutate: func(value *HealObservation) { value.NodeID = "" }, wantError: "requires identity"},
		{name: "timestamp below boundary", mutate: func(value *HealObservation) { value.ObservedAt = 0 }, wantError: "positive time"},
		{name: "confidence below boundary", mutate: func(value *HealObservation) { value.Confidence = -0.0001 }, wantError: "confidence"},
		{name: "confidence above boundary", mutate: func(value *HealObservation) { value.Confidence = 1.0001 }, wantError: "confidence"},
		{name: "confidence infinity", mutate: func(value *HealObservation) { value.Confidence = math.Inf(1) }, wantError: "confidence"},
		{name: "candidate with unknown band", mutate: func(value *HealObservation) { value.CandidateHash = "candidate" }, wantError: "requires APPLIED or BELOW_CAP"},
		{name: "no candidate with applied band", mutate: func(value *HealObservation) { value.DecisionBand = DecisionApplied }, wantError: "must use UNKNOWN"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			if test.mutate != nil {
				test.mutate(&value)
			}
			err := value.Validate()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestValidationValueEqualityKindsAndCollectionOwnership(t *testing.T) {
	tests := []struct {
		name  string
		left  ValidationValue
		right ValidationValue
		want  bool
	}{
		{name: "absent", left: AbsentValidationValue(), right: AbsentValidationValue(), want: true},
		{name: "redacted", left: RedactedValidationValue(), right: RedactedValidationValue(), want: true},
		{name: "same scalar", left: ScalarValidationValue("值"), right: ScalarValidationValue("值"), want: true},
		{name: "different scalar", left: ScalarValidationValue("a"), right: ScalarValidationValue("b")},
		{name: "different kind", left: AbsentValidationValue(), right: RedactedValidationValue()},
		{name: "same ordered collection", left: CollectionValidationValue([]string{"a", "b"}), right: CollectionValidationValue([]string{"a", "b"}), want: true},
		{name: "collection order differs", left: CollectionValidationValue([]string{"a", "b"}), right: CollectionValidationValue([]string{"b", "a"})},
		{name: "collection length differs", left: CollectionValidationValue([]string{"a"}), right: CollectionValidationValue([]string{"a", "b"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.left.Equal(test.right); got != test.want {
				t.Fatalf("Equal() = %v, want %v", got, test.want)
			}
		})
	}

	source := []string{"first", "second"}
	value := CollectionValidationValue(source)
	source[0] = "mutated input"
	first, ok := value.CollectionValue()
	if !ok || first[0] != "first" {
		t.Fatalf("CollectionValue() = %v, %v", first, ok)
	}
	first[0] = "mutated output"
	second, _ := value.CollectionValue()
	if second[0] != "first" {
		t.Fatalf("CollectionValue() aliases stored state: %v", second)
	}
	if got, ok := ScalarValidationValue("scalar").CollectionValue(); ok || got != nil {
		t.Fatalf("scalar CollectionValue() = %v, %v", got, ok)
	}
}

func TestValidationObservationFinalDispositionStateMatrix(t *testing.T) {
	valid := ValidationObservation{
		ID: "observation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step",
		ValidationStepID: "validation", NodeID: "node", NodeVersionID: "node-v1",
		AssertionKind: "visible", Expected: AbsentValidationValue(), Actual: AbsentValidationValue(),
		Reason: "passed", HealReviewStatus: "not_attempted", ObservedAt: 1,
	}
	tests := []struct {
		name        string
		grouped     bool
		final       bool
		disposition ValidationBranchDisposition
		wantError   bool
	}{
		{name: "standalone progress"},
		{name: "standalone final without branch disposition", final: true},
		{name: "grouped progress", grouped: true},
		{name: "winning branch", grouped: true, final: true, disposition: ValidationBranchWon},
		{name: "satisfied non-winner", grouped: true, final: true, disposition: ValidationBranchSatisfiedNotWinner},
		{name: "unsatisfied branch", grouped: true, final: true, disposition: ValidationBranchNotSatisfied},
		{name: "unobserved branch", grouped: true, final: true, disposition: ValidationBranchNotObserved},
		{name: "grouped final missing disposition", grouped: true, final: true, wantError: true},
		{name: "grouped progress with disposition", grouped: true, disposition: ValidationBranchWon, wantError: true},
		{name: "standalone final with disposition", final: true, disposition: ValidationBranchWon, wantError: true},
		{name: "unsupported disposition", grouped: true, final: true, disposition: "UNKNOWN", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			if test.grouped {
				value.GroupID, value.BranchID = "group", "branch"
			}
			value.Final = test.final
			value.BranchDisposition = test.disposition
			err := value.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

func TestStepFactTerminalPhaseAndBoundaryMatrix(t *testing.T) {
	valid := StepFact{ID: "fact", RunID: "run", ExecutionID: "execution", StepExecution: "step", Phase: PhaseSucceeded, ObservedAt: 1}
	tests := []struct {
		name      string
		mutate    func(*StepFact)
		wantError bool
	}{
		{name: "succeeded"},
		{name: "failed", mutate: func(value *StepFact) { value.Phase = PhaseFailed }},
		{name: "canceled", mutate: func(value *StepFact) { value.Phase = PhaseCanceled }},
		{name: "aborted", mutate: func(value *StepFact) { value.Phase = PhaseAborted }},
		{name: "missing identity", mutate: func(value *StepFact) { value.StepExecution = "" }, wantError: true},
		{name: "non-terminal phase", mutate: func(value *StepFact) { value.Phase = "RUNNING" }, wantError: true},
		{name: "timestamp below boundary", mutate: func(value *StepFact) { value.ObservedAt = 0 }, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			if test.mutate != nil {
				test.mutate(&value)
			}
			err := value.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

func TestStepTransitionCommitRejectsCombinedFactLimitAndCrossStepHeal(t *testing.T) {
	overLimit := StepTransitionCommit{CommitID: "commit", ExpectedRevision: 1, Event: StepPhaseEvent{
		ID: "step", ExecutionID: "execution", WorkflowStepID: "workflow-step", DisplayName: "Step",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 1,
	}}
	overLimit.FinalValidations = make([]ValidationObservation, maxStepTransitionFacts/2+1)
	overLimit.HealObservations = make([]HealObservation, maxStepTransitionFacts/2)
	if err := overLimit.Validate(); err == nil || !strings.Contains(err.Error(), "fact limit") {
		t.Fatalf("combined fact limit error = %v", err)
	}

	crossStep := StepTransitionCommit{CommitID: "commit", ExpectedRevision: 1, Event: StepPhaseEvent{
		ID: "step", ExecutionID: "execution", WorkflowStepID: "workflow-step", DisplayName: "Step",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 1,
	}, HealObservations: []HealObservation{{
		ID: "heal", RunID: "run", ExecutionID: "execution", StepExecutionID: "other-step",
		NodeID: "node", BaseNodeVersionID: "node-v1", DecisionBand: DecisionUnknown, ObservedAt: 1,
	}}}
	if err := crossStep.Validate(); err == nil || !strings.Contains(err.Error(), "committed step") {
		t.Fatalf("cross-step heal error = %v", err)
	}
}

func TestValidationGroupTerminalObservationRejectsIdentityAndMemberDuplicates(t *testing.T) {
	valid := NewValidationGroupTerminalObservation(
		"terminal", "run", "execution", "step", "group", ValidationTerminalPassed, "branch",
		[]ValidationMemberIdentity{{BranchID: "branch", NodeID: "node"}}, 1,
	)
	missingIdentity := valid
	missingIdentity.ID = ""
	if err := missingIdentity.Validate(); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("missing identity error = %v", err)
	}
	missingMemberIdentity := NewValidationGroupTerminalObservation(
		"terminal", "run", "execution", "step", "group", ValidationTerminalPassed, "branch",
		[]ValidationMemberIdentity{{BranchID: "branch"}}, 1,
	)
	if err := missingMemberIdentity.Validate(); err == nil || !strings.Contains(err.Error(), "member requires identity") {
		t.Fatalf("missing member identity error = %v", err)
	}
	duplicateMember := NewValidationGroupTerminalObservation(
		"terminal", "run", "execution", "step", "group", ValidationTerminalPassed, "branch",
		[]ValidationMemberIdentity{{BranchID: "branch", NodeID: "node"}, {BranchID: "branch", NodeID: "node"}}, 1,
	)
	if err := duplicateMember.Validate(); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate member error = %v", err)
	}
}
