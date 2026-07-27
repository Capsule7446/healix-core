package heal

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func publicMatrixCandidate(selector string, score float64) Candidate {
	return Candidate{
		Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: selector},
		Fingerprint: fingerprint.Fingerprint{
			Tag: "button", Attributes: map[string]string{"id": "submit"},
			ARIA: fingerprint.ARIA{Role: "button"}, FormID: "checkout",
		},
		Score: score,
	}
}

func TestDecisionValidateOutcomeAndOrderingMatrix(t *testing.T) {
	best := publicMatrixCandidate("#best", 0.9)
	second := publicMatrixCandidate("#second", 0.8)
	validSafetyRejected := Decision{Outcome: OutcomeSafetyRejected, Best: &best, Candidates: []Candidate{best}}
	if err := validSafetyRejected.Validate(); err != nil {
		t.Fatalf("valid safety rejection: %v", err)
	}

	tests := []struct {
		name     string
		decision Decision
		want     string
	}{
		{name: "below cap missing best", decision: Decision{Outcome: OutcomeBelowCap, NeedsReview: true}, want: "requires a best"},
		{name: "safety rejection missing best", decision: Decision{Outcome: OutcomeSafetyRejected}, want: "requires a best"},
		{name: "safety rejection cannot require review", decision: Decision{Outcome: OutcomeSafetyRejected, Best: &best, Candidates: []Candidate{best}, NeedsReview: true}, want: "cannot require review"},
		{name: "no candidate cannot require review", decision: Decision{Outcome: OutcomeNoCandidate, NeedsReview: true}, want: "cannot require review"},
		{name: "best requires candidates", decision: Decision{Outcome: OutcomeApplied, Best: &best}, want: "non-empty"},
		{name: "best must equal first", decision: Decision{Outcome: OutcomeApplied, Best: &best, Candidates: []Candidate{second, best}}, want: "deterministic"},
		{name: "candidates must be ordered", decision: Decision{Outcome: OutcomeApplied, Best: &best, Candidates: []Candidate{best, publicMatrixCandidate("#higher", 0.95)}}, want: "deterministic"},
		{name: "invalid best fingerprint", decision: func() Decision {
			invalid := best
			invalid.Fingerprint.Tag = ""
			return Decision{Outcome: OutcomeApplied, Best: &invalid, Candidates: []Candidate{invalid}}
		}(), want: "invalid fingerprint"},
		{name: "invalid later candidate score", decision: func() Decision {
			invalid := second
			invalid.Score = math.NaN()
			return Decision{Outcome: OutcomeApplied, Best: &best, Candidates: []Candidate{best, invalid}}
		}(), want: "score must be finite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.decision.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestAssessCoversSafetyRulesAndURLNormalization(t *testing.T) {
	target := assessmentSpec()
	target.Fingerprint.FormID = "checkout"
	base := publicMatrixCandidate("#submit", 0.9)
	base.Fingerprint = target.Fingerprint
	decision := func(candidate Candidate) Decision {
		return Decision{Outcome: OutcomeApplied, Best: &candidate, Candidates: []Candidate{candidate}}
	}

	tests := []struct {
		name       string
		target     fingerprint.NodeSpec
		candidate  Candidate
		current    ExecutionContext
		want       Disposition
		wantReason ReasonCode
	}{
		{
			name: "scheme host case and fragment are normalized",
			target: func() fingerprint.NodeSpec {
				value := target
				value.PageURL = "HTTPS://SHOP.TEST/checkout#first"
				return value
			}(),
			candidate: base,
			current:   ExecutionContext{Origin: target.Origin, PageURL: "https://shop.test/checkout#second"},
			want:      DispositionAllow,
		},
		{
			name:   "tag mismatch blocks",
			target: target,
			candidate: func() Candidate {
				value := base
				value.Fingerprint.Tag = "a"
				return value
			}(),
			current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL},
			want:    DispositionBlock, wantReason: ReasonTagMismatch,
		},
		{
			name:   "form mismatch blocks",
			target: target,
			candidate: func() Candidate {
				value := base
				value.Fingerprint.FormID = "signup"
				return value
			}(),
			current: ExecutionContext{Origin: target.Origin, PageURL: target.PageURL},
			want:    DispositionBlock, wantReason: ReasonFormMismatch,
		},
		{
			name: "malformed URLs compare as opaque values",
			target: func() fingerprint.NodeSpec {
				value := target
				value.PageURL = "%"
				value.Origin = ""
				return value
			}(),
			candidate: base,
			current:   ExecutionContext{PageURL: "%bad"},
			want:      DispositionReview, wantReason: ReasonPageMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Assess(test.target, decision(test.candidate), test.current, SafetyPolicy{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Disposition != test.want {
				t.Fatalf("disposition = %q, want %q; assessment=%#v", got.Disposition, test.want, got)
			}
			if test.wantReason != "" && !containsReason(got.Reasons, test.wantReason) {
				t.Fatalf("reasons = %v, want %q", got.Reasons, test.wantReason)
			}
		})
	}
}

func TestAssessAmbiguityMarginBusinessBoundaryAndPrecedence(t *testing.T) {
	target := assessmentSpec()
	first := publicMatrixCandidate("#first", 0.875)
	first.Fingerprint = target.Fingerprint
	second := publicMatrixCandidate("#second", 0.75)
	second.Fingerprint = target.Fingerprint
	decision := Decision{Outcome: OutcomeApplied, Best: &first, Candidates: []Candidate{first, second}}

	for _, test := range []struct {
		name   string
		margin float64
		page   string
		want   []ReasonCode
	}{
		{name: "exact margin is allowed", margin: 0.125, page: target.PageURL},
		{name: "inside margin needs review", margin: math.Nextafter(0.125, 1), page: target.PageURL, want: []ReasonCode{ReasonAmbiguous}},
		{name: "existing page review takes precedence", margin: 1, page: "https://shop.test/other", want: []ReasonCode{ReasonPageMismatch}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Assess(target, decision, ExecutionContext{Origin: target.Origin, PageURL: test.page}, SafetyPolicy{MinimumMargin: test.margin})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Reasons, test.want) {
				t.Fatalf("reasons = %v, want %v", got.Reasons, test.want)
			}
		})
	}

	if _, err := Assess(target, Decision{Outcome: "invalid"}, ExecutionContext{}, SafetyPolicy{}); err == nil {
		t.Fatal("Assess accepted an invalid decision")
	}
}

