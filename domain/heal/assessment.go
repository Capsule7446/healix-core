package heal

import (
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// Disposition 表示自愈候选的安全处置结果。
type Disposition string

const (
	// DispositionAllow 表示候选可直接应用。
	DispositionAllow Disposition = "allow"
	// DispositionReview 表示候选需人工审查后再决定。
	DispositionReview Disposition = "review"
	// DispositionBlock 表示候选违反安全边界，禁止应用。
	DispositionBlock Disposition = "block"
)

// ReasonCode 标识导致自愈降级或阻止的具体原因。
type ReasonCode string

const (
	// ReasonNoCandidate 表示没有可用候选。
	ReasonNoCandidate ReasonCode = "no_candidate"
	// ReasonOriginMismatch 表示候选记录的来源与当前来源不一致。
	ReasonOriginMismatch ReasonCode = "origin_mismatch"
	// ReasonOriginUnknown 表示当前来源未知，无法确认安全边界。
	ReasonOriginUnknown ReasonCode = "origin_unknown"
	// ReasonPageMismatch 表示候选记录的页面与当前页面不一致。
	ReasonPageMismatch ReasonCode = "page_mismatch"
	// ReasonPageUnknown 表示当前页面未知，只能降级处置。
	ReasonPageUnknown ReasonCode = "page_unknown"
	// ReasonRoleMismatch 表示目标 role 与候选 role 不一致。
	ReasonRoleMismatch ReasonCode = "role_mismatch"
	// ReasonTagMismatch 表示目标标签与候选标签不一致。
	ReasonTagMismatch ReasonCode = "tag_mismatch"
	// ReasonFormMismatch 表示目标表单与候选表单不一致。
	ReasonFormMismatch ReasonCode = "form_mismatch"
	// ReasonAmbiguous 表示最佳候选与次佳候选的分数差不足。
	ReasonAmbiguous ReasonCode = "ambiguous"
)

// ExecutionContext 提供当前页面 URL 和来源，用于自愈安全评估。
type ExecutionContext struct{ PageURL, Origin string }

// SafetyPolicy 配置候选分数差的最低安全裕量。
type SafetyPolicy struct{ MinimumMargin float64 }

// Assessment 保存自愈处置、原因列表和面向调用方的解释。
type Assessment struct {
	Disposition Disposition
	Reasons     []ReasonCode
	Explanation string
}

// Assess 根据候选决策、目标身份、当前页面上下文和安全策略确定自愈处置。
func Assess(target fingerprint.ElementTargetSpec, decision Decision, current ExecutionContext, policy SafetyPolicy) (Assessment, error) {
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
		// 目标记录了来源时，只有确认当前来源一致才能自愈；未知来源应拒绝，而不是放行，
		// 因为无法报告位置的页面并不比已知但错误的位置更安全。
		if target.Origin != "" {
			switch {
			case current.Origin == "":
				add(ReasonOriginUnknown)
				a.Disposition = DispositionBlock
			case target.Origin != current.Origin:
				add(ReasonOriginMismatch)
				a.Disposition = DispositionBlock
			}
		}
		// 页面 URL 比来源弱，同源导航属于正常情况，因此页面无法确认时降级为审查而非直接阻止。
		if target.PageURL != "" && a.Disposition != DispositionBlock {
			switch {
			case current.PageURL == "":
				add(ReasonPageUnknown)
				a.Disposition = DispositionReview
			case normalizedURL(target.PageURL) != normalizedURL(current.PageURL):
				add(ReasonPageMismatch)
				a.Disposition = DispositionReview
			}
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

// reasonText 将原因码按顺序拼接为解释文本；无原因时返回固定的 `safe` 文本。
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

// normalizedURL 解析 URL 并移除片段、统一 scheme 和 host 大小写。
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
