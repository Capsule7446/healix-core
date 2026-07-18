package heal

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// candidateLess defines the complete candidate order. Score remains the
// semantic ranking signal; the remaining fields are deterministic arbitration
// keys only and do not claim that one equally-scored candidate is more likely
// to be correct than another.
func candidateLess(left, right Candidate) bool {
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.Selector.Type != right.Selector.Type {
		return left.Selector.Type < right.Selector.Type
	}
	if left.Selector.Value != right.Selector.Value {
		return left.Selector.Value < right.Selector.Value
	}
	if left.Selector.Priority != right.Selector.Priority {
		return left.Selector.Priority < right.Selector.Priority
	}
	return fingerprintCanonicalKey(left.Fingerprint) < fingerprintCanonicalKey(right.Fingerprint)
}

func sameCandidate(left, right Candidate) bool {
	return left.Score == right.Score &&
		left.Selector.Type == right.Selector.Type &&
		left.Selector.Value == right.Selector.Value &&
		left.Selector.Priority == right.Selector.Priority &&
		fingerprintCanonicalKey(left.Fingerprint) == fingerprintCanonicalKey(right.Fingerprint)
}

// fingerprintCanonicalKey serializes every fingerprint field with explicit
// length prefixes and sorted map keys. It is deliberately independent of Go's
// map iteration order and of infrastructure JSON encoders.
func fingerprintCanonicalKey(value fingerprint.Fingerprint) string {
	var out strings.Builder
	writeCanonicalString(&out, value.Tag)
	keys := make([]string, 0, len(value.Attributes))
	for key := range value.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	writeCanonicalInt(&out, len(keys))
	for _, key := range keys {
		writeCanonicalString(&out, key)
		writeCanonicalString(&out, value.Attributes[key])
	}
	writeCanonicalString(&out, value.Text)
	writeCanonicalString(&out, value.ARIA.Role)
	writeCanonicalString(&out, value.ARIA.Name)
	writeCanonicalInt(&out, len(value.Path))
	for _, segment := range value.Path {
		writeCanonicalString(&out, segment)
	}
	writeCanonicalInt(&out, value.SiblingIndex)
	writeCanonicalString(&out, value.Neighbors.Prev)
	writeCanonicalString(&out, value.Neighbors.Next)
	writeCanonicalString(&out, value.Neighbors.ParentTag)
	writeCanonicalString(&out, value.LabelText)
	writeCanonicalString(&out, value.FormID)
	return out.String()
}

func writeCanonicalString(out *strings.Builder, value string) {
	writeCanonicalInt(out, len(value))
	out.WriteByte(':')
	out.WriteString(value)
}

func writeCanonicalInt(out *strings.Builder, value int) {
	out.WriteString(strconv.Itoa(value))
	out.WriteByte(';')
}
