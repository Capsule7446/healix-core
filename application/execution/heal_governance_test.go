package execution

import (
	"math"
	"reflect"
	"strings"
	"testing"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
)

func healGovernancePlan(runID string, sequence uint64, band evidence.DecisionBand, streak domainautomation.HealStreak) HealGovernancePlan {
	observation := evidence.HealObservation{
		ID: "fact-" + runID, RunID: runID, ExecutionID: "execution-" + runID, StepExecutionID: "step-" + runID,
		NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Confidence: 0.9,
		DecisionBand: band, Succeeded: true, ObservedAt: int64(sequence),
	}
	return HealGovernancePlan{
		Snapshot: HealGovernanceSnapshot{
			Key:                  HealGovernanceKey{NodeID: "node", BaseNodeVersionID: "base"},
			CurrentNodeVersionID: "base", Revision: 1, Streak: streak,
		},
		Fact: HealAcceptedFact{
			Kind: HealAcceptedObservation, FactID: observation.ID, CommitID: "commit-" + runID,
			RunID: runID, Sequence: sequence, Observation: &observation,
		},
	}
}

func TestDefaultHealGovernancePlannerValidatesExportedBoundaryInputs(t *testing.T) {
	valid := healGovernancePlan("run", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	for _, test := range []struct {
		name   string
		mutate func(*HealGovernancePlan)
	}{
		{"zero payloads", func(plan *HealGovernancePlan) { plan.Fact.Observation = nil }},
		{"multiple payloads", func(plan *HealGovernancePlan) { plan.Fact.Reset = &evidence.HealCandidateReset{} }},
		{"unsupported fact kind", func(plan *HealGovernancePlan) { plan.Fact.Kind = "OTHER" }},
		{"unsupported decision band", func(plan *HealGovernancePlan) { plan.Fact.Observation.DecisionBand = "OTHER" }},
		{"unsupported candidate status", func(plan *HealGovernancePlan) { plan.Snapshot.CandidateStatus = "OTHER" }},
		{"whitespace node", func(plan *HealGovernancePlan) { plan.Snapshot.Key.NodeID = " \t" }},
		{"whitespace base", func(plan *HealGovernancePlan) { plan.Snapshot.Key.BaseNodeVersionID = " \n" }},
		{"whitespace current", func(plan *HealGovernancePlan) { plan.Snapshot.CurrentNodeVersionID = "  " }},
		{"whitespace fact", func(plan *HealGovernancePlan) { plan.Fact.FactID = "  " }},
		{"whitespace commit", func(plan *HealGovernancePlan) { plan.Fact.CommitID = "  " }},
		{"whitespace run", func(plan *HealGovernancePlan) { plan.Fact.RunID = "  " }},
		{"zero revision", func(plan *HealGovernancePlan) { plan.Snapshot.Revision = 0 }},
		{"zero sequence", func(plan *HealGovernancePlan) { plan.Fact.Sequence = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := valid
			observation := *valid.Fact.Observation
			plan.Fact.Observation = &observation
			test.mutate(&plan)
			if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDefaultHealGovernancePlannerAcceptsRevisionAndSequenceUpperBoundary(t *testing.T) {
	plan := healGovernancePlan("run", math.MaxUint64, evidence.DecisionApplied, domainautomation.HealStreak{})
	plan.Snapshot.Revision = domainautomation.Revision(math.MaxUint64)
	decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
	if err != nil || decision.Sequence != math.MaxUint64 || decision.ExpectedRevision != domainautomation.Revision(math.MaxUint64) {
		t.Fatalf("PlanHealGovernance() = (%#v, %v)", decision, err)
	}
	if strings.TrimSpace(decision.FactID) == "" {
		t.Fatal("decision lost fact identity")
	}
}

func TestDefaultHealGovernancePlannerBindsAcceptedFactWithoutMutation(t *testing.T) {
	plan := healGovernancePlan("run-1", 11, evidence.DecisionApplied, domainautomation.HealStreak{})
	decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Key != plan.Snapshot.Key || decision.FactID != plan.Fact.FactID || decision.Sequence != plan.Fact.Sequence || decision.ExpectedRevision != plan.Snapshot.Revision {
		t.Fatalf("decision authority binding = %#v", decision)
	}
	if decision.Effect != nil || len(decision.NextStreak.Contributions) != 1 {
		t.Fatalf("first observation decision = %#v", decision)
	}
	plan.Fact.Observation.RunID = "mutated"
	if decision.NextStreak.Contributions[0].RunID != "run-1" {
		t.Fatal("decision aliases evidence input")
	}
}

func TestDefaultHealGovernancePlannerEmitsBandSpecificThirdRunEffect(t *testing.T) {
	for _, test := range []struct {
		name string
		band evidence.DecisionBand
		want HealTerminalEffectKind
	}{
		{"applied", evidence.DecisionApplied, HealEffectAutoPublish},
		{"below cap", evidence.DecisionBelowCap, HealEffectAwaitApproval},
	} {
		t.Run(test.name, func(t *testing.T) {
			planner := NewDefaultHealGovernancePlanner()
			streak := domainautomation.HealStreak{}
			var decision HealGovernanceDecision
			for index, sequence := range []uint64{11, 19, 27} {
				plan := healGovernancePlan("run-"+string(rune('1'+index)), sequence, test.band, streak)
				var err error
				decision, err = planner.PlanHealGovernance(plan)
				if err != nil {
					t.Fatal(err)
				}
				streak = decision.NextStreak
				if index < 2 && decision.Effect != nil {
					t.Fatalf("premature effect = %#v", decision.Effect)
				}
			}
			if decision.Effect == nil || decision.Effect.Kind != test.want || len(decision.Effect.Contributions) != 3 {
				t.Fatalf("third-run effect = %#v", decision.Effect)
			}
			wantSequences := []uint64{11, 19, 27}
			for index, contribution := range decision.Effect.Contributions {
				if contribution.Sequence != wantSequences[index] {
					t.Fatalf("effect contributions = %#v", decision.Effect.Contributions)
				}
			}
		})
	}
}

func TestDefaultHealGovernancePlannerResetIsScopedAndImmutable(t *testing.T) {
	streak := domainautomation.HealStreak{}
	first := healGovernancePlan("run-1", 1, evidence.DecisionApplied, streak)
	decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(first)
	if err != nil {
		t.Fatal(err)
	}
	reset := evidence.HealCandidateReset{ExecutionID: "execution-reset", StepExecutionID: "step-reset", NodeID: "node", BaseNodeVersionID: "base", ObservedAt: 2}
	plan := HealGovernancePlan{
		Snapshot: HealGovernanceSnapshot{Key: first.Snapshot.Key, CurrentNodeVersionID: "base", Revision: 2, Streak: decision.NextStreak},
		Fact:     HealAcceptedFact{Kind: HealAcceptedReset, FactID: "reset", CommitID: "reset-commit", RunID: "run-2", Sequence: 2, Reset: &reset},
	}
	result, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.Effect == nil || result.Effect.Kind != HealEffectReset || result.NextStreak.Disposition != domainautomation.HealStreakReset {
		t.Fatalf("reset decision = %#v", result)
	}
	other := plan
	other.Snapshot.Key.NodeID = "other"
	if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(other); err == nil {
		t.Fatal("cross-node reset was accepted")
	}
	if !reflect.DeepEqual(plan.Snapshot.Streak.Contributions, decision.NextStreak.Contributions) {
		t.Fatal("planner mutated the snapshot streak")
	}
}

func matureHealStreak(t *testing.T, band evidence.DecisionBand) (domainautomation.HealStreak, HealGovernanceDecision) {
	t.Helper()
	planner := NewDefaultHealGovernancePlanner()
	streak := domainautomation.HealStreak{}
	var decision HealGovernanceDecision
	for index, sequence := range []uint64{11, 19, 27} {
		plan := healGovernancePlan("run-"+string(rune('1'+index)), sequence, band, streak)
		var err error
		decision, err = planner.PlanHealGovernance(plan)
		if err != nil {
			t.Fatal(err)
		}
		streak = decision.NextStreak
	}
	return streak, decision
}

func TestDefaultHealGovernancePlannerFourthRunDoesNotRepeatTerminalEffect(t *testing.T) {
	streak, _ := matureHealStreak(t, evidence.DecisionApplied)
	plan := healGovernancePlan("run-4", 35, evidence.DecisionApplied, streak)
	decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Effect != nil || decision.NextStreak.LastSequence != 35 || decision.NextStreak.Disposition != streak.Disposition || !reflect.DeepEqual(decision.NextStreak.Contributions, streak.Contributions) {
		t.Fatalf("fourth-run decision = %#v", decision)
	}
}

func TestDefaultHealGovernancePlannerValidatesExistingTerminalEffectProvenance(t *testing.T) {
	streak, terminal := matureHealStreak(t, evidence.DecisionApplied)
	plan := healGovernancePlan("run-4", 35, evidence.DecisionApplied, streak)
	plan.Snapshot.ExistingTerminalEffect = &HealTerminalEffectSnapshot{
		Kind:          HealEffectAutoPublish,
		CandidateHash: streak.CandidateHash,
		Band:          streak.Band,
		Contributions: append([]HealContributionSnapshot(nil), terminal.Effect.Contributions...),
		VersionID:     "version-1",
	}
	plan.Snapshot.ExistingTerminalEffect.Contributions[0].RunID = "different-run"
	if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan); err == nil {
		t.Fatal("terminal effect with conflicting provenance was accepted")
	}
}

func TestDefaultHealGovernancePlannerExistingPromotionPrecedesStaleBase(t *testing.T) {
	streak, terminal := matureHealStreak(t, evidence.DecisionApplied)
	plan := healGovernancePlan("run-4", 35, evidence.DecisionApplied, streak)
	plan.Snapshot.CurrentNodeVersionID = "promoted-version"
	plan.Snapshot.ExistingTerminalEffect = &HealTerminalEffectSnapshot{
		Kind:          HealEffectAutoPublish,
		CandidateHash: streak.CandidateHash,
		Band:          streak.Band,
		Contributions: append([]HealContributionSnapshot(nil), terminal.Effect.Contributions...),
		VersionID:     "promoted-version",
	}
	decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Effect != nil || decision.NextStreak.Disposition != domainautomation.HealStreakAutoPublish {
		t.Fatalf("existing promotion decision = %#v", decision)
	}
}

func TestDefaultHealGovernancePlannerRejectedStatusRequiresRejectedStreak(t *testing.T) {
	streak, _ := matureHealStreak(t, evidence.DecisionBelowCap)
	plan := healGovernancePlan("run-4", 35, evidence.DecisionBelowCap, streak)
	plan.Snapshot.CandidateStatus = domainautomation.HealCandidateRejected
	if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan); err == nil {
		t.Fatal("rejected candidate with active await-approval streak was accepted")
	}
}

func TestDefaultHealGovernancePlannerRejectsMalformedContributionSnapshot(t *testing.T) {
	streak, _ := matureHealStreak(t, evidence.DecisionApplied)
	streak.Contributions[1].CommitID = streak.Contributions[0].CommitID
	plan := healGovernancePlan("run-4", 35, evidence.DecisionApplied, streak)
	if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan); err == nil {
		t.Fatal("streak with conflicting contribution provenance was accepted")
	}
}

func TestDefaultHealGovernancePlannerRejectsStreakOutsideGovernanceKey(t *testing.T) {
	for _, field := range []string{"node", "base"} {
		t.Run(field, func(t *testing.T) {
			plan := healGovernancePlan("run-2", 2, evidence.DecisionApplied, domainautomation.HealStreak{
				NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: domainautomation.HealDecisionBandApplied,
				Contributions: []domainautomation.ContributingHealFact{{FactID: "fact-run-1", CommitID: "commit-run-1", RunID: "run-1", ExecutionID: "execution-run-1", StepExecutionID: "step-run-1", Sequence: 1}},
				LastSequence:  1, Observing: true, Disposition: domainautomation.HealStreakObserving,
			})
			if field == "node" {
				plan.Snapshot.Streak.NodeID = "other-node"
			} else {
				plan.Snapshot.Streak.BaseNodeVersionID = "other-base"
			}
			if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan); err == nil {
				t.Fatalf("streak with mismatched %s was accepted", field)
			}
		})
	}
}

func TestDefaultHealGovernancePlannerRequiresCurrentNodeVersion(t *testing.T) {
	plan := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	plan.Snapshot.CurrentNodeVersionID = ""
	if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan); err == nil {
		t.Fatal("snapshot without current node version was accepted")
	}
}
