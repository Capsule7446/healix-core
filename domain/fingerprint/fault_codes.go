package fingerprint

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeSelectorInvalid          fault.Code = "FINGERPRINT_SELECTOR_INVALID"
	CodeElementTargetSpecInvalid fault.Code = "FINGERPRINT_ELEMENT_TARGET_SPEC_INVALID"
	CodeDescriptorInvalid        fault.Code = "FINGERPRINT_DESCRIPTOR_INVALID"
	CodeFrameworkStackInvalid    fault.Code = "FINGERPRINT_FRAMEWORK_STACK_INVALID"
	CodeFrameworkDetectorFailed  fault.Code = "FINGERPRINT_FRAMEWORK_DETECTOR_FAILED"
)

func selectorInvalidError() error {
	return mustFingerprintFault(fault.InvalidArgument, CodeSelectorInvalid, "element selector is invalid")
}

func elementTargetSpecInvalidError(violations []fault.Violation) error {
	return mustFingerprintFault(fault.InvalidArgument, CodeElementTargetSpecInvalid, "element target spec is invalid", fault.WithViolations(capViolations(violations)...))
}

func descriptorInvalidError(violations []fault.Violation) error {
	return mustFingerprintFault(fault.InvalidArgument, CodeDescriptorInvalid, "element fingerprint descriptor is invalid", fault.WithViolations(capViolations(violations)...))
}

func frameworkStackInvalidError(violations []fault.Violation) error {
	return mustFingerprintFault(fault.InvalidArgument, CodeFrameworkStackInvalid, "framework stack is invalid", fault.WithViolations(capViolations(violations)...))
}

// frameworkDetectorFailedError keeps the detector's own error as a private cause.
// A host-supplied detector reports whatever it likes, including page URLs and DOM
// fragments, so none of its text may reach public fault text.
func frameworkDetectorFailedError(cause error) error {
	return wrapFingerprintFault(cause, fault.Internal, CodeFrameworkDetectorFailed, "framework detection could not be completed")
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

// mustFingerprintFault takes its Kind explicitly. An earlier helper hardcoded
// fault.InvalidArgument, which would silently misclassify any code whose
// registered Kind differs without the compiler noticing.
func mustFingerprintFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func wrapFingerprintFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// joinField builds a logical field path relative to the aggregate being validated.
func joinField(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
