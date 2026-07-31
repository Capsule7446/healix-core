package automation

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeExecutionFlowInvalid fault.Code = "AUTOMATION_EXECUTION_FLOW_INVALID"

	CodePersistedRevisionInvalid          fault.Code = "AUTOMATION_PERSISTED_REVISION_INVALID"
	CodeRevisionExhausted                 fault.Code = "AUTOMATION_REVISION_EXHAUSTED"
	CodePersistedVersionNumberInvalid     fault.Code = "AUTOMATION_PERSISTED_VERSION_NUMBER_INVALID"
	CodeHealCandidateIdentityInvalid      fault.Code = "AUTOMATION_HEAL_CANDIDATE_IDENTITY_INVALID"
	CodeHealCandidateStateInvalid         fault.Code = "AUTOMATION_HEAL_CANDIDATE_STATE_INVALID"
	CodeHealCandidateReviewStatusInvalid  fault.Code = "AUTOMATION_HEAL_CANDIDATE_REVIEW_STATUS_INVALID"
	CodeHealCandidateReviewCommandInvalid fault.Code = "AUTOMATION_HEAL_CANDIDATE_REVIEW_COMMAND_INVALID"
	CodeHealApprovalStatusInvalid         fault.Code = "AUTOMATION_HEAL_APPROVAL_STATUS_INVALID"
	CodeHealDecisionBandInvalid           fault.Code = "AUTOMATION_HEAL_DECISION_BAND_INVALID"
	CodeHealConfidenceInvalid             fault.Code = "AUTOMATION_HEAL_CONFIDENCE_INVALID"
	CodeHealStreakStateInvalid            fault.Code = "AUTOMATION_HEAL_STREAK_STATE_INVALID"
	CodeHealObservationInvalid            fault.Code = "AUTOMATION_HEAL_OBSERVATION_INVALID"
	CodeHealSequenceConflict              fault.Code = "AUTOMATION_HEAL_SEQUENCE_CONFLICT"
	CodeHealProvenanceConflict            fault.Code = "AUTOMATION_HEAL_PROVENANCE_CONFLICT"
	CodeHealStreakRejectionInvalid        fault.Code = "AUTOMATION_HEAL_STREAK_REJECTION_INVALID"
)

func persistedRevisionInvalidError() error {
	return mustAutomationFault(fault.FailedPrecondition, CodePersistedRevisionInvalid, "persisted revision must be non-zero")
}

func persistedVersionNumberInvalidError() error {
	return mustAutomationFault(fault.FailedPrecondition, CodePersistedVersionNumberInvalid, "persisted version number must be positive")
}

func revisionExhaustedError() error {
	return mustAutomationFault(fault.ResourceExhausted, CodeRevisionExhausted, "revision value is exhausted")
}

func healCandidateIdentityInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealCandidateIdentityInvalid, "heal candidate identity is invalid")
}

func healCandidateStateInvalidError() error {
	return mustAutomationFault(fault.FailedPrecondition, CodeHealCandidateStateInvalid, "heal candidate state does not allow this operation")
}

func healCandidateReviewStatusInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealCandidateReviewStatusInvalid, "heal candidate review status is invalid")
}

func healCandidateReviewCommandInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealCandidateReviewCommandInvalid, "heal candidate review command is invalid")
}

func healApprovalStatusInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealApprovalStatusInvalid, "heal approval status is invalid")
}

func healDecisionBandInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealDecisionBandInvalid, "heal decision band is invalid")
}

func healConfidenceInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealConfidenceInvalid, "heal confidence is invalid")
}

func healStreakStateInvalidError(cause error) error {
	return wrapAutomationFault(cause, fault.FailedPrecondition, CodeHealStreakStateInvalid, "persisted heal streak state is invalid")
}

func healObservationInvalidError(cause error) error {
	return wrapAutomationFault(cause, fault.InvalidArgument, CodeHealObservationInvalid, "heal observation is invalid")
}

func healSequenceConflictError(cause error) error {
	return wrapAutomationFault(cause, fault.Conflict, CodeHealSequenceConflict, "heal sequence conflicts with persisted ordering")
}

func healProvenanceConflictError(cause error) error {
	return wrapAutomationFault(cause, fault.Conflict, CodeHealProvenanceConflict, "heal observation conflicts with persisted provenance")
}

func healStreakRejectionInvalidError(cause error) error {
	return wrapAutomationFault(cause, fault.FailedPrecondition, CodeHealStreakRejectionInvalid, "heal streak cannot be rejected in its current state")
}

// executionFlowInvalidError is the aggregate validation envelope for execution
// flow input: one top-level fault carrying every field failure as an ordered
// violation, never one code per failing field and never a joined message.
//
// Violations beyond fault.MaxViolations are dropped rather than allowed to fail
// construction, because the input reaching this envelope is untrusted and a
// construction failure would surface as a panic. The kept prefix is
// deterministic: callers build the slice by walking their input in order.
func executionFlowInvalidError(violations []fault.Violation) error {
	if len(violations) > fault.MaxViolations {
		violations = violations[:fault.MaxViolations]
	}
	return mustAutomationFault(fault.InvalidArgument, CodeExecutionFlowInvalid, "execution flow input is invalid", fault.WithViolations(violations...))
}

func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}

func wrapAutomationFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func mustAutomationFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
