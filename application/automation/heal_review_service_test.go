package automation

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type healReviewSourceFake struct {
	candidate domain.HealCandidate
	streak    domain.HealStreak
}

func (fake *healReviewSourceFake) LoadCandidate(context.Context, string, string, string) (domain.HealCandidate, error) {
	return fake.candidate, nil
}

func (fake *healReviewSourceFake) LoadStreak(context.Context, string, string, string) (domain.HealStreak, error) {
	return fake.streak, nil
}

type healReviewTransactionFake struct {
	intent      HealReviewIntent
	outcome     *HealReviewOutcome
	replay      *HealReviewOutcome
	err         error
	lookupCalls int
}

func (fake *healReviewTransactionFake) LookupHealReview(_ context.Context, _ string, _ string) (HealReviewOutcome, bool, error) {
	fake.lookupCalls++
	if fake.err != nil {
		return HealReviewOutcome{}, false, fake.err
	}
	if fake.replay == nil {
		return HealReviewOutcome{}, false, nil
	}
	return cloneHealReviewOutcome(*fake.replay), true, nil
}

func (fake *healReviewTransactionFake) CommitHealReview(_ context.Context, intent HealReviewIntent) (HealReviewOutcome, error) {
	fake.intent = cloneHealReviewIntent(intent)
	if fake.err != nil {
		return HealReviewOutcome{}, fake.err
	}
	if fake.outcome != nil {
		return cloneHealReviewOutcome(*fake.outcome), nil
	}
	return HealReviewOutcome{
		Status: HealReviewApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest,
		Result: HealReviewResult{Decision: intent.Decision, Candidate: intent.NextCandidate, ElementTarget: intent.NextNode, Streak: intent.NextStreak},
	}, nil
}

type reviewerAuthorizerFake struct {
	id    string
	err   error
	calls *int
}

func (fake reviewerAuthorizerFake) AuthorizeReviewer(context.Context) (string, error) {
	if fake.calls != nil {
		*fake.calls++
	}
	return fake.id, fake.err
}

type reviewClockFake int64

func (clock reviewClockFake) Now() int64 { return int64(clock) }

type candidateVerifierFake struct{ err error }

func (fake candidateVerifierFake) VerifyCandidate(context.Context, domain.HealCandidate) error {
	return fake.err
}

type healReviewIdentityFake struct {
	versionID string
	sequence  uint64
}

func (fake healReviewIdentityFake) NewNodeVersionID(context.Context, string) (string, error) {
	return fake.versionID, nil
}

func (fake healReviewIdentityFake) NextRejectionSequence(context.Context, string) (uint64, error) {
	return fake.sequence, nil
}

