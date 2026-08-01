package heal

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// Heal is exported, so a Host can call it with its own DOM snapshot and keep
// that snapshot afterwards. Everything in the returned Decision therefore has
// to be the caller's to edit. Two references used to escape: each Candidate
// carried the snapshot candidate's own fingerprint, and Best was a struct copy
// of Candidates[0], which shares that fingerprint's map and slices.

// retainedSnapshot is the shape a Host actually implements: it keeps its
// candidates and hands the same slice out on every call.
type retainedSnapshot struct{ candidates []SnapshotCandidate }

func (s *retainedSnapshot) Candidates(context.Context) ([]SnapshotCandidate, error) {
	return s.candidates, nil
}

func aliasingSnapshot() []SnapshotCandidate {
	return []SnapshotCandidate{{
		Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#submit", Priority: 1},
		Fingerprint: fingerprint.Fingerprint{
			Tag:        "button",
			Attributes: map[string]string{"id": "submit"},
			Path:       []string{"html", "body", "button"},
			Framework:  fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact, Confidence: 0.9}},
		},
	}}
}

func aliasingTarget() fingerprint.ElementTargetSpec {
	return fingerprint.ElementTargetSpec{
		UUID: "00000000-0000-0000-0000-000000000001", ID: "target-1",
		PageURL: "https://example.test/form", Origin: "https://example.test",
		Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit", Priority: 1}},
		Fingerprint: fingerprint.Fingerprint{
			Tag:        "button",
			Attributes: map[string]string{"id": "submit"},
			Path:       []string{"html", "body", "button"},
			Framework:  fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact, Confidence: 0.9}},
		},
	}
}

func TestHealDecisionNeverAliasesTheCallersSnapshot(t *testing.T) {
	snapshot := &retainedSnapshot{candidates: aliasingSnapshot()}
	healer := &DefaultHealer{Weights: DefaultWeights(), Thresholds: DefaultThresholds()}
	decision, err := healer.Heal(context.Background(), aliasingTarget(), snapshot)
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if len(decision.Candidates) == 0 {
		t.Fatal("no candidate produced; this fixture must reach the scoring path")
	}

	decision.Candidates[0].Fingerprint.Attributes["id"] = "mutated"
	decision.Candidates[0].Fingerprint.Path[0] = "mutated"
	decision.Candidates[0].Fingerprint.Framework[0].Kind = "MUTATED"

	if got := snapshot.candidates[0].Fingerprint.Attributes["id"]; got != "submit" {
		t.Errorf("candidate attributes alias the caller's snapshot: %q", got)
	}
	if got := snapshot.candidates[0].Fingerprint.Path[0]; got != "html" {
		t.Errorf("candidate path aliases the caller's snapshot: %q", got)
	}
	if got := string(snapshot.candidates[0].Fingerprint.Framework[0].Kind); got != string(fingerprint.FrameworkReact) {
		t.Errorf("candidate framework aliases the caller's snapshot: %q", got)
	}
}

func TestHealDecisionBestDoesNotShareAFingerprintWithCandidatesZero(t *testing.T) {
	healer := &DefaultHealer{Weights: DefaultWeights(), Thresholds: DefaultThresholds()}
	decision, err := healer.Heal(context.Background(), aliasingTarget(), &retainedSnapshot{candidates: aliasingSnapshot()})
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if decision.Best == nil {
		t.Skip("this fixture did not reach a Best outcome; nothing to check")
	}

	decision.Best.Fingerprint.Attributes["id"] = "mutated"
	decision.Best.Fingerprint.Path[0] = "mutated"
	if got := decision.Candidates[0].Fingerprint.Attributes["id"]; got == "mutated" {
		t.Error("Best and Candidates[0] share one attributes map")
	}
	if got := decision.Candidates[0].Fingerprint.Path[0]; got == "mutated" {
		t.Error("Best and Candidates[0] share one path slice")
	}
}
