package node

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

type matrixElement struct {
	exists          bool
	visible         bool
	text            string
	attributes      map[string]string
	state           ValidationState
	existsErr       error
	visibleErr      error
	textErr         error
	attributeErr    error
	stateErr        error
	waitStableErr   error
	actionErr       error
	waitStableCalls int
	clicks          int
	hovers          int
	inputs          []string
	selections      [][]string
}

func (e *matrixElement) Exists(context.Context) (bool, error)  { return e.exists, e.existsErr }
func (e *matrixElement) Visible(context.Context) (bool, error) { return e.visible, e.visibleErr }
func (e *matrixElement) Text(context.Context) (string, error)  { return e.text, e.textErr }
func (e *matrixElement) Attribute(_ context.Context, name string) (string, bool, error) {
	value, ok := e.attributes[name]
	return value, ok, e.attributeErr
}
func (e *matrixElement) Click(context.Context) error {
	e.clicks++
	return e.actionErr
}
func (e *matrixElement) Input(_ context.Context, value string) error {
	e.inputs = append(e.inputs, value)
	return e.actionErr
}
func (e *matrixElement) Select(_ context.Context, value string, more ...string) error {
	e.selections = append(e.selections, append([]string{value}, more...))
	return e.actionErr
}
func (e *matrixElement) Hover(context.Context) error {
	e.hovers++
	return e.actionErr
}
func (e *matrixElement) WaitStable(context.Context) error {
	e.waitStableCalls++
	return e.waitStableErr
}
func (e *matrixElement) ValidationState(context.Context) (ValidationState, error) {
	return e.state, e.stateErr
}

type matrixDriver struct {
	element          Element
	locate           func(context.Context, fingerprint.ElementTargetSpec) (Element, error)
	snapshot         func(context.Context) (heal.DOMSnapshot, error)
	navigateErr      error
	pressErr         error
	networkIdleErr   error
	navigations      []string
	presses          []string
	locateCalls      int
	networkIdleCalls int
}

func TestStepPhaseTransitionCompleteMatrix(t *testing.T) {
	allowed := map[Phase]map[Phase]bool{
		"":                 {PhaseRunning: true},
		PhaseRunning:       {PhaseHealing: true, PhaseTransitioning: true, PhaseValidating: true, PhaseSucceeded: true, PhaseFailed: true, PhaseCanceled: true},
		PhaseHealing:       {PhaseTransitioning: true, PhaseFailed: true, PhaseCanceled: true},
		PhaseTransitioning: {PhaseValidating: true, PhaseSucceeded: true, PhaseFailed: true, PhaseCanceled: true},
		PhaseValidating:    {PhaseSucceeded: true, PhaseFailed: true, PhaseCanceled: true},
	}
	phases := []Phase{"", PhaseRunning, PhaseHealing, PhaseTransitioning, PhaseValidating, PhaseSucceeded, PhaseFailed, PhaseCanceled, "UNKNOWN"}
	for _, current := range phases {
		for _, next := range phases {
			err := ValidatePhaseTransition(current, next)
			if allowed[current][next] != (err == nil) {
				t.Fatalf("transition %q -> %q err=%v, allowed=%v", current, next, err, allowed[current][next])
			}
		}
	}
	var execution *StepExecution
	if err := execution.CanTransition(PhaseRunning); err == nil {
		t.Fatal("nil step execution accepted a transition")
	}
}

