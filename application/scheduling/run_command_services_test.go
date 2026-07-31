package scheduling

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

func validCommandRun(t *testing.T, status domainexecution.RunStatus) domainexecution.Run {
	t.Helper()
	command := validCreateRunCommand()
	command.RunID = "run"
	resolved := validResolvedCreateRun(t, command)
	snapshot, err := BuildRunSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	run, err := domainexecution.NewRun(domainexecution.Run{ID: "run", ExecutionFlowID: "task", TestTaskVersionID: "task-v1", EnvironmentID: "env", Status: domainexecution.Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if status == domainexecution.Queued {
		return run
	}
	run, err = run.Transition(domainexecution.Running, 1)
	if err != nil {
		t.Fatal(err)
	}
	if status == domainexecution.Running {
		return run
	}
	run, err = run.Transition(status, 2)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestCanonicalCommandDigestsAreStableAndPayloadSensitive(t *testing.T) {
	cancel := CancelRunCommand{CommandID: "c", RunID: "run", ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2}
	first, err := CancelRunRequestDigest(cancel)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := CancelRunRequestDigest(cancel)
	cancel.At++
	changed, _ := CancelRunRequestDigest(cancel)
	if first != second || first == changed || len(first) != 71 {
		t.Fatalf("digests=%q/%q/%q", first, second, changed)
	}
	abort := AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "token"}}
	if digest, err := AbortRunRequestDigest(abort); err != nil || len(digest) != 71 {
		t.Fatalf("abort digest=%q/%v", digest, err)
	}
	reorder := ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, RunIDs: []string{"a", "b"}}
	digest, err := ReorderQueueRequestDigest(reorder)
	if err != nil {
		t.Fatal(err)
	}
	reorder.RunIDs[0] = "mutated"
	original, _ := ReorderQueueRequestDigest(ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, RunIDs: []string{"a", "b"}})
	if digest != original {
		t.Fatal("reorder digest retained caller slice")
	}
}

func TestRunServicesRejectMalformedAppliedAndReplayedLifecycleBeforeSignal(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*domainexecution.Run, *int64)
	}{
		{"missing task", func(run *domainexecution.Run, _ *int64) { run.ExecutionFlowID = "" }},
		{"missing version", func(run *domainexecution.Run, _ *int64) { run.TestTaskVersionID = "" }},
		{"missing environment", func(run *domainexecution.Run, _ *int64) { run.EnvironmentID = "" }},
		{"invalid finished timestamp", func(run *domainexecution.Run, _ *int64) { run.FinishedAt = 0 }},
		{"invalid started timestamp", func(run *domainexecution.Run, _ *int64) { run.StartedAt = 3 }},
		{"missing snapshot seal", func(run *domainexecution.Run, _ *int64) {
			*run = domainexecution.Run{ID: run.ID, ExecutionFlowID: run.ExecutionFlowID, TestTaskVersionID: run.TestTaskVersionID, EnvironmentID: run.EnvironmentID, Status: run.Status, CreatedAt: run.CreatedAt, QueuedAt: run.QueuedAt, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt}
		}},
		{"invalid snapshot seal", func(run *domainexecution.Run, _ *int64) {
			run.SnapshotSchemaVersion = domainexecution.RunSnapshotSchemaV1
			run.SnapshotDigest = "sha256:" + strings.Repeat("a", 64)
		}},
		{"wrong revision", func(_ *domainexecution.Run, revision *int64) { *revision = 3 }},
	}
	for _, applied := range []bool{true, false} {
		for _, mutation := range mutations {
			t.Run(fmt.Sprintf("applied=%v/%s", applied, mutation.name), func(t *testing.T) {
				for _, operation := range []string{"cancel", "abort"} {
					store := &commandStoreStub{}
					status := domainexecution.Canceled
					if operation == "abort" {
						status = domainexecution.Aborted
					}
					result := RunCommandResult{Run: validCommandRun(t, status), Revision: 2, WasApplied: applied, SignalRequired: true}
					mutation.mutate(&result.Run, &result.Revision)
					store.cancelResult, store.abortResult = result, result
					signal := signalStub{store: store}
					var got RunCommandResult
					var err error
					if operation == "cancel" {
						got, err = NewCancelRunService(store, signal).CancelRun(context.Background(), CancelRunCommand{CommandID: "c", RunID: "run", ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2})
					} else {
						got, err = NewAbortRunService(store, signal).AbortRun(context.Background(), AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "token"}})
					}
					if !errors.Is(err, ErrRunAdapterContract) || got != (RunCommandResult{}) || len(store.calls) != 1 {
						t.Fatalf("operation/result/error/calls=%s/%#v/%v/%v", operation, got, err, store.calls)
					}
				}
			})
		}
	}
}

