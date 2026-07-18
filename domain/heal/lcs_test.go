package heal

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

var (
	benchmarkLCSLengthResult int
	benchmarkLCSCandidates   []SnapshotCandidate
)

func TestLCSLengthMatchesMatrixOracle(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
	}{
		{name: "both-empty"},
		{name: "left-empty", b: []string{"html", "body"}},
		{name: "right-empty", a: []string{"html", "body"}},
		{name: "identical", a: []string{"html", "body", "main"}, b: []string{"html", "body", "main"}},
		{name: "disjoint", a: []string{"a", "b"}, b: []string{"c", "d"}},
		{name: "duplicates", a: []string{"div", "div", "span"}, b: []string{"div", "span", "div"}},
		{name: "asymmetric", a: []string{"html", "body", "main", "form", "button"}, b: []string{"body", "button"}},
		{name: "unicode", a: []string{"页面", "表单", "按钮"}, b: []string{"页面", "容器", "按钮"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, want := lcsLength(test.a, test.b), matrixLCSLength(test.a, test.b); got != want {
				t.Fatalf("lcsLength(%v, %v) = %d, want %d", test.a, test.b, got, want)
			}
		})
	}

	random := rand.New(rand.NewSource(1))
	alphabet := []string{"html", "body", "div", "span", "form", "button", "页面", "按钮"}
	for iteration := 0; iteration < 1_000; iteration++ {
		a := randomPath(random, alphabet, random.Intn(65))
		b := randomPath(random, alphabet, random.Intn(65))
		if got, want := lcsLength(a, b), matrixLCSLength(a, b); got != want {
			t.Fatalf("iteration %d lcsLength(%v, %v) = %d, want %d", iteration, a, b, got, want)
		}
	}
}

func TestNarrowByPathLCSMatchesMatrixOracle(t *testing.T) {
	target := []string{"html", "body", "main", "form", "button"}
	candidates := []SnapshotCandidate{
		{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#one"}, Fingerprint: fingerprint.Fingerprint{Path: []string{"html", "body", "main", "button"}}},
		{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#two"}, Fingerprint: fingerprint.Fingerprint{Path: []string{"html", "body", "aside", "button"}}},
		{Selector: fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: "#three"}, Fingerprint: fingerprint.Fingerprint{Path: []string{"html", "body", "main", "form", "button"}}},
	}
	if got, want := narrowByPathLCS(target, candidates), matrixNarrowByPathLCS(target, candidates); !reflect.DeepEqual(got, want) {
		t.Fatalf("narrowed = %#v, want %#v", got, want)
	}
}

func BenchmarkLCSLength(b *testing.B) {
	tests := []struct {
		left, right int
		shape       string
	}{
		{left: 5, right: 5, shape: "partial"},
		{left: 16, right: 16, shape: "partial"},
		{left: 32, right: 32, shape: "partial"},
		{left: 64, right: 64, shape: "partial"},
		{left: 8, right: 64, shape: "asymmetric"},
		{left: 64, right: 64, shape: "identical"},
		{left: 64, right: 64, shape: "disjoint"},
	}
	for _, test := range tests {
		b.Run(fmt.Sprintf("%dx%d/%s", test.left, test.right, test.shape), func(b *testing.B) {
			left := benchmarkPath(test.left, 0)
			right := benchmarkPath(test.right, 3)
			switch test.shape {
			case "identical":
				right = append([]string(nil), left...)
			case "disjoint":
				right = benchmarkDisjointPath(test.right)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkLCSLengthResult = lcsLength(left, right)
			}
		})
	}
}

func BenchmarkNarrowByPathLCS(b *testing.B) {
	tests := []struct {
		candidates int
		depth      int
	}{
		{candidates: 32, depth: 10},
		{candidates: 256, depth: 10},
		{candidates: 32, depth: 40},
		{candidates: 256, depth: 40},
	}
	for _, test := range tests {
		b.Run(fmt.Sprintf("candidates=%d/depth=%d", test.candidates, test.depth), func(b *testing.B) {
			target := benchmarkPath(test.depth, 0)
			candidates := make([]SnapshotCandidate, test.candidates)
			for index := range candidates {
				candidates[index] = SnapshotCandidate{
					Selector:    fingerprint.Selector{Type: fingerprint.SelectorCSS, Value: fmt.Sprintf("#candidate-%d", index)},
					Fingerprint: fingerprint.Fingerprint{Path: benchmarkPath(test.depth, index%7)},
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkLCSCandidates = narrowByPathLCS(target, candidates)
			}
		})
	}
}

func matrixLCSLength(a, b []string) int {
	dp := make([][]int, len(a)+1)
	for index := range dp {
		dp[index] = make([]int, len(b)+1)
	}
	for left := 1; left <= len(a); left++ {
		for right := 1; right <= len(b); right++ {
			switch {
			case a[left-1] == b[right-1]:
				dp[left][right] = dp[left-1][right-1] + 1
			case dp[left-1][right] >= dp[left][right-1]:
				dp[left][right] = dp[left-1][right]
			default:
				dp[left][right] = dp[left][right-1]
			}
		}
	}
	return dp[len(a)][len(b)]
}

func matrixNarrowByPathLCS(target []string, candidates []SnapshotCandidate) []SnapshotCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	distances := make([]int, len(candidates))
	maximum := 0
	for index, candidate := range candidates {
		distances[index] = matrixLCSLength(target, candidate.Fingerprint.Path)
		maximum = max(maximum, distances[index])
	}
	result := make([]SnapshotCandidate, 0, len(candidates))
	for index, candidate := range candidates {
		if distances[index] >= maximum {
			result = append(result, candidate)
		}
	}
	return result
}

func randomPath(random *rand.Rand, alphabet []string, length int) []string {
	result := make([]string, length)
	for index := range result {
		result[index] = alphabet[random.Intn(len(alphabet))]
	}
	return result
}

func benchmarkPath(length, offset int) []string {
	alphabet := []string{"html", "body", "div", "main", "section", "form", "label", "button"}
	result := make([]string, length)
	for index := range result {
		result[index] = alphabet[(index+offset)%len(alphabet)]
	}
	return result
}

func benchmarkDisjointPath(length int) []string {
	result := make([]string, length)
	for index := range result {
		result[index] = fmt.Sprintf("node-%d", index)
	}
	return result
}
