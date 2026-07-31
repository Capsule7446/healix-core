package automation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

type samplingTransactionProbe struct {
	lookupOutcome  PublishSamplingOutcome
	lookupFound    bool
	lookupErr      error
	publishOutcome PublishSamplingOutcome
	publishErr     error
	lookupCalls    int
	publishCalls   int
	intent         PublishSamplingIntent
}

func (transaction *samplingTransactionProbe) LookupSamplingPublication(context.Context, string, string) (PublishSamplingOutcome, bool, error) {
	transaction.lookupCalls++
	return transaction.lookupOutcome, transaction.lookupFound, transaction.lookupErr
}

func (transaction *samplingTransactionProbe) PublishSampling(_ context.Context, intent PublishSamplingIntent) (PublishSamplingOutcome, error) {
	transaction.publishCalls++
	transaction.intent = intent
	return transaction.publishOutcome, transaction.publishErr
}

func createSamplingCommand(t testing.TB) SamplingPublicationCommand {
	t.Helper()
	publication, err := MapSamplingPublication(SamplingPublicationRequest{
		FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2,
		Workspace: sampledWorkflow(sampling.ResolutionModeCreate),
		Nodes:     []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "forced", ElementTargetVersionID: "forced-v1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return SamplingPublicationCommand{PublicationID: "publication", Publication: publication}
}

func TestSamplingPublicationPublishCoversLookupAndTransactionFailures(t *testing.T) {
	failure := errors.New("sampling transaction unavailable")
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}

	transaction := &samplingTransactionProbe{lookupErr: failure}
	result, err := NewSamplingPublicationService(transaction).Publish(context.Background(), command)
	if !errors.Is(err, failure) || !reflect.DeepEqual(result, domain.SamplingPublicationResult{}) || transaction.lookupCalls != 1 || transaction.publishCalls != 0 {
		t.Fatalf("lookup failure/result/error/calls = %#v/%v/%d/%d", result, err, transaction.lookupCalls, transaction.publishCalls)
	}

	transaction = &samplingTransactionProbe{publishErr: failure}
	result, err = NewSamplingPublicationService(transaction).Publish(context.Background(), command)
	if !errors.Is(err, failure) || !reflect.DeepEqual(result, domain.SamplingPublicationResult{}) || transaction.lookupCalls != 1 || transaction.publishCalls != 1 {
		t.Fatalf("publish failure/result/error/calls = %#v/%v/%d/%d", result, err, transaction.lookupCalls, transaction.publishCalls)
	}
	if transaction.intent.PublicationID != command.PublicationID || transaction.intent.RequestDigest == "" || transaction.intent.Publication.FlowFragment.FlowFragment.ID != command.Publication.FlowFragment.FlowFragment.ID || transaction.intent.Publication.FlowFragment.Current.ID != command.Publication.FlowFragment.Current.ID {
		t.Fatalf("publish intent = %#v", transaction.intent)
	}
}

func TestSamplingPublicationLookupIdentityConflictPreventsPublish(t *testing.T) {
	command := createSamplingCommand(t)
	transaction := &samplingTransactionProbe{lookupErr: SamplingPublicationIdentityConflictError()}
	result, err := NewSamplingPublicationService(transaction).Publish(context.Background(), command)
	if !fault.IsCode(err, CodeSamplingPublicationIdentityConflict) || !reflect.DeepEqual(result, domain.SamplingPublicationResult{}) || transaction.lookupCalls != 1 || transaction.publishCalls != 0 {
		t.Fatalf("result/error/lookup/publish = %#v/%v/%d/%d", result, err, transaction.lookupCalls, transaction.publishCalls)
	}
}

func TestSamplingPublicationPublishRejectsInvalidCommandBeforeDependencies(t *testing.T) {
	plain := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	tests := []struct {
		name    string
		command SamplingPublicationCommand
		want    string
	}{
		{name: "missing publication identity", command: func() SamplingPublicationCommand { value := plain; value.PublicationID = " "; return value }(), want: "sampling publication id is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transaction := &samplingTransactionProbe{}
			if _, err := NewSamplingPublicationService(transaction).Publish(context.Background(), test.command); err == nil || !strings.Contains(err.Error(), test.want) || transaction.lookupCalls != 0 || transaction.publishCalls != 0 {
				t.Fatalf("error/lookup/publish calls = %v/%d/%d", err, transaction.lookupCalls, transaction.publishCalls)
			}
		})
	}
}

