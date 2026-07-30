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
	ElementTargetID   string
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
	if o.ID == "" || o.RunID == "" || o.ExecutionID == "" || o.StepExecutionID == "" || o.ElementTargetID == "" || o.BaseNodeVersionID == "" {
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

type ValidationValueKind string

const (
	ValidationValueAbsent     ValidationValueKind = "absent"
	ValidationValueScalar     ValidationValueKind = "scalar"
	ValidationValueCollection ValidationValueKind = "collection"
	ValidationValueRedacted   ValidationValueKind = "redacted"
)

type ValidationValue struct {
	Kind       ValidationValueKind
	Scalar     string
	collection []string
}

func AbsentValidationValue() ValidationValue { return ValidationValue{Kind: ValidationValueAbsent} }
func ScalarValidationValue(value string) ValidationValue {
	return ValidationValue{Kind: ValidationValueScalar, Scalar: value}
}
func CollectionValidationValue(values []string) ValidationValue {
	owned := make([]string, len(values))
	copy(owned, values)
	return ValidationValue{Kind: ValidationValueCollection, collection: owned}
}
func RedactedValidationValue() ValidationValue { return ValidationValue{Kind: ValidationValueRedacted} }

func (v ValidationValue) Validate() error {
	switch v.Kind {
	case ValidationValueAbsent, ValidationValueRedacted:
		if v.Scalar != "" || v.collection != nil {
			return errors.New("validation value kind does not carry a payload")
		}
	case ValidationValueScalar:
		if v.collection != nil {
			return errors.New("scalar validation value cannot carry a collection")
		}
	case ValidationValueCollection:
		if v.Scalar != "" || v.collection == nil {
			return errors.New("collection validation value requires only a collection payload")
		}
	default:
		return fmt.Errorf("unsupported validation value kind %q", v.Kind)
	}
	return nil
}

func (v ValidationValue) Equal(other ValidationValue) bool {
	if v.Kind != other.Kind || v.Scalar != other.Scalar || len(v.collection) != len(other.collection) {
		return false
	}
	for index := range v.collection {
		if v.collection[index] != other.collection[index] {
			return false
		}
	}
	return true
}

func (v ValidationValue) CollectionValue() ([]string, bool) {
	if v.Kind != ValidationValueCollection {
		return nil, false
	}
	owned := make([]string, len(v.collection))
	copy(owned, v.collection)
	return owned, true
}

type ValidationTerminalReason string

const (
	ValidationTerminalPassed      ValidationTerminalReason = "passed"
	ValidationTerminalTimeout     ValidationTerminalReason = "timeout"
	ValidationTerminalCanceled    ValidationTerminalReason = "canceled"
	ValidationTerminalSystemError ValidationTerminalReason = "system_error"
)

type ValidationBranchDisposition string

const (
	ValidationBranchWon                ValidationBranchDisposition = "won"
	ValidationBranchSatisfiedNotWinner ValidationBranchDisposition = "satisfied_not_winner"
	ValidationBranchNotSatisfied       ValidationBranchDisposition = "not_satisfied"
	ValidationBranchNotObserved        ValidationBranchDisposition = "not_observed"
)

func (d ValidationBranchDisposition) Validate() error {
	switch d {
	case ValidationBranchWon, ValidationBranchSatisfiedNotWinner, ValidationBranchNotSatisfied, ValidationBranchNotObserved:
		return nil
	default:
		return fmt.Errorf("unsupported validation branch disposition %q", d)
	}
}

type ValidationMemberIdentity struct {
	BranchID        string
	ElementTargetID string
}

type ValidationGroupTerminalObservation struct {
	ID              string
	RunID           string
	ExecutionID     string
	StepExecutionID string
	GroupID         string
	TerminalReason  ValidationTerminalReason
	WinningBranchID string
	expectedMembers []ValidationMemberIdentity
	ObservedAt      int64
}

func NewValidationGroupTerminalObservation(id, runID, executionID, stepExecutionID, groupID string, terminalReason ValidationTerminalReason, winningBranchID string, expectedMembers []ValidationMemberIdentity, observedAt int64) ValidationGroupTerminalObservation {
	owned := make([]ValidationMemberIdentity, len(expectedMembers))
	copy(owned, expectedMembers)
	return ValidationGroupTerminalObservation{
		ID: id, RunID: runID, ExecutionID: executionID, StepExecutionID: stepExecutionID,
		GroupID: groupID, TerminalReason: terminalReason, WinningBranchID: winningBranchID,
		expectedMembers: owned, ObservedAt: observedAt,
	}
}

func (o ValidationGroupTerminalObservation) ExpectedMembers() []ValidationMemberIdentity {
	owned := make([]ValidationMemberIdentity, len(o.expectedMembers))
	copy(owned, o.expectedMembers)
	return owned
}

func (o ValidationGroupTerminalObservation) Validate() error {
	if o.ID == "" || o.RunID == "" || o.ExecutionID == "" || o.StepExecutionID == "" || o.GroupID == "" || o.ObservedAt <= 0 {
		return errors.New("validation group terminal observation requires identity and positive time")
	}
	switch o.TerminalReason {
	case ValidationTerminalPassed:
		if o.WinningBranchID == "" {
			return errors.New("passed validation group requires a winning branch")
		}
	case ValidationTerminalTimeout, ValidationTerminalCanceled, ValidationTerminalSystemError:
		if o.WinningBranchID != "" {
			return errors.New("failed validation group cannot have a winning branch")
		}
	default:
		return fmt.Errorf("unsupported validation terminal reason %q", o.TerminalReason)
	}
	if len(o.expectedMembers) == 0 {
		return errors.New("validation group requires expected members")
	}
	seen := make(map[ValidationMemberIdentity]struct{}, len(o.expectedMembers))
	hasWinningBranch := false
	for _, member := range o.expectedMembers {
		if member.BranchID == "" || member.ElementTargetID == "" {
			return errors.New("validation group expected member requires identity")
		}
		if _, exists := seen[member]; exists {
			return errors.New("validation group expected member is duplicated")
		}
		seen[member] = struct{}{}
		hasWinningBranch = hasWinningBranch || member.BranchID == o.WinningBranchID
	}
	if o.TerminalReason == ValidationTerminalPassed && !hasWinningBranch {
		return errors.New("validation group winning branch is not expected")
	}
	return nil
}

type ValidationProgressObservation struct {
	ID                     string
	RunID                  string
	ExecutionID            string
	StepExecutionID        string
	ValidationStepID       string
	ElementTargetID        string
	ElementTargetVersionID string
	GroupID                string
	BranchID               string
	AssertionKind          string
	Expected               ValidationValue
	Actual                 ValidationValue
	Passed                 bool
	Reason                 string
	Selector               fingerprint.Selector
	Healed                 bool
	HealConfidence         float64
	HealReviewStatus       string
	ObservedAt             int64
}

func (o ValidationProgressObservation) Validate() error {
	return ValidationObservation{
		ID: o.ID, RunID: o.RunID, ExecutionID: o.ExecutionID,
		StepExecutionID: o.StepExecutionID, ValidationStepID: o.ValidationStepID,
		ElementTargetID: o.ElementTargetID, ElementTargetVersionID: o.ElementTargetVersionID,
		GroupID: o.GroupID, BranchID: o.BranchID,
		AssertionKind: o.AssertionKind, Expected: o.Expected, Actual: o.Actual,
		Passed: o.Passed, Reason: o.Reason, Selector: o.Selector,
		Healed: o.Healed, HealConfidence: o.HealConfidence,
		HealReviewStatus: o.HealReviewStatus, ObservedAt: o.ObservedAt,
	}.Validate()
}

type ValidationObservation struct {
	ID                     string
	RunID                  string
	ExecutionID            string
	StepExecutionID        string
	ValidationStepID       string
	ElementTargetID        string
	ElementTargetVersionID string
	GroupID                string
	BranchID               string
	AssertionKind          string
	Expected               ValidationValue
	Actual                 ValidationValue
	Passed                 bool
	Reason                 string
	BranchDisposition      ValidationBranchDisposition
	Selector               fingerprint.Selector
	Healed                 bool
	HealConfidence         float64
	HealReviewStatus       string
	ObservedAt             int64
	Final                  bool
}

func (o ValidationObservation) Validate() error {
	if o.ID == "" || o.RunID == "" || o.ExecutionID == "" || o.StepExecutionID == "" || o.ValidationStepID == "" || o.ElementTargetID == "" || o.ElementTargetVersionID == "" || o.AssertionKind == "" || o.Reason == "" {
		return errors.New("validation observation requires identity and reason")
	}
	if o.ObservedAt <= 0 {
		return errors.New("validation observation requires positive time")
	}
	if err := o.Expected.Validate(); err != nil {
		return fmt.Errorf("validation expected value: %w", err)
	}
	if err := o.Actual.Validate(); err != nil {
		return fmt.Errorf("validation actual value: %w", err)
	}
	if (o.GroupID == "") != (o.BranchID == "") {
		return errors.New("validation group and branch identity must be present together")
	}
	if o.Final && o.GroupID != "" {
		if err := o.BranchDisposition.Validate(); err != nil {
			return err
		}
	} else if o.BranchDisposition != "" {
		return errors.New("validation branch disposition requires a final grouped observation")
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
