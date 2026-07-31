package evidence

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeStepTransitionCommitInvalid       fault.Code = "EVIDENCE_STEP_TRANSITION_COMMIT_INVALID"
	CodeCommitFactLimitExceeded           fault.Code = "EVIDENCE_COMMIT_FACT_LIMIT_EXCEEDED"
	CodeStepProgressEventInvalid          fault.Code = "EVIDENCE_STEP_PROGRESS_EVENT_INVALID"
	CodeStepFactInvalid                   fault.Code = "EVIDENCE_STEP_FACT_INVALID"
	CodeHealObservationInvalid            fault.Code = "EVIDENCE_HEAL_OBSERVATION_INVALID"
	CodeValidationObservationInvalid      fault.Code = "EVIDENCE_VALIDATION_OBSERVATION_INVALID"
	CodeValidationGroupObservationInvalid fault.Code = "EVIDENCE_VALIDATION_GROUP_OBSERVATION_INVALID"
)

// Every identity in this context — commit, step execution, execution, validation,
// heal, group, element target — is caller-supplied and stays out of public text.
// Sub-validation failures degrade into the enclosing envelope's violations rather
// than nesting a second fault, so a host classifies without recursive unwrapping.

func stepTransitionCommitInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeStepTransitionCommitInvalid, "step transition commit is invalid", fault.WithViolations(capViolations(violations)...))
}

// commitFactLimitExceededError is separate from the commit envelope because the
// remediation differs: split the commit rather than correct a field.
func commitFactLimitExceededError() error {
	return mustEvidenceFault(fault.OutOfRange, CodeCommitFactLimitExceeded, "step transition commit exceeds its fact limit")
}

func stepProgressEventInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeStepProgressEventInvalid, "step progress event is invalid", fault.WithViolations(capViolations(violations)...))
}

func stepFactInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeStepFactInvalid, "step fact is invalid", fault.WithViolations(capViolations(violations)...))
}

func healObservationInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeHealObservationInvalid, "heal observation is invalid", fault.WithViolations(capViolations(violations)...))
}

func validationObservationInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeValidationObservationInvalid, "validation observation is invalid", fault.WithViolations(capViolations(violations)...))
}

func validationGroupObservationInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeValidationGroupObservationInvalid, "validation group observation is invalid", fault.WithViolations(capViolations(violations)...))
}

// capViolations keeps the deterministic leading prefix when an aggregate exceeds
// the envelope cap, so untrusted input cannot turn validation into a panic.
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

// atCap lets the collection walks stop once the envelope is full. Because
// violations are appended in input order, stopping early yields exactly the same
// leading prefix that capViolations would keep, without building the tens of
// thousands of violations a maximum-size commit could otherwise produce.
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

func mustEvidenceFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
