package automation

import (
	"context"
	"fmt"
	"reflect"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type HealReviewSource interface {
	LoadCandidate(context.Context, string, string, string) (domain.HealCandidate, error)
	LoadStreak(context.Context, string, string, string) (domain.HealStreak, error)
}

type ReviewerAuthorizer interface {
	AuthorizeReviewer(context.Context) (string, error)
}

type ReviewClock interface {
	Now() int64
}

type CandidateVerifier interface {
	VerifyCandidate(context.Context, domain.HealCandidate) error
}

type HealReviewIdentityProvider interface {
	NewNodeVersionID(context.Context, string) (string, error)
	NextRejectionSequence(context.Context, string) (uint64, error)
}

type HealReviewService struct {
	source      HealReviewSource
	nodes       NodeRepository
	transaction HealReviewTransaction
	reviewers   ReviewerAuthorizer
	clock       ReviewClock
	verifier    CandidateVerifier
	identities  HealReviewIdentityProvider
}

func NewHealReviewService(source HealReviewSource, nodes NodeRepository, transaction HealReviewTransaction, reviewers ReviewerAuthorizer, clock ReviewClock, verifier CandidateVerifier, identities HealReviewIdentityProvider) (HealReviewService, error) {
	for name, dependency := range map[string]any{
		"heal review source":            source,
		"node repository":               nodes,
		"heal review transaction":       transaction,
		"reviewer authorizer":           reviewers,
		"review clock":                  clock,
		"candidate verifier":            verifier,
		"heal review identity provider": identities,
	} {
		if isNilHealReviewDependency(dependency) {
			return HealReviewService{}, fmt.Errorf("%s is required", name)
		}
	}
	return HealReviewService{source: source, nodes: nodes, transaction: transaction, reviewers: reviewers, clock: clock, verifier: verifier, identities: identities}, nil
}

func isNilHealReviewDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (s HealReviewService) Approve(ctx context.Context, command domain.HealCandidateReviewCommand) (domain.ElementTargetAggregate, error) {
	reviewer, err := s.authorizeReviewer(ctx)
	if err != nil {
		return domain.ElementTargetAggregate{}, err
	}
	replay, found, err := s.lookup(ctx, command, HealReviewApprove)
	if err != nil {
		return domain.ElementTargetAggregate{}, err
	}
	if found {
		if replay.Result.ElementTarget == nil {
			return domain.ElementTargetAggregate{}, fmt.Errorf("%w: replayed approval requires node", ErrHealReviewContract)
		}
		return replay.Result.ElementTarget.Clone(), nil
	}
	intent, err := s.prepare(ctx, command, HealReviewApprove, reviewer, s.clock.Now())
	if err != nil {
		return domain.ElementTargetAggregate{}, err
	}
	versionID, err := s.identities.NewNodeVersionID(ctx, command.CommandID)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("allocate heal node version identity: %w", err)
	}
	candidate := intent.NextCandidate
	node, err := intent.NextNodeValue().PublishVersion(versionID, candidate.PageURL, candidate.Origin, candidate.Selectors, candidate.Fingerprint, domain.SourceAutoHeal, intent.ReviewedAt)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("publish approved heal candidate: %w", err)
	}
	intent.NextNode = &node
	intent.RequestDigest, err = HealReviewRequestDigest(intent)
	if err != nil {
		return domain.ElementTargetAggregate{}, err
	}
	outcome, err := s.transaction.CommitHealReview(ctx, intent)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("commit heal review: %w", err)
	}
	if err := validateHealReviewOutcome(intent, outcome); err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("%w: %v", ErrHealReviewContract, err)
	}
	if outcome.Result.ElementTarget == nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("%w: approval result requires node", ErrHealReviewContract)
	}
	return outcome.Result.ElementTarget.Clone(), nil
}

func (s HealReviewService) Reject(ctx context.Context, command domain.HealCandidateReviewCommand) error {
	reviewer, err := s.authorizeReviewer(ctx)
	if err != nil {
		return err
	}
	_, found, err := s.lookup(ctx, command, HealReviewReject)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	intent, err := s.prepare(ctx, command, HealReviewReject, reviewer, s.clock.Now())
	if err != nil {
		return err
	}
	streak, err := s.source.LoadStreak(ctx, command.ElementTargetID, command.BaseNodeVersionID, command.CandidateHash)
	if err != nil {
		return fmt.Errorf("load heal streak: %w", err)
	}
	sequence, err := s.identities.NextRejectionSequence(ctx, command.CommandID)
	if err != nil {
		return fmt.Errorf("allocate heal rejection sequence: %w", err)
	}
	next, err := streak.Reject(sequence)
	if err != nil {
		return err
	}
	intent.NextNode = nil
	intent.ExpectedStreak = &streak
	intent.ExpectedStreakDigest, err = HealReviewStreakDigest(streak)
	if err != nil {
		return err
	}
	intent.NextStreak = &next.Next
	intent.RequestDigest, err = HealReviewRequestDigest(intent)
	if err != nil {
		return err
	}
	outcome, err := s.transaction.CommitHealReview(ctx, intent)
	if err != nil {
		return fmt.Errorf("commit heal review: %w", err)
	}
	return validateHealReviewOutcome(intent, outcome)
}

