package execution

import (
	"errors"
	"fmt"
)

// WorkerFence identifies the sole Host worker permitted to mutate one Run.
// ClaimToken is opaque and Host-generated. It must be globally unique for every
// successful acquisition, including reacquisition by the same worker, so an old
// released owner can never pass the fence through an ABA token reuse.
type WorkerFence struct {
	RunID      string
	ClaimToken string
}

func (f WorkerFence) Validate() error {
	if f.RunID == "" || f.ClaimToken == "" {
		return errors.New("worker fence run id and claim token are required")
	}
	return nil
}

var ErrStaleWorkerFence = errors.New("stale worker fence")

type StaleWorkerFenceError struct {
	Fence WorkerFence
}

func (e *StaleWorkerFenceError) Error() string {
	return fmt.Sprintf("%v: run %q", ErrStaleWorkerFence, e.Fence.RunID)
}

func (e *StaleWorkerFenceError) Is(target error) bool {
	return target == ErrStaleWorkerFence
}