func TestSamplingPublicationReplayValidatesEveryAuthoritativeFieldBeforeReturning(t *testing.T) {
	command := createSamplingCommand(t)
	valid := samplingOutcomeFor(t, command, PublishSamplingReplayed)
	tests := []struct {
		name   string
		mutate func(*PublishSamplingOutcome)
	}{
		{name: "unsupported status", mutate: func(outcome *PublishSamplingOutcome) { outcome.Status = "UNKNOWN" }},
		{name: "publication identity", mutate: func(outcome *PublishSamplingOutcome) { outcome.PublicationID = "other" }},
		{name: "request digest", mutate: func(outcome *PublishSamplingOutcome) { outcome.RequestDigest = "sha256:other" }},
		{name: "workflow version", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.WorkflowVersionID = "other" }},
		{name: "mapping count", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.Nodes = nil }},
		{name: "mapping identity", mutate: func(outcome *PublishSamplingOutcome) { outcome.Result.Nodes[0].ElementTargetVersionID = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := valid
			outcome.Result.Nodes = append([]domain.SamplingNodeMapping(nil), valid.Result.Nodes...)
			test.mutate(&outcome)
			transaction := &samplingTransactionProbe{lookupOutcome: outcome, lookupFound: true}
			result, err := NewSamplingPublicationService(transaction).Publish(context.Background(), command)
			if !fault.IsCode(err, CodeSamplingPublicationContractViolation) || !reflect.DeepEqual(result, domain.SamplingPublicationResult{}) || transaction.publishCalls != 0 {
				t.Fatalf("result/error/publish calls = %#v/%v/%d", result, err, transaction.publishCalls)
			}
		})
	}
}

type healReviewSourceProbe struct {
	candidate      domain.HealCandidate
	streak         domain.HealStreak
	candidateErr   error
	streakErr      error
	candidateCalls int
	streakCalls    int
}

func (source *healReviewSourceProbe) LoadCandidate(context.Context, string, string, string) (domain.HealCandidate, error) {
	source.candidateCalls++
	return source.candidate, source.candidateErr
}

func (source *healReviewSourceProbe) LoadStreak(context.Context, string, string, string) (domain.HealStreak, error) {
	source.streakCalls++
	return source.streak, source.streakErr
}

type healReviewTransactionProbe struct {
	lookupOutcome HealReviewOutcome
	lookupFound   bool
	lookupErr     error
	commitOutcome *HealReviewOutcome
	commitBuilder func(HealReviewIntent) HealReviewOutcome
	commitErr     error
	lookupCalls   int
	commitCalls   int
	intent        HealReviewIntent
}

func (transaction *healReviewTransactionProbe) LookupHealReview(context.Context, string, string) (HealReviewOutcome, bool, error) {
	transaction.lookupCalls++
	return cloneHealReviewOutcome(transaction.lookupOutcome), transaction.lookupFound, transaction.lookupErr
}

func (transaction *healReviewTransactionProbe) CommitHealReview(_ context.Context, intent HealReviewIntent) (HealReviewOutcome, error) {
	transaction.commitCalls++
	transaction.intent = cloneHealReviewIntent(intent)
	if transaction.commitErr != nil {
		return HealReviewOutcome{}, transaction.commitErr
	}
	if transaction.commitOutcome != nil {
		return cloneHealReviewOutcome(*transaction.commitOutcome), nil
	}
	if transaction.commitBuilder != nil {
		return cloneHealReviewOutcome(transaction.commitBuilder(intent)), nil
	}
	return HealReviewOutcome{
		Status: HealReviewApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest,
		Result: HealReviewResult{Decision: intent.Decision, Candidate: intent.NextCandidate, ElementTarget: intent.NextNode, Streak: intent.NextStreak},
	}, nil
}

