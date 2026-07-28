package fingerprint

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

type strictDetectorFunc func(context.Context, PageObservation) ([]FrameworkMatch, error)

func (f strictDetectorFunc) Detect(ctx context.Context, observation PageObservation) ([]FrameworkMatch, error) {
	return f(ctx, observation)
}

type pointerDetector struct{}

func (*pointerDetector) Detect(context.Context, PageObservation) ([]FrameworkMatch, error) {
	return nil, nil
}

func validStrictFingerprint() Fingerprint {
	return Fingerprint{Tag: "button", Attributes: map[string]string{}, Framework: FrameworkStack{}}
}

func TestSelectorValidateStrictBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name     string
		selector Selector
		valid    bool
	}{
		{"zero enum", Selector{Value: "x"}, false},
		{"unknown enum", Selector{Type: "other", Value: "x"}, false},
		{"empty", Selector{Type: SelectorCSS}, false},
		{"whitespace", Selector{Type: SelectorCSS, Value: "\t\n"}, false},
		{"unicode", Selector{Type: SelectorText, Value: "你好"}, true},
		{"negative priority", Selector{Type: SelectorCSS, Value: "x", Priority: -1}, false},
		{"minimum priority", Selector{Type: SelectorCSS, Value: "x", Priority: math.MinInt}, false},
		{"zero priority", Selector{Type: SelectorCSS, Value: "x", Priority: 0}, true},
		{"max priority", Selector{Type: SelectorCSS, Value: "x", Priority: int(^uint(0) >> 1)}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if (tt.selector.Validate() == nil) != tt.valid {
				t.Fatalf("validity mismatch")
			}
		})
	}
}

