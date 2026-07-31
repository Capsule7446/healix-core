package node

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeElementNotFound fault.Code = "EXECUTION_ELEMENT_NOT_FOUND"
	CodeTimeout         fault.Code = "EXECUTION_OPERATION_TIMEOUT"
	CodeCanceled        fault.Code = "EXECUTION_OPERATION_CANCELED"
	CodeTransientDriver fault.Code = "EXECUTION_TRANSIENT_DRIVER"
	CodeOperationFailed fault.Code = "EXECUTION_OPERATION_FAILED"
)
