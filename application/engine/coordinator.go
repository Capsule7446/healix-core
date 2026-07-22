package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/node"
)

var (
	ErrTimelineConfiguration   = errors.New("engine: invalid timeline configuration")
	ErrCompletionConfiguration = errors.New("engine: invalid completion configuration")
)

// RunCoordinator owns application-level lifecycle around one compiled Program.
type RunCoordinator struct{}

func (RunCoordinator) Run(ctx context.Context, program node.Program, cfg Config) (result RunResult, runErr error) {
	result = RunResult{ExecutionOutcome: ExecutionNotStarted, RecordingOutcome: RecordingDisabled, TimelineOutcome: TimelineDisabled}
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
			return result, fmt.Errorf("%w: recorder returned nil timeline", ErrTimelineConfiguration)
		}
		result.TimelineOutcome = TimelineComplete
	}

	rt := newRuntime(program, cfg, timeline)
	runErr = program.Root.Run(ctx, rt)
	result.ExecutionOutcome = executionOutcome(ctx, node.LeafExecutionError(runErr))
	if errors.Is(runErr, node.ErrStepTimelineStart) {
		result.ExecutionOutcome = ExecutionNotStarted
		result.TimelineOutcome = TimelineStartFailed
	}
	if errors.Is(runErr, node.ErrStepTimelineFinish) {
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
	if program.Root == nil {
		return fmt.Errorf("program root is required")
	}
	if cfg.StepTimeline != nil && cfg.Recorder == nil {
		return fmt.Errorf("%w: recorder is required when step timeline is enabled", ErrTimelineConfiguration)
	}
	if cfg.CompletionChain.HasHandlers() && cfg.ReadOnlyBrowser == nil {
		return fmt.Errorf("%w: read-only browser is required when completion handlers are enabled", ErrCompletionConfiguration)
	}
	return nil
}

func executionOutcome(ctx context.Context, err error) ExecutionOutcome {
	if err == nil {
		if ctx.Err() != nil {
			return ExecutionCanceled
		}
		return ExecutionSucceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ExecutionCanceled
	}
	return ExecutionFailed
}

func newRuntime(program node.Program, cfg Config, timeline node.RecordingTimeline) *node.Runtime {
	scratchpad := make(map[string]any, len(cfg.Variables))
	for name, value := range cfg.Variables {
		scratchpad[name] = value
	}
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
		Scratchpad:         scratchpad,
	}
}
