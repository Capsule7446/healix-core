package node

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

type testElement struct{}

func (testElement) Exists(context.Context) (bool, error)                    { return true, nil }
func (testElement) Visible(context.Context) (bool, error)                   { return true, nil }
func (testElement) Text(context.Context) (string, error)                    { return "value", nil }
func (testElement) Attribute(context.Context, string) (string, bool, error) { return "", false, nil }
func (testElement) Count(context.Context) (int, error)                      { return 1, nil }
func (testElement) Click(context.Context) error                             { return nil }
func (testElement) Input(context.Context, string) error                     { return nil }
func (testElement) Select(context.Context, string, ...string) error         { return nil }
func (testElement) Hover(context.Context) error                             { return nil }
func (testElement) WaitStable(context.Context) error                        { return nil }

type selectingTestElement struct {
	testElement
	selections *[][]string
}

func (e selectingTestElement) Select(_ context.Context, option string, more ...string) error {
	values := append([]string{option}, more...)
	*e.selections = append(*e.selections, values)
	return nil
}

type textTestElement struct {
	testElement
	text   string
	inputs *[]string
}

func (e textTestElement) Text(context.Context) (string, error) { return e.text, nil }
func (e textTestElement) Input(_ context.Context, value string) error {
	*e.inputs = append(*e.inputs, value)
	return nil
}

type testHealingPort struct {
	decision heal.Decision
	err      error
	calls    int
	specs    []fingerprint.ElementTargetSpec
}

func (p *testHealingPort) Recover(_ context.Context, spec fingerprint.ElementTargetSpec, _ heal.DOMSnapshot) (heal.Decision, error) {
	p.calls++
	p.specs = append(p.specs, spec)
	return p.decision, p.err
}

type testSnapshot struct{}

func (testSnapshot) Candidates(context.Context) ([]heal.SnapshotCandidate, error) { return nil, nil }

type testDriver struct {
	locate      func(context.Context, fingerprint.ElementTargetSpec) (Element, error)
	locateSpecs []fingerprint.ElementTargetSpec
}

func (d *testDriver) Navigate(context.Context, string) error { return nil }
func (d *testDriver) Press(context.Context, string) error    { return nil }
func (d *testDriver) Locate(ctx context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
	d.locateSpecs = append(d.locateSpecs, spec)
	if d.locate != nil {
		return d.locate(ctx, spec)
	}
	return testElement{}, nil
}
func (d *testDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return testSnapshot{}, nil
}
func (d *testDriver) WaitNetworkIdle(context.Context) error { return nil }

type testHealer struct {
	decision heal.Decision
	// decisions, when non-empty, is consumed one entry per call so a test can
	// drive a spec through more than one recovery inside a single entry.
	decisions []heal.Decision
	err       error
	calls     int
	specs     []fingerprint.ElementTargetSpec
}

func (h *testHealer) Heal(_ context.Context, spec fingerprint.ElementTargetSpec, _ heal.DOMSnapshot) (heal.Decision, error) {
	h.calls++
	h.specs = append(h.specs, spec)
	if len(h.decisions) > 0 {
		decision := h.decisions[0]
		h.decisions = h.decisions[1:]
		return decision, h.err
	}
	return h.decision, h.err
}

type testFacts struct {
	eventErrFor            map[Phase]error
	eventErrors            []error
	healDecisionErr        error
	events                 []Event
	healSpecIDs            []string
	healOldSelectors       []fingerprint.Selector
	healDecisions          []heal.Decision
	validationObservations []ValidationObservation
	validationGroups       []ValidationGroupTerminalObservation
	fences                 []domainexecution.WorkerFence
	rejectCanceled         bool
}

