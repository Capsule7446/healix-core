// 包指标定义了从承诺的工作空间修复观察中得出的只读质量预测。它不拥有运行生命周期、修复事件、判决或写入存储库。
package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// DecisionBand 是用不可变的愈合​​观察记录的策略门。 UNKNOWN 对于无候选观测值和无法安全重建策略区间的历史记录有效。
type DecisionBand string

const (
	DecisionBandUnknown  DecisionBand = "UNKNOWN"
	DecisionBandApplied  DecisionBand = "APPLIED"
	DecisionBandBelowCap DecisionBand = "BELOW_CAP"
)

func (band DecisionBand) Validate() error {
	switch band {
	case DecisionBandUnknown, DecisionBandApplied, DecisionBandBelowCap:
		return nil
	default:
		return fmt.Errorf("unknown heal quality decision band %q", band)
	}
}

// PolicyWeights 是确定性治疗者权重的存储中立质量标识。现场订单是 V1 指纹合约的一部分。
type PolicyWeights struct {
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
}

// PolicySnapshot 包含可以改变确定性修复决策的每一项设置。相同的快照必须产生相同的指纹。
type PolicySnapshot struct {
	Version    int
	ReviewCap  float64
	AppliedCap float64
	Weights    PolicyWeights
}

type PolicyFingerprint string

func (fingerprint PolicyFingerprint) Validate() error {
	value := string(fingerprint)
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("policy fingerprint must contain %d hex characters", sha256.Size*2)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("policy fingerprint is not hexadecimal: %w", err)
	}
	return nil
}

// FingerprintPolicy 根据版本、阈值和所有十个权重生成平台稳定的身份。 IEEE-754 位以固定字段顺序进行编码，避免了区域设置和 JSON 格式差异。
func FingerprintPolicy(policy PolicySnapshot) (PolicyFingerprint, error) {
	if policy.Version <= 0 {
		return "", errors.New("healer policy fingerprint requires a positive version")
	}
	values := []float64{policy.ReviewCap, policy.AppliedCap, policy.Weights.Tag, policy.Weights.ID,
		policy.Weights.RoleName, policy.Weights.Class, policy.Weights.Attrs, policy.Weights.Text,
		policy.Weights.Index, policy.Weights.Neighbor, policy.Weights.LabelText, policy.Weights.Container}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", errors.New("healer policy fingerprint requires finite values")
		}
	}
	buffer := make([]byte, 8*(len(values)+1))
	binary.BigEndian.PutUint64(buffer, uint64(policy.Version))
	for index, value := range values {
		if value == 0 { // 规范化负零
			value = 0
		}
		binary.BigEndian.PutUint64(buffer[(index+1)*8:], math.Float64bits(value))
	}
	digest := sha256.Sum256(buffer)
	return PolicyFingerprint(hex.EncodeToString(digest[:])), nil
}

// ObservationFact 是投影所需的最小不可变工作空间事实。 CandidateHash 仅用于区分选定的候选者与无候选者和不可分类的历史记录。
type ObservationFact struct {
	ObservationID     string
	ObservedAtMS      int64
	Policy            PolicySnapshot
	PolicyFingerprint PolicyFingerprint
	DecisionBand      DecisionBand
	CandidateHash     string
	Succeeded         bool
}

// Query：查询是包含/不包含 UTC 毫秒范围。零政策意味着所有政策；非零值过滤到一个精确的冻结策略。
type Query struct {
	FromMS            int64
	ThroughMS         int64
	PolicyFingerprint PolicyFingerprint
}

func (query Query) Validate() error {
	if query.FromMS < 0 || query.ThroughMS <= query.FromMS {
		return fmt.Errorf("heal quality range must be non-negative and non-empty")
	}
	if query.PolicyFingerprint != "" {
		return query.PolicyFingerprint.Validate()
	}
	return nil
}

// Bucket：存储桶按 UTC 日、冻结策略和决策范围分组。计数是主要的；利率是纯粹的派生值，永远不会作为事实存在。
type Bucket struct {
	Day               string
	Policy            PolicySnapshot
	PolicyFingerprint PolicyFingerprint
	DecisionBand      DecisionBand
	AttemptCount      int
	CandidateSelected int
	AppliedSuccess    int
	AppliedFailure    int
	NoCandidate       int
	UnknownLegacy     int
}

func (bucket Bucket) ClassifiedAttempts() int {
	classified := bucket.AttemptCount - bucket.UnknownLegacy
	if classified < 0 {
		return 0
	}
	return classified
}

