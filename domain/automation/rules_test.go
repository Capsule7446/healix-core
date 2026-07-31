package automation

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func contributions(runIDs ...string) []ContributingHealFact {
	result := make([]ContributingHealFact, len(runIDs))
	for index, runID := range runIDs {
		result[index] = acceptedHealObservation(runID, uint64(index+1), "candidate", HealDecisionBandApplied).contribution()
	}
	return result
}

func acceptedHealObservation(runID string, sequence uint64, candidateHash string, band HealDecisionBand) HealObservation {
	return HealObservation{
		FactID: "fact-" + runID, CommitID: "commit-" + runID, RunID: runID,
		ExecutionID: "execution-" + runID, StepExecutionID: "step-" + runID, Sequence: sequence,
		ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: candidateHash,
		Band: band, Outcome: HealSucceeded, BaseIsCurrent: true,
	}
}

func TestHealStreakRetainsCompleteContributionProvenance(t *testing.T) {
	streak := HealStreak{}
	for index, sequence := range []uint64{11, 19, 27} {
		observation := acceptedHealObservation(fmt.Sprintf("run-%d", index+1), sequence, "candidate", HealDecisionBandApplied)
		decision, err := streak.Observe(observation)
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		streak = decision.Next
	}
	if streak.Disposition != HealStreakAutoPublish || len(streak.Contributions) != 3 {
		t.Fatalf("mature streak = %#v", streak)
	}
	for index, contribution := range streak.Contributions {
		wantRunID := fmt.Sprintf("run-%d", index+1)
		if contribution.RunID != wantRunID || contribution.FactID != "fact-"+wantRunID || contribution.Sequence != []uint64{11, 19, 27}[index] {
			t.Fatalf("contribution %d = %#v", index, contribution)
		}
	}
}

func TestHealStreakRejectsConflictingContributionReplay(t *testing.T) {
	first := acceptedHealObservation("run-1", 1, "candidate", HealDecisionBandApplied)
	decision, err := (HealStreak{}).Observe(first)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := decision.Next.Observe(first)
	if err != nil || len(replay.Next.Contributions) != 1 {
		t.Fatalf("exact replay = %#v, %v", replay.Next, err)
	}
	conflict := first
	conflict.CommitID = "other-commit"
	if _, err := decision.Next.Observe(conflict); err == nil {
		t.Fatal("conflicting replay was accepted")
	}
}

func TestVersionRulesIncludeDeletedVersions(t *testing.T) {
	versions := []VersionMeta{{ID: "v1", VersionNumber: 1}, {ID: "v2", VersionNumber: 2, DeletedAt: 10}}
	got, err := NextVersionNumber(versions)
	if err != nil || got != 3 {
		t.Fatalf("NextVersionNumber() = %d, %v; want 3, nil", got, err)
	}
	if id, ok := ResolveCurrentVersion(versions); !ok || id != "v1" {
		t.Fatalf("ResolveCurrentVersion() = %q, %v; want v1, true", id, ok)
	}
	for _, invalid := range []int{0, -1, math.MinInt} {
		versionID := "version-secret"
		_, err := NextVersionNumber([]VersionMeta{{ID: versionID, VersionNumber: invalid}})
		descriptor, ok := fault.Describe(err)
		if !fault.IsCode(err, CodePersistedVersionNumberInvalid) || !ok || descriptor.Code() != CodePersistedVersionNumberInvalid || descriptor.Kind() != fault.FailedPrecondition || descriptor.Message() != "persisted version number must be positive" || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 || strings.Contains(err.Error(), versionID) {
			t.Fatalf("invalid version number %d error/descriptor = %v/%#v", invalid, err, descriptor)
		}
	}
	versions[0].DeletedAt = 11
	if id, ok := ResolveCurrentVersion(versions); ok || id != "" {
		t.Fatalf("ResolveCurrentVersion() = %q, %v; want empty, false", id, ok)
	}
}

