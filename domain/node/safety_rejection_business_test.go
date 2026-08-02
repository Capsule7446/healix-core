package node

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

// staticPageLocator answers with one fixed location. A Runtime can no longer
// be handed an origin directly, which is deliberate: the field it used to be
// written into was never populated in production, so a test that set it was
// asserting on a code path no host could reach.
type staticPageLocator struct {
	location PageLocation
	err      error
	calls    int
}

func (l *staticPageLocator) CurrentLocation(context.Context) (PageLocation, error) {
	l.calls++
	return l.location, l.err
}

func TestStepSafetyRejectionIsRecordedBeforeFailure(t *testing.T) {
	target := fingerprint.ElementTargetSpec{ID: "submit", Origin: "https://shop.test", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}}
	candidate := heal.Candidate{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"}, Score: 0.99}
	facts := &testFacts{}
	driver := &testDriver{locate: func(_ context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
		if len(spec.Selectors) > 0 && spec.Selectors[0].Value == "#old" {
			return nil, fmt.Errorf("missing: %w", NewElementNotFoundError())
		}
		return testElement{}, nil
	}}
	locator := &staticPageLocator{location: PageLocation{URL: "https://evil.test/checkout", Origin: "https://evil.test"}}
	rt := &Runtime{Driver: driver, Healer: &testHealer{decision: heal.Decision{Outcome: heal.OutcomeApplied, Best: &candidate, Candidates: []heal.Candidate{candidate}}}, Facts: facts, PageLocator: locator}
	err := (&StepNode{NodeID: "step", Target: target, Action: Action{Kind: ActionClick}}).Run(context.Background(), rt)
	if locator.calls == 0 {
		t.Fatal("the live location was never consulted")
	}
	if err == nil || !fault.IsCode(err, CodeHealingRefused) {
		t.Fatalf("err=%v", err)
	}
	if descriptor, ok := fault.Describe(err); !ok || descriptor.Kind() != fault.FailedPrecondition || strings.Contains(descriptor.Message(), "origin_mismatch") {
		t.Fatalf("public message leaked reason: %v (%#v)", err, descriptor)
	}
	var classified *fault.Error
	if !errors.As(err, &classified) {
		t.Fatalf("could not find the classified fault in the chain: %v", err)
	}
	if cause := errors.Unwrap(classified); cause == nil || !strings.Contains(cause.Error(), "origin_mismatch") {
		t.Fatalf("private cause = %v, want it to retain %q", cause, "origin_mismatch")
	}
	if len(facts.healDecisions) != 1 || facts.healDecisions[0].Outcome != heal.OutcomeSafetyRejected {
		t.Fatalf("decisions=%+v", facts.healDecisions)
	}
	if len(rt.SelectorOverlay) != 0 {
		t.Fatalf("overlay=%v", rt.SelectorOverlay)
	}
}
