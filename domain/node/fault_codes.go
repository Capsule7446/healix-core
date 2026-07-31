package node

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodeElementNotFound           fault.Code = "EXECUTION_ELEMENT_NOT_FOUND"
	CodeTimeout                   fault.Code = "EXECUTION_OPERATION_TIMEOUT"
	CodeCanceled                  fault.Code = "EXECUTION_OPERATION_CANCELED"
	CodeTransientDriver           fault.Code = "EXECUTION_TRANSIENT_DRIVER"
	CodeOperationFailed           fault.Code = "EXECUTION_OPERATION_FAILED"
	CodeStepTimelineStartFailed   fault.Code = "EXECUTION_STEP_TIMELINE_START_FAILED"
	CodeStepTimelineFinishFailed  fault.Code = "EXECUTION_STEP_TIMELINE_FINISH_FAILED"
	CodeNodeCompletionObservation fault.Code = "EXECUTION_NODE_COMPLETION_OBSERVATION_FAILED"
	CodeLeafCompletionFailed      fault.Code = "EXECUTION_LEAF_COMPLETION_FAILED"
)
