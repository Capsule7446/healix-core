package heal

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

var benchmarkScoreResult float64
var benchmarkHealDecision Decision

func TestScoreMatchesLegacyOracle(t *testing.T) {
	random := rand.New(rand.NewSource(2))
	for iteration := 0; iteration < 1_000; iteration++ {
		target := randomScoringFingerprint(random)
		candidate := randomScoringFingerprint(random)
		weights := randomScoringWeights(random)
		got := score(weights, target, candidate)
		want := legacyScore(weights, target, candidate)
		if got != want {
			t.Fatalf("iteration %d score = %.17g, want %.17g", iteration, got, want)
		}
	}
}

func randomScoringWeights(random *rand.Rand) Weights {
	return Weights{
		Tag: float64(random.Intn(11)) / 10, ID: float64(random.Intn(11)) / 10,
		RoleName: float64(random.Intn(11)) / 10, Class: float64(random.Intn(11)) / 10,
		Attrs: float64(random.Intn(11)) / 10, Text: float64(random.Intn(11)) / 10,
		Index: float64(random.Intn(11)) / 10, Neighbor: float64(random.Intn(11)) / 10,
		LabelText: float64(random.Intn(11)) / 10, Container: float64(random.Intn(11)) / 10,
	}
}

func TestPreparedTargetScorerMatchesLegacyDecision(t *testing.T) {
	target := fingerprint.NodeSpec{ID: "checkout.submit", Fingerprint: benchmarkScoringFingerprint(0)}
	candidates := make([]SnapshotCandidate, 64)
	for index := range candidates {
		candidates[index] = SnapshotCandidate{
			Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: fmt.Sprintf("#candidate-%d", index)},
			Fingerprint: benchmarkScoringFingerprint(index + 1),
		}
	}
	got, err := NewDefaultHealer().Heal(context.Background(), target, fakeSnapshot{candidates: candidates})
	if err != nil {
		t.Fatal(err)
	}
	want, err := legacyHealDecision(NewDefaultHealer(), target, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepared decision differs from legacy decision\ngot:  %#v\nwant: %#v", got, want)
	}
}

func BenchmarkScoreCandidates(b *testing.B) {
	for _, count := range []int{1, 32, 256} {
		b.Run(fmt.Sprintf("candidates=%d", count), func(b *testing.B) {
			target := benchmarkScoringFingerprint(0)
			candidates := make([]fingerprint.Fingerprint, count)
			for index := range candidates {
				candidates[index] = benchmarkScoringFingerprint(index + 1)
			}
			weights := DefaultWeights()
			b.ReportAllocs()
			for b.Loop() {
				total := 0.0
				for _, candidate := range candidates {
					total += score(weights, target, candidate)
				}
				benchmarkScoreResult = total
			}
		})
	}
}

func BenchmarkDefaultHealerHealScoring(b *testing.B) {
	for _, count := range []int{1, 32, 256} {
		b.Run(fmt.Sprintf("candidates=%d", count), func(b *testing.B) {
			target := fingerprint.NodeSpec{ID: "checkout.submit", Fingerprint: benchmarkScoringFingerprint(0)}
			candidates := make([]SnapshotCandidate, count)
			for index := range candidates {
				candidates[index] = SnapshotCandidate{
					Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: fmt.Sprintf("#candidate-%d", index)},
					Fingerprint: benchmarkScoringFingerprint(index + 1),
				}
			}
			healer := NewDefaultHealer()
			snapshot := fakeSnapshot{candidates: candidates}
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				decision, err := healer.Heal(ctx, target, snapshot)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkHealDecision = decision
			}
		})
	}
}

