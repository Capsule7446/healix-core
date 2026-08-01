package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

type resultTimelineSink struct {
	startErr      error
	failStartAt   int
	startedEvents int
	finishErr     error
	events        []node.StepTimelineEvent
}

func (s *resultTimelineSink) RecordStepTimelineEvent(_ context.Context, event node.StepTimelineEvent) error {
	if event.Boundary == node.StepBoundaryStarted {
		s.startedEvents++
		if s.startErr != nil && (s.failStartAt == 0 || s.startedEvents == s.failStartAt) {
			return s.startErr
		}
	}
	if event.Boundary == node.StepBoundaryFinished && s.finishErr != nil {
		return s.finishErr
	}
	s.events = append(s.events, event)
	return nil
}

func TestRunProgramReportsTimelineStartFailureBeforeLeafExecution(t *testing.T) {
	driver := &engineTestDriver{}
	startErr := errors.New("start rejected")
	result, err := runProgramForTest(context.Background(), navigationCompiledEntry(mustInstanceID("run-timeline-start"), "timeline-start", "https://example.test"), Config{
		InstanceID:   mustInstanceID("run-timeline-start"),
		Driver:       driver,
		Recorder:     &engineTestRecorder{},
		StepTimeline: &resultTimelineSink{startErr: startErr},
	})
	if !fault.IsCode(err, node.CodeStepTimelineStartFailed) {
		t.Fatalf("error = %v, want timeline start error", err)
	}
	if result.ExecutionOutcome != ExecutionNotStarted || result.TimelineOutcome != TimelineStartFailed {
		t.Fatalf("result = %+v", result)
	}
	if driver.navigated != "" {
		t.Fatalf("leaf executed navigation to %q", driver.navigated)
	}
}

func TestRunProgramReportsFailureWhenLaterLeafTimelineStartFails(t *testing.T) {
	driver := &engineTestDriver{}
	startErr := errors.New("second start rejected")
	program := node.Program{Root: &node.WorkflowNode{
		NodeID: "two-leaves",
		Children: []node.Node{
			&node.StepNode{NodeID: "first", Action: node.Action{Kind: node.ActionNavigate, Value: "https://first.test"}},
			&node.StepNode{NodeID: "second", Action: node.Action{Kind: node.ActionNavigate, Value: "https://second.test"}},
		},
	}}
	result, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run-second-timeline-start"), program), Config{
		InstanceID:   mustInstanceID("run-second-timeline-start"),
		Driver:       driver,
		Recorder:     &engineTestRecorder{},
		StepTimeline: &resultTimelineSink{startErr: startErr, failStartAt: 2},
	})
	if !fault.IsCode(err, node.CodeStepTimelineStartFailed) {
		t.Fatalf("error = %v, want timeline start error", err)
	}
	if result.ExecutionOutcome != ExecutionFailed || result.TimelineOutcome != TimelineStartFailed {
		t.Fatalf("result = %+v", result)
	}
	if driver.navigated != "https://first.test" {
		t.Fatalf("navigated = %q, want only first leaf", driver.navigated)
	}
}

func TestRunProgramKeepsExecutionSuccessWhenTimelineFinishFails(t *testing.T) {
	finishErr := errors.New("finish rejected")
	result, err := runProgramForTest(context.Background(), navigationCompiledEntry(mustInstanceID("run-result"), "result", "https://example.test"), Config{
		InstanceID:   mustInstanceID("run-result"),
		Driver:       &engineTestDriver{},
		Recorder:     &engineTestRecorder{},
		StepTimeline: &resultTimelineSink{finishErr: finishErr},
	})
	if !fault.IsCode(err, node.CodeStepTimelineFinishFailed) {
		t.Fatalf("error = %v, want timeline finish error", err)
	}
	if result.ExecutionOutcome != ExecutionSucceeded || result.TimelineOutcome != TimelineFinishFailed || result.RecordingOutcome != RecordingSucceeded {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunProgramRejectsTimelineWithoutRecorder(t *testing.T) {
	root := &runtimeCaptureNode{}
	result, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), node.Program{Root: root}), Config{
		InstanceID: mustInstanceID("run"), Driver: &engineTestDriver{}, StepTimeline: &resultTimelineSink{},
	})
	if !fault.IsCode(err, CodeTimelineConfigurationInvalid) {
		t.Fatalf("error = %v, want timeline configuration error", err)
	}
	if root.runs != 0 || result.ExecutionOutcome != ExecutionNotStarted {
		t.Fatalf("root runs = %d, result = %+v", root.runs, result)
	}
}

func TestExecutionOutcomePreservesSuccess(t *testing.T) {
	if got := executionOutcome(nil); got != ExecutionSucceeded {
		t.Fatalf("execution outcome = %s, want %s", got, ExecutionSucceeded)
	}
}

