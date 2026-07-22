package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/node"
)

type resultTimelineSink struct {
	finishErr error
	events    []node.StepTimelineEvent
}

func (s *resultTimelineSink) RecordStepTimelineEvent(_ context.Context, event node.StepTimelineEvent) error {
	if event.Boundary == node.StepBoundaryFinished && s.finishErr != nil {
		return s.finishErr
	}
	s.events = append(s.events, event)
	return nil
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
