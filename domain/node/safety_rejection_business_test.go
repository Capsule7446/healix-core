package node

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

func TestStepSafetyRejectionIsRecordedBeforeFailure(t *testing.T) {
	target := fingerprint.NodeSpec{ID: "submit", Origin: "https://shop.test", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}}
	candidate := heal.Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"}, Score: 0.99}
	facts := &testFacts{}
	driver := &testDriver{locate: func(_ context.Context, spec fingerprint.NodeSpec) (Element, error) {
		if len(spec.Selectors) > 0 && spec.Selectors[0].Value == "#old" {
			return nil, fmt.Errorf("missing: %w", ErrElementNotFound)
		}
		return testElement{}, nil
	}}
	rt := &Runtime{Driver: driver, Healer: &testHealer{decision: heal.Decision{Outcome: heal.OutcomeApplied, Best: &candidate, Candidates: []heal.Candidate{candidate}}}, Facts: facts, Origin: "https://evil.test"}
	err := (&StepNode{NodeID: "step", Target: target, Action: Action{Kind: ActionClick}}).Run(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "origin_mismatch") {
		t.Fatalf("err=%v", err)
	}
	if len(facts.healDecisions) != 1 || facts.healDecisions[0].Outcome != heal.OutcomeSafetyRejected {
		t.Fatalf("decisions=%+v", facts.healDecisions)
	}
	if len(rt.SelectorOverlay) != 0 {
		t.Fatalf("overlay=%v", rt.SelectorOverlay)
	}
}