type healReviewIdentityProbe struct {
	versionID     string
	sequence      uint64
	versionErr    error
	sequenceErr   error
	versionCalls  int
	sequenceCalls int
}

func (identity *healReviewIdentityProbe) NewNodeVersionID(context.Context, string) (string, error) {
	identity.versionCalls++
	return identity.versionID, identity.versionErr
}

func (identity *healReviewIdentityProbe) NextRejectionSequence(context.Context, string) (uint64, error) {
	identity.sequenceCalls++
	return identity.sequence, identity.sequenceErr
}

func healReviewMatrixFixture(t *testing.T) (*healReviewSourceProbe, *nodeRepositoryFake, domain.HealCandidateReviewCommand) {
	t.Helper()
	source, nodes, _, command := healReviewFixture(t)
	return &healReviewSourceProbe{candidate: source.candidate, streak: source.streak}, nodes, command
}

func newHealReviewMatrixService(
	t testing.TB,
	source HealReviewSource,
	nodes NodeRepository,
	transaction HealReviewTransaction,
	reviewers ReviewerAuthorizer,
	clock ReviewClock,
	verifier CandidateVerifier,
	identities HealReviewIdentityProvider,
) HealReviewService {
	t.Helper()
	service, err := NewHealReviewService(source, nodes, transaction, reviewers, clock, verifier, identities)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestHealReviewApproveCoversDependencyFailuresWithoutCommit(t *testing.T) {
	failure := errors.New("heal review dependency unavailable")
	tests := []struct {
		name      string
		configure func(*healReviewSourceProbe, *nodeRepositoryFake, *healReviewTransactionProbe, *reviewerAuthorizerFake, *candidateVerifierFake, *healReviewIdentityProbe)
	}{
		{name: "lookup", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, transaction *healReviewTransactionProbe, _ *reviewerAuthorizerFake, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			transaction.lookupErr = failure
		}},
		{name: "load candidate", configure: func(source *healReviewSourceProbe, _ *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *reviewerAuthorizerFake, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			source.candidateErr = failure
		}},
		{name: "verify candidate", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *reviewerAuthorizerFake, verifier *candidateVerifierFake, _ *healReviewIdentityProbe) {
			verifier.err = failure
		}},
		{name: "load node", configure: func(_ *healReviewSourceProbe, nodes *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *reviewerAuthorizerFake, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			nodes.loadErr = failure
		}},
		{name: "allocate version", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *reviewerAuthorizerFake, _ *candidateVerifierFake, identity *healReviewIdentityProbe) {
			identity.versionErr = failure
		}},
		{name: "commit", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, transaction *healReviewTransactionProbe, _ *reviewerAuthorizerFake, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			transaction.commitErr = failure
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			transaction := &healReviewTransactionProbe{}
			reviewer := &reviewerAuthorizerFake{id: "reviewer"}
			verifier := &candidateVerifierFake{}
			identity := &healReviewIdentityProbe{versionID: "node-v2", sequence: 4}
			test.configure(source, nodes, transaction, reviewer, verifier, identity)
			service := newHealReviewMatrixService(t, source, nodes, transaction, reviewer, reviewClockFake(10), verifier, identity)
			result, err := service.Approve(context.Background(), command)
			if !errors.Is(err, failure) || !reflect.DeepEqual(result, domain.ElementTargetAggregate{}) {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			wantCommitCalls := 0
			if test.name == "commit" {
				wantCommitCalls = 1
			}
			if transaction.commitCalls != wantCommitCalls {
				t.Fatalf("commit calls = %d, want %d", transaction.commitCalls, wantCommitCalls)
			}
		})
	}
}