func (m *testFacts) RecordProgress(ctx context.Context, fence domainexecution.WorkerFence, evt Event) error {
	m.fences = append(m.fences, fence)
	return m.recordEvent(ctx, evt)
}
func (m *testFacts) CommitTerminal(ctx context.Context, fence domainexecution.WorkerFence, commit TerminalCommit) error {
	m.fences = append(m.fences, fence)
	return m.recordEvent(ctx, commit.Event)
}
func (m *testFacts) recordEvent(ctx context.Context, evt Event) error {
	if m.rejectCanceled && ctx.Err() != nil {
		return ctx.Err()
	}
	if len(m.eventErrors) > 0 {
		err := m.eventErrors[0]
		m.eventErrors = m.eventErrors[1:]
		if err != nil {
			return err
		}
	}
	if err := m.eventErrFor[evt.Phase]; err != nil {
		return err
	}
	m.events = append(m.events, evt)
	return nil
}
func (m *testFacts) StageHealDecision(_ context.Context, fence domainexecution.WorkerFence, _, specID string, oldSelector fingerprint.Selector, decision heal.Decision) error {
	m.fences = append(m.fences, fence)
	m.healSpecIDs = append(m.healSpecIDs, specID)
	m.healOldSelectors = append(m.healOldSelectors, oldSelector)
	m.healDecisions = append(m.healDecisions, decision)
	return m.healDecisionErr
}
func (m *testFacts) StageValidationObservation(_ context.Context, fence domainexecution.WorkerFence, observation ValidationObservation) error {
	m.fences = append(m.fences, fence)
	m.validationObservations = append(m.validationObservations, observation)
	return nil
}
func (m *testFacts) StageValidationGroupTerminal(_ context.Context, fence domainexecution.WorkerFence, observation ValidationGroupTerminalObservation) error {
	m.fences = append(m.fences, fence)
	m.validationGroups = append(m.validationGroups, observation)
	return nil
}

func validDecision(selector fingerprint.Selector) heal.Decision {
	candidate := heal.Candidate{Selector: selector, Score: 0.9}
	return heal.Decision{
		Outcome:    heal.OutcomeApplied,
		Best:       &candidate,
		Candidates: []heal.Candidate{candidate},
	}
}

func TestExecutionFactsUseWorkerFenceForProgressAndTerminalCommit(t *testing.T) {
	sink := &testFacts{}
	runtime := &Runtime{InstanceID: mustInstanceID("run"), ClaimToken: "claim", Facts: sink}
	if err := runtime.emit(context.Background(), "step", PhaseRunning); err != nil {
		t.Fatal(err)
	}
	if err := runtime.emitTerminal(context.Background(), "step", PhaseSucceeded); err != nil {
		t.Fatal(err)
	}
	want := domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim"}
	if len(sink.fences) != 2 || sink.fences[0] != want || sink.fences[1] != want {
		t.Fatalf("fences = %#v, want progress and terminal fenced by %#v", sink.fences, want)
	}
}

func TestEmitRunningRollbackPreservesSuccessfulOccurrenceSequence(t *testing.T) {
	sink := &testFacts{eventErrors: []error{errors.New("persist failed"), nil, nil}}
	runtime := &Runtime{InstanceID: mustInstanceID("run"), Facts: sink}

	if err := runtime.emit(context.Background(), "step", PhaseRunning); err == nil {
		t.Fatal("first RUNNING emit should fail")
	}
	if err := runtime.emit(context.Background(), "step", PhaseRunning); err != nil {
		t.Fatalf("second RUNNING emit: %v", err)
	}
	if err := runtime.emit(context.Background(), "step", PhaseSucceeded); err != nil {
		t.Fatalf("SUCCEEDED emit: %v", err)
	}
	if len(sink.events) != 2 || sink.events[0].Occurrence != 1 || sink.events[1].Occurrence != 1 {
		t.Fatalf("events = %#v, want successful occurrence 1 pair", sink.events)
	}
}

func TestEmitRunningRollbackHandlesRepeatedFailures(t *testing.T) {
	sink := &testFacts{eventErrors: []error{errors.New("one"), errors.New("two"), nil}}
	runtime := &Runtime{InstanceID: mustInstanceID("run"), Facts: sink}

	for i := 0; i < 2; i++ {
		if err := runtime.emit(context.Background(), "step", PhaseRunning); err == nil {
			t.Fatalf("RUNNING failure %d unexpectedly succeeded", i+1)
		}
	}
	if err := runtime.emit(context.Background(), "step", PhaseRunning); err != nil {
		t.Fatalf("successful RUNNING emit: %v", err)
	}
	if got := sink.events[0].Occurrence; got != 1 {
		t.Fatalf("successful occurrence = %d, want 1", got)
	}
}

