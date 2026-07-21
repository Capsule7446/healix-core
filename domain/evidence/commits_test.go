package evidence

import (
	"math"
	"testing"
)

func TestStepTransitionCommitValidatesAtomicFactIdentity(t *testing.T) {
	valid := StepTransitionCommit{CommitID: "commit", Event: StepPhaseEvent{
		ID: "step", ExecutionID: "execution", WorkflowStepID: "workflow-step", DisplayName: "提交",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 10,
	}, FinalValidations: []ValidationObservation{{
		ID: "validation", RunID: "run", ExecutionID: "execution", StepExecutionID: "step",
		ValidationStepID: "validation-step", NodeID: "node", NodeVersionID: "node-v1", AssertionKind: "visible",
		Reason: "passed", HealReviewStatus: "not_attempted", ObservedAt: 10, Final: true,
	}}, HealObservations: []HealObservationCommit{{Observation: HealObservation{
		ID: "heal", RunID: "run", ExecutionID: "execution", StepExecutionID: "step", NodeID: "node",
		BaseNodeVersionID: "node-v1", DecisionBand: DecisionUnknown, ObservedAt: 10,
	}, PromotedVersionID: "node-v2"}},
		OriginalSelectorResets: []HealCandidateReset{{NodeID: "node", BaseNodeVersionID: "node-v1", ObservedAt: 10}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid commit: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*StepTransitionCommit)
	}{
		{"missing commit id", func(command *StepTransitionCommit) { command.CommitID = "" }},
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
		{"heal missing observed time", func(command *StepTransitionCommit) { command.HealObservations[0].Observation.ObservedAt = 0 }},
		{"invalid reset", func(command *StepTransitionCommit) { command.OriginalSelectorResets[0].ObservedAt = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := valid
			command.FinalValidations = append([]ValidationObservation(nil), valid.FinalValidations...)
			command.HealObservations = append([]HealObservationCommit(nil), valid.HealObservations...)
			command.OriginalSelectorResets = append([]HealCandidateReset(nil), valid.OriginalSelectorResets...)
			test.mutate(&command)
			if err := command.Validate(); err == nil {
				t.Fatal("invalid atomic fact command was accepted")
			}
		})
	}
}