func TestHealReviewRejectCoversDependencyFailuresWithoutPartialCommit(t *testing.T) {
	failure := errors.New("heal review dependency unavailable")
	tests := []struct {
		name      string
		configure func(*healReviewSourceProbe, *nodeRepositoryFake, *healReviewTransactionProbe, *candidateVerifierFake, *healReviewIdentityProbe)
	}{
		{name: "lookup", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, transaction *healReviewTransactionProbe, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			transaction.lookupErr = failure
		}},
		{name: "load candidate", configure: func(source *healReviewSourceProbe, _ *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			source.candidateErr = failure
		}},
		{name: "verify candidate", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, _ *healReviewTransactionProbe, verifier *candidateVerifierFake, _ *healReviewIdentityProbe) {
			verifier.err = failure
		}},
		{name: "load node", configure: func(_ *healReviewSourceProbe, nodes *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			nodes.loadErr = failure
		}},
		{name: "load streak", configure: func(source *healReviewSourceProbe, _ *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			source.streakErr = failure
		}},
		{name: "allocate sequence", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, _ *healReviewTransactionProbe, _ *candidateVerifierFake, identity *healReviewIdentityProbe) {
			identity.sequenceErr = failure
		}},
		{name: "commit", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, transaction *healReviewTransactionProbe, _ *candidateVerifierFake, _ *healReviewIdentityProbe) {
			transaction.commitErr = failure
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			transaction := &healReviewTransactionProbe{}
			verifier := &candidateVerifierFake{}
			identity := &healReviewIdentityProbe{versionID: "node-v2", sequence: 4}
			test.configure(source, nodes, transaction, verifier, identity)
			service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), verifier, identity)
			if err := service.Reject(context.Background(), command); !errors.Is(err, failure) {
				t.Fatalf("error = %v", err)
			}
			wantCommitCalls := 0
			if test.name == "commit" {
				wantCommitCalls = 1
			}
			if transaction.commitCalls != wantCommitCalls {
				t.Fatalf("commit calls = %d, want %d", transaction.commitCalls, wantCommitCalls)
			}
		})
	}
}

func TestHealReviewRejectReplayIsIdempotentAndSkipsMutableDependencies(t *testing.T) {
	source, nodes, command := healReviewMatrixFixture(t)
	request := HealReviewRequest{CommandID: command.CommandID, Decision: HealReviewReject, ElementTargetID: command.ElementTargetID, BaseNodeVersionID: command.BaseNodeVersionID, CandidateHash: command.CandidateHash, ExpectedCandidateRevision: command.ExpectedCandidateRevision, ExpectedNodeRevision: command.ExpectedNodeRevision}
	digest, err := HealReviewRequestIdentityDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := source.candidate.Review(domain.HealCandidateRejected)
	if err != nil {
		t.Fatal(err)
	}
	rejection, err := source.streak.Reject(4)
	if err != nil {
		t.Fatal(err)
	}
	streak := rejection.Next
	transaction := &healReviewTransactionProbe{
		lookupFound:   true,
		lookupOutcome: HealReviewOutcome{Status: HealReviewReplayed, CommandID: command.CommandID, RequestDigest: digest, Result: HealReviewResult{Decision: HealReviewReject, Candidate: rejected, Streak: &streak}},
	}
	source.candidateErr = errors.New("must not load candidate")
	source.streakErr = errors.New("must not load streak")
	nodes.loadErr = errors.New("must not load node")
	service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{err: errors.New("must not verify")}, &healReviewIdentityProbe{versionErr: errors.New("must not allocate"), sequenceErr: errors.New("must not allocate")})
	if err := service.Reject(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if transaction.lookupCalls != 1 || transaction.commitCalls != 0 || source.candidateCalls != 0 || source.streakCalls != 0 || nodes.saveCalls != 0 {
		t.Fatalf("lookup/commit/candidate/streak/node save = %d/%d/%d/%d/%d", transaction.lookupCalls, transaction.commitCalls, source.candidateCalls, source.streakCalls, nodes.saveCalls)
	}
}

