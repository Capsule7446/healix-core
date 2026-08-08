package heal

import (
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// CandidateEvidence 是一个候选匹配信号的确定性投影。
type CandidateEvidence struct {
	Dimension string
	Score     float64
	Matched   bool
}

// EvidenceFor 按评分维度顺序返回稳定证据，不修改候选的持久化形状或决策顺序。
func EvidenceFor(target fingerprint.Fingerprint, candidate fingerprint.Fingerprint) []CandidateEvidence {
	return []CandidateEvidence{
		{Dimension: "tag", Score: boolScore(strings.EqualFold(target.Tag, candidate.Tag) && target.Tag != ""), Matched: target.Tag != "" && strings.EqualFold(target.Tag, candidate.Tag)},
		{Dimension: "id", Score: boolScore(target.Attributes["id"] != "" && target.Attributes["id"] == candidate.Attributes["id"]), Matched: target.Attributes["id"] != "" && target.Attributes["id"] == candidate.Attributes["id"]},
		{Dimension: "role", Score: boolScore(target.ARIA.Role != "" && target.ARIA.Role == candidate.ARIA.Role), Matched: target.ARIA.Role != "" && target.ARIA.Role == candidate.ARIA.Role},
		{Dimension: "form", Score: boolScore(target.FormID != "" && target.FormID == candidate.FormID), Matched: target.FormID != "" && target.FormID == candidate.FormID},
	}
}

// boolScore 将匹配布尔值转换为 1 或 0 的分数。
func boolScore(matched bool) float64 {
	if matched {
		return 1
	}
	return 0
}