func TestEmitRunningRollbackPreservesNestedSameIDLIFO(t *testing.T) {
	sink := &testFacts{}
	runtime := &Runtime{InstanceID: mustInstanceID("run"), Facts: sink}
	if err := runtime.emit(context.Background(), "step", PhaseRunning); err != nil {
		t.Fatal(err)
	}
	sink.eventErrors = []error{errors.New("nested persist failed"), nil, nil, nil}
	if err := runtime.emit(context.Background(), "step", PhaseRunning); err == nil {
		t.Fatal("failed nested RUNNING unexpectedly succeeded")
	}
	if err := runtime.emit(context.Background(), "step", PhaseRunning); err != nil {
		t.Fatal(err)
	}
	if err := runtime.emit(context.Background(), "step", PhaseSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := runtime.emit(context.Background(), "step", PhaseSucceeded); err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 2, 1}
	for i, occurrence := range want {
		if sink.events[i].Occurrence != occurrence {
			t.Fatalf("event occurrences = %#v, want %v", sink.events, want)
		}
	}
}

func TestEmitTerminalFailureRetainsOccurrenceForFallback(t *testing.T) {
	sink := &testFacts{eventErrors: []error{nil, errors.New("succeeded failed"), nil}}
	runtime := &Runtime{InstanceID: mustInstanceID("run"), Facts: sink}

	occurrence, err := runtime.beginOccurrence(context.Background(), "step")
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.releaseOccurrence("step", occurrence)
	if err := runtime.emit(context.Background(), "step", PhaseSucceeded); err == nil {
		t.Fatal("SUCCEEDED emit should fail")
	}
	if err := runtime.emitTerminal(context.Background(), "step", PhaseFailed); err != nil {
		t.Fatalf("fallback FAILED emit: %v", err)
	}
	if len(sink.events) != 2 || sink.events[0].Occurrence != sink.events[1].Occurrence || sink.events[1].Phase != PhaseFailed {
		t.Fatalf("events = %#v, want RUNNING then FAILED on same occurrence", sink.events)
	}
}

func TestOccurrenceCleanupAfterAllTerminalWritesFailAllowsReuse(t *testing.T) {
	sink := &testFacts{eventErrors: []error{nil, errors.New("succeeded failed"), errors.New("failed failed"), nil, nil}}
	runtime := &Runtime{InstanceID: mustInstanceID("run"), Facts: sink}

	occurrence, err := runtime.beginOccurrence(context.Background(), "step")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.emit(context.Background(), "step", PhaseSucceeded); err == nil {
		t.Fatal("SUCCEEDED emit should fail")
	}
	if err := runtime.emitTerminal(context.Background(), "step", PhaseFailed); err == nil {
		t.Fatal("FAILED emit should fail")
	}
	runtime.releaseOccurrence("step", occurrence)
	if err := runtime.emit(context.Background(), "step", PhaseRunning); err != nil {
		t.Fatalf("reused RUNNING emit: %v", err)
	}
	if err := runtime.emit(context.Background(), "step", PhaseSucceeded); err != nil {
		t.Fatalf("reused SUCCEEDED emit: %v", err)
	}
	if got := sink.events[1].Occurrence; got != 2 {
		t.Fatalf("reused occurrence = %d, want 2", got)
	}
}

func TestOccurrenceCleanupPreservesNestedSameIDLIFO(t *testing.T) {
	sink := &testFacts{eventErrors: []error{nil, nil, errors.New("inner canceled failed"), nil}}
	runtime := &Runtime{InstanceID: mustInstanceID("run"), Facts: sink}

	outer, err := runtime.beginOccurrence(context.Background(), "step")
	if err != nil {
		t.Fatal(err)
	}
	inner, err := runtime.beginOccurrence(context.Background(), "step")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.emitTerminal(context.Background(), "step", PhaseCanceled); err == nil {
		t.Fatal("inner CANCELED emit should fail")
	}
	runtime.releaseOccurrence("step", inner)
	if err := runtime.emit(context.Background(), "step", PhaseSucceeded); err != nil {
		t.Fatalf("outer terminal emit: %v", err)
	}
	runtime.releaseOccurrence("step", outer)
	if got := sink.events[len(sink.events)-1].Occurrence; got != outer {
		t.Fatalf("outer terminal occurrence = %d, want %d", got, outer)
	}
}