func TestHealReviewUseCasesRejectInvalidIdentityAndTrustedTimeBeforeCommit(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*healReviewSourceProbe, *nodeRepositoryFake, *domain.HealCandidateReviewCommand) ReviewClock
		target    error
		want      string
	}{
		{name: "candidate identity", configure: func(source *healReviewSourceProbe, _ *nodeRepositoryFake, _ *domain.HealCandidateReviewCommand) ReviewClock {
			source.candidate.Hash = "other"
			return reviewClockFake(10)
		}, target: CodeHealReviewAuthorityConflict},
		{name: "node identity", configure: func(_ *healReviewSourceProbe, nodes *nodeRepositoryFake, _ *domain.HealCandidateReviewCommand) ReviewClock {
			nodes.current.ElementTarget.ID = "other"
			return reviewClockFake(10)
		}, target: CodeHealReviewAuthorityConflict},
		{name: "node revision", configure: func(_ *healReviewSourceProbe, nodes *nodeRepositoryFake, _ *domain.HealCandidateReviewCommand) ReviewClock {
			nodes.current.ElementTarget.Revision++
			return reviewClockFake(10)
		}, target: CodeAutomationRevisionConflict},
		{name: "trusted time", configure: func(_ *healReviewSourceProbe, _ *nodeRepositoryFake, _ *domain.HealCandidateReviewCommand) ReviewClock {
			return reviewClockFake(0)
		}, want: "trusted review time must be positive"},
	}
	for _, test := range tests {
		for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
			t.Run(test.name+"/"+string(decision), func(t *testing.T) {
				source, nodes, command := healReviewMatrixFixture(t)
				clock := test.configure(source, nodes, &command)
				transaction := &healReviewTransactionProbe{}
				service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, clock, candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2", sequence: 4})
				var err error
				if decision == HealReviewApprove {
					_, err = service.Approve(context.Background(), command)
				} else {
					err = service.Reject(context.Background(), command)
				}
				if err == nil || test.target != nil && !errors.Is(err, test.target) || test.want != "" && !strings.Contains(err.Error(), test.want) || transaction.commitCalls != 0 {
					t.Fatalf("error/commit calls = %v/%d", err, transaction.commitCalls)
				}
			})
		}
	}
}

func validHealReviewReplay(t *testing.T, decision HealReviewDecision) (*healReviewSourceProbe, *nodeRepositoryFake, domain.HealCandidateReviewCommand, HealReviewOutcome) {
	t.Helper()
	source, nodes, command := healReviewMatrixFixture(t)
	request := HealReviewRequest{CommandID: command.CommandID, Decision: decision, ElementTargetID: command.ElementTargetID, BaseNodeVersionID: command.BaseNodeVersionID, CandidateHash: command.CandidateHash, ExpectedCandidateRevision: command.ExpectedCandidateRevision, ExpectedNodeRevision: command.ExpectedNodeRevision}
	digest, err := HealReviewRequestIdentityDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	status := domain.HealCandidatePromoted
	if decision == HealReviewReject {
		status = domain.HealCandidateRejected
	}
	reviewed, err := source.candidate.Review(status)
	if err != nil {
		t.Fatal(err)
	}
	result := HealReviewResult{Decision: decision, Candidate: reviewed}
	if decision == HealReviewApprove {
		node, err := nodes.current.PublishVersion("node-v2", reviewed.PageURL, reviewed.Origin, reviewed.Selectors, reviewed.Fingerprint, domain.SourceAutoHeal, 10)
		if err != nil {
			t.Fatal(err)
		}
		result.ElementTarget = &node
	} else {
		rejection, err := source.streak.Reject(4)
		if err != nil {
			t.Fatal(err)
		}
		streak := rejection.Next
		result.Streak = &streak
	}
	return source, nodes, command, HealReviewOutcome{Status: HealReviewReplayed, CommandID: command.CommandID, RequestDigest: digest, Result: result}
}

