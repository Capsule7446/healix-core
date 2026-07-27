package node

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

type nodePhaseFailingFacts struct {
	testFacts
	nodeID string
	phase  Phase
	err    error
}

func (facts *nodePhaseFailingFacts) RecordProgress(ctx context.Context, fence domainexecution.WorkerFence, event Event) error {
	if event.NodeID == facts.nodeID && event.Phase == facts.phase {
		return facts.err
	}
	return facts.testFacts.RecordProgress(ctx, fence, event)
}

func (facts *nodePhaseFailingFacts) CommitTerminal(ctx context.Context, fence domainexecution.WorkerFence, commit TerminalCommit) error {
	if commit.Event.NodeID == facts.nodeID && commit.Event.Phase == facts.phase {
		return facts.err
	}
	return facts.testFacts.CommitTerminal(ctx, fence, commit)
}

type operationObserverFunc func(context.Context, OperationObservation) error

func (observe operationObserverFunc) RecordOperation(ctx context.Context, observation OperationObservation) error {
	return observe(ctx, observation)
}

func TestWaitNodeRunDependencyAndLifecycleFailureMatrix(t *testing.T) {
	persistenceErr := errors.New("execution facts unavailable")
	waitErr := errors.New("network idle failed")
	tests := []struct {
		name       string
		wait       *WaitNode
		runtime    *Runtime
		wantCauses []error
		wantText   string
	}{
		{
			name: "running event rejected",
			wait: &WaitNode{NodeID: "wait", Kind: WaitSleep},
			runtime: &Runtime{Facts: &nodePhaseFailingFacts{
				nodeID: "wait", phase: PhaseRunning, err: persistenceErr,
			}},
			wantCauses: []error{persistenceErr},
		},
		{
			name:     "leaf lifecycle cannot start without recording timeline",
			wait:     &WaitNode{NodeID: "wait", Kind: WaitSleep},
			runtime:  &Runtime{StepTimeline: &timelineSinkStub{}},
			wantText: "recording timeline is required",
		},
		{
			name: "leaf lifecycle and terminal event both fail",
			wait: &WaitNode{NodeID: "wait", Kind: WaitSleep},
			runtime: &Runtime{
				StepTimeline: &timelineSinkStub{},
				Facts:        &nodePhaseFailingFacts{nodeID: "wait", phase: PhaseFailed, err: persistenceErr},
			},
			wantCauses: []error{persistenceErr},
			wantText:   "recording timeline is required",
		},
		{
			name: "wait operation and terminal event both fail",
			wait: &WaitNode{NodeID: "wait", Kind: WaitNetworkIdle, Timeout: time.Second},
			runtime: &Runtime{
				Driver: &matrixDriver{networkIdleErr: waitErr},
				Facts:  &nodePhaseFailingFacts{nodeID: "wait", phase: PhaseFailed, err: persistenceErr},
			},
			wantCauses: []error{waitErr, persistenceErr},
		},
		{
			name: "succeeded event rejected",
			wait: &WaitNode{NodeID: "wait", Kind: WaitSleep},
			runtime: &Runtime{Facts: &nodePhaseFailingFacts{
				nodeID: "wait", phase: PhaseSucceeded, err: persistenceErr,
			}},
			wantCauses: []error{persistenceErr},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.wait.Run(context.Background(), test.runtime)
			if err == nil {
				t.Fatal("Run() unexpectedly succeeded")
			}
			for _, cause := range test.wantCauses {
				if !errors.Is(err, cause) {
					t.Fatalf("Run() error = %v, want cause %v", err, cause)
				}
			}
			if test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("Run() error = %v, want text %q", err, test.wantText)
			}
		})
	}
}

func TestWaitNodeRunCancellationDuringPublicStepInterval(t *testing.T) {
	runtime := &Runtime{StepInterval: time.Hour}
	if err := (&WaitNode{NodeID: "first", Kind: WaitSleep}).Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (&WaitNode{NodeID: "second", Kind: WaitSleep}).Run(ctx, runtime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}
}

func TestWaitNodeRunPropagatesVisibilityReadFailure(t *testing.T) {
	readErr := errors.New("visibility read failed")
	wait := &WaitNode{NodeID: "wait", Kind: WaitElementVisible, Target: fingerprint.NodeSpec{ID: "target"}, Timeout: time.Second}
	runtime := &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true, visibleErr: readErr}}}
	if err := wait.Run(context.Background(), runtime); !errors.Is(err, readErr) {
		t.Fatalf("Run() error = %v, want visibility read failure", err)
	}
}

