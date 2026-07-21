package heal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// CandidateSample is the replay-safe projection of one ranked healing candidate.
type CandidateSample struct {
	CandidateHash   string
	FingerprintHash string
	Score           float64
	Rank            int
	Eligible        bool
	Selected        bool
	Status          string
	Evidence        []CandidateEvidence
}

const (
	CandidateSampleEligible = "eligible"
	CandidateSampleBelowCap = "below_review_cap"
	CandidateSampleSelected = "selected"
)

// Samples returns a deterministic, immutable projection of the decision candidates.
func (d Decision) Samples(target fingerprint.Fingerprint, reviewCap float64) []CandidateSample {
	if math.IsNaN(reviewCap) || math.IsInf(reviewCap, 0) || reviewCap < 0 || reviewCap > 1 {
		return nil
	}
	out := make([]CandidateSample, 0, len(d.Candidates))
	for index, candidate := range d.Candidates {
		eligible := candidate.Score >= reviewCap
		status := CandidateSampleBelowCap
		if eligible {
			status = CandidateSampleEligible
		}
		selected := d.Best != nil && sameCandidate(candidate, *d.Best)
		if selected {
			status = CandidateSampleSelected
		}
		out = append(out, CandidateSample{
			CandidateHash: CandidateHash(candidate), FingerprintHash: fingerprintHash(candidate.Fingerprint),
			Score: candidate.Score, Rank: index + 1,
			Eligible: eligible, Selected: selected, Status: status,
			Evidence: EvidenceFor(target, candidate.Fingerprint),
		})
	}
	return out
}

// CandidateHash is stable across processes and excludes raw page content.
func CandidateHash(candidate Candidate) string {
	var value strings.Builder
	writeCanonicalString(&value, "heal-candidate:v3")
	writeCanonicalString(&value, string(candidate.Selector.Type))
	writeCanonicalString(&value, candidate.Selector.Value)
	writeCanonicalInt(&value, candidate.Selector.Priority)
	writeCanonicalString(&value, fingerprintCanonicalKey(candidate.Fingerprint))
	digest := sha256.Sum256([]byte(value.String()))
	return hex.EncodeToString(digest[:])
}

func fingerprintHash(value fingerprint.Fingerprint) string {
	digest := sha256.Sum256([]byte("fingerprint:v2\x00" + fingerprintCanonicalKey(value)))
	return hex.EncodeToString(digest[:])
}

func ValidateSamples(samples []CandidateSample) error {
	seenSelected := false
	for index, sample := range samples {
		if sample.Rank != index+1 || sample.CandidateHash == "" || sample.FingerprintHash == "" || math.IsNaN(sample.Score) || math.IsInf(sample.Score, 0) || sample.Score < 0 || sample.Score > 1 {
			return fmt.Errorf("heal: invalid candidate sample at rank %d", index+1)
		}
		if sample.Selected {
			if seenSelected || !sample.Eligible {
				return fmt.Errorf("heal: candidate sample selection is invalid")
			}
			seenSelected = true
		}
	}
	return nil
}

// SortSamples provides a defensive copy for adapters that need stable ordering.
func SortSamples(samples []CandidateSample) []CandidateSample {
	out := append([]CandidateSample(nil), samples...)
	for i := range out {
		out[i].Evidence = append([]CandidateEvidence(nil), out[i].Evidence...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}
