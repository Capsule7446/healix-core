package workspace

import (
	"testing"
)

func TestHealCandidateEvidenceValidatesReplayIdentity(t *testing.T) {
	evidence := HealCandidateEvidence{
		ObservationID: "heal-1", CandidateHash: "hash-1", FingerprintHash: "fp-hash",
		Rank: 1, Score: 0.9, ObservedAt: 10,
	}
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := evidence
	invalid.CandidateHash = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected missing candidate hash error")
	}
}
