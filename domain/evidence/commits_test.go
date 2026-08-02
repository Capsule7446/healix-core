package evidence

import (
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// joinLines renders a violation key list one per line so a mismatch reads as a diff.
func joinLines(keys []string) string { return strings.Join(keys, "\n") }

func validGroupedStepTransitionCommit() StepTransitionCommit {
	return StepTransitionCommit{CommitID: "commit", ExpectedRevision: 1, Event: StepPhaseEvent{
		ID: mustStepExecutionID("step"), EntryID: mustEntryID("execution"), FlowFragmentStepID: "workflow-step", DisplayName: "validate",
		Kind: "VALIDATION_GROUP", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 10,
	}, FinalValidations: []ValidationObservation{
		{ID: "member-a", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), Occurrence: 1, ValidationStepID: "validation-a", ElementTargetID: "node-a", ElementTargetVersionID: "node-a-v1", GroupID: "group", BranchID: "branch-a", AssertionKind: "selected_values", Expected: CollectionValidationValue([]string{"a, b", ""}), Actual: CollectionValidationValue([]string{"", "a, b"}), Passed: true, Reason: "passed", BranchDisposition: ValidationBranchWon, HealReviewStatus: "not_attempted", ObservedAt: 10, Final: true},
		{ID: "member-b", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), Occurrence: 1, ValidationStepID: "validation-b", ElementTargetID: "node-b", ElementTargetVersionID: "node-b-v1", GroupID: "group", BranchID: "branch-b", AssertionKind: "visible", Expected: ScalarValidationValue("true"), Actual: ScalarValidationValue("false"), Reason: "normal_unsatisfied", BranchDisposition: ValidationBranchNotSatisfied, HealReviewStatus: "not_attempted", ObservedAt: 10, Final: true},
	}, FinalValidationGroups: []ValidationGroupTerminalObservation{NewValidationGroupTerminalObservation(
		"group-final", mustInstanceID("run"), mustEntryID("execution"), mustStepExecutionID("step"), 1, "group", ValidationTerminalPassed, "branch-a",
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
				commit.FinalValidationGroups[0].ID, mustInstanceID("run"), mustEntryID("execution"), mustStepExecutionID("step"), 1, "other-group", ValidationTerminalPassed, "other-branch",
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
				"group-final", mustInstanceID("run"), mustEntryID("execution"), mustStepExecutionID("step"), 1, "group", ValidationTerminalTimeout, "",
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
		ID: mustStepExecutionID("step"), EntryID: mustEntryID("execution"), FlowFragmentStepID: "workflow-step", DisplayName: "提交",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 10,
	}, FinalValidations: []ValidationObservation{{
		ID: "validation", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), Occurrence: 1,
		ValidationStepID: "validation-step", ElementTargetID: "node", ElementTargetVersionID: "node-v1", AssertionKind: "visible",
		Expected: AbsentValidationValue(), Actual: AbsentValidationValue(),
		Reason: "passed", HealReviewStatus: "not_attempted", ObservedAt: 10, Final: true,
	}}, HealObservations: []HealObservation{{
		ID: "heal", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), Occurrence: 1, ElementTargetID: "node",
		BaseNodeVersionID: "node-v1", DecisionBand: DecisionUnknown, ObservedAt: 10,
	}},
		OriginalSelectorResets: []HealCandidateReset{{EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), Occurrence: 1, ElementTargetID: "node", BaseNodeVersionID: "node-v1", ObservedAt: 10}}}
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
		{"missing event identity", func(command *StepTransitionCommit) { command.Event.EntryID = execution.EntryID{} }},
		{"missing event display name", func(command *StepTransitionCommit) { command.Event.DisplayName = "" }},
		{"missing event kind", func(command *StepTransitionCommit) { command.Event.Kind = "" }},
		{"nonpositive event occurrence", func(command *StepTransitionCommit) { command.Event.Occurrence = 0 }},
		{"nonpositive event timestamp", func(command *StepTransitionCommit) { command.Event.Timestamp = 0 }},
		{"non final validation", func(command *StepTransitionCommit) { command.FinalValidations[0].Final = false }},
		{"cross step validation", func(command *StepTransitionCommit) {
			command.FinalValidations[0].StepExecutionID = mustStepExecutionID("other")
		}},
		{"cross execution validation", func(command *StepTransitionCommit) { command.FinalValidations[0].EntryID = mustEntryID("other") }},
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
		ID: mustStepExecutionID("step"), EntryID: mustEntryID("execution"), FlowFragmentStepID: "workflow-step", DisplayName: "heal",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 10,
	}, HealObservations: []HealObservation{{
		ID: "heal", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"),
		ElementTargetID: "node", BaseNodeVersionID: "node-v1", DecisionBand: DecisionUnknown, ObservedAt: 10,
	}}, OriginalSelectorResets: []HealCandidateReset{{
		EntryID: mustEntryID("execution"), StepExecutionID: mustStepExecutionID("step"), ElementTargetID: "node", BaseNodeVersionID: "node-v1", ObservedAt: 10,
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

// The topology check drains members as their groups consume them, then reports
// whatever is left over. Ranging over that leftover map made the reported error
// depend on Go's randomised map iteration order, so the same commit could be
// rejected with a different error on a different run. The property asserted here
// is stability, not a particular message: the error must be a function of the
// input alone.
func TestValidationGroupTopologyReportsLeftoverMembersDeterministically(t *testing.T) {
	template := validGroupedStepTransitionCommit().FinalValidations[1]

	grouped := template
	grouped.ID = "leftover-grouped"
	grouped.ValidationStepID = "validation-grouped"
	grouped.BranchID = "branch-extra"
	grouped.ElementTargetID = "node-extra"
	grouped.ElementTargetVersionID = "node-extra-v1"

	orphan := template
	orphan.ID = "leftover-orphan"
	orphan.ValidationStepID = "validation-orphan"
	orphan.GroupID = "orphan-group"
	orphan.BranchID = "branch-orphan"
	orphan.ElementTargetID = "node-orphan"
	orphan.ElementTargetVersionID = "node-orphan-v1"

	tests := []struct {
		name  string
		extra []ValidationObservation
	}{
		{name: "grouped leftover first", extra: []ValidationObservation{grouped, orphan}},
		{name: "orphan leftover first", extra: []ValidationObservation{orphan, grouped}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commit := validGroupedStepTransitionCommit()
			commit.FinalValidations = append(commit.FinalValidations, test.extra...)
			first := commit.Validate()
			if first == nil {
				t.Fatal("leftover grouped validations accepted")
			}
			for attempt := 1; attempt < 200; attempt++ {
				again := commit.Validate()
				if again == nil || again.Error() != first.Error() {
					t.Fatalf("attempt %d: error = %v, want the same error as the first run %v", attempt, again, first)
				}
			}
		})
	}
}

// brokenStepTransitionCommit breaks several independent rules at once so the whole
// violation order is observable in one envelope.
func brokenStepTransitionCommit() StepTransitionCommit {
	commit := validGroupedStepTransitionCommit()
	commit.CommitID = " "
	commit.ExpectedRevision = 0
	commit.Event.Phase = "RUNNING"
	commit.Event.Occurrence = 0
	commit.Event.Timestamp = 0
	return commit
}

func TestStepTransitionCommitOrdersViolationsDeterministically(t *testing.T) {
	want := []string{
		violationKey(fault.CodeFieldRequired, "commitId"),
		violationKey(fault.CodeFieldInvalid, "expectedRevision"),
		violationKey(fault.CodeFieldInvalid, "event.phase"),
		violationKey(fault.CodeFieldInvalid, "event.occurrence"),
		violationKey(fault.CodeFieldInvalid, "event.timestamp"),
	}
	descriptor := requireEnvelope(t, brokenStepTransitionCommit().Validate(), CodeStepTransitionCommitInvalid)
	got := violationKeys(descriptor.Violations())
	if len(got) < len(want) {
		t.Fatalf("violations =\n%s\nwant at least\n%s", joinLines(got), joinLines(want))
	}
	// The scalar commit and event violations come first, in declaration order,
	// before any collection walk contributes.
	if joinLines(got[:len(want)]) != joinLines(want) {
		t.Fatalf("leading violations =\n%s\nwant\n%s", joinLines(got[:len(want)]), joinLines(want))
	}
	// A later map iteration slipping into a walk would show up as instability.
	for attempt := 0; attempt < 100; attempt++ {
		repeat := requireEnvelope(t, brokenStepTransitionCommit().Validate(), CodeStepTransitionCommitInvalid)
		if keys := violationKeys(repeat.Violations()); joinLines(keys) != joinLines(got) {
			t.Fatalf("violation order is unstable on attempt %d:\n%s", attempt, joinLines(keys))
		}
	}
}

func TestStepTransitionCommitTruncatesViolationsAtCap(t *testing.T) {
	commit := validGroupedStepTransitionCommit()
	template := HealObservation{
		ID: "heal", InstanceID: mustInstanceID("run"), EntryID: mustEntryID("other-execution"), StepExecutionID: mustStepExecutionID("other-step"),
		ElementTargetID: "node", BaseNodeVersionID: "node-v1", DecisionBand: DecisionApplied, ObservedAt: 0,
	}
	for index := 0; index < 40; index++ {
		commit.HealObservations = append(commit.HealObservations, template)
	}

	first := requireEnvelope(t, commit.Validate(), CodeStepTransitionCommitInvalid)
	if len(first.Violations()) != fault.MaxViolations {
		t.Fatalf("violations = %d, want the cap %d", len(first.Violations()), fault.MaxViolations)
	}
	second := requireEnvelope(t, commit.Validate(), CodeStepTransitionCommitInvalid)
	if joinLines(violationKeys(first.Violations())) != joinLines(violationKeys(second.Violations())) {
		t.Fatal("truncated violation prefix is not deterministic")
	}
}