func TestRuntimeSpecLookupAppliesCanonicalSpecAndOverlay(t *testing.T) {
	base := fingerprint.ElementTargetSpec{ID: "version", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#inline"}}}
	canonical := fingerprint.ElementTargetSpec{ID: "version", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#canonical"}}}
	runtime := &Runtime{Specs: map[string]fingerprint.ElementTargetSpec{"version": canonical}, SelectorOverlay: map[string][]fingerprint.Selector{
		"version": {{Type: fingerprint.SelectorTestID, Value: "healed"}},
	}}
	got := runtime.effectiveSpec(base)
	if got.Selectors[0].Value != "healed" {
		t.Fatalf("effective spec = %#v", got)
	}
	got.Selectors[0].Value = "mutated"
	if runtime.SelectorOverlay["version"][0].Value != "healed" {
		t.Fatal("effective selector aliases the runtime overlay")
	}
	lookedUp, ok := runtime.specByID("version")
	if !ok || lookedUp.Selectors[0].Value != "healed" {
		t.Fatalf("specByID = %#v, %v", lookedUp, ok)
	}
	if _, ok := runtime.specByID("missing"); ok {
		t.Fatal("missing spec was found")
	}
}

func (d *matrixDriver) Navigate(_ context.Context, value string) error {
	d.navigations = append(d.navigations, value)
	return d.navigateErr
}
func (d *matrixDriver) Press(_ context.Context, value string) error {
	d.presses = append(d.presses, value)
	return d.pressErr
}
func (d *matrixDriver) Locate(ctx context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
	d.locateCalls++
	if d.locate != nil {
		return d.locate(ctx, spec)
	}
	return d.element, nil
}
func (d *matrixDriver) Snapshot(ctx context.Context) (heal.DOMSnapshot, error) {
	if d.snapshot != nil {
		return d.snapshot(ctx)
	}
	return testSnapshot{}, nil
}
func (d *matrixDriver) WaitNetworkIdle(context.Context) error {
	d.networkIdleCalls++
	return d.networkIdleErr
}

func TestStepActionBusinessMatrix(t *testing.T) {
	target := fingerprint.ElementTargetSpec{ID: "target"}
	tests := []struct {
		name   string
		action Action
		check  func(*testing.T, *matrixElement, *matrixDriver, *Runtime)
	}{
		{name: "click", action: Action{Kind: ActionClick}, check: func(t *testing.T, element *matrixElement, _ *matrixDriver, _ *Runtime) {
			if element.clicks != 1 {
				t.Fatalf("clicks = %d, want 1", element.clicks)
			}
		}},
		{name: "input expands variable", action: Action{Kind: ActionInput, Value: "hello ${name}"}, check: func(t *testing.T, element *matrixElement, _ *matrixDriver, _ *Runtime) {
			if !reflect.DeepEqual(element.inputs, []string{"hello Alice"}) {
				t.Fatalf("inputs = %v", element.inputs)
			}
		}},
		{name: "input expands namespaced parameter", action: Action{Kind: ActionInput, Value: "hello ${params.User} from ${env.Region}"}, check: func(t *testing.T, element *matrixElement, _ *matrixDriver, _ *Runtime) {
			if !reflect.DeepEqual(element.inputs, []string{"hello Parameter Alice from east"}) {
				t.Fatalf("inputs = %v", element.inputs)
			}
		}},
		{name: "select falls back to scalar", action: Action{Kind: ActionSelect, Value: "east"}, check: func(t *testing.T, element *matrixElement, _ *matrixDriver, _ *Runtime) {
			if !reflect.DeepEqual(element.selections, [][]string{{"east"}}) {
				t.Fatalf("selections = %v", element.selections)
			}
		}},
		{name: "select multiple", action: Action{Kind: ActionSelect, Values: []string{"east", "west"}}, check: func(t *testing.T, element *matrixElement, _ *matrixDriver, _ *Runtime) {
			if !reflect.DeepEqual(element.selections, [][]string{{"east", "west"}}) {
				t.Fatalf("selections = %v", element.selections)
			}
		}},
		{name: "hover", action: Action{Kind: ActionHover}, check: func(t *testing.T, element *matrixElement, _ *matrixDriver, _ *Runtime) {
			if element.hovers != 1 {
				t.Fatalf("hovers = %d, want 1", element.hovers)
			}
		}},
		{name: "noop", action: Action{Kind: ActionNoop}},
		{name: "empty kind is noop", action: Action{}},
		{name: "extract", action: Action{Kind: ActionExtract, Value: "order_id"}, check: func(t *testing.T, _ *matrixElement, _ *matrixDriver, runtime *Runtime) {
			if runtime.Scratchpad["order_id"] != "ORDER-42" {
				t.Fatalf("scratchpad = %#v", runtime.Scratchpad)
			}
		}},
		{name: "navigate bypasses element", action: Action{Kind: ActionNavigate, Value: "https://${host}/checkout"}, check: func(t *testing.T, element *matrixElement, driver *matrixDriver, _ *Runtime) {
			if !reflect.DeepEqual(driver.navigations, []string{"https://example.test/checkout"}) || driver.locateCalls != 0 || element.waitStableCalls != 0 {
				t.Fatalf("driver=%+v element=%+v", driver, element)
			}
		}},
		{name: "press bypasses element", action: Action{Kind: ActionPress, Value: "${key}"}, check: func(t *testing.T, element *matrixElement, driver *matrixDriver, _ *Runtime) {
			if !reflect.DeepEqual(driver.presses, []string{"Enter"}) || driver.locateCalls != 0 || element.waitStableCalls != 0 {
				t.Fatalf("driver=%+v element=%+v", driver, element)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			element := &matrixElement{exists: true, visible: true, text: "ORDER-42"}
			driver := &matrixDriver{element: element}
			facts := &testFacts{}
			runtime := &Runtime{RunID: "run", Driver: driver, Facts: facts,
				Scratchpad: map[string]any{"name": "Alice", "host": "example.test", "key": "Enter", "params.User": "contamination"}, parameterScope: map[string]parameter.Value{"User": parameter.TextValue("Parameter Alice"), "env.Region": parameter.TextValue("east")}}
			step := &StepNode{NodeID: "step", Target: target, Action: test.action}
			if err := step.Run(context.Background(), runtime); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if test.check != nil {
				test.check(t, element, driver, runtime)
			}
			if test.action.Kind != ActionNavigate && test.action.Kind != ActionPress && element.waitStableCalls != 1 {
				t.Fatalf("WaitStable calls = %d, want 1", element.waitStableCalls)
			}
			if got := facts.events[len(facts.events)-1].Phase; got != PhaseSucceeded {
				t.Fatalf("last phase = %s", got)
			}
		})
	}
}

func TestTypedEnvironmentStringInterpolation(t *testing.T) {
	number, err := parameter.NewNumberValue("001.2500")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]parameter.Value{
		"env.text":    parameter.TextValue("hello 世界"),
		"env.number":  number,
		"env.boolean": parameter.BooleanValue(true),
		"env.single":  parameter.SingleSelectValue("primary"),
		"env.multi":   parameter.MultiSelectValue([]string{"east", "west"}),
	}
	tests := []struct {
		name       string
		expression string
		want       string
		wantError  string
	}{
		{name: "TEXT", expression: "${env.text}", want: "hello 世界"},
		{name: "NUMBER uses canonical string", expression: "${env.number}", want: "1.25"},
		{name: "BOOLEAN", expression: "${env.boolean}", want: "true"},
		{name: "SINGLE_SELECT", expression: "${env.single}", want: "primary"},
		{name: "MULTI_SELECT is not an ordinary string", expression: "${env.multi}", wantError: "undefined variable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			element := &matrixElement{exists: true, visible: true}
			runtime := &Runtime{RunID: "run", Driver: &matrixDriver{element: element}, Facts: &testFacts{}, parameterScope: values}
			err := (&StepNode{NodeID: "step", Target: fingerprint.ElementTargetSpec{ID: "target"}, Action: Action{Kind: ActionInput, Value: test.expression}}).Run(context.Background(), runtime)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Run() error = %v, want containing %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(element.inputs, []string{test.want}) {
				t.Fatalf("inputs = %v, want %q", element.inputs, test.want)
			}
		})
	}
}

func TestNestedWorkflowExecutesWithNamespacedChildParameterAndEnvironment(t *testing.T) {
	element := &matrixElement{exists: true, visible: true}
	driver := &matrixDriver{element: element}
	step := &StepNode{NodeID: "child-step", Target: fingerprint.ElementTargetSpec{ID: "target"}, Action: Action{Kind: ActionInput, Value: "${params.User}@${env.Region}"}}
	call := &WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "child", Children: []Node{step}}, Bindings: map[string]parameter.Binding{"User": parameter.LiteralBinding(parameter.TextValue("Child Alice"))}, Values: map[string]parameter.Value{"User": parameter.TextValue("Child Alice")}, Constraints: map[string]parameter.Constraint{"User": {Type: parameter.Text}}}
	runtime := &Runtime{RunID: "run", Driver: driver, Facts: &testFacts{}, Scratchpad: map[string]any{"params.User": "contamination", "env.Region": "contamination"}, parameterScope: map[string]parameter.Value{"User": parameter.TextValue("Parent Alice"), "env.Region": parameter.TextValue("east")}}
	if err := call.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(element.inputs, []string{"Child Alice@east"}) {
		t.Fatalf("nested input = %v", element.inputs)
	}
	if runtime.Parameters()["User"].Text() != "Parent Alice" || runtime.Scratchpad["params.User"] != "contamination" {
		t.Fatal("nested scope leaked or polluted scratchpad")
	}
}