func containsReason(reasons []ReasonCode, want ReasonCode) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func TestPolicyV1ValidationPropagatesThresholdAndWeightRules(t *testing.T) {
	invalidThresholds := DefaultPolicyV1()
	invalidThresholds.Thresholds.ReviewCap = invalidThresholds.Thresholds.AppliedCap
	if err := invalidThresholds.Validate(); err == nil || !strings.Contains(err.Error(), "review_cap") {
		t.Fatalf("threshold error = %v", err)
	}

	invalidWeights := DefaultPolicyV1()
	invalidWeights.Weights = Weights{}
	if err := invalidWeights.Validate(); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("weight error = %v", err)
	}
}

func TestWeightsValidateRejectsFiniteValuesWhoseSumOverflows(t *testing.T) {
	weights := Weights{Tag: math.MaxFloat64, ID: math.MaxFloat64}
	if err := weights.Validate(); err == nil || !strings.Contains(err.Error(), "sum of weights") {
		t.Fatalf("overflowing weight sum error = %v", err)
	}
}

func TestDecisionSamplesReviewCapBoundaryAndInvalidInputs(t *testing.T) {
	candidate := publicMatrixCandidate("#candidate", 1)
	decision := Decision{Outcome: OutcomeApplied, Best: &candidate, Candidates: []Candidate{candidate}}
	for _, cap := range []float64{0, 1} {
		samples := decision.Samples(candidate.Fingerprint, cap)
		if len(samples) != 1 || !samples[0].Eligible || !samples[0].Selected || samples[0].Status != CandidateSampleSelected {
			t.Fatalf("Samples(reviewCap=%v) = %#v", cap, samples)
		}
	}
	for _, cap := range []float64{math.Nextafter(0, -1), math.Nextafter(1, 2), math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := decision.Samples(candidate.Fingerprint, cap); got != nil {
			t.Fatalf("Samples(reviewCap=%v) = %#v, want nil", cap, got)
		}
	}
}

