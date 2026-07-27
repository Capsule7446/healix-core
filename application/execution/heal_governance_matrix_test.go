package execution

import (
	"reflect"
	"strings"
	"testing"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
)

func TestDefaultHealGovernancePlannerRejectsEveryPlanIdentityBoundary(t *testing.T) {
	valid := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	tests := []struct {
		name   string
		mutate func(*HealGovernancePlan)
		want   string
	}{
		{name: "blank node", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.Key.NodeID = " \t\n" }, want: "requires node and base identity"},
		{name: "blank base version", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.Key.BaseNodeVersionID = " \t\n" }, want: "requires node and base identity"},
		{name: "blank current version", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.CurrentNodeVersionID = " \t\n" }, want: "requires current node version identity"},
		{name: "zero revision", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.Revision = 0 }, want: "snapshot revision"},
		{name: "missing fact id", mutate: func(plan *HealGovernancePlan) { plan.Fact.FactID = "" }, want: "requires fact, commit, run, and sequence identity"},
		{name: "missing commit id", mutate: func(plan *HealGovernancePlan) { plan.Fact.CommitID = "" }, want: "requires fact, commit, run, and sequence identity"},
		{name: "missing run id", mutate: func(plan *HealGovernancePlan) { plan.Fact.RunID = "" }, want: "requires fact, commit, run, and sequence identity"},
		{name: "zero sequence", mutate: func(plan *HealGovernancePlan) { plan.Fact.Sequence = 0 }, want: "requires fact, commit, run, and sequence identity"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := valid
			observation := *valid.Fact.Observation
			plan.Fact.Observation = &observation
			test.mutate(&plan)
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			if err == nil || !strings.Contains(err.Error(), test.want) || !reflect.DeepEqual(decision, HealGovernanceDecision{}) {
				t.Fatalf("PlanHealGovernance() = (%#v, %v), want %q", decision, err, test.want)
			}
		})
	}
}

func TestDefaultHealGovernancePlannerRejectsEveryAcceptedFactShapeMismatch(t *testing.T) {
	valid := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	tests := []struct {
		name   string
		mutate func(*HealGovernancePlan)
		want   string
	}{
		{name: "unknown fact kind", mutate: func(plan *HealGovernancePlan) { plan.Fact.Kind = "UNKNOWN" }, want: "unsupported accepted heal fact kind"},
		{name: "observation missing payload", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation = nil }, want: "exactly one observation payload"},
		{name: "observation has reset payload", mutate: func(plan *HealGovernancePlan) { plan.Fact.Reset = validResetPayload() }, want: "exactly one observation payload"},
		{name: "observation fact mismatch", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.ID = "other" }, want: "does not match governance identity"},
		{name: "observation run mismatch", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.RunID = "other" }, want: "does not match governance identity"},
		{name: "observation node mismatch", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.NodeID = "other" }, want: "does not match governance identity"},
		{name: "observation base mismatch", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.BaseNodeVersionID = "other" }, want: "does not match governance identity"},
		{name: "unsupported decision band", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.DecisionBand = "INVALID" }, want: "unsupported evidence decision band"},
		{name: "reset missing payload", mutate: func(plan *HealGovernancePlan) { plan.Fact.Kind, plan.Fact.Observation = HealAcceptedReset, nil }, want: "exactly one reset payload"},
		{name: "reset has observation payload", mutate: func(plan *HealGovernancePlan) {
			plan.Fact.Kind, plan.Fact.Reset = HealAcceptedReset, validResetPayload()
		}, want: "exactly one reset payload"},
		{name: "reset node mismatch", mutate: func(plan *HealGovernancePlan) {
			plan.Fact.Kind, plan.Fact.Observation, plan.Fact.Reset = HealAcceptedReset, nil, validResetPayload()
			plan.Fact.Reset.NodeID = "other"
		}, want: "does not match governance identity"},
		{name: "reset base mismatch", mutate: func(plan *HealGovernancePlan) {
			plan.Fact.Kind, plan.Fact.Observation, plan.Fact.Reset = HealAcceptedReset, nil, validResetPayload()
			plan.Fact.Reset.BaseNodeVersionID = "other"
		}, want: "does not match governance identity"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := valid
			observation := *valid.Fact.Observation
			plan.Fact.Observation = &observation
			test.mutate(&plan)
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			if err == nil || !strings.Contains(err.Error(), test.want) || !reflect.DeepEqual(decision, HealGovernanceDecision{}) {
				t.Fatalf("PlanHealGovernance() = (%#v, %v), want %q", decision, err, test.want)
			}
		})
	}
}

func validResetPayload() *evidence.HealCandidateReset {
	return &evidence.HealCandidateReset{
		ExecutionID: "execution-reset", StepExecutionID: "step-reset", NodeID: "node", BaseNodeVersionID: "base", ObservedAt: 1,
	}
}

