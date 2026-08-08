package automation

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// VersionMeta 是版本生命周期规则所需的最少信息。删除的版本仍然是序列的一部分，因此仍然会影响下一个版本号。
type VersionMeta struct {
	ID            string
	VersionNumber int
	DeletedAt     int64
}

// CodeVersionNumberExhausted 表示版本号已耗尽。
const CodeVersionNumberExhausted fault.Code = "AUTOMATION_VERSION_NUMBER_EXHAUSTED"

// VersionNumberOverflowError 构造版本号容量耗尽错误。
func VersionNumberOverflowError() error {
	err, constructionErr := fault.New(
		fault.ResourceExhausted,
		CodeVersionNumberExhausted,
		"automation version number capacity is exhausted",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// NextVersionNumber 根据现有版本计算下一个版本号；持久化版本无效或容量耗尽时返回错误。
func NextVersionNumber(existing []VersionMeta) (int, error) {
	maximum := 0
	for _, version := range existing {
		if version.VersionNumber <= 0 {
			return 0, persistedVersionNumberInvalidError()
		}
		if version.VersionNumber > maximum {
			maximum = version.VersionNumber
		}
	}
	if maximum == math.MaxInt {
		return 0, VersionNumberOverflowError()
	}
	return maximum + 1, nil
}

// ResolveCurrentVersion 从未删除版本中解析版本号最大的当前版本。
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

// HealOutcome 标识一次自愈观测的结果。
type HealOutcome string

const (
	// HealOriginalRecovered 表示原始选择器已恢复。
	HealOriginalRecovered HealOutcome = "ORIGINAL_RECOVERED"
	// HealFailed 表示自愈失败。
	HealFailed HealOutcome = "FAILED"
	// HealSucceeded 表示自愈成功。
	HealSucceeded HealOutcome = "SUCCEEDED"
)

// HealDecisionBand 将确定性治疗者的置信门保留在工作区分类帐中。未知的观察结果将作为证据保留，但不会成为可审查的候选者。
type HealDecisionBand string

const (
	// HealDecisionBandUnknown 表示没有候选治理区间。
	HealDecisionBandUnknown HealDecisionBand = "UNKNOWN"
	// HealDecisionBandApplied 表示达到自动应用阈值。
	HealDecisionBandApplied HealDecisionBand = "APPLIED"
	// HealDecisionBandBelowCap 表示达到审查阈值但未达到自动应用阈值。
	HealDecisionBandBelowCap HealDecisionBand = "BELOW_CAP"
)

// ValidateHealDecisionBand 校验候选身份与决策区间是否匹配。
func ValidateHealDecisionBand(candidateHash string, band HealDecisionBand) error {
	hasCandidate := strings.TrimSpace(candidateHash) != ""
	if !hasCandidate && band != HealDecisionBandUnknown {
		return healDecisionBandInvalidError()
	}
	if hasCandidate && band != HealDecisionBandApplied && band != HealDecisionBandBelowCap {
		return healDecisionBandInvalidError()
	}
	return nil
}

// ValidateHealConfidence 校验置信度为非 NaN 且位于 [0,1] 区间。
func ValidateHealConfidence(confidence float64) error {
	if math.IsNaN(confidence) || confidence < 0 || confidence > 1 {
		return healConfidenceInvalidError()
	}
	return nil
}

// HealCandidateStatus 标识自愈候选的审查生命周期状态。
type HealCandidateStatus string

const (
	// HealCandidateObserving 表示候选正在积累观测。
	HealCandidateObserving HealCandidateStatus = "OBSERVING"
	// HealCandidateAwaitingApproval 表示候选等待审批。
	HealCandidateAwaitingApproval HealCandidateStatus = "AWAITING_APPROVAL"
	// HealCandidatePromoted 表示候选已提升为正式版本。
	HealCandidatePromoted HealCandidateStatus = "PROMOTED"
	// HealCandidateRejected 表示候选已被拒绝。
	HealCandidateRejected HealCandidateStatus = "REJECTED"
	// HealCandidateStale 表示候选基线已过期。
	HealCandidateStale HealCandidateStatus = "STALE"
)

// HealApprovalStatus 标识审查命令携带的审批决定；它不是持久化生命周期状态。
type HealApprovalStatus string

const (
	// HealApprovalApproved 表示审批通过。
	HealApprovalApproved HealApprovalStatus = "APPROVED"
	// HealApprovalRejected 表示审批拒绝。
	HealApprovalRejected HealApprovalStatus = "REJECTED"
)

// HealCandidate 表示带有稳定身份、选择器和指纹的自愈候选。
type HealCandidate struct {
	Hash              string
	ElementTargetID   string
	BaseNodeVersionID string
	Status            HealCandidateStatus
	PageURL           string
	Origin            string
	Selectors         []fingerprint.Selector
	Fingerprint       fingerprint.Fingerprint
	Revision          Revision
}

// Validate 校验候选身份及其是否处于等待审批状态。
func (candidate HealCandidate) Validate() error {
	if err := candidate.validateIdentity(); err != nil {
		return err
	}
	if candidate.Status != HealCandidateAwaitingApproval {
		return healCandidateStateInvalidError()
	}
	return nil
}

// ValidateReviewed 校验候选身份及其是否处于已批准或已拒绝状态。
func (candidate HealCandidate) ValidateReviewed() error {
	if err := candidate.validateIdentity(); err != nil {
		return err
	}
	if candidate.Status != HealCandidatePromoted && candidate.Status != HealCandidateRejected {
		return healCandidateStateInvalidError()
	}
	return nil
}

// validateIdentity 校验候选哈希、节点版本身份和持久化修订。
func (candidate HealCandidate) validateIdentity() error {
	if strings.TrimSpace(candidate.Hash) == "" || strings.TrimSpace(candidate.ElementTargetID) == "" ||
		strings.TrimSpace(candidate.BaseNodeVersionID) == "" {
		return healCandidateIdentityInvalidError()
	}
	return candidate.Revision.ValidatePersisted()
}

// Review 复制候选并应用批准或拒绝状态，成功时递增修订且不修改原值。
func (candidate HealCandidate) Review(status HealCandidateStatus) (HealCandidate, error) {
	if err := candidate.Validate(); err != nil {
		return HealCandidate{}, err
	}
	if status != HealCandidatePromoted && status != HealCandidateRejected {
		return HealCandidate{}, healCandidateReviewStatusInvalidError()
	}
	nextRevision, err := candidate.Revision.Next()
	if err != nil {
		return HealCandidate{}, err
	}
	next := candidate
	next.Selectors = append([]fingerprint.Selector(nil), candidate.Selectors...)
	next.Fingerprint = candidate.Fingerprint.Clone()
	next.Status = status
	next.Revision = nextRevision
	return next, nil
}

// HealCandidateReviewCommand 携带候选身份、期望修订和审查命令身份。
type HealCandidateReviewCommand struct {
	CommandID                 string
	ElementTargetID           string
	BaseNodeVersionID         string
	CandidateHash             string
	ExpectedCandidateRevision Revision
	ExpectedNodeRevision      Revision
}

// Validate 校验审查命令身份、期望修订及审批状态。
func (command HealCandidateReviewCommand) Validate(approval HealApprovalStatus) error {
	for _, identity := range []string{
		command.CommandID,
		command.ElementTargetID,
		command.BaseNodeVersionID,
		command.CandidateHash,
	} {
		if identity != strings.TrimSpace(identity) || parameter.ValidateName(identity) != nil {
			return healCandidateReviewCommandInvalidError()
		}
	}
	if err := command.ExpectedCandidateRevision.ValidatePersisted(); err != nil {
		return err
	}
	if err := command.ExpectedNodeRevision.ValidatePersisted(); err != nil {
		return err
	}
	switch approval {
	case HealApprovalApproved, HealApprovalRejected:
	default:
		return healApprovalStatusInvalidError()
	}
	return nil
}

// HealStreakDisposition 标识自愈连续观测的终态或进行中状态。
type HealStreakDisposition string

const (
	// HealStreakObserving 表示连续观测仍在积累。
	HealStreakObserving HealStreakDisposition = "OBSERVING"
	// HealStreakAutoPublish 表示达到自动发布条件。
	HealStreakAutoPublish HealStreakDisposition = "AUTO_PUBLISH"
	// HealStreakAwaitApproval 表示达到审查条件并等待审批。
	HealStreakAwaitApproval HealStreakDisposition = "AWAIT_APPROVAL"
	// HealStreakReset 表示连续状态已重置。
	HealStreakReset HealStreakDisposition = "RESET"
	// HealStreakStale 表示基线已过期。
	HealStreakStale HealStreakDisposition = "STALE"
	// HealStreakRejected 表示连续状态已被拒绝。
	HealStreakRejected HealStreakDisposition = "REJECTED"
)

// ContributingHealFact 标识组成自愈连续状态的一条事实及其顺序。
type ContributingHealFact struct {
	FactID          string
	CommitID        string
	InstanceID      string
	EntryID         string
	StepExecutionID string
	Sequence        uint64
}

// HealObservation 表示一次带有候选治理和基线状态的自愈观测。
type HealObservation struct {
	FactID            string
	CommitID          string
	InstanceID        string
	EntryID           string
	StepExecutionID   string
	Sequence          uint64
	ElementTargetID   string
	BaseNodeVersionID string
	CandidateHash     string
	Band              HealDecisionBand
	Outcome           HealOutcome
	BaseIsCurrent     bool
}

// HealStreak 保存自愈候选、贡献事实、已消费来源和当前处置状态。
type HealStreak struct {
	ElementTargetID      string
	BaseNodeVersionID    string
	CandidateHash        string
	Band                 HealDecisionBand
	Contributions        []ContributingHealFact
	ConsumedObservations []ContributingHealFact
	LastSequence         uint64
	Observing            bool
	Disposition          HealStreakDisposition
}

// HealStreakDecision 包装接收观测或拒绝后的下一份连续状态。
type HealStreakDecision struct {
	Next HealStreak
}

// Observe 接收一条观测并按来源、序列、基线和决策区间计算下一份连续状态。
func (streak HealStreak) Observe(observation HealObservation) (HealStreakDecision, error) {
	if err := streak.validate(); err != nil {
		return HealStreakDecision{}, healStreakStateInvalidError(err)
	}
	if err := observation.validate(); err != nil {
		return HealStreakDecision{}, healObservationInvalidError(err)
	}
	contribution := observation.contribution()
	for _, existing := range streak.consumedProvenance() {
		if existing == contribution {
			return HealStreakDecision{Next: streak.Clone()}, nil
		}
		if existing.FactID == contribution.FactID || existing.CommitID == contribution.CommitID || existing.InstanceID == contribution.InstanceID || existing.Sequence == contribution.Sequence {
			return HealStreakDecision{}, healProvenanceConflictError(errors.New("heal contribution replay conflicts with persisted provenance"))
		}
	}
	if observation.Sequence <= streak.LastSequence {
		return HealStreakDecision{}, healSequenceConflictError(fmt.Errorf("heal observation sequence %d is not newer than %d", observation.Sequence, streak.LastSequence))
	}
	if streak.Disposition == HealStreakAutoPublish || streak.Disposition == HealStreakAwaitApproval {
		return HealStreakDecision{Next: streak.withObservation(contribution)}, nil
	}
	if streak.Disposition == HealStreakStale {
		if observation.Outcome == HealSucceeded && observation.Band != HealDecisionBandUnknown && observation.BaseIsCurrent && observation.ElementTargetID == streak.ElementTargetID && observation.BaseNodeVersionID != streak.BaseNodeVersionID {
			return HealStreakDecision{Next: newHealStreak(observation)}, nil
		}
		return HealStreakDecision{Next: streak.withObservation(contribution)}, nil
	}
	if streak.Disposition == HealStreakRejected {
		if observation.Outcome == HealSucceeded && observation.Band != HealDecisionBandUnknown && observation.BaseIsCurrent && observation.ElementTargetID == streak.ElementTargetID && observation.BaseNodeVersionID != streak.BaseNodeVersionID {
			return HealStreakDecision{Next: newHealStreak(observation)}, nil
		}
		return HealStreakDecision{Next: streak.withObservation(contribution)}, nil
	}
	if streak.Disposition == HealStreakReset {
		if observation.Outcome == HealSucceeded && observation.Band != HealDecisionBandUnknown && observation.BaseIsCurrent {
			return HealStreakDecision{Next: newHealStreak(observation)}, nil
		}
		return HealStreakDecision{Next: streak.withObservation(contribution)}, nil
	}
	advanced := streak.withObservation(contribution)
	if streak.Observing && (streak.ElementTargetID != observation.ElementTargetID || streak.BaseNodeVersionID != observation.BaseNodeVersionID) {
		return HealStreakDecision{Next: advanced}, nil
	}
	if !observation.BaseIsCurrent {
		if streak.Observing {
			return HealStreakDecision{Next: advanced.withDisposition(HealStreakStale)}, nil
		}
		return HealStreakDecision{Next: newHealTerminal(observation, HealStreakStale)}, nil
	}
	if observation.Outcome == HealFailed {
		return HealStreakDecision{Next: advanced}, nil
	}
	if observation.Outcome == HealOriginalRecovered {
		return HealStreakDecision{Next: newHealTerminal(observation, HealStreakReset)}, nil
	}
	if observation.Band == HealDecisionBandUnknown {
		return HealStreakDecision{Next: advanced}, nil
	}
	if !streak.Observing || !streak.matches(observation) {
		return HealStreakDecision{Next: newHealStreak(observation)}, nil
	}
	next := streak.withObservation(contribution)
	next.Contributions = append(next.Contributions, contribution)
	if len(next.Contributions) >= 3 {
		next.Observing = false
		if next.Band == HealDecisionBandApplied {
			next.Disposition = HealStreakAutoPublish
		} else {
			next.Disposition = HealStreakAwaitApproval
		}
	}
	return HealStreakDecision{Next: next}, nil
}

// Validate 校验连续状态及其贡献事实，失败时返回自愈状态错误。
func (streak HealStreak) Validate() error {
	if err := streak.validate(); err != nil {
		return healStreakStateInvalidError(err)
	}
	return nil
}

// validate 执行连续状态的内部结构、来源顺序和处置状态校验。
func (streak HealStreak) validate() error {
	provenance := streak.consumedProvenance()
	if err := validateHealContributions(provenance); err != nil {
		return fmt.Errorf("validate consumed heal observations: %w", err)
	}
	if len(provenance) > 0 && provenance[len(provenance)-1].Sequence > streak.LastSequence {
		return fmt.Errorf("consumed heal observation exceeds last sequence")
	}
	if !streak.Observing && streak.Disposition == "" && len(streak.Contributions) == 0 {
		if strings.TrimSpace(streak.ElementTargetID) != "" ||
			strings.TrimSpace(streak.BaseNodeVersionID) != "" ||
			strings.TrimSpace(streak.CandidateHash) != "" ||
			streak.Band != "" ||
			len(streak.ConsumedObservations) != 0 ||
			streak.LastSequence != 0 {
			return fmt.Errorf("empty heal streak contains persisted state")
		}
		return nil
	}
	if streak.Disposition == HealStreakReset || streak.Disposition == HealStreakStale {
		if streak.Observing || strings.TrimSpace(streak.ElementTargetID) == "" || strings.TrimSpace(streak.BaseNodeVersionID) == "" || streak.LastSequence == 0 {
			return fmt.Errorf("%s heal streak requires inactive node/base sequence identity", streak.Disposition)
		}
		return nil
	}
	if streak.Disposition == HealStreakRejected {
		if streak.Observing || strings.TrimSpace(streak.ElementTargetID) == "" || strings.TrimSpace(streak.BaseNodeVersionID) == "" || strings.TrimSpace(streak.CandidateHash) == "" || streak.LastSequence == 0 {
			return fmt.Errorf("rejected heal streak requires inactive candidate identity")
		}
		return validateHealContributions(streak.Contributions)
	}
	if strings.TrimSpace(streak.ElementTargetID) == "" || strings.TrimSpace(streak.BaseNodeVersionID) == "" || strings.TrimSpace(streak.CandidateHash) == "" {
		return fmt.Errorf("heal streak requires node, base version, and candidate identity")
	}
	if streak.Band != HealDecisionBandApplied && streak.Band != HealDecisionBandBelowCap {
		return fmt.Errorf("heal streak requires APPLIED or BELOW_CAP decision band")
	}
	if streak.Observing {
		if streak.Disposition != HealStreakObserving || len(streak.Contributions) >= 3 {
			return fmt.Errorf("observing heal streak has invalid disposition or maturity")
		}
	} else {
		switch streak.Disposition {
		case HealStreakAutoPublish:
			if streak.Band != HealDecisionBandApplied || len(streak.Contributions) != 3 {
				return fmt.Errorf("auto-publish streak requires three APPLIED runs")
			}
		case HealStreakAwaitApproval:
			if streak.Band != HealDecisionBandBelowCap || len(streak.Contributions) != 3 {
				return fmt.Errorf("await-approval streak requires three BELOW_CAP runs")
			}
		default:
			return fmt.Errorf("unsupported inactive heal streak disposition %q", streak.Disposition)
		}
	}
	if streak.LastSequence == 0 {
		return fmt.Errorf("heal streak requires a last sequence")
	}
	return validateHealContributions(streak.Contributions)
}

// Reject 在等待审批状态下记录新的拒绝序列并返回拒绝状态。
func (streak HealStreak) Reject(sequence uint64) (HealStreakDecision, error) {
	if err := streak.validate(); err != nil {
		return HealStreakDecision{}, healStreakStateInvalidError(err)
	}
	if streak.Disposition != HealStreakAwaitApproval {
		return HealStreakDecision{}, healStreakRejectionInvalidError(errors.New("only an await-approval heal streak can be rejected"))
	}
	if sequence == 0 || sequence <= streak.LastSequence {
		return HealStreakDecision{}, healSequenceConflictError(fmt.Errorf("heal rejection sequence %d is not newer than %d", sequence, streak.LastSequence))
	}
	next := streak.Clone()
	next.LastSequence = sequence
	next.Observing = false
	next.Disposition = HealStreakRejected
	return HealStreakDecision{Next: next}, nil
}

// validateHealContributions 校验贡献事实身份唯一且序列严格递增。
func validateHealContributions(contributions []ContributingHealFact) error {
	for index, contribution := range contributions {
		if err := contribution.validate(); err != nil {
			return fmt.Errorf("heal contribution %d: %w", index, err)
		}
		if index > 0 && contribution.Sequence <= contributions[index-1].Sequence {
			return fmt.Errorf("heal contribution sequences must be strictly increasing")
		}
		for _, earlier := range contributions[:index] {
			if earlier.FactID == contribution.FactID || earlier.CommitID == contribution.CommitID || earlier.InstanceID == contribution.InstanceID || earlier.Sequence == contribution.Sequence {
				return fmt.Errorf("heal contribution identity is duplicated")
			}
		}
	}
	return nil
}

// validate 校验贡献事实所需的身份字段和非零序列。
func (contribution ContributingHealFact) validate() error {
	if strings.TrimSpace(contribution.FactID) == "" || strings.TrimSpace(contribution.CommitID) == "" || strings.TrimSpace(contribution.InstanceID) == "" || strings.TrimSpace(contribution.EntryID) == "" || strings.TrimSpace(contribution.StepExecutionID) == "" || contribution.Sequence == 0 {
		return fmt.Errorf("heal contribution requires fact, commit, instance, entry, step, and sequence identity")
	}
	return nil
}

// validate 校验观测身份、结果和候选治理字段。
func (observation HealObservation) validate() error {
	if strings.TrimSpace(observation.FactID) == "" || strings.TrimSpace(observation.CommitID) == "" || strings.TrimSpace(observation.InstanceID) == "" || strings.TrimSpace(observation.EntryID) == "" || strings.TrimSpace(observation.StepExecutionID) == "" || observation.Sequence == 0 || strings.TrimSpace(observation.ElementTargetID) == "" || strings.TrimSpace(observation.BaseNodeVersionID) == "" {
		return fmt.Errorf("heal observation requires fact, commit, instance, entry, step, sequence, node, and base version identity")
	}
	if observation.Outcome != HealSucceeded && observation.Outcome != HealOriginalRecovered && observation.Outcome != HealFailed {
		return fmt.Errorf("unsupported heal outcome %q", observation.Outcome)
	}
	if observation.Outcome == HealSucceeded {
		return ValidateHealDecisionBand(observation.CandidateHash, observation.Band)
	}
	if strings.TrimSpace(observation.CandidateHash) != "" || observation.Band != "" && observation.Band != HealDecisionBandUnknown {
		return fmt.Errorf("non-success heal observation cannot carry candidate governance data")
	}
	return nil
}

// contribution 将观测转换为用于去重和顺序追踪的贡献事实。
func (observation HealObservation) contribution() ContributingHealFact {
	return ContributingHealFact{
		FactID: observation.FactID, CommitID: observation.CommitID, InstanceID: observation.InstanceID,
		EntryID: observation.EntryID, StepExecutionID: observation.StepExecutionID, Sequence: observation.Sequence,
	}
}

// newHealStreak 根据成功观测创建新的观察中连续状态。
func newHealStreak(observation HealObservation) HealStreak {
	return HealStreak{
		ElementTargetID: observation.ElementTargetID, BaseNodeVersionID: observation.BaseNodeVersionID,
		CandidateHash: observation.CandidateHash, Band: observation.Band,
		Contributions:        []ContributingHealFact{observation.contribution()},
		ConsumedObservations: []ContributingHealFact{observation.contribution()},
		LastSequence:         observation.Sequence,
		Observing:            true, Disposition: HealStreakObserving,
	}
}

// newHealTerminal 根据观测创建不可继续观察的终态连续状态。
func newHealTerminal(observation HealObservation, disposition HealStreakDisposition) HealStreak {
	return HealStreak{
		ElementTargetID: observation.ElementTargetID, BaseNodeVersionID: observation.BaseNodeVersionID,
		LastSequence: observation.Sequence, Disposition: disposition,
		ConsumedObservations: []ContributingHealFact{observation.contribution()},
	}
}

// matches 判断观测是否与连续状态的节点、基线、候选和决策区间完全匹配。
func (streak HealStreak) matches(observation HealObservation) bool {
	return streak.ElementTargetID == observation.ElementTargetID && streak.BaseNodeVersionID == observation.BaseNodeVersionID &&
		streak.CandidateHash == observation.CandidateHash && streak.Band == observation.Band
}

// isTerminal 判断连续状态是否已进入终态处置。
func (streak HealStreak) isTerminal() bool {
	return streak.Disposition == HealStreakAutoPublish || streak.Disposition == HealStreakAwaitApproval ||
		streak.Disposition == HealStreakReset || streak.Disposition == HealStreakStale || streak.Disposition == HealStreakRejected
}

// Clone 返回自愈连续状态的深复制，独立复制 Contributions 和 ConsumedObservations 两个切片，
// 因此修改副本不会改变原值。
func (streak HealStreak) Clone() HealStreak {
	streak.Contributions = append([]ContributingHealFact(nil), streak.Contributions...)
	streak.ConsumedObservations = append([]ContributingHealFact(nil), streak.ConsumedObservations...)
	return streak
}

// consumedProvenance 返回已消费来源；旧数据缺少该字段时回退到贡献事实。
func (streak HealStreak) consumedProvenance() []ContributingHealFact {
	if len(streak.ConsumedObservations) != 0 {
		return streak.ConsumedObservations
	}
	return streak.Contributions
}

// withObservation 返回追加观测后的连续状态副本并更新最后序列。
func (streak HealStreak) withObservation(contribution ContributingHealFact) HealStreak {
	next := streak.Clone()
	next.ConsumedObservations = append(next.consumedProvenance(), contribution)
	next.LastSequence = contribution.Sequence
	return next
}

// withDisposition 返回应用处置状态后的连续状态副本并停止观察。
func (streak HealStreak) withDisposition(disposition HealStreakDisposition) HealStreak {
	next := streak.Clone()
	next.Observing = false
	next.Disposition = disposition
	return next
}
