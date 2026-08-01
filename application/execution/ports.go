package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const (
	CodeFactCommitterRequired  fault.Code = "EXECUTION_FACT_COMMITTER_REQUIRED"
	CodeStepRevisionConflict   fault.Code = "EXECUTION_STEP_REVISION_CONFLICT"
	CodeCommitIdentityConflict fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_IDENTITY_CONFLICT"
	// CodeStepTransitionCommitPayloadTooLarge covers both the overall payload
	// budget and any one string exceeding its own byte limit: the remediation is
	// always to shrink the commit, never to correct a specific field's value.
	CodeStepTransitionCommitPayloadTooLarge fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_PAYLOAD_TOO_LARGE"
	// CodeStepTransitionCommitRunMismatch covers a commit fact whose own InstanceID
	// disagrees with the claimed worker fence's InstanceID. FAILED_PRECONDITION: the
	// caller must re-read the authoritative claim, not supply a different value.
	CodeStepTransitionCommitRunMismatch fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_RUN_MISMATCH"
)

func stepTransitionCommitPayloadTooLargeError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.OutOfRange, CodeStepTransitionCommitPayloadTooLarge, "step transition commit payload is too large")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func stepTransitionCommitRunMismatchError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.FailedPrecondition, CodeStepTransitionCommitRunMismatch, "step transition commit does not match the claimed run")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func StepRevisionConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeStepRevisionConflict,
		"step transition revision conflicts with current state",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func CommitIdentityConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeCommitIdentityConflict,
		"step transition commit identity conflicts with the previously accepted commit",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func FactCommitterRequiredError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeFactCommitterRequired,
		"execution fact committer is required",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const (
	MaxStepTransitionPayloadBytes = 1 << 20
	maxStepTransitionStringBytes  = 64 << 10
)

func ValidateStepTransitionPayloadSize(commit evidence.StepTransitionCommit) error {
	_, err := ownStepTransitionCommit(commit)
	return err
}

func ownStepTransitionCommit(commit evidence.StepTransitionCommit) (evidence.StepTransitionCommit, error) {
	if err := validateStepTransitionStringBounds(reflect.ValueOf(commit)); err != nil {
		return evidence.StepTransitionCommit{}, err
	}
	payload, err := json.Marshal(commit)
	if err != nil {
		return evidence.StepTransitionCommit{}, fmt.Errorf("encode step transition commit: %w", err)
	}
	if len(payload) > MaxStepTransitionPayloadBytes {
		return evidence.StepTransitionCommit{}, stepTransitionCommitPayloadTooLargeError(fmt.Errorf("step transition commit exceeds byte limit %d", MaxStepTransitionPayloadBytes))
	}
	var owned evidence.StepTransitionCommit
	if err := json.Unmarshal(payload, &owned); err != nil {
		return evidence.StepTransitionCommit{}, fmt.Errorf("clone step transition commit: %w", err)
	}
	return owned, nil
}

func validateStepTransitionStringBounds(value reflect.Value) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return validateStepTransitionStringBounds(value.Elem())
	case reflect.String:
		if value.Len() > maxStepTransitionStringBytes {
			return stepTransitionCommitPayloadTooLargeError(fmt.Errorf("step transition string exceeds byte limit %d", maxStepTransitionStringBytes))
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if err := validateStepTransitionStringBounds(value.Field(index)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateStepTransitionStringBounds(value.Index(index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if err := validateStepTransitionStringBounds(iterator.Key()); err != nil {
				return err
			}
			if err := validateStepTransitionStringBounds(iterator.Value()); err != nil {
				return err
			}
		}
	}
	return nil
}

// final facts, Heal governance, effects, outbox records, and authoritative replay result.
type StepTransitionTransaction interface {
	CommitStepTransition(context.Context, domainexecution.WorkerFence, evidence.StepTransitionCommit, HealGovernancePlanner) (evidence.StepTransitionCommitResult, error)
}

type FactCommitter struct {
	transaction StepTransitionTransaction
	planner     HealGovernancePlanner
}

func NewFactCommitter(transaction StepTransitionTransaction, planner HealGovernancePlanner) FactCommitter {
	return FactCommitter{transaction: transaction, planner: planner}
}

func (c FactCommitter) CommitStepTransition(ctx context.Context, fence domainexecution.WorkerFence, commit evidence.StepTransitionCommit) (evidence.StepTransitionCommitResult, error) {
	if isNilInterface(c.transaction) {
		return evidence.StepTransitionCommitResult{}, FactCommitterRequiredError()
	}
	if isNilInterface(c.planner) {
		// Same missing-dependency failure as the branch above, and it already has a
		// registered code; leaving it bare made one of two identical conditions
		// unclassifiable.
		return evidence.StepTransitionCommitResult{}, FactCommitterRequiredError()
	}
	return c.transaction.CommitStepTransition(ctx, fence, commit, c.planner)
}

type StepTransitionService struct {
	committer FactCommitter
}

func NewStepTransitionService(committer FactCommitter) StepTransitionService {
	return StepTransitionService{committer: committer}
}

func (s StepTransitionService) Commit(ctx context.Context, fence domainexecution.WorkerFence, commit evidence.StepTransitionCommit) (evidence.StepTransitionCommitResult, error) {
	if isNilInterface(s.committer.transaction) || isNilInterface(s.committer.planner) {
		return evidence.StepTransitionCommitResult{}, FactCommitterRequiredError()
	}
	// Both validators return their own classified faults. Wrapping them in an
	// uncoded fmt.Errorf put an unclassified layer on the outside of a coded fault
	// exactly at the public boundary, which is what forced hosts to fall back to a
	// blanket INTERNAL response.
	if err := fence.Validate(); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	if err := commit.Validate(); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	if err := validateCommitRunBinding(fence.InstanceID, commit); err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	owned, err := ownStepTransitionCommit(commit)
	if err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	result, err := s.committer.CommitStepTransition(ctx, fence, owned)
	if err != nil {
		return evidence.StepTransitionCommitResult{}, err
	}
	result.Promotions = append([]evidence.NodeVersionPromotion(nil), result.Promotions...)
	return result, nil
}

func validateCommitRunBinding(instanceID domainexecution.InstanceID, commit evidence.StepTransitionCommit) error {
	for _, observation := range commit.FinalValidations {
		if observation.InstanceID != instanceID {
			return stepTransitionCommitRunMismatchError(fmt.Errorf("validation observation run %q does not match worker fence run %q", observation.InstanceID, instanceID))
		}
	}
	for _, group := range commit.FinalValidationGroups {
		if group.InstanceID != instanceID {
			return stepTransitionCommitRunMismatchError(fmt.Errorf("validation group run %q does not match worker fence run %q", group.InstanceID, instanceID))
		}
	}
	for _, observation := range commit.HealObservations {
		if observation.InstanceID != instanceID {
			return stepTransitionCommitRunMismatchError(fmt.Errorf("heal observation run %q does not match worker fence run %q", observation.InstanceID, instanceID))
		}
	}
	return nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// ProgressWriter persists non-terminal execution observations under the active worker claim.
type ProgressWriter interface {
	RecordStepProgress(context.Context, domainexecution.WorkerFence, evidence.StepProgressEvent) error
	RecordValidationProgress(context.Context, domainexecution.WorkerFence, evidence.ValidationProgressObservation) error
}
