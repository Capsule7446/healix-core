package automation

import "math"

// Revision 是稳定聚合使用的不透明乐观并发令牌。
type Revision uint64

// ValidatePersisted 校验修订已持久化且非零。
func (r Revision) ValidatePersisted() error {
	if r == 0 {
		return persistedRevisionInvalidError()
	}
	return nil
}

// Next 返回递增后的修订；零值或达到 uint64 上限时返回对应错误。
func (r Revision) Next() (Revision, error) {
	if err := r.ValidatePersisted(); err != nil {
		return 0, err
	}
	if r == Revision(math.MaxUint64) {
		return 0, revisionExhaustedError()
	}
	return r + 1, nil
}
