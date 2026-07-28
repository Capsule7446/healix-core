package heal

import (
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type Disposition string

const (
	DispositionAllow  Disposition = "allow"
	DispositionReview Disposition = "review"
	DispositionBlock  Disposition = "block"
)

type ReasonCode string

const (
	ReasonNoCandidate    ReasonCode = "no_candidate"
	ReasonOriginMismatch ReasonCode = "origin_mismatch"
	ReasonPageMismatch   ReasonCode = "page_mismatch"
	ReasonRoleMismatch   ReasonCode = "role_mismatch"
	ReasonTagMismatch    ReasonCode = "tag_mismatch"
	ReasonFormMismatch   ReasonCode = "form_mismatch"
	ReasonAmbiguous      ReasonCode = "ambiguous"
	ReasonBelowCap       ReasonCode = "below_cap"
)

type ExecutionContext struct{ PageURL, Origin string }
type SafetyPolicy struct{ MinimumMargin float64 }
type Assessment struct {
	Disposition Disposition
	Reasons     []ReasonCode
	Explanation string
}

func Assess(target fingerprint.NodeSpec, decision Decision, current ExecutionContext, policy SafetyPolicy) (Assessment, error) {
	if err := decision.Validate(); err != nil {
		return Assessment{}, err
	}
	if math.IsNaN(policy.MinimumMargin) || math.IsInf(policy.MinimumMargin, 0) {
		return Assessment{}, fmt.Errorf("heal: minimum margin must be finite, got %v", policy.MinimumMargin)
	}
	if policy.MinimumMargin <= 0 {
		policy.MinimumMargin = 0.05
	}
	a := Assessment{Disposition: DispositionAllow}
	add := func(code ReasonCode) { a.Reasons = append(a.Reasons, code) }
	if decision.Outcome == OutcomeNoCandidate || decision.Best == nil {
		add(ReasonNoCandidate)
		a.Disposition = DispositionBlock
	}
	if a.Disposition != DispositionBlock {
		best := decision.Best.Fingerprint
		if target.Origin != "" && current.Origin != "" && target.Origin != current.Origin {
			add(ReasonOriginMismatch)
			a.Disposition = DispositionBlock
		}
		if target.PageURL != "" && current.PageURL != "" && normalizedURL(target.PageURL) != normalizedURL(current.PageURL) && a.Disposition != DispositionBlock {
			add(ReasonPageMismatch)
			a.Disposition = DispositionReview
		}
		if target.Role != "" && best.ARIA.Role != "" && target.Role != best.ARIA.Role {
			add(ReasonRoleMismatch)
			a.Disposition = DispositionBlock
		}
		if target.Fingerprint.Tag != "" && best.Tag != "" && target.Fingerprint.Tag != best.Tag {
			add(ReasonTagMismatch)
			a.Disposition = DispositionBlock
		}
		if target.Fingerprint.FormID != "" && best.FormID != "" && target.Fingerprint.FormID != best.FormID {
			add(ReasonFormMismatch)
			a.Disposition = DispositionBlock
		}
		if len(decision.Candidates) > 1 && policy.MinimumMargin > 0 && decision.Candidates[0].Score-decision.Candidates[1].Score < policy.MinimumMargin && a.Disposition == DispositionAllow {
			add(ReasonAmbiguous)
			a.Disposition = DispositionReview
		}
	}
	a.Explanation = fmt.Sprintf("%s: %s", a.Disposition, reasonText(a.Reasons))
	return a, nil
}
func reasonText(reasons []ReasonCode) string {
	if len(reasons) == 0 {
		return "safe"
	}
	parts := make([]string, len(reasons))
	for i, r := range reasons {
		parts[i] = string(r)
	}
	return strings.Join(parts, ",")
}
func normalizedURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String()
}
