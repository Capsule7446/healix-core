package node

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

type scriptedValidationElement struct {
	testElement
	values []string
	calls  int
}

func (e *scriptedValidationElement) Text(context.Context) (string, error) {
	if len(e.values) == 0 {
		return "", nil
	}
	index := e.calls
	e.calls++
	if index >= len(e.values) {
		index = len(e.values) - 1
	}
	return e.values[index], nil
}

func TestValidationGroupDoesNotLatchSeparateANDPasses(t *testing.T) {
	a := &scriptedValidationElement{values: []string{"yes", "no", "no", "no"}}
	b := &scriptedValidationElement{values: []string{"no", "yes", "yes", "yes"}}
	driver := &testDriver{locate: func(_ context.Context, spec fingerprint.NodeSpec) (Element, error) {
		switch spec.ID {
		case "a":
			return a, nil
		case "b":
			return b, nil
		default:
			return nil, fmt.Errorf("%w: %s", ErrElementNotFound, spec.ID)
		}
	}}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: 200 * time.Millisecond,
		Branches: []ValidationBranch{{ID: "a-and-b", Nodes: []*ValidationNode{
			{NodeID: "a", Target: fingerprint.NodeSpec{ID: "a"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
			{NodeID: "b", Target: fingerprint.NodeSpec{ID: "b"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
		}}}}
	err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver})
	if err == nil || !strings.Contains(err.Error(), "no validation branch") {
		t.Fatalf("group Run error = %v, want same-round AND timeout", err)
	}
}

func TestValidationGroupPassesWhenBranchStaysTrue(t *testing.T) {
	driver := &testDriver{locate: func(context.Context, fingerprint.NodeSpec) (Element, error) {
		return &scriptedValidationElement{values: []string{"yes"}}, nil
	}}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: 200 * time.Millisecond,
		Branches: []ValidationBranch{{ID: "first", Nodes: []*ValidationNode{
			{NodeID: "a", Target: fingerprint.NodeSpec{ID: "a"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
			{NodeID: "b", Target: fingerprint.NodeSpec{ID: "b"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
		}}}}
	if err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver}); err != nil {
		t.Fatalf("group Run: %v", err)
	}
}

func TestValidationObservationRecordsFirstChangeAndFinal(t *testing.T) {
	element := &scriptedValidationElement{values: []string{"等待", "成功", "成功"}}
	facts := &testFacts{}
	validation := &ValidationNode{NodeID: "status", Target: fingerprint.NodeSpec{ID: "status", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#status"}}},
		Assertion: ValidationAssertion{Kind: "text_equals", Expected: "成功"}, MaxWait: time.Second, Stability: 200 * time.Millisecond}
	err := validation.Run(context.Background(), &Runtime{RunID: "run", Driver: &testDriver{locate: func(context.Context, fingerprint.NodeSpec) (Element, error) {
		return element, nil
	}}, Facts: facts})
	if err != nil {
		t.Fatalf("validation Run: %v", err)
	}
	if got := len(facts.validationObservations); got != 3 {
		t.Fatalf("observation count = %d, want first + changed + final", got)
	}
	first, changed, final := facts.validationObservations[0], facts.validationObservations[1], facts.validationObservations[2]
	if first.Passed || first.Actual != "等待" || first.Final || !changed.Passed || changed.Actual != "成功" || changed.Final || !final.Passed || !final.Final {
		t.Fatalf("unexpected validation observations: %#v", facts.validationObservations)
	}
	if final.Selector.Value != "#status" || final.ObservedAtMS <= 0 {
		t.Fatalf("final observation lacks selector/timestamp: %#v", final)
	}
}

func TestValidationExpandsRuntimeVariablesWithoutPersistingResolvedExpectation(t *testing.T) {
	element := &scriptedValidationElement{values: []string{"READY"}}
	facts := &testFacts{}
	validation := &ValidationNode{NodeID: "status", Target: fingerprint.NodeSpec{ID: "status"},
		Assertion: ValidationAssertion{Kind: "text_equals", Expected: "${expected_status}"}, MaxWait: time.Second, Stability: 200 * time.Millisecond}
	err := validation.Run(context.Background(), &Runtime{RunID: "run", Scratchpad: map[string]any{"expected_status": "READY"}, Facts: facts,
		Driver: &testDriver{locate: func(context.Context, fingerprint.NodeSpec) (Element, error) { return element, nil }}})
	if err != nil {
		t.Fatalf("validation Run: %v", err)
	}
	if validation.Assertion.Expected != "${expected_status}" {
		t.Fatalf("persisted assertion was mutated to %q", validation.Assertion.Expected)
	}
	if len(facts.validationObservations) == 0 || facts.validationObservations[len(facts.validationObservations)-1].Assertion.Expected != "${expected_status}" {
		t.Fatalf("validation evidence leaked the resolved value: %#v", facts.validationObservations)
	}
}

func TestValidationExpandsExpectedValuesWithoutMutatingTemplate(t *testing.T) {
	validation := &ValidationNode{NodeID: "selection", Assertion: ValidationAssertion{
		Kind: "selected_set_equals", ExpectedValues: []string{"${expected_choice}"},
	}}
	runtime := &Runtime{Scratchpad: map[string]any{"expected_choice": "A"}}

	first, err := validation.resolvedAssertion(runtime)
	if err != nil {
		t.Fatalf("resolve first assertion: %v", err)
	}
	if got := first.ExpectedValues[0]; got != "A" {
		t.Fatalf("first resolved value = %q, want A", got)
	}
	if got := validation.Assertion.ExpectedValues[0]; got != "${expected_choice}" {
		t.Fatalf("assertion template was mutated to %q", got)
	}

	runtime.Scratchpad["expected_choice"] = "B"
	second, err := validation.resolvedAssertion(runtime)
	if err != nil {
		t.Fatalf("resolve second assertion: %v", err)
	}
	if got := second.ExpectedValues[0]; got != "B" {
		t.Fatalf("second resolved value = %q, want B", got)
	}
}

func TestValidationGroupExpandsRuntimeVariablesForEachBranchMember(t *testing.T) {
	driver := &testDriver{locate: func(context.Context, fingerprint.NodeSpec) (Element, error) {
		return &scriptedValidationElement{values: []string{"READY"}}, nil
	}}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: 200 * time.Millisecond,
		Branches: []ValidationBranch{{ID: "expected", Nodes: []*ValidationNode{{NodeID: "member", Target: fingerprint.NodeSpec{ID: "member"},
			Assertion: ValidationAssertion{Kind: "text_equals", Expected: "${expected_status}"}}}}}}
	if err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver, Scratchpad: map[string]any{"expected_status": "READY"}}); err != nil {
		t.Fatalf("validation group Run: %v", err)
	}
}

