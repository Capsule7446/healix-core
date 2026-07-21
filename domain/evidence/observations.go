package evidence

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type DecisionBand string

const (
	DecisionUnknown  DecisionBand = "UNKNOWN"
	DecisionApplied  DecisionBand = "APPLIED"
	DecisionBelowCap DecisionBand = "BELOW_CAP"
)

func ValidateDecisionBand(candidateHash string, band DecisionBand) error {
	hasCandidate := strings.TrimSpace(candidateHash) != ""
	if !hasCandidate && band != DecisionUnknown {
		return errors.New("observation without a candidate must use UNKNOWN decision band")
	}
	if hasCandidate && band != DecisionApplied && band != DecisionBelowCap {
		return errors.New("observation with a candidate requires APPLIED or BELOW_CAP decision band")
	}
	return nil
}

func ValidateConfidence(confidence float64) error {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence must be finite and between 0 and 1")
	}
	return nil
}

type HealObservation struct {
	ID                string
	RunID             string
	ExecutionID       string
	StepExecutionID   string
	NodeID            string
	BaseNodeVersionID string
	CandidateHash     string
	Selector          fingerprint.Selector
	Fingerprint       fingerprint.Fingerprint
	Confidence        float64
	DecisionBand      DecisionBand
	Succeeded         bool
	ObservedAt        int64
}

func (o HealObservation) Validate() error {
	if o.ID == "" || o.RunID == "" || o.ExecutionID == "" || o.StepExecutionID == "" || o.NodeID == "" || o.BaseNodeVersionID == "" {
		return errors.New("heal observation requires identity")
	}
	if o.ObservedAt <= 0 {
		return errors.New("heal observation requires positive time")
	}
	if err := ValidateConfidence(o.Confidence); err != nil {
		return err
	}
	return ValidateDecisionBand(o.CandidateHash, o.DecisionBand)
}

type ValidationProgressObservation struct {
	ID               string
	RunID            string
	ExecutionID      string
	StepExecutionID  string
	ValidationStepID string
	NodeID           string
	NodeVersionID    string
	AssertionKind    string
	Expected         string
	Actual           string
	Passed           bool
	Reason           string
	Selector         fingerprint.Selector
	Healed           bool
	HealConfidence   float64
	HealReviewStatus string
	ObservedAt       int64
}

func (o ValidationProgressObservation) Validate() error {
	return ValidationObservation{
		ID: o.ID, RunID: o.RunID, ExecutionID: o.ExecutionID,
		StepExecutionID: o.StepExecutionID, ValidationStepID: o.ValidationStepID,
		NodeID: o.NodeID, NodeVersionID: o.NodeVersionID,
		AssertionKind: o.AssertionKind, Expected: o.Expected, Actual: o.Actual,
		Passed: o.Passed, Reason: o.Reason, Selector: o.Selector,
		Healed: o.Healed, HealConfidence: o.HealConfidence,
		HealReviewStatus: o.HealReviewStatus, ObservedAt: o.ObservedAt,
	}.Validate()
}

type ValidationObservation struct {
	ID               string
	RunID            string
	ExecutionID      string
	StepExecutionID  string
	ValidationStepID string
	NodeID           string
	NodeVersionID    string
	AssertionKind    string
	Expected         string
	Actual           string
	Passed           bool
	Reason           string
	Selector         fingerprint.Selector
	Healed           bool
	HealConfidence   float64
	HealReviewStatus string
	ObservedAt       int64
	Final            bool
}

func (o ValidationObservation) Validate() error {
	if o.ID == "" || o.RunID == "" || o.ExecutionID == "" || o.StepExecutionID == "" || o.ValidationStepID == "" || o.NodeID == "" || o.NodeVersionID == "" || o.AssertionKind == "" || o.Reason == "" {
		return errors.New("validation observation requires identity and reason")
	}
	if o.ObservedAt <= 0 {
		return errors.New("validation observation requires positive time")
	}
	if err := ValidateConfidence(o.HealConfidence); err != nil {
		return err
	}
	switch o.HealReviewStatus {
	case "not_attempted", "auto_applied", "review_pending", "no_candidate":
		return nil
	default:
		return fmt.Errorf("unsupported heal review status %q", o.HealReviewStatus)
	}
}
