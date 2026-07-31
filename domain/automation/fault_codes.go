package automation

import "github.com/Capsule7446/healix-core/domain/fault"

const (
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

func mustAutomationFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
