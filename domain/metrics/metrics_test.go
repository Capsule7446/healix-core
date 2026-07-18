package metrics

import (
	"math"
	"strings"
	"testing"
	"time"
)

func testPolicy(t *testing.T, textWeight float64) (PolicySnapshot, PolicyFingerprint) {
	t.Helper()
	policy := PolicySnapshot{Version: 1, ReviewCap: .6, AppliedCap: .85,
		Weights: PolicyWeights{Tag: .15, ID: .2, RoleName: .2, Text: textWeight}}
	fingerprint, err := FingerprintPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	return policy, fingerprint
}

func TestFingerprintPolicyIsStableAndIncludesEveryDecisionSetting(t *testing.T) {
	base := PolicySnapshot{Version: 1, ReviewCap: .6, AppliedCap: .85,
		Weights: PolicyWeights{Tag: .15, ID: .2, RoleName: .2, Class: .1, Attrs: .1,
			Text: .1, Index: .05, Neighbor: .1, LabelText: .15, Container: .1}}
	first, err := FingerprintPolicy(base)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := FingerprintPolicy(base)
	if first != second || len(first) != 64 {
		t.Fatalf("unstable fingerprint: %q %q", first, second)
	}
	changed := base
	changed.Weights.Container = .11
	third, _ := FingerprintPolicy(changed)
	if third == first {
		t.Fatal("changing one weight did not change the policy fingerprint")
	}
	base.ReviewCap = math.NaN()
	if _, err := FingerprintPolicy(base); err == nil {
		t.Fatal("non-finite policy was accepted")
	}
	base.ReviewCap = 0
	base.AppliedCap = math.Inf(1)
	if _, err := FingerprintPolicy(base); err == nil {
		t.Fatal("infinite policy was accepted")
	}
	base.AppliedCap = 0
	base.Version = 0
	if _, err := FingerprintPolicy(base); err == nil {
		t.Fatal("non-positive policy version was accepted")
	}
}

