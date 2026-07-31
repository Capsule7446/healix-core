package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

const (
	healReviewDigestV1 = "heal-review-v1"

	CodeHealReviewIdentityConflict  fault.Code = "AUTOMATION_HEAL_REVIEW_IDENTITY_CONFLICT"
	CodeHealReviewDecisionConflict  fault.Code = "AUTOMATION_HEAL_REVIEW_DECISION_CONFLICT"
	CodeHealReviewAuthorityConflict fault.Code = "AUTOMATION_HEAL_REVIEW_AUTHORITY_CONFLICT"
	CodeHealReviewContractViolation fault.Code = "AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION"
)

func HealReviewIdentityConflictError() error {
	return newHealReviewFault(
		fault.Conflict,
		CodeHealReviewIdentityConflict,
		"heal review command conflicts with an existing request",
	)
}

func HealReviewDecisionConflictError() error {
	return newHealReviewFault(
		fault.FailedPrecondition,
		CodeHealReviewDecisionConflict,
		"heal candidate is no longer available for review",
	)
}

func HealReviewAuthorityConflictError() error {
	return newHealReviewFault(
		fault.Conflict,
		CodeHealReviewAuthorityConflict,
		"heal review authority changed before the operation completed",
	)
}

// classifyHealReviewCommand gives a malformed review command the caller-facing
// code that domain/automation already publishes for it, and lets an
// already-classified failure through — notably the persisted-revision codes,
// which name a different problem than the command's shape.
//
// It deliberately does not reuse AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION.
// That code is INTERNAL and, as its registry row says, describes a malformed
// adapter outcome. A caller-supplied command that fails validation is
// INVALID_ARGUMENT: the caller can fix it, and reporting it as INTERNAL would
// tell the host the opposite.
func classifyHealReviewCommand(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	err, constructionErr := fault.Wrap(
		cause,
		fault.InvalidArgument,
		domain.CodeHealCandidateReviewCommandInvalid,
		"heal candidate review command is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func healReviewContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(
		cause,
		fault.Internal,
		CodeHealReviewContractViolation,
		"heal review could not be completed",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func newHealReviewFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

type HealReviewDecision string

const (
	HealReviewApprove HealReviewDecision = "APPROVE"
	HealReviewReject  HealReviewDecision = "REJECT"
)

type HealReviewIntent struct {
	CommandID                 string
	RequestDigest             string
	Decision                  HealReviewDecision
	ElementTargetID           string
	BaseNodeVersionID         string
	CandidateHash             string
	ExpectedCandidateRevision domain.Revision
	ExpectedNodeRevision      domain.Revision
	ExpectedStreak            *domain.HealStreak
	ExpectedStreakDigest      string
	NextCandidate             domain.HealCandidate
	NextNode                  *domain.ElementTargetAggregate
	NextStreak                *domain.HealStreak
	ReviewedBy                string
	ReviewedAt                int64
}

type HealReviewStatus string

const (
	HealReviewApplied  HealReviewStatus = "APPLIED"
	HealReviewReplayed HealReviewStatus = "REPLAYED"
)

type HealReviewResult struct {
	Decision      HealReviewDecision
	Candidate     domain.HealCandidate
	ElementTarget *domain.ElementTargetAggregate
	Streak        *domain.HealStreak
}

type HealReviewOutcome struct {
	Status        HealReviewStatus
	CommandID     string
	RequestDigest string
	Result        HealReviewResult
}

type HealReviewTransaction interface {
	LookupHealReview(context.Context, string, string) (HealReviewOutcome, bool, error)
	CommitHealReview(context.Context, HealReviewIntent) (HealReviewOutcome, error)
}

type HealReviewRequest struct {
	CommandID                 string
	Decision                  HealReviewDecision
	ElementTargetID           string
	BaseNodeVersionID         string
	CandidateHash             string
	ExpectedCandidateRevision domain.Revision
	ExpectedNodeRevision      domain.Revision
}

func (request HealReviewRequest) Validate() error {
	return classifyHealReviewCommand(request.checkShape())
}

func (request HealReviewRequest) checkShape() error {
	if strings.TrimSpace(request.CommandID) == "" || strings.TrimSpace(request.ElementTargetID) == "" || strings.TrimSpace(request.BaseNodeVersionID) == "" || strings.TrimSpace(request.CandidateHash) == "" {
		return fmt.Errorf("heal review request requires command, node, base version, and candidate identity")
	}
	if request.Decision != HealReviewApprove && request.Decision != HealReviewReject {
		return fmt.Errorf("unsupported heal review decision %q", request.Decision)
	}
	// A zero expected revision in a REQUEST is caller-fixable: the caller reads
	// the authoritative revision and supplies it. Routing through
	// ValidatePersisted here produced the FAILED_PRECONDITION persisted-state
	// code, which the classifier then let through — telling the caller to repair
	// persisted state when the fix is to correct its own argument.
	if request.ExpectedCandidateRevision == 0 {
		return fmt.Errorf("heal review request requires the expected candidate revision")
	}
	if request.ExpectedNodeRevision == 0 {
		return fmt.Errorf("heal review request requires the expected node revision")
	}
	return nil
}

func HealReviewRequestIdentityDigest(request HealReviewRequest) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(struct {
		Schema  string
		Request HealReviewRequest
	}{Schema: healReviewDigestV1, Request: request})
	if err != nil {
		return "", fmt.Errorf("encode heal review request identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func HealReviewStreakDigest(streak domain.HealStreak) (string, error) {
	encoded, err := json.Marshal(streak)
	if err != nil {
		return "", fmt.Errorf("encode heal review streak authority: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// Validate classifies intent failures as INTERNAL contract violations, not as
// invalid arguments. An intent is built by the review service from adapter
// outcomes, or replayed from persistence — the external command caller cannot
// repair its transition invariants, so reporting them as caller-fixable told the
// host the opposite of the truth. An already-classified failure (the persisted
// revision codes) passes through unchanged.
func (intent HealReviewIntent) Validate() error {
	err := intent.checkShape()
	if err == nil {
		return nil
	}
	if _, classified := fault.CodeOf(err); classified {
		return err
	}
	return healReviewContractViolationError(err)
}

func (intent HealReviewIntent) checkShape() error {
	if strings.TrimSpace(intent.CommandID) == "" || strings.TrimSpace(intent.ElementTargetID) == "" || strings.TrimSpace(intent.BaseNodeVersionID) == "" || strings.TrimSpace(intent.CandidateHash) == "" {
		return fmt.Errorf("heal review intent requires command, node, base version, and candidate identity")
	}
	if strings.TrimSpace(intent.ReviewedBy) == "" || intent.ReviewedAt <= 0 {
		return fmt.Errorf("heal review intent requires trusted reviewer metadata")
	}
	if err := intent.ExpectedCandidateRevision.ValidatePersisted(); err != nil {
		// ValidatePersisted already returns AUTOMATION_PERSISTED_REVISION_INVALID.
		return err
	}
	if err := intent.ExpectedNodeRevision.ValidatePersisted(); err != nil {
		return err
	}
	if intent.NextCandidate.Hash != intent.CandidateHash || intent.NextCandidate.ElementTargetID != intent.ElementTargetID || intent.NextCandidate.BaseNodeVersionID != intent.BaseNodeVersionID || intent.NextCandidate.Revision != intent.ExpectedCandidateRevision+1 {
		return fmt.Errorf("heal review candidate transition does not match authority")
	}
	switch intent.Decision {
	case HealReviewApprove:
		if intent.NextCandidate.Status != domain.HealCandidatePromoted || intent.NextNode == nil || intent.ExpectedStreak != nil || intent.NextStreak != nil {
			return fmt.Errorf("approval requires promoted candidate and node only")
		}
		if intent.NextNode.ElementTarget.ID != intent.ElementTargetID || intent.NextNode.ElementTarget.Revision != intent.ExpectedNodeRevision+1 || intent.NextNode.Current.ID == intent.BaseNodeVersionID {
			return fmt.Errorf("approval node transition does not match authority")
		}
	case HealReviewReject:
		if intent.NextCandidate.Status != domain.HealCandidateRejected || intent.NextNode != nil || intent.ExpectedStreak == nil || intent.NextStreak == nil {
			return fmt.Errorf("rejection requires rejected candidate and streak transition only")
		}
		if strings.TrimSpace(intent.ExpectedStreakDigest) == "" || intent.ExpectedStreak.ElementTargetID != intent.ElementTargetID || intent.ExpectedStreak.BaseNodeVersionID != intent.BaseNodeVersionID || intent.ExpectedStreak.CandidateHash != intent.CandidateHash || intent.NextStreak.ElementTargetID != intent.ElementTargetID || intent.NextStreak.BaseNodeVersionID != intent.BaseNodeVersionID || intent.NextStreak.CandidateHash != intent.CandidateHash || intent.NextStreak.Disposition != domain.HealStreakRejected || intent.NextStreak.LastSequence <= intent.ExpectedStreak.LastSequence {
			return fmt.Errorf("rejection streak transition does not match authority")
		}
	default:
		return fmt.Errorf("unsupported heal review decision %q", intent.Decision)
	}
	return nil
}

func HealReviewRequestDigest(intent HealReviewIntent) (string, error) {
	if err := intent.Validate(); err != nil {
		return "", err
	}
	return HealReviewRequestIdentityDigest(HealReviewRequest{
		CommandID: intent.CommandID, Decision: intent.Decision, ElementTargetID: intent.ElementTargetID,
		BaseNodeVersionID: intent.BaseNodeVersionID, CandidateHash: intent.CandidateHash,
		ExpectedCandidateRevision: intent.ExpectedCandidateRevision, ExpectedNodeRevision: intent.ExpectedNodeRevision,
	})
}

func ValidateHealReviewIntentDigest(intent HealReviewIntent) error {
	digest, err := HealReviewRequestDigest(intent)
	if err != nil {
		return err
	}
	if intent.RequestDigest != digest {
		return HealReviewIdentityConflictError()
	}
	return nil
}

func cloneHealReviewIntent(intent HealReviewIntent) HealReviewIntent {
	result := intent
	result.NextCandidate = cloneHealCandidate(intent.NextCandidate)
	result.NextNode = cloneNodePointer(intent.NextNode)
	result.ExpectedStreak = cloneHealStreakPointer(intent.ExpectedStreak)
	result.NextStreak = cloneHealStreakPointer(intent.NextStreak)
	return result
}

func cloneHealReviewOutcome(outcome HealReviewOutcome) HealReviewOutcome {
	result := outcome
	result.Result.Candidate = cloneHealCandidate(outcome.Result.Candidate)
	result.Result.ElementTarget = cloneNodePointer(outcome.Result.ElementTarget)
	result.Result.Streak = cloneHealStreakPointer(outcome.Result.Streak)
	return result
}

func cloneHealCandidate(candidate domain.HealCandidate) domain.HealCandidate {
	result := candidate
	result.Selectors = append([]fingerprint.Selector(nil), candidate.Selectors...)
	result.Fingerprint = cloneApplicationFingerprint(candidate.Fingerprint)
	return result
}

func cloneNodePointer(node *domain.ElementTargetAggregate) *domain.ElementTargetAggregate {
	if node == nil {
		return nil
	}
	result := node.Clone()
	return &result
}

func cloneHealStreakPointer(streak *domain.HealStreak) *domain.HealStreak {
	if streak == nil {
		return nil
	}
	result := *streak
	result.Contributions = append([]domain.ContributingHealFact(nil), streak.Contributions...)
	return &result
}

func cloneApplicationFingerprint(value fingerprint.Fingerprint) fingerprint.Fingerprint {
	result := value
	result.Path = append([]string(nil), value.Path...)
	result.Framework = value.Framework.Clone()
	if value.Attributes != nil {
		result.Attributes = make(map[string]string, len(value.Attributes))
		for key, item := range value.Attributes {
			result.Attributes[key] = item
		}
	}
	return result
}
