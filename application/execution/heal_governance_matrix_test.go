package execution

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestDefaultHealGovernancePlannerRejectsEveryPlanIdentityBoundary(t *testing.T) {
	valid := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	tests := []struct {
		name   string
		mutate func(*HealGovernancePlan)
		want   string
	}{
		{name: "blank node", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.Key.ElementTargetID = " \t\n" }, want: "requires node and base identity"},
		{name: "blank base version", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.Key.BaseNodeVersionID = " \t\n" }, want: "requires node and base identity"},
		{name: "blank current version", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.CurrentNodeVersionID = " \t\n" }, want: "requires current node version identity"},
		{name: "zero revision", mutate: func(plan *HealGovernancePlan) { plan.Snapshot.Revision = 0 }, want: "snapshot revision"},
		{name: "missing fact id", mutate: func(plan *HealGovernancePlan) { plan.Fact.FactID = "" }, want: "requires fact, commit, run, and sequence identity"},
		{name: "missing commit id", mutate: func(plan *HealGovernancePlan) { plan.Fact.CommitID = "" }, want: "requires fact, commit, run, and sequence identity"},
		{name: "missing run id", mutate: func(plan *HealGovernancePlan) { plan.Fact.InstanceID = domainexecution.InstanceID{} }, want: "requires fact, commit, run, and sequence identity"},
		{name: "zero sequence", mutate: func(plan *HealGovernancePlan) { plan.Fact.Sequence = 0 }, want: "requires fact, commit, run, and sequence identity"},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := valid
			observation := *valid.Fact.Observation
			plan.Fact.Observation = &observation
			test.mutate(&plan)
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			if index < 4 {
				assertHealGovernanceFault(t, decision, err, CodeHealGovernanceSnapshotInvalid, fault.FailedPrecondition, "heal governance snapshot is invalid")
				return
			}
			assertHealGovernanceFault(t, decision, err, CodeHealAcceptedFactInvalid, fault.InvalidArgument, "accepted heal fact is invalid")
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
		{name: "observation run mismatch", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.InstanceID = mustInstanceID("other") }, want: "does not match governance identity"},
		{name: "observation node mismatch", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.ElementTargetID = "other" }, want: "does not match governance identity"},
		{name: "observation base mismatch", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.BaseNodeVersionID = "other" }, want: "does not match governance identity"},
		{name: "unsupported decision band", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.DecisionBand = "INVALID" }, want: "unsupported evidence decision band"},
		{name: "reset missing payload", mutate: func(plan *HealGovernancePlan) { plan.Fact.Kind, plan.Fact.Observation = HealAcceptedReset, nil }, want: "exactly one reset payload"},
		{name: "reset has observation payload", mutate: func(plan *HealGovernancePlan) {
			plan.Fact.Kind, plan.Fact.Reset = HealAcceptedReset, validResetPayload()
		}, want: "exactly one reset payload"},
		{name: "reset node mismatch", mutate: func(plan *HealGovernancePlan) {
			plan.Fact.Kind, plan.Fact.Observation, plan.Fact.Reset = HealAcceptedReset, nil, validResetPayload()
			plan.Fact.Reset.ElementTargetID = "other"
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
			assertHealGovernanceFault(t, decision, err, CodeHealAcceptedFactInvalid, fault.InvalidArgument, "accepted heal fact is invalid")
		})
	}
}

