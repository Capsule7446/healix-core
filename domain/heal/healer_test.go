package heal

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestThresholdsValidate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		thresholds Thresholds
	}{
		{name: "review above applied", thresholds: Thresholds{ReviewCap: 0.9, AppliedCap: 0.8}},
		{name: "review equals applied", thresholds: Thresholds{ReviewCap: 0.8, AppliedCap: 0.8}},
		{name: "review below zero", thresholds: Thresholds{ReviewCap: -0.1, AppliedCap: 0.8}},
		{name: "applied above one", thresholds: Thresholds{ReviewCap: 0.6, AppliedCap: 1.1}},
		{name: "NaN", thresholds: Thresholds{ReviewCap: math.NaN(), AppliedCap: 0.8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.thresholds.Validate(); err == nil {
				t.Fatal("Validate should reject invalid thresholds")
			}
		})
	}
	if err := DefaultThresholds().Validate(); err != nil {
		t.Fatalf("DefaultThresholds.Validate: %v", err)
	}
}

func TestPolicyV1DefaultsAndValidation(t *testing.T) {
	policy := DefaultPolicyV1()
	if policy.Version != PolicyVersionV1 || policy.Thresholds != DefaultThresholds() || policy.Weights != DefaultWeights() {
		t.Fatalf("DefaultPolicyV1 = %+v", policy)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("default policy: %v", err)
	}
	policy.Version++
	if err := policy.Validate(); err == nil {
		t.Fatal("unknown policy version should fail")
	}
	mutated := DefaultPolicyV1()
	mutated.Weights.Text = 99
	mutated.Thresholds.ReviewCap = 0
	fresh := DefaultPolicyV1()
	if fresh.Weights != DefaultWeights() || fresh.Thresholds != DefaultThresholds() {
		t.Fatalf("default policy was contaminated by a caller mutation: %+v", fresh)
	}
}

func TestNewDefaultHealerWithPolicyCopiesAndValidates(t *testing.T) {
	policy := DefaultPolicyV1()
	healer, err := NewDefaultHealerWithPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.Weights.Text = 99
	if healer.Weights.Text != DefaultWeights().Text {
		t.Fatalf("healer policy changed through caller value: %+v", healer.Weights)
	}
	invalid := DefaultPolicyV1()
	invalid.Weights = Weights{}
	if _, err := NewDefaultHealerWithPolicy(invalid); err == nil {
		t.Fatal("invalid policy should be rejected")
	}
}

func TestDecisionValidate(t *testing.T) {
	candidate := Candidate{
		Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#submit"},
		Score:    0.9,
	}
	valid := []Decision{
		{Outcome: OutcomeApplied, Best: &candidate, Candidates: []Candidate{candidate}},
		{Outcome: OutcomeBelowCap, Best: &candidate, Candidates: []Candidate{candidate}, NeedsReview: true},
		{Outcome: OutcomeNoCandidate},
	}
	for i, decision := range valid {
		if err := decision.Validate(); err != nil {
			t.Fatalf("valid decision %d: %v", i, err)
		}
	}

	invalid := []Decision{
		{Outcome: OutcomeApplied},
		{Outcome: OutcomeApplied, Best: &candidate, NeedsReview: true},
		{Outcome: OutcomeBelowCap, Best: &candidate},
		{Outcome: OutcomeNoCandidate, Best: &candidate},
		{Outcome: "unknown"},
		{Outcome: OutcomeApplied, Best: &Candidate{Selector: candidate.Selector, Score: 1.1}},
	}
	for i, decision := range invalid {
		if err := decision.Validate(); err == nil {
			t.Fatalf("invalid decision %d should be rejected: %+v", i, decision)
		}
	}
}

