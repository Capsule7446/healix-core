package sampling

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeSessionInputInvalid        fault.Code = "SAMPLING_SESSION_INPUT_INVALID"
	CodeSessionStateInvalid        fault.Code = "SAMPLING_SESSION_STATE_INVALID"
	CodeCaptureInvalid             fault.Code = "SAMPLING_CAPTURE_INVALID"
	CodeDraftInvalid               fault.Code = "SAMPLING_DRAFT_INVALID"
	CodeDraftStepNotFound          fault.Code = "SAMPLING_DRAFT_STEP_NOT_FOUND"
	CodeDraftElementTargetNotFound fault.Code = "SAMPLING_DRAFT_NODE_NOT_FOUND"
	CodeDraftElementTargetInUse    fault.Code = "SAMPLING_DRAFT_NODE_IN_USE"
	CodeDraftIndexOutOfRange       fault.Code = "SAMPLING_DRAFT_INDEX_OUT_OF_RANGE"
	CodePublicationMappingInvalid  fault.Code = "SAMPLING_PUBLICATION_MAPPING_INVALID"
	CodeWorkspaceInvalid           fault.Code = "SAMPLING_WORKSPACE_INVALID"
	CodeInternal                   fault.Code = "SAMPLING_INTERNAL"
)

// Step and element-target identities are the caller's own and never reach public
// text. Session status and action kind are closed sets, but a rejected value is by
// definition outside the set, so echoing it would echo arbitrary caller input.

func sessionInputInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeSessionInputInvalid, "sampling session input is invalid", fault.WithViolations(capViolations(violations)...))
}

// wrapSessionInputInvalidError keeps a parse failure as a private cause. A URL
// parse error embeds the whole URL in its own text, so it must never be public.
func wrapSessionInputInvalidError(cause error, violations []fault.Violation) error {
	return wrapSamplingFault(cause, fault.InvalidArgument, CodeSessionInputInvalid, "sampling session input is invalid", fault.WithViolations(capViolations(violations)...))
}

func sessionStateInvalidError() error {
	return mustSamplingFault(fault.FailedPrecondition, CodeSessionStateInvalid, "sampling session state does not allow this operation")
}

func captureInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeCaptureInvalid, "sampling capture is invalid", fault.WithViolations(capViolations(violations)...))
}

func draftInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeDraftInvalid, "sampling draft is invalid", fault.WithViolations(capViolations(violations)...))
}

func draftStepNotFoundError() error {
	return mustSamplingFault(fault.NotFound, CodeDraftStepNotFound, "sampling draft step was not found")
}

func draftElementTargetNotFoundError() error {
	return mustSamplingFault(fault.NotFound, CodeDraftElementTargetNotFound, "unpublished element target was not found")
}

func draftElementTargetInUseError() error {
	return mustSamplingFault(fault.FailedPrecondition, CodeDraftElementTargetInUse, "unpublished element target is still referenced")
}

func draftIndexOutOfRangeError() error {
	return mustSamplingFault(fault.OutOfRange, CodeDraftIndexOutOfRange, "sampling draft index is out of range")
}

func publicationMappingInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodePublicationMappingInvalid, "sampling publication mapping is invalid", fault.WithViolations(capViolations(violations)...))
}

func workspaceInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeWorkspaceInvalid, "sampling workspace is invalid", fault.WithViolations(capViolations(violations)...))
}

func internalError() error {
	return mustSamplingFault(fault.Internal, CodeInternal, "sampling operation could not be completed")
}

func wrapInternalError(cause error) error {
	return wrapSamplingFault(cause, fault.Internal, CodeInternal, "sampling operation could not be completed")
}

// capViolations keeps the deterministic leading prefix when an aggregate exceeds
// the envelope cap, so untrusted input cannot turn validation into a panic.
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}

func mustSamplingFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func wrapSamplingFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
