package fingerprint

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestFrameworkInfoValidateBusinessBoundaries(t *testing.T) {
	validKinds := []FrameworkKind{
		FrameworkReact,
		FrameworkVue,
		FrameworkAngular,
		FrameworkSvelte,
		FrameworkSolid,
		FrameworkPreact,
		FrameworkUnknown,
	}
	validEvidence := []FrameworkEvidenceKind{
		"",
		EvidenceScriptLink,
		EvidenceGlobal,
		EvidenceRootMarker,
		EvidenceHydration,
	}
	for _, kind := range validKinds {
		for _, evidence := range validEvidence {
			name := string(kind) + "/" + string(evidence)
			t.Run(name, func(t *testing.T) {
				info := FrameworkInfo{Kind: kind, Version: "版本-1.0 🚀", Confidence: 1, Evidence: evidence}
				if err := info.Validate(); err != nil {
					t.Fatalf("valid framework rejected: %v", err)
				}
			})
		}
	}

	tests := []struct {
		name   string
		mutate func(*FrameworkInfo)
		want   string
	}{
		{name: "unknown kind", mutate: func(info *FrameworkInfo) { info.Kind = "ember" }, want: "unsupported framework"},
		{name: "negative confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Nextafter(0, -1) }, want: "confidence"},
		{name: "above one confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Nextafter(1, 2) }, want: "confidence"},
		{name: "nan confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.NaN() }, want: "confidence"},
		{name: "positive infinite confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Inf(1) }, want: "confidence"},
		{name: "negative infinite confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Inf(-1) }, want: "confidence"},
		{name: "carriage return in version", mutate: func(info *FrameworkInfo) { info.Version = "1\r2" }, want: "line break"},
		{name: "line feed in version", mutate: func(info *FrameworkInfo) { info.Version = "1\n2" }, want: "line break"},
		{name: "unknown evidence", mutate: func(info *FrameworkInfo) { info.Evidence = "dom_dump" }, want: "unsupported framework evidence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := FrameworkInfo{Kind: FrameworkReact, Version: "19", Confidence: 0, Evidence: EvidenceGlobal}
			test.mutate(&info)
			if err := info.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestFrameworkStackValidateRejectsInvalidAndDuplicateKinds(t *testing.T) {
	valid := FrameworkStack{
		{Kind: FrameworkReact, Confidence: 1, Evidence: EvidenceGlobal},
		{Kind: FrameworkVue, Confidence: 0, Evidence: EvidenceScriptLink},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid stack rejected: %v", err)
	}
	if err := (FrameworkStack{{Kind: "invalid"}}).Validate(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("invalid member error = %v", err)
	}
	if err := (FrameworkStack{{Kind: FrameworkReact}, {Kind: FrameworkReact, Version: "different"}}).Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate kind error = %v", err)
	}
}

func TestSortFrameworkStackUsesTotalOrderWithoutAliasingInput(t *testing.T) {
	input := FrameworkStack{
		{Kind: FrameworkVue, Version: "3", Confidence: 0.8},
		{Kind: FrameworkReact, Version: "19", Confidence: 0.9},
		{Kind: FrameworkReact, Version: "18", Confidence: 0.9},
		{Kind: FrameworkAngular, Version: "20", Confidence: 0.9},
	}
	want := FrameworkStack{
		{Kind: FrameworkAngular, Version: "20", Confidence: 0.9},
		{Kind: FrameworkReact, Version: "18", Confidence: 0.9},
		{Kind: FrameworkReact, Version: "19", Confidence: 0.9},
		{Kind: FrameworkVue, Version: "3", Confidence: 0.8},
	}
	original := input.Clone()
	got := SortFrameworkStack(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SortFrameworkStack() = %#v, want %#v", got, want)
	}
	got[0].Version = "mutated"
	if !reflect.DeepEqual(input, original) {
		t.Fatalf("sort result aliases input: input=%#v original=%#v", input, original)
	}
	if got := SortFrameworkStack(nil); got != nil {
		t.Fatalf("nil stack sorted to %#v, want nil", got)
	}
}

func TestDetectFrameworksMergesDuplicatesAndPreservesDetectorContract(t *testing.T) {
	observation := PageObservation{
		PageURL:       "https://example.test",
		ScriptURLs:    []string{"https://cdn.test/react.js"},
		GlobalMarkers: []string{"React"},
		RootMarkers:   []string{"data-reactroot"},
		Hydration:     []string{"__NEXT_DATA__"},
	}
	seen := 0
	detectors := []FrameworkDetector{
		detectorFunc(func(_ context.Context, got PageObservation) ([]FrameworkMatch, error) {
			seen++
			if !reflect.DeepEqual(got, observation) {
				t.Fatalf("observation = %#v, want %#v", got, observation)
			}
			return []FrameworkMatch{
				{Info: FrameworkInfo{Kind: FrameworkReact, Version: " 19 ", Confidence: 0.8, Evidence: EvidenceGlobal}},
				{Info: FrameworkInfo{Kind: FrameworkVue, Version: " 3 ", Confidence: 0.7, Evidence: EvidenceScriptLink}},
			}, nil
		}),
		detectorFunc(func(_ context.Context, _ PageObservation) ([]FrameworkMatch, error) {
			seen++
			return []FrameworkMatch{{Info: FrameworkInfo{Kind: FrameworkReact, Version: " 18 ", Confidence: 0.9, Evidence: EvidenceRootMarker}}}, nil
		}),
	}

	stack, err := DetectFrameworks(context.Background(), observation, detectors)
	if err != nil {
		t.Fatal(err)
	}
	if seen != 2 || !reflect.DeepEqual(stack, FrameworkStack{
		{Kind: FrameworkReact, Version: "18", Confidence: 0.9, Evidence: EvidenceRootMarker},
		{Kind: FrameworkVue, Version: "3", Confidence: 0.7, Evidence: EvidenceScriptLink},
	}) {
		t.Fatalf("detector calls/stack = %d/%#v", seen, stack)
	}
}

func TestDetectFrameworksPropagatesFailuresAndRejectsInvalidMatches(t *testing.T) {
	detectorFailure := errors.New("detector unavailable")
	laterCalls := 0
	_, err := DetectFrameworks(context.Background(), PageObservation{}, []FrameworkDetector{
		detectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
			return nil, detectorFailure
		}),
		detectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
			laterCalls++
			return nil, nil
		}),
	})
	if !errors.Is(err, detectorFailure) || laterCalls != 0 {
		t.Fatalf("detector error/later calls = %v/%d", err, laterCalls)
	}

	_, err = DetectFrameworks(context.Background(), PageObservation{}, []FrameworkDetector{
		detectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
			return []FrameworkMatch{{Info: FrameworkInfo{Kind: "invalid", Confidence: 0.5}}}, nil
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported framework") {
		t.Fatalf("invalid detector match error = %v", err)
	}

	stack, err := DetectFrameworks(context.Background(), PageObservation{}, nil)
	if err != nil || len(stack) != 0 {
		t.Fatalf("empty detector set = %#v, %v", stack, err)
	}
}