func TestServicesRejectMalformedAuthoritativeAdapterOutcomes(t *testing.T) {
	cancelStore := &commandStoreStub{cancelResult: RunCommandResult{Run: domainexecution.Run{ID: "foreign", Status: domainexecution.Canceled}, Revision: 2}}
	result, err := NewCancelRunService(cancelStore, nil).CancelRun(context.Background(), CancelRunCommand{CommandID: "c", RunID: "run", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2})
	if !errors.Is(err, ErrRunIdentityConflict) || result != (RunCommandResult{}) {
		t.Fatalf("cancel result/error=%#v/%v", result, err)
	}
	abortStore := &commandStoreStub{abortResult: RunCommandResult{Run: validCommandRun(t, domainexecution.Aborted), Revision: 2}}
	result, err = NewAbortRunService(abortStore, nil).AbortRun(context.Background(), AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "token"}})
	if err == nil || result != (RunCommandResult{}) {
		t.Fatalf("abort result/error=%#v/%v", result, err)
	}
	queue := &queueStoreStub{result: ReorderQueueResult{ScopeID: "scope", Revision: 2, RunIDs: []string{"a", "b"}}}
	queueResult, err := NewReorderQueueService(queue).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, RunIDs: []string{"b", "a"}})
	if err == nil || queueResult.ScopeID != "" || queueResult.Revision != 0 || queueResult.RunIDs != nil || queueResult.WasApplied {
		t.Fatalf("queue result/error=%#v/%v", queueResult, err)
	}
}

type commandStoreStub struct {
	cancelResult RunCommandResult
	abortResult  RunCommandResult
	cancelErr    error
	abortErr     error
	calls        []string
}

func (s *commandStoreStub) Cancel(context.Context, CancelRunCommand) (RunCommandResult, error) {
	s.calls = append(s.calls, "commit-cancel")
	return s.cancelResult, s.cancelErr
}
func (s *commandStoreStub) Abort(context.Context, AbortRunCommand) (RunCommandResult, error) {
	s.calls = append(s.calls, "commit-abort")
	return s.abortResult, s.abortErr
}

type signalStub struct {
	store *commandStoreStub
	err   error
}

func (s signalStub) SignalRunCancellation(context.Context, string) error {
	s.store.calls = append(s.store.calls, "signal")
	return s.err
}

func TestCancelRejectsSignalRequirementThatDisagreesWithExpectedStatus(t *testing.T) {
	for _, test := range []struct {
		status domainexecution.RunStatus
		signal bool
	}{{domainexecution.Queued, true}, {domainexecution.Running, false}} {
		store := &commandStoreStub{cancelResult: RunCommandResult{Run: validCommandRun(t, domainexecution.Canceled), Revision: 2, WasApplied: true, SignalRequired: test.signal}}
		result, err := NewCancelRunService(store, signalStub{store: store}).CancelRun(context.Background(), CancelRunCommand{CommandID: "c", RunID: "run", ExpectedStatus: test.status, ExpectedRevision: 1, At: 2})
		if err == nil || result != (RunCommandResult{}) || !reflect.DeepEqual(store.calls, []string{"commit-cancel"}) {
			t.Fatalf("status/signal/result/error/calls=%s/%v/%#v/%v/%v", test.status, test.signal, result, err, store.calls)
		}
	}
}

