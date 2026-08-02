package node

import (
	"context"
	"errors"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func transientDriverFault(cause error) error {
	if cause == nil {
		return nil
	}
	return mustWrapNodeFault(cause, fault.Unavailable, CodeTransientDriver, "node driver is temporarily unavailable")
}

func classifyNodeFault(cause error) error {
	if cause == nil {
		return nil
	}
	if _, ok := fault.CodeOf(cause); ok {
		return cause
	}
	switch {
	case errors.Is(cause, context.Canceled):
		return mustWrapNodeFault(cause, fault.Canceled, CodeCanceled, "node operation was canceled")
	case errors.Is(cause, context.DeadlineExceeded):
		return mustWrapNodeFault(cause, fault.DeadlineExceeded, CodeTimeout, "node operation timed out")
	case fault.IsCode(cause, CodeElementNotFound):
		return mustWrapNodeFault(cause, fault.NotFound, CodeElementNotFound, "element was not found")
	default:
		return mustWrapNodeFault(cause, fault.Internal, CodeOperationFailed, "node operation failed")
	}
}

func nodeFaultDetails(err error) (fault.Kind, fault.Code) {
	if err == nil {
		return "", ""
	}
	if kind, ok := fault.KindOf(err); ok {
		code, _ := fault.CodeOf(err)
		return kind, code
	}
	classified := classifyNodeFault(err)
	kind, _ := fault.KindOf(classified)
	code, _ := fault.CodeOf(classified)
	return kind, code
}

func nodeFaultKind(err error) fault.Kind {
	kind, _ := nodeFaultDetails(err)
	return kind
}

func nodeFaultCode(err error) fault.Code {
	_, code := nodeFaultDetails(err)
	return code
}

func mustWrapNodeFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	wrapped, err := fault.Wrap(cause, kind, code, message, options...)
	if err != nil {
		panic(err)
	}
	return wrapped
}

// classifyStepPhaseTransitionInvalid is the boundary classifier for the
// StepExecution phase machine and the leaf lifecycle start boundary: it passes
// an already-classified failure through unchanged (a Facts-port recording
// failure from rt.emit already carries EXECUTION_EVIDENCE_RECORD_FAILED) and
// gives every other phase-entry failure the phase-transition code, dropping
// the node id and phase names it used to echo in that public text.
func classifyStepPhaseTransitionInvalid(cause error) error {
	if cause == nil {
		return nil
	}
	if _, ok := fault.CodeOf(cause); ok {
		return cause
	}
	return stepPhaseTransitionInvalidError(cause)
}

func stepPhaseTransitionInvalidError(cause error) error {
	return mustWrapNodeFault(cause, fault.FailedPrecondition, CodeStepPhaseTransitionInvalid, "step phase transition is invalid")
}

// stepConfigurationInvalidError builds the aggregate-shaped envelope for a
// step-definition shape failure with no separate underlying cause: the
// violation itself carries the whole detail.
func stepConfigurationInvalidError(violations ...fault.Violation) error {
	return mustNodeFault(fault.InvalidArgument, CodeStepConfigurationInvalid, "step configuration is invalid", fault.WithViolations(violations...))
}

// wrapStepConfigurationInvalidError is the same envelope, retaining an
// underlying Go error as the private cause — used when the rejected value
// itself (a URL scheme, an unsupported kind) must never reach public text.
func wrapStepConfigurationInvalidError(cause error, violations ...fault.Violation) error {
	return mustWrapNodeFault(cause, fault.InvalidArgument, CodeStepConfigurationInvalid, "step configuration is invalid", fault.WithViolations(violations...))
}

// healingRefusedError covers only a healing policy's own refusal decision —
// safety rejection or no candidate reaching the review cap — never an adapter
// failure encountered while healing (that is evidenceRecordFailedError).
func healingRefusedError(cause error) error {
	return mustWrapNodeFault(cause, fault.FailedPrecondition, CodeHealingRefused, "healing was refused")
}

// evidenceRecordFailedError covers a Facts-port recording failure: rt.emit,
// heal-sample staging, and heal-decision staging. Distinct from
// CodeTransientDriver, which classifies Driver-port failures, not Facts-port
// ones.
func evidenceRecordFailedError(cause error) error {
	return mustWrapNodeFault(cause, fault.Unavailable, CodeEvidenceRecordFailed, "execution evidence could not be recorded")
}

func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}
