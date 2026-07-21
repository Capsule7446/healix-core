package heal

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// Weights 是启发式节点距离的权重系数（包含标准维度与
// LabelText/Container 两个 Healix 自己扩展的维度）。Healenium 自身的权重
// 未公开（由 ML 学得）；这里给出的是有文档记录的确定性初始值，在被真正
// 信任之前，必须用真实回归集调参。score() 按加权平均归一化
// （Σw_i·sim_i / Σw_i），所以这些权重只表示彼此的相对重要性，不要求总和为 1。
type Weights struct {
	Tag       float64
	ID        float64
	RoleName  float64
	Class     float64
	Attrs     float64
	Text      float64
	Index     float64
	Neighbor  float64
	LabelText float64
	Container float64
	Framework float64
}

// DefaultWeights 返回一组新的启发式权重，用于确定性打分。
// LabelText/Container 是 Healix 扩展维度。函数返回值避免调用方修改全局默认值。
func DefaultWeights() Weights {
	return Weights{
		Tag:       0.15,
		ID:        0.20,
		RoleName:  0.20,
		Class:     0.10,
		Attrs:     0.10,
		Text:      0.10,
		Index:     0.05,
		Neighbor:  0.10,
		LabelText: 0.15,
		Container: 0.10,
		Framework: 0}
}

// Validate 要求权重是非负有限数，且至少启用一个打分维度。
func (w Weights) Validate() error {
	weights := []struct {
		name  string
		value float64
	}{
		{"tag", w.Tag}, {"id", w.ID}, {"role_name", w.RoleName},
		{"class", w.Class}, {"attrs", w.Attrs}, {"text", w.Text},
		{"index", w.Index}, {"neighbor", w.Neighbor}, {"label_text", w.LabelText},
		{"container", w.Container}, {"framework", w.Framework},
	}
	total := 0.0
	for _, weight := range weights {
		if math.IsNaN(weight.value) || math.IsInf(weight.value, 0) || weight.value < 0 {
			return fmt.Errorf("heal: %s weight must be finite and non-negative, got %v", weight.name, weight.value)
		}
		total += weight.value
	}
	if math.IsInf(total, 0) {
		return fmt.Errorf("heal: sum of weights must be finite")
	}
	if total == 0 {
		return fmt.Errorf("heal: at least one weight must be positive")
	}
	return nil
}

type dimension struct {
	weight     float64
	similarity float64
	applicable bool
}

type preparedTargetScorer struct {
	weights       Weights
	target        fingerprint.Fingerprint
	classes       map[string]struct{}
	keyAttributes []string
	textRunes     []rune
	labelRunes    []rune
	totalWeight   float64
}

func prepareTargetScorer(weights Weights, target fingerprint.Fingerprint) preparedTargetScorer {
	prepared := preparedTargetScorer{weights: weights, target: target}
	if target.Tag != "" {
		prepared.totalWeight += weights.Tag
	}
	if target.Attributes["id"] != "" {
		prepared.totalWeight += weights.ID
	}
	if target.ARIA.Role != "" || target.ARIA.Name != "" {
		prepared.totalWeight += weights.RoleName
	}
	prepared.classes = classSet(target.Attributes["class"])
	if len(prepared.classes) > 0 {
		prepared.totalWeight += weights.Class
	}
	for _, key := range keyAttrsFor(target.Attributes) {
		if target.Attributes[key] != "" {
			prepared.keyAttributes = append(prepared.keyAttributes, key)
		}
	}
	if len(prepared.keyAttributes) > 0 {
		prepared.totalWeight += weights.Attrs
	}
	if target.Text != "" {
		prepared.textRunes = []rune(target.Text)
		prepared.totalWeight += weights.Text
	}
	prepared.totalWeight += weights.Index
	if hasNeighborSignal(target.Neighbors) {
		prepared.totalWeight += weights.Neighbor
	}
	if target.LabelText != "" {
		prepared.labelRunes = []rune(target.LabelText)
		prepared.totalWeight += weights.LabelText
	}
	if target.FormID != "" {
		prepared.totalWeight += weights.Container
	}
	if len(target.Framework) > 0 {
		prepared.totalWeight += weights.Framework
	}
	return prepared
}