func assertHealGovernanceFault(t *testing.T, decision HealGovernanceDecision, err error, code fault.Code, kind fault.Kind, message string) {
	t.Helper()
	if !reflect.DeepEqual(decision, HealGovernanceDecision{}) {
		t.Fatalf("decision = %#v, want zero", decision)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Code() != code || descriptor.Kind() != kind || descriptor.Message() != message {
		t.Fatalf("descriptor = %#v, ok = %v, error = %v", descriptor, ok, err)
	}
	if len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 || err.Error() != string(code)+": "+message {
		t.Fatalf("unsafe public contract: %#v, error = %v", descriptor, err)
	}
}

func TestDefaultHealGovernancePlannerExposesSafeFaultFamilies(t *testing.T) {
	planner := NewDefaultHealGovernancePlanner()

	snapshotPlan := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	snapshotPlan.Snapshot.CurrentNodeVersionID = "secret\ncurrent"
	decision, err := planner.PlanHealGovernance(snapshotPlan)
	assertHealGovernanceFault(t, decision, err, CodeHealGovernanceSnapshotInvalid, fault.FailedPrecondition, "heal governance snapshot is invalid")

	factPlan := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
	factPlan.Fact.Kind = HealAcceptedFactKind("secret\nkind")
	decision, err = planner.PlanHealGovernance(factPlan)
	assertHealGovernanceFault(t, decision, err, CodeHealAcceptedFactInvalid, fault.InvalidArgument, "accepted heal fact is invalid")

	streak, terminal := matureHealStreak(t, evidence.DecisionApplied)
	effectPlan := healGovernancePlan("run-next", streak.LastSequence+1, evidence.DecisionApplied, streak)
	effectPlan.Snapshot.ExistingTerminalEffect = &HealTerminalEffectSnapshot{Kind: HealEffectReset, CandidateHash: terminal.Effect.CandidateHash, Band: terminal.Effect.Band, Contributions: terminal.Effect.Contributions}
	decision, err = planner.PlanHealGovernance(effectPlan)
	assertHealGovernanceFault(t, decision, err, CodeHealTerminalEffectConflict, fault.Conflict, "heal terminal effect conflicts with persisted state")
}

func TestDefaultHealGovernancePlannerPrioritizesPersistedStateFaults(t *testing.T) {
	planner := NewDefaultHealGovernancePlanner()
	streak, terminal := matureHealStreak(t, evidence.DecisionApplied)

	streakPlan := healGovernancePlan("run-next", streak.LastSequence+1, evidence.DecisionApplied, streak)
	streakPlan.Snapshot.Key.ElementTargetID = "other-node"
	streakPlan.Fact.FactID = ""
	decision, err := planner.PlanHealGovernance(streakPlan)
	assertHealGovernanceFault(t, decision, err, CodeHealGovernanceSnapshotInvalid, fault.FailedPrecondition, "heal governance snapshot is invalid")

	effectPlan := healGovernancePlan("run-next", streak.LastSequence+1, evidence.DecisionApplied, streak)
	effectPlan.Snapshot.ExistingTerminalEffect = &HealTerminalEffectSnapshot{Kind: HealEffectReset, CandidateHash: terminal.Effect.CandidateHash, Band: terminal.Effect.Band, Contributions: terminal.Effect.Contributions}
	effectPlan.Fact.FactID = ""
	decision, err = planner.PlanHealGovernance(effectPlan)
	assertHealGovernanceFault(t, decision, err, CodeHealTerminalEffectConflict, fault.Conflict, "heal terminal effect conflicts with persisted state")
}

func TestDefaultHealGovernancePlannerRejectsMalformedPersistedStreakIdentities(t *testing.T) {
	streak, _ := matureHealStreak(t, evidence.DecisionApplied)
	tests := []struct {
		name   string
		mutate func(*domainautomation.HealStreak)
	}{
		{name: "node", mutate: func(value *domainautomation.HealStreak) { value.ElementTargetID = "secret\x00node" }},
		{name: "base", mutate: func(value *domainautomation.HealStreak) { value.BaseNodeVersionID = " base " }},
		{name: "candidate", mutate: func(value *domainautomation.HealStreak) { value.CandidateHash = "secret‮candidate" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := healGovernancePlan("run-next", streak.LastSequence+1, evidence.DecisionApplied, streak)
			plan.Fact.FactID = ""
			test.mutate(&plan.Snapshot.Streak)
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			assertHealGovernanceFault(t, decision, err, CodeHealGovernanceSnapshotInvalid, fault.FailedPrecondition, "heal governance snapshot is invalid")
		})
	}
}

func TestDefaultHealGovernancePlannerRejectsMalformedPersistedProvenance(t *testing.T) {
	streak, _ := matureHealStreak(t, evidence.DecisionApplied)
	tests := []struct {
		name   string
		mutate func(*domainautomation.ContributingHealFact)
	}{
		{name: "fact", mutate: func(value *domainautomation.ContributingHealFact) { value.FactID = "secret\x00fact" }},
		{name: "commit", mutate: func(value *domainautomation.ContributingHealFact) { value.CommitID = " secret-commit " }},
		{name: "run", mutate: func(value *domainautomation.ContributingHealFact) { value.InstanceID = "secret‮run" }},
		{name: "execution", mutate: func(value *domainautomation.ContributingHealFact) { value.ExecutionID = string([]byte{0xff}) }},
		{name: "step", mutate: func(value *domainautomation.ContributingHealFact) {
			value.StepExecutionID = strings.Repeat("x", parameter.MaxNameBytes+1)
		}},
	}
	for _, collection := range []string{"contributions", "consumed"} {
		for _, test := range tests {
			t.Run(collection+" "+test.name, func(t *testing.T) {
				plan := healGovernancePlan("run-next", streak.LastSequence+1, evidence.DecisionApplied, streak)
				plan.Fact.FactID = ""
				if collection == "contributions" {
					test.mutate(&plan.Snapshot.Streak.Contributions[0])
				} else {
					test.mutate(&plan.Snapshot.Streak.ConsumedObservations[0])
				}
				decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
				assertHealGovernanceFault(t, decision, err, CodeHealGovernanceSnapshotInvalid, fault.FailedPrecondition, "heal governance snapshot is invalid")
			})
		}
	}
}

func TestDefaultHealGovernancePlannerRejectsMalformedTerminalEffectIdentities(t *testing.T) {
	streak, terminal := matureHealStreak(t, evidence.DecisionApplied)
	tests := []struct {
		name   string
		mutate func(*HealTerminalEffectSnapshot)
	}{
		{name: "malformed version", mutate: func(effect *HealTerminalEffectSnapshot) { effect.VersionID = "secret\x00version" }},
		{name: "auto publish review", mutate: func(effect *HealTerminalEffectSnapshot) { effect.ReviewID = "review" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := healGovernancePlan("run-next", streak.LastSequence+1, evidence.DecisionApplied, streak)
			plan.Fact.FactID = ""
			plan.Snapshot.ExistingTerminalEffect = &HealTerminalEffectSnapshot{Kind: terminal.Effect.Kind, CandidateHash: terminal.Effect.CandidateHash, Band: terminal.Effect.Band, Contributions: terminal.Effect.Contributions}
			test.mutate(plan.Snapshot.ExistingTerminalEffect)
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			assertHealGovernanceFault(t, decision, err, CodeHealTerminalEffectConflict, fault.Conflict, "heal terminal effect conflicts with persisted state")
		})
	}
}

func TestDefaultHealGovernancePlannerRejectsNoncanonicalIdentities(t *testing.T) {
	malformed := []string{
		" identity ",
		"identity\x00",
		"identity‮",
		string([]byte{0xff}),
		strings.Repeat("x", parameter.MaxNameBytes+1),
	}
	for _, identity := range malformed {
		t.Run(fmt.Sprintf("%q", identity), func(t *testing.T) {
			snapshotPlan := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
			snapshotPlan.Snapshot.Key.ElementTargetID = identity
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(snapshotPlan)
			assertHealGovernanceFault(t, decision, err, CodeHealGovernanceSnapshotInvalid, fault.FailedPrecondition, "heal governance snapshot is invalid")

			factPlan := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
			factPlan.Fact.CommitID = identity
			decision, err = NewDefaultHealGovernancePlanner().PlanHealGovernance(factPlan)
			assertHealGovernanceFault(t, decision, err, CodeHealAcceptedFactInvalid, fault.InvalidArgument, "accepted heal fact is invalid")
		})
	}
}

func TestDefaultHealGovernancePlannerRejectsMalformedNestedFactPayloads(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*HealGovernancePlan)
	}{
		{name: "observation execution control", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.ExecutionID = mustEntryID("secret\x00execution") }},
		{name: "observation step format", mutate: func(plan *HealGovernancePlan) {
			plan.Fact.Observation.StepExecutionID = mustStepExecutionID("secret​step")
		}},
		{name: "observation candidate invalid UTF-8", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.CandidateHash = string([]byte{0xff}) }},
		{name: "observation candidate whitespace", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.CandidateHash = "   " }},
		{name: "observation invalid confidence", mutate: func(plan *HealGovernancePlan) { plan.Fact.Observation.Confidence = math.NaN() }},
		{name: "reset execution control", mutate: func(plan *HealGovernancePlan) {
			plan.Fact.Kind, plan.Fact.Observation, plan.Fact.Reset = HealAcceptedReset, nil, validResetPayload()
			plan.Fact.Reset.ExecutionID = mustEntryID("secret\x00execution")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := healGovernancePlan("run-1", 1, evidence.DecisionApplied, domainautomation.HealStreak{})
			test.mutate(&plan)
			decision, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan)
			assertHealGovernanceFault(t, decision, err, CodeHealAcceptedFactInvalid, fault.InvalidArgument, "accepted heal fact is invalid")
		})
	}
}

