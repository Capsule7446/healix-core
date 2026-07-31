package execution

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	CodePlanUnsealed               fault.Code = "EXECUTION_PLAN_UNSEALED"
	CodeStatusTransitionInvalid    fault.Code = "EXECUTION_STATUS_TRANSITION_INVALID"
	CodeRunStatusTransitionInvalid fault.Code = "EXECUTION_RUN_STATUS_TRANSITION_INVALID"
	CodeWorkerFenceStale           fault.Code = "EXECUTION_WORKER_FENCE_STALE"
)

func mustExecutionFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
