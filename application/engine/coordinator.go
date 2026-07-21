package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/node"
)

// RunCoordinator owns application-level lifecycle around one compiled Program.
type RunCoordinator struct{}

func (RunCoordinator) Run(ctx context.Context, program node.Program, cfg Config) (runErr error) {
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

	rt := newRuntime(program, cfg)
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

func newRuntime(program node.Program, cfg Config) *node.Runtime {
	scratchpad := make(map[string]any, len(cfg.Variables))
	for name, value := range cfg.Variables {
		scratchpad[name] = value
	}
	return &node.Runtime{
		RunID:        cfg.RunID,
		ClaimToken:   cfg.ClaimToken,
		StepInterval: cfg.StepInterval,
		Specs:        program.Specs,
		Driver:       cfg.Driver,
		Healer:       cfg.Healer,
		Recorder:     cfg.Recorder,
		Facts:        cfg.Facts,
		Scratchpad:   scratchpad,
	}
}