func TestStepExecutionRejectsInvalidTransitions(t *testing.T) {
	execution := NewStepExecution("submit")
	if err := execution.Transition(PhaseSucceeded); err == nil {
		t.Fatal("initial -> succeeded should be rejected")
	}
	for _, phase := range []Phase{PhaseRunning, PhaseHealing, PhaseTransitioning, PhaseValidating, PhaseSucceeded} {
		if err := execution.Transition(phase); err != nil {
			t.Fatalf("transition to %s: %v", phase, err)
		}
	}
	if got := execution.Phase(); got != PhaseSucceeded {
		t.Fatalf("Phase = %s, want %s", got, PhaseSucceeded)
	}
	if err := execution.Transition(PhaseFailed); err == nil {
		t.Fatal("terminal succeeded -> failed should be rejected")
	}
	canceled := NewStepExecution("cancelled")
	if err := canceled.Transition(PhaseRunning); err != nil {
		t.Fatal(err)
	}
	if err := canceled.Transition(PhaseCanceled); err != nil || canceled.Phase() != PhaseCanceled {
		t.Fatalf("running -> canceled transition failed: phase=%s err=%v", canceled.Phase(), err)
	}
}

func TestHealedSelectorOverlayIsSharedBySpecID(t *testing.T) {
	oldSelector := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#old", Priority: 0}
	newSelector := fingerprint.Selector{Type: fingerprint.SelectorTestID, Value: "submit", Priority: 0}
	spec := fingerprint.ElementTargetSpec{ID: "login.submit", Selectors: []fingerprint.Selector{oldSelector}}

	driver := &testDriver{locate: func(_ context.Context, got fingerprint.ElementTargetSpec) (Element, error) {
		if len(got.Selectors) > 0 && got.Selectors[0].Value == newSelector.Value {
			return testElement{}, nil
		}
		return nil, fmt.Errorf("old selector failed: %w", NewElementNotFoundError())
	}}
	healer := &testHealer{decision: validDecision(newSelector)}
	facts := &testFacts{}
	rt := &Runtime{
		InstanceID: mustInstanceID("run-1"),
		Specs:      map[string]fingerprint.ElementTargetSpec{spec.ID: spec},
		Driver:     driver,
		Healer:     healer,
		Facts:      facts,
	}
	first := &StepNode{NodeID: "click-submit", Target: spec, Action: Action{Kind: ActionNoop}}
	second := &StepNode{NodeID: "check-submit", Target: spec, Action: Action{Kind: ActionNoop}}

	if err := first.Run(context.Background(), rt); err != nil {
		t.Fatalf("first.Run: %v", err)
	}
	if err := second.Run(context.Background(), rt); err != nil {
		t.Fatalf("second.Run: %v", err)
	}
	if healer.calls != 1 {
		t.Fatalf("healer calls = %d, want 1", healer.calls)
	}
	if len(driver.locateSpecs) != 3 {
		t.Fatalf("Locate calls = %d, want 3", len(driver.locateSpecs))
	}
	for i := 1; i < len(driver.locateSpecs); i++ {
		if got := driver.locateSpecs[i].Selectors[0].Value; got != newSelector.Value {
			t.Fatalf("Locate call %d selector = %q, want healed %q", i, got, newSelector.Value)
		}
	}
	if _, wrongKey := rt.SelectorOverlay[first.NodeID]; wrongKey {
		t.Fatalf("selector overlay must not be keyed by step ID %q", first.NodeID)
	}
	if got := rt.SelectorOverlay[spec.ID][0].Value; got != newSelector.Value {
		t.Fatalf("overlay selector = %q, want %q", got, newSelector.Value)
	}
	if got := rt.Specs[spec.ID].Selectors[0].Value; got != oldSelector.Value {
		t.Fatalf("compiled Specs mutated to %q, want original %q", got, oldSelector.Value)
	}
	if len(facts.healSpecIDs) != 1 || facts.healSpecIDs[0] != spec.ID {
		t.Fatalf("heal fact spec IDs = %v, want [%s]", facts.healSpecIDs, spec.ID)
	}
}