func TestStepActionFailureMatrix(t *testing.T) {
	sentinel := errors.New("sentinel")
	tests := []struct {
		name   string
		action Action
		setup  func(*matrixElement, *matrixDriver, *Runtime)
		want   string
	}{
		{name: "missing input variable", action: Action{Kind: ActionInput, Value: "${missing}"}, want: "undefined variable"},
		{name: "empty select", action: Action{Kind: ActionSelect}, want: "NODE_OPERATION_FAILED"},
		{name: "empty extract variable", action: Action{Kind: ActionExtract}, want: "NODE_OPERATION_FAILED"},
		{name: "unknown action", action: Action{Kind: "double_click"}, want: "NODE_OPERATION_FAILED"},
		{name: "wait stable", action: Action{Kind: ActionClick}, setup: func(element *matrixElement, _ *matrixDriver, _ *Runtime) { element.waitStableErr = sentinel }, want: "NODE_OPERATION_FAILED"},
		{name: "element action", action: Action{Kind: ActionClick}, setup: func(element *matrixElement, _ *matrixDriver, _ *Runtime) { element.actionErr = sentinel }, want: "NODE_OPERATION_FAILED"},
		{name: "navigate", action: Action{Kind: ActionNavigate, Value: "https://example.test"}, setup: func(_ *matrixElement, driver *matrixDriver, _ *Runtime) { driver.navigateErr = sentinel }, want: "NODE_OPERATION_FAILED"},
		{name: "press", action: Action{Kind: ActionPress, Value: "Enter"}, setup: func(_ *matrixElement, driver *matrixDriver, _ *Runtime) { driver.pressErr = sentinel }, want: "NODE_OPERATION_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			element := &matrixElement{exists: true, visible: true, text: "value"}
			driver := &matrixDriver{element: element}
			facts := &testFacts{}
			runtime := &Runtime{RunID: "run", Driver: driver, Facts: facts, Scratchpad: map[string]any{}}
			if test.setup != nil {
				test.setup(element, driver, runtime)
			}
			err := (&StepNode{NodeID: "step", Target: fingerprint.ElementTargetSpec{ID: "target"}, Action: test.action}).Run(context.Background(), runtime)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if got := facts.events[len(facts.events)-1].Phase; got != PhaseFailed {
				t.Fatalf("last phase = %s, want FAILED", got)
			}
		})
	}
}