func TestNotExistsRequiresNoApplicableHealCandidate(t *testing.T) {
	old := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#old", Priority: 0}
	newSelector := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new", Priority: 0}
	driver := &testDriver{locate: func(_ context.Context, spec fingerprint.NodeSpec) (Element, error) {
		if len(spec.Selectors) > 0 && spec.Selectors[0].Value == "#new" {
			return testElement{}, nil
		}
		return nil, fmt.Errorf("%w: old selector", ErrElementNotFound)
	}}
	healer := &testHealer{decision: validDecision(newSelector)}
	node := &ValidationNode{NodeID: "missing", Target: fingerprint.NodeSpec{ID: "missing", Selectors: []fingerprint.Selector{old}},
		Assertion: ValidationAssertion{Kind: "not_exists"}, MaxWait: time.Second, Stability: 200 * time.Millisecond}
	err := node.Run(context.Background(), &Runtime{RunID: "run", Driver: driver, Healer: healer})
	if err == nil || !strings.Contains(err.Error(), "not continuously satisfied") {
		t.Fatalf("not_exists Run error = %v, want failure because heal found a candidate", err)
	}
	if healer.calls == 0 {
		t.Fatal("not_exists did not invoke deterministic healing")
	}
}

func TestNotExistsPassesOnlyAfterNoCandidate(t *testing.T) {
	driver := &testDriver{locate: func(context.Context, fingerprint.NodeSpec) (Element, error) {
		return nil, fmt.Errorf("%w: absent", ErrElementNotFound)
	}}
	node := &ValidationNode{NodeID: "missing", Target: fingerprint.NodeSpec{ID: "missing", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#missing"}}},
		Assertion: ValidationAssertion{Kind: "not_exists"}, MaxWait: time.Second, Stability: 200 * time.Millisecond}
	if err := node.Run(context.Background(), &Runtime{RunID: "run", Driver: driver, Healer: &testHealer{decision: heal.Decision{Outcome: heal.OutcomeNoCandidate}}}); err != nil {
		t.Fatalf("not_exists Run: %v", err)
	}
}
