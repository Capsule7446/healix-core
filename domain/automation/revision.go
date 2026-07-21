package automation

import (
	"errors"
	"math"
)

var (
	ErrRevisionZero     = errors.New("persisted revision must be non-zero")
	ErrRevisionOverflow = errors.New("revision overflow")
)

// Revision is an opaque optimistic-concurrency token for a stable aggregate.
type Revision uint64

func (r Revision) ValidatePersisted() error {
	if r == 0 {
		return ErrRevisionZero
	}
	return nil
}

func (r Revision) Next() (Revision, error) {
	if err := r.ValidatePersisted(); err != nil {
		return 0, err
	}
	if r == Revision(math.MaxUint64) {
		return 0, ErrRevisionOverflow
	}
	return r + 1, nil
}