func TestExecutionOutcomePreservesBusinessFailure(t *testing.T) {
	if got := executionOutcome(errors.New("business failed")); got != ExecutionFailed {
		t.Fatalf("execution outcome = %s, want %s", got, ExecutionFailed)
	}
}

func TestRunProgramClassifiesEveryContextTerminationAsCanceled(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(err.Error(), func(t *testing.T) {
			root := &runtimeCaptureNode{err: err}
			result, runErr := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), node.Program{Root: root}), Config{
				InstanceID: mustInstanceID("run"), Driver: &engineTestDriver{},
			})
			if !errors.Is(runErr, err) || result.ExecutionOutcome != ExecutionCanceled || root.runs != 1 {
				t.Fatalf("RunProgram() = (%#v, %v), runs = %d", result, runErr, root.runs)
			}
		})
	}
}

func TestRunProgramRejectsNilRecorderTimelineBeforeExecution(t *testing.T) {
	root := &runtimeCaptureNode{}
	recorder := &engineTestRecorder{nilTimeline: true}
	result, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), node.Program{Root: root}), Config{
		InstanceID: mustInstanceID("run"), Driver: &engineTestDriver{}, Recorder: recorder, StepTimeline: &resultTimelineSink{},
	})
	if !fault.IsCode(err, CodeTimelineConfigurationInvalid) || strings.Contains(err.Error(), "nil timeline") {
		t.Fatalf("error = %v, want nil timeline configuration error", err)
	}
	if root.runs != 0 || !recorder.stopped || !recorder.retained {
		t.Fatalf("root/recorder = %d/%+v", root.runs, recorder)
	}
	if result.ExecutionOutcome != ExecutionNotStarted || result.RecordingOutcome != RecordingSucceeded || result.TimelineOutcome != TimelineStartFailed {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunProgramReportsTimelineStartFailureWhenRecorderStartFails(t *testing.T) {
	result, err := runProgramForTest(context.Background(), navigationCompiledEntry(mustInstanceID("run-start-failure"), "start-failure", "https://example.test"), Config{
		InstanceID:   mustInstanceID("run-start-failure"),
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

func TestRunProgramAllowsEmptyCompletionChainWithoutBrowser(t *testing.T) {
	root := &runtimeCaptureNode{}
	chain, err := node.NewNodeCompletionChain(node.NodeCompletionOptions{})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	result, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), node.Program{Root: root}), Config{
		InstanceID: mustInstanceID("run"), Driver: &engineTestDriver{}, CompletionChain: chain,
	})
	if err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	if root.runs != 1 || result.ExecutionOutcome != ExecutionSucceeded {
		t.Fatalf("root runs = %d, result = %+v", root.runs, result)
	}
}

func TestRunProgramClassifiesMissingCompletionBrowser(t *testing.T) {
	root := &runtimeCaptureNode{}
	chain, err := node.NewNodeCompletionChain(node.NodeCompletionOptions{}, completionHandlerNoop{})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	result, err := runProgramForTest(context.Background(), compiledEntry(mustInstanceID("run"), node.Program{Root: root}), Config{
		InstanceID: mustInstanceID("run"), Driver: &engineTestDriver{}, CompletionChain: chain,
	})
	if !fault.IsCode(err, CodeCompletionConfigurationInvalid) {
		t.Fatalf("error = %v, want completion configuration error", err)
	}
	if root.runs != 0 || result.ExecutionOutcome != ExecutionNotStarted {
		t.Fatalf("root runs = %d, result = %+v", root.runs, result)
	}
}

func TestRunProgramReportsObserverFailureWithoutChangingExecutionOutcome(t *testing.T) {
	observerErr := errors.New("observer failed")
	chain, err := node.NewNodeCompletionChain(node.NodeCompletionOptions{}, completionHandlerNoop{})
	if err != nil {
		t.Fatalf("NewNodeCompletionChain: %v", err)
	}
	result, err := runProgramForTest(context.Background(), navigationCompiledEntry(mustInstanceID("run-observer"), "observer", "https://example.test"), Config{
		InstanceID:         mustInstanceID("run-observer"),
		Driver:             &engineTestDriver{},
		CompletionChain:    chain,
		ReadOnlyBrowser:    readOnlyBrowserNoop{},
		CompletionObserver: completionObserverError{err: observerErr},
	})
	if !fault.IsCode(err, node.CodeNodeCompletionObservation) || !errors.Is(err, observerErr) {
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
func (readOnlyBrowserNoop) ObserveElement(context.Context, fingerprint.ElementTargetSpec, []string) (node.ElementObservation, error) {
	return node.ElementObservation{}, nil
}
