package evidence

import (
	"math"
	"testing"
)

func validGroupedStepTransitionCommit() StepTransitionCommit {
	return StepTransitionCommit{CommitID: "commit", ExpectedRevision: 1, Event: StepPhaseEvent{
		ID: "step", ExecutionID: "execution", WorkflowStepID: "workflow-step", DisplayName: "validate",
		Kind: "VALIDATION_GROUP", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 10,
	}, FinalValidations: []ValidationObservation{
		{ID: "member-a", RunID: "run", ExecutionID: "execution", StepExecutionID: "step", ValidationStepID: "validation-a", ElementTargetID: "node-a", ElementTargetVersionID: "node-a-v1", GroupID: "group", BranchID: "branch-a", AssertionKind: "selected_values", Expected: CollectionValidationValue([]string{"a, b", ""}), Actual: CollectionValidationValue([]string{"", "a, b"}), Passed: true, Reason: "passed", BranchDisposition: ValidationBranchWon, HealReviewStatus: "not_attempted", ObservedAt: 10, Final: true},
		{ID: "member-b", RunID: "run", ExecutionID: "execution", StepExecutionID: "step", ValidationStepID: "validation-b", ElementTargetID: "node-b", ElementTargetVersionID: "node-b-v1", GroupID: "group", BranchID: "branch-b", AssertionKind: "visible", Expected: ScalarValidationValue("true"), Actual: ScalarValidationValue("false"), Reason: "normal_unsatisfied", BranchDisposition: ValidationBranchNotSatisfied, HealReviewStatus: "not_attempted", ObservedAt: 10, Final: true},
	}, FinalValidationGroups: []ValidationGroupTerminalObservation{NewValidationGroupTerminalObservation(
		"group-final", "run", "execution", "step", "group", ValidationTerminalPassed, "branch-a",
		[]ValidationMemberIdentity{{BranchID: "branch-a", ElementTargetID: "node-a"}, {BranchID: "branch-b", ElementTargetID: "node-b"}}, 10,
	)}}
}

func TestStepTransitionCommitValidatesGroupTerminalFactsAndFinalMemberTopology(t *testing.T) {
	valid := validGroupedStepTransitionCommit()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid grouped commit: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*StepTransitionCommit)
	}{
		{"duplicate group", func(commit *StepTransitionCommit) {
			commit.FinalValidationGroups = append(commit.FinalValidationGroups, commit.FinalValidationGroups[0])
		}},
		{"duplicate member", func(commit *StepTransitionCommit) {
			commit.FinalValidations = append(commit.FinalValidations, commit.FinalValidations[0])
		}},
		{"duplicate member identity", func(commit *StepTransitionCommit) {
			member := commit.FinalValidations[1]
			member.ID = commit.FinalValidations[0].ID
			commit.FinalValidations[1] = member
		}},
		{"duplicate group identity", func(commit *StepTransitionCommit) {
			group := NewValidationGroupTerminalObservation(
				commit.FinalValidationGroups[0].ID, "run", "execution", "step", "other-group", ValidationTerminalPassed, "other-branch",
				[]ValidationMemberIdentity{{BranchID: "other-branch", ElementTargetID: "other-node"}}, 10,
			)
			commit.FinalValidationGroups = append(commit.FinalValidationGroups, group)
		}},
		{"missing member", func(commit *StepTransitionCommit) { commit.FinalValidations = commit.FinalValidations[:1] }},
		{"unexpected member", func(commit *StepTransitionCommit) { commit.FinalValidations[1].ElementTargetID = "other" }},
		{"cross group member", func(commit *StepTransitionCommit) { commit.FinalValidations[1].GroupID = "other" }},
		{"winner disposition mismatch", func(commit *StepTransitionCommit) {
			commit.FinalValidations[0].BranchDisposition = ValidationBranchNotSatisfied
		}},
		{"non winner marked won", func(commit *StepTransitionCommit) { commit.FinalValidations[1].BranchDisposition = ValidationBranchWon }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commit := valid
			commit.FinalValidations = append([]ValidationObservation(nil), valid.FinalValidations...)
			commit.FinalValidationGroups = append([]ValidationGroupTerminalObservation(nil), valid.FinalValidationGroups...)
			test.mutate(&commit)
			if err := commit.Validate(); err == nil {
				t.Fatal("invalid group terminal topology accepted")
			}
		})
	}
}

