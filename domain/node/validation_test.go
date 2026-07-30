package node

import (
	"context"
	"errors"
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
	driver := &testDriver{locate: func(_ context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
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
			{NodeID: "a", Target: fingerprint.ElementTargetSpec{ID: "a"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
			{NodeID: "b", Target: fingerprint.ElementTargetSpec{ID: "b"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
		}}}}
	err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver})
	if err == nil || !strings.Contains(err.Error(), "no validation branch") {
		t.Fatalf("group Run error = %v, want same-round AND timeout", err)
	}
}

func TestValidationGroupPassesWhenBranchStaysTrue(t *testing.T) {
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return &scriptedValidationElement{values: []string{"yes"}}, nil
	}}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: 200 * time.Millisecond,
		Branches: []ValidationBranch{{ID: "first", Nodes: []*ValidationNode{
			{NodeID: "a", Target: fingerprint.ElementTargetSpec{ID: "a"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
			{NodeID: "b", Target: fingerprint.ElementTargetSpec{ID: "b"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}},
		}}}}
	if err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver}); err != nil {
		t.Fatalf("group Run: %v", err)
	}
}

func TestValidationGroupRecordsWinnerAndEveryFinalMember(t *testing.T) {
	driver := &testDriver{locate: func(_ context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
		value := "no"
		if spec.ID == "winner" {
			value = "yes"
		}
		return &scriptedValidationElement{values: []string{value}}, nil
	}}
	facts := &testFacts{}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: 200 * time.Millisecond, Branches: []ValidationBranch{
		{ID: "first", Nodes: []*ValidationNode{{NodeID: "winner", GroupID: "group", BranchID: "first", Target: fingerprint.ElementTargetSpec{ID: "winner"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}}}},
		{ID: "second", Nodes: []*ValidationNode{{NodeID: "loser", GroupID: "group", BranchID: "second", Target: fingerprint.ElementTargetSpec{ID: "loser"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}}}},
	}}
	if err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver, Facts: facts}); err != nil {
		t.Fatalf("group Run: %v", err)
	}
	if len(facts.validationGroups) != 1 {
		t.Fatalf("terminal groups = %d, want 1", len(facts.validationGroups))
	}
	terminal := facts.validationGroups[0]
	if terminal.TerminalReason != "passed" || terminal.WinningBranchID != "first" || len(terminal.ExpectedMembers) != 2 {
		t.Fatalf("terminal group = %#v", terminal)
	}
	finals := make(map[string]ValidationObservation)
	for _, observation := range facts.validationObservations {
		if observation.Final {
			finals[observation.NodeID] = observation
		}
	}
	if len(finals) != 2 || finals["winner"].BranchDisposition != "won" || finals["loser"].BranchDisposition != "not_satisfied" {
		t.Fatalf("final members = %#v", finals)
	}
}

func TestValidationGroupDeduplicatesRepeatedMemberIdentity(t *testing.T) {
	member := &ValidationNode{NodeID: "member", Target: fingerprint.ElementTargetSpec{ID: "member"}, Assertion: ValidationAssertion{Kind: "text_equals", Expected: "yes"}}
	facts := &testFacts{}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: 200 * time.Millisecond, Branches: []ValidationBranch{{ID: "branch", Nodes: []*ValidationNode{member, member}}}}
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return &scriptedValidationElement{values: []string{"yes"}}, nil
	}}
	if err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver, Facts: facts}); err != nil {
		t.Fatalf("group Run: %v", err)
	}
	if len(facts.validationGroups) != 1 || len(facts.validationGroups[0].ExpectedMembers) != 1 {
		t.Fatalf("terminal groups = %#v", facts.validationGroups)
	}
	finals := 0
	for _, observation := range facts.validationObservations {
		if observation.Final {
			finals++
		}
	}
	if finals != 1 {
		t.Fatalf("final observation count = %d, want 1", finals)
	}
}

