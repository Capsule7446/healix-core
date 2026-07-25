package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

type fakeTransaction struct{}

func (fakeTransaction) CommitStepTransition(context.Context, domainexecution.WorkerFence, evidence.StepTransitionCommit, HealGovernancePlanner) (evidence.StepTransitionCommitResult, error) {
	return evidence.StepTransitionCommitResult{}, nil
}

type recordingTransaction struct {
	calls  int
	result evidence.StepTransitionCommitResult
	err    error
}

func (c *recordingTransaction) CommitStepTransition(context.Context, domainexecution.WorkerFence, evidence.StepTransitionCommit, HealGovernancePlanner) (evidence.StepTransitionCommitResult, error) {
	c.calls++
	return c.result, c.err
}

func validStepTransitionCommit() evidence.StepTransitionCommit {
	return evidence.StepTransitionCommit{CommitID: "commit", ExpectedRevision: 1, Event: evidence.StepPhaseEvent{
		ID: "step", ExecutionID: "execution", WorkflowStepID: "workflow-step", DisplayName: "step",
		Kind: "ACTION", Phase: "SUCCEEDED", Occurrence: 1, Timestamp: 1,
	}}
}

type retainingTransaction struct {
	commit evidence.StepTransitionCommit
	result evidence.StepTransitionCommitResult
}

func (transaction *retainingTransaction) CommitStepTransition(_ context.Context, _ domainexecution.WorkerFence, commit evidence.StepTransitionCommit, _ HealGovernancePlanner) (evidence.StepTransitionCommitResult, error) {
	transaction.commit = commit
	if len(commit.HealObservations) > 0 {
		commit.HealObservations[0].NodeID = "adapter-mutated"
	}
	return transaction.result, nil
}

func TestStepTransitionServiceOwnsCommitAndReturnedPromotions(t *testing.T) {
	commit := validStepTransitionCommit()
	commit.HealObservations = []evidence.HealObservation{{
		ID: "heal", RunID: "run", ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID,
		NodeID: "node", BaseNodeVersionID: "base", DecisionBand: evidence.DecisionUnknown, ObservedAt: 1,
	}}
	transaction := &retainingTransaction{result: evidence.StepTransitionCommitResult{Promotions: []evidence.NodeVersionPromotion{{NodeID: "node", VersionID: "version"}}}}
	service := NewStepTransitionService(NewFactCommitter(transaction, NewDefaultHealGovernancePlanner()))

	result, err := service.Commit(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	if commit.HealObservations[0].NodeID != "node" || transaction.commit.HealObservations[0].NodeID != "adapter-mutated" {
		t.Fatalf("commit ownership leaked: caller=%q adapter=%q", commit.HealObservations[0].NodeID, transaction.commit.HealObservations[0].NodeID)
	}
	result.Promotions[0].NodeID = "caller-mutated"
	if transaction.result.Promotions[0].NodeID != "node" {
		t.Fatal("returned promotions alias adapter storage")
	}
}

func TestStepTransitionServiceValidatesAndReturnsAuthoritativeResult(t *testing.T) {
	want := evidence.StepTransitionCommitResult{Revision: 2, WasApplied: false}
	committer := &recordingTransaction{result: want}
	service := NewStepTransitionService(NewFactCommitter(committer, NewDefaultHealGovernancePlanner()))
	got, err := service.Commit(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, validStepTransitionCommit())
	if err != nil || got.Revision != want.Revision || got.WasApplied != want.WasApplied || len(got.Promotions) != 0 || committer.calls != 1 {
		t.Fatalf("Commit() = (%#v, %v), calls = %d", got, err, committer.calls)
	}
}

func TestStepTransitionServiceRejectsInvalidInputBeforeCommit(t *testing.T) {
	for _, test := range []struct {
		name   string
		fence  domainexecution.WorkerFence
		commit evidence.StepTransitionCommit
	}{
		{"missing run", domainexecution.WorkerFence{ClaimToken: "claim"}, validStepTransitionCommit()},
		{"missing token", domainexecution.WorkerFence{RunID: "run"}, validStepTransitionCommit()},
		{"invalid commit", domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, evidence.StepTransitionCommit{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			committer := &recordingTransaction{}
			_, err := NewStepTransitionService(NewFactCommitter(committer, NewDefaultHealGovernancePlanner())).Commit(context.Background(), test.fence, test.commit)
			if err == nil || committer.calls != 0 {
				t.Fatalf("Commit() error = %v, calls = %d", err, committer.calls)
			}
		})
	}
}

func TestStepTransitionServiceRejectsCrossRunFactsBeforeCommit(t *testing.T) {
	commit := validStepTransitionCommit()
	commit.HealObservations = []evidence.HealObservation{{
		ID: "heal", RunID: "other-run", ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID,
		NodeID: "node", BaseNodeVersionID: "base", DecisionBand: evidence.DecisionUnknown, ObservedAt: 1,
	}}
	transaction := &recordingTransaction{}
	_, err := NewStepTransitionService(NewFactCommitter(transaction, NewDefaultHealGovernancePlanner())).Commit(
		context.Background(),
		domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"},
		commit,
	)
	if err == nil || transaction.calls != 0 {
		t.Fatalf("Commit() error = %v, calls = %d", err, transaction.calls)
	}
}

func TestStepTransitionServiceRejectsExactSerializedPayloadOverLimit(t *testing.T) {
	committer := &recordingTransaction{}
	commit := validStepTransitionCommit()
	commit.CommitID = strings.Repeat("\\", MaxStepTransitionPayloadBytes/2)
	_, err := NewStepTransitionService(NewFactCommitter(committer, NewDefaultHealGovernancePlanner())).Commit(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, commit)
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversized serialized payload error = %v", err)
	}
	if committer.calls != 0 {
		t.Fatal("oversized payload reached transaction")
	}
}