func TestStepHealingFailureMatrix(t *testing.T) {
	target := fingerprint.ElementTargetSpec{ID: "target", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}}
	sentinel := errors.New("sentinel")
	tests := []struct {
		name   string
		driver *matrixDriver
		healer heal.Healer
		want   string
	}{
		{name: "healing disabled", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}, want: "healing disabled"},
		{name: "snapshot failure", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }, snapshot: func(context.Context) (heal.DOMSnapshot, error) { return nil, sentinel }}, healer: &testHealer{}, want: "snapshot for healing"},
		{name: "healer failure", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}, healer: &testHealer{err: sentinel}, want: "heal failed"},
		{name: "no candidate", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}, healer: &testHealer{decision: heal.Decision{Outcome: heal.OutcomeNoCandidate}}, want: "no heal candidate"},
		{name: "relocate failure", driver: &matrixDriver{locate: func(_ context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
			if len(spec.Selectors) > 0 && spec.Selectors[0].Value == "#new" {
				return nil, sentinel
			}
			return nil, ErrElementNotFound
		}}, healer: &testHealer{decision: validDecision(fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"})}, want: "re-locate after heal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := &testFacts{}
			err := (&StepNode{NodeID: "step", Target: target}).Run(context.Background(), &Runtime{RunID: "run", Driver: test.driver, Healer: test.healer, Facts: facts})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if got := facts.events[len(facts.events)-1].Phase; got != PhaseFailed {
				t.Fatalf("last phase = %s", got)
			}
		})
	}
}

