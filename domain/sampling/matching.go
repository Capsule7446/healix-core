package sampling

import "github.com/Capsule7446/healix-core/domain/fingerprint"

// MatchProfile 是框架中立的身份信号，用于将捕获的元素与现有基线进行比较。它故意不包含工作空间聚合或持久性问题。
type MatchProfile struct {
	Selectors   []fingerprint.Selector
	Fingerprint fingerprint.Fingerprint
	Origin      string
}

// Match 匹配测量选择器重叠和稳定的指纹一致性。权重是域策略：调用者仅将其聚合映射到配置文件中。
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

func uniqueSelectorCount(selectors []fingerprint.Selector) int {
	values := make(map[string]struct{}, len(selectors))
	for _, selector := range selectors {
		values[string(selector.Type)+"\x00"+selector.Value] = struct{}{}
	}
	return len(values)
}

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
