package automation

import (
	"errors"
	"math"
	"testing"
)

func TestRevisionValidationAndOverflow(t *testing.T) {
	if !errors.Is(Revision(0).ValidatePersisted(), ErrRevisionZero) {
		t.Fatal("zero revision accepted")
	}
	if next, err := Revision(1).Next(); err != nil || next != 2 {
		t.Fatalf("next = %d, %v", next, err)
	}
	if _, err := Revision(math.MaxUint64).Next(); !errors.Is(err, ErrRevisionOverflow) {
		t.Fatalf("overflow = %v", err)
	}
}

func TestEnvironmentLifecycleIsImmutableAndIncrementsOnce(t *testing.T) {
	original, err := NewEnvironment(Environment{ID: "env", DisplayName: "Env", Variables: Properties{"tenant": "a"}, Properties: Properties{}, CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := original.UpdateMetadata("New", "https://example.com", Properties{"tenant": "b"}, Properties{"x": "y"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || original.Revision != 1 || original.Variables["tenant"] != "a" {
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