func TestValidateSamplesRejectsMalformedRanksScoresAndSelection(t *testing.T) {
	valid := CandidateSample{
		CandidateHash: "candidate", FingerprintHash: "fingerprint", Score: 0.9,
		Rank: 1, Eligible: true, Selected: true, Status: CandidateSampleSelected,
	}
	tests := []struct {
		name   string
		mutate func(*[]CandidateSample)
	}{
		{name: "rank gap", mutate: func(samples *[]CandidateSample) { (*samples)[0].Rank = 2 }},
		{name: "missing candidate hash", mutate: func(samples *[]CandidateSample) { (*samples)[0].CandidateHash = "" }},
		{name: "missing fingerprint hash", mutate: func(samples *[]CandidateSample) { (*samples)[0].FingerprintHash = "" }},
		{name: "negative score", mutate: func(samples *[]CandidateSample) { (*samples)[0].Score = math.Nextafter(0, -1) }},
		{name: "above one score", mutate: func(samples *[]CandidateSample) { (*samples)[0].Score = math.Nextafter(1, 2) }},
		{name: "nan score", mutate: func(samples *[]CandidateSample) { (*samples)[0].Score = math.NaN() }},
		{name: "infinite score", mutate: func(samples *[]CandidateSample) { (*samples)[0].Score = math.Inf(1) }},
		{name: "selected below eligibility", mutate: func(samples *[]CandidateSample) { (*samples)[0].Eligible = false }},
		{name: "multiple selected", mutate: func(samples *[]CandidateSample) {
			second := valid
			second.Rank = 2
			*samples = append(*samples, second)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			samples := []CandidateSample{valid}
			test.mutate(&samples)
			if err := ValidateSamples(samples); err == nil {
				t.Fatalf("ValidateSamples accepted %#v", samples)
			}
		})
	}
}

func TestValidateSamplesRejectsStatusFlagContradictions(t *testing.T) {
	valid := CandidateSample{
		CandidateHash: "candidate", FingerprintHash: "fingerprint", Score: 0.9,
		Rank: 1, Eligible: true, Selected: true, Status: CandidateSampleSelected,
	}
	tests := []struct {
		name   string
		mutate func(*CandidateSample)
	}{
		{name: "unknown status", mutate: func(sample *CandidateSample) { sample.Status = "unknown" }},
		{name: "selected flag without selected status", mutate: func(sample *CandidateSample) { sample.Status = CandidateSampleEligible }},
		{name: "selected status without selected flag", mutate: func(sample *CandidateSample) { sample.Selected = false }},
		{name: "eligible status on ineligible sample", mutate: func(sample *CandidateSample) {
			sample.Selected = false
			sample.Eligible = false
			sample.Status = CandidateSampleEligible
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sample := valid
			test.mutate(&sample)
			if err := ValidateSamples([]CandidateSample{sample}); err == nil {
				t.Fatalf("ValidateSamples accepted contradictory status: %#v", sample)
			}
		})
	}
}

func TestSortSamplesOrdersAndDeepCopiesEvidence(t *testing.T) {
	input := []CandidateSample{
		{CandidateHash: "second", Rank: 2, Evidence: []CandidateEvidence{{Dimension: "tag", Score: 1, Matched: true}}},
		{CandidateHash: "first", Rank: 1, Evidence: []CandidateEvidence{{Dimension: "id", Score: 1, Matched: true}}},
	}
	original := []CandidateSample{
		{CandidateHash: "second", Rank: 2, Evidence: []CandidateEvidence{{Dimension: "tag", Score: 1, Matched: true}}},
		{CandidateHash: "first", Rank: 1, Evidence: []CandidateEvidence{{Dimension: "id", Score: 1, Matched: true}}},
	}
	got := SortSamples(input)
	if len(got) != 2 || got[0].CandidateHash != "first" || got[1].CandidateHash != "second" {
		t.Fatalf("SortSamples() = %#v", got)
	}
	got[0].Evidence[0].Dimension = "mutated"
	got[1].CandidateHash = "mutated"
	if !reflect.DeepEqual(input, original) {
		t.Fatalf("sorted samples alias input: input=%#v original=%#v", input, original)
	}
	if got := SortSamples(nil); got != nil {
		t.Fatalf("SortSamples(nil) = %#v, want nil", got)
	}
}