func TestHealReviewReplayRejectsMalformedAuthoritativeResults(t *testing.T) {
	common := []struct {
		name   string
		mutate func(*HealReviewOutcome)
	}{
		{name: "status", mutate: func(outcome *HealReviewOutcome) { outcome.Status = "UNKNOWN" }},
		{name: "request identity", mutate: func(outcome *HealReviewOutcome) { outcome.CommandID = "other" }},
		{name: "candidate invalid", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Candidate.Hash = "" }},
		{name: "candidate identity", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Candidate.Hash = "other" }},
	}
	for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
		for _, test := range common {
			t.Run(string(decision)+"/"+test.name, func(t *testing.T) {
				source, nodes, command, valid := validHealReviewReplay(t, decision)
				outcome := cloneHealReviewOutcome(valid)
				test.mutate(&outcome)
				transaction := &healReviewTransactionProbe{lookupFound: true, lookupOutcome: outcome}
				service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "unused", sequence: 99})
				var err error
				if decision == HealReviewApprove {
					_, err = service.Approve(context.Background(), command)
				} else {
					err = service.Reject(context.Background(), command)
				}
				if !fault.IsCode(err, CodeHealReviewContractViolation) || transaction.commitCalls != 0 || source.candidateCalls != 0 {
					t.Fatalf("error/commit/candidate calls = %v/%d/%d", err, transaction.commitCalls, source.candidateCalls)
				}
			})
		}
	}

	approveMutations := []struct {
		name   string
		mutate func(*HealReviewOutcome)
	}{
		{name: "decision shape", mutate: func(outcome *HealReviewOutcome) { outcome.Result.ElementTarget = nil }},
		{name: "node invalid", mutate: func(outcome *HealReviewOutcome) { outcome.Result.ElementTarget.Current.ID = "" }},
		{name: "node identity", mutate: func(outcome *HealReviewOutcome) { outcome.Result.ElementTarget.ElementTarget.Revision++ }},
		{name: "version candidate mismatch", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Candidate.PageURL = "https://other.test" }},
	}
	for _, test := range approveMutations {
		t.Run("approve/"+test.name, func(t *testing.T) {
			source, nodes, command, valid := validHealReviewReplay(t, HealReviewApprove)
			outcome := cloneHealReviewOutcome(valid)
			test.mutate(&outcome)
			transaction := &healReviewTransactionProbe{lookupFound: true, lookupOutcome: outcome}
			_, err := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{}).Approve(context.Background(), command)
			if !fault.IsCode(err, CodeHealReviewContractViolation) || transaction.commitCalls != 0 {
				t.Fatalf("error/commit calls = %v/%d", err, transaction.commitCalls)
			}
		})
	}

	rejectMutations := []struct {
		name   string
		mutate func(*HealReviewOutcome)
	}{
		{name: "decision shape", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Streak = nil }},
		{name: "streak invalid", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Streak.LastSequence = 0 }},
		{name: "streak identity", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Streak.ElementTargetID = "other" }},
	}
	for _, test := range rejectMutations {
		t.Run("reject/"+test.name, func(t *testing.T) {
			source, nodes, command, valid := validHealReviewReplay(t, HealReviewReject)
			outcome := cloneHealReviewOutcome(valid)
			test.mutate(&outcome)
			transaction := &healReviewTransactionProbe{lookupFound: true, lookupOutcome: outcome}
			err := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{}).Reject(context.Background(), command)
			if !fault.IsCode(err, CodeHealReviewContractViolation) || transaction.commitCalls != 0 {
				t.Fatalf("error/commit calls = %v/%d", err, transaction.commitCalls)
			}
		})
	}
}

