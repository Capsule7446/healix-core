package execution

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodePlanUnsealed                    fault.Code = "EXECUTION_PLAN_UNSEALED"
	CodeStatusTransitionInvalid         fault.Code = "EXECUTION_STATUS_TRANSITION_INVALID"
	CodeInstanceStatusTransitionInvalid fault.Code = "EXECUTION_INSTANCE_STATUS_TRANSITION_INVALID"
	CodeWorkerFenceStale                fault.Code = "EXECUTION_WORKER_FENCE_STALE"
	CodeWorkerFenceInvalid              fault.Code = "EXECUTION_WORKER_FENCE_INVALID"

	CodeCreateInstancePlanInvalid      fault.Code = "EXECUTION_CREATE_INSTANCE_PLAN_INVALID"
	CodeCreateInstanceStepShapeInvalid fault.Code = "EXECUTION_CREATE_INSTANCE_STEP_SHAPE_INVALID"
	CodeCreateInstanceSnapshotInvalid  fault.Code = "EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID"
	CodeEnvironmentSnapshotInvalid     fault.Code = "EXECUTION_ENVIRONMENT_SNAPSHOT_INVALID"

	// CodeCreateInstanceSnapshotConflict reuses the code string published by
	// application/scheduling's own constant of the same name. domain/execution
	// cannot import application/scheduling (application depends on domain, never
	// the reverse), so the code string is duplicated here rather than shared; the
	// guard test enforces that this site's Kind and message stay byte-for-byte
	// identical to the published registry row.
	CodeCreateInstanceSnapshotConflict fault.Code = "EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT"
)

func mustExecutionFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func wrapExecutionFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// classifyCreateInstancePlan gives an execution plan's unclassified validation
// failure its registered code, and lets an already-classified one — a step
// shape envelope from a workflow snapshot, a fingerprint spec failure, or a
// parameter failure — through unchanged. Never wrap an already-coded fault a
// second time: that would bury the code the host actually needs to switch on.
func classifyCreateInstancePlan(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return wrapExecutionFault(cause, fault.InvalidArgument, CodeCreateInstancePlanInvalid, "create-instance plan is invalid")
}

// classifyCreateInstanceSnapshot is the equivalent boundary for
// SealInstanceSnapshot, HydrateInstanceSnapshot, and NewInstance: an uncoded snapshot-shape
// failure becomes EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID, while a plan,
// step-shape, or environment fault reached while validating the snapshot
// passes through unchanged.
func classifyCreateInstanceSnapshot(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return wrapExecutionFault(cause, fault.InvalidArgument, CodeCreateInstanceSnapshotInvalid, "create-instance snapshot is invalid")
}

// wrapOrPropagate keeps an already-classified cause unchanged and otherwise
// applies annotate to a bare cause. Internal call sites use it to attach
// caller-supplied identity to a bare error for private-cause use, without ever
// re-wrapping (and thereby burying) another context's own coded fault.
func wrapOrPropagate(cause error, annotate func(error) error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return annotate(cause)
}

// stepShapeInvalidError is the aggregate validation envelope for one workflow
// snapshot's step tree: one top-level fault carrying every shape failure as an
// ordered violation. Violations are capped defensively at construction time in
// addition to the builder's own as-you-go cap, matching the capViolations
// precedent used across every other envelope in this contract.
func stepShapeInvalidError(violations []fault.Violation) error {
	return mustExecutionFault(fault.InvalidArgument, CodeCreateInstanceStepShapeInvalid, "create-instance step shape is invalid", fault.WithViolations(capViolations(violations)...))
}

// environmentSnapshotInvalidError is the aggregate validation envelope for the
// instance's environment, screenshot, and healer policy snapshots.
func environmentSnapshotInvalidError(violations []fault.Violation) error {
	return mustExecutionFault(fault.InvalidArgument, CodeEnvironmentSnapshotInvalid, "environment snapshot is invalid", fault.WithViolations(capViolations(violations)...))
}

// createInstanceSnapshotConflictError reuses the code and safe message
// application/scheduling already publishes under
// EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT; see CodeCreateInstanceSnapshotConflict.
func createInstanceSnapshotConflictError() error {
	return mustExecutionFault(fault.Conflict, CodeCreateInstanceSnapshotConflict, "create-instance snapshot conflicts with the authoritative instance")
}

// capViolations keeps the deterministic leading prefix when an aggregate
// exceeds the envelope cap, so untrusted input cannot turn validation into a
// panic.
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

// atCap lets collection walks stop once the envelope is full. Because
// violations are appended in input order, stopping early yields exactly the
// same leading prefix that capViolations would keep.
func atCap(violations []fault.Violation) bool {
	return len(violations) >= fault.MaxViolations
}

func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}