func TestHealStreakCountsDistinctRunsAndSeparatesMaturityDisposition(t *testing.T) {
	for _, test := range []struct {
		name string
		band HealDecisionBand
		want HealStreakDisposition
	}{
		{"applied auto-publishes", HealDecisionBandApplied, HealStreakAutoPublish},
		{"below cap awaits approval", HealDecisionBandBelowCap, HealStreakAwaitApproval},
	} {
		t.Run(test.name, func(t *testing.T) {
			streak := HealStreak{}
			for sequence, runID := range []string{"run-1", "run-2", "run-2", "run-3"} {
				observationSequence := uint64(sequence + 1)
				if runID == "run-2" && sequence == 2 {
					observationSequence = 2
				}
				decision, err := streak.Observe(HealObservation{FactID: "fact-" + runID, CommitID: "commit-" + runID, ExecutionID: "execution-" + runID, StepExecutionID: "step-" + runID, RunID: runID, Sequence: observationSequence, ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: test.band, Outcome: HealSucceeded, BaseIsCurrent: true})
				if err != nil {
					t.Fatalf("Observe(%s): %v", runID, err)
				}
				streak = decision.Next
			}
			if len(streak.Contributions) != 3 || streak.Observing || streak.Disposition != test.want {
				t.Fatalf("streak = %#v, want 3 distinct runs and %q", streak, test.want)
			}
		})
	}
}

func TestHealStreakIdentityChangeStartsNewConsecutiveSequence(t *testing.T) {
	streak := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "old", Band: HealDecisionBandApplied, Contributions: contributions("run-1", "run-2"), LastSequence: 2, Observing: true, Disposition: HealStreakObserving}
	decision, err := streak.Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-3", Sequence: 3, ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "new", Band: HealDecisionBandApplied, Outcome: HealSucceeded, BaseIsCurrent: true})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if len(decision.Next.Contributions) != 1 || decision.Next.Contributions[0].RunID != "run-3" || decision.Next.Disposition != HealStreakObserving {
		t.Fatalf("changed identity decision = %#v", decision)
	}
}

func TestHealStreakFailurePreservesAndRecoveryIsScoped(t *testing.T) {
	streak := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Contributions: contributions("run-1", "run-2"), LastSequence: 2, Observing: true, Disposition: HealStreakObserving}
	failed, err := streak.Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-3", Sequence: 3, ElementTargetID: "node", BaseNodeVersionID: "base", Outcome: HealFailed, BaseIsCurrent: true})
	if err != nil || len(failed.Next.Contributions) != 2 || failed.Next.LastSequence != 3 {
		t.Fatalf("failed observation changed maturity evidence or ordering: %#v, %v", failed, err)
	}
	other, err := failed.Next.Observe(HealObservation{FactID: "fact-4", CommitID: "commit-4", ExecutionID: "execution-4", StepExecutionID: "step-4", RunID: "run-4", Sequence: 4, ElementTargetID: "other", BaseNodeVersionID: "base", Outcome: HealOriginalRecovered, BaseIsCurrent: true})
	if err != nil || other.Next.Disposition != HealStreakObserving || other.Next.LastSequence != 4 {
		t.Fatalf("other-node recovery reset streak: %#v, %v", other, err)
	}
}

func TestHealStreakRejectsStaleReplayAfterIdentityChange(t *testing.T) {
	streak := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Contributions: contributions("run-2"), LastSequence: 2, Observing: true, Disposition: HealStreakObserving}
	_, err := streak.Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-1", Sequence: 1, ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "other", Band: HealDecisionBandApplied, Outcome: HealSucceeded, BaseIsCurrent: true})
	if err == nil {
		t.Fatal("stale replay was accepted")
	}
}

func TestHealStreakRejectIsImmutableAndTerminal(t *testing.T) {
	observing := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandBelowCap, Contributions: contributions("run-1", "run-2", "run-3"), LastSequence: 3, Disposition: HealStreakAwaitApproval}
	rejected, err := observing.Reject(4)
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if observing.Disposition != HealStreakAwaitApproval || len(observing.Contributions) != 3 {
		t.Fatalf("source streak mutated: %#v", observing)
	}
	if rejected.Next.Disposition != HealStreakRejected || rejected.Next.Observing || rejected.Next.LastSequence != 4 {
		t.Fatalf("rejected streak = %#v", rejected.Next)
	}
	frozen, err := rejected.Next.Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-4", Sequence: 5, ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandBelowCap, Outcome: HealSucceeded, BaseIsCurrent: true})
	if err != nil || frozen.Next.Disposition != HealStreakRejected || len(frozen.Next.Contributions) != 3 {
		t.Fatalf("matching observation reopened rejected streak: %#v, %v", frozen.Next, err)
	}
	restarted, err := frozen.Next.Observe(HealObservation{FactID: "fact-6", CommitID: "commit-6", ExecutionID: "execution-6", StepExecutionID: "step-6", RunID: "run-5", Sequence: 6, ElementTargetID: "node", BaseNodeVersionID: "base-v2", CandidateHash: "candidate-v2", Band: HealDecisionBandApplied, Outcome: HealSucceeded, BaseIsCurrent: true})
	if err != nil || !restarted.Next.Observing || restarted.Next.BaseNodeVersionID != "base-v2" || len(restarted.Next.Contributions) != 1 {
		t.Fatalf("new-base observation did not restart rejected streak: %#v, %v", restarted.Next, err)
	}
	if _, err := observing.Reject(2); err == nil {
		t.Fatal("stale rejection sequence was accepted")
	}
}

