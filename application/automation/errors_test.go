package automation

import (
	"errors"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

func TestRevisionConflictErrorSupportsErrorsIsAndReportsValues(t *testing.T) {
	err := RevisionConflictError{AggregateKind: "node", ID: "node-1", Expected: domain.Revision(2), Actual: domain.Revision(3)}
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("errors.Is did not match")
	}
	if err.Expected != 2 || err.Actual != 3 || err.ID != "node-1" {
		t.Fatal("conflict detail lost")
	}
}