func TestStepTransitionCommitRejectsContradictoryGroupOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*StepTransitionCommit)
	}{
		{"passed member failed", func(commit *StepTransitionCommit) { commit.FinalValidations[0].Passed = false }},
		{"failed member satisfied", func(commit *StepTransitionCommit) {
			commit.FinalValidations[1].BranchDisposition = ValidationBranchSatisfiedNotWinner
		}},
		{"timeout on succeeded step", func(commit *StepTransitionCommit) {
			commit.FinalValidationGroups[0] = NewValidationGroupTerminalObservation(
				"group-final", "run", "execution", "step", "group", ValidationTerminalTimeout, "",
				[]ValidationMemberIdentity{{BranchID: "branch-a", ElementTargetID: "node-a"}, {BranchID: "branch-b", ElementTargetID: "node-b"}}, 10,
			)
		}},
		{"failed step with passed group", func(commit *StepTransitionCommit) { commit.Event.Phase = "FAILED" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commit := validGroupedStepTransitionCommit()
			test.mutate(&commit)
			if err := commit.Validate(); err == nil {
				t.Fatal("contradictory group outcome accepted")
			}
		})
	}
}

func TestStepTransitionCommitValidatesAtomicFactIdentity(t *testing.T) {
	valid := StepTransitionCommit{CommitID: "commit", ExpectedRevision: 1, Event: StepPhaseEvent{
		ID: "step", ExecutionID: "execution", WorkflowStepID: "workflow-step", DisplayName: "提交",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 10,
	}, FinalValidations: []ValidationObservation{{
		ID: "validation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step",
		ValidationStepID: "validation-step", ElementTargetID: "node", ElementTargetVersionID: "node-v1", AssertionKind: "visible",
		Expected: AbsentValidationValue(), Actual: AbsentValidationValue(),
		Reason: "passed", HealReviewStatus: "not_attempted", ObservedAt: 10, Final: true,
	}}, HealObservations: []HealObservation{{
		ID: "heal", RunID: "run", ExecutionID: "execution", StepExecutionID: "step", ElementTargetID: "node",
		BaseNodeVersionID: "node-v1", DecisionBand: DecisionUnknown, ObservedAt: 10,
	}},
		OriginalSelectorResets: []HealCandidateReset{{ExecutionID: "execution", StepExecutionID: "step", ElementTargetID: "node", BaseNodeVersionID: "node-v1", ObservedAt: 10}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid commit: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*StepTransitionCommit)
	}{
		{"missing commit id", func(command *StepTransitionCommit) { command.CommitID = "" }},
		{"zero expected revision", func(command *StepTransitionCommit) { command.ExpectedRevision = 0 }},
		{"non terminal", func(command *StepTransitionCommit) { command.Event.Phase = "RUNNING" }},
		{"missing event identity", func(command *StepTransitionCommit) { command.Event.ExecutionID = "" }},
		{"missing event display name", func(command *StepTransitionCommit) { command.Event.DisplayName = "" }},
		{"missing event kind", func(command *StepTransitionCommit) { command.Event.Kind = "" }},
		{"nonpositive event occurrence", func(command *StepTransitionCommit) { command.Event.Occurrence = 0 }},
		{"nonpositive event timestamp", func(command *StepTransitionCommit) { command.Event.Timestamp = 0 }},
		{"non final validation", func(command *StepTransitionCommit) { command.FinalValidations[0].Final = false }},
		{"cross step validation", func(command *StepTransitionCommit) { command.FinalValidations[0].StepExecutionID = "other" }},
		{"cross execution validation", func(command *StepTransitionCommit) { command.FinalValidations[0].ExecutionID = "other" }},
		{"validation missing identity", func(command *StepTransitionCommit) { command.FinalValidations[0].ID = "" }},
		{"validation missing observed time", func(command *StepTransitionCommit) { command.FinalValidations[0].ObservedAt = 0 }},
		{"validation NaN confidence", func(command *StepTransitionCommit) { command.FinalValidations[0].HealConfidence = math.NaN() }},
		{"validation positive infinite confidence", func(command *StepTransitionCommit) { command.FinalValidations[0].HealConfidence = math.Inf(1) }},
		{"validation negative infinite confidence", func(command *StepTransitionCommit) { command.FinalValidations[0].HealConfidence = math.Inf(-1) }},
		{"validation unsupported review status", func(command *StepTransitionCommit) { command.FinalValidations[0].HealReviewStatus = "unsupported" }},
		{"successful heal on failed step", func(command *StepTransitionCommit) {
			command.Event.Phase = "FAILED"
			command.HealObservations[0].Succeeded = true
			command.OriginalSelectorResets = nil
		}},
		{"reset on failed step", func(command *StepTransitionCommit) {
			command.Event.Phase = "FAILED"
			command.HealObservations = nil
		}},
		{"heal missing observed time", func(command *StepTransitionCommit) { command.HealObservations[0].ObservedAt = 0 }},
		{"invalid reset", func(command *StepTransitionCommit) { command.OriginalSelectorResets[0].ObservedAt = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := valid
			command.FinalValidations = append([]ValidationObservation(nil), valid.FinalValidations...)
			command.HealObservations = append([]HealObservation(nil), valid.HealObservations...)
			command.OriginalSelectorResets = append([]HealCandidateReset(nil), valid.OriginalSelectorResets...)
			test.mutate(&command)
			if err := command.Validate(); err == nil {
				t.Fatal("invalid atomic fact command was accepted")
			}
		})
	}
}

