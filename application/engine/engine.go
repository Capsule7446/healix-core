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
	InstanceID     execution.InstanceID
	SnapshotDigest string
	EntryID        execution.EntryID
	ClaimToken     string
}

type ExecutionAuthorityVerifier interface {
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
	OutcomeSucceeded    ExecutionOutcome = "SUCCEEDED"
	OutcomeFailed       ExecutionOutcome = "FAILED"
	OutcomeCanceled     ExecutionOutcome = "CANCELED"
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

type EntryResult struct {
	ExecutionOutcome ExecutionOutcome
	RecordingOutcome RecordingOutcome
	TimelineOutcome  TimelineOutcome
}

// RunProgram executes only an entry produced by CompilePlan. Identity is
// validated before any runtime port can be observed.
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

// classifyUnclassifiedInstanceFailure is RunProgram's backstop: it guarantees no
// unclassified error ever leaves RunProgram by giving any bare failure the
// same code and message domain/node already publishes for an opaque node
// operation failure, and it lets every already-classified failure through
// unchanged.
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

func detachedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