func TestHealStreakStaleOldBaseAllowsCurrentNewBaseObservation(t *testing.T) {
	stale := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base-v1", LastSequence: 2, Disposition: HealStreakStale}
	decision, err := stale.Observe(HealObservation{
		FactID: "fact-run-3", CommitID: "commit-run-3", RunID: "run-3", ExecutionID: "execution-run-3", StepExecutionID: "step-run-3",
		Sequence: 3, ElementTargetID: "node", BaseNodeVersionID: "base-v2",
		CandidateHash: "candidate", Band: HealDecisionBandApplied, Outcome: HealSucceeded, BaseIsCurrent: true,
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !decision.Next.Observing || decision.Next.BaseNodeVersionID != "base-v2" || len(decision.Next.Contributions) != 1 {
		t.Fatalf("new-base observation = %#v, want a fresh observing streak", decision.Next)
	}
}

func TestHealStreakTerminalAndResetReplayAreIdempotent(t *testing.T) {
	mature := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Contributions: contributions("run-1", "run-2", "run-3"), LastSequence: 3, Disposition: HealStreakAutoPublish}
	replayed, err := mature.Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-4", Sequence: 4, ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Outcome: HealSucceeded})
	if err != nil || replayed.Next.Disposition != HealStreakAutoPublish || replayed.Next.Observing {
		t.Fatalf("terminal replay = %#v, %v", replayed, err)
	}

	reset, err := (HealStreak{}).Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-1", Sequence: 1, ElementTargetID: "node", BaseNodeVersionID: "base", Outcome: HealOriginalRecovered, BaseIsCurrent: true})
	if err != nil || reset.Next.Disposition != HealStreakReset || reset.Next.ElementTargetID != "node" || reset.Next.LastSequence != 1 {
		t.Fatalf("reset = %#v, %v", reset, err)
	}
	resetReplay, err := reset.Next.Observe(HealObservation{FactID: "fact-2", CommitID: "commit-2", ExecutionID: "execution-2", StepExecutionID: "step-2", RunID: "run-2", Sequence: 2, ElementTargetID: "node", BaseNodeVersionID: "base", Outcome: HealFailed})
	if err != nil || resetReplay.Next.Disposition != HealStreakReset || resetReplay.Next.LastSequence != 2 {
		t.Fatalf("reset replay = %#v, %v", resetReplay, err)
	}
	restarted, err := resetReplay.Next.Observe(HealObservation{FactID: "fact-3", CommitID: "commit-3", ExecutionID: "execution-3", StepExecutionID: "step-3", RunID: "run-3", Sequence: 3, ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Outcome: HealSucceeded, BaseIsCurrent: true})
	if err != nil || !restarted.Next.Observing || len(restarted.Next.Contributions) != 1 {
		t.Fatalf("restart after reset = %#v, %v", restarted, err)
	}
}

func TestHealStreakIgnoredObservationAdvancesOrdering(t *testing.T) {
	streak := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Contributions: contributions("run-1"), LastSequence: 1, Observing: true, Disposition: HealStreakObserving}
	ignored, err := streak.Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-10", Sequence: 10, ElementTargetID: "node", BaseNodeVersionID: "base", Outcome: HealFailed, BaseIsCurrent: true})
	if err != nil || ignored.Next.LastSequence != 10 || len(ignored.Next.Contributions) != 1 {
		t.Fatalf("ignored observation = %#v, %v", ignored, err)
	}
	_, err = ignored.Next.Observe(HealObservation{FactID: "fact", CommitID: "commit", ExecutionID: "execution", StepExecutionID: "step", RunID: "run-9", Sequence: 9, ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Outcome: HealSucceeded, BaseIsCurrent: true})
	if err == nil {
		t.Fatal("older observation after ignored newer fact was accepted")
	}
}