func validResetPayload() *evidence.HealCandidateReset {
	return &evidence.HealCandidateReset{
		ExecutionID: mustEntryID("execution-reset"), StepExecutionID: mustStepExecutionID("step-reset"), ElementTargetID: "node", BaseNodeVersionID: "base", ObservedAt: 1,
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

func TestDefaultHealGovernancePlannerRejectsFailedCandidateGovernanceEvidence(t *testing.T) {
	for _, band := range []evidence.DecisionBand{evidence.DecisionApplied, evidence.DecisionBelowCap} {
		t.Run(string(band), func(t *testing.T) {
			plan := healGovernancePlan("run-failed", 1, band, domainautomation.HealStreak{})
			plan.Fact.Observation.Succeeded = false
			if _, err := NewDefaultHealGovernancePlanner().PlanHealGovernance(plan); !fault.IsCode(err, domainautomation.CodeHealObservationInvalid) || err.Error() != "AUTOMATION_HEAL_OBSERVATION_INVALID: heal observation is invalid" {
				t.Fatalf("failed candidate governance evidence error = %v", err)
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
			decision, err := planner.PlanHealGovernance(plan)
			assertHealGovernanceFault(t, decision, err, CodeHealGovernanceSnapshotInvalid, fault.FailedPrecondition, "heal governance snapshot is invalid")
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
		Snapshot: HealGovernanceSnapshot{Key: HealGovernanceKey{ElementTargetID: "node", BaseNodeVersionID: "base"}, CurrentNodeVersionID: "base", Revision: 2, Streak: first.NextStreak},
		Fact:     HealAcceptedFact{Kind: HealAcceptedReset, FactID: "reset", CommitID: "reset-commit", InstanceID: mustInstanceID("reset-run"), Sequence: 2, Reset: reset},
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
			decision, err := planner.PlanHealGovernance(plan)
			assertHealGovernanceFault(t, decision, err, CodeHealTerminalEffectConflict, fault.Conflict, "heal terminal effect conflicts with persisted state")
		})
	}
}
