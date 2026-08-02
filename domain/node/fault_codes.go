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

	// CodeStepConfigurationInvalid covers every step-definition shape failure —
	// action, wait/repeat, assertion, workflow-call target/binding — where the
	// remediation is to supply a different declarative value. Violation field
	// paths distinguish the failing shape; no separate code is minted per field.
	CodeStepConfigurationInvalid fault.Code = "EXECUTION_STEP_CONFIGURATION_INVALID"
	// CodeStepPhaseTransitionInvalid covers the StepExecution phase machine and
	// the leaf lifecycle start boundary. FAILED_PRECONDITION: the caller must
	// reach or repair another state before this phase can be entered.
	CodeStepPhaseTransitionInvalid fault.Code = "EXECUTION_STEP_PHASE_TRANSITION_INVALID"
	// CodeHealingRefused covers a healing policy's refusal decision — safety
	// rejection or no candidate reaching the review cap — never an adapter
	// failure encountered while healing.
	CodeHealingRefused fault.Code = "EXECUTION_HEALING_REFUSED"
	// CodeEvidenceRecordFailed covers a Facts-port recording failure: rt.emit and
	// heal-decision/sample staging. Distinct from CodeTransientDriver, which is
	// the Driver port.
	CodeEvidenceRecordFailed fault.Code = "EXECUTION_EVIDENCE_RECORD_FAILED"
)