func TestMatureHealStreakAdvancesOrderingBeforeReview(t *testing.T) {
	streak := HealStreak{ElementTargetID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandBelowCap, Contributions: contributions("run-1", "run-2", "run-3"), LastSequence: 3, Disposition: HealStreakAwaitApproval}
	advanced, err := streak.Observe(acceptedHealObservation("run-10", 10, "candidate", HealDecisionBandBelowCap))
	if err != nil || advanced.Next.LastSequence != 10 {
		t.Fatalf("mature observation = %#v, %v", advanced, err)
	}
	if _, err := advanced.Next.Reject(4); err == nil {
		t.Fatal("review older than an accepted observation was accepted")
	}
}

func TestTerminalHealStreakCannotRestartForAnotherNode(t *testing.T) {
	for _, disposition := range []HealStreakDisposition{HealStreakStale, HealStreakRejected} {
		t.Run(string(disposition), func(t *testing.T) {
			streak := HealStreak{ElementTargetID: "node-a", BaseNodeVersionID: "base-v1", LastSequence: 2, Disposition: disposition}
			if disposition == HealStreakRejected {
				streak.CandidateHash = "candidate-a"
			}
			decision, err := streak.Observe(HealObservation{
				FactID: "fact-3", CommitID: "commit-3", RunID: "run-3", ExecutionID: "execution-3", StepExecutionID: "step-3",
				Sequence: 3, ElementTargetID: "node-b", BaseNodeVersionID: "base-v2", CandidateHash: "candidate-b",
				Band: HealDecisionBandApplied, Outcome: HealSucceeded, BaseIsCurrent: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Next.ElementTargetID != "node-a" || decision.Next.Disposition != disposition || decision.Next.LastSequence != 3 {
				t.Fatalf("cross-node terminal restart = %#v", decision.Next)
			}
		})
	}
}

func TestHealDecisionInputValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		candidate string
		band      HealDecisionBand
	}{
		{name: "candidate applied", candidate: "candidate", band: HealDecisionBandApplied},
		{name: "candidate below cap", candidate: "candidate", band: HealDecisionBandBelowCap},
		{name: "no candidate unknown", candidate: "", band: HealDecisionBandUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateHealDecisionBand(test.candidate, test.band); err != nil {
				t.Fatalf("valid decision rejected: %v", err)
			}
		})
	}

	for _, test := range []struct {
		name      string
		candidate string
		band      HealDecisionBand
	}{
		{name: "candidate missing", candidate: " \t\n", band: HealDecisionBandApplied},
		{name: "candidate unknown", candidate: "candidate-secret", band: HealDecisionBandUnknown},
		{name: "candidate malformed band", candidate: "candidate-secret", band: HealDecisionBand("malicious\nband-secret")},
		{name: "candidate-free malformed band", candidate: "", band: HealDecisionBand("malicious\nband-secret")},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateHealDecisionBand(test.candidate, test.band)
			descriptor, ok := fault.Describe(err)
			if !fault.IsCode(err, CodeHealDecisionBandInvalid) || !ok || descriptor.Kind() != fault.InvalidArgument || descriptor.Message() != "heal decision band is invalid" || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 || strings.Contains(err.Error(), "candidate-secret") || strings.Contains(err.Error(), "band-secret") || strings.Contains(err.Error(), "malicious") {
				t.Fatalf("error/descriptor = %v/%#v", err, descriptor)
			}
		})
	}

	for _, confidence := range []float64{0, math.SmallestNonzeroFloat64, 0.5, math.Nextafter(1, 0), 1} {
		if err := ValidateHealConfidence(confidence); err != nil {
			t.Fatalf("ValidateHealConfidence(%v) error = %v", confidence, err)
		}
	}
	for _, confidence := range []float64{math.Inf(-1), -math.SmallestNonzeroFloat64, math.Nextafter(1, 2), math.Inf(1), math.NaN()} {
		err := ValidateHealConfidence(confidence)
		descriptor, ok := fault.Describe(err)
		if !fault.IsCode(err, CodeHealConfidenceInvalid) || !ok || descriptor.Kind() != fault.InvalidArgument || descriptor.Message() != "heal confidence is invalid" || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
			t.Fatalf("confidence %v error/descriptor = %v/%#v", confidence, err, descriptor)
		}
	}
}