func TestStepTransitionServiceRejectsNilCommitter(t *testing.T) {
	var typedNil *recordingTransaction
	for _, committer := range []FactCommitter{{}, NewFactCommitter(nil, NewDefaultHealGovernancePlanner()), NewFactCommitter(typedNil, NewDefaultHealGovernancePlanner())} {
		_, err := NewStepTransitionService(committer).Commit(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, validStepTransitionCommit())
		if !errors.Is(err, ErrFactCommitterRequired) {
			t.Fatalf("Commit() error = %v", err)
		}
	}
}

func TestStepTransitionServicePreservesTypedCommitErrors(t *testing.T) {
	for _, want := range []error{domainexecution.ErrStaleWorkerFence, ErrStepRevisionConflict, ErrCommitIdentityConflict} {
		committer := &recordingTransaction{err: want}
		_, err := NewStepTransitionService(NewFactCommitter(committer, NewDefaultHealGovernancePlanner())).Commit(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, validStepTransitionCommit())
		if !errors.Is(err, want) {
			t.Fatalf("Commit() error = %v, want %v", err, want)
		}
	}
}

func TestFactCommitterKeepsAtomicDomainCommitContract(t *testing.T) {
	var _ StepTransitionTransaction = fakeTransaction{}
	committer := NewFactCommitter(fakeTransaction{}, NewDefaultHealGovernancePlanner())
	if isNilInterface(committer.transaction) || isNilInterface(committer.planner) {
		t.Fatal("valid fact committer dependencies were rejected")
	}
}

type fakeProgressWriter struct{}

func (fakeProgressWriter) RecordStepProgress(context.Context, domainexecution.WorkerFence, evidence.StepProgressEvent) error {
	return nil
}
func (fakeProgressWriter) RecordValidationProgress(context.Context, domainexecution.WorkerFence, evidence.ValidationProgressObservation) error {
	return nil
}

func TestProgressWriterKeepsNonTerminalFactContract(t *testing.T) {
	var _ ProgressWriter = fakeProgressWriter{}
}
