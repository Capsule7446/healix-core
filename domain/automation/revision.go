package automation

import "math"

// Revision is an opaque optimistic-concurrency token for a stable aggregate.
type Revision uint64

func (r Revision) ValidatePersisted() error {
	if r == 0 {
		return persistedRevisionInvalidError()
	}
	return nil
}

func (r Revision) Next() (Revision, error) {
	if err := r.ValidatePersisted(); err != nil {
		return 0, err
	}
	if r == Revision(math.MaxUint64) {
		return 0, revisionExhaustedError()
	}
	return r + 1, nil
}