func (s preparedTargetScorer) score(candidate fingerprint.Fingerprint) float64 {
	if s.totalWeight == 0 {
		return 0
	}
	var sum float64
	if s.target.Tag != "" {
		sum += s.weights.Tag * simEqualNonEmpty(s.target.Tag, candidate.Tag)
	}
	if targetID := s.target.Attributes["id"]; targetID != "" {
		sum += s.weights.ID * simEqualNonEmpty(targetID, candidate.Attributes["id"])
	}
	if s.target.ARIA.Role != "" || s.target.ARIA.Name != "" {
		sum += s.weights.RoleName * simRoleName(s.target.ARIA, candidate.ARIA)
	}
	if len(s.classes) > 0 {
		sum += s.weights.Class * simJaccard(s.classes, classSet(candidate.Attributes["class"]))
	}
	if len(s.keyAttributes) > 0 {
		sum += s.weights.Attrs * s.simKeyAttributes(candidate.Attributes)
	}
	if s.target.Text != "" {
		sum += s.weights.Text * simTextPrepared(s.target.Text, s.textRunes, candidate.Text)
	}
	sum += s.weights.Index * simIndex(s.target.SiblingIndex, candidate.SiblingIndex)
	if hasNeighborSignal(s.target.Neighbors) {
		sum += s.weights.Neighbor * simNeighbor(s.target.Neighbors, candidate.Neighbors)
	}
	if s.target.LabelText != "" {
		sum += s.weights.LabelText * simTextPrepared(s.target.LabelText, s.labelRunes, candidate.LabelText)
	}
	if s.target.FormID != "" {
		sum += s.weights.Container * simEqualNonEmpty(s.target.FormID, candidate.FormID)
	}
	if len(s.target.Framework) > 0 {
		sum += s.weights.Framework * frameworkSimilarity(s.target.Framework, candidate.Framework)
	}
	return sum / s.totalWeight
}

func (s preparedTargetScorer) simKeyAttributes(candidate map[string]string) float64 {
	matched := 0
	for _, key := range s.keyAttributes {
		if candidate[key] == s.target.Attributes[key] {
			matched++
		}
	}
	return float64(matched) / float64(len(s.keyAttributes))
}

// score 计算 score(candidate) = Σ w_i·sim_i / Σ w_i，但只在
// target 确实带有对应信号的维度上做加权平均的重新归一化（例如 target 本身
// 就没有 "id" 属性时会排除该维度）。如果不这样处理，一个本来就没有
// id/class/testid 的元素，即使匹配到它的精确克隆，也永远达不到
// applied_cap——因为缺失的维度会被当作彻底不匹配计分，而不是被排除在外。
func score(w Weights, target, candidate fingerprint.Fingerprint) float64 {
	return prepareTargetScorer(w, target).score(candidate)
}

func hasKeyAttrs(attrs map[string]string) bool {
	for _, k := range keyAttrsFor(attrs) {
		if attrs[k] != "" {
			return true
		}
	}
	return false
}

func hasNeighborSignal(n fingerprint.Neighbors) bool {
	return n.Prev != "" || n.Next != "" || n.ParentTag != ""
}

func simEqualNonEmpty(a, b string) float64 {
	if a == "" {
		return 0
	}
	if a == b {
		return 1
	}
	return 0
}

func simRoleName(a, b fingerprint.ARIA) float64 {
	if a.Role == "" && a.Name == "" {
		return 0
	}
	if a.Role == b.Role && a.Name == b.Name {
		return 1
	}
	return 0
}

func classSet(class string) map[string]struct{} {
	fields := strings.Fields(class)
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}

