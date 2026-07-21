package heal

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestDecisionSamplesRetainEligibleCandidatesAndSelection(t *testing.T) {
	target := fingerprint.Fingerprint{Tag: "button", ARIA: fingerprint.ARIA{Role: "button"}}
	first := Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#primary"}, Fingerprint: target, Score: 0.9}
	second := Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#secondary"}, Fingerprint: target, Score: 0.65}
	third := Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#weak"}, Fingerprint: target, Score: 0.4}
	decision := Decision{Outcome: OutcomeApplied, Best: &first, Candidates: []Candidate{first, second, third}}

	samples := decision.Samples(target, 0.6)
	if err := ValidateSamples(samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) != 3 || !samples[0].Selected || !samples[0].Eligible || !samples[1].Eligible || samples[2].Eligible {
		t.Fatalf("samples=%+v", samples)
	}
	if samples[0].Rank != 1 || samples[1].Rank != 2 || samples[2].Rank != 3 {
		t.Fatalf("ranks=%+v", samples)
	}
}