func TestValidationTerminalReasonClassification(t *testing.T) {
	pollTimeout := classifyNodeFault(context.DeadlineExceeded)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"canceled", context.Canceled, "canceled"},
		{"parent deadline", context.DeadlineExceeded, "canceled"},
		{"poll timeout", pollTimeout, "timeout"},
		{"joined cancellation wins", errors.Join(context.Canceled, pollTimeout), "canceled"},
		{"system error", errors.New("driver"), "system_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validationTerminalReason(test.err); got != test.want {
				t.Fatalf("validationTerminalReason() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidationGroupRecordsCanceledTerminalReason(t *testing.T) {
	facts := &testFacts{}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: time.Second, Branches: []ValidationBranch{{ID: "branch", Nodes: []*ValidationNode{{NodeID: "member", GroupID: "group", BranchID: "branch", Target: fingerprint.ElementTargetSpec{ID: "member"}, Assertion: ValidationAssertion{Kind: "visible"}}}}}}
	err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, context.Canceled
	}}, Facts: facts})
	if err == nil {
		t.Fatal("canceled group succeeded")
	}
	if len(facts.validationGroups) != 1 || facts.validationGroups[0].TerminalReason != "canceled" {
		t.Fatalf("terminal groups = %#v", facts.validationGroups)
	}
	finals := make([]ValidationObservation, 0)
	for _, observation := range facts.validationObservations {
		if observation.Final {
			finals = append(finals, observation)
		}
	}
	if len(finals) != 1 || finals[0].BranchDisposition != "not_observed" {
		t.Fatalf("final observations = %#v", finals)
	}
}

func TestValidationEvaluationErrorRecordsOneFinalObservation(t *testing.T) {
	facts := &testFacts{}
	validation := &ValidationNode{NodeID: "validation", Target: fingerprint.ElementTargetSpec{ID: "target"}, Assertion: ValidationAssertion{Kind: "visible"}, MaxWait: time.Second}
	err := validation.Run(context.Background(), &Runtime{RunID: "run", Driver: &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, errors.New("driver failure")
	}}, Facts: facts})
	if err == nil {
		t.Fatal("validation succeeded")
	}
	finals := 0
	for _, observation := range facts.validationObservations {
		if observation.Final {
			finals++
		}
	}
	if finals != 1 {
		t.Fatalf("final observation count = %d, want 1: %#v", finals, facts.validationObservations)
	}
}

func TestValidationObservationRecordsFirstChangeAndFinal(t *testing.T) {
	element := &scriptedValidationElement{values: []string{"等待", "成功", "成功"}}
	facts := &testFacts{}
	validation := &ValidationNode{NodeID: "status", Target: fingerprint.ElementTargetSpec{ID: "status", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#status"}}},
		Assertion: ValidationAssertion{Kind: "text_equals", Expected: "成功"}, MaxWait: time.Second, Stability: 200 * time.Millisecond}
	err := validation.Run(context.Background(), &Runtime{RunID: "run", Driver: &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
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
	validation := &ValidationNode{NodeID: "status", Target: fingerprint.ElementTargetSpec{ID: "status"},
		Assertion: ValidationAssertion{Kind: "text_equals", Expected: "${expected_status}"}, MaxWait: time.Second, Stability: 200 * time.Millisecond}
	err := validation.Run(context.Background(), &Runtime{RunID: "run", Scratchpad: map[string]any{"expected_status": "READY"}, Facts: facts,
		Driver: &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return element, nil }}})
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
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return &scriptedValidationElement{values: []string{"READY"}}, nil
	}}
	group := &ValidationGroupNode{NodeID: "group", MaxWait: time.Second, Stability: 200 * time.Millisecond,
		Branches: []ValidationBranch{{ID: "expected", Nodes: []*ValidationNode{{NodeID: "member", Target: fingerprint.ElementTargetSpec{ID: "member"},
			Assertion: ValidationAssertion{Kind: "text_equals", Expected: "${expected_status}"}}}}}}
	if err := group.Run(context.Background(), &Runtime{RunID: "run", Driver: driver, Scratchpad: map[string]any{"expected_status": "READY"}}); err != nil {
		t.Fatalf("validation group Run: %v", err)
	}
}

func TestValidationSetObservationPreservesTypedSourceCollection(t *testing.T) {
	selected := []string{"second", "", "first\x1fpart", "second"}
	facts := &testFacts{}
	validation := &ValidationNode{
		NodeID: "selection",
		Target: fingerprint.ElementTargetSpec{ID: "selection"},
		Assertion: ValidationAssertion{
			Kind:           "selected_set_equals",
			ExpectedValues: []string{"second", "", "first\x1fpart", "second"},
		},
		MaxWait:   time.Second,
		Stability: time.Nanosecond,
	}
	element := &matrixElement{exists: true, visible: true, state: ValidationState{SelectedTexts: selected}}

	if err := validation.Run(context.Background(), &Runtime{RunID: "run", Driver: &matrixDriver{element: element}, Facts: facts}); err != nil {
		t.Fatalf("validation Run: %v", err)
	}
	final := facts.validationObservations[len(facts.validationObservations)-1]
	if got := fmt.Sprint(final.ActualValues); got != fmt.Sprint(selected) {
		t.Fatalf("actual values = %#v, want source collection %#v", final.ActualValues, selected)
	}
	selected[0] = "mutated"
	if final.ActualValues[0] != "second" {
		t.Fatalf("observation aliases driver collection: %#v", final.ActualValues)
	}
}

