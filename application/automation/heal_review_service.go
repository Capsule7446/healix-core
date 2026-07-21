package automation

import (
	"context"
	"fmt"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type HealApprovalCommit struct {
	Candidate                 domain.HealCandidate
	ExpectedCandidateRevision domain.Revision
	Node                      domain.NodeAggregate
	ExpectedNodeRevision      domain.Revision
	Status                    domain.HealCandidateStatus
	ReviewedBy                string
	ReviewedAt                int64
}

type HealRejectionCommit struct {
	Candidate                 domain.HealCandidate
	ExpectedCandidateRevision domain.Revision
	ReviewedBy                string
	ReviewedAt                int64
}

type HealCandidateRepository interface {
	Load(context.Context, string) (domain.HealCandidate, error)
	CommitApproval(context.Context, HealApprovalCommit) (domain.NodeAggregate, error)
	CommitRejection(context.Context, HealRejectionCommit) error
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

type HealReviewService struct {
	candidates HealCandidateRepository
	nodes      NodeRepository
	reviewers  ReviewerAuthorizer
	clock      ReviewClock
	verifier   CandidateVerifier
}

func NewHealReviewService(candidates HealCandidateRepository, nodes NodeRepository, reviewers ReviewerAuthorizer, clock ReviewClock, verifier CandidateVerifier) HealReviewService {
	return HealReviewService{candidates: candidates, nodes: nodes, reviewers: reviewers, clock: clock, verifier: verifier}
}

func (s HealReviewService) Approve(ctx context.Context, command domain.HealCandidateReviewCommand) (domain.NodeAggregate, error) {
	if err := command.Validate(domain.HealApprovalApproved); err != nil {
		return domain.NodeAggregate{}, err
	}
	candidate, node, err := s.loadForApproval(ctx, command)
	if err != nil {
		return domain.NodeAggregate{}, err
	}
	reviewer, reviewedAt, err := s.reviewMetadata(ctx)
	if err != nil {
		return domain.NodeAggregate{}, err
	}
	if node.Node.Revision != command.ExpectedNodeRevision {
		return domain.NodeAggregate{}, RevisionConflictError{AggregateKind: "node", ID: node.Node.ID, Expected: command.ExpectedNodeRevision, Actual: node.Node.Revision}
	}
	if node.Node.CurrentVersionID != candidate.BaseNodeVersionID {
		return domain.NodeAggregate{}, fmt.Errorf("heal candidate base version is no longer current")
	}
	promoted, err := node.PublishVersion(command.PromotedVersionID, candidate.PageURL, candidate.Origin,
		candidate.Selectors, candidate.Fingerprint, domain.SourceAutoHeal, reviewedAt)
	if err != nil {
		return domain.NodeAggregate{}, fmt.Errorf("publish healed node version: %w", err)
	}
	result, err := s.candidates.CommitApproval(ctx, HealApprovalCommit{
		Candidate: candidate, ExpectedCandidateRevision: candidate.Revision,
		Node: promoted, ExpectedNodeRevision: command.ExpectedNodeRevision,
		Status: domain.HealCandidatePromoted, ReviewedBy: reviewer, ReviewedAt: reviewedAt,
	})
	if err != nil {
		return domain.NodeAggregate{}, fmt.Errorf("commit heal approval: %w", err)
	}
	return result, nil
}

func (s HealReviewService) Reject(ctx context.Context, command domain.HealCandidateReviewCommand) error {
	if err := command.Validate(domain.HealApprovalRejected); err != nil {
		return err
	}
	candidate, err := s.loadCandidate(ctx, command)
	if err != nil {
		return err
	}
	reviewer, reviewedAt, err := s.reviewMetadata(ctx)
	if err != nil {
		return err
	}
	if err := s.candidates.CommitRejection(ctx, HealRejectionCommit{
		Candidate: candidate, ExpectedCandidateRevision: candidate.Revision,
		ReviewedBy: reviewer, ReviewedAt: reviewedAt,
	}); err != nil {
		return fmt.Errorf("commit heal rejection: %w", err)
	}
	return nil
}

func (s HealReviewService) loadCandidate(ctx context.Context, command domain.HealCandidateReviewCommand) (domain.HealCandidate, error) {
	candidate, err := s.candidates.Load(ctx, command.CandidateHash)
	if err != nil {
		return domain.HealCandidate{}, fmt.Errorf("load heal candidate: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return domain.HealCandidate{}, err
	}
	if candidate.Hash != command.CandidateHash || candidate.NodeID != command.NodeID || candidate.BaseNodeVersionID != command.BaseNodeVersionID {
		return domain.HealCandidate{}, fmt.Errorf("heal candidate identity mismatch")
	}
	if err := s.verifier.VerifyCandidate(ctx, candidate); err != nil {
		return domain.HealCandidate{}, fmt.Errorf("verify heal candidate: %w", err)
	}
	return candidate, nil
}

func (s HealReviewService) loadForApproval(ctx context.Context, command domain.HealCandidateReviewCommand) (domain.HealCandidate, domain.NodeAggregate, error) {
	candidate, err := s.loadCandidate(ctx, command)
	if err != nil {
		return domain.HealCandidate{}, domain.NodeAggregate{}, err
	}
	node, err := s.nodes.Load(ctx, command.NodeID)
	if err != nil {
		return domain.HealCandidate{}, domain.NodeAggregate{}, fmt.Errorf("load node: %w", err)
	}
	return candidate, node, nil
}

func (s HealReviewService) reviewMetadata(ctx context.Context) (string, int64, error) {
	reviewer, err := s.reviewers.AuthorizeReviewer(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("authorize heal reviewer: %w", err)
	}
	if reviewer == "" {
		return "", 0, fmt.Errorf("authorized heal reviewer is required")
	}
	reviewedAt := s.clock.Now()
	if reviewedAt <= 0 {
		return "", 0, fmt.Errorf("trusted review time must be positive")
	}
	return reviewer, reviewedAt, nil
}
