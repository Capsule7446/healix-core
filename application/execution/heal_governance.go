package execution

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

const (
	CodeHealGovernanceSnapshotInvalid fault.Code = "EXECUTION_HEAL_GOVERNANCE_SNAPSHOT_INVALID"
	CodeHealAcceptedFactInvalid       fault.Code = "EXECUTION_HEAL_ACCEPTED_FACT_INVALID"
	CodeHealTerminalEffectConflict    fault.Code = "EXECUTION_HEAL_TERMINAL_EFFECT_CONFLICT"
)

func wrapHealGovernanceFault(cause error, kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func healGovernanceSnapshotInvalidError(cause error) error {
	return wrapHealGovernanceFault(cause, fault.FailedPrecondition, CodeHealGovernanceSnapshotInvalid, "heal governance snapshot is invalid")
}

func healAcceptedFactInvalidError(cause error) error {
	return wrapHealGovernanceFault(cause, fault.InvalidArgument, CodeHealAcceptedFactInvalid, "accepted heal fact is invalid")
}

func healTerminalEffectConflictError(cause error) error {
	return wrapHealGovernanceFault(cause, fault.Conflict, CodeHealTerminalEffectConflict, "heal terminal effect conflicts with persisted state")
}

type HealGovernanceKey struct {
	ElementTargetID   string
	BaseNodeVersionID string
}

type HealAcceptedFactKind string

const (
	HealAcceptedObservation HealAcceptedFactKind = "OBSERVATION"
	HealAcceptedReset       HealAcceptedFactKind = "RESET"
)

type HealAcceptedFact struct {
	Kind        HealAcceptedFactKind
	FactID      string
	CommitID    string
	RunID       string
	Sequence    uint64
	Observation *evidence.HealObservation
	Reset       *evidence.HealCandidateReset
}

type HealContributionSnapshot struct {
	FactID          string
	CommitID        string
	RunID           string
	ExecutionID     string
	StepExecutionID string
	Sequence        uint64
}

type HealTerminalEffectKind string

const (
	HealEffectAutoPublish   HealTerminalEffectKind = "AUTO_PUBLISH"
	HealEffectAwaitApproval HealTerminalEffectKind = "AWAIT_APPROVAL"
	HealEffectReset         HealTerminalEffectKind = "RESET"
	HealEffectStale         HealTerminalEffectKind = "STALE"
)

type HealTerminalEffectSnapshot struct {
	Kind          HealTerminalEffectKind
	CandidateHash string
	Band          domainautomation.HealDecisionBand
	Contributions []HealContributionSnapshot
	VersionID     string
	ReviewID      string
}

type HealGovernanceSnapshot struct {
	Key                    HealGovernanceKey
	CurrentNodeVersionID   string
	Revision               domainautomation.Revision
	Streak                 domainautomation.HealStreak
	CandidateStatus        domainautomation.HealCandidateStatus
	ExistingTerminalEffect *HealTerminalEffectSnapshot
}

type HealGovernancePlan struct {
	Snapshot HealGovernanceSnapshot
	Fact     HealAcceptedFact
}

type HealTerminalEffectIntent struct {
	Kind          HealTerminalEffectKind
	CandidateHash string
	Band          domainautomation.HealDecisionBand
	Contributions []HealContributionSnapshot
}

type HealGovernanceDecision struct {
	Key              HealGovernanceKey
	FactID           string
	Sequence         uint64
	ExpectedRevision domainautomation.Revision
	NextStreak       domainautomation.HealStreak
	Effect           *HealTerminalEffectIntent
}

type HealGovernancePlanner interface {
	PlanHealGovernance(HealGovernancePlan) (HealGovernanceDecision, error)
}

type DefaultHealGovernancePlanner struct{}

func NewDefaultHealGovernancePlanner() DefaultHealGovernancePlanner {
	return DefaultHealGovernancePlanner{}
}

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

func validateCanonicalHealIdentity(identity string) error {
	if identity != strings.TrimSpace(identity) || parameter.ValidateName(identity) != nil {
		return errors.New("heal identity is invalid")
	}
	return nil
}

func validateHealContributionIdentities(contribution domainautomation.ContributingHealFact) error {
	for _, identity := range []string{
		contribution.FactID,
		contribution.CommitID,
		contribution.RunID,
		contribution.ExecutionID,
		contribution.StepExecutionID,
	} {
		if err := validateCanonicalHealIdentity(identity); err != nil {
			return err
		}
	}
	return nil
}

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
	for _, identity := range []string{plan.Fact.FactID, plan.Fact.CommitID, plan.Fact.RunID} {
		if err := validateCanonicalHealIdentity(identity); err != nil {
			return healAcceptedFactInvalidError(err)
		}
	}
	if plan.Fact.Sequence == 0 {
		return healAcceptedFactInvalidError(errors.New("accepted heal fact requires a sequence identity"))
	}
	return nil
}

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
			observation.RunID,
			observation.ExecutionID,
			observation.StepExecutionID,
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
		if observation.ID != fact.FactID || observation.RunID != fact.RunID || observation.ElementTargetID != snapshot.Key.ElementTargetID || observation.BaseNodeVersionID != snapshot.Key.BaseNodeVersionID {
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
			FactID: fact.FactID, CommitID: fact.CommitID, RunID: fact.RunID,
			ExecutionID: observation.ExecutionID, StepExecutionID: observation.StepExecutionID, Sequence: fact.Sequence,
			ElementTargetID: observation.ElementTargetID, BaseNodeVersionID: observation.BaseNodeVersionID,
			CandidateHash: observation.CandidateHash, Band: band, Outcome: outcome, BaseIsCurrent: baseIsCurrent,
		}, nil
	case HealAcceptedReset:
		if fact.Reset == nil || fact.Observation != nil {
			return domainautomation.HealObservation{}, fmt.Errorf("accepted reset requires exactly one reset payload")
		}
		reset := *fact.Reset
		for _, identity := range []string{
			reset.ExecutionID,
			reset.StepExecutionID,
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
			FactID: fact.FactID, CommitID: fact.CommitID, RunID: fact.RunID,
			ExecutionID: reset.ExecutionID, StepExecutionID: reset.StepExecutionID, Sequence: fact.Sequence,
			ElementTargetID: reset.ElementTargetID, BaseNodeVersionID: reset.BaseNodeVersionID,
			Outcome: domainautomation.HealOriginalRecovered, BaseIsCurrent: baseIsCurrent,
		}, nil
	default:
		return domainautomation.HealObservation{}, fmt.Errorf("unsupported accepted heal fact kind %q", fact.Kind)
	}
}

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

func cloneHealStreak(streak domainautomation.HealStreak) domainautomation.HealStreak {
	streak.Contributions = append([]domainautomation.ContributingHealFact(nil), streak.Contributions...)
	return streak
}