func TestFingerprintValidateStrictBoundaries(t *testing.T) {
	valid := validStrictFingerprint()
	cases := []struct {
		name  string
		value Fingerprint
		valid bool
	}{
		{"valid", valid, true},
		{"empty tag", Fingerprint{Attributes: map[string]string{}}, false},
		{"whitespace tag", Fingerprint{Tag: " \t", Attributes: map[string]string{}}, false},
		{"nil attributes", Fingerprint{Tag: "button"}, false},
		{"negative sibling", Fingerprint{Tag: "button", Attributes: map[string]string{}, SiblingIndex: -1}, false},
		{"minimum sibling", Fingerprint{Tag: "button", Attributes: map[string]string{}, SiblingIndex: math.MinInt}, false},
		{"maximum sibling", Fingerprint{Tag: "button", Attributes: map[string]string{}, SiblingIndex: math.MaxInt}, true},
		{"invalid framework", Fingerprint{Tag: "button", Attributes: map[string]string{}, Framework: FrameworkStack{{Kind: "invalid"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.value.Framework.Clone()
			if (tc.value.Validate() == nil) != tc.valid {
				t.Fatalf("validity mismatch")
			}
			if len(tc.value.Framework) != len(before) || (len(before) > 0 && !reflect.DeepEqual(tc.value.Framework, before)) {
				t.Fatal("validation mutated framework stack")
			}
		})
	}
}

func TestNodeSpecValidateAggregatesAllFailuresWithoutMutation(t *testing.T) {
	spec := NodeSpec{
		UUID:      "not-a-uuid",
		Selectors: []Selector{{Type: "bad", Priority: -1}, {Type: SelectorCSS}},
		Fingerprint: Fingerprint{
			Framework: FrameworkStack{{Kind: "bad"}},
		},
	}
	before := spec.Selectors[0]
	err := spec.Validate()
	if err == nil {
		t.Fatal("invalid node spec accepted")
	}
	for _, fragment := range []string{"uuid", "id is required", "selectors[0]", "unsupported type", "priority", "selectors[1]", "value is required", "fingerprint.tag"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q missing %q", err, fragment)
		}
	}
	if spec.Selectors[0] != before {
		t.Fatal("validation mutated selector")
	}
}

func TestNodeSpecValidateUUIDAndCollectionBoundaries(t *testing.T) {
	base := NodeSpec{ID: "node", Selectors: []Selector{{Type: SelectorCSS, Value: "#x"}}, Fingerprint: validStrictFingerprint()}
	for _, uuid := range []string{"", "550E8400-E29B-41D4-A716-446655440000", "550e8400-e29b-41d4-a716-446655440000"} {
		spec := base
		spec.UUID = uuid
		if err := spec.Validate(); err != nil {
			t.Errorf("UUID %q rejected: %v", uuid, err)
		}
	}
	for _, uuid := range []string{"550e8400e29b41d4a716446655440000", "550e8400-e29b-41d4-a716-44665544000g", "550e8400-e29b-41d4-a716-446655440000x"} {
		spec := base
		spec.UUID = uuid
		if err := spec.Validate(); err == nil {
			t.Errorf("UUID %q accepted", uuid)
		}
	}
	large := base
	large.Selectors = make([]Selector, 1024)
	for i := range large.Selectors {
		large.Selectors[i] = Selector{Type: SelectorCSS, Value: "#x", Priority: i}
	}
	if err := large.Validate(); err != nil {
		t.Fatalf("bounded selector list rejected: %v", err)
	}
}

func TestFrameworkInfoValidateEnumAndVersionMatrices(t *testing.T) {
	for _, kind := range []FrameworkKind{FrameworkReact, FrameworkVue, FrameworkAngular, FrameworkSvelte, FrameworkSolid, FrameworkPreact, FrameworkUnknown} {
		for _, evidence := range []FrameworkEvidenceKind{"", EvidenceScriptLink, EvidenceGlobal, EvidenceRootMarker, EvidenceHydration} {
			if err := (FrameworkInfo{Kind: kind, Confidence: .5, Evidence: evidence}).Validate(); err != nil {
				t.Errorf("kind %q evidence %q rejected: %v", kind, evidence, err)
			}
		}
	}
	for _, info := range []FrameworkInfo{{Confidence: .5}, {Kind: "other", Confidence: .5}, {Kind: FrameworkReact, Confidence: .5, Evidence: "other"}, {Kind: FrameworkReact, Confidence: .5, Version: "18\n"}, {Kind: FrameworkReact, Confidence: .5, Version: "18\r"}} {
		if err := info.Validate(); err == nil {
			t.Errorf("invalid info accepted: %+v", info)
		}
	}
}

func TestFrameworkStackValidateAndCloneBoundaries(t *testing.T) {
	for _, stack := range []FrameworkStack{nil, {}, {{Kind: FrameworkReact, Confidence: 0}}, {{Kind: FrameworkReact, Confidence: 1}, {Kind: FrameworkVue, Confidence: .5}}} {
		if err := stack.Validate(); err != nil {
			t.Errorf("valid stack %v rejected: %v", stack, err)
		}
	}
	for _, stack := range []FrameworkStack{{{Kind: FrameworkReact}, {Kind: FrameworkReact}}, {{Kind: "invalid"}}} {
		if err := stack.Validate(); err == nil {
			t.Errorf("invalid stack accepted: %v", stack)
		}
	}
	original := FrameworkStack{{Kind: FrameworkReact, Version: "18", Confidence: 1}}
	clone := original.Clone()
	clone[0].Version = "19"
	if original[0].Version != "18" {
		t.Fatal("clone aliases original backing array")
	}
}
func TestFrameworkInfoValidateConfidenceExtremes(t *testing.T) {
	for _, value := range []float64{-1, math.Inf(-1), math.Inf(1), math.NaN(), 1.0001} {
		if (FrameworkInfo{Kind: FrameworkReact, Confidence: value}).Validate() == nil {
			t.Errorf("confidence %v accepted", value)
		}
	}
	for _, value := range []float64{0, 1} {
		if err := (FrameworkInfo{Kind: FrameworkReact, Confidence: value}).Validate(); err != nil {
			t.Errorf("confidence %v: %v", value, err)
		}
	}
}

func TestSortFrameworkStackIsDeterministicAndImmutable(t *testing.T) {
	input := FrameworkStack{{Kind: FrameworkVue, Confidence: .5}, {Kind: FrameworkReact, Confidence: .5}}
	original := input.Clone()
	first := SortFrameworkStack(input)
	second := SortFrameworkStack(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic: %v vs %v", first, second)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatalf("input mutated: %v", input)
	}
	if first[0].Kind != FrameworkReact {
		t.Fatalf("unexpected order: %v", first)
	}
}

func TestDetectFrameworksRejectsNilDetectors(t *testing.T) {
	var nilFunction strictDetectorFunc
	for _, detectors := range [][]FrameworkDetector{{nil}, {(*pointerDetector)(nil)}, {nilFunction}} {
		if _, err := DetectFrameworks(context.Background(), PageObservation{}, detectors); err == nil {
			t.Fatal("expected nil detector error")
		}
	}
}

func TestSortFrameworkStackOrderingAndOwnershipMatrix(t *testing.T) {
	for _, input := range []FrameworkStack{nil, {}, {{Kind: FrameworkReact, Confidence: 1}}} {
		result := SortFrameworkStack(input)
		if len(result) != len(input) || (len(input) > 0 && !reflect.DeepEqual(result, input)) {
			t.Fatalf("boundary sort changed values: %v", result)
		}
	}
	input := FrameworkStack{
		{Kind: FrameworkVue, Version: "2", Confidence: .5},
		{Kind: FrameworkReact, Version: "19", Confidence: .5},
		{Kind: FrameworkReact, Version: "18", Confidence: .5},
		{Kind: FrameworkAngular, Version: "20", Confidence: 1},
	}
	got := SortFrameworkStack(input)
	want := FrameworkStack{input[3], input[2], input[1], input[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	got[0].Version = "changed"
	if input[3].Version != "20" {
		t.Fatal("sorted stack aliases input backing array")
	}
}

func TestDetectFrameworksForwardsContextObservationAndStopsOnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	observation := PageObservation{PageURL: "https://example.test", ScriptURLs: []string{"app.js"}}
	cause := errors.New("detect failed")
	calls := 0
	first := strictDetectorFunc(func(gotCtx context.Context, gotObservation PageObservation) ([]FrameworkMatch, error) {
		calls++
		if gotCtx != ctx || !reflect.DeepEqual(gotObservation, observation) {
			t.Fatal("context or observation not forwarded")
		}
		return nil, cause
	})
	second := strictDetectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
		calls++
		return nil, nil
	})
	_, err := DetectFrameworks(ctx, observation, []FrameworkDetector{first, second})
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("detector calls = %d", calls)
	}
	if !reflect.DeepEqual(observation.ScriptURLs, []string{"app.js"}) {
		t.Fatal("observation mutated")
	}
}