func TestCompareSetEqualsDoesNotCollideOnDelimiterValues(t *testing.T) {
	assertion := ValidationAssertion{Kind: "selected_set_equals", ExpectedValues: []string{"a\x1fb"}}
	passed, _, err := compareSet(assertion, []string{"a", "b"})
	if err != nil {
		t.Fatalf("compareSet: %v", err)
	}
	if passed {
		t.Fatal("different collection boundaries compared equal")
	}
}

func TestCompareSetContainsPreservesMultiplicity(t *testing.T) {
	assertion := ValidationAssertion{Kind: "selected_set_contains", ExpectedValues: []string{"a", "a"}}
	passed, _, err := compareSet(assertion, []string{"a"})
	if err != nil {
		t.Fatalf("compareSet: %v", err)
	}
	if passed {
		t.Fatal("single actual value satisfied duplicate expected values")
	}
}

func TestSensitiveSetObservationRedactsTypedValues(t *testing.T) {
	facts := &testFacts{}
	validation := &ValidationNode{
		NodeID:    "secret-selection",
		Target:    fingerprint.ElementTargetSpec{ID: "secret-selection", Fingerprint: fingerprint.Fingerprint{Attributes: map[string]string{"name": "api_token"}}},
		Assertion: ValidationAssertion{Kind: "selected_set_equals", ExpectedValues: []string{"secret"}},
		MaxWait:   time.Second, Stability: time.Nanosecond,
	}
	element := &matrixElement{exists: true, visible: true, state: ValidationState{SelectedTexts: []string{"secret"}}}
	if err := validation.Run(context.Background(), &Runtime{RunID: "run", Driver: &matrixDriver{element: element}, Facts: facts}); err != nil {
		t.Fatalf("validation Run: %v", err)
	}
	final := facts.validationObservations[len(facts.validationObservations)-1]
	if final.Actual != "••••••••" || final.ActualValues != nil || final.Assertion.ExpectedValues != nil {
		t.Fatalf("sensitive collection leaked: %#v", final)
	}
}

func TestValidationDoesNotHealMixedNotFoundAndSystemLocateErrors(t *testing.T) {
	systemErr := errors.New("browser disconnected")
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, errors.Join(ErrElementNotFound, systemErr)
	}}
	healer := &testHealer{decision: validDecision(fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"})}
	validation := &ValidationNode{NodeID: "field", Target: fingerprint.ElementTargetSpec{ID: "field"}}
	_, _, err := validation.locate(context.Background(), &Runtime{Driver: driver, Healer: healer})
	if !errors.Is(err, systemErr) || healer.calls != 0 {
		t.Fatalf("locate error = %v, healer calls = %d", err, healer.calls)
	}
}

func TestNotExistsRequiresNoApplicableHealCandidate(t *testing.T) {
	old := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#old", Priority: 0}
	newSelector := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new", Priority: 0}
	driver := &testDriver{locate: func(_ context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
		if len(spec.Selectors) > 0 && spec.Selectors[0].Value == "#new" {
			return testElement{}, nil
		}
		return nil, fmt.Errorf("%w: old selector", ErrElementNotFound)
	}}
	healer := &testHealer{decision: validDecision(newSelector)}
	node := &ValidationNode{NodeID: "missing", Target: fingerprint.ElementTargetSpec{ID: "missing", Selectors: []fingerprint.Selector{old}},
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
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, fmt.Errorf("%w: absent", ErrElementNotFound)
	}}
	node := &ValidationNode{NodeID: "missing", Target: fingerprint.ElementTargetSpec{ID: "missing", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#missing"}}},
		Assertion: ValidationAssertion{Kind: "not_exists"}, MaxWait: time.Second, Stability: 200 * time.Millisecond}
	if err := node.Run(context.Background(), &Runtime{RunID: "run", Driver: driver, Healer: &testHealer{decision: heal.Decision{Outcome: heal.OutcomeNoCandidate}}}); err != nil {
		t.Fatalf("not_exists Run: %v", err)
	}
}