func (bucket Bucket) CandidateSelectionRate() float64 {
	return Rate(bucket.CandidateSelected, bucket.ClassifiedAttempts())
}

func (bucket Bucket) AppliedSuccessRate() float64 {
	return Rate(bucket.AppliedSuccess, bucket.CandidateSelected)
}

func (bucket Bucket) AppliedFailureRate() float64 {
	return Rate(bucket.AppliedFailure, bucket.CandidateSelected)
}

func (bucket Bucket) NoCandidateRate() float64 {
	return Rate(bucket.NoCandidate, bucket.ClassifiedAttempts())
}

// Rate：比率是每个质量比率的唯一零分母政策。
func Rate(numerator, denominator int) float64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

type Report struct {
	FromMS    int64
	ThroughMS int64
	Buckets   []Bucket
}

// Reader 是唯一的度量域端口。实现从权威的工作区事实中得出此报告，并且不公开任何突变功能。
type Reader interface {
	QueryHealQuality(context.Context, Query) (Report, error)
}

type bucketKey struct {
	day    string
	policy PolicyFingerprint
	band   DecisionBand
}

// Project：项目对事实进行一次验证和分类，然后返回适合适配器和应用程序视图的确定性日期/策略/频带排序。
func Project(query Query, facts []ObservationFact) (Report, error) {
	if err := query.Validate(); err != nil {
		return Report{}, err
	}
	buckets := make(map[bucketKey]Bucket)
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.ObservationID) == "" {
			return Report{}, errors.New("heal quality fact requires an observation id")
		}
		if _, duplicate := seen[fact.ObservationID]; duplicate {
			return Report{}, fmt.Errorf("duplicate heal quality observation %q", fact.ObservationID)
		}
		seen[fact.ObservationID] = struct{}{}
		if fact.ObservedAtMS < query.FromMS || fact.ObservedAtMS >= query.ThroughMS {
			continue
		}
		if err := fact.PolicyFingerprint.Validate(); err != nil {
			return Report{}, fmt.Errorf("observation %s: %w", fact.ObservationID, err)
		}
		expectedFingerprint, err := FingerprintPolicy(fact.Policy)
		if err != nil {
			return Report{}, fmt.Errorf("observation %s policy: %w", fact.ObservationID, err)
		}
		if expectedFingerprint != fact.PolicyFingerprint {
			return Report{}, fmt.Errorf("observation %s policy fingerprint does not match its snapshot", fact.ObservationID)
		}
		if query.PolicyFingerprint != "" && fact.PolicyFingerprint != query.PolicyFingerprint {
			continue
		}
		if err := fact.DecisionBand.Validate(); err != nil {
			return Report{}, fmt.Errorf("observation %s: %w", fact.ObservationID, err)
		}
		day := time.UnixMilli(fact.ObservedAtMS).UTC().Format(time.DateOnly)
		key := bucketKey{day: day, policy: fact.PolicyFingerprint, band: fact.DecisionBand}
		bucket := buckets[key]
		bucket.Day, bucket.Policy, bucket.PolicyFingerprint, bucket.DecisionBand = key.day, fact.Policy, key.policy, key.band
		bucket.AttemptCount++
		hasCandidate := strings.TrimSpace(fact.CandidateHash) != ""
		switch {
		case hasCandidate && (fact.DecisionBand == DecisionBandApplied || fact.DecisionBand == DecisionBandBelowCap):
			bucket.CandidateSelected++
			if fact.Succeeded {
				bucket.AppliedSuccess++
			} else {
				bucket.AppliedFailure++
			}
		case !hasCandidate && fact.DecisionBand == DecisionBandUnknown:
			bucket.NoCandidate++
		default:
			bucket.UnknownLegacy++
		}
		buckets[key] = bucket
	}

	report := Report{FromMS: query.FromMS, ThroughMS: query.ThroughMS,
		Buckets: make([]Bucket, 0, len(buckets))}
	for _, bucket := range buckets {
		report.Buckets = append(report.Buckets, bucket)
	}
	sort.Slice(report.Buckets, func(i, j int) bool {
		left, right := report.Buckets[i], report.Buckets[j]
		if left.Day != right.Day {
			return left.Day > right.Day
		}
		if left.PolicyFingerprint != right.PolicyFingerprint {
			return left.PolicyFingerprint < right.PolicyFingerprint
		}
		return left.DecisionBand < right.DecisionBand
	})
	return report, nil
}
