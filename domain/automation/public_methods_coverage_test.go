package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestHealStreakValidateStateAndRuleMatrix(t *testing.T) {
	observing := HealStreak{
		NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate",
		Band: HealDecisionBandApplied, Contributions: contributions("run-1"),
		LastSequence: 1, Observing: true, Disposition: HealStreakObserving,
	}
	autoPublish := HealStreak{
		NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate",
		Band: HealDecisionBandApplied, Contributions: contributions("run-1", "run-2", "run-3"),
		LastSequence: 3, Disposition: HealStreakAutoPublish,
	}
	awaitApproval := autoPublish
	awaitApproval.Band = HealDecisionBandBelowCap
	awaitApproval.Disposition = HealStreakAwaitApproval

	tests := []struct {
		name      string
		streak    HealStreak
		wantError string
	}{
		{name: "zero value is inactive"},
		{name: "observing", streak: observing},
		{name: "auto publish", streak: autoPublish},
		{name: "await approval", streak: awaitApproval},
		{name: "reset", streak: HealStreak{NodeID: "node", BaseNodeVersionID: "base", LastSequence: 1, Disposition: HealStreakReset}},
		{name: "stale", streak: HealStreak{NodeID: "node", BaseNodeVersionID: "base", LastSequence: 1, Disposition: HealStreakStale}},
		{name: "rejected", streak: HealStreak{NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", LastSequence: 1, Disposition: HealStreakRejected}},
		{name: "consumed observation beyond last sequence", streak: func() HealStreak {
			value := observing
			value.LastSequence = 0
			return value
		}(), wantError: "exceeds last sequence"},
		{name: "reset cannot be active", streak: HealStreak{NodeID: "node", BaseNodeVersionID: "base", LastSequence: 1, Observing: true, Disposition: HealStreakReset}, wantError: "inactive node/base sequence identity"},
		{name: "rejected requires candidate", streak: HealStreak{NodeID: "node", BaseNodeVersionID: "base", LastSequence: 1, Disposition: HealStreakRejected}, wantError: "inactive candidate identity"},
		{name: "active requires identity", streak: HealStreak{NodeID: "node", BaseNodeVersionID: "base", Band: HealDecisionBandApplied, LastSequence: 1, Observing: true, Disposition: HealStreakObserving}, wantError: "requires node, base version, and candidate identity"},
		{name: "active rejects unknown band", streak: func() HealStreak {
			value := observing
			value.Band = HealDecisionBandUnknown
			return value
		}(), wantError: "requires APPLIED or BELOW_CAP"},
		{name: "observing requires observing disposition", streak: func() HealStreak {
			value := observing
			value.Disposition = HealStreakAutoPublish
			return value
		}(), wantError: "invalid disposition or maturity"},
		{name: "observing cannot be mature", streak: func() HealStreak {
			value := autoPublish
			value.Observing = true
			value.Disposition = HealStreakObserving
			return value
		}(), wantError: "invalid disposition or maturity"},
		{name: "auto publish requires applied band", streak: func() HealStreak {
			value := autoPublish
			value.Band = HealDecisionBandBelowCap
			return value
		}(), wantError: "three APPLIED runs"},
		{name: "auto publish requires three runs", streak: func() HealStreak {
			value := autoPublish
			value.Contributions = contributions("run-1", "run-2")
			return value
		}(), wantError: "three APPLIED runs"},
		{name: "await approval requires below cap band", streak: func() HealStreak {
			value := awaitApproval
			value.Band = HealDecisionBandApplied
			return value
		}(), wantError: "three BELOW_CAP runs"},
		{name: "inactive rejects unsupported disposition", streak: func() HealStreak {
			value := autoPublish
			value.Disposition = HealStreakDisposition("UNKNOWN")
			return value
		}(), wantError: "unsupported inactive"},
		{name: "active requires last sequence", streak: HealStreak{NodeID: "node", BaseNodeVersionID: "base", CandidateHash: "candidate", Band: HealDecisionBandApplied, Observing: true, Disposition: HealStreakObserving}, wantError: "requires a last sequence"},
		{name: "invalid contribution", streak: func() HealStreak {
			value := observing
			value.Contributions = append([]ContributingHealFact(nil), observing.Contributions...)
			value.Contributions[0].FactID = " "
			return value
		}(), wantError: "heal contribution"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.streak.Validate()
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

func TestNodeAggregateCloneOwnsNestedMutableState(t *testing.T) {
	original := versionedNodeAggregate()
	original.Node.Properties = Properties{"region": "east"}
	original.Current.Fingerprint.Path = []string{"main", "button"}
	original.Current.Fingerprint.Attributes = map[string]string{"role": "button"}
	original.Current.Fingerprint.Framework = fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact}}
	original.Versions[1] = original.Current

	cloned := original.Clone()
	cloned.Node.Properties["region"] = "west"
	cloned.Current.Selectors[0].Value = "#changed"
	cloned.Current.Fingerprint.Path[0] = "aside"
	cloned.Current.Fingerprint.Attributes["role"] = "link"
	cloned.Current.Fingerprint.Framework[0].Kind = fingerprint.FrameworkVue
	cloned.Versions[1].Fingerprint.Framework[0].Kind = fingerprint.FrameworkAngular

	if original.Node.Properties["region"] != "east" ||
		original.Current.Selectors[0].Value != "#node" ||
		original.Current.Fingerprint.Path[0] != "main" ||
		original.Current.Fingerprint.Attributes["role"] != "button" ||
		original.Current.Fingerprint.Framework[0].Kind != fingerprint.FrameworkReact ||
		original.Versions[1].Fingerprint.Framework[0].Kind != fingerprint.FrameworkReact {
		t.Fatalf("Clone() aliases nested state: original=%#v clone=%#v", original, cloned)
	}
}
