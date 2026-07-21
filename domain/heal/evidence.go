package heal

import (
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// CandidateEvidence is a deterministic projection of one candidate's matching signals.
type CandidateEvidence struct {
	Dimension string
	Score     float64
	Matched   bool
}

// EvidenceFor returns stable evidence in scorer-dimension order without changing
// the candidate's persisted shape or decision ordering.
func EvidenceFor(target fingerprint.Fingerprint, candidate fingerprint.Fingerprint) []CandidateEvidence {
	return []CandidateEvidence{
		{Dimension: "tag", Score: boolScore(strings.EqualFold(target.Tag, candidate.Tag) && target.Tag != ""), Matched: target.Tag != "" && strings.EqualFold(target.Tag, candidate.Tag)},
		{Dimension: "id", Score: boolScore(target.Attributes["id"] != "" && target.Attributes["id"] == candidate.Attributes["id"]), Matched: target.Attributes["id"] != "" && target.Attributes["id"] == candidate.Attributes["id"]},
		{Dimension: "role", Score: boolScore(target.ARIA.Role != "" && target.ARIA.Role == candidate.ARIA.Role), Matched: target.ARIA.Role != "" && target.ARIA.Role == candidate.ARIA.Role},
		{Dimension: "form", Score: boolScore(target.FormID != "" && target.FormID == candidate.FormID), Matched: target.FormID != "" && target.FormID == candidate.FormID},
	}
}

func boolScore(matched bool) float64 {
	if matched {
		return 1
	}
	return 0
}
