// Package engine 是唯一的执行编排入口：把内存 Program 接到一次全新的 Runtime。
package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/node"
)

// Config 打包了一次 Program 执行所需的领域端口与运行变量。
type Config struct {
	RunID  string
	Driver node.Driver
	// Healer 由组合根注入；nil 表示关闭自愈。
	Healer   heal.Healer
	Recorder node.Recorder
	Facts    node.ExecutionSink
	// StepInterval 是执行局部的节奏设置。它应用于叶子 Step 之间，
	// 不会取代显式的条件等待。
	StepInterval time.Duration
	// Variables 是本次 run 的初始变量。组合根可从环境或密钥系统注入，
	// domain 只接收内存值，不感知具体密钥来源。
	Variables map[string]string
}

// RunProgram 执行已经从不可变运行快照编译出的内存 Program。
func RunProgram(ctx context.Context, program node.Program, cfg Config) (runErr error) {
	if cfg.RunID == "" {
		return fmt.Errorf("run ID is required")
	}
	if cfg.Driver == nil {
		return fmt.Errorf("driver is required")
	}
	if program.Root == nil {
		return fmt.Errorf("program root is required")
	}

	scratchpad := make(map[string]any, len(cfg.Variables))
	for name, value := range cfg.Variables {
		scratchpad[name] = value
	}
	rt := &node.Runtime{
		RunID:        cfg.RunID,
		StepInterval: cfg.StepInterval,
		Specs:        program.Specs,
		Driver:       cfg.Driver,
		Healer:       cfg.Healer,
		Recorder:     cfg.Recorder,
		Facts:        cfg.Facts,
		Scratchpad:   scratchpad,
	}

	if cfg.Recorder != nil {
		if err := cfg.Recorder.Start(ctx, cfg.RunID); err != nil {
			return fmt.Errorf("start recorder: %w", err)
		}
		defer func() {
			cleanupCtx, cancel := detachedTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := cfg.Recorder.Stop(cleanupCtx, true); err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("stop recorder: %w", err))
			}
		}()
	}

	return program.Root.Run(ctx, rt)
}

func detachedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