// TestSecondHealOfOneSpecBuildsOnTheOverlayNotTheCompiledSpec pins the second
// recovery of a spec inside one entry — the case a page navigation creates
// when the selector installed by the first heal goes stale too.
//
// Locating already consults the overlay, so what fails the second time is the
// healed selector, not the compiled one. Handing the healer the compiled spec
// instead would ask it to recover from a selector that stopped being the live
// one at the first heal, stage that same dead selector as the decision's
// old_selector, and rebuild the overlay from the compiled list — silently
// discarding the first healed selector as a fallback. Recovery evidence that
// names the wrong predecessor is worse than absent evidence: it reads as a
// complete chain.
func TestSecondHealOfOneSpecBuildsOnTheOverlayNotTheCompiledSpec(t *testing.T) {
	compiled := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#v1"}
	firstHealed := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#v2"}
	secondHealed := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#v3"}
	spec := fingerprint.ElementTargetSpec{ID: "login.submit", Selectors: []fingerprint.Selector{compiled}}

	live := firstHealed.Value
	driver := &testDriver{locate: func(_ context.Context, got fingerprint.ElementTargetSpec) (Element, error) {
		if len(got.Selectors) > 0 && got.Selectors[0].Value == live {
			return testElement{}, nil
		}
		return nil, NewElementNotFoundError()
	}}
	healer := &testHealer{decisions: []heal.Decision{validDecision(firstHealed), validDecision(secondHealed)}}
	facts := &testFacts{}
	rt := &Runtime{
		InstanceID: mustInstanceID("run-1"),
		Specs:      map[string]fingerprint.ElementTargetSpec{spec.ID: spec},
		Driver:     driver,
		Healer:     healer,
		Facts:      facts,
	}

	if err := (&StepNode{NodeID: "click-submit", Target: spec, Action: Action{Kind: ActionNoop}}).Run(context.Background(), rt); err != nil {
		t.Fatalf("first step: %v", err)
	}
	live = secondHealed.Value
	if err := (&StepNode{NodeID: "confirm-submit", Target: spec, Action: Action{Kind: ActionNoop}}).Run(context.Background(), rt); err != nil {
		t.Fatalf("second step: %v", err)
	}

	if healer.calls != 2 {
		t.Fatalf("healer calls = %d, want 2", healer.calls)
	}
	if got := healer.specs[0].Selectors[0]; got != compiled {
		t.Fatalf("first heal saw %v, want compiled %v", got, compiled)
	}
	if got := healer.specs[1].Selectors[0]; got != firstHealed {
		t.Fatalf("second heal saw %v, want the healed selector that just failed %v", got, firstHealed)
	}
	wantOverlay := []fingerprint.Selector{secondHealed, firstHealed, compiled}
	if got := rt.SelectorOverlay[spec.ID]; !reflect.DeepEqual(got, wantOverlay) {
		t.Fatalf("overlay = %v, want the healed selectors kept as fallbacks %v", got, wantOverlay)
	}
	wantStaged := []fingerprint.Selector{compiled, firstHealed}
	if !reflect.DeepEqual(facts.healOldSelectors, wantStaged) {
		t.Fatalf("staged old selectors = %v, want %v", facts.healOldSelectors, wantStaged)
	}
	if got := rt.Specs[spec.ID].Selectors[0]; got != compiled {
		t.Fatalf("compiled Specs mutated to %v, want %v", got, compiled)
	}
}

