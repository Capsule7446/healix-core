package automation

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type healCandidateRepositoryFake struct {
	candidate  domain.HealCandidate
	loadErr    error
	approveErr error
	rejectErr  error
	approval   HealApprovalCommit
	rejection  HealRejectionCommit
}

func (fake *healCandidateRepositoryFake) Load(context.Context, string) (domain.HealCandidate, error) {
	return fake.candidate, fake.loadErr
}
func (fake *healCandidateRepositoryFake) CommitApproval(_ context.Context, commit HealApprovalCommit) (domain.NodeAggregate, error) {
	fake.approval = commit
	return commit.Node, fake.approveErr
}
func (fake *healCandidateRepositoryFake) CommitRejection(_ context.Context, commit HealRejectionCommit) error {
	fake.rejection = commit
	return fake.rejectErr
}

type reviewerAuthorizerFake struct {
	id  string
	err error
}

func (fake reviewerAuthorizerFake) AuthorizeReviewer(context.Context) (string, error) {
	return fake.id, fake.err
}

type reviewClockFake int64

func (clock reviewClockFake) Now() int64 { return int64(clock) }

type candidateVerifierFake struct{ err error }

func (fake candidateVerifierFake) VerifyCandidate(context.Context, domain.HealCandidate) error {
	return fake.err
}

func healReviewFixture(t *testing.T) (*healCandidateRepositoryFake, *nodeRepositoryFake, domain.HealCandidateReviewCommand) {
	t.Helper()
	node := domain.Node{ID: "node", DisplayName: "Node", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.NodeVersion{ID: "node-v1", NodeID: "node", VersionNumber: 1,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
		Source:      domain.SourceManual, CreatedAt: 1}
	aggregate, err := domain.NewNode(node, version)
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.HealCandidate{Hash: "candidate", NodeID: "node", BaseNodeVersionID: "node-v1",
		Status: domain.HealCandidateAwaitingApproval, PageURL: "https://example.test", Origin: "https://example.test",
		Selectors: version.Selectors, Fingerprint: version.Fingerprint, Revision: 1}
	command := domain.HealCandidateReviewCommand{NodeID: "node", BaseNodeVersionID: "node-v1", CandidateHash: "candidate", PromotedVersionID: "node-v2", ExpectedNodeRevision: 1}
	return &healCandidateRepositoryFake{candidate: candidate}, &nodeRepositoryFake{current: aggregate}, command
}

func TestHealReviewServiceApprovesWithTrustedMetadata(t *testing.T) {
	candidates, nodes, command := healReviewFixture(t)
	service := NewHealReviewService(candidates, nodes, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{})
	result, err := service.Approve(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Current.ID != "node-v2" || candidates.approval.ExpectedCandidateRevision != 1 || candidates.approval.ExpectedNodeRevision != 1 {
		t.Fatalf("approval = %#v", candidates.approval)
	}
	if candidates.approval.ReviewedBy != "reviewer" || candidates.approval.ReviewedAt != 10 || candidates.approval.Status != domain.HealCandidatePromoted {
		t.Fatalf("trusted review metadata = %#v", candidates.approval)
	}
}

func TestHealReviewServiceRejectsWithoutLoadingNode(t *testing.T) {
	candidates, nodes, command := healReviewFixture(t)
	nodes.current = domain.NodeAggregate{}
	command.PromotedVersionID = ""
	service := NewHealReviewService(candidates, nodes, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{})
	if err := service.Reject(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if candidates.rejection.Candidate.Hash != "candidate" || candidates.rejection.ReviewedBy != "reviewer" {
		t.Fatalf("rejection = %#v", candidates.rejection)
	}
}

func TestHealReviewServiceRejectsUnverifiedCandidateAndUntrustedReviewer(t *testing.T) {
	candidates, nodes, command := healReviewFixture(t)
	verifyErr := errors.New("payload mismatch")
	service := NewHealReviewService(candidates, nodes, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{err: verifyErr})
	if _, err := service.Approve(context.Background(), command); !errors.Is(err, verifyErr) {
		t.Fatalf("verify error = %v", err)
	}
	if candidates.approval.Candidate.Hash != "" {
		t.Fatal("unverified candidate committed")
	}

	service = NewHealReviewService(candidates, nodes, reviewerAuthorizerFake{}, reviewClockFake(10), candidateVerifierFake{})
	if _, err := service.Approve(context.Background(), command); err == nil {
		t.Fatal("blank reviewer accepted")
	}
	service = NewHealReviewService(candidates, nodes, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(0), candidateVerifierFake{})
	if _, err := service.Approve(context.Background(), command); err == nil {
		t.Fatal("invalid trusted time accepted")
	}
}

func TestHealReviewServiceRejectsStaleNodeRevisionAndBase(t *testing.T) {
	candidates, nodes, command := healReviewFixture(t)
	service := NewHealReviewService(candidates, nodes, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{})
	command.ExpectedNodeRevision = 2
	if _, err := service.Approve(context.Background(), command); err == nil {
		t.Fatal("stale node revision accepted")
	}
	command.ExpectedNodeRevision = 1
	nodes.current.Node.CurrentVersionID = "other"
	if _, err := service.Approve(context.Background(), command); err == nil {
		t.Fatal("stale base version accepted")
	}
}
