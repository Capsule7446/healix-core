package heal

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func directCandidate(value string, score float64) Candidate {
	return Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: value}, Score: score}
}

func TestThresholdsValidateDirectBoundaries(t *testing.T) {
	tests := []struct {
		name string
		in   Thresholds
		want bool
	}{
		{"zero review", Thresholds{ReviewCap: 0, AppliedCap: 1}, true},
		{"one applied", Thresholds{ReviewCap: 0, AppliedCap: 1}, true},
		{"equal zero", Thresholds{}, false},
		{"negative review", Thresholds{ReviewCap: -1, AppliedCap: 1}, false},
		{"negative applied", Thresholds{ReviewCap: 0, AppliedCap: -1}, false},
		{"review one", Thresholds{ReviewCap: 1, AppliedCap: 1}, false},
		{"review NaN", Thresholds{ReviewCap: math.NaN(), AppliedCap: 1}, false},
		{"applied NaN", Thresholds{ReviewCap: 0, AppliedCap: math.NaN()}, false},
		{"review positive infinity", Thresholds{ReviewCap: math.Inf(1), AppliedCap: 1}, false},
		{"applied negative infinity", Thresholds{ReviewCap: 0, AppliedCap: math.Inf(-1)}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.in.Validate()
			if (err == nil) != test.want {
				t.Fatalf("Validate() error = %v, valid = %v", err, test.want)
			}
		})
	}
}