func TestValidationAssertionExecutionMatrix(t *testing.T) {
	base := func() *matrixElement {
		return &matrixElement{exists: true, visible: true, text: "Hello   World", attributes: map[string]string{"data-state": "READY"}}
	}
	tests := []struct {
		name      string
		assertion ValidationAssertion
		passing   func(*matrixElement)
		failing   func(*matrixElement)
	}{
		{"exists", ValidationAssertion{Kind: "exists"}, func(e *matrixElement) { e.exists = true }, func(e *matrixElement) { e.exists = false }},
		{"not_exists", ValidationAssertion{Kind: "not_exists"}, func(e *matrixElement) { e.exists = false }, func(e *matrixElement) { e.exists = true }},
		{"visible", ValidationAssertion{Kind: "visible"}, func(e *matrixElement) { e.visible = true }, func(e *matrixElement) { e.visible = false }},
		{"not_visible", ValidationAssertion{Kind: "not_visible"}, func(e *matrixElement) { e.visible = false }, func(e *matrixElement) { e.visible = true }},
		{"text_equals", ValidationAssertion{Kind: "text_equals", Expected: "hello world", IgnoreCase: true}, func(e *matrixElement) { e.text = "Hello   World" }, func(e *matrixElement) { e.text = "other" }},
		{"text_contains", ValidationAssertion{Kind: "text_contains", Expected: "World"}, func(e *matrixElement) { e.text = "Hello World" }, func(e *matrixElement) { e.text = "Hello" }},
		{"text_matches", ValidationAssertion{Kind: "text_matches", Expected: `^H.+d$`}, func(e *matrixElement) { e.text = "Hello World" }, func(e *matrixElement) { e.text = "other" }},
		{"attribute_equals", ValidationAssertion{Kind: "attribute_equals", Attribute: "data-state", Expected: "ready", IgnoreCase: true}, func(e *matrixElement) { e.attributes["data-state"] = "READY" }, func(e *matrixElement) { e.attributes["data-state"] = "WAIT" }},
		{"attribute_contains", ValidationAssertion{Kind: "attribute_contains", Attribute: "data-state", Expected: "EAD"}, func(e *matrixElement) { e.attributes["data-state"] = "READY" }, func(e *matrixElement) { e.attributes["data-state"] = "WAIT" }},
		{"value_equals", ValidationAssertion{Kind: "value_equals", Expected: "ready"}, func(e *matrixElement) { e.state.Value = "ready" }, func(e *matrixElement) { e.state.Value = "wait" }},
		{"value_contains", ValidationAssertion{Kind: "value_contains", Expected: "ead"}, func(e *matrixElement) { e.state.Value = "ready" }, func(e *matrixElement) { e.state.Value = "wait" }},
		{"value_matches", ValidationAssertion{Kind: "value_matches", Expected: `^r.+y$`}, func(e *matrixElement) { e.state.Value = "ready" }, func(e *matrixElement) { e.state.Value = "wait" }},
		{"value_not_empty", ValidationAssertion{Kind: "value_not_empty"}, func(e *matrixElement) { e.state.Value = "x" }, func(e *matrixElement) { e.state.Value = "" }},
		{"enabled", ValidationAssertion{Kind: "enabled"}, func(e *matrixElement) { e.state.Enabled = true }, func(e *matrixElement) { e.state.Enabled = false }},
		{"disabled", ValidationAssertion{Kind: "disabled"}, func(e *matrixElement) { e.state.Enabled = false }, func(e *matrixElement) { e.state.Enabled = true }},
		{"checked", ValidationAssertion{Kind: "checked"}, func(e *matrixElement) { e.state.Checked = true }, func(e *matrixElement) { e.state.Checked = false }},
		{"unchecked", ValidationAssertion{Kind: "unchecked"}, func(e *matrixElement) { e.state.Checked = false }, func(e *matrixElement) { e.state.Checked = true }},
		{"mixed", ValidationAssertion{Kind: "mixed"}, func(e *matrixElement) { e.state.Mixed = true }, func(e *matrixElement) { e.state.Mixed = false }},
		{"selected", ValidationAssertion{Kind: "selected"}, func(e *matrixElement) { e.state.Selected = true }, func(e *matrixElement) { e.state.Selected = false }},
		{"unselected", ValidationAssertion{Kind: "unselected"}, func(e *matrixElement) { e.state.Selected = false }, func(e *matrixElement) { e.state.Selected = true }},
		{"pressed", ValidationAssertion{Kind: "pressed"}, func(e *matrixElement) { e.state.Pressed = true }, func(e *matrixElement) { e.state.Pressed = false }},
		{"unpressed", ValidationAssertion{Kind: "unpressed"}, func(e *matrixElement) { e.state.Pressed = false }, func(e *matrixElement) { e.state.Pressed = true }},
		{"selected_text_equals", ValidationAssertion{Kind: "selected_text_equals", Expected: "East"}, func(e *matrixElement) { e.state.SelectedTexts = []string{"East"} }, func(e *matrixElement) { e.state.SelectedTexts = []string{"West"} }},
		{"selected_text_contains", ValidationAssertion{Kind: "selected_text_contains", Expected: "as"}, func(e *matrixElement) { e.state.SelectedTexts = []string{"East"} }, func(e *matrixElement) { e.state.SelectedTexts = []string{"West"} }},
		{"selected_value_equals", ValidationAssertion{Kind: "selected_value_equals", Expected: "east"}, func(e *matrixElement) { e.state.SelectedValues = []string{"east"} }, func(e *matrixElement) { e.state.SelectedValues = []string{"west"} }},
		{"selected_value_contains", ValidationAssertion{Kind: "selected_value_contains", Expected: "as"}, func(e *matrixElement) { e.state.SelectedValues = []string{"east"} }, func(e *matrixElement) { e.state.SelectedValues = []string{"west"} }},
		{"selected_set_equals", ValidationAssertion{Kind: "selected_set_equals", ExpectedValues: []string{"East", "West"}}, func(e *matrixElement) { e.state.SelectedTexts = []string{"West", "East"} }, func(e *matrixElement) { e.state.SelectedTexts = []string{"East"} }},
		{"selected_set_contains", ValidationAssertion{Kind: "selected_set_contains", ExpectedValues: []string{"East"}}, func(e *matrixElement) { e.state.SelectedTexts = []string{"West", "East"} }, func(e *matrixElement) { e.state.SelectedTexts = []string{"West"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, state := range []struct {
				name      string
				configure func(*matrixElement)
				want      bool
			}{{"pass", test.passing, true}, {"fail", test.failing, false}} {
				t.Run(state.name, func(t *testing.T) {
					element := base()
					state.configure(element)
					validation := &ValidationNode{NodeID: "validation", Target: fingerprint.ElementTargetSpec{ID: "target"}, Assertion: test.assertion}
					got, _, err := validation.evaluate(context.Background(), &Runtime{Driver: &matrixDriver{element: element}})
					if err != nil || got != state.want {
						t.Fatalf("evaluate = %v, err=%v, want %v", got, err, state.want)
					}
				})
			}
		})
	}
}

func TestValidationExecutionErrorAndEvidenceMatrix(t *testing.T) {
	t.Run("state capability required", func(t *testing.T) {
		validation := &ValidationNode{NodeID: "value", Target: fingerprint.ElementTargetSpec{ID: "target"}, Assertion: ValidationAssertion{Kind: "value_equals", Expected: "x"}}
		_, _, err := validation.evaluate(context.Background(), &Runtime{Driver: &matrixDriver{element: testElement{}}})
		if err == nil || !strings.Contains(err.Error(), "validation state") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid expanded regex", func(t *testing.T) {
		validation := &ValidationNode{NodeID: "regex", Target: fingerprint.ElementTargetSpec{ID: "target"}, Assertion: ValidationAssertion{Kind: "text_matches", Expected: "${pattern}"}}
		_, _, err := validation.evaluate(context.Background(), &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true, visible: true, text: "x"}}, Scratchpad: map[string]any{"pattern": "["}})
		if err == nil {
			t.Fatal("invalid expanded regex was accepted")
		}
	})
	t.Run("missing attribute is unsatisfied", func(t *testing.T) {
		validation := &ValidationNode{NodeID: "attr", Target: fingerprint.ElementTargetSpec{ID: "target"}, Assertion: ValidationAssertion{Kind: "attribute_equals", Attribute: "data-state", Expected: "ready"}}
		passed, actual, err := validation.evaluate(context.Background(), &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true, visible: true}}})
		if err != nil || passed || actual != "<undefined>" {
			t.Fatalf("passed=%v actual=%q err=%v", passed, actual, err)
		}
	})
	t.Run("element read errors propagate", func(t *testing.T) {
		sentinel := errors.New("read failed")
		tests := []struct {
			name      string
			assertion ValidationAssertion
			configure func(*matrixElement)
		}{
			{name: "exists", assertion: ValidationAssertion{Kind: "exists"}, configure: func(element *matrixElement) { element.existsErr = sentinel }},
			{name: "visible", assertion: ValidationAssertion{Kind: "visible"}, configure: func(element *matrixElement) { element.visibleErr = sentinel }},
			{name: "text", assertion: ValidationAssertion{Kind: "text_equals", Expected: "x"}, configure: func(element *matrixElement) { element.textErr = sentinel }},
			{name: "attribute", assertion: ValidationAssertion{Kind: "attribute_equals", Attribute: "name", Expected: "x"}, configure: func(element *matrixElement) { element.attributeErr = sentinel }},
			{name: "state", assertion: ValidationAssertion{Kind: "value_equals", Expected: "x"}, configure: func(element *matrixElement) { element.stateErr = sentinel }},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				element := &matrixElement{exists: true, visible: true, attributes: map[string]string{"name": "x"}}
				test.configure(element)
				validation := &ValidationNode{NodeID: "validation", Target: fingerprint.ElementTargetSpec{ID: "target"}, Assertion: test.assertion}
				_, _, err := validation.evaluate(context.Background(), &Runtime{Driver: &matrixDriver{element: element}})
				if !errors.Is(err, sentinel) {
					t.Fatalf("error = %v, want sentinel", err)
				}
			})
		}
	})
	t.Run("sensitive evidence is redacted", func(t *testing.T) {
		facts := &testFacts{}
		validation := &ValidationNode{NodeID: "secret", Target: fingerprint.ElementTargetSpec{ID: "secret", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#secret"}}, Fingerprint: fingerprint.Fingerprint{Attributes: map[string]string{"name": "api_token"}}}, Assertion: ValidationAssertion{Kind: "value_equals", Expected: "top-secret", ExpectedValues: []string{"must-not-leak"}}}
		recorder := newValidationObservationRecorder()
		if err := recorder.record(context.Background(), &Runtime{RunID: "run", Facts: facts}, validation, false, "top-secret", nil, "normal_unsatisfied", true); err != nil {
			t.Fatal(err)
		}
		got := facts.validationObservations[0]
		if got.Actual != "••••••••" || got.Assertion.Expected != "••••••••" || got.Assertion.ExpectedValues != nil {
			t.Fatalf("evidence leaked: %#v", got)
		}
	})
}

func TestValidationHealingMatrix(t *testing.T) {
	target := fingerprint.ElementTargetSpec{ID: "target", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}}}
	newSelector := fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#new"}
	sentinel := errors.New("sentinel")

	t.Run("system locate error is not absence", func(t *testing.T) {
		validation := &ValidationNode{NodeID: "validation", Target: target}
		_, absent, err := validation.locate(context.Background(), &Runtime{Driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, sentinel }}})
		if !errors.Is(err, sentinel) || absent {
			t.Fatalf("absent=%v err=%v", absent, err)
		}
	})

	t.Run("not found without healer is absence", func(t *testing.T) {
		validation := &ValidationNode{NodeID: "validation", Target: target}
		_, absent, err := validation.locate(context.Background(), &Runtime{Driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}})
		if err != nil || !absent {
			t.Fatalf("absent=%v err=%v", absent, err)
		}
	})

	tests := []struct {
		name   string
		driver *matrixDriver
		healer *testHealer
		facts  *testFacts
		want   string
	}{
		{name: "snapshot failure", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }, snapshot: func(context.Context) (heal.DOMSnapshot, error) { return nil, sentinel }}, healer: &testHealer{}, want: "snapshot for healing"},
		{name: "healer failure", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}, healer: &testHealer{err: sentinel}, want: "sentinel"},
		{name: "invalid decision", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}, healer: &testHealer{decision: heal.Decision{Outcome: heal.OutcomeApplied}}, want: "invalid heal decision"},
		{name: "fact failure", driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}, healer: &testHealer{decision: validDecision(newSelector)}, facts: &testFacts{healDecisionErr: sentinel}, want: "re-locate after heal"},
		{name: "relocate failure", driver: &matrixDriver{locate: func(_ context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
			if len(spec.Selectors) > 0 && spec.Selectors[0].Value == "#new" {
				return nil, sentinel
			}
			return nil, ErrElementNotFound
		}}, healer: &testHealer{decision: validDecision(newSelector)}, want: "re-locate after heal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validation := &ValidationNode{NodeID: "validation", Target: target}
			runtime := &Runtime{RunID: "run", Driver: test.driver, Healer: test.healer}
			if test.facts != nil {
				runtime.Facts = test.facts
			}
			_, absent, err := validation.locate(context.Background(), runtime)
			if err == nil || !strings.Contains(err.Error(), test.want) || absent {
				t.Fatalf("absent=%v error=%v, want %q", absent, err, test.want)
			}
		})
	}

	t.Run("no candidate is absence and is recorded", func(t *testing.T) {
		facts := &testFacts{}
		validation := &ValidationNode{NodeID: "validation", Target: target}
		_, absent, err := validation.locate(context.Background(), &Runtime{RunID: "run", Driver: &matrixDriver{locate: func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, ErrElementNotFound }}, Healer: &testHealer{decision: heal.Decision{Outcome: heal.OutcomeNoCandidate}}, Facts: facts})
		if err != nil || !absent || !reflect.DeepEqual(facts.healSpecIDs, []string{"target"}) {
			t.Fatalf("absent=%v facts=%v err=%v", absent, facts.healSpecIDs, err)
		}
	})

	t.Run("candidate relocates and installs overlay", func(t *testing.T) {
		driver := &matrixDriver{locate: func(_ context.Context, spec fingerprint.ElementTargetSpec) (Element, error) {
			if len(spec.Selectors) > 0 && spec.Selectors[0].Value == "#new" {
				return &matrixElement{exists: true}, nil
			}
			return nil, ErrElementNotFound
		}}
		runtime := &Runtime{Driver: driver, Healer: &testHealer{decision: validDecision(newSelector)}}
		validation := &ValidationNode{NodeID: "validation", Target: target}
		_, absent, err := validation.locate(context.Background(), runtime)
		if err != nil || absent || runtime.SelectorOverlay["target"][0].Value != "#new" {
			t.Fatalf("absent=%v overlay=%v err=%v", absent, runtime.SelectorOverlay, err)
		}
	})
}

