package execution

import (
	"errors"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// WorkerFence identifies the sole Host worker permitted to mutate one Run.
// ClaimToken is opaque and Host-generated. It must be globally unique for every
// successful acquisition, including reacquisition by the same worker, so an old
// released owner can never pass the fence through an ABA token reuse.
type WorkerFence struct {
	RunID      InstanceID
	ClaimToken string
}

func (f WorkerFence) Validate() error {
	if f.RunID.Validate() != nil || f.ClaimToken == "" {
		return errors.New("worker fence run id and claim token are required")
	}
	return nil
}

func NewStaleWorkerFenceError() error {
	return mustExecutionFault(fault.Conflict, CodeWorkerFenceStale, "worker execution authority is stale")
}
