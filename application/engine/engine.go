// Package engine 是唯一的执行编排入口：把内存 Program 接到一次全新的 Runtime。
package engine

import (
	"context"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

const (
	// CodeExecutionAuthorityVerifierRequired 表示运行配置缺少执行权威校验器。
	CodeExecutionAuthorityVerifierRequired fault.Code = "EXECUTION_AUTHORITY_VERIFIER_REQUIRED"
	// CodeExecutionIdentityMismatch 表示编译入口、封存快照或工作线程执行身份不一致。
	CodeExecutionIdentityMismatch fault.Code = "EXECUTION_IDENTITY_MISMATCH"
)

// ExecutionIdentityMismatchError 构造编译入口与封存执行身份不一致的前置条件错误。
func ExecutionIdentityMismatchError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeExecutionIdentityMismatch,
		"execution identity does not match the sealed entry",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// ExecutionAuthorityVerifierRequiredError 构造缺少执行权威校验器的前置条件错误。
func ExecutionAuthorityVerifierRequiredError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeExecutionAuthorityVerifierRequired,
		"execution authority verifier is required",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// ExecutionAuthority 携带执行实例、快照摘要、入口和领取令牌的权威身份。
type ExecutionAuthority struct {
	InstanceID     execution.InstanceID
	SnapshotDigest string
	EntryID        execution.EntryID
	ClaimToken     string
}

// ExecutionAuthorityVerifier 定义在运行节点程序前校验执行权威的端口。
type ExecutionAuthorityVerifier interface {
	// VerifyExecutionAuthority 校验给定执行权威；失败时阻止运行。
	VerifyExecutionAuthority(context.Context, ExecutionAuthority) error
}

// Config 打包了一次 Program 执行所需的领域端口与运行变量。
type Config struct {
	// InstanceID、SnapshotDigest、EntryID 与 ClaimToken 必须来自本次已领取
	// 执行权的权威身份，不能从待执行的 CompiledEntry 反向填充。
	InstanceID        execution.InstanceID
	SnapshotDigest    string
	EntryID           execution.EntryID
	ClaimToken        string
	AuthorityVerifier ExecutionAuthorityVerifier
	Driver            node.Driver
	// PageLocator 报告实时页面位置，自愈安全评估据此判断页面是否仍在录制
	// 时的 Origin 上。启用自愈时必须提供。
	PageLocator node.PageLocator
	// Healer 由组合根注入；nil 表示关闭自愈。
	Healer             heal.Healer
	Recorder           node.Recorder
	Facts              node.ExecutionSink
	StepTimeline       node.StepTimelineSink
	CompletionChain    *node.NodeCompletionChain
	ReadOnlyBrowser    node.ReadOnlyBrowser
	CompletionObserver node.NodeCompletionObserver
	// StepInterval 是执行局部的节奏设置。它应用于叶子 Step 之间，
	// 不会取代显式的条件等待。
	StepInterval time.Duration
}

// ExecutionOutcome 表示节点程序的执行结果。
type ExecutionOutcome string

// RecordingOutcome 表示录制器的启动和停止结果。
type RecordingOutcome string

// TimelineOutcome 表示步骤时间线的配置、完成或观测结果。
type TimelineOutcome string

const (
	// OutcomeSucceeded 表示节点程序成功完成。
	OutcomeSucceeded ExecutionOutcome = "SUCCEEDED"
	// OutcomeFailed 表示节点程序以非取消错误失败。
	OutcomeFailed ExecutionOutcome = "FAILED"
	// OutcomeCanceled 表示节点程序因上下文取消或截止时间结束。
	OutcomeCanceled ExecutionOutcome = "CANCELED"
	// ExecutionNotStarted 表示引擎已知节点程序尚未开始。
	ExecutionNotStarted ExecutionOutcome = "NOT_STARTED"
	// ExecutionInterrupted 表示运行是否完成无法确定；RunProgram 不返回此状态，恢复流程通过
	// InterruptedEngineOutcome 为中途终止的 RUNNING 入口作出决策。
	ExecutionInterrupted ExecutionOutcome = "INTERRUPTED"

	// RecordingDisabled 表示未配置录制器。
	RecordingDisabled RecordingOutcome = "DISABLED"
	// RecordingSucceeded 表示录制器已启动并成功停止。
	RecordingSucceeded RecordingOutcome = "SUCCEEDED"
	// RecordingStartFailed 表示录制器启动失败。
	RecordingStartFailed RecordingOutcome = "START_FAILED"
	// RecordingStopFailed 表示录制器停止失败。
	RecordingStopFailed RecordingOutcome = "STOP_FAILED"
	// RecordingUnobserved 表示录制是否运行及其结束状态未知；RunProgram 不返回此状态，仅由
	// InterruptedEngineOutcome 在中断恢复流程中使用。
	RecordingUnobserved RecordingOutcome = "UNOBSERVED"

	// TimelineDisabled 表示未配置步骤时间线。
	TimelineDisabled TimelineOutcome = "DISABLED"
	// TimelineComplete 表示步骤时间线已完成。
	TimelineComplete TimelineOutcome = "COMPLETE"
	// TimelineStartFailed 表示步骤时间线启动失败。
	TimelineStartFailed TimelineOutcome = "START_FAILED"
	// TimelineFinishFailed 表示步骤时间线结束失败。
	TimelineFinishFailed TimelineOutcome = "FINISH_FAILED"
	// TimelineUnobserved 表示步骤时间线是否完成未知，仅由中断恢复流程使用。
	TimelineUnobserved TimelineOutcome = "UNOBSERVED"
)

// EntryResult 汇总一次入口运行的执行、录制和步骤时间线结果。
type EntryResult struct {
	ExecutionOutcome ExecutionOutcome
	RecordingOutcome RecordingOutcome
	TimelineOutcome  TimelineOutcome
}

// RunProgram 仅运行由 CompilePlan 产生的入口；在任何运行时端口可被观察前先校验身份和执行权威。
func RunProgram(ctx context.Context, entry CompiledEntry, cfg Config) (EntryResult, error) {
	result := EntryResult{ExecutionOutcome: ExecutionNotStarted, RecordingOutcome: RecordingDisabled, TimelineOutcome: TimelineDisabled}
	if entry.identity.instanceID.Validate() != nil ||
		entry.InstanceID != entry.identity.instanceID ||
		entry.SnapshotDigest != entry.identity.snapshotDigest ||
		entry.EntryID != entry.identity.entryID ||
		cfg.InstanceID != entry.identity.instanceID ||
		cfg.SnapshotDigest != entry.identity.snapshotDigest ||
		cfg.EntryID != entry.identity.entryID ||
		cfg.ClaimToken == "" {
		return result, ExecutionIdentityMismatchError()
	}
	if cfg.AuthorityVerifier == nil {
		return result, ExecutionAuthorityVerifierRequiredError()
	}
	authority := ExecutionAuthority{
		InstanceID: cfg.InstanceID, SnapshotDigest: cfg.SnapshotDigest,
		EntryID: cfg.EntryID, ClaimToken: cfg.ClaimToken,
	}
	if err := cfg.AuthorityVerifier.VerifyExecutionAuthority(ctx, authority); err != nil {
		return result, err
	}
	result, runErr := runProgram(ctx, entry.program, cfg)
	return result, classifyUnclassifiedInstanceFailure(runErr)
}

// classifyUnclassifiedInstanceFailure 是 RunProgram 的兜底分类器：为裸错误补上 domain/node 已发布的
// 节点操作失败错误码和消息，并让所有已分类错误原样通过。
func classifyUnclassifiedInstanceFailure(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	err, constructionErr := fault.Wrap(cause, fault.Internal, node.CodeOperationFailed, "node operation failed")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// detachedTimeout 在不继承父上下文取消状态的前提下创建带截止时间的清理上下文。
func detachedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
