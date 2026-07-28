package heal

import (
	"math"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestSimTextUsesRuneLengthForUnicode(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want float64
	}{
		{name: "ascii different", a: "ab", b: "cd", want: 0},
		{name: "CJK different", a: "登录", b: "注册", want: 0},
		{name: "emoji one of two differs", a: "✅完成", b: "❌完成", want: 2.0 / 3.0},
		{name: "combining rune identical", a: "e\u0301", b: "e\u0301", want: 1},
		{name: "empty", a: "", b: "", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := simText(tc.a, tc.b); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("simText(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestSimIndexStaysBoundedAtIntegerExtremes(t *testing.T) {
	tests := []struct {
		name      string
		target    int
		candidate int
	}{
		{name: "max against negative one", target: math.MaxInt, candidate: -1},
		{name: "min against max", target: math.MinInt, candidate: math.MaxInt},
		{name: "max against min", target: math.MaxInt, candidate: math.MinInt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := simIndex(test.target, test.candidate)
			if math.IsNaN(got) || math.IsInf(got, 0) || got < 0 || got > 1 {
				t.Fatalf("simIndex(%d, %d) = %v, want finite value in [0, 1]", test.target, test.candidate, got)
			}
			if got != 0 {
				t.Fatalf("simIndex(%d, %d) = %v, want 0 for opposite extremes", test.target, test.candidate, got)
			}
		})
	}
}

func TestWeightsValidate(t *testing.T) {
	if err := DefaultWeights().Validate(); err != nil {
		t.Fatalf("DefaultWeights.Validate: %v", err)
	}
	if err := (Weights{}).Validate(); err == nil {
		t.Fatal("zero weights should be rejected")
	}
	invalid := DefaultWeights()
	invalid.Text = -0.1
	if err := invalid.Validate(); err == nil {
		t.Fatal("negative weight should be rejected")
	}
	invalid = DefaultWeights()
	invalid.Text = math.NaN()
	if err := invalid.Validate(); err == nil {
		t.Fatal("NaN weight should be rejected")
	}
}

func TestScore_LabelTextMatchIncreasesScore(t *testing.T) {
	target := fingerprint.Fingerprint{Tag: "input", LabelText: "Username"}
	matching := fingerprint.Fingerprint{Tag: "input", LabelText: "Username"}
	mismatching := fingerprint.Fingerprint{Tag: "input", LabelText: "Password"}

	sMatch := score(DefaultWeights(), target, matching)
	sMismatch := score(DefaultWeights(), target, mismatching)
	if sMatch <= sMismatch {
		t.Fatalf("expected matching label text to score higher: match=%.3f mismatch=%.3f", sMatch, sMismatch)
	}
}

func TestScore_LabelTextDimensionSkippedWhenTargetHasNone(t *testing.T) {
	target := fingerprint.Fingerprint{Tag: "input"}
	withLabel := fingerprint.Fingerprint{Tag: "input", LabelText: "Anything"}
	withoutLabel := fingerprint.Fingerprint{Tag: "input"}

	if score(DefaultWeights(), target, withLabel) != score(DefaultWeights(), target, withoutLabel) {
		t.Fatalf("label text dimension should be excluded when target has no label signal")
	}
}

func TestScore_ContainerMatchIncreasesScore(t *testing.T) {
	target := fingerprint.Fingerprint{Tag: "button", FormID: "loginForm"}
	sameForm := fingerprint.Fingerprint{Tag: "button", FormID: "loginForm"}
	otherForm := fingerprint.Fingerprint{Tag: "button", FormID: "signupForm"}

	sSame := score(DefaultWeights(), target, sameForm)
	sOther := score(DefaultWeights(), target, otherForm)
	if sSame <= sOther {
		t.Fatalf("expected same-form candidate to score higher: same=%.3f other=%.3f", sSame, sOther)
	}
}

func TestScore_DynamicDataAttributeContributesToAttrsDimension(t *testing.T) {
	target := fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"data-qa": "submit-btn"}}
	matching := fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"data-qa": "submit-btn"}}
	mismatching := fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"data-qa": "cancel-btn"}}

	sMatch := score(DefaultWeights(), target, matching)
	sMismatch := score(DefaultWeights(), target, mismatching)
	if sMatch <= sMismatch {
		t.Fatalf("expected matching data-qa to score higher: match=%.3f mismatch=%.3f", sMatch, sMismatch)
	}
}

func TestFingerprintCanonicalKeyIgnoresAttributeInsertionOrder(t *testing.T) {
	left := fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"z": "last", "a": "first"}}
	right := fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"a": "first", "z": "last"}}
	if fingerprintCanonicalKey(left) != fingerprintCanonicalKey(right) {
		t.Fatal("canonical key depends on map insertion order")
	}
}