func TestHealReviewAppliedOutcomeRejectsEveryDivergentAuthoritativeValue(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*HealReviewOutcome)
	}{
		{name: "candidate", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Candidate.Revision++ }},
		{name: "node", mutate: func(outcome *HealReviewOutcome) { outcome.Result.ElementTarget = nil }},
		{name: "streak", mutate: func(outcome *HealReviewOutcome) { outcome.Result.Streak = &domain.HealStreak{} }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			transaction := &healReviewTransactionProbe{commitBuilder: func(intent HealReviewIntent) HealReviewOutcome {
				outcome := HealReviewOutcome{Status: HealReviewApplied, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: HealReviewResult{Decision: intent.Decision, Candidate: intent.NextCandidate, ElementTarget: intent.NextNode, Streak: intent.NextStreak}}
				test.mutate(&outcome)
				return outcome
			}}
			result, err := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2"}).Approve(context.Background(), command)
			if !fault.IsCode(err, CodeHealReviewContractViolation) || !reflect.DeepEqual(result, domain.ElementTargetAggregate{}) || transaction.commitCalls != 1 {
				t.Fatalf("result/error/commit calls = %#v/%v/%d", result, err, transaction.commitCalls)
			}
		})
	}
}

func TestHealReviewConcurrentReplayReturnsAuthoritativeWinner(t *testing.T) {
	for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
		t.Run(string(decision), func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			transaction := &healReviewTransactionProbe{commitBuilder: func(intent HealReviewIntent) HealReviewOutcome {
				return HealReviewOutcome{Status: HealReviewReplayed, CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: HealReviewResult{Decision: intent.Decision, Candidate: intent.NextCandidate, ElementTarget: intent.NextNode, Streak: intent.NextStreak}}
			}}
			service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2", sequence: 4})
			if decision == HealReviewApprove {
				result, err := service.Approve(context.Background(), command)
				if err != nil || result.Current.ID != "node-v2" {
					t.Fatalf("result/error = %#v/%v", result, err)
				}
			} else if err := service.Reject(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			if transaction.commitCalls != 1 {
				t.Fatalf("commit calls = %d", transaction.commitCalls)
			}
		})
	}
}

func TestHealReviewUseCasesRejectInvalidCommandsBeforeSideEffects(t *testing.T) {
	for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
		t.Run(string(decision), func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			command.CommandID = "malicious\ncommand"
			transaction := &healReviewTransactionProbe{}
			service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2", sequence: 4})
			var err error
			if decision == HealReviewApprove {
				_, err = service.Approve(context.Background(), command)
			} else {
				err = service.Reject(context.Background(), command)
			}
			if !fault.IsCode(err, domain.CodeHealCandidateReviewCommandInvalid) || transaction.lookupCalls != 0 || transaction.commitCalls != 0 || source.candidateCalls != 0 || source.streakCalls != 0 {
				t.Fatalf("error/lookup/commit/candidate/streak = %v/%d/%d/%d/%d", err, transaction.lookupCalls, transaction.commitCalls, source.candidateCalls, source.streakCalls)
			}
		})
	}
}

func TestHealReviewUseCasesRejectInvalidCandidateReviewerAndGeneratedIdentity(t *testing.T) {
	for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
		t.Run(string(decision)+"/empty reviewer", func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			transaction := &healReviewTransactionProbe{}
			service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2", sequence: 4})
			var err error
			if decision == HealReviewApprove {
				_, err = service.Approve(context.Background(), command)
			} else {
				err = service.Reject(context.Background(), command)
			}
			if err == nil || transaction.lookupCalls != 0 || transaction.commitCalls != 0 {
				t.Fatalf("error/lookup/commit = %v/%d/%d", err, transaction.lookupCalls, transaction.commitCalls)
			}
		})

		t.Run(string(decision)+"/invalid candidate", func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			source.candidate.Status = domain.HealCandidateObserving
			transaction := &healReviewTransactionProbe{}
			service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2", sequence: 4})
			var err error
			if decision == HealReviewApprove {
				_, err = service.Approve(context.Background(), command)
			} else {
				err = service.Reject(context.Background(), command)
			}
			if !fault.IsCode(err, domain.CodeHealCandidateStateInvalid) || transaction.commitCalls != 0 {
				t.Fatalf("error/commit = %v/%d", err, transaction.commitCalls)
			}
		})
	}

	t.Run("approve generated version conflicts", func(t *testing.T) {
		source, nodes, command := healReviewMatrixFixture(t)
		transaction := &healReviewTransactionProbe{}
		_, err := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v1"}).Approve(context.Background(), command)
		if err == nil || !strings.Contains(err.Error(), "new version id must differ from the current version") || transaction.commitCalls != 0 {
			t.Fatalf("error/commit = %v/%d", err, transaction.commitCalls)
		}
	})

	t.Run("reject generated sequence is stale", func(t *testing.T) {
		source, nodes, command := healReviewMatrixFixture(t)
		transaction := &healReviewTransactionProbe{}
		err := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{sequence: 3}).Reject(context.Background(), command)
		if !fault.IsCode(err, domain.CodeHealSequenceConflict) || transaction.commitCalls != 0 {
			t.Fatalf("error/commit = %v/%d", err, transaction.commitCalls)
		}
	})
}

