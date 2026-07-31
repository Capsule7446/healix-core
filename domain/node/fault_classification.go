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

func mustWrapNodeFault(cause error, kind fault.Kind, code fault.Code, message string) error {
	wrapped, err := fault.Wrap(cause, kind, code, message)
	if err != nil {
		panic(err)
	}
	return wrapped
}
