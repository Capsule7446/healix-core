package heal

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// CandidateLess 定义完整的候选顺序。分数仍然是语义排名信号；其余字段仅是确定性仲裁密钥，并不表明一个得分相同的候选者比另一个候选者更有可能是正确的。
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

// FingerprintCanonicalKey 使用显式长度前缀和排序的映射键序列化每个指纹字段。它故意独立于 Go 的映射迭代顺序和基础设施 JSON 编码器。
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