type matrixNode struct {
	id    string
	calls int
	err   error
}

func (n *matrixNode) ID() string                          { return n.id }
func (n *matrixNode) Run(context.Context, *Runtime) error { n.calls++; return n.err }

func TestCompositeNodeExecutionMatrix(t *testing.T) {
	t.Run("repeat count and short circuit", func(t *testing.T) {
		first := &matrixNode{id: "first"}
		stop := &matrixNode{id: "stop", err: errors.New("stop")}
		last := &matrixNode{id: "last"}
		facts := &testFacts{}
		err := (&RepeatNode{NodeID: "repeat", Times: 3, Children: []Node{first, stop, last}}).Run(context.Background(), &Runtime{RunID: "run", Facts: facts})
		if err == nil || first.calls != 1 || stop.calls != 1 || last.calls != 0 || facts.events[len(facts.events)-1].Phase != PhaseFailed {
			t.Fatalf("calls=%d/%d/%d events=%v err=%v", first.calls, stop.calls, last.calls, facts.events, err)
		}
	})
	t.Run("repeat succeeds exact times", func(t *testing.T) {
		child := &matrixNode{id: "child"}
		facts := &testFacts{}
		if err := (&RepeatNode{NodeID: "repeat", Times: 3, Children: []Node{child}}).Run(context.Background(), &Runtime{Facts: facts}); err != nil {
			t.Fatal(err)
		}
		if child.calls != 3 || facts.events[len(facts.events)-1].Phase != PhaseSucceeded {
			t.Fatalf("calls=%d events=%v", child.calls, facts.events)
		}
	})
	t.Run("workflow short circuits", func(t *testing.T) {
		stop := &matrixNode{id: "stop", err: errors.New("stop")}
		last := &matrixNode{id: "last"}
		facts := &testFacts{}
		err := (&WorkflowNode{NodeID: "workflow", Children: []Node{stop, last}}).Run(context.Background(), &Runtime{Facts: facts})
		if err == nil || last.calls != 0 || facts.events[len(facts.events)-1].Phase != PhaseFailed {
			t.Fatalf("last=%d events=%v err=%v", last.calls, facts.events, err)
		}
	})
	t.Run("workflow call nil target", func(t *testing.T) {
		if err := (&WorkflowCallNode{}).Run(context.Background(), &Runtime{}); err == nil {
			t.Fatal("nil target was accepted")
		}
	})
	t.Run("workflow call restores absent scope after child failure", func(t *testing.T) {
		child := &matrixNode{id: "child", err: errors.New("stop")}
		runtime := &Runtime{Scratchpad: map[string]any{"source": "east"}}
		call := &WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "target", Children: []Node{child}}, Bindings: map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.TextValue("east"))}, Values: map[string]parameter.Value{"region": parameter.TextValue("east")}}
		if err := call.Run(context.Background(), runtime); err == nil {
			t.Fatal("child failure was lost")
		}
		if _, ok := runtime.Scratchpad["region"]; ok {
			t.Fatalf("scope leaked: %#v", runtime.Scratchpad)
		}
		if _, ok := runtime.Scratchpad["params.region"]; ok {
			t.Fatalf("prefixed scope leaked: %#v", runtime.Scratchpad)
		}
	})
}

