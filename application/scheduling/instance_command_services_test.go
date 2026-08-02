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

func validCommandInstance(t *testing.T, status domainexecution.InstanceStatus) domainexecution.Instance {
	t.Helper()
	command := validCreateInstanceCommand()
	command.InstanceID = mustInstanceID("run")
	resolved := validResolvedCreateInstance(t, command)
	snapshot, err := BuildInstanceSnapshot(command, resolved)
	if err != nil {
		t.Fatal(err)
	}
	run, err := domainexecution.NewInstance(domainexecution.Instance{ID: mustInstanceID("run"), ExecutionFlowID: "task", TestTaskVersionID: "task-v1", EnvironmentID: "env", Status: domainexecution.Queued, CreatedAt: 1, QueuedAt: 1}, snapshot)
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
	cancel := CancelInstanceCommand{CommandID: "c", InstanceID: mustInstanceID("run"), ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2}
	first, err := CancelInstanceRequestDigest(cancel)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := CancelInstanceRequestDigest(cancel)
	cancel.At++
	changed, _ := CancelInstanceRequestDigest(cancel)
	if first != second || first == changed || len(first) != 71 {
		t.Fatalf("digests=%q/%q/%q", first, second, changed)
	}
	abort := AbortInstanceCommand{CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "token"}}
	if digest, err := AbortInstanceRequestDigest(abort); err != nil || len(digest) != 71 {
		t.Fatalf("abort digest=%q/%v", digest, err)
	}
	reorder := ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, InstanceIDs: []string{"a", "b"}}
	digest, err := ReorderQueueRequestDigest(reorder)
	if err != nil {
		t.Fatal(err)
	}
	reorder.InstanceIDs[0] = "mutated"
	original, _ := ReorderQueueRequestDigest(ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, InstanceIDs: []string{"a", "b"}})
	if digest != original {
		t.Fatal("reorder digest retained caller slice")
	}
}

func TestInstanceServicesRejectMalformedAppliedAndReplayedLifecycleBeforeSignal(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*domainexecution.Instance, *int64)
	}{
		{"missing task", func(run *domainexecution.Instance, _ *int64) { run.ExecutionFlowID = "" }},
		{"missing version", func(run *domainexecution.Instance, _ *int64) { run.TestTaskVersionID = "" }},
		{"missing environment", func(run *domainexecution.Instance, _ *int64) { run.EnvironmentID = "" }},
		{"invalid finished timestamp", func(run *domainexecution.Instance, _ *int64) { run.FinishedAt = 0 }},
		{"invalid started timestamp", func(run *domainexecution.Instance, _ *int64) { run.StartedAt = 3 }},
		{"missing snapshot seal", func(run *domainexecution.Instance, _ *int64) {
			*run = domainexecution.Instance{ID: run.ID, ExecutionFlowID: run.ExecutionFlowID, TestTaskVersionID: run.TestTaskVersionID, EnvironmentID: run.EnvironmentID, Status: run.Status, CreatedAt: run.CreatedAt, QueuedAt: run.QueuedAt, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt}
		}},
		{"invalid snapshot seal", func(run *domainexecution.Instance, _ *int64) {
			run.SnapshotSchemaVersion = domainexecution.InstanceSnapshotSchemaV1
			run.SnapshotDigest = "sha256:" + strings.Repeat("a", 64)
		}},
		{"wrong revision", func(_ *domainexecution.Instance, revision *int64) { *revision = 3 }},
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
					result := InstanceCommandResult{Run: validCommandInstance(t, status), Revision: 2, WasApplied: applied, SignalRequired: true}
					mutation.mutate(&result.Run, &result.Revision)
					store.cancelResult, store.abortResult = result, result
					signal := signalStub{store: store}
					var got InstanceCommandResult
					var err error
					if operation == "cancel" {
						got, err = NewCancelInstanceService(store, signal).CancelInstance(context.Background(), CancelInstanceCommand{CommandID: "c", InstanceID: mustInstanceID("run"), ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2})
					} else {
						got, err = NewAbortInstanceService(store, signal).AbortInstance(context.Background(), AbortInstanceCommand{CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "token"}})
					}
					if !fault.IsCode(err, CodeInstanceAdapterContractViolation) || got != (InstanceCommandResult{}) || len(store.calls) != 1 {
						t.Fatalf("operation/result/error/calls=%s/%#v/%v/%v", operation, got, err, store.calls)
					}
				}
			})
		}
	}
}

