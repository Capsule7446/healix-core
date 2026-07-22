// Package engine 是唯一的执行编排入口：把内存 Program 接到一次全新的 Runtime。
package engine

import (
	"context"
	"time"

	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

// Config 打包了一次 Program 执行所需的领域端口与运行变量。
type Config struct {
	RunID      string
	ClaimToken string
	Driver     node.Driver
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
	// Variables 是本次 run 的初始变量。组合根可从环境或密钥系统注入，
	// domain 只接收内存值，不感知具体密钥来源。
	Variables map[string]string
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

// RunProgram executes an in-memory Program compiled from an immutable run snapshot.
func RunProgram(ctx context.Context, program node.Program, cfg Config) error {
	_, err := RunProgramWithResult(ctx, program, cfg)
	return err
}

func RunProgramWithResult(ctx context.Context, program node.Program, cfg Config) (RunResult, error) {
	return (RunCoordinator{}).Run(ctx, program, cfg)
}

func detachedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
