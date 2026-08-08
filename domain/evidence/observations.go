package evidence

import (
	"fmt"
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// DecisionBand 表示自愈候选在审核阈值下的决策档位。
type DecisionBand string

const (
	// DecisionUnknown 表示没有候选或未产生决策。
	DecisionUnknown DecisionBand = "UNKNOWN"
	// DecisionApplied 表示候选已达到应用档位。
	DecisionApplied DecisionBand = "APPLIED"
	// DecisionBelowCap 表示候选达到审核但未达到直接应用档位。
	DecisionBelowCap DecisionBand = "BELOW_CAP"
)

// ValidateDecisionBand 校验候选摘要与决策档位的组合，不在公开违规文本中回显档位或摘要。
func ValidateDecisionBand(candidateHash string, band DecisionBand) error {
	if violations := appendDecisionBandViolations(nil, candidateHash, band, ""); len(violations) != 0 {
		return healObservationInvalidError(violations)
	}
	return nil
}

// appendDecisionBandViolations 追加候选摘要与决策档位不一致时的字段违规。
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

// ValidateConfidence 校验置信度为有限且位于闭区间 [0,1]。
func ValidateConfidence(confidence float64) error {
	if violations := appendConfidenceViolations(nil, confidence, ""); len(violations) != 0 {
		return healObservationInvalidError(violations)
	}
	return nil
}

// appendConfidenceViolations 追加置信度非有限或越界时的字段违规。
func appendConfidenceViolations(violations []fault.Violation, confidence float64, prefix string) []fault.Violation {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "confidence"), "confidence must be finite and within the inclusive range from zero through one"))
	}
	return violations
}

// appendOccurrenceViolations 追加非正 Occurrence 的字段违规，供所有事实和观测载体复用。
func appendOccurrenceViolations(violations []fault.Violation, occurrence int, prefix string) []fault.Violation {
	if occurrence <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "occurrence"), "occurrence must be positive"))
	}
	return violations
}

// joinField 将字段名拼接为相对于当前聚合的逻辑字段路径。
func joinField(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// HealObservation 记录一次自愈尝试的候选、指纹、置信度、决策和执行坐标。
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

// Validate 校验自愈观测的身份、Occurrence、时间、置信度和决策档位。
func (o HealObservation) Validate() error {
	var violations []fault.Violation
	if o.ID == "" || o.InstanceID.Validate() != nil || o.EntryID.Validate() != nil || o.StepExecutionID.Validate() != nil || o.ElementTargetID == "" || o.BaseNodeVersionID == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "heal observation identity is required"))
	}
	violations = appendOccurrenceViolations(violations, o.Occurrence, "")
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

// ValidationValueKind 标识验证实际值是缺失、标量、集合或已脱敏。
type ValidationValueKind string

const (
	// ValidationValueAbsent 表示没有观察到实际值。
	ValidationValueAbsent ValidationValueKind = "absent"
	// ValidationValueScalar 表示实际值是单个字符串。
	ValidationValueScalar ValidationValueKind = "scalar"
	// ValidationValueCollection 表示实际值是字符串集合。
	ValidationValueCollection ValidationValueKind = "collection"
	// ValidationValueRedacted 表示实际值存在但已脱敏。
	ValidationValueRedacted ValidationValueKind = "redacted"
)

// ValidationValue 保存验证期望或实际值的安全表示；原始页面内容不会进入错误文本。
type ValidationValue struct {
	Kind       ValidationValueKind
	Scalar     string
	collection []string
}

// AbsentValidationValue 返回无实际值的验证值。
func AbsentValidationValue() ValidationValue { return ValidationValue{Kind: ValidationValueAbsent} }

// ScalarValidationValue 返回携带标量字符串的验证值。
func ScalarValidationValue(value string) ValidationValue {
	return ValidationValue{Kind: ValidationValueScalar, Scalar: value}
}

// CollectionValidationValue 返回集合验证值，并复制调用方切片。
func CollectionValidationValue(values []string) ValidationValue {
	owned := make([]string, len(values))
	copy(owned, values)
	return ValidationValue{Kind: ValidationValueCollection, collection: owned}
}

// RedactedValidationValue 返回已脱敏的验证值。
func RedactedValidationValue() ValidationValue { return ValidationValue{Kind: ValidationValueRedacted} }

// Validate 校验值类型与载荷组合，不回显类型或载荷，因为载荷属于观测到的页面内容。
func (v ValidationValue) Validate() error {
	if violations := v.appendViolations(nil, ""); len(violations) != 0 {
		return validationObservationInvalidError(violations)
	}
	return nil
}

// appendViolations 追加验证值类型与载荷不一致时的字段违规。
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

// Equal 比较两个验证值的类型、标量和集合内容是否完全相等。
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

// CollectionValue 返回集合载荷的副本；非集合类型返回 nil 和 false。
func (v ValidationValue) CollectionValue() ([]string, bool) {
	if v.Kind != ValidationValueCollection {
		return nil, false
	}
	owned := make([]string, len(v.collection))
	copy(owned, v.collection)
	return owned, true
}