func TestServicesRejectMalformedAuthoritativeAdapterOutcomes(t *testing.T) {
	cancelStore := &commandStoreStub{cancelResult: InstanceCommandResult{Run: domainexecution.Instance{ID: mustInstanceID("foreign"), Status: domainexecution.Canceled}, Revision: 2}}
	result, err := NewCancelInstanceService(cancelStore, nil).CancelInstance(context.Background(), CancelInstanceCommand{CommandID: "c", InstanceID: mustInstanceID("run"), ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2})
	if !fault.IsCode(err, CodeInstanceIdentityConflict) || result != (InstanceCommandResult{}) {
		t.Fatalf("cancel result/error=%#v/%v", result, err)
	}
	abortStore := &commandStoreStub{abortResult: InstanceCommandResult{Run: validCommandInstance(t, domainexecution.Aborted), Revision: 2}}
	result, err = NewAbortInstanceService(abortStore, nil).AbortInstance(context.Background(), AbortInstanceCommand{CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "token"}})
	if err == nil || result != (InstanceCommandResult{}) {
		t.Fatalf("abort result/error=%#v/%v", result, err)
	}
	queue := &queueStoreStub{result: ReorderQueueResult{ScopeID: "scope", Revision: 2, InstanceIDs: []string{"a", "b"}}}
	queueResult, err := NewReorderQueueService(queue).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, InstanceIDs: []string{"b", "a"}})
	if err == nil || queueResult.ScopeID != "" || queueResult.Revision != 0 || queueResult.InstanceIDs != nil || queueResult.WasApplied {
		t.Fatalf("queue result/error=%#v/%v", queueResult, err)
	}
}

type commandStoreStub struct {
	cancelResult InstanceCommandResult
	abortResult  InstanceCommandResult
	cancelErr    error
	abortErr     error
	calls        []string
}

func (s *commandStoreStub) Cancel(context.Context, CancelInstanceCommand) (InstanceCommandResult, error) {
	s.calls = append(s.calls, "commit-cancel")
	return s.cancelResult, s.cancelErr
}
func (s *commandStoreStub) Abort(context.Context, AbortInstanceCommand) (InstanceCommandResult, error) {
	s.calls = append(s.calls, "commit-abort")
	return s.abortResult, s.abortErr
}

type signalStub struct {
	store *commandStoreStub
	err   error
}

func (s signalStub) SignalInstanceCancellation(context.Context, domainexecution.InstanceID) error {
	s.store.calls = append(s.store.calls, "signal")
	return s.err
}

type typedNilSignaler struct{}

func (*typedNilSignaler) SignalInstanceCancellation(context.Context, domainexecution.InstanceID) error {
	panic("typed nil signaler invoked")
}

type functionSignaler func(context.Context, string) error

func (signaler functionSignaler) SignalInstanceCancellation(ctx context.Context, instanceID domainexecution.InstanceID) error {
	return signaler(ctx, instanceID.String())
}

type channelSignaler chan struct{}

func (channelSignaler) SignalInstanceCancellation(context.Context, domainexecution.InstanceID) error {
	return nil
}

type mapSignaler map[string]struct{}

func (mapSignaler) SignalInstanceCancellation(context.Context, domainexecution.InstanceID) error {
	return nil
}

type sliceSignaler []string

func (sliceSignaler) SignalInstanceCancellation(context.Context, domainexecution.InstanceID) error {
	return nil
}

func TestCancelRejectsSignalRequirementThatDisagreesWithExpectedStatus(t *testing.T) {
	for _, test := range []struct {
		status domainexecution.InstanceStatus
		signal bool
	}{{domainexecution.Queued, true}, {domainexecution.Running, false}} {
		store := &commandStoreStub{cancelResult: InstanceCommandResult{Run: validCommandInstance(t, domainexecution.Canceled), Revision: 2, WasApplied: true, SignalRequired: test.signal}}
		result, err := NewCancelInstanceService(store, signalStub{store: store}).CancelInstance(context.Background(), CancelInstanceCommand{CommandID: "c", InstanceID: mustInstanceID("run"), ExpectedStatus: test.status, ExpectedRevision: 1, At: 2})
		if err == nil || result != (InstanceCommandResult{}) || !reflect.DeepEqual(store.calls, []string{"commit-cancel"}) {
			t.Fatalf("status/signal/result/error/calls=%s/%v/%#v/%v/%v", test.status, test.signal, result, err, store.calls)
		}
	}
}

func TestRunningInstanceCancelAndAbortSignalOnlyAfterAtomicCommit(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*commandStoreStub, signalStub) (InstanceCommandResult, error)
		want []string
	}{
		{"cancel", func(store *commandStoreStub, signal signalStub) (InstanceCommandResult, error) {
			return NewCancelInstanceService(store, signal).CancelInstance(context.Background(), CancelInstanceCommand{CommandID: "c", InstanceID: mustInstanceID("run"), ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2})
		}, []string{"commit-cancel", "signal"}},
		{"abort", func(store *commandStoreStub, signal signalStub) (InstanceCommandResult, error) {
			return NewAbortInstanceService(store, signal).AbortInstance(context.Background(), AbortInstanceCommand{CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "unique"}})
		}, []string{"commit-abort", "signal"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &commandStoreStub{}
			store.cancelResult = InstanceCommandResult{Run: validCommandInstance(t, domainexecution.Canceled), Revision: 2, WasApplied: true, SignalRequired: true}
			store.abortResult = InstanceCommandResult{Run: validCommandInstance(t, domainexecution.Aborted), Revision: 2, WasApplied: true, SignalRequired: true}
			result, err := tc.call(store, signalStub{store: store})
			if err != nil || result.Revision != 2 || !reflect.DeepEqual(store.calls, tc.want) {
				t.Fatalf("result/calls/error=%#v/%v/%v", result, store.calls, err)
			}
		})
	}
}

