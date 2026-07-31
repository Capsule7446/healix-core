package parameter

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeNameInvalid           fault.Code = "PARAMETER_NAME_INVALID"
	CodeValueInvalid          fault.Code = "PARAMETER_VALUE_INVALID"
	CodeConstraintUnsatisfied fault.Code = "PARAMETER_CONSTRAINT_UNSATISFIED"
	CodeBindingUnresolvable   fault.Code = "PARAMETER_BINDING_UNRESOLVABLE"
)

// These codes carry no violations. Each one reports a single rejected value that
// the caller already holds, so a field-level breakdown would add an i18n key
// without telling the caller anything it cannot read back from its own input.

func nameInvalidError() error {
	return mustParameterFault(fault.InvalidArgument, CodeNameInvalid, "parameter name is invalid")
}

func valueInvalidError() error {
	return mustParameterFault(fault.InvalidArgument, CodeValueInvalid, "parameter value is invalid")
}

// wrapValueInvalidError keeps the canonicalisation failure as a private cause.
// The cause must stay free of the rejected input as well: a host is allowed to
// log causes, and a parameter value is exactly what must not reach a log.
func wrapValueInvalidError(cause error) error {
	return wrapParameterFault(cause, fault.InvalidArgument, CodeValueInvalid, "parameter value is invalid")
}

func constraintUnsatisfiedError() error {
	return mustParameterFault(fault.InvalidArgument, CodeConstraintUnsatisfied, "parameter value does not satisfy its constraint")
}

func bindingUnresolvableError() error {
	return mustParameterFault(fault.FailedPrecondition, CodeBindingUnresolvable, "parameter binding cannot be resolved")
}

func mustParameterFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func wrapParameterFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