func TestCompositeNodeRunDependencyFailureMatrix(t *testing.T) {
	persistenceErr := errors.New("execution facts unavailable")
	childErr := errors.New("child failed")
	tests := []struct {
		name   string
		nodeID string
		make   func(Node) Node
	}{
		{
			name:   "repeat",
			nodeID: "repeat",
			make: func(child Node) Node {
				children := []Node{}
				if child != nil {
					children = append(children, child)
				}
				return &RepeatNode{NodeID: "repeat", Times: 1, Children: children}
			},
		},
		{
			name:   "workflow",
			nodeID: "workflow",
			make: func(child Node) Node {
				children := []Node{}
				if child != nil {
					children = append(children, child)
				}
				return &WorkflowNode{NodeID: "workflow", Children: children}
			},
		},
		{
			name:   "workflow call",
			nodeID: "call",
			make: func(child Node) Node {
				children := []Node{}
				if child != nil {
					children = append(children, child)
				}
				return &WorkflowCallNode{NodeID: "call", Target: &WorkflowNode{NodeID: "target", Children: children}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name+" rejects running event failure", func(t *testing.T) {
			facts := &nodePhaseFailingFacts{nodeID: test.nodeID, phase: PhaseRunning, err: persistenceErr}
			err := test.make(nil).Run(context.Background(), &Runtime{Facts: facts})
			if !errors.Is(err, persistenceErr) {
				t.Fatalf("Run() error = %v, want persistence failure", err)
			}
		})

		t.Run(test.name+" preserves child and terminal failures", func(t *testing.T) {
			facts := &nodePhaseFailingFacts{nodeID: test.nodeID, phase: PhaseFailed, err: persistenceErr}
			err := test.make(&matrixNode{id: "child", err: childErr}).Run(context.Background(), &Runtime{Facts: facts})
			if !errors.Is(err, childErr) || !errors.Is(err, persistenceErr) {
				t.Fatalf("Run() error = %v, want child and persistence failures", err)
			}
		})

		t.Run(test.name+" rejects succeeded event failure", func(t *testing.T) {
			facts := &nodePhaseFailingFacts{nodeID: test.nodeID, phase: PhaseSucceeded, err: persistenceErr}
			err := test.make(nil).Run(context.Background(), &Runtime{Facts: facts})
			if !errors.Is(err, persistenceErr) {
				t.Fatalf("Run() error = %v, want persistence failure", err)
			}
		})
	}
}

func TestWorkflowNodeOwnsEmptyScopeWithoutLeakingIt(t *testing.T) {
	capture := &parameterCaptureNode{}
	runtime := &Runtime{}
	workflow := &WorkflowNode{NodeID: "root", OwnsParameterScope: true, Parameters: nil, Children: []Node{capture}}
	if err := workflow.Run(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if capture.got == nil || len(capture.got) != 0 {
		t.Fatalf("owned parameter scope = %#v, want non-nil empty scope", capture.got)
	}
	if runtime.Parameters() != nil {
		t.Fatalf("owned scope leaked after workflow: %#v", runtime.Parameters())
	}
}

func TestPollerDefaultBoundariesAndRetainedErrors(t *testing.T) {
	t.Run("non-positive timeout and interval use defaults", func(t *testing.T) {
		calls := 0
		err := (Poller{}).Run(context.Background(), 0, func(context.Context) (bool, error) {
			calls++
			return true, nil
		})
		if err != nil || calls != 1 {
			t.Fatalf("Run() calls = %d, error = %v", calls, err)
		}
	})

	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "element not found", err: ErrElementNotFound},
		{name: "transient driver", err: TransientError("locate", errors.New("temporary"))},
	} {
		t.Run(test.name+" is retained in timeout", func(t *testing.T) {
			err := (Poller{Interval: time.Millisecond}).Run(context.Background(), 3*time.Millisecond, func(context.Context) (bool, error) {
				return false, test.err
			})
			if !errors.Is(err, test.err) || errorKind(err) != ErrorTimeout {
				t.Fatalf("Run() error = %v, want retained %v and timeout classification", err, test.err)
			}
		})
	}
}

func TestStepNodeRunPublicFailureMatrix(t *testing.T) {
	persistenceErr := errors.New("execution facts unavailable")
	observerErr := errors.New("operation observer unavailable")
	driverErr := errors.New("driver failed")
	navigateObserverCalls := 0
	pressObserverCalls := 0
	actionObserverCalls := 0
	missingTimelineRuntime := func() *Runtime {
		return &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, StepTimeline: &timelineSinkStub{}}
	}
	tests := []struct {
		name     string
		step     *StepNode
		runtime  *Runtime
		want     error
		wantText string
	}{
		{
			name:    "positive timeout bounds execution",
			step:    &StepNode{NodeID: "step", Timeout: time.Second, Target: fingerprint.NodeSpec{ID: "target"}},
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}},
		},
		{
			name: "running event rejected",
			step: &StepNode{NodeID: "step", Target: fingerprint.NodeSpec{ID: "target"}},
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, Facts: &nodePhaseFailingFacts{
				nodeID: "step", phase: PhaseRunning, err: persistenceErr,
			}},
			want: persistenceErr,
		},
		{
			name:     "leaf lifecycle requires timeline",
			step:     &StepNode{NodeID: "step", Target: fingerprint.NodeSpec{ID: "target"}},
			runtime:  missingTimelineRuntime(),
			wantText: "recording timeline is required",
		},
		{
			name:     "action value interpolation failure",
			step:     &StepNode{NodeID: "step", Target: fingerprint.NodeSpec{ID: "target"}, Action: Action{Kind: ActionInput, Value: "${missing}"}},
			runtime:  &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}},
			wantText: "missing",
		},
		{
			name:     "select value interpolation failure",
			step:     &StepNode{NodeID: "step", Target: fingerprint.NodeSpec{ID: "target"}, Action: Action{Kind: ActionSelect, Values: []string{"${missing}"}}},
			runtime:  &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}},
			wantText: "missing",
		},
		{
			name:     "invalid navigation URL",
			step:     &StepNode{NodeID: "step", Action: Action{Kind: ActionNavigate, Value: "relative/path"}},
			runtime:  &Runtime{Driver: &matrixDriver{}},
			wantText: "invalid navigation URL",
		},
		{
			name: "navigate observation is best effort",
			step: &StepNode{NodeID: "step", Action: Action{Kind: ActionNavigate, Value: "https://example.test"}},
			runtime: &Runtime{Driver: &matrixDriver{}, OperationObserver: operationObserverFunc(func(context.Context, OperationObservation) error {
				navigateObserverCalls++
				return observerErr
			})},
		},
		{
			name:    "press driver failure",
			step:    &StepNode{NodeID: "step", Action: Action{Kind: ActionPress, Value: "Enter"}},
			runtime: &Runtime{Driver: &matrixDriver{pressErr: driverErr}},
			want:    driverErr,
		},
		{
			name: "press observation is best effort",
			step: &StepNode{NodeID: "step", Action: Action{Kind: ActionPress, Value: "Enter"}},
			runtime: &Runtime{Driver: &matrixDriver{}, OperationObserver: operationObserverFunc(func(context.Context, OperationObservation) error {
				pressObserverCalls++
				return observerErr
			})},
		},
		{
			name: "optional skip succeeded event rejected",
			step: &StepNode{NodeID: "step", Optional: true, Target: fingerprint.NodeSpec{ID: "target"}},
			runtime: &Runtime{
				Driver: &matrixDriver{locate: func(context.Context, fingerprint.NodeSpec) (Element, error) { return nil, ErrElementNotFound }},
				Facts:  &nodePhaseFailingFacts{nodeID: "step", phase: PhaseSucceeded, err: persistenceErr},
			},
			want: persistenceErr,
		},
		{
			name: "healing event rejected",
			step: &StepNode{NodeID: "step", Target: fingerprint.NodeSpec{ID: "target"}},
			runtime: &Runtime{
				Driver: &matrixDriver{locate: func(context.Context, fingerprint.NodeSpec) (Element, error) { return nil, ErrElementNotFound }},
				Healer: &testHealer{},
				Facts:  &nodePhaseFailingFacts{nodeID: "step", phase: PhaseHealing, err: persistenceErr},
			},
			want: persistenceErr,
		},
		{
			name: "transitioning event rejected",
			step: &StepNode{NodeID: "step", Target: fingerprint.NodeSpec{ID: "target"}},
			runtime: &Runtime{
				Driver: &matrixDriver{element: &matrixElement{exists: true}},
				Facts:  &nodePhaseFailingFacts{nodeID: "step", phase: PhaseTransitioning, err: persistenceErr},
			},
			want: persistenceErr,
		},
		{
			name: "action observation is best effort",
			step: &StepNode{NodeID: "step", Target: fingerprint.NodeSpec{ID: "target"}},
			runtime: &Runtime{
				Driver: &matrixDriver{element: &matrixElement{exists: true}},
				OperationObserver: operationObserverFunc(func(context.Context, OperationObservation) error {
					actionObserverCalls++
					return observerErr
				}),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.step.Run(context.Background(), test.runtime)
			if test.want == nil && test.wantText == "" {
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Run() unexpectedly succeeded")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Run() error = %v, want cause %v", err, test.want)
			}
			if test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("Run() error = %v, want text %q", err, test.wantText)
			}
		})
	}
	if navigateObserverCalls != 1 || pressObserverCalls != 1 || actionObserverCalls != 1 {
		t.Fatalf("best-effort observer calls = navigate:%d press:%d action:%d, want 1 each", navigateObserverCalls, pressObserverCalls, actionObserverCalls)
	}
}

