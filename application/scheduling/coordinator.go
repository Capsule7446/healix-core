package scheduling

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
)

const claimReleaseTimeout = 5 * time.Second

var (
	ErrInvalidClaim         = errors.New("invalid scheduling claim")
	ErrSchedulingDependency = errors.New("scheduling dependency is unavailable")
)

func isNilPort(port any) bool {
	if port == nil {
		return true
	}
	value := reflect.ValueOf(port)
	return value.Kind() == reflect.Ptr && value.IsNil()
}

type Claim struct {
	Snapshot execution.RunSnapshot
	Fence    execution.WorkerFence
}

type ClaimSource interface {
	ClaimNext(context.Context, string, int64) (Claim, bool, error)
	Release(context.Context, Claim) error
}

type EntryStateReader interface {
	LoadEntryStates(context.Context, Claim) ([]EntryState, error)
}

type ApplyDecisionResult struct {
	Fence   execution.WorkerFence
	Applied bool
}

type DecisionWriter interface {
	// ApplyDecision atomically fences and applies entry transitions, successor
	// start, and final Run status from one pure Decision.
	ApplyDecision(context.Context, Claim, Decision, int64) (ApplyDecisionResult, error)
}

type Coordinator struct {
	claims ClaimSource
	states EntryStateReader
	writer DecisionWriter
}

func NewCoordinator(claims ClaimSource, states EntryStateReader, writer DecisionWriter) Coordinator {
	return Coordinator{claims: claims, states: states, writer: writer}
}

func (c Coordinator) ProcessNext(ctx context.Context, workerID string, occurredAt int64) (claimed bool, resultErr error) {
	if isNilPort(c.claims) || isNilPort(c.states) || isNilPort(c.writer) {
		return false, ErrSchedulingDependency
	}
	claim, found, err := c.claims.ClaimNext(ctx, workerID, occurredAt)
	if err != nil {
		return false, fmt.Errorf("claim next run: %w", err)
	}
	if !found {
		return false, nil
	}
	defer func() {
		releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), claimReleaseTimeout)
		defer cancel()
		if err := c.claims.Release(releaseContext, claim); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release run claim: %w", err))
		}
	}()
	if claim.Fence.Validate() != nil || claim.Fence.RunID != claim.Snapshot.RunID() || claim.Snapshot.Digest() == "" {
		return true, ErrInvalidClaim
	}
	states, err := c.states.LoadEntryStates(ctx, claim)
	if err != nil {
		return true, fmt.Errorf("load entry states: %w", err)
	}
	decision, err := DecideAdvance(claim.Snapshot, states)
	if err != nil {
		return true, fmt.Errorf("decide run advance: %w", err)
	}
	if decision.NextExecutionID == "" && len(decision.Transitions) == 0 && decision.FinalStatus == nil {
		return true, nil
	}
	applied, err := c.writer.ApplyDecision(ctx, claim, decision, occurredAt)
	if err != nil {
		return true, fmt.Errorf("apply scheduling decision: %w", err)
	}
	if !applied.Applied || applied.Fence != claim.Fence {
		return true, &execution.StaleWorkerFenceError{Fence: claim.Fence}
	}
	return true, nil
}