func TestTypedNilSignalerReturnsRedactedRetryableFault(t *testing.T) {
	var pointerSignaler *typedNilSignaler
	var funcSignaler functionSignaler
	var channelSignaler channelSignaler
	var mapSignaler mapSignaler
	var sliceSignaler sliceSignaler
	for _, test := range []struct {
		name     string
		signaler InstanceCancellationSignaler
	}{
		{name: "pointer", signaler: pointerSignaler},
		{name: "function", signaler: funcSignaler},
		{name: "channel", signaler: channelSignaler},
		{name: "map", signaler: mapSignaler},
		{name: "slice", signaler: sliceSignaler},
	} {
		t.Run(test.name, func(t *testing.T) {
			committed := InstanceCommandResult{Run: validCommandInstance(t, domainexecution.Aborted), Revision: 2, WasApplied: false, SignalRequired: true}
			store := &commandStoreStub{abortResult: committed}

			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("typed-nil signaler panicked: %v", recovered)
				}
			}()
			result, err := NewAbortInstanceService(store, test.signaler).AbortInstance(context.Background(), AbortInstanceCommand{
				CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2,
				Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "unique"},
			})
			if !fault.IsCode(err, CodeInstanceSignalRetryable) || !reflect.DeepEqual(result, committed) {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			if strings.Contains(err.Error(), "typed nil") || strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("public retry error leaked internal details: %v", err)
			}
		})
	}
}

func TestSignalFailureReturnsAuthoritativeCommittedResultAndRetryableError(t *testing.T) {
	store := &commandStoreStub{abortResult: InstanceCommandResult{Run: validCommandInstance(t, domainexecution.Aborted), Revision: 2, WasApplied: true, SignalRequired: true}}
	store.abortResult.Run.ID = mustInstanceID("run")
	sensitiveCause := errors.New("host signal failure: secret-token")
	result, err := NewAbortInstanceService(store, signalStub{store: store, err: sensitiveCause}).AbortInstance(context.Background(), AbortInstanceCommand{CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "unique"}})
	if !fault.IsCode(err, CodeInstanceSignalRetryable) || !errors.Is(err, sensitiveCause) || result.Run.Status != domainexecution.Aborted || !result.WasApplied {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
	if strings.Contains(err.Error(), "sensitive-run-id") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("public retry error leaked sensitive details: %v", err)
	}
	store.abortResult.WasApplied = false
	if _, err = NewAbortInstanceService(store, signalStub{store: store}).AbortInstance(context.Background(), AbortInstanceCommand{CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "unique"}}); err != nil {
		t.Fatalf("reconciliation signal: %v", err)
	}
	if !reflect.DeepEqual(store.calls, []string{"commit-abort", "signal", "commit-abort", "signal"}) {
		t.Fatalf("calls=%v", store.calls)
	}
}

func TestTransactionErrorsExposeNoNonAuthoritativeResult(t *testing.T) {
	store := &commandStoreStub{abortErr: domainexecution.NewStaleWorkerFenceError()}
	result, err := NewAbortInstanceService(store, nil).AbortInstance(context.Background(), AbortInstanceCommand{CommandID: "a", InstanceID: mustInstanceID("run"), ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{InstanceID: mustInstanceID("run"), ClaimToken: "stale"}})
	if !fault.IsCode(err, domainexecution.CodeWorkerFenceStale) || result != (InstanceCommandResult{}) {
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
	store := &queueStoreStub{result: ReorderQueueResult{ScopeID: "scope", Revision: 2, InstanceIDs: []string{"b", "a"}, WasApplied: true}}
	result, err := NewReorderQueueService(store).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r", ScopeID: "scope", ExpectedRevision: 1, InstanceIDs: []string{"b", "a"}})
	if err != nil || !reflect.DeepEqual(result.InstanceIDs, []string{"b", "a"}) {
		t.Fatalf("result/error=%#v/%v", result, err)
	}
	store.result.InstanceIDs[0] = "mutated"
	if result.InstanceIDs[0] != "b" {
		t.Fatal("result aliases store state")
	}
	_, err = NewReorderQueueService(store).ReorderQueue(context.Background(), ReorderQueueCommand{CommandID: "r2", ScopeID: "scope", ExpectedRevision: 2, InstanceIDs: []string{"a", "a"}})
	if !fault.IsCode(err, CodeQueueMembershipConflict) {
		t.Fatalf("duplicate error=%v", err)
	}
}
