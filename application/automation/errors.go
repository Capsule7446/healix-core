package automation

import (
	"errors"
	"fmt"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

var ErrRevisionConflict = errors.New("revision conflict")

type RevisionConflictError struct {
	AggregateKind string
	ID            string
	Expected      domain.Revision
	Actual        domain.Revision
}

func (e RevisionConflictError) Error() string {
	return fmt.Sprintf("%s %s: %v (expected %d, actual %d)", e.AggregateKind, e.ID, ErrRevisionConflict, e.Expected, e.Actual)
}

func (e RevisionConflictError) Unwrap() error { return ErrRevisionConflict }
