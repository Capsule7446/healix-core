// Package engine 是唯一的执行编排入口：把内存 Program 接到一次全新的 Runtime。
package engine

import (
	"context"
	"time"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

// ExecutionIdentityMismatchError reports that a compiled entry no longer agrees
// with its sealed Run/snapshot/execution identity or the supplied worker Run.
const (
	CodeExecutionAuthorityVerifierRequired fault.Code = "EXECUTION_AUTHORITY_VERIFIER_REQUIRED"
	CodeExecutionIdentityMismatch          fault.Code = "EXECUTION_IDENTITY_MISMATCH"
)

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

type ExecutionAuthority struct {
	RunID          string
	SnapshotDigest string
	ExecutionID    string
	ClaimToken     string
}

type ExecutionAuthorityVerifier interface {
	VerifyExecutionAuthority(context.Context, ExecutionAuthority) error
}

// Config 打包了一次 Program 执行所需的领域端口与运行变量。
type Config struct {
	// RunID、SnapshotDigest、ExecutionID 与 ClaimToken 必须来自本次已领取
	// 执行权的权威身份，不能从待执行的 CompiledEntry 反向填充。
	RunID             string
	SnapshotDigest    string
	ExecutionID       string
	ClaimToken        string
	AuthorityVerifier ExecutionAuthorityVerifier
	Driver            node.Driver
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

type ExecutionOutcome string

type RecordingOutcome string

type TimelineOutcome string

const (
	ExecutionSucceeded  ExecutionOutcome = "SUCCEEDED"
	ExecutionFailed     ExecutionOutcome = "FAILED"
	ExecutionCanceled   ExecutionOutcome = "CANCELED"
	ExecutionNotStarted ExecutionOutcome = "NOT_STARTED"

	RecordingDisabled    RecordingOutcome = "DISABLED"
	RecordingSucceeded   RecordingOutcome = "SUCCEEDED"
	RecordingStartFailed RecordingOutcome = "START_FAILED"
	RecordingStopFailed  RecordingOutcome = "STOP_FAILED"

	TimelineDisabled     TimelineOutcome = "DISABLED"
	TimelineComplete     TimelineOutcome = "COMPLETE"
	TimelineStartFailed  TimelineOutcome = "START_FAILED"
	TimelineFinishFailed TimelineOutcome = "FINISH_FAILED"
)

type RunResult struct {
	ExecutionOutcome ExecutionOutcome
	RecordingOutcome RecordingOutcome
	TimelineOutcome  TimelineOutcome
}

// RunProgram executes only an entry produced by CompilePlan. Identity is
// validated before any runtime port can be observed.
func RunProgram(ctx context.Context, entry CompiledEntry, cfg Config) (RunResult, error) {
	result := RunResult{ExecutionOutcome: ExecutionNotStarted, RecordingOutcome: RecordingDisabled, TimelineOutcome: TimelineDisabled}
	if entry.identity.runID == "" ||
		entry.RunID != entry.identity.runID ||
		entry.SnapshotDigest != entry.identity.snapshotDigest ||
		entry.ExecutionID != entry.identity.executionID ||
		cfg.RunID != entry.identity.runID ||
		cfg.SnapshotDigest != entry.identity.snapshotDigest ||
		cfg.ExecutionID != entry.identity.executionID ||
		cfg.ClaimToken == "" {
		return result, ExecutionIdentityMismatchError()
	}
	if cfg.AuthorityVerifier == nil {
		return result, ExecutionAuthorityVerifierRequiredError()
	}
	authority := ExecutionAuthority{
		RunID: cfg.RunID, SnapshotDigest: cfg.SnapshotDigest,
		ExecutionID: cfg.ExecutionID, ClaimToken: cfg.ClaimToken,
	}
	if err := cfg.AuthorityVerifier.VerifyExecutionAuthority(ctx, authority); err != nil {
		return result, err
	}
	return runProgram(ctx, entry.program, cfg)
}

func detachedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
