package execution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/evidence"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type fakeTransaction struct{}

func (fakeTransaction) CommitStepTransition(context.Context, domainexecution.WorkerFence, evidence.StepTransitionCommit, HealGovernancePlanner) (evidence.StepTransitionCommitResult, error) {
	return evidence.StepTransitionCommitResult{}, nil
}

type recordingTransaction struct {
	calls   int
	result  evidence.StepTransitionCommitResult
	err     error
	fence   domainexecution.WorkerFence
	commit  evidence.StepTransitionCommit
	planner HealGovernancePlanner
}

func (c *recordingTransaction) CommitStepTransition(_ context.Context, fence domainexecution.WorkerFence, commit evidence.StepTransitionCommit, planner HealGovernancePlanner) (evidence.StepTransitionCommitResult, error) {
	c.calls++
	c.fence = fence
	c.commit = commit
	c.planner = planner
	return c.result, c.err
}

type plannerFixture struct{}

func (*plannerFixture) PlanHealGovernance(HealGovernancePlan) (HealGovernanceDecision, error) {
	return HealGovernanceDecision{}, nil
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
		commit.HealObservations[0].ElementTargetID = "adapter-mutated"
	}
	return transaction.result, nil
}

func TestStepTransitionServiceOwnsCommitAndReturnedPromotions(t *testing.T) {
	commit := validStepTransitionCommit()
	commit.HealObservations = []evidence.HealObservation{{
		ID: "heal", RunID: "run", ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID,
		ElementTargetID: "node", BaseNodeVersionID: "base", DecisionBand: evidence.DecisionUnknown, ObservedAt: 1,
	}}
	transaction := &retainingTransaction{result: evidence.StepTransitionCommitResult{Promotions: []evidence.NodeVersionPromotion{{ElementTargetID: "node", VersionID: "version"}}}}
	service := NewStepTransitionService(NewFactCommitter(transaction, NewDefaultHealGovernancePlanner()))

	result, err := service.Commit(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	if commit.HealObservations[0].ElementTargetID != "node" || transaction.commit.HealObservations[0].ElementTargetID != "adapter-mutated" {
		t.Fatalf("commit ownership leaked: caller=%q adapter=%q", commit.HealObservations[0].ElementTargetID, transaction.commit.HealObservations[0].ElementTargetID)
	}
	result.Promotions[0].ElementTargetID = "caller-mutated"
	if transaction.result.Promotions[0].ElementTargetID != "node" {
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
	tests := []struct {
		name   string
		commit evidence.StepTransitionCommit
	}{
		{name: "final validation", commit: crossRunFinalValidationCommit()},
		{name: "validation group", commit: crossRunValidationGroupCommit()},
		{name: "heal observation", commit: crossRunHealObservationCommit()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transaction := &recordingTransaction{}
			_, err := NewStepTransitionService(NewFactCommitter(transaction, NewDefaultHealGovernancePlanner())).Commit(
				context.Background(),
				domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"},
				test.commit,
			)
			if err == nil || !strings.Contains(err.Error(), "does not match worker fence run") || transaction.calls != 0 {
				t.Fatalf("Commit() error = %v, calls = %d", err, transaction.calls)
			}
		})
	}
}

func crossRunFinalValidationCommit() evidence.StepTransitionCommit {
	commit := validStepTransitionCommit()
	commit.FinalValidations = []evidence.ValidationObservation{{
		ID: "validation", RunID: "other-run", ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID,
		ValidationStepID: "validation-step", ElementTargetID: "node", ElementTargetVersionID: "node-v1", AssertionKind: "visible",
		Expected: evidence.AbsentValidationValue(), Actual: evidence.AbsentValidationValue(),
		Reason: "passed", HealReviewStatus: "not_attempted", ObservedAt: 1, Final: true,
	}}
	return commit
}

func crossRunValidationGroupCommit() evidence.StepTransitionCommit {
	commit := validStepTransitionCommit()
	commit.FinalValidations = []evidence.ValidationObservation{
		{ID: "member-a", RunID: "run", ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID, ValidationStepID: "validation-a", ElementTargetID: "node-a", ElementTargetVersionID: "node-a-v1", GroupID: "group", BranchID: "branch-a", AssertionKind: "visible", Expected: evidence.AbsentValidationValue(), Actual: evidence.AbsentValidationValue(), Passed: true, Reason: "passed", BranchDisposition: evidence.ValidationBranchWon, HealReviewStatus: "not_attempted", ObservedAt: 1, Final: true},
		{ID: "member-b", RunID: "run", ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID, ValidationStepID: "validation-b", ElementTargetID: "node-b", ElementTargetVersionID: "node-b-v1", GroupID: "group", BranchID: "branch-b", AssertionKind: "visible", Expected: evidence.AbsentValidationValue(), Actual: evidence.AbsentValidationValue(), Reason: "normal_unsatisfied", BranchDisposition: evidence.ValidationBranchNotSatisfied, HealReviewStatus: "not_attempted", ObservedAt: 1, Final: true},
	}
	commit.FinalValidationGroups = []evidence.ValidationGroupTerminalObservation{evidence.NewValidationGroupTerminalObservation(
		"group-final", "other-run", commit.Event.ExecutionID, commit.Event.ID, "group", evidence.ValidationTerminalPassed, "branch-a",
		[]evidence.ValidationMemberIdentity{{BranchID: "branch-a", ElementTargetID: "node-a"}, {BranchID: "branch-b", ElementTargetID: "node-b"}}, 1,
	)}
	return commit
}

func crossRunHealObservationCommit() evidence.StepTransitionCommit {
	commit := validStepTransitionCommit()
	commit.HealObservations = []evidence.HealObservation{{
		ID: "heal", RunID: "other-run", ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID,
		ElementTargetID: "node", BaseNodeVersionID: "base", DecisionBand: evidence.DecisionUnknown, ObservedAt: 1,
	}}
	return commit
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

func TestValidateStepTransitionPayloadSizeRejectsOversizedTopLevelAndNestedStrings(t *testing.T) {
	tests := []struct {
		name      string
		nested    bool
		value     string
		wantError bool
	}{
		{name: "empty payload", value: ""},
		{name: "ordinary ascii", value: "command"},
		{name: "ordinary unicode", value: "命令🚀"},
		{name: "oversized ascii", value: strings.Repeat("a", MaxStepTransitionPayloadBytes+1), wantError: true},
		{name: "oversized unicode", value: strings.Repeat("界", MaxStepTransitionPayloadBytes/len("界")+1), wantError: true},
		{name: "nested ordinary unicode", nested: true, value: "节点🚀"},
		{name: "nested oversized ascii", nested: true, value: strings.Repeat("a", MaxStepTransitionPayloadBytes+1), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commit := evidence.StepTransitionCommit{CommitID: test.value}
			if test.nested {
				commit = validStepTransitionCommit()
				commit.OriginalSelectorResets = []evidence.HealCandidateReset{{
					ExecutionID: commit.Event.ExecutionID, StepExecutionID: commit.Event.ID,
					ElementTargetID: "node", BaseNodeVersionID: test.value, ObservedAt: 1,
				}}
				if !test.wantError {
					if err := commit.Validate(); err != nil {
						t.Fatalf("nested boundary fixture is invalid: %v", err)
					}
				}
			}
			err := ValidateStepTransitionPayloadSize(commit)
			if (err != nil) != test.wantError {
				t.Fatalf("ValidateStepTransitionPayloadSize() error = %v, wantError = %v", err, test.wantError)
			}
			if test.wantError && !strings.Contains(err.Error(), "string exceeds byte limit") {
				t.Fatalf("error = %v, want string byte limit", err)
			}
		})
	}
}

func TestValidateStepTransitionPayloadSizeAggregateByteBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		target    int
		wantError bool
	}{
		{name: "max minus one", target: MaxStepTransitionPayloadBytes - 1},
		{name: "max", target: MaxStepTransitionPayloadBytes},
		{name: "max plus one", target: MaxStepTransitionPayloadBytes + 1, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commit := stepTransitionCommitWithEncodedSize(t, test.target)
			err := ValidateStepTransitionPayloadSize(commit)
			if (err != nil) != test.wantError {
				t.Fatalf("ValidateStepTransitionPayloadSize() error = %v, wantError = %v", err, test.wantError)
			}
			if test.wantError && !strings.Contains(err.Error(), "commit exceeds byte limit") {
				t.Fatalf("error = %v, want aggregate byte limit", err)
			}
		})
	}
}

