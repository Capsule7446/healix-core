package node

import (
	"errors"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestSnapshotErrorStoresSafeFaultContract(t *testing.T) {
	snapshot := snapshotError(errors.New("driver password=top-secret failed"))
	if snapshot == nil {
		t.Fatal("snapshotError() = nil")
	}
	if snapshot.Kind != fault.Internal || snapshot.Code != CodeOperationFailed {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Message != "node operation failed" {
		t.Fatalf("snapshot message = %q", snapshot.Message)
	}
}

func TestSnapshotErrorPreservesKnownFaultDetails(t *testing.T) {
	snapshot := snapshotError(transientDriverFault(errors.New("temporary")))
	if snapshot == nil || snapshot.Kind != fault.Unavailable || snapshot.Code != CodeTransientDriver {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Message != "node driver is temporarily unavailable" {
		t.Fatalf("snapshot message = %q", snapshot.Message)
	}
}
