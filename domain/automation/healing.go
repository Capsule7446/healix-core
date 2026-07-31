package automation

import (
	"fmt"
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// VersionMeta 是版本生命周期规则所需的最少信息。删除的版本仍然是序列的一部分，因此仍然会影响下一个版本号。
type VersionMeta struct {
	ID            string
	VersionNumber int
	DeletedAt     int64
}

const CodeVersionNumberExhausted fault.Code = "AUTOMATION_VERSION_NUMBER_EXHAUSTED"

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

func NextVersionNumber(existing []VersionMeta) (int, error) {
	maximum := 0
	for _, version := range existing {
		if version.VersionNumber <= 0 {
			return 0, fmt.Errorf("persisted version %q requires a positive version number", version.ID)
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

// HealDecisionBand 将确定性治疗者的置信门保留在工作区分类帐中。未知的观察结果将作为证据保留，但不会成为可审查的候选者。
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

// HealCandidateReviewCommand carries stable candidate identity and review metadata.
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

func (candidate HealCandidate) Validate() error {
	if err := candidate.validateIdentity(); err != nil {
		return err
	}
	if candidate.Status != HealCandidateAwaitingApproval {
		return fmt.Errorf("heal candidate %q is not awaiting approval", candidate.Hash)
	}
	return nil
}

func (candidate HealCandidate) ValidateReviewed() error {
	if err := candidate.validateIdentity(); err != nil {
		return err
	}
	if candidate.Status != HealCandidatePromoted && candidate.Status != HealCandidateRejected {
		return fmt.Errorf("heal candidate %q is not reviewed", candidate.Hash)
	}
	return nil
}

func (candidate HealCandidate) validateIdentity() error {
	if strings.TrimSpace(candidate.Hash) == "" || strings.TrimSpace(candidate.ElementTargetID) == "" ||
		strings.TrimSpace(candidate.BaseNodeVersionID) == "" {
		return fmt.Errorf("heal candidate requires identity")
	}
	return candidate.Revision.ValidatePersisted()
}

func (candidate HealCandidate) Review(status HealCandidateStatus) (HealCandidate, error) {
	if err := candidate.Validate(); err != nil {
		return HealCandidate{}, err
	}
	if status != HealCandidatePromoted && status != HealCandidateRejected {
		return HealCandidate{}, fmt.Errorf("unsupported reviewed heal candidate status %q", status)
	}
	next := candidate
	next.Selectors = append([]fingerprint.Selector(nil), candidate.Selectors...)
	next.Fingerprint = cloneFingerprint(candidate.Fingerprint)
	next.Status = status
	next.Revision++
	return next, nil
}

type HealCandidateReviewCommand struct {
	CommandID                 string
	ElementTargetID           string
	BaseNodeVersionID         string
	CandidateHash             string
	ExpectedCandidateRevision Revision
	ExpectedNodeRevision      Revision
}

func (command HealCandidateReviewCommand) Validate(approval HealApprovalStatus) error {
	if strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.ElementTargetID) == "" || strings.TrimSpace(command.BaseNodeVersionID) == "" ||
		strings.TrimSpace(command.CandidateHash) == "" {
		return fmt.Errorf("heal candidate review requires command, node, base version, and candidate hash")
	}
	if err := command.ExpectedCandidateRevision.ValidatePersisted(); err != nil {
		return fmt.Errorf("heal candidate review requires expected candidate revision: %w", err)
	}
	if err := command.ExpectedNodeRevision.ValidatePersisted(); err != nil {
		return fmt.Errorf("heal candidate review requires expected node revision: %w", err)
	}
	switch approval {
	case HealApprovalApproved, HealApprovalRejected:
	default:
		return fmt.Errorf("unsupported heal approval status %q", approval)
	}
	return nil
}

type HealStreakDisposition string

const (
	HealStreakObserving     HealStreakDisposition = "OBSERVING"
	HealStreakAutoPublish   HealStreakDisposition = "AUTO_PUBLISH"
	HealStreakAwaitApproval HealStreakDisposition = "AWAIT_APPROVAL"
	HealStreakReset         HealStreakDisposition = "RESET"
	HealStreakStale         HealStreakDisposition = "STALE"
	HealStreakRejected      HealStreakDisposition = "REJECTED"
)

type ContributingHealFact struct {
	FactID          string
	CommitID        string
	RunID           string
	ExecutionID     string
	StepExecutionID string
	Sequence        uint64
}

type HealObservation struct {
	FactID            string
	CommitID          string
	RunID             string
	ExecutionID       string
	StepExecutionID   string
	Sequence          uint64
	ElementTargetID   string
	BaseNodeVersionID string
	CandidateHash     string
	Band              HealDecisionBand
	Outcome           HealOutcome
	BaseIsCurrent     bool
}

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

type HealStreakDecision struct {
	Next HealStreak
}

func (streak HealStreak) Observe(observation HealObservation) (HealStreakDecision, error) {
	if err := streak.validate(); err != nil {
		return HealStreakDecision{}, err
	}
	if err := observation.validate(); err != nil {
		return HealStreakDecision{}, err
	}
	contribution := observation.contribution()
	for _, existing := range streak.consumedProvenance() {
		if existing == contribution {
			return HealStreakDecision{Next: streak.clone()}, nil
		}
		if existing.FactID == contribution.FactID || existing.CommitID == contribution.CommitID || existing.RunID == contribution.RunID || existing.Sequence == contribution.Sequence {
			return HealStreakDecision{}, fmt.Errorf("heal contribution replay conflicts with persisted provenance")
		}
	}
	if observation.Sequence <= streak.LastSequence {
		return HealStreakDecision{}, fmt.Errorf("heal observation sequence %d is not newer than %d", observation.Sequence, streak.LastSequence)
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

func (streak HealStreak) Validate() error {
	return streak.validate()
}

func (streak HealStreak) validate() error {
	provenance := streak.consumedProvenance()
	if err := validateHealContributions(provenance); err != nil {
		return fmt.Errorf("validate consumed heal observations: %w", err)
	}
	if len(provenance) > 0 && provenance[len(provenance)-1].Sequence > streak.LastSequence {
		return fmt.Errorf("consumed heal observation exceeds last sequence")
	}
	if !streak.Observing && streak.Disposition == "" && len(streak.Contributions) == 0 {
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

func (streak HealStreak) Reject(sequence uint64) (HealStreakDecision, error) {
	if err := streak.validate(); err != nil {
		return HealStreakDecision{}, err
	}
	if streak.Disposition != HealStreakAwaitApproval {
		return HealStreakDecision{}, fmt.Errorf("only an await-approval heal streak can be rejected")
	}
	if sequence == 0 || sequence <= streak.LastSequence {
		return HealStreakDecision{}, fmt.Errorf("heal rejection sequence %d is not newer than %d", sequence, streak.LastSequence)
	}
	next := streak.clone()
	next.LastSequence = sequence
	next.Observing = false
	next.Disposition = HealStreakRejected
	return HealStreakDecision{Next: next}, nil
}

func validateHealContributions(contributions []ContributingHealFact) error {
	for index, contribution := range contributions {
		if err := contribution.validate(); err != nil {
			return fmt.Errorf("heal contribution %d: %w", index, err)
		}
		if index > 0 && contribution.Sequence <= contributions[index-1].Sequence {
			return fmt.Errorf("heal contribution sequences must be strictly increasing")
		}
		for _, earlier := range contributions[:index] {
			if earlier.FactID == contribution.FactID || earlier.CommitID == contribution.CommitID || earlier.RunID == contribution.RunID || earlier.Sequence == contribution.Sequence {
				return fmt.Errorf("heal contribution identity is duplicated")
			}
		}
	}
	return nil
}

func (contribution ContributingHealFact) validate() error {
	if strings.TrimSpace(contribution.FactID) == "" || strings.TrimSpace(contribution.CommitID) == "" || strings.TrimSpace(contribution.RunID) == "" || strings.TrimSpace(contribution.ExecutionID) == "" || strings.TrimSpace(contribution.StepExecutionID) == "" || contribution.Sequence == 0 {
		return fmt.Errorf("heal contribution requires fact, commit, run, execution, step, and sequence identity")
	}
	return nil
}

func (observation HealObservation) validate() error {
	if strings.TrimSpace(observation.FactID) == "" || strings.TrimSpace(observation.CommitID) == "" || strings.TrimSpace(observation.RunID) == "" || strings.TrimSpace(observation.ExecutionID) == "" || strings.TrimSpace(observation.StepExecutionID) == "" || observation.Sequence == 0 || strings.TrimSpace(observation.ElementTargetID) == "" || strings.TrimSpace(observation.BaseNodeVersionID) == "" {
		return fmt.Errorf("heal observation requires fact, commit, run, execution, step, sequence, node, and base version identity")
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

func (observation HealObservation) contribution() ContributingHealFact {
	return ContributingHealFact{
		FactID: observation.FactID, CommitID: observation.CommitID, RunID: observation.RunID,
		ExecutionID: observation.ExecutionID, StepExecutionID: observation.StepExecutionID, Sequence: observation.Sequence,
	}
}

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

func newHealTerminal(observation HealObservation, disposition HealStreakDisposition) HealStreak {
	return HealStreak{
		ElementTargetID: observation.ElementTargetID, BaseNodeVersionID: observation.BaseNodeVersionID,
		LastSequence: observation.Sequence, Disposition: disposition,
		ConsumedObservations: []ContributingHealFact{observation.contribution()},
	}
}

func (streak HealStreak) matches(observation HealObservation) bool {
	return streak.ElementTargetID == observation.ElementTargetID && streak.BaseNodeVersionID == observation.BaseNodeVersionID &&
		streak.CandidateHash == observation.CandidateHash && streak.Band == observation.Band
}

func (streak HealStreak) isTerminal() bool {
	return streak.Disposition == HealStreakAutoPublish || streak.Disposition == HealStreakAwaitApproval ||
		streak.Disposition == HealStreakReset || streak.Disposition == HealStreakStale || streak.Disposition == HealStreakRejected
}

func (streak HealStreak) clone() HealStreak {
	streak.Contributions = append([]ContributingHealFact(nil), streak.Contributions...)
	streak.ConsumedObservations = append([]ContributingHealFact(nil), streak.ConsumedObservations...)
	return streak
}

func (streak HealStreak) consumedProvenance() []ContributingHealFact {
	if len(streak.ConsumedObservations) != 0 {
		return streak.ConsumedObservations
	}
	return streak.Contributions
}

func (streak HealStreak) withObservation(contribution ContributingHealFact) HealStreak {
	next := streak.clone()
	next.ConsumedObservations = append(next.consumedProvenance(), contribution)
	next.LastSequence = contribution.Sequence
	return next
}

func (streak HealStreak) withDisposition(disposition HealStreakDisposition) HealStreak {
	next := streak.clone()
	next.Observing = false
	next.Disposition = disposition
	return next
}