func (s HealReviewService) lookup(ctx context.Context, command domain.HealCandidateReviewCommand, decision HealReviewDecision) (HealReviewOutcome, bool, error) {
	request := HealReviewRequest{CommandID: command.CommandID, Decision: decision, ElementTargetID: command.ElementTargetID, BaseNodeVersionID: command.BaseNodeVersionID, CandidateHash: command.CandidateHash, ExpectedCandidateRevision: command.ExpectedCandidateRevision, ExpectedNodeRevision: command.ExpectedNodeRevision}
	digest, err := HealReviewRequestIdentityDigest(request)
	if err != nil {
		return HealReviewOutcome{}, false, err
	}
	outcome, found, err := s.transaction.LookupHealReview(ctx, command.CommandID, digest)
	if err != nil {
		return HealReviewOutcome{}, false, fmt.Errorf("lookup heal review: %w", err)
	}
	if !found {
		return HealReviewOutcome{}, false, nil
	}
	if err := validateHealReviewReplay(request, digest, outcome); err != nil {
		return HealReviewOutcome{}, false, fmt.Errorf("%w: %v", ErrHealReviewContract, err)
	}
	return cloneHealReviewOutcome(outcome), true, nil
}

func validateHealReviewReplay(request HealReviewRequest, digest string, outcome HealReviewOutcome) error {
	if outcome.Status != HealReviewApplied && outcome.Status != HealReviewReplayed {
		return fmt.Errorf("unsupported replay status %q", outcome.Status)
	}
	if outcome.CommandID != request.CommandID || outcome.RequestDigest != digest || outcome.Result.Decision != request.Decision {
		return fmt.Errorf("replay identity does not match request")
	}
	candidate := outcome.Result.Candidate
	if err := candidate.ValidateReviewed(); err != nil {
		return fmt.Errorf("validate replay candidate: %w", err)
	}
	if candidate.Hash != request.CandidateHash || candidate.ElementTargetID != request.ElementTargetID || candidate.BaseNodeVersionID != request.BaseNodeVersionID || candidate.Revision != request.ExpectedCandidateRevision+1 {
		return fmt.Errorf("replay candidate does not match request")
	}
	switch request.Decision {
	case HealReviewApprove:
		if outcome.Result.ElementTarget == nil || outcome.Result.Streak != nil || candidate.Status != domain.HealCandidatePromoted {
			return fmt.Errorf("approval replay has invalid decision shape")
		}
		node := outcome.Result.ElementTarget
		if err := node.ValidateLoadedHistory(); err != nil {
			return fmt.Errorf("validate replay node: %w", err)
		}
		if node.ElementTarget.ID != request.ElementTargetID || node.ElementTarget.Revision != request.ExpectedNodeRevision+1 || node.ElementTarget.CurrentVersionID != node.Current.ID || node.Current.ID == request.BaseNodeVersionID {
			return fmt.Errorf("approval replay node does not match request")
		}
		if node.Current.ElementTargetID != request.ElementTargetID || node.Current.Source != domain.SourceAutoHeal || node.Current.PageURL != candidate.PageURL || node.Current.Origin != candidate.Origin || !reflect.DeepEqual(node.Current.Selectors, candidate.Selectors) || !reflect.DeepEqual(node.Current.Fingerprint, candidate.Fingerprint) {
			return fmt.Errorf("approval replay version does not match candidate")
		}
	case HealReviewReject:
		if outcome.Result.ElementTarget != nil || outcome.Result.Streak == nil || candidate.Status != domain.HealCandidateRejected {
			return fmt.Errorf("rejection replay has invalid decision shape")
		}
		streak := outcome.Result.Streak
		if err := streak.Validate(); err != nil {
			return fmt.Errorf("validate replay streak: %w", err)
		}
		if streak.ElementTargetID != request.ElementTargetID || streak.BaseNodeVersionID != request.BaseNodeVersionID || streak.CandidateHash != request.CandidateHash || streak.Disposition != domain.HealStreakRejected {
			return fmt.Errorf("rejection replay streak does not match request")
		}
	}
	return nil
}