func TestDecisionRejectsEveryInvalidSelectorShape(t *testing.T) {
	valid := Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#submit"}, Score: .9}
	tests := []struct {
		name     string
		selector fingerprint.Selector
	}{
		{name: "unsupported type", selector: fingerprint.Selector{Type: "shadow", Value: "#submit"}},
		{name: "blank value", selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "  "}},
		{name: "negative priority", selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#submit", Priority: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			candidate.Selector = test.selector
			decision := Decision{Outcome: OutcomeApplied, Best: &candidate, Candidates: []Candidate{candidate}}
			if err := decision.Validate(); err == nil {
				t.Fatalf("invalid selector accepted: %+v", test.selector)
			}
		})
	}
}

func TestDefaultHealerRejectsInvalidConfigurationBeforeSnapshot(t *testing.T) {
	h := NewDefaultHealer()
	h.Thresholds = Thresholds{ReviewCap: 0.9, AppliedCap: 0.8}
	_, err := h.Heal(context.Background(), fingerprint.ElementTargetSpec{}, nil)
	if err == nil {
		t.Fatal("Heal should reject invalid thresholds")
	}
}

type fakeSnapshot struct {
	candidates []SnapshotCandidate
	err        error
}

func (f fakeSnapshot) Candidates(ctx context.Context) ([]SnapshotCandidate, error) {
	return f.candidates, f.err
}

func TestDefaultHealerRejectsNilAndPropagatesSnapshotFailures(t *testing.T) {
	var nilHealer *DefaultHealer
	if err := nilHealer.Validate(); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("nil healer validation error = %v", err)
	}
	if _, err := nilHealer.Heal(context.Background(), fingerprint.ElementTargetSpec{}, fakeSnapshot{}); err == nil {
		t.Fatal("nil healer unexpectedly healed")
	}

	healer := NewDefaultHealer()
	if _, err := healer.Heal(context.Background(), fingerprint.ElementTargetSpec{}, nil); err == nil || !strings.Contains(err.Error(), "snapshot is nil") {
		t.Fatalf("nil snapshot error = %v", err)
	}
	snapshotErr := errors.New("snapshot unavailable")
	if _, err := healer.Heal(context.Background(), fingerprint.ElementTargetSpec{}, fakeSnapshot{err: snapshotErr}); !errors.Is(err, snapshotErr) {
		t.Fatalf("snapshot error = %v, want %v", err, snapshotErr)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := healer.Heal(canceled, fingerprint.ElementTargetSpec{}, fakeSnapshot{err: canceled.Err()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v", err)
	}
}

func TestHealThresholdBoundariesAreInclusive(t *testing.T) {
	target := fingerprint.ElementTargetSpec{Fingerprint: fingerprint.Fingerprint{
		Tag: "button", Attributes: map[string]string{"id": "submit"},
	}}
	healer := &DefaultHealer{
		Weights:    Weights{Tag: 1, ID: 1},
		Thresholds: Thresholds{ReviewCap: .5, AppliedCap: 1},
	}
	tests := []struct {
		name        string
		fingerprint fingerprint.Fingerprint
		want        Outcome
		review      bool
	}{
		{name: "applied cap", fingerprint: target.Fingerprint, want: OutcomeApplied},
		{name: "review cap", fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"id": "other"}}, want: OutcomeBelowCap, review: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := healer.Heal(context.Background(), target, fakeSnapshot{candidates: []SnapshotCandidate{{
				Fingerprint: test.fingerprint,
				Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#candidate"},
			}}})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Outcome != test.want || decision.NeedsReview != test.review || decision.Best == nil {
				t.Fatalf("decision = %+v, want outcome=%q review=%v", decision, test.want, test.review)
			}
		})
	}
}

func TestHealRejectsInvalidCandidateSelectorFromSnapshot(t *testing.T) {
	target := loginSubmitTarget()
	_, err := NewDefaultHealer().Heal(context.Background(), target, fakeSnapshot{candidates: []SnapshotCandidate{{
		Fingerprint: target.Fingerprint,
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: ""},
	}}})
	if err == nil || !strings.Contains(err.Error(), "invalid selector") {
		t.Fatalf("invalid snapshot candidate error = %v", err)
	}
}

