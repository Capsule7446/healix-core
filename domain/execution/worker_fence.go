package execution

import (
	"github.com/Capsule7446/healix-core/domain/fault"
)

// WorkerFence identifies the sole Host worker permitted to mutate one Instance.
// ClaimToken is opaque and Host-generated. It must be globally unique for every
// successful acquisition, including reacquisition by the same worker, so an old
// released owner can never pass the fence through an ABA token reuse.
type WorkerFence struct {
	InstanceID InstanceID
	ClaimToken string
}

func (f WorkerFence) Validate() error {
	if f.InstanceID.Validate() != nil || f.ClaimToken == "" {
		return mustExecutionFault(fault.InvalidArgument, CodeWorkerFenceInvalid, "worker execution authority is invalid")
	}
	return nil
}

func NewStaleWorkerFenceError() error {
	return mustExecutionFault(fault.Conflict, CodeWorkerFenceStale, "worker execution authority is stale")
}
