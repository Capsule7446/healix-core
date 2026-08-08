package heal

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// Weights 是启发式节点距离的非负权重系数；评分按启用信号计算加权平均，权重总和无需为 1。
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

// dimension 保存一个评分维度的权重、相似度和是否适用状态。
type dimension struct {
	weight     float64
	similarity float64
	applicable bool
}

// preparedTargetScorer 保存针对目标指纹预计算的评分输入和总权重。
type preparedTargetScorer struct {
	weights       Weights
	target        fingerprint.Fingerprint
	classes       map[string]struct{}
	keyAttributes []string
	textRunes     []rune
	labelRunes    []rune
	totalWeight   float64
}

// prepareTargetScorer 预计算目标指纹的启用维度、属性集合和文本 rune，供多个候选复用。
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

// score 使用预计算目标对候选指纹计算归一化加权相似度。
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

// simKeyAttributes 计算目标启用属性键中与候选值相等的比例。
func (s preparedTargetScorer) simKeyAttributes(candidate map[string]string) float64 {
	matched := 0
	for _, key := range s.keyAttributes {
		if candidate[key] == s.target.Attributes[key] {
			matched++
		}
	}
	return float64(matched) / float64(len(s.keyAttributes))
}

// score 计算 score(candidate) = Σ w_i·sim_i / Σ w_i，并仅纳入目标实际提供信号的维度，
// 使缺失的目标属性不会被当作不匹配而降低精确克隆的分数。
func score(w Weights, target, candidate fingerprint.Fingerprint) float64 {
	return prepareTargetScorer(w, target).score(candidate)
}

// hasKeyAttrs 判断属性映射是否包含至少一个有值的评分键。
func hasKeyAttrs(attrs map[string]string) bool {
	for _, k := range keyAttrsFor(attrs) {
		if attrs[k] != "" {
			return true
		}
	}
	return false
}

// hasNeighborSignal 判断邻接节点结构是否提供至少一个信号。
func hasNeighborSignal(n fingerprint.Neighbors) bool {
	return n.Prev != "" || n.Next != "" || n.ParentTag != ""
}

// simEqualNonEmpty 对非空且相等的字符串返回 1，其余情况返回 0。
func simEqualNonEmpty(a, b string) float64 {
	if a == "" {
		return 0
	}
	if a == b {
		return 1
	}
	return 0
}

// simRoleName 比较 ARIA role 和 name 的完整组合相似度。
func simRoleName(a, b fingerprint.ARIA) float64 {
	if a.Role == "" && a.Name == "" {
		return 0
	}
	if a.Role == b.Role && a.Name == b.Name {
		return 1
	}
	return 0
}

// classSet 将空白分隔的 class 字符串转换为去重集合。
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

// keyAttrs 是 id/class 之外用于稳定身份评分的固定属性键。
var keyAttrs = []string{"data-testid", "name", "type", "aria-label", "placeholder", "href"}

// keyAttrsFor 返回固定属性键，并追加目标中出现的其他 data-* 属性。
func keyAttrsFor(target map[string]string) []string {
	keys := append([]string(nil), keyAttrs...)
	for k := range target {
		if strings.HasPrefix(k, "data-") && k != "data-testid" {
			keys = append(keys, k)
		}
	}
	return keys
}

// simKeyAttrs 计算目标存在且候选值一致的评分键比例。
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

// simText 返回 1 减归一化 Levenshtein 距离；两段文本都为空时返回 0。
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

// simTextPrepared 使用预分解的目标 rune 计算与候选文本的归一化相似度。
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

// levenshtein 计算两个字符串按 Unicode rune 的编辑距离。
func levenshtein(a, b string) int {
	return levenshteinRunes([]rune(a), []rune(b))
}

// levenshteinRunes 使用双行动态规划缓冲区计算两个 rune 切片的编辑距离。
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

// min3 返回三个整数中的最小值。
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
	if target < 0 || candidate < 0 {
		return 0
	}
	maxIdx := max(target, candidate)
	if maxIdx == 0 {
		return 1
	}
	minIdx := min(target, candidate)
	diff := maxIdx - minIdx
	sim := 1 - float64(diff)/float64(maxIdx)
	if sim < 0 {
		return 0
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