// TestInjectedHealingPortIsUsedInPreferenceToHealer covers the seam a host
// integrates through when it supplies its own recovery service rather than the
// in-process heal.Healer.
//
// Both fields feed one call site, and the adapter around Healer is what every
// other healing test exercises — so the injected-port arm carried no evidence
// at all despite being the arm an external integrator lands on. The two cases
// below separate the two things the field has to do: enable recovery on its
// own, and win when the algorithmic healer is also present.
func TestInjectedHealingPortIsUsedInPreferenceToHealer(t *testing.T) {
	healedSelector := fingerprint.Selector{Type: fingerprint.SelectorTestID, Value: "submit"}
	spec := fingerprint.ElementTargetSpec{ID: "login.submit", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}}

	tests := []struct {
		name       string
		withHealer bool
	}{
		{name: "port alone enables recovery"},
		{name: "port wins over healer", withHealer: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver := &testDriver{locate: func(_ context.Context, got fingerprint.ElementTargetSpec) (Element, error) {
				if len(got.Selectors) > 0 && got.Selectors[0].Value == healedSelector.Value {
					return testElement{}, nil
				}
				return nil, NewElementNotFoundError()
			}}
			port := &testHealingPort{decision: validDecision(healedSelector)}
			rt := &Runtime{
				InstanceID: mustInstanceID("run-1"),
				Specs:      map[string]fingerprint.ElementTargetSpec{spec.ID: spec},
				Driver:     driver,
				Healing:    port,
				Facts:      &testFacts{},
			}
			var healer *testHealer
			if test.withHealer {
				healer = &testHealer{decision: validDecision(fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#wrong"})}
				rt.Healer = healer
			}

			if err := (&StepNode{NodeID: "click-submit", Target: spec, Action: Action{Kind: ActionNoop}}).Run(context.Background(), rt); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if port.calls != 1 {
				t.Fatalf("port calls = %d, want 1", port.calls)
			}
			if got := port.specs[0].ID; got != spec.ID {
				t.Fatalf("port saw spec %q, want %q", got, spec.ID)
			}
			if got := rt.SelectorOverlay[spec.ID][0]; got != healedSelector {
				t.Fatalf("overlay selector = %v, want the port's candidate %v", got, healedSelector)
			}
			if healer != nil && healer.calls != 0 {
				t.Fatalf("healer calls = %d, want 0 when a port is injected", healer.calls)
			}
		})
	}
}

// TestHealedSelectorOverlaySurvivesAWorkflowCallScope pins the overlay against
// the one boundary in the tree that swaps runtime state mid-execution.
//
// A workflow call replaces the parameter scope for the duration of the target,
// and a healed selector is not parameter state — it is a fact about the live
// page that outlives any binding frame. Rediscovering it per sub-workflow
// would spend a second recovery on an element the run has already relocated.
func TestHealedSelectorOverlaySurvivesAWorkflowCallScope(t *testing.T) {
	healedSelector := fingerprint.Selector{Type: fingerprint.SelectorTestID, Value: "submit"}
	spec := fingerprint.ElementTargetSpec{ID: "shared.submit", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}}

	driver := &testDriver{locate: func(_ context.Context, got fingerprint.ElementTargetSpec) (Element, error) {
		if len(got.Selectors) > 0 && got.Selectors[0].Value == healedSelector.Value {
			return testElement{}, nil
		}
		return nil, NewElementNotFoundError()
	}}
	healer := &testHealer{decision: validDecision(healedSelector)}
	rt := &Runtime{
		InstanceID: mustInstanceID("run-1"),
		Specs:      map[string]fingerprint.ElementTargetSpec{spec.ID: spec},
		Driver:     driver,
		Healer:     healer,
		Facts:      &testFacts{},
	}

	parentStep := &StepNode{NodeID: "parent-submit", Target: spec, Action: Action{Kind: ActionNoop}}
	if err := parentStep.Run(context.Background(), rt); err != nil {
		t.Fatalf("parent step: %v", err)
	}
	childStep := &StepNode{NodeID: "child-submit", Target: spec, Action: Action{Kind: ActionNoop}}
	call := &WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "child", Children: []Node{childStep}}}
	if err := call.Run(context.Background(), rt); err != nil {
		t.Fatalf("workflow call: %v", err)
	}

	if healer.calls != 1 {
		t.Fatalf("healer calls = %d, want 1 — the sub-workflow re-healed a spec the parent already recovered", healer.calls)
	}
	last := driver.locateSpecs[len(driver.locateSpecs)-1]
	if got := last.Selectors[0]; got != healedSelector {
		t.Fatalf("child located with %v, want the overlay selector %v", got, healedSelector)
	}
}

