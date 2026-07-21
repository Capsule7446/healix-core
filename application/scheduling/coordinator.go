package scheduling

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
)

const claimReleaseTimeout = 5 * time.Second

var ErrInvalidClaim = errors.New("invalid scheduling claim")

type Claim struct {
	Plan  execution.Plan
	Token string
}

type ClaimSource interface {
	ClaimNext(context.Context, string, int64) (Claim, bool, error)
	Release(context.Context, Claim) error
}

type EntryStateReader interface {
	LoadEntryStates(context.Context, Claim) ([]EntryState, error)
}

type DecisionWriter interface {
	ApplyDecision(context.Context, Claim, Decision, int64) error
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
	if claim.Token == "" || !claim.Plan.IsSealed() {
		return true, ErrInvalidClaim
	}
	states, err := c.states.LoadEntryStates(ctx, claim)
	if err != nil {
		return true, fmt.Errorf("load entry states: %w", err)
	}
	decision, err := DecideAdvance(claim.Plan, states)
	if err != nil {
		return true, fmt.Errorf("decide run advance: %w", err)
	}
	if decision.NextExecutionID == "" && len(decision.Transitions) == 0 && decision.FinalStatus == nil {
		return true, nil
	}
	if err := c.writer.ApplyDecision(ctx, claim, decision, occurredAt); err != nil {
		return true, fmt.Errorf("apply scheduling decision: %w", err)
	}
	return true, nil
}
