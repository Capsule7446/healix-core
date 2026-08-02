package automation

import (
	"reflect"

	"github.com/Capsule7446/healix-core/domain/fault"
)

const CodeAutomationConfigurationInvalid fault.Code = "AUTOMATION_CONFIGURATION_INVALID"

func AutomationConfigurationError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeAutomationConfigurationInvalid,
		"automation service is not configured",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

const CodeHealCandidateStaleBase fault.Code = "AUTOMATION_HEAL_CANDIDATE_STALE_BASE"

func HealCandidateStaleBaseError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeHealCandidateStaleBase,
		"heal candidate base version is no longer current",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func isNilDependency(value any) bool {
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

const CodeAutomationRevisionConflict fault.Code = "AUTOMATION_REVISION_CONFLICT"

func AutomationRevisionConflictError() error {
	err, constructionErr := fault.New(
		fault.Conflict,
		CodeAutomationRevisionConflict,
		"automation revision conflicts with current state",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
