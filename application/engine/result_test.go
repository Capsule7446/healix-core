package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

type resultTimelineSink struct {
	startErr  error
	finishErr error
	events    []node.StepTimelineEvent
}

func (s *resultTimelineSink) RecordStepTimelineEvent(_ context.Context, event node.StepTimelineEvent) error {
	if event.Boundary == node.StepBoundaryStarted && s.startErr != nil {
		return s.startErr
	}
	if event.Boundary == node.StepBoundaryFinished && s.finishErr != nil {
		return s.finishErr
	}
	s.events = append(s.events, event)
	return nil
}

func TestRunProgramWithResultReportsTimelineStartFailureBeforeLeafExecution(t *testing.T) {
	driver := &engineTestDriver{}
	startErr := errors.New("start rejected")
	result, err := RunProgramWithResult(context.Background(), navigationProgram("timeline-start", "https://example.test"), Config{
		RunID:        "run-timeline-start",
		Driver:       driver,
		Recorder:     &engineTestRecorder{},
		StepTimeline: &resultTimelineSink{startErr: startErr},
	})
	if !errors.Is(err, node.ErrStepTimelineStart) {
		t.Fatalf("error = %v, want timeline start error", err)
	}
	if result.ExecutionOutcome != ExecutionNotStarted || result.TimelineOutcome != TimelineStartFailed {
		t.Fatalf("result = %+v", result)
	}
	if driver.navigated != "" {
		t.Fatalf("leaf executed navigation to %q", driver.navigated)
	}
}

func TestRunProgramWithResultKeepsExecutionSuccessWhenTimelineFinishFails(t *testing.T) {
	finishErr := errors.New("finish rejected")
	result, err := RunProgramWithResult(context.Background(), navigationProgram("result", "https://example.test"), Config{
		RunID:        "run-result",
		Driver:       &engineTestDriver{},
		Recorder:     &engineTestRecorder{},
		StepTimeline: &resultTimelineSink{finishErr: finishErr},
	})
	if !errors.Is(err, node.ErrStepTimelineFinish) {
		t.Fatalf("error = %v, want timeline finish error", err)
	}
	if result.ExecutionOutcome != ExecutionSucceeded || result.TimelineOutcome != TimelineFinishFailed || result.RecordingOutcome != RecordingSucceeded {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunProgramWithResultRejectsTimelineWithoutRecorder(t *testing.T) {
	root := &runtimeCaptureNode{}
	result, err := RunProgramWithResult(context.Background(), node.Program{Root: root}, Config{
		RunID: "run", Driver: &engineTestDriver{}, StepTimeline: &resultTimelineSink{},
	})
	if !errors.Is(err, ErrTimelineConfiguration) {
		t.Fatalf("error = %v, want timeline configuration error", err)
	}
	if root.runs != 0 || result.ExecutionOutcome != ExecutionNotStarted {
		t.Fatalf("root runs = %d, result = %+v", root.runs, result)
	}
}

func TestExecutionOutcomePreservesBusinessFailureAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := executionOutcome(ctx, errors.New("business failed")); got != ExecutionFailed {
		t.Fatalf("execution outcome = %s, want %s", got, ExecutionFailed)
	}
}

func TestRunProgramWithResultReportsTimelineStartFailureWhenRecorderStartFails(t *testing.T) {
	result, err := RunProgramWithResult(context.Background(), navigationProgram("start-failure", "https://example.test"), Config{
		RunID:        "run-start-failure",
		Driver:       &engineTestDriver{},
		Recorder:     &engineTestRecorder{startErr: errors.New("start failed")},
		StepTimeline: &resultTimelineSink{},
	})
	if err == nil {
		t.Fatal("recorder start failure was accepted")
	}
	if result.ExecutionOutcome != ExecutionNotStarted || result.RecordingOutcome != RecordingStartFailed || result.TimelineOutcome != TimelineStartFailed {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunProgramWithResultAllowsEmptyCompletionChainWithoutBrowser(t *testing.T) {
	root := &runtimeCaptureNode{}
	chain, err := node.NewNodeCompletionChain(node.NodeCompletionOptions{})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	result, err := RunProgramWithResult(context.Background(), node.Program{Root: root}, Config{
		RunID: "run", Driver: &engineTestDriver{}, CompletionChain: chain,
	})
	if err != nil {
		t.Fatalf("RunProgramWithResult: %v", err)
	}
	if root.runs != 1 || result.ExecutionOutcome != ExecutionSucceeded {
		t.Fatalf("root runs = %d, result = %+v", root.runs, result)
	}
}

func TestRunProgramWithResultClassifiesMissingCompletionBrowser(t *testing.T) {
	root := &runtimeCaptureNode{}
	chain, err := node.NewNodeCompletionChain(node.NodeCompletionOptions{}, completionHandlerNoop{})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	result, err := RunProgramWithResult(context.Background(), node.Program{Root: root}, Config{
		RunID: "run", Driver: &engineTestDriver{}, CompletionChain: chain,
	})
	if !errors.Is(err, ErrCompletionConfiguration) {
		t.Fatalf("error = %v, want completion configuration error", err)
	}
	if root.runs != 0 || result.ExecutionOutcome != ExecutionNotStarted {
		t.Fatalf("root runs = %d, result = %+v", root.runs, result)
	}
}

func TestRunProgramWithResultReportsObserverFailureWithoutChangingExecutionOutcome(t *testing.T) {
	observerErr := errors.New("observer failed")
	chain, err := node.NewNodeCompletionChain(node.NodeCompletionOptions{}, completionHandlerNoop{})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	result, err := RunProgramWithResult(context.Background(), navigationProgram("observer", "https://example.test"), Config{
		RunID:              "run-observer",
		Driver:             &engineTestDriver{},
		CompletionChain:    chain,
		ReadOnlyBrowser:    readOnlyBrowserNoop{},
		CompletionObserver: completionObserverError{err: observerErr},
	})
	if !errors.Is(err, node.ErrNodeCompletionObservation) || !errors.Is(err, observerErr) {
		t.Fatalf("error = %v, want completion observation error", err)
	}
	if result.ExecutionOutcome != ExecutionSucceeded {
		t.Fatalf("execution outcome = %s", result.ExecutionOutcome)
	}
}

type completionHandlerNoop struct{}

func (completionHandlerNoop) Name() string                                             { return "noop" }
func (completionHandlerNoop) Handle(context.Context, node.NodeCompletionContext) error { return nil }

type completionObserverError struct{ err error }

func (o completionObserverError) RecordNodeCompletion(context.Context, node.NodeCompletionObservation) error {
	return o.err
}

type readOnlyBrowserNoop struct{}

func (readOnlyBrowserNoop) CaptureScreenshot(context.Context, node.ScreenshotOptions) (node.ScreenshotArtifact, error) {
	return node.ScreenshotArtifact{}, nil
}
func (readOnlyBrowserNoop) SnapshotDOM(context.Context) (heal.DOMSnapshot, error) { return nil, nil }
func (readOnlyBrowserNoop) ObserveElement(context.Context, fingerprint.NodeSpec, []string) (node.ElementObservation, error) {
	return node.ElementObservation{}, nil
}
