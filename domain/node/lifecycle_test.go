package node

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

type timelineStub struct {
	marks []TimelineMark
	next  int
}

func (t *timelineStub) Mark() TimelineMark {
	mark := t.marks[t.next]
	t.next++
	return mark
}

type timelineSinkStub struct {
	events []StepTimelineEvent
	errAt  StepBoundary
	err    error
}

func (s *timelineSinkStub) RecordStepTimelineEvent(_ context.Context, event StepTimelineEvent) error {
	if event.Boundary == s.errAt {
		if s.err != nil {
			return s.err
		}
		return errors.New("timeline rejected")
	}
	s.events = append(s.events, event)
	return nil
}

func TestStepTimelineEventValidate(t *testing.T) {
	tests := []struct {
		name  string
		event StepTimelineEvent
		valid bool
	}{
		{name: "started", event: StepTimelineEvent{Step: StepExecutionRef{RunID: "run", NodeID: "step", Occurrence: 1}, Boundary: StepBoundaryStarted, Mark: TimelineMark{Sequence: 1}}, valid: true},
		{name: "finished", event: StepTimelineEvent{Step: StepExecutionRef{RunID: "run", NodeID: "step", Occurrence: 1}, Boundary: StepBoundaryFinished, Outcome: StepOutcomeSucceeded, Mark: TimelineMark{Offset: time.Millisecond, Sequence: 2}}, valid: true},
		{name: "started with outcome", event: StepTimelineEvent{Step: StepExecutionRef{RunID: "run", NodeID: "step", Occurrence: 1}, Boundary: StepBoundaryStarted, Outcome: StepOutcomeSucceeded, Mark: TimelineMark{Sequence: 1}}},
		{name: "finished without outcome", event: StepTimelineEvent{Step: StepExecutionRef{RunID: "run", NodeID: "step", Occurrence: 1}, Boundary: StepBoundaryFinished, Mark: TimelineMark{Sequence: 2}}},
		{name: "invalid identity", event: StepTimelineEvent{Boundary: StepBoundaryStarted, Mark: TimelineMark{Sequence: 1}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.event.Validate() == nil; got != test.valid {
				t.Fatalf("Validate valid = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestCompletionChainBlocksNextLeafAndIgnoresHandlerFailure(t *testing.T) {
	order := []string{}
	chain, err := NewNodeCompletionChain(NodeCompletionOptions{},
		completionHandlerStub{name: "capture", handle: func(input NodeCompletionContext) error {
			order = append(order, "handler:"+input.Snapshot.Execution.NodeID)
			return errors.New("capture failed")
		}},
	)
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	rt := &Runtime{RunID: "run", CompletionChain: chain}
	for _, nodeID := range []string{"first", "second"} {
		lifecycle, beginErr := rt.beginLeafLifecycle(context.Background(), nodeID, "STEP", 1)
		if beginErr != nil {
			t.Fatalf("begin %s: %v", nodeID, beginErr)
		}
		order = append(order, "node:"+nodeID)
		if completeErr := lifecycle.Complete(context.Background(), nil); completeErr != nil {
			t.Fatalf("complete %s: %v", nodeID, completeErr)
		}
	}
	want := []string{"node:first", "handler:first", "node:second", "handler:second"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestCompletionChainPropagatesCancellationReceivedDuringHandler(t *testing.T) {
	started := make(chan struct{})
	observed := make(chan error, 1)
	chain, err := NewNodeCompletionChain(NodeCompletionOptions{HandlerTimeout: time.Second}, completionContextHandlerStub{
		name: "cancel-aware",
		handle: func(ctx context.Context, _ NodeCompletionContext) error {
			close(started)
			<-ctx.Done()
			observed <- ctx.Err()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		chain.run(ctx, NodeCompletionContext{})
		close(done)
	}()
	<-started
	cancel()
	select {
	case err := <-observed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handler context error = %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handler did not receive parent cancellation")
	}
	<-done
}

func TestReadOnlyBrowserSnapshotDOMFreezesCandidates(t *testing.T) {
	attributes := map[string]string{"role": "button"}
	path := []string{"main", "button"}
	framework := fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact}}
	source := &mutableDOMSnapshot{candidates: []heal.SnapshotCandidate{{
		Fingerprint: fingerprint.Fingerprint{
			Attributes: attributes,
			Path:       path,
			Framework:  framework,
		},
	}}}
	browser := runtimeReadOnlyBrowser{
		runtime: &Runtime{Driver: snapshotDriverStub{snapshot: source}},
	}

	snapshot, err := browser.SnapshotDOM(context.Background())
	if err != nil {
		t.Fatalf("SnapshotDOM: %v", err)
	}
	attributes["role"] = "link"
	path[0] = "aside"
	framework[0].Kind = fingerprint.FrameworkVue

	candidates, err := snapshot.Candidates(context.Background())
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	got := candidates[0].Fingerprint
	if got.Attributes["role"] != "button" || got.Path[0] != "main" || got.Framework[0].Kind != fingerprint.FrameworkReact {
		t.Fatalf("snapshot changed after capture: %+v", got)
	}
}

type mutableDOMSnapshot struct {
	candidates []heal.SnapshotCandidate
}

func (s *mutableDOMSnapshot) Candidates(context.Context) ([]heal.SnapshotCandidate, error) {
	return s.candidates, nil
}

type snapshotDriverStub struct {
	snapshot heal.DOMSnapshot
}

func (d snapshotDriverStub) Navigate(context.Context, string) error { return nil }
func (d snapshotDriverStub) Press(context.Context, string) error    { return nil }
func (d snapshotDriverStub) Locate(context.Context, fingerprint.NodeSpec) (Element, error) {
	return nil, nil
}
func (d snapshotDriverStub) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return d.snapshot, nil
}
func (d snapshotDriverStub) WaitNetworkIdle(context.Context) error { return nil }

func TestLeafLifecyclePreservesBusinessFailureAfterContextCancellation(t *testing.T) {
	original := errors.New("business failed")
	var observed NodeOutcome
	chain, err := NewNodeCompletionChain(NodeCompletionOptions{}, completionHandlerStub{
		name: "observe",
		handle: func(input NodeCompletionContext) error {
			observed = input.Snapshot.Outcome
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	lifecycle, err := (&Runtime{RunID: "run", CompletionChain: chain}).beginLeafLifecycle(context.Background(), "step", "STEP", 1)
	if err != nil {
		t.Fatalf("beginLeafLifecycle: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lifecycle.Complete(ctx, original); !errors.Is(err, original) {
		t.Fatalf("Complete error = %v, want original failure", err)
	}
	if observed != NodeOutcomeFailed {
		t.Fatalf("snapshot outcome = %s, want %s", observed, NodeOutcomeFailed)
	}
}

func TestCompletionChainIsolatesSnapshotErrorBetweenHandlers(t *testing.T) {
	original := errors.New("node failed")
	var observed string
	chain, err := NewNodeCompletionChain(NodeCompletionOptions{},
		completionHandlerStub{name: "mutate", handle: func(input NodeCompletionContext) error {
			input.Snapshot.Error.Message = "mutated"
			return nil
		}},
		completionHandlerStub{name: "observe", handle: func(input NodeCompletionContext) error {
			observed = input.Snapshot.Error.Message
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	lifecycle, err := (&Runtime{RunID: "run", CompletionChain: chain}).beginLeafLifecycle(context.Background(), "step", "STEP", 1)
	if err != nil {
		t.Fatalf("beginLeafLifecycle: %v", err)
	}
	if err := lifecycle.Complete(context.Background(), original); !errors.Is(err, original) {
		t.Fatalf("Complete error = %v, want original failure", err)
	}
	if observed != original.Error() {
		t.Fatalf("second handler observed error %q, want %q", observed, original.Error())
	}
}

func TestCompletionSnapshotDurationExcludesHandlerTime(t *testing.T) {
	var snapshot NodeExecutionSnapshot
	chain, err := NewNodeCompletionChain(NodeCompletionOptions{}, completionHandlerStub{name: "capture", handle: func(input NodeCompletionContext) error {
		snapshot = input.Snapshot
		time.Sleep(20 * time.Millisecond)
		return nil
	}})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	lifecycle, err := (&Runtime{RunID: "run", CompletionChain: chain}).beginLeafLifecycle(context.Background(), "step", "STEP", 1)
	if err != nil {
		t.Fatalf("beginLeafLifecycle: %v", err)
	}
	if err := lifecycle.Complete(context.Background(), nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if snapshot.Duration >= 20*time.Millisecond {
		t.Fatalf("snapshot duration %s includes handler time", snapshot.Duration)
	}
}

func TestLeafLifecycleRecordsTimelineAndRunsCompletionHandlersInOrder(t *testing.T) {
	timeline := &timelineStub{marks: []TimelineMark{{Sequence: 1}, {Offset: 5 * time.Millisecond, Sequence: 2}}}
	sink := &timelineSinkStub{}
	order := []string{}
	chain, err := NewNodeCompletionChain(NodeCompletionOptions{},
		completionHandlerStub{name: "first", handle: func(NodeCompletionContext) error { order = append(order, "first"); return errors.New("ignored") }},
		completionHandlerStub{name: "second", handle: func(NodeCompletionContext) error { order = append(order, "second"); return nil }},
	)
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	rt := &Runtime{RunID: "run", Timeline: timeline, StepTimeline: sink, CompletionChain: chain}

	lifecycle, err := rt.beginLeafLifecycle(context.Background(), "step", "STEP", 1)
	if err != nil {
		t.Fatalf("beginLeafLifecycle: %v", err)
	}
	runErr := lifecycle.Complete(context.Background(), nil)
	if runErr != nil {
		t.Fatalf("Complete changed node result: %v", runErr)
	}
	if !reflect.DeepEqual(order, []string{"first", "second"}) {
		t.Fatalf("handler order = %v", order)
	}
	if len(sink.events) != 2 || sink.events[0].Boundary != StepBoundaryStarted || sink.events[1].Boundary != StepBoundaryFinished {
		t.Fatalf("timeline events = %#v", sink.events)
	}
	if sink.events[1].Outcome != StepOutcomeSucceeded {
		t.Fatalf("finish outcome = %s", sink.events[1].Outcome)
	}
}

func TestLeafLifecycleStartFailurePreventsExecution(t *testing.T) {
	original := errors.New("timeline rejected")
	sink := &timelineSinkStub{errAt: StepBoundaryStarted, err: original}
	rt := &Runtime{RunID: "run", Timeline: &timelineStub{marks: []TimelineMark{{Sequence: 1}}}, StepTimeline: sink}
	_, err := rt.beginLeafLifecycle(context.Background(), "step", "STEP", 1)
	if !errors.Is(err, ErrStepTimelineStart) || !errors.Is(err, original) {
		t.Fatalf("error = %v, want start sentinel and original error", err)
	}
}

func TestLeafLifecycleFinishFailurePreservesOriginalFailure(t *testing.T) {
	nodeErr := errors.New("node failed")
	timelineErr := errors.New("timeline rejected")
	sink := &timelineSinkStub{errAt: StepBoundaryFinished, err: timelineErr}
	rt := &Runtime{RunID: "run", Timeline: &timelineStub{marks: []TimelineMark{{Sequence: 1}, {Sequence: 2}}}, StepTimeline: sink}
	lifecycle, err := rt.beginLeafLifecycle(context.Background(), "step", "STEP", 1)
	if err != nil {
		t.Fatalf("beginLeafLifecycle: %v", err)
	}
	err = lifecycle.Complete(context.Background(), nodeErr)
	if !errors.Is(err, nodeErr) || !errors.Is(err, ErrStepTimelineFinish) || !errors.Is(err, timelineErr) {
		t.Fatalf("error chain = %v, want node, finish sentinel, and timeline errors", err)
	}
}

type completionContextHandlerStub struct {
	name   string
	handle func(context.Context, NodeCompletionContext) error
}

func (h completionContextHandlerStub) Name() string { return h.name }
func (h completionContextHandlerStub) Handle(ctx context.Context, input NodeCompletionContext) error {
	return h.handle(ctx, input)
}

type completionHandlerStub struct {
	name   string
	handle func(NodeCompletionContext) error
}

func (h completionHandlerStub) Name() string { return h.name }
func (h completionHandlerStub) Handle(_ context.Context, input NodeCompletionContext) error {
	return h.handle(input)
}
