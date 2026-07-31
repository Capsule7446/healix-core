package scheduling

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const claimReleaseTimeout = 5 * time.Second

const (
	CodeSchedulingDependencyRequired fault.Code = "EXECUTION_SCHEDULING_DEPENDENCY_REQUIRED"
	CodeSchedulingClaimInvalid       fault.Code = "EXECUTION_SCHEDULING_CLAIM_INVALID"
)

func schedulingClaimInvalidError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeSchedulingClaimInvalid,
		"scheduling claim is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func schedulingDependencyRequiredError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeSchedulingDependencyRequired, "execution scheduling dependency is required")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func isNilPort(port any) bool {
	if port == nil {
		return true
	}
	value := reflect.ValueOf(port)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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
		return false, schedulingDependencyRequiredError()
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
		return true, schedulingClaimInvalidError()
	}
	states, err := c.states.LoadEntryStates(ctx, claim)
	if err != nil {
		return true, fmt.Errorf("load entry states: %w", err)
	}
	decision, err := DecideAdvance(claim.Snapshot, states)
	if err != nil {
		// DecideAdvance already returns EXECUTION_ENTRY_STATES_INVALID.
		return true, err
	}
	if decision.NextExecutionID == "" && len(decision.Transitions) == 0 && decision.FinalStatus == nil {
		return true, nil
	}
	applied, err := c.writer.ApplyDecision(ctx, claim, decision, occurredAt)
	if err != nil {
		return true, fmt.Errorf("apply scheduling decision: %w", err)
	}
	if !applied.Applied || applied.Fence != claim.Fence {
		return true, execution.NewStaleWorkerFenceError()
	}
	return true, nil
}