func TestWaitNodeExecutionMatrix(t *testing.T) {
	t.Run("element retries until present", func(t *testing.T) {
		driver := &matrixDriver{}
		driver.locate = func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
			if driver.locateCalls < 2 {
				return nil, fmt.Errorf("%w: absent", ErrElementNotFound)
			}
			return &matrixElement{exists: true}, nil
		}
		if err := (&WaitNode{NodeID: "wait", Kind: WaitElement, Target: fingerprint.ElementTargetSpec{ID: "target"}, Timeout: time.Second}).Run(context.Background(), &Runtime{Driver: driver}); err != nil {
			t.Fatal(err)
		}
		if driver.locateCalls != 2 {
			t.Fatalf("locate calls = %d", driver.locateCalls)
		}
	})
	t.Run("invisible succeeds when element is removed", func(t *testing.T) {
		driver := &matrixDriver{}
		driver.locate = func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
			return nil, ErrElementNotFound
		}
		err := (&WaitNode{NodeID: "dismissed", Kind: WaitElementInvisible, Target: fingerprint.ElementTargetSpec{ID: "dialog"}, Timeout: time.Second}).Run(context.Background(), &Runtime{Driver: driver})
		if err != nil {
			t.Fatalf("removed element should satisfy invisible wait: %v", err)
		}
		if driver.locateCalls != 1 {
			t.Fatalf("locate calls = %d, want 1", driver.locateCalls)
		}
	})
	t.Run("invisible retries while element remains visible", func(t *testing.T) {
		driver := &matrixDriver{}
		driver.locate = func(context.Context, fingerprint.ElementTargetSpec) (Element, error) {
			if driver.locateCalls < 2 {
				return &matrixElement{visible: true}, nil
			}
			return nil, ErrElementNotFound
		}
		err := (&WaitNode{NodeID: "dismissed", Kind: WaitElementInvisible, Target: fingerprint.ElementTargetSpec{ID: "dialog"}, Timeout: time.Second}).Run(context.Background(), &Runtime{Driver: driver})
		if err != nil {
			t.Fatalf("invisible wait should succeed after removal: %v", err)
		}
		if driver.locateCalls != 2 {
			t.Fatalf("locate calls = %d, want 2", driver.locateCalls)
		}
	})
	t.Run("network idle success and error", func(t *testing.T) {
		for _, test := range []struct {
			name    string
			failure error
		}{{"success", nil}, {"failure", errors.New("busy")}} {
			t.Run(test.name, func(t *testing.T) {
				driver := &matrixDriver{networkIdleErr: test.failure}
				facts := &testFacts{}
				err := (&WaitNode{NodeID: "network", Kind: WaitNetworkIdle, Timeout: time.Second}).Run(context.Background(), &Runtime{Driver: driver, Facts: facts})
				if (test.failure == nil) != (err == nil) || driver.networkIdleCalls != 1 {
					t.Fatalf("calls=%d err=%v", driver.networkIdleCalls, err)
				}
				want := PhaseSucceeded
				if test.failure != nil {
					want = PhaseFailed
				}
				if facts.events[len(facts.events)-1].Phase != want {
					t.Fatalf("events = %v", facts.events)
				}
			})
		}
	})

	t.Run("sleep cancellation emits canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		facts := &testFacts{}
		err := (&WaitNode{NodeID: "sleep", Kind: WaitSleep, Duration: time.Hour}).Run(ctx, &Runtime{Facts: facts})
		if !errors.Is(err, context.Canceled) || facts.events[len(facts.events)-1].Phase != PhaseCanceled {
			t.Fatalf("events=%v err=%v", facts.events, err)
		}
	})
	t.Run("unknown kind fails", func(t *testing.T) {
		if err := (&WaitNode{NodeID: "wait", Kind: "event"}).Run(context.Background(), &Runtime{}); err == nil {
			t.Fatal("unknown wait kind accepted")
		}
	})
}
