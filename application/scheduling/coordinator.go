package scheduling

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const claimReleaseTimeout = 5 * time.Second

const (
	CodeSchedulingDependencyRequired fault.Code = "EXECUTION_SCHEDULING_DEPENDENCY_REQUIRED"
	CodeSchedulingClaimInvalid       fault.Code = "EXECUTION_SCHEDULING_CLAIM_INVALID"
	// CodeSchedulingAdapterUnavailable covers every claim/release/state/decision
	// port failure below: the host adapter, not the caller, needs to become
	// reachable again, so the remediation is retry rather than a different
	// argument.
	CodeSchedulingAdapterUnavailable fault.Code = "EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE"
)

// classifySchedulingAdapterFailure gives a bare scheduling port failure its
// registered code, and lets an already-classified failure (for example
// DecideAdvance's own EXECUTION_ENTRY_STATES_INVALID) through unchanged so this
// boundary never buries a code a dependency already produced.
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
	Snapshot execution.InstanceSnapshot
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
	// ApplyDecision atomically fences and applies the complete set of entry
	// state writes from one pure Decision. Transitions is the full list;
	// NextEntryID is a shortcut reference to the entry that is being started
	// (its Pending→Running transition is included in Transitions).
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
		return false, classifySchedulingAdapterFailure(err)
	}
	if !found {
		return false, nil
	}
	defer func() {
		releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), claimReleaseTimeout)
		defer cancel()
		if err := c.claims.Release(releaseContext, claim); err != nil {
			resultErr = errors.Join(resultErr, classifySchedulingAdapterFailure(err))
		}
	}()
	if claim.Fence.Validate() != nil || claim.Fence.InstanceID != claim.Snapshot.InstanceID() || claim.Snapshot.Digest() == "" {
		return true, schedulingClaimInvalidError()
	}
	states, err := c.states.LoadEntryStates(ctx, claim)
	if err != nil {
		return true, classifySchedulingAdapterFailure(err)
	}
	decision, err := DecideAdvance(claim.Snapshot, states)
	if err != nil {
		// DecideAdvance already returns EXECUTION_ENTRY_STATES_INVALID.
		return true, err
	}
	if decision.NextEntryID.Validate() != nil && len(decision.Transitions) == 0 && decision.FinalStatus == nil {
		return true, nil
	}
	applied, err := c.writer.ApplyDecision(ctx, claim, decision, occurredAt)
	if err != nil {
		return true, classifySchedulingAdapterFailure(err)
	}
	if !applied.Applied || applied.Fence != claim.Fence {
		return true, execution.NewStaleWorkerFenceError()
	}
	return true, nil
}