func (s HealReviewService) prepare(ctx context.Context, command domain.HealCandidateReviewCommand, decision HealReviewDecision, reviewer string, reviewedAt int64) (HealReviewIntent, error) {
	approval := domain.HealApprovalApproved
	status := domain.HealCandidatePromoted
	if decision == HealReviewReject {
		approval = domain.HealApprovalRejected
		status = domain.HealCandidateRejected
	}
	if err := command.Validate(approval); err != nil {
		return HealReviewIntent{}, err
	}
	candidate, err := s.source.LoadCandidate(ctx, command.ElementTargetID, command.BaseNodeVersionID, command.CandidateHash)
	if err != nil {
		return HealReviewIntent{}, fmt.Errorf("load heal candidate: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return HealReviewIntent{}, err
	}
	if candidate.ElementTargetID != command.ElementTargetID || candidate.BaseNodeVersionID != command.BaseNodeVersionID || candidate.Hash != command.CandidateHash {
		return HealReviewIntent{}, ErrHealReviewCASConflict
	}
	if candidate.Revision != command.ExpectedCandidateRevision {
		return HealReviewIntent{}, AutomationRevisionConflictError()
	}
	if err := s.verifier.VerifyCandidate(ctx, candidate); err != nil {
		return HealReviewIntent{}, fmt.Errorf("verify heal candidate: %w", err)
	}
	node, err := s.nodes.Load(ctx, command.ElementTargetID)
	if err != nil {
		return HealReviewIntent{}, fmt.Errorf("load node: %w", err)
	}
	if node.ElementTarget.ID != command.ElementTargetID {
		return HealReviewIntent{}, ErrHealReviewCASConflict
	}
	if node.ElementTarget.Revision != command.ExpectedNodeRevision {
		return HealReviewIntent{}, AutomationRevisionConflictError()
	}
	if node.ElementTarget.CurrentVersionID != command.BaseNodeVersionID {
		return HealReviewIntent{}, HealCandidateStaleBaseError()
	}
	nextCandidate, err := candidate.Review(status)
	if err != nil {
		return HealReviewIntent{}, err
	}
	if reviewedAt <= 0 {
		return HealReviewIntent{}, fmt.Errorf("trusted review time must be positive")
	}
	return HealReviewIntent{CommandID: command.CommandID, Decision: decision, ElementTargetID: command.ElementTargetID, BaseNodeVersionID: command.BaseNodeVersionID, CandidateHash: command.CandidateHash, ExpectedCandidateRevision: command.ExpectedCandidateRevision, ExpectedNodeRevision: command.ExpectedNodeRevision, NextCandidate: nextCandidate, NextNode: &node, ReviewedBy: reviewer, ReviewedAt: reviewedAt}, nil
}

func (intent HealReviewIntent) NextNodeValue() domain.ElementTargetAggregate {
	if intent.NextNode == nil {
		return domain.ElementTargetAggregate{}
	}
	return intent.NextNode.Clone()
}

func validateHealReviewOutcome(intent HealReviewIntent, outcome HealReviewOutcome) error {
	if outcome.Status != HealReviewApplied && outcome.Status != HealReviewReplayed {
		return fmt.Errorf("unsupported status %q", outcome.Status)
	}
	if outcome.Status == HealReviewReplayed {
		request := HealReviewRequest{
			CommandID:                 intent.CommandID,
			Decision:                  intent.Decision,
			ElementTargetID:           intent.ElementTargetID,
			BaseNodeVersionID:         intent.BaseNodeVersionID,
			CandidateHash:             intent.CandidateHash,
			ExpectedCandidateRevision: intent.ExpectedCandidateRevision,
			ExpectedNodeRevision:      intent.ExpectedNodeRevision,
		}
		if err := validateHealReviewReplay(request, intent.RequestDigest, outcome); err != nil {
			return fmt.Errorf("validate concurrent heal review replay: %w", err)
		}
		return nil
	}
	if outcome.CommandID != intent.CommandID || outcome.RequestDigest != intent.RequestDigest || outcome.Result.Decision != intent.Decision {
		return fmt.Errorf("outcome identity does not match intent")
	}
	if !reflect.DeepEqual(cloneHealCandidate(outcome.Result.Candidate), cloneHealCandidate(intent.NextCandidate)) {
		return fmt.Errorf("outcome candidate does not match intent")
	}
	if !reflect.DeepEqual(cloneNodePointer(outcome.Result.ElementTarget), cloneNodePointer(intent.NextNode)) {
		return fmt.Errorf("outcome node does not match intent")
	}
	if !reflect.DeepEqual(cloneHealStreakPointer(outcome.Result.Streak), cloneHealStreakPointer(intent.NextStreak)) {
		return fmt.Errorf("outcome streak does not match intent")
	}
	return nil
}

func (s HealReviewService) authorizeReviewer(ctx context.Context) (string, error) {
	reviewer, err := s.reviewers.AuthorizeReviewer(ctx)
	if err != nil {
		return "", fmt.Errorf("authorize heal reviewer: %w", err)
	}
	if reviewer == "" {
		return "", fmt.Errorf("authorized heal reviewer is required")
	}
	return reviewer, nil
}