func TestDefaultHealGovernancePlannerAcceptsEveryEvidenceDecisionBandAndOutcome(t *testing.T) {
	tests := []struct {
		name      string
		band      evidence.DecisionBand
		succeeded bool
		wantBand  domainautomation.HealDecisionBand
		wantState domainautomation.HealStreakDisposition
	}{
		{name: "unknown failed", band: evidence.DecisionUnknown},
		{name: "unknown succeeded", band: evidence.DecisionUnknown, succeeded: true},
		{name: "applied succeeded", band: evidence.DecisionApplied, succeeded: true, wantBand: domainautomation.HealDecisionBandApplied, wantState: domainautomation.HealStreakObserving},
		{name: "below cap succeeded", band: evidence.DecisionBelowCap, succeeded: true, wantBand: domainautomation.HealDecisionBandBelowCap, wantState: domainautomation.HealStreakObserving},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := healGovernancePlan("run-1", 1, test.band, domainautomation.HealStreak{})
			plan.Fact.Observation.Succeeded = test.succeeded
			if test.band == evidence.DecisionUnknown {
				plan.Fact.Observation.CandidateHash = ""
				plan.Fact.Observation.Confidence = 0
			}
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			if err != nil {
				t.Fatal(err)
			}
			if decision.NextStreak.Band != test.wantBand || decision.NextStreak.Disposition != test.wantState || decision.Effect != nil {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
}

func TestDefaultHealGovernancePlannerAcceptsFailedAppliedDecisionBands(t *testing.T) {
	for _, band := range []evidence.DecisionBand{evidence.DecisionApplied, evidence.DecisionBelowCap} {
		t.Run(string(band), func(t *testing.T) {
			plan := healGovernancePlan("run-failed", 1, band, domainautomation.HealStreak{})
			plan.Fact.Observation.Succeeded = false
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			if err != nil {
				t.Fatalf("valid failed evidence was rejected: %v", err)
			}
			if decision.NextStreak.Disposition != "" || decision.NextStreak.Band != "" || decision.Effect != nil || decision.NextStreak.LastSequence != 1 {
				t.Fatalf("failed evidence decision = %#v", decision)
			}
		})
	}
}

func TestDefaultHealGovernancePlannerCandidateStatusMatchesEveryStreakState(t *testing.T) {
	planner := NewDefaultHealGovernancePlanner()
	observingDecision, err := planner.PlanHealGovernance(healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{}))
	if err != nil {
		t.Fatal(err)
	}
	auto, _ := matureHealStreak(t, evidence.DecisionApplied)
	awaiting, _ := matureHealStreak(t, evidence.DecisionBelowCap)
	rejectedDecision, err := awaiting.Reject(awaiting.LastSequence + 1)
	if err != nil {
		t.Fatal(err)
	}
	stalePlan := healGovernancePlan("run-stale", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	stalePlan.Snapshot.CurrentNodeVersionID = "new-current"
	staleDecision, err := planner.PlanHealGovernance(stalePlan)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		status domainautomation.HealCandidateStatus
		streak domainautomation.HealStreak
	}{
		{name: "observing", status: domainautomation.HealCandidateObserving, streak: observingDecision.NextStreak},
		{name: "awaiting approval", status: domainautomation.HealCandidateAwaitingApproval, streak: awaiting},
		{name: "promoted", status: domainautomation.HealCandidatePromoted, streak: auto},
		{name: "rejected", status: domainautomation.HealCandidateRejected, streak: rejectedDecision.Next},
		{name: "stale", status: domainautomation.HealCandidateStale, streak: staleDecision.NextStreak},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := healGovernancePlan("next-run-"+string(rune('a'+index)), test.streak.LastSequence+1, evidence.DecisionApplied, test.streak)
			plan.Snapshot.CandidateStatus = test.status
			if _, err := planner.PlanHealGovernance(plan); err != nil {
				t.Fatalf("matching status rejected: %v", err)
			}
		})
	}

	for _, status := range []domainautomation.HealCandidateStatus{
		domainautomation.HealCandidateObserving,
		domainautomation.HealCandidateAwaitingApproval,
		domainautomation.HealCandidatePromoted,
		domainautomation.HealCandidateRejected,
		domainautomation.HealCandidateStale,
		"UNKNOWN",
	} {
		t.Run("mismatch "+string(status), func(t *testing.T) {
			plan := healGovernancePlan("mismatch", observingDecision.NextStreak.LastSequence+1, evidence.DecisionApplied, observingDecision.NextStreak)
			plan.Snapshot.CandidateStatus = status
			if status == domainautomation.HealCandidateObserving {
				plan.Snapshot.Streak = domainautomation.HealStreak{}
			}
			if _, err := planner.PlanHealGovernance(plan); err == nil || !strings.Contains(err.Error(), "conflicts with streak disposition") {
				t.Fatalf("mismatched status error = %v", err)
			}
		})
	}
}

