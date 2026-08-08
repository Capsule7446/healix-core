package sampling

import "github.com/Capsule7446/healix-core/domain/fingerprint"

// MatchProfile 包含用于比较采样元素与基线的选择器、指纹和来源信号。
type MatchProfile struct {
	Selectors   []fingerprint.Selector
	Fingerprint fingerprint.Fingerprint
	Origin      string
}

// Match 计算采样配置与基线配置的相似度及唯一选择器重叠数。
func Match(sampled, baseline MatchProfile) (similarity float64, selectorOverlap int) {
	selectorOverlap = overlap(sampled.Selectors, baseline.Selectors)
	union := uniqueSelectorCount(sampled.Selectors) + uniqueSelectorCount(baseline.Selectors) - selectorOverlap
	if union > 0 {
		similarity = float64(selectorOverlap) / float64(union) * .62
	}
	if sampled.Fingerprint.Tag != "" && sampled.Fingerprint.Tag == baseline.Fingerprint.Tag {
		similarity += .13
	}
	if sampled.Fingerprint.ARIA.Role != "" && sampled.Fingerprint.ARIA.Role == baseline.Fingerprint.ARIA.Role {
		similarity += .1
	}
	if sampled.Fingerprint.ARIA.Name != "" && sampled.Fingerprint.ARIA.Name == baseline.Fingerprint.ARIA.Name {
		similarity += .1
	}
	if sampled.Origin != "" && sampled.Origin == baseline.Origin {
		similarity += .05
	}
	return similarity, selectorOverlap
}

// uniqueSelectorCount 返回选择器类型和值组合的去重数量。
func uniqueSelectorCount(selectors []fingerprint.Selector) int {
	values := make(map[string]struct{}, len(selectors))
	for _, selector := range selectors {
		values[string(selector.Type)+"\x00"+selector.Value] = struct{}{}
	}
	return len(values)
}

// overlap 返回两个选择器集合的唯一交集数量。
func overlap(left, right []fingerprint.Selector) int {
	values := make(map[string]struct{}, len(left))
	for _, selector := range left {
		values[string(selector.Type)+"\x00"+selector.Value] = struct{}{}
	}
	count := 0
	for _, selector := range right {
		key := string(selector.Type) + "\x00" + selector.Value
		if _, exists := values[key]; exists {
			count++
			delete(values, key)
		}
	}
	return count
}
