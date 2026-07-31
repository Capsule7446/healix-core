package automation

import (
	"errors"
	"fmt"
	"reflect"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
)

var (
	ErrRevisionConflict       = errors.New("revision conflict")
	ErrHealCandidateStaleBase = errors.New("heal candidate base version is no longer current")
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

type RevisionConflictError struct {
	AggregateKind string
	ID            string
	Expected      domain.Revision
	Actual        domain.Revision
}

func (e RevisionConflictError) Error() string {
	return fmt.Sprintf("%s %s: %v (expected %d, actual %d)", e.AggregateKind, e.ID, ErrRevisionConflict, e.Expected, e.Actual)
}

func (e RevisionConflictError) Unwrap() error { return ErrRevisionConflict }
