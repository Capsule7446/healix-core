package heal

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestAssessDispositionBusinessMatrix(t *testing.T) {
	target := assessmentSpec()
	base := Candidate{Selector: target.Selectors[0], Fingerprint: target.Fingerprint, Score: 0.99}
	cases := []struct {
		name       string
		decision   Decision
		current    ExecutionContext
		policy     SafetyPolicy
		want       Disposition
		wantReason ReasonCode
	}{
		{name: "allow high confidence", decision: Decision{Outcome: OutcomeApplied, Best: &base, Candidates: []Candidate{base}}, current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, want: DispositionAllow},
		{name: "review below cap", decision: Decision{Outcome: OutcomeBelowCap, Best: &Candidate{Selector: base.Selector, Fingerprint: base.Fingerprint, Score: 0.7}, Candidates: []Candidate{{Selector: base.Selector, Fingerprint: base.Fingerprint, Score: 0.7}}, NeedsReview: true}, current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, want: DispositionReview, wantReason: ReasonBelowCap},
		{name: "block no candidate", decision: Decision{Outcome: OutcomeNoCandidate}, current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, want: DispositionBlock, wantReason: ReasonNoCandidate},
		{name: "block role mismatch", decision: Decision{Outcome: OutcomeApplied, Best: &Candidate{Selector: base.Selector, Fingerprint: fingerprint.Fingerprint{Tag: "button", ARIA: fingerprint.ARIA{Role: "link"}}, Score: 0.99}, Candidates: []Candidate{{Selector: base.Selector, Fingerprint: fingerprint.Fingerprint{Tag: "button", ARIA: fingerprint.ARIA{Role: "link"}}, Score: 0.99}}}, current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, want: DispositionBlock, wantReason: ReasonRoleMismatch},
		{name: "review page mismatch", decision: Decision{Outcome: OutcomeApplied, Best: &base, Candidates: []Candidate{base}}, current: ExecutionContext{Origin: target.Origin, PageURL: "https://shop.test/account"}, want: DispositionReview, wantReason: ReasonPageMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Assess(target, tc.decision, tc.current, tc.policy)
			if err != nil {
				t.Fatal(err)
			}
			if got.Disposition != tc.want {
				t.Fatalf("disposition=%s want=%s explanation=%s", got.Disposition, tc.want, got.Explanation)
			}
			if tc.wantReason != "" && (len(got.Reasons) == 0 || got.Reasons[0] != tc.wantReason) {
				t.Fatalf("reasons=%v want=%s", got.Reasons, tc.wantReason)
			}
		})
	}
}