func TestHealCandidateReviewCommandValidation(t *testing.T) {
	valid := HealCandidateReviewCommand{
		CommandID:                 "command-secret",
		ElementTargetID:           "node-secret",
		BaseNodeVersionID:         "version-secret",
		CandidateHash:             "candidate-secret",
		ExpectedCandidateRevision: 1,
		ExpectedNodeRevision:      1,
	}
	for _, approval := range []HealApprovalStatus{HealApprovalApproved, HealApprovalRejected} {
		if err := valid.Validate(approval); err != nil {
			t.Fatalf("valid %q decision rejected: %v", approval, err)
		}
	}

	tests := []struct {
		name        string
		command     HealCandidateReviewCommand
		approval    HealApprovalStatus
		wantCode    fault.Code
		wantKind    fault.Kind
		wantMessage string
	}{
		{
			name: "whitespace identity",
			command: func() HealCandidateReviewCommand {
				command := valid
				command.CommandID = " \t\n"
				return command
			}(),
			approval:    HealApprovalApproved,
			wantCode:    CodeHealCandidateReviewCommandInvalid,
			wantKind:    fault.InvalidArgument,
			wantMessage: "heal candidate review command is invalid",
		},
		{
			name:        "malicious approval",
			command:     valid,
			approval:    HealApprovalStatus("malicious\napproval-secret"),
			wantCode:    CodeHealApprovalStatusInvalid,
			wantKind:    fault.InvalidArgument,
			wantMessage: "heal approval status is invalid",
		},
		{
			name: "candidate revision zero",
			command: func() HealCandidateReviewCommand {
				command := valid
				command.ExpectedCandidateRevision = 0
				return command
			}(),
			approval:    HealApprovalRejected,
			wantCode:    CodePersistedRevisionInvalid,
			wantKind:    fault.FailedPrecondition,
			wantMessage: "persisted revision must be non-zero",
		},
		{
			name: "node revision zero",
			command: func() HealCandidateReviewCommand {
				command := valid
				command.ExpectedNodeRevision = 0
				return command
			}(),
			approval:    HealApprovalRejected,
			wantCode:    CodePersistedRevisionInvalid,
			wantKind:    fault.FailedPrecondition,
			wantMessage: "persisted revision must be non-zero",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.command.Validate(test.approval)
			descriptor, ok := fault.Describe(err)
			if !fault.IsCode(err, test.wantCode) || !ok || descriptor.Code() != test.wantCode || descriptor.Kind() != test.wantKind || descriptor.Message() != test.wantMessage || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
				t.Fatalf("error/descriptor = %v/%#v", err, descriptor)
			}
			for _, secret := range []string{"command-secret", "node-secret", "version-secret", "candidate-secret", "malicious", "approval-secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("public error leaked %q: %q", secret, err.Error())
				}
			}
		})
	}

	identitySetters := []struct {
		name string
		set  func(*HealCandidateReviewCommand, string)
	}{
		{name: "command", set: func(command *HealCandidateReviewCommand, value string) { command.CommandID = value }},
		{name: "node", set: func(command *HealCandidateReviewCommand, value string) { command.ElementTargetID = value }},
		{name: "base version", set: func(command *HealCandidateReviewCommand, value string) { command.BaseNodeVersionID = value }},
		{name: "candidate", set: func(command *HealCandidateReviewCommand, value string) { command.CandidateHash = value }},
	}
	malformedIdentities := []string{"", " \t\n", " leading", "trailing ", "line\nfeed", "carriage\rreturn", "tab\tvalue", "nul\x00value", "nextline", "override‮value", "zero​width", strings.Repeat("x", parameter.MaxNameBytes+1), string([]byte{0xff})}
	for _, field := range identitySetters {
		for _, malformed := range malformedIdentities {
			t.Run("malformed "+field.name, func(t *testing.T) {
				command := valid
				field.set(&command, malformed)
				err := command.Validate(HealApprovalApproved)
				if !fault.IsCode(err, CodeHealCandidateReviewCommandInvalid) || malformed != "" && strings.Contains(err.Error(), malformed) {
					t.Fatalf("malformed identity error = %v", err)
				}
			})
		}
	}
}

func TestEnvironmentAllowsBlankBaseURLAndArbitraryVariableNames(t *testing.T) {
	valid := Environment{ID: "env", DisplayName: "无地址环境", Variables: EnvironmentVariables{"Tenant": parameter.TextValue("north")}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("blank base URL rejected: %v", err)
	}
	for _, key := range []string{"base_url", "username", "password"} {
		candidate := valid
		candidate.Variables = candidate.Variables.Clone()
		candidate.Variables[key] = parameter.TextValue("value")
		if err := candidate.Validate(); err != nil {
			t.Fatalf("variable key %q rejected: %v", key, err)
		}
	}
}
