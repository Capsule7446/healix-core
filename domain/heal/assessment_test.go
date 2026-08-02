package heal

import (
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"testing"
)

func assessmentSpec() fingerprint.ElementTargetSpec {
	return fingerprint.ElementTargetSpec{ID: "target", PageURL: "https://shop.test/checkout", Origin: "https://shop.test", Role: "button", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"id": "submit"}, SiblingIndex: 0}}
}
func TestAssessBlocksOriginMismatch(t *testing.T) {
	target := assessmentSpec()
	candidate := Candidate{Selector: target.Selectors[0], Fingerprint: target.Fingerprint, Score: 0.99}
	d := Decision{Outcome: OutcomeApplied, Best: &candidate, Candidates: []Candidate{candidate}}
	a, err := Assess(target, d, ExecutionContext{Origin: "https://evil.test", PageURL: target.PageURL}, SafetyPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Disposition != DispositionBlock || len(a.Reasons) != 1 || a.Reasons[0] != ReasonOriginMismatch {
		t.Fatalf("assessment=%+v", a)
	}
}
func TestAssessReviewsAmbiguousCandidates(t *testing.T) {
	target := assessmentSpec()
	a1 := Candidate{Selector: target.Selectors[0], Fingerprint: target.Fingerprint, Score: 0.91}
	a2 := a1
	a2.Selector.Value = "#other"
	a2.Score = 0.90
	d := Decision{Outcome: OutcomeApplied, Best: &a1, Candidates: []Candidate{a1, a2}}
	a, err := Assess(target, d, ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, SafetyPolicy{MinimumMargin: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	if a.Disposition != DispositionReview || a.Reasons[0] != ReasonAmbiguous {
		t.Fatalf("assessment=%+v", a)
	}
}
func TestAssessAllowsDistinctHighConfidenceCandidate(t *testing.T) {
	target := assessmentSpec()
	candidate := Candidate{Selector: target.Selectors[0], Fingerprint: target.Fingerprint, Score: 0.99}
	d := Decision{Outcome: OutcomeApplied, Best: &candidate, Candidates: []Candidate{candidate}}
	a, err := Assess(target, d, ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, SafetyPolicy{MinimumMargin: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	if a.Disposition != DispositionAllow {
		t.Fatalf("assessment=%+v", a)
	}
}