func TestDetectFrameworksEmptyInvalidAndOrderedResults(t *testing.T) {
	got, err := DetectFrameworks(context.Background(), PageObservation{}, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty detectors = %v, %v", got, err)
	}
	invalid := strictDetectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
		return []FrameworkMatch{{Info: FrameworkInfo{Kind: "invalid"}}}, nil
	})
	if _, err := DetectFrameworks(context.Background(), PageObservation{}, []FrameworkDetector{invalid}); err == nil {
		t.Fatal("invalid detector result accepted")
	}
	ordered := strictDetectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
		return []FrameworkMatch{
			{Info: FrameworkInfo{Kind: FrameworkVue, Confidence: .5}},
			{Info: FrameworkInfo{Kind: FrameworkReact, Confidence: 1}},
		}, nil
	})
	got, err = DetectFrameworks(context.Background(), PageObservation{}, []FrameworkDetector{ordered})
	if err != nil || len(got) != 2 || got[0].Kind != FrameworkReact || got[1].Kind != FrameworkVue {
		t.Fatalf("ordered result = %v, %v", got, err)
	}
}

func TestDetectFrameworksMergesDuplicatesWithoutMutatingMatches(t *testing.T) {
	matches := []FrameworkMatch{{Info: FrameworkInfo{Kind: FrameworkReact, Version: " 18 ", Confidence: .8}}, {Info: FrameworkInfo{Kind: FrameworkReact, Version: "17", Confidence: .7}}}
	detector := strictDetectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) { return matches, nil })
	got, err := DetectFrameworks(context.Background(), PageObservation{}, []FrameworkDetector{detector})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Version != "18" {
		t.Fatalf("got %v", got)
	}
	if matches[0].Info.Version != " 18 " {
		t.Fatalf("detector output mutated: %q", matches[0].Info.Version)
	}
}
