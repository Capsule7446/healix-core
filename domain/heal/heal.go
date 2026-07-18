// Package heal defines deterministic selector relocation ports and their default
// implementation. The package performs pure scoring without browser or LLM
// dependencies, keeping relocation decisions in the domain layer.
package heal

import (
	"context"
	"fmt"
	"math"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// Outcome classifies the result of a healing attempt for persistence and
// downstream execution decisions.
type Outcome string

const (
	// OutcomeApplied 是高置信度的自愈（分数 >= applied_cap），
	// 直接应用并继续正常运行。
	OutcomeApplied Outcome = "applied"
	// OutcomeBelowCap 是中置信度的自愈（review_cap <= 分数 < applied_cap）：
	// applied, but marked for human review.
	OutcomeBelowCap Outcome = "below_cap"
	// OutcomeNoCandidate 表示没有候选达到 review_cap；
	// counted as an unsuccessful healing attempt.
	OutcomeNoCandidate Outcome = "no_candidate"
)

// Candidate is a DOM node considered as a selector replacement, with its
// calculated similarity score in the inclusive range [0,1].
type Candidate struct {
	Selector    fingerprint.Selector
	Fingerprint fingerprint.Fingerprint
	Score       float64
}

// Decision 是 Healer 针对一次定位失败尝试给出的判定结果。
type Decision struct {
	Outcome     Outcome
	Best        *Candidate // 当 Outcome == OutcomeNoCandidate 时为 nil
	Candidates  []Candidate
	NeedsReview bool // OutcomeBelowCap 时为 true
}

// Validate 保护 Decision 的结果组合不变量，避免执行层应用语义矛盾的自愈结果。
func (d Decision) Validate() error {
	switch d.Outcome {
	case OutcomeApplied:
		if d.Best == nil {
			return fmt.Errorf("heal: applied decision requires a best candidate")
		}
		if d.NeedsReview {
			return fmt.Errorf("heal: applied decision cannot require review")
		}
	case OutcomeBelowCap:
		if d.Best == nil {
			return fmt.Errorf("heal: below_cap decision requires a best candidate")
		}
		if !d.NeedsReview {
			return fmt.Errorf("heal: below_cap decision must require review")
		}
	case OutcomeNoCandidate:
		if d.Best != nil {
			return fmt.Errorf("heal: no_candidate decision cannot have a best candidate")
		}
		if d.NeedsReview {
			return fmt.Errorf("heal: no_candidate decision cannot require review")
		}
	default:
		return fmt.Errorf("heal: unknown decision outcome %q", d.Outcome)
	}

	if d.Best != nil {
		if err := d.Best.validate("best candidate"); err != nil {
			return err
		}
	}
	for i := range d.Candidates {
		if err := d.Candidates[i].validate(fmt.Sprintf("candidate %d", i)); err != nil {
			return err
		}
		if i > 0 && candidateLess(d.Candidates[i], d.Candidates[i-1]) {
			return fmt.Errorf("heal: candidates must follow the deterministic score/tie-break order")
		}
	}
	if d.Best != nil {
		if len(d.Candidates) == 0 {
			return fmt.Errorf("heal: best candidate requires a non-empty candidate list")
		}
		if !sameCandidate(*d.Best, d.Candidates[0]) {
			return fmt.Errorf("heal: best candidate must equal the first ordered candidate")
		}
	}
	return nil
}

func (c Candidate) validate(label string) error {
	if err := c.Selector.Validate(); err != nil {
		return fmt.Errorf("heal: %s has invalid selector: %w", label, err)
	}
	if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) || c.Score < 0 || c.Score > 1 {
		return fmt.Errorf("heal: %s score must be finite and within [0,1], got %v", label, c.Score)
	}
	return nil
}

// SnapshotCandidate 把一个实时 DOM 节点的指纹，与基础设施层已经能用来
// 重新定位它的 Selector（例如合成的稳定 CSS/XPath 路径）配对——
// 这样本包里的打分代码就完全不需要直接与浏览器交互。
type SnapshotCandidate struct {
	Fingerprint fingerprint.Fingerprint
	Selector    fingerprint.Selector
}

// DOMSnapshot 是 Healer 用来对候选打分的实时页面最小只读视图。
// 宿主提供具体实现；端口位于此处以确保领域层不依赖任何浏览器库。
type DOMSnapshot interface {
	Candidates(ctx context.Context) ([]SnapshotCandidate, error)
}

// Healer relocates elements whose selectors no longer resolve using
// deterministic fingerprint scoring without external model calls.
type Healer interface {
	Heal(ctx context.Context, target fingerprint.NodeSpec, snapshot DOMSnapshot) (Decision, error)
}