func TestHealReviewUseCasesRejectInvalidCommandBeforeMutableState(t *testing.T) {
	for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
		t.Run(string(decision), func(t *testing.T) {
			source, nodes, command := healReviewMatrixFixture(t)
			command.CommandID = ""
			transaction := &healReviewTransactionProbe{}
			service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2", sequence: 4})
			var err error
			if decision == HealReviewApprove {
				_, err = service.Approve(context.Background(), command)
			} else {
				err = service.Reject(context.Background(), command)
			}
			if err == nil || transaction.lookupCalls != 0 || transaction.commitCalls != 0 || source.candidateCalls != 0 {
				t.Fatalf("error/lookup/commit/candidate calls = %v/%d/%d/%d", err, transaction.lookupCalls, transaction.commitCalls, source.candidateCalls)
			}
		})
	}
}

func TestHealReviewCommitRejectsUnsupportedAndMalformedConcurrentOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		builder func(HealReviewIntent) HealReviewOutcome
		want    string
	}{
		{name: "unsupported status", want: "unsupported status", builder: func(intent HealReviewIntent) HealReviewOutcome {
			return HealReviewOutcome{Status: "UNKNOWN", CommandID: intent.CommandID, RequestDigest: intent.RequestDigest, Result: HealReviewResult{Decision: intent.Decision, Candidate: intent.NextCandidate, ElementTarget: intent.NextNode, Streak: intent.NextStreak}}
		}},
		{name: "malformed concurrent replay", want: "validate concurrent heal review replay", builder: func(intent HealReviewIntent) HealReviewOutcome {
			return HealReviewOutcome{Status: HealReviewReplayed, CommandID: "other", RequestDigest: intent.RequestDigest, Result: HealReviewResult{Decision: intent.Decision, Candidate: intent.NextCandidate, ElementTarget: intent.NextNode, Streak: intent.NextStreak}}
		}},
	}
	for _, decision := range []HealReviewDecision{HealReviewApprove, HealReviewReject} {
		for _, test := range tests {
			t.Run(string(decision)+"/"+test.name, func(t *testing.T) {
				source, nodes, command := healReviewMatrixFixture(t)
				transaction := &healReviewTransactionProbe{commitBuilder: test.builder}
				service := newHealReviewMatrixService(t, source, nodes, transaction, reviewerAuthorizerFake{id: "reviewer"}, reviewClockFake(10), candidateVerifierFake{}, &healReviewIdentityProbe{versionID: "node-v2", sequence: 4})
				var err error
				if decision == HealReviewApprove {
					_, err = service.Approve(context.Background(), command)
				} else {
					err = service.Reject(context.Background(), command)
				}
				if !fault.IsCode(err, CodeHealReviewContractViolation) || transaction.commitCalls != 1 {
					t.Fatalf("error/commit calls = %v/%d", err, transaction.commitCalls)
				}
			})
		}
	}
}