func stepTransitionCommitWithEncodedSize(t *testing.T, target int) evidence.StepTransitionCommit {
	t.Helper()
	commit := validStepTransitionCommit()
	commit.OriginalSelectorResets = make([]evidence.HealCandidateReset, 17)
	for index := range commit.OriginalSelectorResets {
		commit.OriginalSelectorResets[index] = evidence.HealCandidateReset{
			ExecutionID:       commit.Event.ExecutionID,
			StepExecutionID:   commit.Event.ID,
			ElementTargetID:   "node-" + strconv.Itoa(index),
			BaseNodeVersionID: "v",
			ObservedAt:        1,
		}
	}
	payload, err := json.Marshal(commit)
	if err != nil {
		t.Fatal(err)
	}
	remaining := target - len(payload)
	if remaining < 0 {
		t.Fatalf("target %d is smaller than base payload %d", target, len(payload))
	}
	perFieldCapacity := MaxStepTransitionPayloadBytes / len(commit.OriginalSelectorResets)
	for index := range commit.OriginalSelectorResets {
		length := min(remaining, perFieldCapacity-len(commit.OriginalSelectorResets[index].BaseNodeVersionID))
		commit.OriginalSelectorResets[index].BaseNodeVersionID += strings.Repeat("x", length)
		remaining -= length
	}
	if remaining != 0 {
		t.Fatalf("target %d exceeds fixture string capacity by %d bytes", target, remaining)
	}
	payload, err = json.Marshal(commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != target {
		t.Fatalf("fixture payload size = %d, want %d", len(payload), target)
	}
	if err := commit.Validate(); err != nil {
		t.Fatalf("sized fixture is not a valid domain commit: %v", err)
	}
	return commit
}

func TestStepTransitionServiceRejectsNilCommitter(t *testing.T) {
	var typedNil *recordingTransaction
	for _, committer := range []FactCommitter{{}, NewFactCommitter(nil, NewDefaultHealGovernancePlanner()), NewFactCommitter(typedNil, NewDefaultHealGovernancePlanner())} {
		_, err := NewStepTransitionService(committer).Commit(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, validStepTransitionCommit())
		if !fault.IsCode(err, CodeFactCommitterRequired) {
			t.Fatalf("Commit() error = %v", err)
		}
	}
}

func TestStepTransitionServicePreservesTypedCommitErrors(t *testing.T) {
	for _, want := range []error{domainexecution.NewStaleWorkerFenceError(), ErrStepRevisionConflict, ErrCommitIdentityConflict} {
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

func TestFactCommitterRejectsMissingDependenciesAndDelegatesAuthoritatively(t *testing.T) {
	var typedNilTransaction *recordingTransaction
	var typedNilPlanner *plannerFixture
	tests := []struct {
		name      string
		committer FactCommitter
		wantCode  fault.Code
		wantText  string
	}{
		{name: "missing transaction", committer: NewFactCommitter(nil, &plannerFixture{}), wantCode: CodeFactCommitterRequired},
		{name: "typed nil transaction", committer: NewFactCommitter(typedNilTransaction, &plannerFixture{}), wantCode: CodeFactCommitterRequired},
		{name: "missing planner", committer: NewFactCommitter(&recordingTransaction{}, nil), wantText: "heal governance planner is required"},
		{name: "typed nil planner", committer: NewFactCommitter(&recordingTransaction{}, typedNilPlanner), wantText: "heal governance planner is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.committer.CommitStepTransition(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, validStepTransitionCommit())
			if err == nil || !reflect.DeepEqual(result, evidence.StepTransitionCommitResult{}) {
				t.Fatalf("CommitStepTransition() = (%#v, %v)", result, err)
			}
			if test.wantCode != "" && !fault.IsCode(err, test.wantCode) {
				t.Fatalf("error = %v, want code %v", err, test.wantCode)
			}
			if test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want text %q", err, test.wantText)
			}
		})
	}

	dependencyFailure := errors.New("transaction failed")
	wantResult := evidence.StepTransitionCommitResult{Revision: 2, WasApplied: true}
	transaction := &recordingTransaction{result: wantResult}
	planner := &plannerFixture{}
	fence := domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}
	commit := validStepTransitionCommit()
	result, err := NewFactCommitter(transaction, planner).CommitStepTransition(context.Background(), fence, commit)
	if err != nil || !reflect.DeepEqual(result, wantResult) || transaction.calls != 1 || transaction.fence != fence || transaction.commit.CommitID != commit.CommitID || transaction.planner != planner {
		t.Fatalf("delegation result/error/transaction = %#v/%v/%#v", result, err, transaction)
	}
	transaction.err = dependencyFailure
	result, err = NewFactCommitter(transaction, planner).CommitStepTransition(context.Background(), fence, commit)
	if !errors.Is(err, dependencyFailure) || !reflect.DeepEqual(result, wantResult) || transaction.calls != 2 {
		t.Fatalf("dependency result/error/calls = %#v/%v/%d", result, err, transaction.calls)
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
