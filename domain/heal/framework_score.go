package heal

import "github.com/Capsule7446/healix-core/domain/fingerprint"

func frameworkSimilarity(target, candidate fingerprint.FrameworkStack) float64 {
	if len(target) == 0 || len(candidate) == 0 {
		return 0
	}
	matches := 0
	for _, expected := range target {
		for _, actual := range candidate {
			if expected.Kind == actual.Kind && (expected.Version == "" || actual.Version == "" || expected.Version == actual.Version) {
				matches++
				break
			}
		}
	}
	return float64(matches) / float64(len(target))
}
