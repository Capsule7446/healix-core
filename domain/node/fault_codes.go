package node

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeElementNotFound fault.Code = "NODE_ELEMENT_NOT_FOUND"
	CodeTimeout         fault.Code = "NODE_TIMEOUT"
	CodeCanceled        fault.Code = "NODE_CANCELED"
	CodeTransientDriver fault.Code = "NODE_TRANSIENT_DRIVER"
	CodeOperationFailed fault.Code = "NODE_OPERATION_FAILED"
)
