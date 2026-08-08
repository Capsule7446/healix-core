package execution

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// wrapHealGovernanceFault 按给定 Kind、错误码和安全消息包装自愈治理错误。
func wrapHealGovernanceFault(cause error, kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// healGovernanceSnapshotInvalidError 构造自愈治理快照无效的前置条件错误。
func healGovernanceSnapshotInvalidError(cause error) error {
	return wrapHealGovernanceFault(cause, fault.FailedPrecondition, CodeHealGovernanceSnapshotInvalid, "heal governance snapshot is invalid")
}

// healAcceptedFactInvalidError 构造已接受自愈事实无效的调用方错误。
func healAcceptedFactInvalidError(cause error) error {
	return wrapHealGovernanceFault(cause, fault.InvalidArgument, CodeHealAcceptedFactInvalid, "accepted heal fact is invalid")
}

// healTerminalEffectConflictError 构造终态效果与持久化状态冲突的错误。
func healTerminalEffectConflictError(cause error) error {
	return wrapHealGovernanceFault(cause, fault.Conflict, CodeHealTerminalEffectConflict, "heal terminal effect conflicts with persisted state")
}

// HealGovernanceKey 标识一个元素目标及其基线节点版本的自愈治理范围。
type HealGovernanceKey struct {
	ElementTargetID   string
	BaseNodeVersionID string
}

// HealAcceptedFactKind 表示已接受事实是观测还是重置。
type HealAcceptedFactKind string

const (
	// HealAcceptedObservation 表示事实携带自愈观测。
	HealAcceptedObservation HealAcceptedFactKind = "OBSERVATION"
	// HealAcceptedReset 表示事实携带原始状态恢复重置。
	HealAcceptedReset HealAcceptedFactKind = "RESET"
)

// HealAcceptedFact 携带已接受事实的身份、顺序以及观测或重置载荷。
type HealAcceptedFact struct {
	Kind        HealAcceptedFactKind
	FactID      string
	CommitID    string
	InstanceID  domainexecution.InstanceID
	Sequence    uint64
	Observation *evidence.HealObservation
	Reset       *evidence.HealCandidateReset
}

// HealContributionSnapshot 保存用于治理 streak 的事实贡献身份快照。
type HealContributionSnapshot struct {
	FactID          string
	CommitID        string
	InstanceID      string
	EntryID         string
	StepExecutionID string
	Sequence        uint64
}

// HealTerminalEffectKind 表示 streak 转换产生的终态效果。
type HealTerminalEffectKind string

const (
	// HealEffectAutoPublish 表示自动发布候选版本。
	HealEffectAutoPublish HealTerminalEffectKind = "AUTO_PUBLISH"
	// HealEffectAwaitApproval 表示创建待审核候选。
	HealEffectAwaitApproval HealTerminalEffectKind = "AWAIT_APPROVAL"
	// HealEffectReset 表示重置治理状态。
	HealEffectReset HealTerminalEffectKind = "RESET"
	// HealEffectStale 表示将候选标记为过期。
	HealEffectStale HealTerminalEffectKind = "STALE"
)

// HealTerminalEffectSnapshot 保存已持久化终态效果及其候选、决策区间、贡献和发布身份。
type HealTerminalEffectSnapshot struct {
	Kind          HealTerminalEffectKind
	CandidateHash string
	Band          domainautomation.HealDecisionBand
	Contributions []HealContributionSnapshot
	VersionID     string
	ReviewID      string
}

// HealGovernanceSnapshot 保存治理范围当前节点版本、修订、streak、候选状态和既有效果。
type HealGovernanceSnapshot struct {
	Key                    HealGovernanceKey
	CurrentNodeVersionID   string
	Revision               domainautomation.Revision
	Streak                 domainautomation.HealStreak
	CandidateStatus        domainautomation.HealCandidateStatus
	ExistingTerminalEffect *HealTerminalEffectSnapshot
}

// HealGovernancePlan 将治理快照与一个已接受事实交给规划器。
type HealGovernancePlan struct {
	Snapshot HealGovernanceSnapshot
	Fact     HealAcceptedFact
}

// HealTerminalEffectIntent 描述本次 streak 转换需要提交的终态效果。
type HealTerminalEffectIntent struct {
	Kind          HealTerminalEffectKind
	CandidateHash string
	Band          domainautomation.HealDecisionBand
	Contributions []HealContributionSnapshot
}

// HealGovernanceDecision 保存事实应用所需的键、期望修订、下一 streak 和可选效果。
type HealGovernanceDecision struct {
	Key              HealGovernanceKey
	FactID           string
	Sequence         uint64
	ExpectedRevision domainautomation.Revision
	NextStreak       domainautomation.HealStreak
	Effect           *HealTerminalEffectIntent
}

// HealGovernancePlanner 定义根据已接受事实规划自愈治理转换的端口。
type HealGovernancePlanner interface {
	// PlanHealGovernance 校验计划并返回下一 streak 与终态效果决策。
	PlanHealGovernance(HealGovernancePlan) (HealGovernanceDecision, error)
}

// DefaultHealGovernancePlanner 使用 Core 领域规则规划自愈治理。
type DefaultHealGovernancePlanner struct{}

// NewDefaultHealGovernancePlanner 构造默认自愈治理规划器。
func NewDefaultHealGovernancePlanner() DefaultHealGovernancePlanner {
	return DefaultHealGovernancePlanner{}
}

// PlanHealGovernance 校验治理计划、映射已接受事实、推进 streak，并生成终态效果意图。
func (DefaultHealGovernancePlanner) PlanHealGovernance(plan HealGovernancePlan) (HealGovernanceDecision, error) {
	if err := validateHealGovernancePlan(plan); err != nil {
		return HealGovernanceDecision{}, err
	}
	observation, err := mapAcceptedHealFact(plan.Fact, plan.Snapshot)
	if err != nil {
		return HealGovernanceDecision{}, healAcceptedFactInvalidError(err)
	}
	previous := cloneHealStreak(plan.Snapshot.Streak)
	transition, err := previous.Observe(observation)
	if err != nil {
		return HealGovernanceDecision{}, err
	}
	decision := HealGovernanceDecision{
		Key: plan.Snapshot.Key, FactID: plan.Fact.FactID, Sequence: plan.Fact.Sequence,
		ExpectedRevision: plan.Snapshot.Revision, NextStreak: cloneHealStreak(transition.Next),
	}
	decision.Effect = terminalEffectForTransition(previous, transition.Next)
	return decision, nil
}

// validateCanonicalHealIdentity 校验自愈身份已规范化且符合参数名称规则。
func validateCanonicalHealIdentity(identity string) error {
	if identity != strings.TrimSpace(identity) || parameter.ValidateName(identity) != nil {
		return errors.New("heal identity is invalid")
	}
	return nil
}

// validateHealContributionIdentities 校验贡献事实中所有身份字段。
func validateHealContributionIdentities(contribution domainautomation.ContributingHealFact) error {
	for _, identity := range []string{
		contribution.FactID,
		contribution.CommitID,
		contribution.InstanceID,
		contribution.EntryID,
		contribution.StepExecutionID,
	} {
		if err := validateCanonicalHealIdentity(identity); err != nil {
			return err
		}
	}
	return nil
}

// validateHealStreakIdentities 校验 streak 本身及其贡献、已消费观测中的身份字段。
func validateHealStreakIdentities(streak domainautomation.HealStreak) error {
	for _, identity := range []string{streak.ElementTargetID, streak.BaseNodeVersionID, streak.CandidateHash} {
		if identity != "" {
			if err := validateCanonicalHealIdentity(identity); err != nil {
				return err
			}
		}
	}
	for _, contribution := range streak.Contributions {
		if err := validateHealContributionIdentities(contribution); err != nil {
			return err
		}
	}
	for _, contribution := range streak.ConsumedObservations {
		if err := validateHealContributionIdentities(contribution); err != nil {
			return err
		}
	}
	return nil
}

// validateHealGovernancePlan 校验治理快照、事实身份、streak 一致性和既有效果契约。
func validateHealGovernancePlan(plan HealGovernancePlan) error {
	for _, identity := range []string{
		plan.Snapshot.Key.ElementTargetID,
		plan.Snapshot.Key.BaseNodeVersionID,
		plan.Snapshot.CurrentNodeVersionID,
	} {
		if err := validateCanonicalHealIdentity(identity); err != nil {
			return healGovernanceSnapshotInvalidError(err)
		}
	}
	if err := plan.Snapshot.Revision.ValidatePersisted(); err != nil {
		return healGovernanceSnapshotInvalidError(err)
	}
	if err := plan.Snapshot.Streak.Validate(); err != nil {
		return healGovernanceSnapshotInvalidError(err)
	}
	if err := validateHealStreakIdentities(plan.Snapshot.Streak); err != nil {
		return healGovernanceSnapshotInvalidError(err)
	}
	streak := plan.Snapshot.Streak
	if streak.Disposition != "" && (streak.ElementTargetID != plan.Snapshot.Key.ElementTargetID || streak.BaseNodeVersionID != plan.Snapshot.Key.BaseNodeVersionID) {
		return healGovernanceSnapshotInvalidError(errors.New("heal governance streak does not match snapshot key"))
	}
	if err := validateHealCandidateStatus(plan.Snapshot.CandidateStatus, streak.Disposition); err != nil {
		return healGovernanceSnapshotInvalidError(err)
	}
	if err := validateExistingHealEffect(plan.Snapshot.ExistingTerminalEffect, streak); err != nil {
		return healTerminalEffectConflictError(err)
	}
	for _, identity := range []string{plan.Fact.FactID, plan.Fact.CommitID, plan.Fact.InstanceID.String()} {
		if err := validateCanonicalHealIdentity(identity); err != nil {
			return healAcceptedFactInvalidError(err)
		}
	}
	if plan.Fact.Sequence == 0 {
		return healAcceptedFactInvalidError(errors.New("accepted heal fact requires a sequence identity"))
	}
	return nil
}

// validateHealCandidateStatus 校验候选状态与 streak 处置状态匹配。
func validateHealCandidateStatus(status domainautomation.HealCandidateStatus, disposition domainautomation.HealStreakDisposition) error {
	if status == "" {
		return nil
	}
	matches := false
	switch status {
	case domainautomation.HealCandidateObserving:
		matches = disposition == domainautomation.HealStreakObserving
	case domainautomation.HealCandidateAwaitingApproval:
		matches = disposition == domainautomation.HealStreakAwaitApproval
	case domainautomation.HealCandidatePromoted:
		matches = disposition == domainautomation.HealStreakAutoPublish
	case domainautomation.HealCandidateRejected:
		matches = disposition == domainautomation.HealStreakRejected
	case domainautomation.HealCandidateStale:
		matches = disposition == domainautomation.HealStreakStale
	}
	if !matches {
		return fmt.Errorf("heal candidate status %q conflicts with streak disposition %q", status, disposition)
	}
	return nil
}

// validateHealEffectIdentities 校验终态效果的发布身份与效果种类约束。
func validateHealEffectIdentities(effect *HealTerminalEffectSnapshot) error {
	for _, identity := range []string{effect.VersionID, effect.ReviewID} {
		if identity != "" {
			if err := validateCanonicalHealIdentity(identity); err != nil {
				return err
			}
		}
	}
	switch effect.Kind {
	case HealEffectAutoPublish:
		if effect.ReviewID != "" {
			return errors.New("auto-publish effect cannot carry review identity")
		}
	case HealEffectAwaitApproval:
		if effect.VersionID != "" {
			return errors.New("await-approval effect cannot carry version identity")
		}
	case HealEffectReset, HealEffectStale:
		if effect.VersionID != "" || effect.ReviewID != "" {
			return errors.New("terminal effect cannot carry publication identity")
		}
	}
	return nil
}

// validateExistingHealEffect 校验已存在效果与 streak 终态、候选哈希、决策区间和贡献完全一致。
func validateExistingHealEffect(effect *HealTerminalEffectSnapshot, streak domainautomation.HealStreak) error {
	if effect == nil {
		return nil
	}
	if err := validateHealEffectIdentities(effect); err != nil {
		return err
	}
	var expectedKind HealTerminalEffectKind
	switch streak.Disposition {
	case domainautomation.HealStreakAutoPublish:
		expectedKind = HealEffectAutoPublish
	case domainautomation.HealStreakAwaitApproval:
		expectedKind = HealEffectAwaitApproval
	case domainautomation.HealStreakReset:
		expectedKind = HealEffectReset
	case domainautomation.HealStreakStale:
		expectedKind = HealEffectStale
	default:
		return fmt.Errorf("existing heal effect requires a terminal streak")
	}
	contributions := make([]HealContributionSnapshot, len(streak.Contributions))
	for index, contribution := range streak.Contributions {
		contributions[index] = HealContributionSnapshot(contribution)
	}
	if effect.Kind != expectedKind || effect.CandidateHash != streak.CandidateHash || effect.Band != streak.Band || !slices.Equal(effect.Contributions, contributions) {
		return fmt.Errorf("existing heal effect conflicts with terminal streak")
	}
	return nil
}

// mapAcceptedHealFact 将观测或重置事实映射为领域 HealObservation，并绑定治理范围和当前基线。
func mapAcceptedHealFact(fact HealAcceptedFact, snapshot HealGovernanceSnapshot) (domainautomation.HealObservation, error) {
	baseIsCurrent := snapshot.CurrentNodeVersionID == snapshot.Key.BaseNodeVersionID
	switch fact.Kind {
	case HealAcceptedObservation:
		if fact.Observation == nil || fact.Reset != nil {
			return domainautomation.HealObservation{}, fmt.Errorf("accepted observation requires exactly one observation payload")
		}
		observation := *fact.Observation
		if err := domainautomation.ValidateHealConfidence(observation.Confidence); err != nil {
			return domainautomation.HealObservation{}, err
		}
		for _, identity := range []string{
			observation.ID,
			observation.InstanceID.String(),
			observation.EntryID.String(),
			observation.StepExecutionID.String(),
			observation.ElementTargetID,
			observation.BaseNodeVersionID,
		} {
			if err := validateCanonicalHealIdentity(identity); err != nil {
				return domainautomation.HealObservation{}, err
			}
		}
		if observation.CandidateHash != "" {
			if err := validateCanonicalHealIdentity(observation.CandidateHash); err != nil {
				return domainautomation.HealObservation{}, err
			}
		}
		if observation.ID != fact.FactID || observation.InstanceID != fact.InstanceID || observation.ElementTargetID != snapshot.Key.ElementTargetID || observation.BaseNodeVersionID != snapshot.Key.BaseNodeVersionID {
			return domainautomation.HealObservation{}, fmt.Errorf("accepted observation does not match governance identity")
		}
		band, err := mapHealDecisionBand(observation.DecisionBand)
		if err != nil {
			return domainautomation.HealObservation{}, err
		}
		outcome := domainautomation.HealFailed
		if observation.Succeeded {
			outcome = domainautomation.HealSucceeded
		}
		return domainautomation.HealObservation{
			FactID: fact.FactID, CommitID: fact.CommitID, InstanceID: fact.InstanceID.String(),
			EntryID: observation.EntryID.String(), StepExecutionID: observation.StepExecutionID.String(), Sequence: fact.Sequence,
			ElementTargetID: observation.ElementTargetID, BaseNodeVersionID: observation.BaseNodeVersionID,
			CandidateHash: observation.CandidateHash, Band: band, Outcome: outcome, BaseIsCurrent: baseIsCurrent,
		}, nil
	case HealAcceptedReset:
		if fact.Reset == nil || fact.Observation != nil {
			return domainautomation.HealObservation{}, fmt.Errorf("accepted reset requires exactly one reset payload")
		}
		reset := *fact.Reset
		for _, identity := range []string{
			reset.EntryID.String(),
			reset.StepExecutionID.String(),
			reset.ElementTargetID,
			reset.BaseNodeVersionID,
		} {
			if err := validateCanonicalHealIdentity(identity); err != nil {
				return domainautomation.HealObservation{}, err
			}
		}
		if reset.ElementTargetID != snapshot.Key.ElementTargetID || reset.BaseNodeVersionID != snapshot.Key.BaseNodeVersionID {
			return domainautomation.HealObservation{}, fmt.Errorf("accepted reset does not match governance identity")
		}
		return domainautomation.HealObservation{
			FactID: fact.FactID, CommitID: fact.CommitID, InstanceID: fact.InstanceID.String(),
			EntryID: reset.EntryID.String(), StepExecutionID: reset.StepExecutionID.String(), Sequence: fact.Sequence,
			ElementTargetID: reset.ElementTargetID, BaseNodeVersionID: reset.BaseNodeVersionID,
			Outcome: domainautomation.HealOriginalRecovered, BaseIsCurrent: baseIsCurrent,
		}, nil
	default:
		return domainautomation.HealObservation{}, fmt.Errorf("unsupported accepted heal fact kind %q", fact.Kind)
	}
}

// mapHealDecisionBand 将 evidence 决策区间映射为 automation 领域区间。
func mapHealDecisionBand(band evidence.DecisionBand) (domainautomation.HealDecisionBand, error) {
	switch band {
	case evidence.DecisionUnknown:
		return domainautomation.HealDecisionBandUnknown, nil
	case evidence.DecisionApplied:
		return domainautomation.HealDecisionBandApplied, nil
	case evidence.DecisionBelowCap:
		return domainautomation.HealDecisionBandBelowCap, nil
	default:
		return "", fmt.Errorf("unsupported evidence decision band %q", band)
	}
}

// terminalEffectForTransition 在 streak 处置发生变化时生成相应的终态效果意图。
func terminalEffectForTransition(previous, next domainautomation.HealStreak) *HealTerminalEffectIntent {
	if previous.Disposition == next.Disposition {
		return nil
	}
	var kind HealTerminalEffectKind
	switch next.Disposition {
	case domainautomation.HealStreakAutoPublish:
		kind = HealEffectAutoPublish
	case domainautomation.HealStreakAwaitApproval:
		kind = HealEffectAwaitApproval
	case domainautomation.HealStreakReset:
		kind = HealEffectReset
	case domainautomation.HealStreakStale:
		kind = HealEffectStale
	default:
		return nil
	}
	contributions := make([]HealContributionSnapshot, len(next.Contributions))
	for index, contribution := range next.Contributions {
		contributions[index] = HealContributionSnapshot(contribution)
	}
	return &HealTerminalEffectIntent{Kind: kind, CandidateHash: next.CandidateHash, Band: next.Band, Contributions: contributions}
}

// cloneHealStreak 返回 streak 的独立副本，保持切片所有权隔离。
func cloneHealStreak(streak domainautomation.HealStreak) domainautomation.HealStreak {
	return streak.Clone()
}
