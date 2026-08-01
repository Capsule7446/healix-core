package evidence

import (
	"fmt"
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type DecisionBand string

const (
	DecisionUnknown  DecisionBand = "UNKNOWN"
	DecisionApplied  DecisionBand = "APPLIED"
	DecisionBelowCap DecisionBand = "BELOW_CAP"
)

// ValidateDecisionBand rejects without naming the band or the candidate hash: the
// band is caller-declared and the hash identifies a heal candidate.
func ValidateDecisionBand(candidateHash string, band DecisionBand) error {
	if violations := appendDecisionBandViolations(nil, candidateHash, band, ""); len(violations) != 0 {
		return healObservationInvalidError(violations)
	}
	return nil
}

func appendDecisionBandViolations(violations []fault.Violation, candidateHash string, band DecisionBand, prefix string) []fault.Violation {
	hasCandidate := strings.TrimSpace(candidateHash) != ""
	field := joinField(prefix, "decisionBand")
	if !hasCandidate && band != DecisionUnknown {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "an observation without a candidate must use the unknown decision band"))
	}
	if hasCandidate && band != DecisionApplied && band != DecisionBelowCap {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "an observation with a candidate requires an applied or below-cap decision band"))
	}
	return violations
}

func ValidateConfidence(confidence float64) error {
	if violations := appendConfidenceViolations(nil, confidence, ""); len(violations) != 0 {
		return healObservationInvalidError(violations)
	}
	return nil
}

func appendConfidenceViolations(violations []fault.Violation, confidence float64, prefix string) []fault.Violation {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "confidence"), "confidence must be finite and within the inclusive range from zero through one"))
	}
	return violations
}

// joinField builds a logical field path relative to the aggregate being validated.
func joinField(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

type HealObservation struct {
	ID                string
	InstanceID        execution.InstanceID
	EntryID           execution.EntryID
	StepExecutionID   execution.StepExecutionID
	Occurrence        int
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
	var violations []fault.Violation
	if o.ID == "" || o.InstanceID.Validate() != nil || o.EntryID.Validate() != nil || o.StepExecutionID.Validate() != nil || o.ElementTargetID == "" || o.BaseNodeVersionID == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "heal observation identity is required"))
	}
	if o.ObservedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "observedAt", "heal observation time must be positive"))
	}
	violations = appendConfidenceViolations(violations, o.Confidence, "")
	violations = appendDecisionBandViolations(violations, o.CandidateHash, o.DecisionBand, "")
	if len(violations) != 0 {
		return healObservationInvalidError(violations)
	}
	return nil
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

// Validate never echoes the kind or the payload. The payload is observed page
// content, which is exactly what a validation value may not disclose.
func (v ValidationValue) Validate() error {
	if violations := v.appendViolations(nil, ""); len(violations) != 0 {
		return validationObservationInvalidError(violations)
	}
	return nil
}

func (v ValidationValue) appendViolations(violations []fault.Violation, prefix string) []fault.Violation {
	switch v.Kind {
	case ValidationValueAbsent, ValidationValueRedacted:
		if v.Scalar != "" || v.collection != nil {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, joinField(prefix, "payload"), "this validation value kind must not carry a payload"))
		}
	case ValidationValueScalar:
		if v.collection != nil {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, joinField(prefix, "payload"), "a scalar validation value must not carry a collection"))
		}
	case ValidationValueCollection:
		if v.Scalar != "" || v.collection == nil {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, joinField(prefix, "payload"), "a collection validation value requires only a collection payload"))
		}
	default:
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "kind"), "validation value kind is not supported"))
	}
	return violations
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

func (d ValidationBranchDisposition) isSupported() bool {
	switch d {
	case ValidationBranchWon, ValidationBranchSatisfiedNotWinner, ValidationBranchNotSatisfied, ValidationBranchNotObserved:
		return true
	default:
		return false
	}
}

func (d ValidationBranchDisposition) Validate() error {
	if !d.isSupported() {
		return validationObservationInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldInvalid, "branchDisposition", "validation branch disposition is not supported"),
		})
	}
	return nil
}

type ValidationMemberIdentity struct {
	BranchID        string
	ElementTargetID string
}

type ValidationGroupTerminalObservation struct {
	ID              string
	InstanceID      execution.InstanceID
	EntryID         execution.EntryID
	StepExecutionID execution.StepExecutionID
	Occurrence      int
	GroupID         string
	TerminalReason  ValidationTerminalReason
	WinningBranchID string
	expectedMembers []ValidationMemberIdentity
	ObservedAt      int64
}

