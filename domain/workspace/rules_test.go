package workspace

import (
	"math"
	"testing"
)

func TestVersionRulesIncludeDeletedVersions(t *testing.T) {
	versions := []VersionMeta{{ID: "v1", VersionNumber: 1}, {ID: "v2", VersionNumber: 2, DeletedAt: 10}}
	if got := NextVersionNumber(versions); got != 3 {
		t.Fatalf("NextVersionNumber() = %d, want 3", got)
	}
	if id, ok := ResolveCurrentVersion(versions); !ok || id != "v1" {
		t.Fatalf("ResolveCurrentVersion() = %q, %v; want v1, true", id, ok)
	}
	versions[0].DeletedAt = 11
	if id, ok := ResolveCurrentVersion(versions); ok || id != "" {
		t.Fatalf("ResolveCurrentVersion() = %q, %v; want empty, false", id, ok)
	}
}

func TestHealStreakResetAndPromotionRules(t *testing.T) {
	current := HealStreak{CandidateHash: "candidate-a", Count: 2, Observing: true}
	for _, outcome := range []HealOutcome{HealOriginalRecovered, HealFailed} {
		decision, err := current.Observe(outcome, "", true)
		if err != nil || !decision.ResetAll || decision.NextCount != 0 {
			t.Fatalf("outcome %s decision=%+v err=%v", outcome, decision, err)
		}
	}
	changed, err := current.Observe(HealSucceeded, "candidate-b", true)
	if err != nil || !changed.ResetOthers || changed.NextCount != 1 || changed.Promote {
		t.Fatalf("different candidate decision=%+v err=%v", changed, err)
	}
	promote, err := current.Observe(HealSucceeded, "candidate-a", true)
	if err != nil || !promote.ResetOthers || promote.NextCount != 3 || !promote.Promote {
		t.Fatalf("third observation decision=%+v err=%v", promote, err)
	}
	stale, err := current.Observe(HealSucceeded, "candidate-a", false)
	if err != nil || !stale.MarkStale || stale.Promote {
		t.Fatalf("stale base decision=%+v err=%v", stale, err)
	}
}

func TestHealDecisionBandIsAnExplicitPolicyDecision(t *testing.T) {
	for _, band := range []HealDecisionBand{HealDecisionBandApplied, HealDecisionBandBelowCap} {
		if err := ValidateHealDecisionBand("candidate", band); err != nil {
			t.Fatalf("same confidence may belong to policy-selected band %q: %v", band, err)
		}
	}
	if err := ValidateHealDecisionBand("candidate", HealDecisionBandUnknown); err == nil {
		t.Fatal("candidate without an explicit applied/below-cap band was accepted")
	}
	if err := ValidateHealDecisionBand("", HealDecisionBandUnknown); err != nil {
		t.Fatalf("candidate-free observation was rejected: %v", err)
	}
	if err := ValidateHealConfidence(1.01); err == nil {
		t.Fatal("confidence above one was accepted")
	}
	if err := ValidateHealConfidence(math.NaN()); err == nil {
		t.Fatal("NaN confidence was accepted")
	}
}

func TestHealCandidateReviewCommandValidation(t *testing.T) {
	command := HealCandidateReviewCommand{NodeID: "node-1", BaseNodeVersionID: "nv-1",
		CandidateHash: "candidate", PromotedVersionID: "nv-2", ReviewedBy: "local-user", ReviewedAt: 1}
	if err := command.Validate(HealApprovalApproved); err != nil {
		t.Fatalf("valid approval rejected: %v", err)
	}
	command.PromotedVersionID = ""
	if err := command.Validate(HealApprovalApproved); err == nil {
		t.Fatal("approval without promoted version id was accepted")
	}
	if err := command.Validate(HealApprovalRejected); err != nil {
		t.Fatalf("valid rejection rejected: %v", err)
	}
	command.ReviewedBy = ""
	if err := command.Validate(HealApprovalRejected); err == nil {
		t.Fatal("review without reviewer was accepted")
	}
}

func TestStatusTransitionsRejectIllegalMoves(t *testing.T) {
	for _, transition := range [][2]TestTaskRunStatus{{RunQueued, RunRunning}, {RunQueued, RunCanceled},
		{RunRunning, RunSucceeded}, {RunRunning, RunFailed}, {RunRunning, RunAborted}} {
		if err := ValidateRunStatusTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("valid run transition %s -> %s rejected: %v", transition[0], transition[1], err)
		}
	}
	for _, transition := range [][2]TestTaskRunStatus{{RunQueued, RunSucceeded}, {RunRunning, RunCanceled},
		{RunSucceeded, RunRunning}} {
		if err := ValidateRunStatusTransition(transition[0], transition[1]); err == nil {
			t.Fatalf("illegal run transition %s -> %s accepted", transition[0], transition[1])
		}
	}
	if err := ValidateExecutionStatusTransition(ExecutionPending, ExecutionRunning); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutionStatusTransition(ExecutionSucceeded, ExecutionRunning); err == nil {
		t.Fatal("terminal execution restarted")
	}
}

func TestEnvironmentAllowsBlankBaseURLAndRejectsReservedCustomVariables(t *testing.T) {
	valid := Environment{ID: "env", DisplayName: "无地址环境", Variables: Properties{"Tenant": "north"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("blank base URL rejected: %v", err)
	}
	for _, key := range []string{"base_url", "username", "password"} {
		invalid := valid
		invalid.Variables = invalid.Variables.Clone()
		invalid.Variables[key] = "override"
		if err := invalid.Validate(); err == nil {
			t.Fatalf("reserved environment key %q accepted", key)
		}
	}
}
