package workspace

import (
	"fmt"
	"math"
	"strings"
)

// VersionMeta is the minimum information required by version lifecycle rules.
// Deleted versions remain part of the sequence and therefore still contribute
// to the next version number.
type VersionMeta struct {
	ID            string
	VersionNumber int
	DeletedAt     int64
}

func NextVersionNumber(existing []VersionMeta) int {
	maximum := 0
	for _, version := range existing {
		if version.VersionNumber > maximum {
			maximum = version.VersionNumber
		}
	}
	return maximum + 1
}

func ResolveCurrentVersion(versions []VersionMeta) (string, bool) {
	var current VersionMeta
	found := false
	for _, version := range versions {
		if version.DeletedAt != 0 || version.ID == "" {
			continue
		}
		if !found || version.VersionNumber > current.VersionNumber {
			current = version
			found = true
		}
	}
	return current.ID, found
}

type HealOutcome string

const (
	HealOriginalRecovered HealOutcome = "ORIGINAL_RECOVERED"
	HealFailed            HealOutcome = "FAILED"
	HealSucceeded         HealOutcome = "SUCCEEDED"
)

// HealDecisionBand preserves the deterministic healer's confidence gate in
// the workspace ledger. UNKNOWN observations are retained as evidence but do
// not become reviewable candidates.
type HealDecisionBand string

const (
	HealDecisionBandUnknown  HealDecisionBand = "UNKNOWN"
	HealDecisionBandApplied  HealDecisionBand = "APPLIED"
	HealDecisionBandBelowCap HealDecisionBand = "BELOW_CAP"
)

func ValidateHealDecisionBand(candidateHash string, band HealDecisionBand) error {
	hasCandidate := strings.TrimSpace(candidateHash) != ""
	if !hasCandidate && band != HealDecisionBandUnknown {
		return fmt.Errorf("heal observation without a candidate must use UNKNOWN decision band")
	}
	if hasCandidate && band != HealDecisionBandApplied && band != HealDecisionBandBelowCap {
		return fmt.Errorf("heal observation with a candidate requires APPLIED or BELOW_CAP decision band")
	}
	return nil
}

func ValidateHealConfidence(confidence float64) error {
	if math.IsNaN(confidence) || confidence < 0 || confidence > 1 {
		return fmt.Errorf("heal confidence must be between 0 and 1")
	}
	return nil
}

type HealCandidateStatus string

const (
	HealCandidateObserving        HealCandidateStatus = "OBSERVING"
	HealCandidateAwaitingApproval HealCandidateStatus = "AWAITING_APPROVAL"
	HealCandidatePromoted         HealCandidateStatus = "PROMOTED"
	HealCandidateRejected         HealCandidateStatus = "REJECTED"
	HealCandidateStale            HealCandidateStatus = "STALE"
)

type HealApprovalStatus string

const (
	HealApprovalNotRequired HealApprovalStatus = "NOT_REQUIRED"
	HealApprovalPending     HealApprovalStatus = "PENDING"
	HealApprovalApproved    HealApprovalStatus = "APPROVED"
	HealApprovalRejected    HealApprovalStatus = "REJECTED"
)

// HealCandidateReviewCommand carries the stable candidate identity and audit
// metadata for either approval or rejection. PromotedVersionID is required
// only by approval because the adapter creates that version atomically.
type HealCandidateReviewCommand struct {
	NodeID            string
	BaseNodeVersionID string
	CandidateHash     string
	PromotedVersionID string
	ReviewedBy        string
	ReviewedAt        int64
}

func (command HealCandidateReviewCommand) Validate(approval HealApprovalStatus) error {
	if strings.TrimSpace(command.NodeID) == "" || strings.TrimSpace(command.BaseNodeVersionID) == "" ||
		strings.TrimSpace(command.CandidateHash) == "" {
		return fmt.Errorf("heal candidate review requires node, base version, and candidate hash")
	}
	if strings.TrimSpace(command.ReviewedBy) == "" || command.ReviewedAt <= 0 {
		return fmt.Errorf("heal candidate review requires reviewer and review time")
	}
	switch approval {
	case HealApprovalApproved:
		if strings.TrimSpace(command.PromotedVersionID) == "" {
			return fmt.Errorf("heal candidate approval requires a promoted version id")
		}
	case HealApprovalRejected:
		if strings.TrimSpace(command.PromotedVersionID) != "" {
			return fmt.Errorf("heal candidate rejection cannot promote a version")
		}
	default:
		return fmt.Errorf("unsupported heal approval status %q", approval)
	}
	return nil
}

type HealStreak struct {
	CandidateHash string
	Count         int
	Observing     bool
}

type HealStreakDecision struct {
	NextCount   int
	ResetAll    bool
	ResetOthers bool
	MarkStale   bool
	Promote     bool
}

// Observe applies the three streak reset rules: the original node recovered,
// healing failed, or another candidate succeeded. A candidate can only be
// promoted while its base version is still current.
func (streak HealStreak) Observe(outcome HealOutcome, candidateHash string, baseIsCurrent bool) (HealStreakDecision, error) {
	switch outcome {
	case HealOriginalRecovered, HealFailed:
		return HealStreakDecision{ResetAll: true}, nil
	case HealSucceeded:
		if candidateHash == "" {
			return HealStreakDecision{}, fmt.Errorf("successful heal requires a candidate hash")
		}
	default:
		return HealStreakDecision{}, fmt.Errorf("unsupported heal outcome %q", outcome)
	}
	if !baseIsCurrent {
		return HealStreakDecision{MarkStale: true}, nil
	}
	if !streak.Observing {
		return HealStreakDecision{NextCount: streak.Count}, nil
	}
	next := 1
	if streak.Observing && streak.CandidateHash == candidateHash {
		next = streak.Count + 1
	}
	return HealStreakDecision{NextCount: next, ResetOthers: true, Promote: next >= 3}, nil
}

func ValidateExecutionStatusTransition(from, to ExecutionStatus) error {
	return ValidateWorkflowExecutionTransition(from, to)
}