func validValidationNode(id string) *ValidationNode {
	return &ValidationNode{
		NodeID:    id,
		Target:    fingerprint.NodeSpec{ID: "target"},
		Assertion: ValidationAssertion{Kind: "exists"},
		MaxWait:   500 * time.Millisecond,
		Stability: time.Nanosecond,
	}
}

func validValidationGroup(id string) *ValidationGroupNode {
	return &ValidationGroupNode{
		NodeID:    id,
		Branches:  []ValidationBranch{{ID: "branch", Nodes: []*ValidationNode{validValidationNode("member")}}},
		MaxWait:   500 * time.Millisecond,
		Stability: time.Nanosecond,
	}
}

func TestValidationNodeRunDependencyAndLifecycleFailureMatrix(t *testing.T) {
	persistenceErr := errors.New("execution facts unavailable")
	observerErr := errors.New("operation observer unavailable")
	observerCalls := 0
	tests := []struct {
		name     string
		runtime  *Runtime
		want     error
		wantText string
	}{
		{
			name: "running event rejected",
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, Facts: &nodePhaseFailingFacts{
				nodeID: "validation", phase: PhaseRunning, err: persistenceErr,
			}},
			want: persistenceErr,
		},
		{
			name:     "leaf lifecycle requires timeline",
			runtime:  &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, StepTimeline: &timelineSinkStub{}},
			wantText: "recording timeline is required",
		},
		{
			name: "validating event rejected",
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, Facts: &nodePhaseFailingFacts{
				nodeID: "validation", phase: PhaseValidating, err: persistenceErr,
			}},
			want: persistenceErr,
		},
		{
			name: "operation observation is best effort",
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, OperationObserver: operationObserverFunc(func(context.Context, OperationObservation) error {
				observerCalls++
				return observerErr
			})},
		},
		{
			name: "succeeded event rejected",
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, Facts: &nodePhaseFailingFacts{
				nodeID: "validation", phase: PhaseSucceeded, err: persistenceErr,
			}},
			want: persistenceErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validValidationNode("validation").Run(context.Background(), test.runtime)
			if test.want == nil && test.wantText == "" {
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Run() unexpectedly succeeded")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Run() error = %v, want cause %v", err, test.want)
			}
			if test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("Run() error = %v, want text %q", err, test.wantText)
			}
		})
	}
	if observerCalls != 1 {
		t.Fatalf("best-effort observer calls = %d, want 1", observerCalls)
	}
}

