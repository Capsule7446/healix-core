package engine

import (
	"context"
	"errors"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/node"
)

const (
	CodeTimelineConfigurationInvalid   fault.Code = "EXECUTION_TIMELINE_CONFIGURATION_INVALID"
	CodeCompletionConfigurationInvalid fault.Code = "EXECUTION_COMPLETION_CONFIGURATION_INVALID"
	// CodeRuntimeConfigurationInvalid covers the coordinator's own constructor
	// checks on Config/Program: none of these are a caller argument the coordinator
	// can re-validate differently, so the remediation is always to repair the
	// runtime configuration before starting, hence FAILED_PRECONDITION.
	CodeRuntimeConfigurationInvalid fault.Code = "EXECUTION_RUNTIME_CONFIGURATION_INVALID"
	// CodeSchedulingAdapterUnavailable covers a recorder start/stop failure: the
	// host adapter, not the caller-supplied configuration, is unavailable, so the
	// remediation is retry rather than correcting an argument.
	CodeSchedulingAdapterUnavailable fault.Code = "EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE"
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

// runtimeConfigurationInvalidError classifies a coordinator constructor check.
// The detail (which field, or why) stays private on cause; the public message
// is always the fixed registry text.
func runtimeConfigurationInvalidError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.FailedPrecondition, CodeRuntimeConfigurationInvalid, "execution runtime configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// classifySchedulingAdapterFailure gives a bare recorder start/stop failure its
// registered code, and lets an already-classified failure through unchanged so
// this boundary never buries a code the adapter already produced.
func classifySchedulingAdapterFailure(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	err, constructionErr := fault.Wrap(cause, fault.Unavailable, CodeSchedulingAdapterUnavailable, "scheduling adapter is unavailable")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func runProgram(ctx context.Context, program node.Program, cfg Config) (result EntryResult, runErr error) {
	result = EntryResult{ExecutionOutcome: ExecutionNotStarted, RecordingOutcome: RecordingDisabled, TimelineOutcome: TimelineDisabled}
	if ctx == nil {
		return result, runtimeConfigurationInvalidError(errors.New("context is required"))
	}
	if err := validateConfig(program, cfg); err != nil {
		return result, err
	}

	var timeline node.RecordingTimeline
	if cfg.Recorder != nil {
		var err error
		timeline, err = cfg.Recorder.Start(ctx, cfg.InstanceID)
		if err != nil {
			result.RecordingOutcome = RecordingStartFailed
			if cfg.StepTimeline != nil {
				result.TimelineOutcome = TimelineStartFailed
			}
			return result, classifySchedulingAdapterFailure(err)
		}
		result.RecordingOutcome = RecordingSucceeded
		defer func() {
			cleanupCtx, cancel := detachedTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := cfg.Recorder.Stop(cleanupCtx, true); err != nil {
				result.RecordingOutcome = RecordingStopFailed
				runErr = errors.Join(runErr, classifySchedulingAdapterFailure(err))
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
	if cfg.InstanceID.Validate() != nil {
		return runtimeConfigurationInvalidError(errors.New("instance ID is required"))
	}
	if cfg.Facts != nil && cfg.ClaimToken == "" {
		return runtimeConfigurationInvalidError(errors.New("claim token is required when execution facts are enabled"))
	}
	if cfg.Driver == nil {
		return runtimeConfigurationInvalidError(errors.New("driver is required"))
	}
	if cfg.StepInterval < 0 {
		return runtimeConfigurationInvalidError(errors.New("step interval must not be negative"))
	}
	if program.Root == nil {
		return runtimeConfigurationInvalidError(errors.New("program root is required"))
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
		return OutcomeSucceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return OutcomeCanceled
	}
	return OutcomeFailed
}

func newRuntime(program node.Program, cfg Config, timeline node.RecordingTimeline) *node.Runtime {
	return &node.Runtime{
		InstanceID:         cfg.InstanceID,
		EntryID:            cfg.EntryID,
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