func loginSubmitTarget() fingerprint.ElementTargetSpec {
	return fingerprint.ElementTargetSpec{
		ID:   "login.submit",
		Role: "button",
		Fingerprint: fingerprint.Fingerprint{
			Tag:          "button",
			Attributes:   map[string]string{"type": "submit", "class": "primary", "data-testid": "login-submit"},
			Text:         "登录",
			ARIA:         fingerprint.ARIA{Role: "button", Name: "登录"},
			Path:         []string{"html", "body", "div#app", "form#loginForm", "button"},
			SiblingIndex: 2,
			Neighbors:    fingerprint.Neighbors{Prev: "input#password", ParentTag: "form"},
		},
	}
}

func TestHeal_ExactCloneAppliesHighConfidence(t *testing.T) {
	target := loginSubmitTarget()
	clone := SnapshotCandidate{
		Fingerprint: target.Fingerprint, // DOM 重新渲染出完全相同的节点，只是旧 selector 失效了
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#loginForm button.primary"},
	}

	h := NewDefaultHealer()
	decision, err := h.Heal(context.Background(), target, fakeSnapshot{candidates: []SnapshotCandidate{clone}})
	if err != nil {
		t.Fatalf("Heal: %v", err)
	}
	if decision.Outcome != OutcomeApplied {
		t.Fatalf("outcome = %v, want %v (best score %.3f)", decision.Outcome, OutcomeApplied, decision.Candidates[0].Score)
	}
	if decision.Best == nil || decision.Best.Score < h.Thresholds.AppliedCap {
		t.Fatalf("best candidate score below applied_cap: %+v", decision.Best)
	}
}

func TestHeal_PartialMatchGoesToReview(t *testing.T) {
	target := loginSubmitTarget()
	partial := SnapshotCandidate{
		Fingerprint: fingerprint.Fingerprint{
			Tag:          "button",
			Attributes:   map[string]string{"type": "submit", "class": "secondary"}, // testid 消失，class 变了
			Text:         "登录",
			ARIA:         fingerprint.ARIA{Role: "button", Name: "登录"},
			Path:         []string{"html", "body", "div#app", "form#loginForm", "button"},
			SiblingIndex: 3,
			Neighbors:    fingerprint.Neighbors{Prev: "input#username", ParentTag: "form"},
		},
		Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#loginForm button.secondary"},
	}

	h := NewDefaultHealer()
	decision, err := h.Heal(context.Background(), target, fakeSnapshot{candidates: []SnapshotCandidate{partial}})
	if err != nil {
		t.Fatalf("Heal: %v", err)
	}
	if decision.Outcome != OutcomeBelowCap {
		t.Fatalf("outcome = %v, want %v (best score %.3f)", decision.Outcome, OutcomeBelowCap, decision.Candidates[0].Score)
	}
	if !decision.NeedsReview {
		t.Fatalf("expected NeedsReview = true for a below_cap heal")
	}
}

func TestHeal_UnrelatedElementIsNoCandidate(t *testing.T) {
	target := loginSubmitTarget()
	unrelated := SnapshotCandidate{
		Fingerprint: fingerprint.Fingerprint{
			Tag:          "a",
			Attributes:   map[string]string{"href": "/help"},
			Text:         "帮助中心",
			Path:         []string{"html", "body", "footer"},
			SiblingIndex: 0,
		},
		Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "footer a"},
	}

	h := NewDefaultHealer()
	decision, err := h.Heal(context.Background(), target, fakeSnapshot{candidates: []SnapshotCandidate{unrelated}})
	if err != nil {
		t.Fatalf("Heal: %v", err)
	}
	if decision.Outcome != OutcomeNoCandidate {
		t.Fatalf("outcome = %v, want %v (best score %.3f)", decision.Outcome, OutcomeNoCandidate, decision.Candidates[0].Score)
	}
	if decision.Best != nil {
		t.Fatalf("expected no Best candidate for no_candidate outcome")
	}
}