func TestWeightsValidateDirectBoundariesAndPrecedence(t *testing.T) {
	valid := []Weights{{Tag: 1}, {Tag: math.SmallestNonzeroFloat64}, {Tag: math.MaxFloat64}}
	for _, weights := range valid {
		if err := weights.Validate(); err != nil {
			t.Fatalf("Validate(%+v): %v", weights, err)
		}
	}
	invalid := []struct {
		name, want string
		weights    Weights
	}{
		{"all zero", "at least one", Weights{}},
		{"negative", "tag weight", Weights{Tag: -1, ID: 1}},
		{"NaN", "id weight", Weights{Tag: 1, ID: math.NaN()}},
		{"positive infinity", "role_name weight", Weights{Tag: 1, RoleName: math.Inf(1)}},
		{"negative infinity", "class weight", Weights{Tag: 1, Class: math.Inf(-1)}},
		{"field error precedes overflowing sum", "tag weight", Weights{Tag: -1, ID: math.MaxFloat64, RoleName: math.MaxFloat64}},
		{"overflowing finite sum", "sum of weights", Weights{Tag: math.MaxFloat64, ID: math.MaxFloat64}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			err := test.weights.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPolicyV1ValidateDirectPrecedence(t *testing.T) {
	tests := []struct {
		name string
		in   PolicyV1
		want string
	}{
		{"valid", DefaultPolicyV1(), ""},
		{"version minus one", PolicyV1{Version: -1}, "unsupported policy version"},
		{"version zero", PolicyV1{}, "unsupported policy version"},
		{"version max int", PolicyV1{Version: int(^uint(0) >> 1)}, "unsupported policy version"},
		{"threshold before weights", PolicyV1{Version: 1, Thresholds: Thresholds{}, Weights: Weights{}}, "review_cap"},
		{"weights after valid thresholds", PolicyV1{Version: 1, Thresholds: Thresholds{ReviewCap: 0, AppliedCap: 1}}, "at least one weight"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.in.Validate()
			if test.want == "" && err != nil || test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDecisionValidateDirectMalformedShapesAndScores(t *testing.T) {
	good := directCandidate("#a", 1)
	valid := []Decision{
		{Outcome: OutcomeApplied, Best: &good, Candidates: []Candidate{good}},
		{Outcome: OutcomeBelowCap, Best: &good, Candidates: []Candidate{good}, NeedsReview: true},
		{Outcome: OutcomeSafetyRejected, Best: &good, Candidates: []Candidate{good}},
		{Outcome: OutcomeNoCandidate},
	}
	for _, decision := range valid {
		if err := decision.Validate(); err != nil {
			t.Fatalf("valid decision %+v: %v", decision, err)
		}
	}
	for _, score := range []float64{-1, math.NaN(), math.Inf(-1), math.Inf(1), 1.000001} {
		candidate := directCandidate("#a", score)
		decision := Decision{Outcome: OutcomeApplied, Best: &candidate, Candidates: []Candidate{candidate}}
		if err := decision.Validate(); err == nil {
			t.Fatalf("score %v accepted", score)
		}
	}
	invalid := []Decision{
		{Outcome: OutcomeSafetyRejected},
		{Outcome: OutcomeSafetyRejected, Best: &good, Candidates: []Candidate{good}, NeedsReview: true},
		{Outcome: OutcomeApplied, Best: &good, Candidates: []Candidate{good, directCandidate("#0", 1)}},
	}
	for _, decision := range invalid {
		if err := decision.Validate(); err == nil {
			t.Fatalf("malformed decision accepted: %+v", decision)
		}
	}
}

func TestAssessDirectMarginBoundariesAndErrorPrecedence(t *testing.T) {
	target := assessmentSpec()
	first := Candidate{Selector: target.Selectors[0], Fingerprint: target.Fingerprint, Score: 1}
	second := first
	second.Selector.Value = "#second"
	second.Score = 0
	decision := Decision{Outcome: OutcomeApplied, Best: &first, Candidates: []Candidate{first, second}}

	for _, test := range []struct {
		name   string
		margin float64
		want   Disposition
	}{
		{"negative defaults", -1, DispositionAllow},
		{"zero defaults", 0, DispositionAllow},
		{"difference equals one", 1, DispositionAllow},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Assess(target, decision, ExecutionContext{Origin: target.Origin, PageURL: target.PageURL}, SafetyPolicy{MinimumMargin: test.margin})
			if err != nil || got.Disposition != test.want {
				t.Fatalf("Assess() = %+v, %v", got, err)
			}
		})
	}
	for _, margin := range []float64{math.NaN(), math.Inf(-1), math.Inf(1)} {
		if _, err := Assess(target, decision, ExecutionContext{}, SafetyPolicy{MinimumMargin: margin}); err == nil {
			t.Fatalf("invalid minimum margin %v accepted", margin)
		}
	}
	badDecision := Decision{Outcome: "malformed"}
	_, err := Assess(target, badDecision, ExecutionContext{}, SafetyPolicy{MinimumMargin: math.NaN()})
	if err == nil || !strings.Contains(err.Error(), "unknown decision outcome") {
		t.Fatalf("decision validation must precede policy validation: %v", err)
	}
}

func TestDefaultHealerHealDirectErrorPrecedenceAndCollections(t *testing.T) {
	invalid := &DefaultHealer{}
	if _, err := invalid.Heal(context.Background(), fingerprint.NodeSpec{}, nil); err == nil || !strings.Contains(err.Error(), "review_cap") {
		t.Fatalf("configuration must precede nil snapshot: %v", err)
	}
	healer := NewDefaultHealer()
	for name, candidates := range map[string][]SnapshotCandidate{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			decision, err := healer.Heal(context.Background(), fingerprint.NodeSpec{}, fakeSnapshot{candidates: candidates})
			if err != nil || decision.Outcome != OutcomeNoCandidate || decision.Candidates != nil {
				t.Fatalf("Heal() = %+v, %v", decision, err)
			}
		})
	}
	candidate := SnapshotCandidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#x"}}
	decision, err := healer.Heal(context.Background(), fingerprint.NodeSpec{}, fakeSnapshot{candidates: []SnapshotCandidate{candidate, candidate}})
	if err != nil || len(decision.Candidates) != 2 {
		t.Fatalf("duplicate snapshot candidates = %+v, %v", decision, err)
	}
}

func TestCandidateHashDirectStabilityAndFieldSensitivity(t *testing.T) {
	base := Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "a\x00雪😀'", Priority: 1}, Fingerprint: fingerprint.Fingerprint{Tag: "a", Attributes: map[string]string{"data-x": "雪😀'"}}, Score: 0}
	if got := CandidateHash(base); len(got) != 64 {
		t.Fatalf("hash length = %d, hash = %q", len(got), got)
	}
	changedScore := base
	changedScore.Score = math.NaN()
	if CandidateHash(base) != CandidateHash(changedScore) {
		t.Fatal("score must not affect candidate identity")
	}
	for _, mutate := range []func(*Candidate){
		func(c *Candidate) { c.Selector.Type = fingerprint.SelectorXPath },
		func(c *Candidate) { c.Selector.Value += "x" },
		func(c *Candidate) { c.Selector.Priority = 0 },
		func(c *Candidate) { c.Fingerprint.Tag = "button" },
	} {
		changed := base
		mutate(&changed)
		if CandidateHash(base) == CandidateHash(changed) {
			t.Fatal("identity field change did not affect hash")
		}
	}
}