func TestRunningCancelAndAbortSignalOnlyAfterAtomicCommit(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*commandStoreStub, signalStub) (RunCommandResult, error)
		want []string
	}{
		{"cancel", func(store *commandStoreStub, signal signalStub) (RunCommandResult, error) {
			return NewCancelRunService(store, signal).CancelRun(context.Background(), CancelRunCommand{CommandID: "c", RunID: "run", ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2})
		}, []string{"commit-cancel", "signal"}},
		{"abort", func(store *commandStoreStub, signal signalStub) (RunCommandResult, error) {
			return NewAbortRunService(store, signal).AbortRun(context.Background(), AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "unique"}})
		}, []string{"commit-abort", "signal"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &commandStoreStub{}
			store.cancelResult = RunCommandResult{Run: validCommandRun(t, domainexecution.Canceled), Revision: 2, WasApplied: true, SignalRequired: true}
			store.abortResult = RunCommandResult{Run: validCommandRun(t, domainexecution.Aborted), Revision: 2, WasApplied: true, SignalRequired: true}
			result, err := tc.call(store, signalStub{store: store})
			if err != nil || result.Revision != 2 || !reflect.DeepEqual(store.calls, tc.want) {
				t.Fatalf("result/calls/error=%#v/%v/%v", result, store.calls, err)
			}
		})
	}
}

func TestSignalFailureReturnsAuthoritativeCommittedResultAndRetryableError(t *testing.T) {
	store := &commandStoreStub{abortResult: RunCommandResult{Run: validCommandRun(t, domainexecution.Aborted), Revision: 2, WasApplied: true, SignalRequired: true}}
	store.abortResult.Run.ID = "run"
	sensitiveCause := errors.New("host signal failure: secret-token")
	result, err := NewAbortRunService(store, signalStub{store: store, err: sensitiveCause}).AbortRun(context.Background(), AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "unique"}})
	if !fault.IsCode(err, CodeRunSignalRetryable) || !errors.Is(err, sensitiveCause) || result.Run.Status != domainexecution.Aborted || !result.WasApplied {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
	if strings.Contains(err.Error(), "sensitive-run-id") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("public retry error leaked sensitive details: %v", err)
	}
	store.abortResult.WasApplied = false
	if _, err = NewAbortRunService(store, signalStub{store: store}).AbortRun(context.Background(), AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "unique"}}); err != nil {
		t.Fatalf("reconciliation signal: %v", err)
	}
	if !reflect.DeepEqual(store.calls, []string{"commit-abort", "signal", "commit-abort", "signal"}) {
		t.Fatalf("calls=%v", store.calls)
	}
}

func TestTransactionErrorsExposeNoNonAuthoritativeResult(t *testing.T) {
	store := &commandStoreStub{abortErr: domainexecution.NewStaleWorkerFenceError()}
	result, err := NewAbortRunService(store, nil).AbortRun(context.Background(), AbortRunCommand{CommandID: "a", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "stale"}})
	if !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) || result != (RunCommandResult{}) {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
}

type queueStoreStub struct {
	result ReorderQueueResult
	err    error
	seen   ReorderQueueCommand
}

func (s *queueStoreStub) Reorder(_ context.Context, command ReorderQueueCommand) (ReorderQueueResult, error) {
	s.seen = command
	return s.result, s.err
}
func TestReorderClonesAuthoritativePermutationAndRejectsDuplicates(t *testing.T) {
	store := &queueStoreStub{result: ReorderQueueResult{ScopeID: "scope", Revision: 2, RunIDs: []string{"b", "a"}, WasApplied: true}}
	result, err := NewReorderQueueService(store).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, RunIDs: []string{"b", "a"}})
	if err != nil || !reflect.DeepEqual(result.RunIDs, []string{"b", "a"}) {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
	store.result.RunIDs[0] = "mutated"
	if result.RunIDs[0] != "b" {
		t.Fatal("result aliases store state")
	}
	_, err = NewReorderQueueService(store).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r2", ScopeID: "scope", ExpectedRevision: 2, RunIDs: []string{"a", "a"}})
	if !errors.Is(err, ErrQueueMembershipConflict) {
		t.Fatalf("duplicate error=%v", err)
	}
}