func TestHeal_EmptySnapshotIsNoCandidate(t *testing.T) {
	h := NewDefaultHealer()
	decision, err := h.Heal(context.Background(), loginSubmitTarget(), fakeSnapshot{})
	if err != nil {
		t.Fatalf("Heal: %v", err)
	}
	if decision.Outcome != OutcomeNoCandidate {
		t.Fatalf("outcome = %v, want %v", decision.Outcome, OutcomeNoCandidate)
	}
}

func TestHeal_PicksHighestScoringOfMultipleCandidates(t *testing.T) {
	target := loginSubmitTarget()
	good := SnapshotCandidate{Fingerprint: target.Fingerprint, Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#loginForm button.primary"}}
	bad := SnapshotCandidate{
		Fingerprint: fingerprint.Fingerprint{Tag: "div", Path: []string{"html", "body"}},
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "body > div"},
	}

	h := NewDefaultHealer()
	decision, err := h.Heal(context.Background(), target, fakeSnapshot{candidates: []SnapshotCandidate{bad, good}})
	if err != nil {
		t.Fatalf("Heal: %v", err)
	}
	if decision.Best == nil || decision.Best.Selector.Value != good.Selector.Value {
		t.Fatalf("expected the good candidate to win, got %+v", decision.Best)
	}
}

func TestHeal_TiedCandidatesUseStableTotalOrder(t *testing.T) {
	target := fingerprint.ElementTargetSpec{Fingerprint: fingerprint.Fingerprint{
		Tag: "button", Text: "保存", Path: []string{"html", "body", "section", "button"},
	}}
	left := SnapshotCandidate{
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Text: "保存", Path: target.Fingerprint.Path},
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#z-last"},
	}
	right := SnapshotCandidate{
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Text: "保存", Path: target.Fingerprint.Path},
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#a-first"},
	}
	for _, candidates := range [][]SnapshotCandidate{{left, right}, {right, left}} {
		decision, err := NewDefaultHealer().Heal(context.Background(), target, fakeSnapshot{candidates: candidates})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Best == nil || decision.Best.Selector.Value != "#a-first" {
			t.Fatalf("tie winner = %+v", decision.Best)
		}
		if decision.Candidates[0].Selector.Value != "#a-first" || decision.Candidates[1].Selector.Value != "#z-last" {
			t.Fatalf("candidate order = %+v", decision.Candidates)
		}
	}
}

func TestHeal_TiedCandidatesUseSelectorPriorityAsDeterministicKey(t *testing.T) {
	target := fingerprint.ElementTargetSpec{Fingerprint: fingerprint.Fingerprint{
		Tag: "button", Text: "保存", Path: []string{"html", "body", "button"},
	}}
	highPriority := SnapshotCandidate{
		Fingerprint: target.Fingerprint,
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "button", Priority: 9},
	}
	lowPriority := SnapshotCandidate{
		Fingerprint: target.Fingerprint,
		Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "button", Priority: 1},
	}
	for _, candidates := range [][]SnapshotCandidate{{highPriority, lowPriority}, {lowPriority, highPriority}} {
		decision, err := NewDefaultHealer().Heal(context.Background(), target, fakeSnapshot{candidates: candidates})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Best == nil || decision.Best.Selector.Priority != 1 {
			t.Fatalf("tie winner = %+v", decision.Best)
		}
	}
}

func TestDecisionValidateRequiresOrderedCandidatesAndBestAtFront(t *testing.T) {
	first := Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#first"}, Score: 0.9}
	second := Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#second"}, Score: 0.8}
	for name, decision := range map[string]Decision{
		"unordered":    {Outcome: OutcomeApplied, Best: &first, Candidates: []Candidate{second, first}},
		"wrong best":   {Outcome: OutcomeApplied, Best: &second, Candidates: []Candidate{first, second}},
		"missing list": {Outcome: OutcomeApplied, Best: &first},
	} {
		if err := decision.Validate(); err == nil {
			t.Fatalf("%s decision unexpectedly valid", name)
		}
	}
}
