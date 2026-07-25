package execution

import (
	"fmt"
	"slices"
	"strings"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/evidence"
)

type HealGovernanceKey struct {
	NodeID            string
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
		return HealGovernanceDecision{}, err
	}
	previous := cloneHealStreak(plan.Snapshot.Streak)
	transition, err := previous.Observe(observation)
	if err != nil {
		return HealGovernanceDecision{}, fmt.Errorf("observe accepted heal fact: %w", err)
	}
	decision := HealGovernanceDecision{
		Key: plan.Snapshot.Key, FactID: plan.Fact.FactID, Sequence: plan.Fact.Sequence,
		ExpectedRevision: plan.Snapshot.Revision, NextStreak: cloneHealStreak(transition.Next),
	}
	decision.Effect = terminalEffectForTransition(previous, transition.Next)
	return decision, nil
}

func validateHealGovernancePlan(plan HealGovernancePlan) error {
	if strings.TrimSpace(plan.Snapshot.Key.NodeID) == "" || strings.TrimSpace(plan.Snapshot.Key.BaseNodeVersionID) == "" {
		return fmt.Errorf("heal governance snapshot requires node and base identity")
	}
	if strings.TrimSpace(plan.Snapshot.CurrentNodeVersionID) == "" {
		return fmt.Errorf("heal governance snapshot requires current node version identity")
	}
	if err := plan.Snapshot.Revision.ValidatePersisted(); err != nil {
		return fmt.Errorf("heal governance snapshot revision: %w", err)
	}
	if strings.TrimSpace(plan.Fact.FactID) == "" || strings.TrimSpace(plan.Fact.CommitID) == "" || strings.TrimSpace(plan.Fact.RunID) == "" || plan.Fact.Sequence == 0 {
		return fmt.Errorf("accepted heal fact requires fact, commit, run, and sequence identity")
	}
	streak := plan.Snapshot.Streak
	if streak.Disposition != "" && (streak.NodeID != plan.Snapshot.Key.NodeID || streak.BaseNodeVersionID != plan.Snapshot.Key.BaseNodeVersionID) {
		return fmt.Errorf("heal governance streak does not match snapshot key")
	}
	if err := validateHealCandidateStatus(plan.Snapshot.CandidateStatus, streak.Disposition); err != nil {
		return err
	}
	return validateExistingHealEffect(plan.Snapshot.ExistingTerminalEffect, streak)
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

func validateExistingHealEffect(effect *HealTerminalEffectSnapshot, streak domainautomation.HealStreak) error {
	if effect == nil {
		return nil
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
		if observation.ID != fact.FactID || observation.RunID != fact.RunID || observation.NodeID != snapshot.Key.NodeID || observation.BaseNodeVersionID != snapshot.Key.BaseNodeVersionID {
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
			NodeID: observation.NodeID, BaseNodeVersionID: observation.BaseNodeVersionID,
			CandidateHash: observation.CandidateHash, Band: band, Outcome: outcome, BaseIsCurrent: baseIsCurrent,
		}, nil
	case HealAcceptedReset:
		if fact.Reset == nil || fact.Observation != nil {
			return domainautomation.HealObservation{}, fmt.Errorf("accepted reset requires exactly one reset payload")
		}
		reset := *fact.Reset
		if reset.NodeID != snapshot.Key.NodeID || reset.BaseNodeVersionID != snapshot.Key.BaseNodeVersionID {
			return domainautomation.HealObservation{}, fmt.Errorf("accepted reset does not match governance identity")
		}
		return domainautomation.HealObservation{
			FactID: fact.FactID, CommitID: fact.CommitID, RunID: fact.RunID,
			ExecutionID: reset.ExecutionID, StepExecutionID: reset.StepExecutionID, Sequence: fact.Sequence,
			NodeID: reset.NodeID, BaseNodeVersionID: reset.BaseNodeVersionID,
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
