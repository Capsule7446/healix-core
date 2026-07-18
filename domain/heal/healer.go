package heal

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// 阈值定义用于对治疗决策进行分类的置信边界。
type Thresholds struct {
	AppliedCap float64 // 分数 >= AppliedCap：直接应用
	ReviewCap  float64 // ReviewCap <= 分数 < AppliedCap：应用，但强制待审 + 录屏
}

// DefaultThresholds 返回一份新的默认阈值，避免调用方污染后续 Run 的策略。
func DefaultThresholds() Thresholds {
	return Thresholds{AppliedCap: 0.85, ReviewCap: 0.60}
}

// Validate 要求阈值落在相似度区间内，且 review 档严格低于直接应用档，
// 从而始终保留一个可供人工复审的 below_cap 区间。
func (t Thresholds) Validate() error {
	if math.IsNaN(t.ReviewCap) || math.IsInf(t.ReviewCap, 0) || t.ReviewCap < 0 || t.ReviewCap > 1 {
		return fmt.Errorf("heal: review_cap must be finite and within [0,1], got %v", t.ReviewCap)
	}
	if math.IsNaN(t.AppliedCap) || math.IsInf(t.AppliedCap, 0) || t.AppliedCap < 0 || t.AppliedCap > 1 {
		return fmt.Errorf("heal: applied_cap must be finite and within [0,1], got %v", t.AppliedCap)
	}
	if t.ReviewCap >= t.AppliedCap {
		return fmt.Errorf("heal: review_cap %v must be lower than applied_cap %v", t.ReviewCap, t.AppliedCap)
	}
	return nil
}

// DefaultHealer 是 Healer 的确定性实现。它通过祖先路径相似性缩小候选范围，对剩余候选进行评分，并使用配置的阈值对最佳结果进行分类。
type DefaultHealer struct {
	Weights    Weights
	Thresholds Thresholds
}

// NewDefaultHealer 使用包默认值构造一个 DefaultHealer。
func NewDefaultHealer() *DefaultHealer {
	policy := DefaultPolicyV1()
	return &DefaultHealer{Weights: policy.Weights, Thresholds: policy.Thresholds}
}

// NewDefaultHealerWithPolicy 从完整的、经过验证的策略快照构造一个治疗器。该值被复制，因此后续调用者更改无法更改正在进行的运行。
func NewDefaultHealerWithPolicy(policy PolicyV1) (*DefaultHealer, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &DefaultHealer{Weights: policy.Weights, Thresholds: policy.Thresholds}, nil
}

// Validate 检查 DefaultHealer 的全部可配置领域参数。
func (h *DefaultHealer) Validate() error {
	if h == nil {
		return fmt.Errorf("heal: default healer is nil")
	}
	if err := h.Thresholds.Validate(); err != nil {
		return err
	}
	return h.Weights.Validate()
}

var _ Healer = (*DefaultHealer)(nil)

func (h *DefaultHealer) Heal(ctx context.Context, target fingerprint.NodeSpec, snapshot DOMSnapshot) (Decision, error) {
	if err := h.Validate(); err != nil {
		return Decision{}, err
	}
	if snapshot == nil {
		return Decision{}, fmt.Errorf("heal: snapshot is nil")
	}
	all, err := snapshot.Candidates(ctx)
	if err != nil {
		return Decision{}, err
	}
	if len(all) == 0 {
		return validateDecision(Decision{Outcome: OutcomeNoCandidate})
	}

	narrowed := narrowByPathLCS(target.Fingerprint.Path, all)

	scorer := prepareTargetScorer(h.Weights, target.Fingerprint)
	scored := make([]Candidate, 0, len(narrowed))
	for _, c := range narrowed {
		scored = append(scored, Candidate{
			Selector:    c.Selector,
			Fingerprint: c.Fingerprint,
			Score:       scorer.score(c.Fingerprint),
		})
	}
	sortCandidates(scored)

	decision := Decision{Candidates: scored}
	if len(scored) == 0 {
		decision.Outcome = OutcomeNoCandidate
		return validateDecision(decision)
	}

	best := scored[0]
	switch {
	case best.Score >= h.Thresholds.AppliedCap:
		decision.Outcome = OutcomeApplied
		decision.Best = &best
	case best.Score >= h.Thresholds.ReviewCap:
		decision.Outcome = OutcomeBelowCap
		decision.NeedsReview = true
		decision.Best = &best
	default:
		decision.Outcome = OutcomeNoCandidate
	}
	return validateDecision(decision)
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidateLess(candidates[i], candidates[j])
	})
}

func validateDecision(decision Decision) (Decision, error) {
	if err := decision.Validate(); err != nil {
		return Decision{}, err
	}
	return decision, nil
}
