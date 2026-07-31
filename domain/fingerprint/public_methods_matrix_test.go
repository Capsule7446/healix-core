package fingerprint

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
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
		name      string
		mutate    func(*FrameworkInfo)
		wantField string
	}{
		{name: "unknown kind", mutate: func(info *FrameworkInfo) { info.Kind = "ember" }, wantField: "kind"},
		{name: "negative confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Nextafter(0, -1) }, wantField: "confidence"},
		{name: "above one confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Nextafter(1, 2) }, wantField: "confidence"},
		{name: "nan confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.NaN() }, wantField: "confidence"},
		{name: "positive infinite confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Inf(1) }, wantField: "confidence"},
		{name: "negative infinite confidence", mutate: func(info *FrameworkInfo) { info.Confidence = math.Inf(-1) }, wantField: "confidence"},
		{name: "carriage return in version", mutate: func(info *FrameworkInfo) { info.Version = "1\r2" }, wantField: "version"},
		{name: "line feed in version", mutate: func(info *FrameworkInfo) { info.Version = "1\n2" }, wantField: "version"},
		{name: "unknown evidence", mutate: func(info *FrameworkInfo) { info.Evidence = "dom_dump" }, wantField: "evidence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := FrameworkInfo{Kind: FrameworkReact, Version: "19", Confidence: 0, Evidence: EvidenceGlobal}
			test.mutate(&info)
			err := info.Validate()
			requireViolation(t, err, CodeFrameworkStackInvalid, fault.CodeFieldInvalid, test.wantField)
			// The rejected value is by definition outside the closed set, so it is
			// arbitrary caller input and must never reach public text.
			requireNoPublicLeak(t, err, "ember", "dom_dump")
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
	requireViolation(t, (FrameworkStack{{Kind: "invalid"}}).Validate(), CodeFrameworkStackInvalid, fault.CodeFieldInvalid, "frameworks.0.kind")
	requireViolation(t, (FrameworkStack{{Kind: FrameworkReact}, {Kind: FrameworkReact, Version: "different"}}).Validate(), CodeFrameworkStackInvalid, fault.CodeFieldDuplicate, "frameworks.1.kind")
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
	// A host detector's own text is outside Core's control, so it stays a private
	// cause: reachable through Unwrap, absent from every public field.
	requireEnvelope(t, err, CodeFrameworkDetectorFailed)
	requireNoPublicLeak(t, err, detectorFailure.Error())

	_, err = DetectFrameworks(context.Background(), PageObservation{}, []FrameworkDetector{
		detectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
			return []FrameworkMatch{{Info: FrameworkInfo{Kind: "invalid", Confidence: 0.5}}}, nil
		}),
	})
	requireViolation(t, err, CodeFrameworkStackInvalid, fault.CodeFieldInvalid, "frameworks.0.kind")

	_, err = DetectFrameworks(context.Background(), PageObservation{}, []FrameworkDetector{nil})
	requireViolation(t, err, CodeFrameworkStackInvalid, fault.CodeFieldRequired, "detectors.0")

	stack, err := DetectFrameworks(context.Background(), PageObservation{}, nil)
	if err != nil || len(stack) != 0 {
		t.Fatalf("empty detector set = %#v, %v", stack, err)
	}
}
