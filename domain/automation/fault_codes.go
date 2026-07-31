package automation

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodePersistedRevisionInvalid      fault.Code = "AUTOMATION_PERSISTED_REVISION_INVALID"
	CodeRevisionExhausted             fault.Code = "AUTOMATION_REVISION_EXHAUSTED"
	CodePersistedVersionNumberInvalid fault.Code = "AUTOMATION_PERSISTED_VERSION_NUMBER_INVALID"
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

func mustAutomationFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
