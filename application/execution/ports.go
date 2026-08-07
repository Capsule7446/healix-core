package execution

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

func stepTransitionCommitPayloadTooLargeError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.OutOfRange, CodeStepTransitionCommitPayloadTooLarge, "step transition commit payload is too large")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func stepTransitionCommitInstanceMismatchError(cause error) error {
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
	// Expressed in walked content bytes.
	//
	// It was the same number when the measure was len(json.Marshal), and it was
	// briefly scaled to 1<<18 on the theory that the old framing ran about 4.5x
	// the content, so a quarter of the limit would hold the boundary where it
	// was. That reasoning was wrong: the ratio is not a constant. Framing is
	// charged per field, not per byte, so a commit made of a few long strings
	// carries almost no framing while a commit made of many short fields carries
	// a lot. Five sixty-kilobyte strings each pass the per-string cap and total
	// 300 KiB of content — accepted under the old megabyte of JSON, rejected by
	// a quarter-megabyte of content.
	//
	// No single factor preserves a shape-dependent boundary, so there is nothing
	// to preserve and the question is which way to be wrong. Rejecting a payload
	// a host successfully sent yesterday is the worse failure, and the real
	// blow-up is already bounded from two other directions:
	// maxStepTransitionStringBytes caps any one string at 64 KiB and
	// maxStepTransitionFacts caps the fact count at 10,000.
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
	// The size is measured by walking the value, and the copy is made by the type
	// that owns it. Both used to be one json.Marshal round trip, which measured
	// the wrong thing and produced the wrong copy: an execution coordinate is a
	// struct whose only field is unexported, so it encoded as {} — two bytes
	// toward the budget instead of its real length, and a zero value on the way
	// back out.
	if size := stepTransitionPayloadBytes(reflect.ValueOf(commit)); size > MaxStepTransitionPayloadBytes {
		return evidence.StepTransitionCommit{}, stepTransitionCommitPayloadTooLargeError(fmt.Errorf("step transition commit exceeds byte limit %d", MaxStepTransitionPayloadBytes))
	}
	return commit.Clone(), nil
}

// stepTransitionPayloadBytes measures the content a commit carries: string
// bytes plus a fixed width per fixed-size field. It replaced len(json.Marshal),
// which measured the wrong thing twice over — it counted the framing of one
// particular encoding, and it counted each execution coordinate as the two
// bytes of {} rather than its real length.
//
// The unit changed, and no scalar can carry the old boundary across: the old
// measure charged framing per field, so its ratio to content depended on the
// shape of the commit. See MaxStepTransitionPayloadBytes for which way that
// leaves the limit and why.
//
// Unexported string fields are counted: reflect can read their length even
// though it cannot hand them out.
func stepTransitionPayloadBytes(value reflect.Value) int {
	return walkStepTransitionBytes(value, 0)
}

// maxStepTransitionWalkDepth bounds the walk. The commit tree is finite today,
// but json.Marshal used to return an error on a cycle and the walk that replaced
// it would recurse until the stack gave out. A pointer field added to an
// observation is all it would take.
const maxStepTransitionWalkDepth = 64

func walkStepTransitionBytes(value reflect.Value, depth int) int {
	if !value.IsValid() || depth > maxStepTransitionWalkDepth {
		return 0
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return 1
		}
		return walkStepTransitionBytes(value.Elem(), depth+1)
	case reflect.String:
		return value.Len()
	case reflect.Struct:
		total := 0
		for index := 0; index < value.NumField(); index++ {
			total += walkStepTransitionBytes(value.Field(index), depth+1)
		}
		return total
	case reflect.Slice, reflect.Array:
		total := 0
		for index := 0; index < value.Len(); index++ {
			total += walkStepTransitionBytes(value.Index(index), depth+1)
		}
		return total
	case reflect.Map:
		total := 0
		iterator := value.MapRange()
		for iterator.Next() {
			total += walkStepTransitionBytes(iterator.Key(), depth+1)
			total += walkStepTransitionBytes(iterator.Value(), depth+1)
		}
		return total
	default:
		return 8
	}
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
	if err := validateCommitInstanceBinding(fence.InstanceID, commit); err != nil {
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

func validateCommitInstanceBinding(instanceID domainexecution.InstanceID, commit evidence.StepTransitionCommit) error {
	for _, observation := range commit.FinalValidations {
		if observation.InstanceID != instanceID {
			return stepTransitionCommitInstanceMismatchError(fmt.Errorf("validation observation instance %q does not match worker fence instance %q", observation.InstanceID, instanceID))
		}
	}
	for _, group := range commit.FinalValidationGroups {
		if group.InstanceID != instanceID {
			return stepTransitionCommitInstanceMismatchError(fmt.Errorf("validation group instance %q does not match worker fence instance %q", group.InstanceID, instanceID))
		}
	}
	for _, observation := range commit.HealObservations {
		if observation.InstanceID != instanceID {
			return stepTransitionCommitInstanceMismatchError(fmt.Errorf("heal observation instance %q does not match worker fence instance %q", observation.InstanceID, instanceID))
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
