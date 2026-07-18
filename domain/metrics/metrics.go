// Package metrics defines the read-only quality projection derived from
// committed workspace healing observations. It owns no run lifecycle, healing
// event, verdict, or write repository.
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

// DecisionBand is the policy gate recorded with an immutable healing
// observation. UNKNOWN is valid for no-candidate observations and historical
// records whose policy band cannot be reconstructed safely.
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

// PolicyWeights is the storage-neutral quality identity of the deterministic
// healer weights. Field order is part of the V1 fingerprint contract.
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

// PolicySnapshot contains every setting that can alter one deterministic
// healing decision. Equal snapshots must produce equal fingerprints.
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

// FingerprintPolicy produces a platform-stable identity from the version,
// thresholds, and all ten weights. IEEE-754 bits are encoded in fixed field
// order, avoiding locale and JSON formatting differences.
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
		if value == 0 { // canonicalize negative zero
			value = 0
		}
		binary.BigEndian.PutUint64(buffer[(index+1)*8:], math.Float64bits(value))
	}
	digest := sha256.Sum256(buffer)
	return PolicyFingerprint(hex.EncodeToString(digest[:])), nil
}

// ObservationFact is the minimum immutable workspace fact needed by the
// projection. CandidateHash is used only to distinguish selected candidates
// from no-candidate and unclassifiable historical records.
type ObservationFact struct {
	ObservationID     string
	ObservedAtMS      int64
	Policy            PolicySnapshot
	PolicyFingerprint PolicyFingerprint
	DecisionBand      DecisionBand
	CandidateHash     string
	Succeeded         bool
}

// Query is an inclusive/exclusive UTC millisecond range. Zero policy means all
// policies; a non-zero value filters to one exact frozen policy.
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

// Bucket is grouped by UTC day, frozen policy, and decision band. Counts are
// primary; rates are pure derived values and are never persisted as facts.
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

// Rate is the sole zero-denominator policy for every quality ratio.
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

// Reader is the only metrics domain port. Implementations derive this report
// from the authoritative workspace facts and expose no mutation capability.
type Reader interface {
	QueryHealQuality(context.Context, Query) (Report, error)
}

type bucketKey struct {
	day    string
	policy PolicyFingerprint
	band   DecisionBand
}

// Project validates and classifies facts once, then returns a deterministic
// day/policy/band ordering suitable for adapters and application views.
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
