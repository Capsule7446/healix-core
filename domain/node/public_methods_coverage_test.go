package node

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestNodeIdentifiersPreserveOpaqueIdentity(t *testing.T) {
	tests := []struct {
		name string
		want string
		node Node
	}{
		{name: "wait empty", node: &WaitNode{}},
		{name: "wait unicode", want: "等待-一", node: &WaitNode{NodeID: "等待-一"}},
		{name: "repeat", want: "repeat-1", node: &RepeatNode{NodeID: "repeat-1"}},
		{name: "validation", want: "validation-1", node: &ValidationNode{NodeID: "validation-1"}},
		{name: "validation group", want: "group-1", node: &ValidationGroupNode{NodeID: "group-1"}},
		{name: "step", want: "step-1", node: &StepNode{NodeID: "step-1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.node.ID(); got != test.want {
				t.Fatalf("ID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorkflowCallNodeIDUsesExplicitThenTargetIdentity(t *testing.T) {
	tests := []struct {
		name string
		node WorkflowCallNode
		want string
	}{
		{name: "explicit", node: WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "target"}}, want: "call"},
		{name: "target fallback", node: WorkflowCallNode{Target: &WorkflowNode{NodeID: "target"}}, want: "target"},
		{name: "missing", node: WorkflowCallNode{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.node.ID(); got != test.want {
				t.Fatalf("ID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLifecycleSideEffectFaultsExposeSafeStableContracts(t *testing.T) {
	cause := errors.New("node=node-secret occurrence=999 adapter=adapter-secret")
	tests := []struct {
		name    string
		err     error
		code    fault.Code
		message string
	}{
		{"start", stepTimelineStartError(cause), CodeStepTimelineStartFailed, "step timeline start could not be recorded"},
		{"finish", stepTimelineFinishError(cause), CodeStepTimelineFinishFailed, "step timeline finish could not be recorded"},
		{"observation", nodeCompletionObservationError(cause), CodeNodeCompletionObservation, "node completion observation could not be recorded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor, ok := fault.Describe(test.err)
			if !ok || descriptor.Code() != test.code || descriptor.Kind() != fault.Internal || descriptor.Message() != test.message || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 || !errors.Is(test.err, cause) {
				t.Fatalf("descriptor/error = %#v/%v", descriptor, test.err)
			}
			for _, secret := range []string{"node-secret", "999", "adapter-secret", cause.Error()} {
				if strings.Contains(test.err.Error(), secret) {
					t.Fatalf("public error leaked %q: %q", secret, test.err.Error())
				}
			}
		})
	}
}

func TestLeafCompletionErrorPreservesCausesBehindSafeContract(t *testing.T) {
	nodeErr := errors.New("node-secret")
	timelineCause := errors.New("timeline-secret")
	observationCause := errors.New("observation-secret")
	err := newLeafCompletionError(
		nodeErr,
		stepTimelineFinishError(timelineCause),
		nodeCompletionObservationError(observationCause),
	)

	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Code() != CodeLeafCompletionFailed || descriptor.Kind() != fault.Internal || descriptor.Message() != "leaf execution completion failed" {
		t.Fatalf("descriptor/error = %#v/%v", descriptor, err)
	}
	for _, secret := range []string{"node-secret", "timeline-secret", "observation-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("public error leaked %q: %q", secret, err.Error())
		}
	}
	for _, cause := range []error{nodeErr, timelineCause, observationCause} {
		if !errors.Is(err, cause) {
			t.Fatalf("completion error does not wrap %v", cause)
		}
	}
	if !fault.IsCode(err, CodeStepTimelineFinishFailed) || !fault.IsCode(err, CodeNodeCompletionObservation) {
		t.Fatalf("side-effect codes were not preserved: %v", err)
	}
}

func TestLeafExecutionErrorExtractsNodeCause(t *testing.T) {
	nodeErr := errors.New("node failed")
	wrapper := newLeafCompletionError(nodeErr, stepTimelineFinishError(errors.New("timeline failed")), nil)
	if got := LeafExecutionError(wrapper); !errors.Is(got, nodeErr) {
		t.Fatalf("LeafExecutionError() = %v, want node cause", got)
	}
	plain := errors.New("plain")
	if got := LeafExecutionError(plain); got != plain {
		t.Fatalf("LeafExecutionError() = %v, want original", got)
	}
}

func TestNodeCompletionChainHasHandlersNilEmptyAndPresent(t *testing.T) {
	var nilChain *NodeCompletionChain
	if nilChain.HasHandlers() {
		t.Fatal("nil chain reports handlers")
	}
	empty, err := NewNodeCompletionChain(NodeCompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.HasHandlers() {
		t.Fatal("empty chain reports handlers")
	}
	present, err := NewNodeCompletionChain(NodeCompletionOptions{}, completionHandlerStub{name: "handler", handle: func(NodeCompletionContext) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if !present.HasHandlers() {
		t.Fatal("non-empty chain reports no handlers")
	}
}

func TestRuntimeLeafExecutionStartedTracksPublicLeafRun(t *testing.T) {
	var nilRuntime *Runtime
	if nilRuntime.LeafExecutionStarted() {
		t.Fatal("nil runtime reports a started leaf")
	}
	runtime := &Runtime{}
	if runtime.LeafExecutionStarted() {
		t.Fatal("fresh runtime reports a started leaf")
	}
	if err := (&WaitNode{NodeID: "wait", Kind: WaitSleep}).Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if !runtime.LeafExecutionStarted() {
		t.Fatal("completed wait did not mark leaf execution started")
	}
}

type screenshotBrowserStub struct {
	artifact ScreenshotArtifact
	err      error
}

func (stub screenshotBrowserStub) CaptureScreenshot(context.Context, ScreenshotOptions) (ScreenshotArtifact, error) {
	return stub.artifact, stub.err
}

func (screenshotBrowserStub) SnapshotDOM(context.Context) (heal.DOMSnapshot, error) {
	return testSnapshot{}, nil
}

func (screenshotBrowserStub) ObserveElement(context.Context, fingerprint.ElementTargetSpec, []string) (ElementObservation, error) {
	return ElementObservation{}, nil
}

func runWithCompletionBrowser(t *testing.T, runtime *Runtime, handle func(ReadOnlyBrowser)) {
	t.Helper()
	chain, err := NewNodeCompletionChain(NodeCompletionOptions{}, completionHandlerStub{
		name: "browser probe",
		handle: func(input NodeCompletionContext) error {
			handle(input.Browser)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.CompletionChain = chain
	if err := (&WaitNode{NodeID: "wait", Kind: WaitSleep}).Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
}

func TestCompletionBrowserCopiesScreenshotAndObservesElementThroughPublicLeafRun(t *testing.T) {
	source := []byte{1, 2, 3}
	runtime := &Runtime{
		Driver: &matrixDriver{element: &matrixElement{
			exists: true, visible: true, text: "ready", attributes: map[string]string{"role": "button"},
		}},
		ReadOnlyBrowser: screenshotBrowserStub{artifact: ScreenshotArtifact{MediaType: "image/png", Data: source}},
	}
	var artifact ScreenshotArtifact
	var observation ElementObservation
	runWithCompletionBrowser(t, runtime, func(browser ReadOnlyBrowser) {
		var err error
		artifact, err = browser.CaptureScreenshot(context.Background(), ScreenshotOptions{FullPage: true})
		if err != nil {
			t.Fatal(err)
		}
		observation, err = browser.ObserveElement(
			context.Background(),
			fingerprint.ElementTargetSpec{ID: "node"},
			[]string{"role", "missing"},
		)
		if err != nil {
			t.Fatal(err)
		}
	})
	source[0] = 9
	if artifact.Data[0] != 1 {
		t.Fatalf("CaptureScreenshot() aliases provider bytes: %v", artifact.Data)
	}
	if !observation.Exists || !observation.Visible || observation.Text != "ready" ||
		observation.Attributes["role"] != "button" || len(observation.Attributes) != 1 {
		t.Fatalf("ObserveElement() = %#v", observation)
	}
}

func TestWaitNodeValidateKindAndBoundaryMatrix(t *testing.T) {
	tests := []struct {
		name      string
		wait      WaitNode
		wantError bool
	}{
		{name: "empty kind is zero duration sleep"},
		{name: "sleep zero duration", wait: WaitNode{Kind: WaitSleep}},
		{name: "sleep positive duration", wait: WaitNode{Kind: WaitSleep, Duration: time.Nanosecond}},
		{name: "sleep negative duration", wait: WaitNode{Kind: WaitSleep, Duration: -time.Nanosecond}, wantError: true},
		{name: "sleep rejects timeout", wait: WaitNode{Kind: WaitSleep, Timeout: time.Nanosecond}, wantError: true},
		{name: "element default timeout", wait: WaitNode{Kind: WaitElement}},
		{name: "element explicit timeout", wait: WaitNode{Kind: WaitElement, Timeout: time.Nanosecond}},
		{name: "element rejects duration", wait: WaitNode{Kind: WaitElement, Duration: time.Nanosecond}, wantError: true},
		{name: "element rejects negative timeout", wait: WaitNode{Kind: WaitElement, Timeout: -time.Nanosecond}, wantError: true},
		{name: "visible", wait: WaitNode{Kind: WaitElementVisible}},
		{name: "invisible", wait: WaitNode{Kind: WaitElementInvisible}},
		{name: "visibility rejects duration", wait: WaitNode{Kind: WaitElementVisible, Duration: time.Nanosecond}, wantError: true},
		{name: "visibility rejects negative timeout", wait: WaitNode{Kind: WaitElementInvisible, Timeout: -time.Nanosecond}, wantError: true},
		{name: "network idle", wait: WaitNode{Kind: WaitNetworkIdle}},
		{name: "network idle rejects duration", wait: WaitNode{Kind: WaitNetworkIdle, Duration: time.Nanosecond}, wantError: true},
		{name: "network idle rejects negative timeout", wait: WaitNode{Kind: WaitNetworkIdle, Timeout: -time.Nanosecond}, wantError: true},
		{name: "unknown kind", wait: WaitNode{Kind: "UNKNOWN"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.wait.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

type runtimeParametersProbe struct {
	probe func(*Runtime) error
}

func (runtimeParametersProbe) ID() string { return "runtime-parameters-probe" }

func (probe runtimeParametersProbe) Run(_ context.Context, runtime *Runtime) error {
	return probe.probe(runtime)
}

func TestRuntimeParametersAndInterpolationPreserveTypedScopeThroughPublicNodes(t *testing.T) {
	number, err := parameter.NewNumberValue("1.20")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]parameter.Value{
		"text": parameter.TextValue("ready"), "number": number,
		"boolean": parameter.BooleanValue(true), "choice": parameter.SingleSelectValue("east"),
		"many": parameter.MultiSelectValue([]string{"east", "west"}),
	}
	element := &matrixElement{exists: true}
	runtime := &Runtime{Driver: &matrixDriver{element: element}, Scratchpad: map[string]any{"scratch": "fallback", "non-string": 42}}
	probe := runtimeParametersProbe{probe: func(runtime *Runtime) error {
		parameters := runtime.Parameters()
		delete(parameters, "text")
		parameters["many"] = parameter.MultiSelectValue([]string{"mutated"})
		again := runtime.Parameters()
		if again["text"].Text() != "ready" || len(again["many"].MultiSelect()) != 2 {
			t.Fatalf("Parameters() aliases runtime scope: %#v", again)
		}
		return nil
	}}
	step := &StepNode{
		NodeID: "interpolate",
		Target: fingerprint.ElementTargetSpec{ID: "input"},
		Action: Action{Kind: ActionInput, Value: "${text}|${params.text}|${number}|${boolean}|${choice}|${scratch}"},
	}
	workflow := &WorkflowNode{NodeID: "root", OwnsParameterScope: true, Parameters: values, Children: []Node{probe, step}}
	if err := workflow.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if len(element.inputs) != 1 || element.inputs[0] != "ready|ready|1.2|true|east|fallback" {
		t.Fatalf("interpolated inputs = %v", element.inputs)
	}
	if runtime.Parameters() != nil {
		t.Fatalf("owned parameter scope leaked after workflow: %#v", runtime.Parameters())
	}
	var nilRuntime *Runtime
	if nilRuntime.Parameters() != nil || (&Runtime{}).Parameters() != nil {
		t.Fatal("nil parameter scope must remain distinguishable from an empty owned scope")
	}

	tests := []struct {
		name string
		key  string
	}{
		{name: "multi select is not scalar", key: "many"},
		{name: "scratch non-string", key: "non-string"},
		{name: "missing", key: "missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step := &StepNode{NodeID: "reject non-scalar", Target: fingerprint.ElementTargetSpec{ID: "input"}, Action: Action{Kind: ActionInput, Value: "${" + test.key + "}"}}
			workflow := &WorkflowNode{NodeID: "root", OwnsParameterScope: true, Parameters: values, Children: []Node{step}}
			if err := workflow.Run(context.Background(), runtime); err == nil {
				t.Fatalf("interpolation unexpectedly accepted %q", test.key)
			}
		})
	}
}

func TestTransientDriverFaultContract(t *testing.T) {
	if transientDriverFault(nil) != nil {
		t.Fatal("nil error was classified")
	}
	cause := errors.New("temporary")
	err := transientDriverFault(cause)
	kind, hasKind := fault.KindOf(err)
	code, hasCode := fault.CodeOf(err)
	if !errors.Is(err, cause) || !hasKind || kind != fault.Unavailable || !hasCode || code != CodeTransientDriver {
		t.Fatalf("transientDriverFault() = %#v", err)
	}
}

func TestNewNodeCompletionChainValidationMatrix(t *testing.T) {
	tests := []struct {
		name      string
		options   NodeCompletionOptions
		handlers  []NodeCompletionHandler
		wantError bool
	}{
		{name: "empty chain"},
		{name: "valid handler", handlers: []NodeCompletionHandler{completionHandlerStub{name: "capture", handle: func(NodeCompletionContext) error { return nil }}}},
		{name: "nil handler", handlers: []NodeCompletionHandler{nil}, wantError: true},
		{name: "blank handler name", handlers: []NodeCompletionHandler{completionHandlerStub{}}, wantError: true},
		{name: "negative handler timeout", options: NodeCompletionOptions{HandlerTimeout: -time.Nanosecond}, wantError: true},
		{name: "negative chain timeout", options: NodeCompletionOptions{ChainTimeout: -time.Nanosecond}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, err := NewNodeCompletionChain(test.options, test.handlers...)
			if (err != nil) != test.wantError {
				t.Fatalf("NewNodeCompletionChain() chain = %#v, error = %v", chain, err)
			}
		})
	}
}

func TestStepTimelineEventBoundaryOutcomeMatrix(t *testing.T) {
	valid := StepTimelineEvent{
		Step:     StepExecutionRef{InstanceID: mustInstanceID("run"), NodeID: "step", Occurrence: 1},
		Boundary: StepBoundaryStarted, Mark: TimelineMark{Sequence: 1},
	}
	tests := []struct {
		name      string
		mutate    func(*StepTimelineEvent)
		wantError bool
	}{
		{name: "started"},
		{name: "finished succeeded", mutate: func(event *StepTimelineEvent) {
			event.Boundary, event.Outcome = StepBoundaryFinished, StepOutcomeSucceeded
		}},
		{name: "finished failed", mutate: func(event *StepTimelineEvent) {
			event.Boundary, event.Outcome = StepBoundaryFinished, StepOutcomeFailed
		}},
		{name: "finished canceled", mutate: func(event *StepTimelineEvent) {
			event.Boundary, event.Outcome = StepBoundaryFinished, StepOutcomeCanceled
		}},
		{name: "occurrence below boundary", mutate: func(event *StepTimelineEvent) { event.Step.Occurrence = 0 }, wantError: true},
		{name: "negative offset", mutate: func(event *StepTimelineEvent) { event.Mark.Offset = -time.Nanosecond }, wantError: true},
		{name: "sequence below boundary", mutate: func(event *StepTimelineEvent) { event.Mark.Sequence = 0 }, wantError: true},
		{name: "started with outcome", mutate: func(event *StepTimelineEvent) { event.Outcome = StepOutcomeSucceeded }, wantError: true},
		{name: "finished without outcome", mutate: func(event *StepTimelineEvent) { event.Boundary = StepBoundaryFinished }, wantError: true},
		{name: "unknown boundary", mutate: func(event *StepTimelineEvent) { event.Boundary = "UNKNOWN" }, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			if test.mutate != nil {
				test.mutate(&event)
			}
			err := event.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

type snapshotFailureStub struct{ err error }

func (stub snapshotFailureStub) Candidates(context.Context) ([]heal.SnapshotCandidate, error) {
	return nil, stub.err
}

func TestCompletionBrowserPropagatesEveryDependencyFailureThroughPublicLeafRun(t *testing.T) {
	sentinel := errors.New("dependency failed")
	t.Run("screenshot", func(t *testing.T) {
		runtime := &Runtime{ReadOnlyBrowser: screenshotBrowserStub{err: sentinel}}
		runWithCompletionBrowser(t, runtime, func(browser ReadOnlyBrowser) {
			if _, err := browser.CaptureScreenshot(context.Background(), ScreenshotOptions{}); !errors.Is(err, sentinel) {
				t.Fatalf("CaptureScreenshot() error = %v", err)
			}
		})
	})
	t.Run("snapshot driver", func(t *testing.T) {
		runtime := &Runtime{Driver: &matrixDriver{snapshot: func(context.Context) (heal.DOMSnapshot, error) { return nil, sentinel }}, ReadOnlyBrowser: screenshotBrowserStub{}}
		runWithCompletionBrowser(t, runtime, func(browser ReadOnlyBrowser) {
			if _, err := browser.SnapshotDOM(context.Background()); !errors.Is(err, sentinel) {
				t.Fatalf("SnapshotDOM() error = %v", err)
			}
		})
	})
	t.Run("snapshot candidates", func(t *testing.T) {
		runtime := &Runtime{Driver: &matrixDriver{snapshot: func(context.Context) (heal.DOMSnapshot, error) { return snapshotFailureStub{err: sentinel}, nil }}, ReadOnlyBrowser: screenshotBrowserStub{}}
		runWithCompletionBrowser(t, runtime, func(browser ReadOnlyBrowser) {
			if _, err := browser.SnapshotDOM(context.Background()); !errors.Is(err, sentinel) {
				t.Fatalf("SnapshotDOM() error = %v", err)
			}
		})
	})

	observeTests := []struct {
		name      string
		configure func(*matrixDriver, *matrixElement)
	}{
		{name: "locate", configure: func(driver *matrixDriver, _ *matrixElement) {
			driver.locate = func(context.Context, fingerprint.ElementTargetSpec) (Element, error) { return nil, sentinel }
		}},
		{name: "exists", configure: func(_ *matrixDriver, element *matrixElement) { element.existsErr = sentinel }},
		{name: "visible", configure: func(_ *matrixDriver, element *matrixElement) { element.visibleErr = sentinel }},
		{name: "text", configure: func(_ *matrixDriver, element *matrixElement) { element.textErr = sentinel }},
		{name: "attribute", configure: func(_ *matrixDriver, element *matrixElement) { element.attributeErr = sentinel }},
	}
	for _, test := range observeTests {
		t.Run("observe "+test.name, func(t *testing.T) {
			element := &matrixElement{exists: true, visible: true, attributes: map[string]string{"role": "button"}}
			driver := &matrixDriver{element: element}
			test.configure(driver, element)
			runtime := &Runtime{Driver: driver, ReadOnlyBrowser: screenshotBrowserStub{}}
			runWithCompletionBrowser(t, runtime, func(browser ReadOnlyBrowser) {
				if _, err := browser.ObserveElement(context.Background(), fingerprint.ElementTargetSpec{ID: "node"}, []string{"role"}); !errors.Is(err, sentinel) {
					t.Fatalf("ObserveElement() error = %v", err)
				}
			})
		})
	}
}