func NewValidationGroupTerminalObservation(id string, instanceID execution.InstanceID, entryID execution.EntryID, stepExecutionID execution.StepExecutionID, occurrence int, groupID string, terminalReason ValidationTerminalReason, winningBranchID string, expectedMembers []ValidationMemberIdentity, observedAt int64) ValidationGroupTerminalObservation {
	owned := make([]ValidationMemberIdentity, len(expectedMembers))
	copy(owned, expectedMembers)
	return ValidationGroupTerminalObservation{
		ID: id, InstanceID: instanceID, EntryID: entryID, StepExecutionID: stepExecutionID,
		Occurrence: occurrence,
		GroupID:    groupID, TerminalReason: terminalReason, WinningBranchID: winningBranchID,
		expectedMembers: owned, ObservedAt: observedAt,
	}
}

func (o ValidationGroupTerminalObservation) ExpectedMembers() []ValidationMemberIdentity {
	owned := make([]ValidationMemberIdentity, len(o.expectedMembers))
	copy(owned, o.expectedMembers)
	return owned
}

// Validate reports every failure through one envelope with ordered violations.
// Branch, group, and element target identities and the terminal reason all stay
// out of public text.
func (o ValidationGroupTerminalObservation) Validate() error {
	var violations []fault.Violation
	if o.ID == "" || o.InstanceID.Validate() != nil || o.EntryID.Validate() != nil || o.StepExecutionID.Validate() != nil || o.GroupID == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "validation group observation identity is required"))
	}
	if o.ObservedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "observedAt", "validation group observation time must be positive"))
	}
	switch o.TerminalReason {
	case ValidationTerminalPassed:
		if o.WinningBranchID == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, "winningBranchId", "a passed validation group requires a winning branch"))
		}
	case ValidationTerminalTimeout, ValidationTerminalCanceled, ValidationTerminalSystemError:
		if o.WinningBranchID != "" {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, "winningBranchId", "a validation group that did not pass must not have a winning branch"))
		}
	default:
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "terminalReason", "validation group terminal reason is not supported"))
	}
	if len(o.expectedMembers) == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "expectedMembers", "a validation group requires expected members"))
	}
	seen := make(map[ValidationMemberIdentity]struct{}, len(o.expectedMembers))
	hasWinningBranch := false
	for index, member := range o.expectedMembers {
		if atCap(violations) {
			break
		}
		field := fmt.Sprintf("expectedMembers.%d", index)
		if member.BranchID == "" || member.ElementTargetID == "" {
			violations = append(violations, mustViolation(fault.CodeFieldRequired, field, "expected member identity is required"))
		}
		if _, exists := seen[member]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field, "expected member is duplicated"))
		}
		seen[member] = struct{}{}
		hasWinningBranch = hasWinningBranch || member.BranchID == o.WinningBranchID
	}
	if o.TerminalReason == ValidationTerminalPassed && !hasWinningBranch {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "winningBranchId", "the winning branch is not among the expected members"))
	}
	if len(violations) != 0 {
		return validationGroupObservationInvalidError(violations)
	}
	return nil
}

type ValidationProgressObservation struct {
	ID                     string
	InstanceID             execution.InstanceID
	EntryID                execution.EntryID
	StepExecutionID        execution.StepExecutionID
	Occurrence             int
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
		ID: o.ID, InstanceID: o.InstanceID, EntryID: o.EntryID,
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
	InstanceID             execution.InstanceID
	EntryID                execution.EntryID
	StepExecutionID        execution.StepExecutionID
	Occurrence             int
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

// Validate reports every failure through one envelope with ordered violations.
// The expected and actual values are observed page content, so their sub
// validation failures degrade into violations here rather than nesting a fault
// whose text could carry that content.
func (o ValidationObservation) Validate() error {
	var violations []fault.Violation
	if o.ID == "" || o.InstanceID.Validate() != nil || o.EntryID.Validate() != nil || o.StepExecutionID.Validate() != nil || o.ValidationStepID == "" || o.ElementTargetID == "" || o.ElementTargetVersionID == "" || o.AssertionKind == "" || o.Reason == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "validation observation identity and reason are required"))
	}
	if o.ObservedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "observedAt", "validation observation time must be positive"))
	}
	violations = o.Expected.appendViolations(violations, "expected")
	violations = o.Actual.appendViolations(violations, "actual")
	if (o.GroupID == "") != (o.BranchID == "") {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "groupMembership", "group and branch identity must be present together"))
	}
	if o.Final && o.GroupID != "" {
		if !o.BranchDisposition.isSupported() {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "branchDisposition", "validation branch disposition is not supported"))
		}
	} else if o.BranchDisposition != "" {
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, "branchDisposition", "a branch disposition requires a final grouped observation"))
	}
	violations = appendConfidenceViolations(violations, o.HealConfidence, "heal")
	switch o.HealReviewStatus {
	case "not_attempted", "auto_applied", "review_pending", "no_candidate":
	default:
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "healReviewStatus", "heal review status is not supported"))
	}
	if len(violations) != 0 {
		return validationObservationInvalidError(violations)
	}
	return nil
}