// TestExtractAfterHealingWritesHealedElementTextForLaterInterpolation joins the
// two halves that only meet in a real run: recovery hands back an element, and
// extract turns that element into a scratchpad variable other steps read.
//
// Nothing rebinds the element between the heal and the read, so the risk is not
// arithmetic — it is that a future change routes extract past the healed
// element and captures text from the stale one. A downstream ${var} step makes
// that visible at the point a host would notice it.
func TestExtractAfterHealingWritesHealedElementTextForLaterInterpolation(t *testing.T) {
	healedSelector := fingerprint.Selector{Type: fingerprint.SelectorTestID, Value: "order"}
	spec := fingerprint.ElementTargetSpec{ID: "receipt.order", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}}

	var inputs []string
	element := textTestElement{text: "ORDER-42", inputs: &inputs}
	driver := &testDriver{locate: func(_ context.Context, got fingerprint.ElementTargetSpec) (Element, error) {
		if len(got.Selectors) > 0 && got.Selectors[0].Value == healedSelector.Value {
			return element, nil
		}
		return nil, NewElementNotFoundError()
	}}
	rt := &Runtime{
		InstanceID: mustInstanceID("run-1"),
		Specs:      map[string]fingerprint.ElementTargetSpec{spec.ID: spec},
		Driver:     driver,
		Healer:     &testHealer{decision: validDecision(healedSelector)},
		Facts:      &testFacts{},
		Scratchpad: map[string]any{},
	}

	extract := &StepNode{NodeID: "read-order", Target: spec, Action: Action{Kind: ActionExtract, Value: "order_id"}}
	if err := extract.Run(context.Background(), rt); err != nil {
		t.Fatalf("extract step: %v", err)
	}
	if got := rt.Scratchpad["order_id"]; got != "ORDER-42" {
		t.Fatalf("scratchpad order_id = %#v, want the healed element's text", got)
	}

	echo := &StepNode{NodeID: "echo-order", Target: spec, Action: Action{Kind: ActionInput, Value: "${order_id}"}}
	if err := echo.Run(context.Background(), rt); err != nil {
		t.Fatalf("echo step: %v", err)
	}
	if !reflect.DeepEqual(inputs, []string{"ORDER-42"}) {
		t.Fatalf("inputs = %v, want the extracted value interpolated", inputs)
	}
}

func TestStepDoesNotHealSystemLocateErrors(t *testing.T) {
	systemErr := errors.New("browser disconnected")
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, systemErr
	}}
	healer := &testHealer{decision: validDecision(fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"})}
	step := &StepNode{NodeID: "submit", Target: fingerprint.ElementTargetSpec{ID: "submit"}}
	err := step.Run(context.Background(), &Runtime{Driver: driver, Healer: healer})
	if !errors.Is(err, systemErr) {
		t.Fatalf("Run error = %v, want system locate error", err)
	}
	if healer.calls != 0 {
		t.Fatalf("healer calls = %d, want 0", healer.calls)
	}
}

func TestStepDoesNotHealMixedNotFoundAndSystemLocateErrors(t *testing.T) {
	systemErr := errors.New("browser disconnected")
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, errors.Join(NewElementNotFoundError(), systemErr)
	}}
	healer := &testHealer{decision: validDecision(fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"})}
	step := &StepNode{NodeID: "submit", Target: fingerprint.ElementTargetSpec{ID: "submit"}}
	err := step.Run(context.Background(), &Runtime{Driver: driver, Healer: healer})
	if !errors.Is(err, systemErr) || healer.calls != 0 {
		t.Fatalf("Run error = %v, healer calls = %d", err, healer.calls)
	}
}

func TestOptionalStepSkipsMissingTargetWithoutHealing(t *testing.T) {
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, NewElementNotFoundError()
	}}
	healer := &testHealer{decision: validDecision(fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"})}
	facts := &testFacts{}
	step := &StepNode{NodeID: "close-modal", Target: fingerprint.ElementTargetSpec{ID: "modal.close"}, Optional: true}
	err := step.Run(context.Background(), &Runtime{InstanceID: mustInstanceID("run-1"), Driver: driver, Healer: healer, Facts: facts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if healer.calls != 0 {
		t.Fatalf("healer calls = %d, want 0", healer.calls)
	}
	if got := facts.events[len(facts.events)-1].Phase; got != PhaseSucceeded {
		t.Fatalf("last phase = %s, want SUCCEEDED", got)
	}
}

func TestStepExpandsSelectValuesWithoutMutatingCompiledAction(t *testing.T) {
	var selections [][]string
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return selectingTestElement{selections: &selections}, nil
	}}
	step := &StepNode{
		NodeID: "choose-groups",
		Target: fingerprint.ElementTargetSpec{ID: "groups"},
		Action: Action{Kind: ActionSelect, Values: []string{"${PRIMARY}", "${SECONDARY}"}},
	}
	rt := &Runtime{Driver: driver, Scratchpad: map[string]any{"PRIMARY": "A", "SECONDARY": "B"}}
	if err := step.Run(context.Background(), rt); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	rt.Scratchpad["PRIMARY"] = "C"
	rt.Scratchpad["SECONDARY"] = "D"
	if err := step.Run(context.Background(), rt); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if got := step.Action.Values; len(got) != 2 || got[0] != "${PRIMARY}" || got[1] != "${SECONDARY}" {
		t.Fatalf("compiled action values mutated: %v", got)
	}
	if len(selections) != 2 || selections[0][0] != "A" || selections[0][1] != "B" || selections[1][0] != "C" || selections[1][1] != "D" {
		t.Fatalf("selections = %v, want [[A B] [C D]]", selections)
	}
}

func TestStepPersistsCanceledEventAfterExecutionContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		cancel()
		return nil, context.Canceled
	}}
	facts := &testFacts{rejectCanceled: true}
	step := &StepNode{NodeID: "cancelled", Target: fingerprint.ElementTargetSpec{ID: "target"}}
	err := step.Run(ctx, &Runtime{InstanceID: mustInstanceID("run-1"), Driver: driver, Facts: facts})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context canceled", err)
	}
	if len(facts.events) == 0 || facts.events[len(facts.events)-1].Phase != PhaseCanceled {
		t.Fatalf("events = %+v, want terminal CANCELED", facts.events)
	}
}