func legacyScore(weights Weights, target, candidate fingerprint.Fingerprint) float64 {
	dimensions := []dimension{
		{weights.Tag, simEqualNonEmpty(target.Tag, candidate.Tag), target.Tag != ""},
		{weights.ID, simEqualNonEmpty(target.Attributes["id"], candidate.Attributes["id"]), target.Attributes["id"] != ""},
		{weights.RoleName, simRoleName(target.ARIA, candidate.ARIA), target.ARIA.Role != "" || target.ARIA.Name != ""},
		{weights.Class, simJaccard(classSet(target.Attributes["class"]), classSet(candidate.Attributes["class"])), len(classSet(target.Attributes["class"])) > 0},
		{weights.Attrs, simKeyAttrs(target.Attributes, candidate.Attributes), hasKeyAttrs(target.Attributes)},
		{weights.Text, simText(target.Text, candidate.Text), target.Text != ""},
		{weights.Index, simIndex(target.SiblingIndex, candidate.SiblingIndex), true},
		{weights.Neighbor, simNeighbor(target.Neighbors, candidate.Neighbors), hasNeighborSignal(target.Neighbors)},
		{weights.LabelText, simText(target.LabelText, candidate.LabelText), target.LabelText != ""},
		{weights.Container, simEqualNonEmpty(target.FormID, candidate.FormID), target.FormID != ""},
	}
	var sum, total float64
	for _, current := range dimensions {
		if !current.applicable {
			continue
		}
		sum += current.weight * current.similarity
		total += current.weight
	}
	if total == 0 {
		return 0
	}
	return sum / total
}

func legacyHealDecision(healer *DefaultHealer, target fingerprint.NodeSpec, all []SnapshotCandidate) (Decision, error) {
	narrowed := narrowByPathLCS(target.Fingerprint.Path, all)
	scored := make([]Candidate, 0, len(narrowed))
	for _, candidate := range narrowed {
		scored = append(scored, Candidate{Selector: candidate.Selector, Fingerprint: candidate.Fingerprint,
			Score: legacyScore(healer.Weights, target.Fingerprint, candidate.Fingerprint)})
	}
	sortCandidates(scored)
	decision := Decision{Candidates: scored}
	if len(scored) == 0 {
		decision.Outcome = OutcomeNoCandidate
		return validateDecision(decision)
	}
	best := scored[0]
	switch {
	case best.Score >= healer.Thresholds.AppliedCap:
		decision.Outcome = OutcomeApplied
		decision.Best = &best
	case best.Score >= healer.Thresholds.ReviewCap:
		decision.Outcome = OutcomeBelowCap
		decision.NeedsReview = true
		decision.Best = &best
	default:
		decision.Outcome = OutcomeNoCandidate
	}
	return validateDecision(decision)
}

func benchmarkScoringFingerprint(offset int) fingerprint.Fingerprint {
	return fingerprint.Fingerprint{
		Tag: "button",
		Attributes: map[string]string{
			"id": "checkout", "class": "button primary large checkout", "data-testid": "checkout-submit",
			"data-qa": "checkout-action", "data-cy": fmt.Sprintf("checkout-%d", offset%3), "name": "submit",
			"type": "submit", "aria-label": "提交订单", "placeholder": "", "href": "",
		},
		Text:         fmt.Sprintf("确认并提交第 %d 个订单，配送到台北市中山区", offset%7),
		ARIA:         fingerprint.ARIA{Role: "button", Name: "提交订单"},
		SiblingIndex: offset % 5,
		Neighbors:    fingerprint.Neighbors{Prev: "input", Next: "a", ParentTag: "form"},
		LabelText:    fmt.Sprintf("订单确认 %d", offset%4),
		FormID:       "checkout-form",
		Path:         []string{"html", "body", "main", "form", "button"},
	}
}

func randomScoringFingerprint(random *rand.Rand) fingerprint.Fingerprint {
	values := []string{"", "button", "input", "checkout", "primary secondary", "提交订单", "data-value"}
	attributes := map[string]string{}
	for _, key := range []string{"id", "class", "data-testid", "data-qa", "data-cy", "name", "type", "aria-label", "placeholder", "href"} {
		if random.Intn(3) != 0 {
			attributes[key] = values[random.Intn(len(values))]
		}
	}
	return fingerprint.Fingerprint{
		Tag:          values[random.Intn(len(values))],
		Attributes:   attributes,
		Text:         values[random.Intn(len(values))],
		ARIA:         fingerprint.ARIA{Role: values[random.Intn(len(values))], Name: values[random.Intn(len(values))]},
		SiblingIndex: random.Intn(8),
		Neighbors: fingerprint.Neighbors{Prev: values[random.Intn(len(values))], Next: values[random.Intn(len(values))],
			ParentTag: values[random.Intn(len(values))]},
		LabelText: values[random.Intn(len(values))],
		FormID:    values[random.Intn(len(values))],
	}
}
