package automation

import (
	"math"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestRevisionValidationAndOverflow(t *testing.T) {
	invalid := Revision(0).ValidatePersisted()
	invalidDescriptor, invalidOK := fault.Describe(invalid)
	if !fault.IsCode(invalid, CodePersistedRevisionInvalid) || !invalidOK || invalidDescriptor.Kind() != fault.FailedPrecondition || invalidDescriptor.Message() != "persisted revision must be non-zero" || len(invalidDescriptor.Params()) != 0 || len(invalidDescriptor.Violations()) != 0 {
		t.Fatalf("zero revision error/descriptor = %v/%#v", invalid, invalidDescriptor)
	}
	if next, err := Revision(1).Next(); err != nil || next != 2 {
		t.Fatalf("next = %d, %v", next, err)
	}
	if err := Revision(math.MaxUint64).ValidatePersisted(); err != nil {
		t.Fatalf("maximum persisted revision rejected: %v", err)
	}
	next, exhausted := Revision(math.MaxUint64).Next()
	exhaustedDescriptor, exhaustedOK := fault.Describe(exhausted)
	if next != 0 || !fault.IsCode(exhausted, CodeRevisionExhausted) || !exhaustedOK || exhaustedDescriptor.Kind() != fault.ResourceExhausted || exhaustedDescriptor.Message() != "revision value is exhausted" || len(exhaustedDescriptor.Params()) != 0 || len(exhaustedDescriptor.Violations()) != 0 {
		t.Fatalf("overflow result/error/descriptor = %d/%v/%#v", next, exhausted, exhaustedDescriptor)
	}
}

func TestEnvironmentLifecycleIsImmutableAndIncrementsOnce(t *testing.T) {
	original, err := NewEnvironment(Environment{ID: "env", DisplayName: "Env", Variables: EnvironmentVariables{"tenant": parameter.TextValue("a")}, CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := original.UpdateMetadata("New", "https://example.com", EnvironmentVariables{"x": parameter.TextValue("y")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || original.Revision != 1 || original.Variables["tenant"].Text() != "a" {
		t.Fatal("update mutated input or revision")
	}
	deleted, err := updated.Delete(2)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := deleted.Restore(2)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Revision != 3 || restored.Revision != 4 || restored.DeletedAt != 0 {
		t.Fatal("lifecycle revisions invalid")
	}
}