func TestStepRejectsInvalidHealDecision(t *testing.T) {
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, NewElementNotFoundError()
	}}
	healer := &testHealer{decision: heal.Decision{Outcome: heal.OutcomeApplied}}
	step := &StepNode{NodeID: "submit", Target: fingerprint.ElementTargetSpec{ID: "submit"}}
	err := step.Run(context.Background(), &Runtime{Driver: driver, Healer: healer})
	if err == nil || !fault.IsCode(err, CodeOperationFailed) {
		t.Fatalf("Run error = %v, want invalid decision error", err)
	}
}

func TestStepPropagatesCriticalFactErrors(t *testing.T) {
	auditErr := errors.New("audit unavailable")
	spec := fingerprint.ElementTargetSpec{ID: "submit", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit"}}}

	t.Run("execution event", func(t *testing.T) {
		facts := &testFacts{eventErrFor: map[Phase]error{PhaseSucceeded: auditErr}}
		step := &StepNode{NodeID: "submit", Target: spec}
		err := step.Run(context.Background(), &Runtime{Driver: &testDriver{}, Facts: facts})
		if !errors.Is(err, auditErr) {
			t.Fatalf("Run error = %v, want audit error", err)
		}
		if got := facts.events[len(facts.events)-1].Phase; got != PhaseFailed {
			t.Fatalf("last persisted phase = %s, want FAILED fallback", got)
		}
		if facts.events[len(facts.events)-1].Occurrence != facts.events[0].Occurrence {
			t.Fatalf("fallback occurrence = %d, want %d", facts.events[len(facts.events)-1].Occurrence, facts.events[0].Occurrence)
		}
	})

	t.Run("heal decision", func(t *testing.T) {
		driver := &testDriver{locate: func(_ context.Context, got fingerprint.ElementTargetSpec) (Element, error) {
			if got.Selectors[0].Value == "#new" {
				return testElement{}, nil
			}
			return nil, NewElementNotFoundError()
		}}
		healer := &testHealer{decision: validDecision(fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"})}
		facts := &testFacts{healDecisionErr: auditErr}
		step := &StepNode{NodeID: "submit", Target: spec}
		err := step.Run(context.Background(), &Runtime{InstanceID: mustInstanceID("run-1"), Driver: driver, Healer: healer, Facts: facts})
		if !errors.Is(err, auditErr) {
			t.Fatalf("Run error = %v, want audit error", err)
		}
	})

}

func TestWaitElementPropagatesSystemLocateError(t *testing.T) {
	systemErr := errors.New("browser disconnected")
	driver := &testDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
		return nil, systemErr
	}}
	wait := &WaitNode{NodeID: "wait-submit", Kind: WaitElement, Target: fingerprint.ElementTargetSpec{ID: "submit"}}
	err := wait.Run(context.Background(), &Runtime{Driver: driver})
	if !errors.Is(err, systemErr) {
		t.Fatalf("Run error = %v, want system locate error", err)
	}
}
