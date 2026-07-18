package sampling

import (
	"math"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestMatchCombinesSelectorAndStableFingerprintSignals(t *testing.T) {
	sampled := MatchProfile{
		Selectors: []fingerprint.Selector{
			{Type: fingerprint.SelectorTestID, Value: "submit"},
			{Type: fingerprint.SelectorCSS, Value: "#submit"},
		},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", ARIA: fingerprint.ARIA{Role: "button", Name: "提交"}},
		Origin:      "https://example.test",
	}
	baseline := MatchProfile{
		Selectors: []fingerprint.Selector{
			{Type: fingerprint.SelectorTestID, Value: "submit"},
			{Type: fingerprint.SelectorCSS, Value: ".primary"},
		},
		Fingerprint: sampled.Fingerprint,
		Origin:      sampled.Origin,
	}

	score, overlap := Match(sampled, baseline)
	if overlap != 1 {
		t.Fatalf("selector overlap = %d, want 1", overlap)
	}
	want := (1.0/3.0)*.62 + .13 + .1 + .1 + .05
	if math.Abs(score-want) > 1e-12 {
		t.Fatalf("similarity = %f, want %f", score, want)
	}
}

func TestMatchDoesNotRewardMissingOptionalSignals(t *testing.T) {
	profile := MatchProfile{Fingerprint: fingerprint.Fingerprint{Tag: "button"}}
	score, overlap := Match(profile, profile)
	if overlap != 0 || score != .13 {
		t.Fatalf("Match() = (%f, %d), want (0.13, 0)", score, overlap)
	}

	emptyScore, emptyOverlap := Match(MatchProfile{}, MatchProfile{})
	if emptyOverlap != 0 || emptyScore != 0 {
		t.Fatalf("empty Match() = (%f, %d), want (0, 0)", emptyScore, emptyOverlap)
	}
}

func TestMatchCountsDuplicateSelectorsOnce(t *testing.T) {
	selector := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#submit"}
	score, overlap := Match(MatchProfile{Selectors: []fingerprint.Selector{selector}}, MatchProfile{
		Selectors: []fingerprint.Selector{selector, selector},
	})
	if overlap != 1 {
		t.Fatalf("selector overlap = %d, want 1", overlap)
	}
	if math.Abs(score-.62) > 1e-12 {
		t.Fatalf("duplicate selector changed Jaccard score: got %f, want 0.62", score)
	}
}

func FuzzMatchSymmetryBoundsAndSetSemantics(f *testing.F) {
	f.Add("css", "#submit", "css", "#submit")
	f.Add("role", "button[name=提交]", "testid", "submit")
	f.Fuzz(func(t *testing.T, leftType, leftValue, rightType, rightValue string) {
		left := fingerprint.Selector{Type: fingerprint.SelectorType(leftType), Value: leftValue}
		right := fingerprint.Selector{Type: fingerprint.SelectorType(rightType), Value: rightValue}
		a := MatchProfile{Selectors: []fingerprint.Selector{left, left}}
		b := MatchProfile{Selectors: []fingerprint.Selector{right, right}}
		forward, forwardOverlap := Match(a, b)
		reverse, reverseOverlap := Match(b, a)
		if forward < 0 || forward > 1 || math.Abs(forward-reverse) > 1e-12 || forwardOverlap != reverseOverlap {
			t.Fatalf("asymmetric/out-of-range match: forward=(%f,%d) reverse=(%f,%d)", forward, forwardOverlap, reverse, reverseOverlap)
		}
		deduped, _ := Match(MatchProfile{Selectors: []fingerprint.Selector{left}}, MatchProfile{Selectors: []fingerprint.Selector{right}})
		if math.Abs(forward-deduped) > 1e-12 {
			t.Fatalf("duplicate selectors changed score: duplicate=%f deduped=%f", forward, deduped)
		}
	})
}
