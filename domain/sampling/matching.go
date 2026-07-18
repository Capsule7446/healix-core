package sampling

import "github.com/Capsule7446/healix-core/domain/fingerprint"

// MatchProfile is the framework-neutral identity signal used to compare a
// captured element with an existing baseline. It deliberately contains no
// workspace aggregate or persistence concern.
type MatchProfile struct {
	Selectors   []fingerprint.Selector
	Fingerprint fingerprint.Fingerprint
	Origin      string
}

// Match measures selector overlap and stable fingerprint agreement. The
// weights are domain policy: callers only map their aggregate into profiles.
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