func TestFingerprintPolicyCanonicalizesNegativeZero(t *testing.T) {
	positive, err := FingerprintPolicy(PolicySnapshot{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	negativeZero := math.Copysign(0, -1)
	negative, err := FingerprintPolicy(PolicySnapshot{Version: 1, ReviewCap: negativeZero})
	if err != nil {
		t.Fatal(err)
	}
	if positive != negative {
		t.Fatalf("zero fingerprints differ: %s != %s", positive, negative)
	}
}

func TestProjectClassifiesBandsNoCandidateAndUnknownLegacy(t *testing.T) {
	policy, fingerprint := testPolicy(t, .1)
	from := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC).UnixMilli()
	report, err := Project(Query{FromMS: from, ThroughMS: from + 24*60*60*1000}, []ObservationFact{
		{ObservationID: "applied-pass", ObservedAtMS: from, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandApplied, CandidateHash: "a", Succeeded: true},
		{ObservationID: "applied-fail", ObservedAtMS: from + 1, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandApplied, CandidateHash: "b"},
		{ObservationID: "review-pass", ObservedAtMS: from + 2, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandBelowCap, CandidateHash: "c", Succeeded: true},
		{ObservationID: "no-candidate", ObservedAtMS: from + 3, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandUnknown},
		{ObservationID: "legacy", ObservedAtMS: from + 4, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandUnknown, CandidateHash: "old"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Buckets) != 3 {
		t.Fatalf("buckets = %#v", report.Buckets)
	}
	byBand := map[DecisionBand]Bucket{}
	for _, bucket := range report.Buckets {
		byBand[bucket.DecisionBand] = bucket
	}
	applied := byBand[DecisionBandApplied]
	if applied.AttemptCount != 2 || applied.CandidateSelected != 2 || applied.AppliedSuccess != 1 || applied.AppliedFailure != 1 {
		t.Fatalf("applied = %#v", applied)
	}
	below := byBand[DecisionBandBelowCap]
	if below.AttemptCount != 1 || below.AppliedSuccess != 1 {
		t.Fatalf("below cap = %#v", below)
	}
	unknown := byBand[DecisionBandUnknown]
	if unknown.AttemptCount != 2 || unknown.NoCandidate != 1 || unknown.UnknownLegacy != 1 || unknown.ClassifiedAttempts() != 1 {
		t.Fatalf("unknown = %#v", unknown)
	}
	if applied.Policy != policy || unknown.Policy != policy {
		t.Fatalf("bucket lost its frozen policy: applied=%#v unknown=%#v", applied.Policy, unknown.Policy)
	}
}

func TestProjectUsesUTCDateAndExclusiveRange(t *testing.T) {
	policy, fingerprint := testPolicy(t, .1)
	from := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC).UnixMilli()
	through := from + 2*24*60*60*1000
	report, err := Project(Query{FromMS: from, ThroughMS: through}, []ObservationFact{
		{ObservationID: "before", ObservedAtMS: from - 1, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandApplied, CandidateHash: "a"},
		{ObservationID: "day-one", ObservedAtMS: from + 24*60*60*1000 - 1, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandApplied, CandidateHash: "b"},
		{ObservationID: "day-two", ObservedAtMS: from + 24*60*60*1000, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandApplied, CandidateHash: "c"},
		{ObservationID: "through", ObservedAtMS: through, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandApplied, CandidateHash: "d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Buckets) != 2 || report.Buckets[0].Day != "2026-07-17" || report.Buckets[1].Day != "2026-07-16" {
		t.Fatalf("UTC buckets = %#v", report.Buckets)
	}
}

func TestProjectSeparatesPoliciesAndSupportsPolicyFilter(t *testing.T) {
	firstPolicy, first := testPolicy(t, .1)
	secondPolicy, second := testPolicy(t, .2)
	query := Query{FromMS: 1, ThroughMS: 100}
	facts := []ObservationFact{
		{ObservationID: "first", ObservedAtMS: 2, Policy: firstPolicy, PolicyFingerprint: first, DecisionBand: DecisionBandApplied, CandidateHash: "a"},
		{ObservationID: "second", ObservedAtMS: 3, Policy: secondPolicy, PolicyFingerprint: second, DecisionBand: DecisionBandApplied, CandidateHash: "b"},
	}
	report, err := Project(query, facts)
	if err != nil || len(report.Buckets) != 2 {
		t.Fatalf("separate policies: report=%#v err=%v", report, err)
	}
	query.PolicyFingerprint = second
	report, err = Project(query, facts)
	if err != nil || len(report.Buckets) != 1 || report.Buckets[0].PolicyFingerprint != second {
		t.Fatalf("filtered policy: report=%#v err=%v", report, err)
	}
}

func TestProjectRejectsInvalidFactsAndDuplicates(t *testing.T) {
	policy, fingerprint := testPolicy(t, .1)
	valid := ObservationFact{ObservationID: "same", ObservedAtMS: 2, Policy: policy, PolicyFingerprint: fingerprint,
		DecisionBand: DecisionBandApplied, CandidateHash: "a"}
	for name, facts := range map[string][]ObservationFact{
		"missing id":        {{ObservedAtMS: 2, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: DecisionBandApplied}},
		"bad fingerprint":   {{ObservationID: "bad", ObservedAtMS: 2, Policy: policy, PolicyFingerprint: "bad", DecisionBand: DecisionBandApplied}},
		"mismatched policy": {{ObservationID: "bad", ObservedAtMS: 2, Policy: policy, PolicyFingerprint: PolicyFingerprint(strings.Repeat("0", 64)), DecisionBand: DecisionBandApplied}},
		"bad band":          {{ObservationID: "bad", ObservedAtMS: 2, Policy: policy, PolicyFingerprint: fingerprint, DecisionBand: "MAYBE"}},
		"duplicate fact id": {valid, valid},
	} {
		if _, err := Project(Query{FromMS: 1, ThroughMS: 10}, facts); err == nil {
			t.Fatalf("%s unexpectedly passed", name)
		}
	}
}

func TestQueryValidationAndRates(t *testing.T) {
	for _, query := range []Query{{FromMS: -1, ThroughMS: 1}, {FromMS: 1, ThroughMS: 1}, {FromMS: 2, ThroughMS: 1}} {
		if err := query.Validate(); err == nil {
			t.Fatalf("invalid query passed: %#v", query)
		}
	}
	if Rate(1, 4) != .25 || Rate(1, 0) != 0 || Rate(-1, 4) != 0 {
		t.Fatal("rate zero/positive denominator policy is incorrect")
	}
	bucket := Bucket{AttemptCount: 5, CandidateSelected: 3, AppliedSuccess: 2, AppliedFailure: 1,
		NoCandidate: 1, UnknownLegacy: 1}
	if bucket.CandidateSelectionRate() != .75 || bucket.AppliedSuccessRate() != 2.0/3.0 ||
		bucket.AppliedFailureRate() != 1.0/3.0 || bucket.NoCandidateRate() != .25 {
		t.Fatalf("bucket rates are incorrect: %#v", bucket)
	}
	if err := (PolicyFingerprint(strings.Repeat("z", 64))).Validate(); err == nil {
		t.Fatal("non-hex policy fingerprint passed")
	}
	if err := (PolicyFingerprint("abc")).Validate(); err == nil {
		t.Fatal("short policy fingerprint passed")
	}
	if err := (Query{FromMS: 1, ThroughMS: 2, PolicyFingerprint: "bad"}).Validate(); err == nil {
		t.Fatal("query accepted an invalid policy fingerprint")
	}
	if got := (Bucket{AttemptCount: 1, UnknownLegacy: 2}).ClassifiedAttempts(); got != 0 {
		t.Fatalf("negative classified attempts were not clamped: %d", got)
	}
}
