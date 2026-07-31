package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/node"
)

const (
	CodeTimelineConfigurationInvalid   fault.Code = "EXECUTION_TIMELINE_CONFIGURATION_INVALID"
	CodeCompletionConfigurationInvalid fault.Code = "EXECUTION_COMPLETION_CONFIGURATION_INVALID"
)

func timelineConfigurationError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeTimelineConfigurationInvalid, "execution timeline configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func completionConfigurationError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeCompletionConfigurationInvalid, "execution completion configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func runProgram(ctx context.Context, program node.Program, cfg Config) (result RunResult, runErr error) {
	result = RunResult{ExecutionOutcome: ExecutionNotStarted, RecordingOutcome: RecordingDisabled, TimelineOutcome: TimelineDisabled}
	if ctx == nil {
		return result, fmt.Errorf("context is required")
	}
	if err := validateConfig(program, cfg); err != nil {
		return result, err
	}

	var timeline node.RecordingTimeline
	if cfg.Recorder != nil {
		var err error
		timeline, err = cfg.Recorder.Start(ctx, cfg.RunID)
		if err != nil {
			result.RecordingOutcome = RecordingStartFailed
			if cfg.StepTimeline != nil {
				result.TimelineOutcome = TimelineStartFailed
			}
			return result, fmt.Errorf("start recorder: %w", err)
		}
		result.RecordingOutcome = RecordingSucceeded
		defer func() {
			cleanupCtx, cancel := detachedTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := cfg.Recorder.Stop(cleanupCtx, true); err != nil {
				result.RecordingOutcome = RecordingStopFailed
				runErr = errors.Join(runErr, fmt.Errorf("stop recorder: %w", err))
			}
		}()
	}
	if cfg.StepTimeline != nil {
		if timeline == nil {
			result.TimelineOutcome = TimelineStartFailed
			return result, timelineConfigurationError()
		}
		result.TimelineOutcome = TimelineComplete
	}

	rt := newRuntime(program, cfg, timeline)
	runErr = program.Root.Run(ctx, rt)
	result.ExecutionOutcome = executionOutcome(node.LeafExecutionError(runErr))
	if fault.IsCode(runErr, node.CodeStepTimelineStartFailed) {
		if !rt.LeafExecutionStarted() {
			result.ExecutionOutcome = ExecutionNotStarted
		}
		result.TimelineOutcome = TimelineStartFailed
	}
	if fault.IsCode(runErr, node.CodeStepTimelineFinishFailed) {
		result.TimelineOutcome = TimelineFinishFailed
	}
	return result, runErr
}

func validateConfig(program node.Program, cfg Config) error {
	if cfg.RunID == "" {
		return fmt.Errorf("run ID is required")
	}
	if cfg.Facts != nil && cfg.ClaimToken == "" {
		return fmt.Errorf("claim token is required when execution facts are enabled")
	}
	if cfg.Driver == nil {
		return fmt.Errorf("driver is required")
	}
	if cfg.StepInterval < 0 {
		return fmt.Errorf("step interval must not be negative")
	}
	if program.Root == nil {
		return fmt.Errorf("program root is required")
	}
	if cfg.StepTimeline != nil && cfg.Recorder == nil {
		return timelineConfigurationError()
	}
	if cfg.CompletionChain.HasHandlers() && cfg.ReadOnlyBrowser == nil {
		return completionConfigurationError()
	}
	return nil
}

func executionOutcome(err error) ExecutionOutcome {
	if err == nil {
		return ExecutionSucceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ExecutionCanceled
	}
	return ExecutionFailed
}

func newRuntime(program node.Program, cfg Config, timeline node.RecordingTimeline) *node.Runtime {
	return &node.Runtime{
		RunID:              cfg.RunID,
		ClaimToken:         cfg.ClaimToken,
		StepInterval:       cfg.StepInterval,
		Specs:              program.Specs,
		Driver:             cfg.Driver,
		Healer:             cfg.Healer,
		Recorder:           cfg.Recorder,
		Facts:              cfg.Facts,
		Timeline:           timeline,
		StepTimeline:       cfg.StepTimeline,
		CompletionChain:    cfg.CompletionChain,
		ReadOnlyBrowser:    cfg.ReadOnlyBrowser,
		CompletionObserver: cfg.CompletionObserver,
		Scratchpad:         map[string]any{},
	}
}