func TestNewHealReviewServiceRejectsNilDependencies(t *testing.T) {
	validSource := &healReviewSourceFake{}
	validNodes := &nodeRepositoryFake{}
	validTransaction := &healReviewTransactionFake{}
	validReviewers := reviewerAuthorizerFake{}
	validClock := reviewClockFake(1)
	validVerifier := candidateVerifierFake{}
	validIdentities := healReviewIdentityFake{}

	tests := []struct {
		name        string
		source      HealReviewSource
		nodes       NodeRepository
		transaction HealReviewTransaction
		reviewers   ReviewerAuthorizer
		clock       ReviewClock
		verifier    CandidateVerifier
		identities  HealReviewIdentityProvider
	}{
		{name: "nil source", nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "typed nil source", source: (*healReviewSourceFake)(nil), nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "nil nodes", source: validSource, transaction: validTransaction, reviewers: validReviewers, clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "typed nil nodes", source: validSource, nodes: (*nodeRepositoryFake)(nil), transaction: validTransaction, reviewers: validReviewers, clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "nil transaction", source: validSource, nodes: validNodes, reviewers: validReviewers, clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "typed nil transaction", source: validSource, nodes: validNodes, transaction: (*healReviewTransactionFake)(nil), reviewers: validReviewers, clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "nil reviewers", source: validSource, nodes: validNodes, transaction: validTransaction, clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "typed nil reviewers", source: validSource, nodes: validNodes, transaction: validTransaction, reviewers: (*reviewerAuthorizerFake)(nil), clock: validClock, verifier: validVerifier, identities: validIdentities},
		{name: "nil clock", source: validSource, nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, verifier: validVerifier, identities: validIdentities},
		{name: "typed nil clock", source: validSource, nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, clock: (*reviewClockFake)(nil), verifier: validVerifier, identities: validIdentities},
		{name: "nil verifier", source: validSource, nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, clock: validClock, identities: validIdentities},
		{name: "typed nil verifier", source: validSource, nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, clock: validClock, verifier: (*candidateVerifierFake)(nil), identities: validIdentities},
		{name: "nil identities", source: validSource, nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, clock: validClock, verifier: validVerifier},
		{name: "typed nil identities", source: validSource, nodes: validNodes, transaction: validTransaction, reviewers: validReviewers, clock: validClock, verifier: validVerifier, identities: (*healReviewIdentityFake)(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewHealReviewService(test.source, test.nodes, test.transaction, test.reviewers, test.clock, test.verifier, test.identities); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func healReviewFixture(t *testing.T) (*healReviewSourceFake, *nodeRepositoryFake, *healReviewTransactionFake, domain.HealCandidateReviewCommand) {
	t.Helper()
	node := domain.ElementTarget{ID: "node", DisplayName: "ElementTarget", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.ElementTargetVersion{ID: "node-v1", ElementTargetID: "node", VersionNumber: 1,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
		Source:      domain.SourceManual, CreatedAt: 1}
	aggregate, err := domain.NewElementTarget(node, version)
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.HealCandidate{Hash: "candidate", ElementTargetID: "node", BaseNodeVersionID: "node-v1",
		Status: domain.HealCandidateAwaitingApproval, PageURL: "https://example.test", Origin: "https://example.test",
		Selectors: version.Selectors, Fingerprint: version.Fingerprint, Revision: 1}
	contributions := []domain.ContributingHealFact{
		{FactID: "f1", CommitID: "c1", RunID: "r1", ExecutionID: "e1", StepExecutionID: "s1", Sequence: 1},
		{FactID: "f2", CommitID: "c2", RunID: "r2", ExecutionID: "e2", StepExecutionID: "s2", Sequence: 2},
		{FactID: "f3", CommitID: "c3", RunID: "r3", ExecutionID: "e3", StepExecutionID: "s3", Sequence: 3},
	}
	streak := domain.HealStreak{ElementTargetID: "node", BaseNodeVersionID: "node-v1", CandidateHash: "candidate", Band: domain.HealDecisionBandBelowCap, Contributions: contributions, LastSequence: 3, Disposition: domain.HealStreakAwaitApproval}
	command := domain.HealCandidateReviewCommand{CommandID: "review-1", ElementTargetID: "node", BaseNodeVersionID: "node-v1", CandidateHash: "candidate", ExpectedCandidateRevision: 1, ExpectedNodeRevision: 1}
	return &healReviewSourceFake{candidate: candidate, streak: streak}, &nodeRepositoryFake{current: aggregate}, &healReviewTransactionFake{}, command
}

func newReviewService(t *testing.T, source HealReviewSource, nodes NodeRepository, transaction HealReviewTransaction) HealReviewService {
	t.Helper()
	service, err := NewHealReviewService(source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, healReviewIdentityFake{versionID: "node-v2", sequence: 4})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestHealReviewServiceApprovesWithOneAtomicIntent(t *testing.T) {
	source, nodes, transaction, command := healReviewFixture(t)
	result, err := newReviewService(t, source, nodes, transaction).Approve(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	intent := transaction.intent
	if result.Current.ID != "node-v2" || intent.Decision != HealReviewApprove || intent.NextNode == nil || intent.NextStreak != nil || intent.ExpectedStreak != nil {
		t.Fatalf("approval intent = %#v", intent)
	}
	if intent.NextCandidate.Status != domain.HealCandidatePromoted || intent.NextCandidate.Revision != 2 || intent.ReviewedBy != "reviewer" || intent.ReviewedAt != 10 {
		t.Fatalf("approval transition = %#v", intent)
	}
	if err := ValidateHealReviewIntentDigest(intent); err != nil {
		t.Fatalf("intent digest: %v", err)
	}
}

func TestHealReviewServiceRejectsWithCandidateAndStreakOnly(t *testing.T) {
	source, nodes, transaction, command := healReviewFixture(t)
	if err := newReviewService(t, source, nodes, transaction).Reject(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	intent := transaction.intent
	if intent.Decision != HealReviewReject || intent.NextNode != nil || intent.ExpectedStreak == nil || intent.NextStreak == nil {
		t.Fatalf("rejection intent = %#v", intent)
	}
	if intent.NextCandidate.Status != domain.HealCandidateRejected || intent.NextStreak.Disposition != domain.HealStreakRejected || intent.NextStreak.LastSequence != 4 {
		t.Fatalf("rejection transition = %#v", intent)
	}
}

func TestHealReviewServiceRejectsStaleStateBeforeCommit(t *testing.T) {
	source, nodes, transaction, command := healReviewFixture(t)
	command.ExpectedCandidateRevision = 2
	if err := newReviewService(t, source, nodes, transaction).Reject(context.Background(), command); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("candidate conflict = %v", err)
	}
	if transaction.intent.CommandID != "" {
		t.Fatal("stale candidate was committed")
	}
	command.ExpectedCandidateRevision = 1
	nodes.current.ElementTarget.CurrentVersionID = "other"
	if err := newReviewService(t, source, nodes, transaction).Reject(context.Background(), command); !fault.IsCode(err, CodeHealCandidateStaleBase) {
		t.Fatalf("base conflict = %v", err)
	}
}

func TestHealReviewRequestDigestUsesImmutableCommandIdentity(t *testing.T) {
	source, nodes, _, command := healReviewFixture(t)
	base := HealReviewIntent{CommandID: command.CommandID, Decision: HealReviewApprove, ElementTargetID: command.ElementTargetID, BaseNodeVersionID: command.BaseNodeVersionID, CandidateHash: command.CandidateHash, ExpectedCandidateRevision: command.ExpectedCandidateRevision, ExpectedNodeRevision: command.ExpectedNodeRevision, NextCandidate: source.candidate, NextNode: &nodes.current, ReviewedBy: "reviewer-a", ReviewedAt: 10}
	base.NextCandidate.Status = domain.HealCandidatePromoted
	base.NextCandidate.Revision++
	published, err := nodes.current.PublishVersion("version-a", base.NextCandidate.PageURL, base.NextCandidate.Origin, base.NextCandidate.Selectors, base.NextCandidate.Fingerprint, domain.SourceAutoHeal, 10)
	if err != nil {
		t.Fatal(err)
	}
	base.NextNode = &published
	first, err := HealReviewRequestDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	changed := cloneHealReviewIntent(base)
	changed.ReviewedBy = "reviewer-b"
	changed.ReviewedAt = 99
	changed.NextNode.Current.ID = "version-b"
	changed.NextNode.ElementTarget.CurrentVersionID = "version-b"
	second, err := HealReviewRequestDigest(changed)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("trusted/generated fields changed replay identity: %q != %q", first, second)
	}
	request := HealReviewRequest{CommandID: changed.CommandID, Decision: changed.Decision, ElementTargetID: changed.ElementTargetID, BaseNodeVersionID: changed.BaseNodeVersionID, CandidateHash: changed.CandidateHash, ExpectedCandidateRevision: changed.ExpectedCandidateRevision, ExpectedNodeRevision: changed.ExpectedNodeRevision + 1}
	third, err := HealReviewRequestIdentityDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("expected revision did not change replay identity")
	}
}

func TestHealReviewServiceReplaysBeforeLoadingMutableState(t *testing.T) {
	source, nodes, transaction, command := healReviewFixture(t)
	digest, err := HealReviewRequestIdentityDigest(HealReviewRequest{CommandID: command.CommandID, Decision: HealReviewApprove, ElementTargetID: command.ElementTargetID, BaseNodeVersionID: command.BaseNodeVersionID, CandidateHash: command.CandidateHash, ExpectedCandidateRevision: command.ExpectedCandidateRevision, ExpectedNodeRevision: command.ExpectedNodeRevision})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := source.candidate.Review(domain.HealCandidatePromoted)
	if err != nil {
		t.Fatal(err)
	}
	node, err := nodes.current.PublishVersion("stable-version", promoted.PageURL, promoted.Origin, promoted.Selectors, promoted.Fingerprint, domain.SourceAutoHeal, 10)
	if err != nil {
		t.Fatal(err)
	}
	transaction.replay = &HealReviewOutcome{Status: HealReviewReplayed, CommandID: command.CommandID, RequestDigest: digest, Result: HealReviewResult{Decision: HealReviewApprove, Candidate: promoted, ElementTarget: &node}}
	source.candidate.Status = domain.HealCandidatePromoted
	result, err := newReviewService(t, source, nodes, transaction).Approve(context.Background(), command)
	if err != nil || result.Current.ID != "stable-version" {
		t.Fatalf("replay = %#v, %v", result, err)
	}
	if transaction.intent.CommandID != "" {
		t.Fatal("replay reached commit")
	}
}

func TestHealReviewServiceAuthorizesBeforeReplayExactlyOnce(t *testing.T) {
	for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
		t.Run(string(decision), func(t *testing.T) {
			source, nodes, transaction, command := healReviewFixture(t)
			calls := 0
			authorizationErr := errors.New("not authorized")
			service, err := NewHealReviewService(source, nodes, transaction, reviewerAuthorizerFake{err: authorizationErr, calls: &calls}, reviewClockFake(10), candidateVerifierFake{}, healReviewIdentityFake{versionID: "node-v2", sequence: 4})
			if err != nil {
				t.Fatal(err)
			}
			if decision == HealReviewApprove {
				_, err := service.Approve(context.Background(), command)
				if !errors.Is(err, authorizationErr) {
					t.Fatalf("Approve() error = %v", err)
				}
			} else if err := service.Reject(context.Background(), command); !errors.Is(err, authorizationErr) {
				t.Fatalf("Reject() error = %v", err)
			}
			if calls != 1 || transaction.lookupCalls != 0 || transaction.intent.CommandID != "" {
				t.Fatalf("authorization/lookup/commit = %d/%d/%q", calls, transaction.lookupCalls, transaction.intent.CommandID)
			}
		})
	}
}

func TestHealReviewServiceReplayAuthorizesExactlyOnce(t *testing.T) {
	source, nodes, transaction, command := healReviewFixture(t)
	digest, err := HealReviewRequestIdentityDigest(HealReviewRequest{CommandID: command.CommandID, Decision: HealReviewApprove, ElementTargetID: command.ElementTargetID, BaseNodeVersionID: command.BaseNodeVersionID, CandidateHash: command.CandidateHash, ExpectedCandidateRevision: command.ExpectedCandidateRevision, ExpectedNodeRevision: command.ExpectedNodeRevision})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := source.candidate.Review(domain.HealCandidatePromoted)
	if err != nil {
		t.Fatal(err)
	}
	node, err := nodes.current.PublishVersion("stable-version", promoted.PageURL, promoted.Origin, promoted.Selectors, promoted.Fingerprint, domain.SourceAutoHeal, 10)
	if err != nil {
		t.Fatal(err)
	}
	transaction.replay = &HealReviewOutcome{Status: HealReviewReplayed, CommandID: command.CommandID, RequestDigest: digest, Result: HealReviewResult{Decision: HealReviewApprove, Candidate: promoted, ElementTarget: &node}}
	calls := 0
	service, err := NewHealReviewService(source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer", calls: &calls}, reviewClockFake(10), candidateVerifierFake{}, healReviewIdentityFake{versionID: "unused", sequence: 99})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || transaction.lookupCalls != 1 || transaction.intent.CommandID != "" {
		t.Fatalf("authorization/lookup/commit = %d/%d/%q", calls, transaction.lookupCalls, transaction.intent.CommandID)
	}
}

func TestHealReviewServiceRejectsMalformedTransactionOutcome(t *testing.T) {
	source, nodes, transaction, command := healReviewFixture(t)
	transaction.outcome = &HealReviewOutcome{Status: HealReviewApplied, CommandID: "other"}
	_, err := newReviewService(t, source, nodes, transaction).Approve(context.Background(), command)
	if !errors.Is(err, ErrHealReviewContract) {
		t.Fatalf("malformed outcome error = %v", err)
	}
}

func TestHealReviewServiceDoesNotAliasTransactionOutcome(t *testing.T) {
	source, nodes, transaction, command := healReviewFixture(t)
	result, err := newReviewService(t, source, nodes, transaction).Approve(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	result.Current.Fingerprint.Attributes["mutated"] = "yes"
	if _, ok := transaction.intent.NextNode.Current.Fingerprint.Attributes["mutated"]; ok {
		t.Fatal("returned node aliases transaction intent")
	}
}