func TestDefaultHealGovernancePlannerValidatesEveryTerminalEffectKind(t *testing.T) {
	planner := NewDefaultHealGovernancePlanner()
	autoStreak, autoDecision := matureHealStreak(t, evidence.DecisionApplied)
	awaitStreak, awaitDecision := matureHealStreak(t, evidence.DecisionBelowCap)

	first, err := planner.PlanHealGovernance(healGovernancePlan("reset-source", 1, evidence.DecisionApplied, domainautomation.HealStreak{}))
	if err != nil {
		t.Fatal(err)
	}
	reset := validResetPayload()
	reset.ObservedAt = 2
	resetPlan := HealGovernancePlan{
		Snapshot: HealGovernanceSnapshot{Key: HealGovernanceKey{NodeID: "node", BaseNodeVersionID: "base"}, CurrentNodeVersionID: "base", Revision: 2, Streak: first.NextStreak},
		Fact:     HealAcceptedFact{Kind: HealAcceptedReset, FactID: "reset", CommitID: "reset-commit", RunID: "reset-run", Sequence: 2, Reset: reset},
	}
	resetDecision, err := planner.PlanHealGovernance(resetPlan)
	if err != nil {
		t.Fatal(err)
	}

	stalePlan := healGovernancePlan("stale-run", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	stalePlan.Snapshot.CurrentNodeVersionID = "new-current"
	staleDecision, err := planner.PlanHealGovernance(stalePlan)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		streak   domainautomation.HealStreak
		terminal *HealTerminalEffectIntent
	}{
		{name: "auto publish", streak: autoStreak, terminal: autoDecision.Effect},
		{name: "await approval", streak: awaitStreak, terminal: awaitDecision.Effect},
		{name: "reset", streak: resetDecision.NextStreak, terminal: resetDecision.Effect},
		{name: "stale", streak: staleDecision.NextStreak, terminal: staleDecision.Effect},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := healGovernancePlan("effect-next-"+string(rune('a'+index)), test.streak.LastSequence+1, evidence.DecisionApplied, test.streak)
			plan.Snapshot.ExistingTerminalEffect = &HealTerminalEffectSnapshot{
				Kind: test.terminal.Kind, CandidateHash: test.terminal.CandidateHash, Band: test.terminal.Band,
				Contributions: append([]HealContributionSnapshot(nil), test.terminal.Contributions...),
			}
			if _, err := planner.PlanHealGovernance(plan); err != nil {
				t.Fatalf("matching terminal effect rejected: %v", err)
			}
		})
	}
}

func TestDefaultHealGovernancePlannerRejectsEveryTerminalEffectConflict(t *testing.T) {
	planner := NewDefaultHealGovernancePlanner()
	streak, terminal := matureHealStreak(t, evidence.DecisionApplied)
	base := healGovernancePlan("effect-next", streak.LastSequence+1, evidence.DecisionApplied, streak)
	base.Snapshot.ExistingTerminalEffect = &HealTerminalEffectSnapshot{
		Kind: terminal.Effect.Kind, CandidateHash: terminal.Effect.CandidateHash, Band: terminal.Effect.Band,
		Contributions: append([]HealContributionSnapshot(nil), terminal.Effect.Contributions...),
	}
	tests := []struct {
		name   string
		mutate func(*HealGovernancePlan)
		want   string
	}{
		{name: "nonterminal streak", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.Streak = domainautomation.HealStreak{} }, want: "requires a terminal streak"},
		{name: "wrong kind", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.ExistingTerminalEffect.Kind = HealEffectReset }, want: "conflicts with terminal streak"},
		{name: "wrong candidate", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.ExistingTerminalEffect.CandidateHash = "other" }, want: "conflicts with terminal streak"},
		{name: "wrong band", mutate: func(plan *HealGovernancePlan) {
			plan.Snapshot.ExistingTerminalEffect.Band = domainautomation.HealDecisionBandBelowCap
		}, want: "conflicts with terminal streak"},
		{name: "missing contribution", mutate: func(plan *HealGovernancePlan) {
			plan.Snapshot.ExistingTerminalEffect.Contributions = plan.Snapshot.ExistingTerminalEffect.Contributions[:2]
		}, want: "conflicts with terminal streak"},
		{name: "changed contribution", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.ExistingTerminalEffect.Contributions[0].FactID = "other" }, want: "conflicts with terminal streak"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := base
			effect := *base.Snapshot.ExistingTerminalEffect
			effect.Contributions = append([]HealContributionSnapshot(nil), effect.Contributions...)
			plan.Snapshot.ExistingTerminalEffect = &effect
			test.mutate(&plan)
			if _, err := planner.PlanHealGovernance(plan); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("terminal effect error = %v, want %q", err, test.want)
			}
		})
	}
}