func TestValidationGroupNodeRunDependencyAndLifecycleFailureMatrix(t *testing.T) {
	persistenceErr := errors.New("execution facts unavailable")
	tests := []struct {
		name     string
		runtime  *Runtime
		wantText string
	}{
		{
			name: "running event rejected",
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, Facts: &nodePhaseFailingFacts{
				nodeID: "group", phase: PhaseRunning, err: persistenceErr,
			}},
		},
		{
			name:     "leaf lifecycle requires timeline",
			runtime:  &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, StepTimeline: &timelineSinkStub{}},
			wantText: "recording timeline is required",
		},
		{
			name: "validating event rejected",
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, Facts: &nodePhaseFailingFacts{
				nodeID: "group", phase: PhaseValidating, err: persistenceErr,
			}},
		},
		{
			name: "succeeded event rejected",
			runtime: &Runtime{Driver: &matrixDriver{element: &matrixElement{exists: true}}, Facts: &nodePhaseFailingFacts{
				nodeID: "group", phase: PhaseSucceeded, err: persistenceErr,
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validValidationGroup("group").Run(context.Background(), test.runtime)
			if err == nil {
				t.Fatal("Run() unexpectedly succeeded")
			}
			if test.wantText != "" {
				if !strings.Contains(err.Error(), test.wantText) {
					t.Fatalf("Run() error = %v, want text %q", err, test.wantText)
				}
			} else if !errors.Is(err, persistenceErr) {
				t.Fatalf("Run() error = %v, want persistence failure", err)
			}
		})
	}
}

func TestWorkflowCallCarriesEnvironmentValuesThroughPublicNestedScopes(t *testing.T) {
	capture := &parameterCaptureNode{}
	call := &WorkflowCallNode{
		NodeID:      "call",
		Target:      &WorkflowNode{NodeID: "target", Children: []Node{capture}},
		Values:      map[string]parameter.Value{"input": parameter.TextValue("value")},
		Constraints: map[string]parameter.Constraint{"input": {Type: parameter.Text}},
	}
	outer := &WorkflowNode{
		NodeID:             "outer",
		OwnsParameterScope: true,
		Parameters:         map[string]parameter.Value{"env.region": parameter.TextValue("east")},
		Children:           []Node{call},
	}
	if err := outer.Run(context.Background(), &Runtime{}); err != nil {
		t.Fatal(err)
	}
	if capture.got["input"].Text() != "value" || capture.got["env.region"].Text() != "east" {
		t.Fatalf("workflow call scope = %#v", capture.got)
	}
}
