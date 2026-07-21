package fingerprint

import (
	"context"
	"testing"
)

type detectorFunc func(context.Context, PageObservation) ([]FrameworkMatch, error)

func (f detectorFunc) Detect(ctx context.Context, observation PageObservation) ([]FrameworkMatch, error) {
	return f(ctx, observation)
}

func TestDetectFrameworksNormalizesAndOrdersMatches(t *testing.T) {
	stack, err := DetectFrameworks(context.Background(), PageObservation{ScriptURLs: []string{"https://cdn.test/vue.js"}}, []FrameworkDetector{
		detectorFunc(func(context.Context, PageObservation) ([]FrameworkMatch, error) {
			return []FrameworkMatch{{Info: FrameworkInfo{Kind: FrameworkVue, Version: " 3.4.0 ", Confidence: 0.9, Evidence: EvidenceScriptLink}}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stack) != 1 || stack[0].Version != "3.4.0" {
		t.Fatalf("stack=%+v", stack)
	}
}