func TestStepTransitionCommitRejectsDuplicateHealAndResetIdentities(t *testing.T) {
	base := StepTransitionCommit{CommitID: "commit", ExpectedRevision: 1, Event: StepPhaseEvent{
		ID: "step", ExecutionID: "execution", WorkflowStepID: "workflow-step", DisplayName: "heal",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 10,
	}, HealObservations: []HealObservation{{
		ID: "heal", RunID: "run", ExecutionID: "execution", StepExecutionID: "step",
		ElementTargetID: "node", BaseNodeVersionID: "node-v1", DecisionBand: DecisionUnknown, ObservedAt: 10,
	}}, OriginalSelectorResets: []HealCandidateReset{{
		ExecutionID: "execution", StepExecutionID: "step", ElementTargetID: "node", BaseNodeVersionID: "node-v1", ObservedAt: 10,
	}}}
	for _, test := range []struct {
		name   string
		mutate func(*StepTransitionCommit)
	}{
		{"heal observation", func(commit *StepTransitionCommit) {
			commit.HealObservations = append(commit.HealObservations, commit.HealObservations[0])
		}},
		{"selector reset", func(commit *StepTransitionCommit) {
			commit.OriginalSelectorResets = append(commit.OriginalSelectorResets, commit.OriginalSelectorResets[0])
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			commit := base
			commit.HealObservations = append([]HealObservation(nil), base.HealObservations...)
			commit.OriginalSelectorResets = append([]HealCandidateReset(nil), base.OriginalSelectorResets...)
			test.mutate(&commit)
			if err := commit.Validate(); err == nil {
				t.Fatal("duplicate governance fact identity was accepted")
			}
			if len(base.HealObservations) != 1 || len(base.OriginalSelectorResets) != 1 {
				t.Fatal("validation mutated the source envelope")
			}
		})
	}
}
