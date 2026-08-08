package engine

import (
	"context"
	"errors"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/node"
)

const (
	// CodeTimelineConfigurationInvalid 表示步骤时间线配置缺少录制器或其他必需条件。
	CodeTimelineConfigurationInvalid fault.Code = "EXECUTION_TIMELINE_CONFIGURATION_INVALID"
	// CodeCompletionConfigurationInvalid 表示完成处理链缺少只读浏览器配置。
	CodeCompletionConfigurationInvalid fault.Code = "EXECUTION_COMPLETION_CONFIGURATION_INVALID"
	// CodeRuntimeConfigurationInvalid 表示协调器对 Config 或 Program 的运行时配置校验失败，属于启动前
	// 必须满足的前置条件。
	CodeRuntimeConfigurationInvalid fault.Code = "EXECUTION_RUNTIME_CONFIGURATION_INVALID"
	// CodeSchedulingAdapterUnavailable 表示录制器启动或停止失败，宿主调度适配器当前不可用。
	CodeSchedulingAdapterUnavailable fault.Code = "EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE"
)

// timelineConfigurationError 构造步骤时间线配置无效的前置条件错误。
func timelineConfigurationError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeTimelineConfigurationInvalid, "execution timeline configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// completionConfigurationError 构造完成处理配置无效的前置条件错误。
func completionConfigurationError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeCompletionConfigurationInvalid, "execution completion configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// runtimeConfigurationInvalidError 将协调器运行时配置校验失败分类为固定的前置条件错误；具体字段和
// 原因保留在私有 cause 中，公共消息始终使用注册表固定文本。
func runtimeConfigurationInvalidError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.FailedPrecondition, CodeRuntimeConfigurationInvalid, "execution runtime configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// classifySchedulingAdapterFailure 为未分类的录制器启动/停止失败补上注册错误码，并让适配器已分类的
// 错误原样通过，避免在边界掩盖已有错误码。
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

// runProgram 启动录制和步骤时间线，运行节点程序，并在退出时停止录制并汇总结果状态。
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

// validateConfig 校验运行实例身份、执行端口、时间线、完成处理和自愈相关配置。
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
	// 启用自愈时必须提供页面定位端口，以便安全检查确认实时页面位置。
	if cfg.Healer != nil && cfg.PageLocator == nil {
		return runtimeConfigurationInvalidError(errors.New("page locator is required when healing is enabled"))
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

// executionOutcome 将节点运行错误映射为成功、取消或失败结果。
func executionOutcome(err error) ExecutionOutcome {
	if err == nil {
		return OutcomeSucceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return OutcomeCanceled
	}
	return OutcomeFailed
}

// newRuntime 将执行配置和可选录制时间线组装为节点运行时，并初始化独立 scratchpad。
func newRuntime(program node.Program, cfg Config, timeline node.RecordingTimeline) *node.Runtime {
	return &node.Runtime{
		InstanceID:         cfg.InstanceID,
		EntryID:            cfg.EntryID,
		ClaimToken:         cfg.ClaimToken,
		StepInterval:       cfg.StepInterval,
		Specs:              program.Specs,
		Driver:             cfg.Driver,
		PageLocator:        cfg.PageLocator,
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