// ValidationTerminalReason 表示验证组终止的原因。
type ValidationTerminalReason string

const (
	// ValidationTerminalPassed 表示验证组有获胜分支并通过。
	ValidationTerminalPassed ValidationTerminalReason = "passed"
	// ValidationTerminalTimeout 表示验证组因超时终止。
	ValidationTerminalTimeout ValidationTerminalReason = "timeout"
	// ValidationTerminalCanceled 表示验证组因取消终止。
	ValidationTerminalCanceled ValidationTerminalReason = "canceled"
	// ValidationTerminalSystemError 表示验证组因系统错误终止。
	ValidationTerminalSystemError ValidationTerminalReason = "system_error"
)

// ValidationBranchDisposition 表示验证组分支的终态归类。
type ValidationBranchDisposition string

const (
	// ValidationBranchWon 表示分支是获胜分支。
	ValidationBranchWon ValidationBranchDisposition = "won"
	// ValidationBranchSatisfiedNotWinner 表示分支满足但未获胜。
	ValidationBranchSatisfiedNotWinner ValidationBranchDisposition = "satisfied_not_winner"
	// ValidationBranchNotSatisfied 表示分支未满足。
	ValidationBranchNotSatisfied ValidationBranchDisposition = "not_satisfied"
	// ValidationBranchNotObserved 表示分支未被观测。
	ValidationBranchNotObserved ValidationBranchDisposition = "not_observed"
)

// isSupported 判断分支终态归类是否属于支持集合。
func (d ValidationBranchDisposition) isSupported() bool {
	switch d {
	case ValidationBranchWon, ValidationBranchSatisfiedNotWinner, ValidationBranchNotSatisfied, ValidationBranchNotObserved:
		return true
	default:
		return false
	}
}

// Validate 校验分支终态归类，并返回验证观测错误。
func (d ValidationBranchDisposition) Validate() error {
	if !d.isSupported() {
		return validationObservationInvalidError([]fault.Violation{
			mustViolation(fault.CodeFieldInvalid, "branchDisposition", "validation branch disposition is not supported"),
		})
	}
	return nil
}

// ValidationMemberIdentity 标识验证组期望的分支和元素目标成员。
type ValidationMemberIdentity struct {
	BranchID        string
	ElementTargetID string
}

// ValidationGroupTerminalObservation 记录验证组终态、获胜分支和期望成员集合。
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

// NewValidationGroupTerminalObservation 创建验证组终态观测，并复制期望成员切片。
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

// ExpectedMembers 返回期望成员切片的副本，避免暴露未导出内部所有权。
func (o ValidationGroupTerminalObservation) ExpectedMembers() []ValidationMemberIdentity {
	owned := make([]ValidationMemberIdentity, len(o.expectedMembers))
	copy(owned, o.expectedMembers)
	return owned
}

// Validate 通过一个携带有序违规项的封套报告所有失败；分支、分组、元素目标身份和终止原因
// 均不会进入公开文本。
func (o ValidationGroupTerminalObservation) Validate() error {
	var violations []fault.Violation
	if o.ID == "" || o.InstanceID.Validate() != nil || o.EntryID.Validate() != nil || o.StepExecutionID.Validate() != nil || o.GroupID == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "validation group observation identity is required"))
	}
	violations = appendOccurrenceViolations(violations, o.Occurrence, "")
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

// ValidationProgressObservation 记录非终态验证进度及自愈状态。
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

// Validate 将进度观测映射为普通验证观测并复用其校验规则。
func (o ValidationProgressObservation) Validate() error {
	return ValidationObservation{
		ID: o.ID, InstanceID: o.InstanceID, EntryID: o.EntryID,
		StepExecutionID: o.StepExecutionID, Occurrence: o.Occurrence,
		ValidationStepID: o.ValidationStepID,
		ElementTargetID:  o.ElementTargetID, ElementTargetVersionID: o.ElementTargetVersionID,
		GroupID: o.GroupID, BranchID: o.BranchID,
		AssertionKind: o.AssertionKind, Expected: o.Expected, Actual: o.Actual,
		Passed: o.Passed, Reason: o.Reason, Selector: o.Selector,
		Healed: o.Healed, HealConfidence: o.HealConfidence,
		HealReviewStatus: o.HealReviewStatus, ObservedAt: o.ObservedAt,
	}.Validate()
}

// ValidationObservation 记录一次验证结果、期望/实际值、分支归类和自愈信息。
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

// Validate 通过一个携带有序违规项的封套报告所有失败；期望值和实际值属于页面内容，子校验
// 失败降级为当前违规项，避免嵌套 fault 泄露这些内容。
func (o ValidationObservation) Validate() error {
	var violations []fault.Violation
	if o.ID == "" || o.InstanceID.Validate() != nil || o.EntryID.Validate() != nil || o.StepExecutionID.Validate() != nil || o.ValidationStepID == "" || o.ElementTargetID == "" || o.ElementTargetVersionID == "" || o.AssertionKind == "" || o.Reason == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "validation observation identity and reason are required"))
	}
	violations = appendOccurrenceViolations(violations, o.Occurrence, "")
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
