package scheduling

import (
	"context"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// TestAbortInstanceKeepsOneCodeAndDistinctCauseChains settles the open question
// about validateAbort publishing the same top-level code from two branches. One
// code is right: the caller's contract is "this abort command is invalid", and
// the branches differ in why, not in what. What must not drift is the cause. A
// malformed fence carries its own fault forward so an operator can see which
// authority check failed; a well-formed fence that simply belongs to another
// instance has no sub-fault to carry, and inventing one would put a second
// meaning behind the same code.
func TestAbortInstanceKeepsOneCodeAndDistinctCauseChains(t *testing.T) {
	valid := AbortInstanceCommand{
		CommandID: "command", InstanceID: mustInstanceID("run"), ExpectedRevision: 0, At: 1,
		Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim"},
	}
	foreignFence := valid
	foreignFence.Fence.InstanceID = mustInstanceID("other")
	malformedFence := valid
	malformedFence.Fence.ClaimToken = ""

	tests := []struct {
		name           string
		command        AbortInstanceCommand
		wantFenceCause bool
	}{
		{name: "well formed fence for another instance", command: foreignFence},
		{name: "malformed fence", command: malformedFence, wantFenceCause: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &commandStoreStub{}
			result, err := NewAbortInstanceService(store, nil).AbortInstance(context.Background(), test.command)
			if !fault.IsCode(err, CodeAbortInstanceCommandInvalid) {
				t.Fatalf("AbortInstance() error = %v, want %s", err, CodeAbortInstanceCommandInvalid)
			}
			if got := fault.IsCode(err, domainexecution.CodeWorkerFenceInvalid); got != test.wantFenceCause {
				t.Errorf("fence fault present in cause chain = %t, want %t (error %v)", got, test.wantFenceCause, err)
			}
			if result != (InstanceCommandResult{}) || len(store.calls) != 0 {
				t.Fatalf("rejected command reached the store: result %#v, calls %v", result, store.calls)
			}
		})
	}
}