// simJaccard 是 |A∩B| / |A∪B|，当两个集合都为空（无信号）时返回 0。
func simJaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// keyAttrs 是 Playwright/Healenium 视为稳定身份信号的固定属性，id/class 之外
// 的部分（id/class 单独计分）。data-testid 只是最常见的一个约定，实际项目里
// 还有 data-qa/data-cy/data-test 等各测试框架自己的写法，因此不满足于固定
// 清单——keyAttrsFor 会再把 target 上出现的所有 data-* 属性动态并进来。
var keyAttrs = []string{"data-testid", "name", "type", "aria-label", "placeholder", "href"}

// keyAttrsFor 返回本次比较要检查的 key 集合：固定清单，加上 target 自己
// 带有的、固定清单里没覆盖到的任意 data-* 属性。
func keyAttrsFor(target map[string]string) []string {
	keys := append([]string(nil), keyAttrs...)
	for k := range target {
		if strings.HasPrefix(k, "data-") && k != "data-testid" {
			keys = append(keys, k)
		}
	}
	return keys
}

// simKeyAttrs 是 target 上存在的 keyAttrsFor 中，candidate 也一致匹配的比例。
func simKeyAttrs(target, candidate map[string]string) float64 {
	considered, matched := 0, 0
	for _, k := range keyAttrsFor(target) {
		v, ok := target[k]
		if !ok || v == "" {
			continue
		}
		considered++
		if candidate[k] == v {
			matched++
		}
	}
	if considered == 0 {
		return 0
	}
	return float64(matched) / float64(considered)
}

// simText 是 1 减去归一化的 Levenshtein 距离；两段文本都为空（无信号）时
// 返回 0。
func simText(a, b string) float64 {
	if a == "" && b == "" {
		return 0
	}
	maxLen := utf8.RuneCountInString(a)
	if n := utf8.RuneCountInString(b); n > maxLen {
		maxLen = n
	}
	if maxLen == 0 {
		return 1
	}
	dist := levenshtein(a, b)
	sim := 1 - float64(dist)/float64(maxLen)
	if sim < 0 {
		sim = 0
	}
	return sim
}

func simTextPrepared(target string, targetRunes []rune, candidate string) float64 {
	if target == "" && candidate == "" {
		return 0
	}
	candidateRunes := []rune(candidate)
	maxLen := len(targetRunes)
	if len(candidateRunes) > maxLen {
		maxLen = len(candidateRunes)
	}
	if maxLen == 0 {
		return 1
	}
	dist := levenshteinRunes(targetRunes, candidateRunes)
	sim := 1 - float64(dist)/float64(maxLen)
	if sim < 0 {
		sim = 0
	}
	return sim
}

func levenshtein(a, b string) int {
	return levenshteinRunes([]rune(a), []rune(b))
}

func levenshteinRunes(ra, rb []rune) int {
	n, m := len(ra), len(rb)
	prev := make([]int, m+1)
	curr := make([]int, m+1)
	for j := 0; j <= m; j++ {
		prev[j] = j
	}
	for i := 1; i <= n; i++ {
		curr[0] = i
		for j := 1; j <= m; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[m]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// simIndex 是 1 - |Δsibling_index| / max(target, candidate, 1)。
func simIndex(target, candidate int) float64 {
	maxIdx := target
	if candidate > maxIdx {
		maxIdx = candidate
	}
	if maxIdx == 0 {
		return 1
	}
	diff := target - candidate
	if diff < 0 {
		diff = -diff
	}
	sim := 1 - float64(diff)/float64(maxIdx)
	if sim < 0 {
		sim = 0
	}
	return sim
}

// simNeighbor 是 {prev, next, parent} 标签中匹配的比例，只统计 target 确实
// 有取值的位置。
func simNeighbor(target, candidate fingerprint.Neighbors) float64 {
	considered, matched := 0, 0
	pairs := [][2]string{
		{target.Prev, candidate.Prev},
		{target.Next, candidate.Next},
		{target.ParentTag, candidate.ParentTag},
	}
	for _, p := range pairs {
		if p[0] == "" {
			continue
		}
		considered++
		if p[0] == p[1] {
			matched++
		}
	}
	if considered == 0 {
		return 0
	}
	return float64(matched) / float64(considered)
}
