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
		{name: "allow safe below cap", decision: Decision{Outcome: OutcomeBelowCap, Best: &Candidate{Selector: base.Selector, Fingerprint: base.Fingerprint, Score: 0.7}, Candidates: []Candidate{{Selector: base.Selector, Fingerprint: base.Fingerprint, Score: 0.7}}, NeedsReview: true}, current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, want: DispositionAllow},
		{name: "block no candidate", decision: Decision{Outcome: OutcomeNoCandidate}, current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, want: DispositionBlock, wantReason: ReasonNoCandidate},
		{name: "block role mismatch", decision: Decision{Outcome: OutcomeApplied, Best: &Candidate{Selector: base.Selector, Fingerprint: fingerprint.Fingerprint{Tag: "button", ARIA: fingerprint.ARIA{Role: "link"}}, Score: 0.99}, Candidates: []Candidate{{Selector: base.Selector, Fingerprint: fingerprint.Fingerprint{Tag: "button", ARIA: fingerprint.ARIA{Role: "link"}}, Score: 0.99}}}, current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, want: DispositionBlock, wantReason: ReasonRoleMismatch},
		{name: "review page mismatch", decision: Decision{Outcome: OutcomeApplied, Best: &base, Candidates: []Candidate{base}}, current: ExecutionContext{Origin: target.Origin, PageURL: "https://shop.test/account"}, want: DispositionReview, wantReason: ReasonPageMismatch},
		// An unknown current origin is the shape a caller with no live page
		// port produces. It must block, not fall through: the check exists
		// for the case where the page moved, and a page that moved somewhere
		// unreportable is not safer than one that moved somewhere named.
		{name: "block unknown origin", decision: Decision{Outcome: OutcomeApplied, Best: &base, Candidates: []Candidate{base}}, current: ExecutionContext{PageURL: target.PageURL}, want: DispositionBlock, wantReason: ReasonOriginUnknown},
		{name: "block unknown origin and page", decision: Decision{Outcome: OutcomeApplied, Best: &base, Candidates: []Candidate{base}}, current: ExecutionContext{}, want: DispositionBlock, wantReason: ReasonOriginUnknown},
		{name: "block origin mismatch outranks page", decision: Decision{Outcome: OutcomeApplied, Best: &base, Candidates: []Candidate{base}}, current: ExecutionContext{Origin: "https://evil.test", PageURL: "https://evil.test/account"}, want: DispositionBlock, wantReason: ReasonOriginMismatch},
		// The page URL is the weaker signal — same-origin navigation is
		// ordinary — so an unconfirmed page downgrades rather than blocks.
		{name: "review unknown page on a confirmed origin", decision: Decision{Outcome: OutcomeApplied, Best: &base, Candidates: []Candidate{base}}, current: ExecutionContext{Origin: target.Origin}, want: DispositionReview, wantReason: ReasonPageUnknown},
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
