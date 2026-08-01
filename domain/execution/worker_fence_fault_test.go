package execution

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestWorkerFenceInvalidErrorCarriesCode(t *testing.T) {
	tests := []struct {
		name  string
		fence WorkerFence
	}{
		{name: "empty claim token", fence: WorkerFence{InstanceID: mustInstanceID("run")}},
		{name: "empty instance id", fence: WorkerFence{ClaimToken: "claim"}},
		{name: "zero instance id", fence: WorkerFence{InstanceID: InstanceID{}, ClaimToken: "claim"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.fence.Validate()
			if err == nil {
				t.Fatal("Validate() returned nil, want error")
			}
			if !fault.IsCode(err, CodeWorkerFenceInvalid) {
				t.Fatalf("Validate() error = %v, want code %s", err, CodeWorkerFenceInvalid)
			}
		})
	}
}

func TestWorkerFenceInvalidIsDistinctFromStale(t *testing.T) {
	if CodeWorkerFenceInvalid == CodeWorkerFenceStale {
		t.Fatal("CodeWorkerFenceInvalid and CodeWorkerFenceStale are the same code")
	}
	malformed := WorkerFence{InstanceID: mustInstanceID("run")}.Validate()
	if fault.IsCode(malformed, CodeWorkerFenceStale) {
		t.Fatal("a malformed fence error should not match CodeWorkerFenceStale")
	}
}

func TestWorkerFenceValidReturnsNil(t *testing.T) {
	fence := WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "claim"}
	if err := fence.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}