func TestValidateSamplesDirectMalformedAndDuplicateCollections(t *testing.T) {
	valid := CandidateSample{CandidateHash: "candidate", FingerprintHash: "fingerprint", Score: 0, Rank: 1, Eligible: true, Selected: true, Status: CandidateSampleSelected}
	if err := ValidateSamples(nil); err != nil {
		t.Fatalf("nil: %v", err)
	}
	if err := ValidateSamples([]CandidateSample{}); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if err := ValidateSamples([]CandidateSample{valid}); err != nil {
		t.Fatalf("single: %v", err)
	}
	for _, mutate := range []func(*CandidateSample){
		func(s *CandidateSample) { s.Rank = -1 },
		func(s *CandidateSample) { s.Rank = 0 },
		func(s *CandidateSample) { s.Rank = int(^uint(0) >> 1) },
		func(s *CandidateSample) { s.CandidateHash = "" },
		func(s *CandidateSample) { s.FingerprintHash = "" },
		func(s *CandidateSample) { s.Score = -1 },
		func(s *CandidateSample) { s.Score = math.NaN() },
		func(s *CandidateSample) { s.Score = math.Inf(1) },
		func(s *CandidateSample) { s.Score = 1.1 },
	} {
		bad := valid
		mutate(&bad)
		if err := ValidateSamples([]CandidateSample{bad}); err == nil {
			t.Fatalf("malformed sample accepted: %+v", bad)
		}
	}
	duplicate := valid
	duplicate.Rank = 2
	duplicate.Selected = false
	duplicate.Status = CandidateSampleEligible
	if err := ValidateSamples([]CandidateSample{valid, duplicate}); err != nil {
		t.Fatalf("duplicate candidate samples should remain valid: %v", err)
	}
	secondSelected := duplicate
	secondSelected.Selected = true
	secondSelected.Status = CandidateSampleSelected
	if err := ValidateSamples([]CandidateSample{valid, secondSelected}); err == nil {
		t.Fatal("multiple selected samples accepted")
	}
}

func TestSortSamplesDirectStableOrderingAndDeepOwnership(t *testing.T) {
	nilOut := SortSamples(nil)
	if nilOut != nil {
		t.Fatalf("SortSamples(nil) = %#v", nilOut)
	}
	a := CandidateSample{CandidateHash: "a", Rank: 2, Evidence: []CandidateEvidence{{Dimension: "a"}}}
	b := CandidateSample{CandidateHash: "b", Rank: 1, Evidence: []CandidateEvidence{{Dimension: "b"}}}
	c := CandidateSample{CandidateHash: "c", Rank: 2, Evidence: []CandidateEvidence{{Dimension: "c"}}}
	in := []CandidateSample{a, b, c}
	out := SortSamples(in)
	if got := []string{out[0].CandidateHash, out[1].CandidateHash, out[2].CandidateHash}; !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("stable order = %v", got)
	}
	out[0].CandidateHash = "changed"
	out[1].Evidence[0].Dimension = "changed"
	if in[1].CandidateHash != "b" || in[0].Evidence[0].Dimension != "a" {
		t.Fatalf("output aliases input: in=%+v out=%+v", in, out)
	}
}
