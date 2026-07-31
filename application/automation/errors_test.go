package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestRevisionConflictErrorExposesSafeStableContract(t *testing.T) {
	err := AutomationRevisionConflictError()
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Code() != CodeAutomationRevisionConflict || descriptor.Kind() != fault.Conflict || descriptor.Message() != "automation revision conflicts with current state" {
		t.Fatalf("descriptor = %#v, ok = %v", descriptor, ok)
	}
	if len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
		t.Fatalf("public schema = %#v", descriptor)
	}
	for _, secret := range []string{"node-1", "expected 2", "actual 3"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("public error leaked %q: %v", secret, err)
		}
	}
}
