package scheduling

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

func TestRunCommandErrorsExposeStableContracts(t *testing.T) {
	cause := errors.New("dependency failed")
	tests := []struct {
		name       string
		err        error
		wantText   string
		wantTarget error
		wantCause  error
	}{
		{name: "command identity", err: &CommandConflictError{CommandID: "command"}, wantText: "run command identity conflict: command", wantTarget: ErrRunCommandConflict},
		{name: "run identity", err: &RunIdentityConflictError{RunID: "run"}, wantText: "run identity conflict: run", wantTarget: ErrRunIdentityConflict},
		{name: "run revision", err: &RunRevisionConflictError{RunID: "run", Expected: 2, Actual: 3}, wantText: "run revision conflict: run expected 2 actual 3", wantTarget: ErrRunRevisionConflict},
		{name: "run status", err: &RunStatusConflictError{RunID: "run", Expected: domainexecution.Running, Actual: domainexecution.Succeeded}, wantText: "run status conflict: run expected RUNNING actual SUCCEEDED", wantTarget: ErrRunStatusConflict},
		{name: "queue revision", err: &QueueRevisionConflictError{ScopeID: "scope", Expected: 2, Actual: 3}, wantText: "queue revision conflict: scope expected 2 actual 3", wantTarget: ErrQueueRevisionConflict},
		{name: "queue membership", err: &QueueMembershipConflictError{ScopeID: "scope"}, wantText: "queue membership conflict: scope", wantTarget: ErrQueueMembershipConflict},
		{name: "adapter contract", err: &RunAdapterContractError{Operation: "validate", Cause: cause}, wantText: "run command adapter contract violation: validate: dependency failed", wantTarget: ErrRunAdapterContract, wantCause: cause},
		{name: "signal retry", err: &RunSignalRetryableError{RunID: "run", Cause: cause}, wantText: "run cancellation signal must be retried: run: dependency failed", wantTarget: ErrRunSignalRetryable, wantCause: cause},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.wantText {
				t.Fatalf("Error() = %q, want %q", got, test.wantText)
			}
			if !errors.Is(test.err, test.wantTarget) {
				t.Fatalf("errors.Is(%v, %v) = false", test.err, test.wantTarget)
			}
			if test.wantCause != nil && !errors.Is(test.err, test.wantCause) {
				t.Fatalf("errors.Is(%v, cause) = false", test.err)
			}
		})
	}
}

func TestCancelRunRejectsEachInvalidCommandBeforeStore(t *testing.T) {
	valid := CancelRunCommand{CommandID: "command", RunID: "run", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 0, At: 1}
	tests := []struct {
		name   string
		mutate func(*CancelRunCommand)
	}{
		{name: "blank command id", mutate: func(command *CancelRunCommand) { command.CommandID = " \t\n" }},
		{name: "blank run id", mutate: func(command *CancelRunCommand) { command.RunID = " \t\n" }},
		{name: "negative revision", mutate: func(command *CancelRunCommand) { command.ExpectedRevision = -1 }},
		{name: "zero timestamp", mutate: func(command *CancelRunCommand) { command.At = 0 }},
		{name: "negative timestamp", mutate: func(command *CancelRunCommand) { command.At = -1 }},
		{name: "unknown status", mutate: func(command *CancelRunCommand) { command.ExpectedStatus = "UNKNOWN" }},
		{name: "terminal status", mutate: func(command *CancelRunCommand) { command.ExpectedStatus = domainexecution.Succeeded }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := valid
			test.mutate(&command)
			store := &commandStoreStub{}
			result, err := NewCancelRunService(store, nil).CancelRun(context.Background(), command)
			if err == nil || !strings.Contains(err.Error(), "invalid cancel run command") {
				t.Fatalf("CancelRun() error = %v", err)
			}
			if result != (RunCommandResult{}) || len(store.calls) != 0 {
				t.Fatalf("rejected command result/calls = %#v/%v", result, store.calls)
			}
		})
	}
}

func TestAbortRunRejectsEachInvalidCommandBeforeStore(t *testing.T) {
	valid := AbortRunCommand{
		CommandID: "command", RunID: "run", ExpectedRevision: 0, At: 1,
		Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"},
	}
	tests := []struct {
		name   string
		mutate func(*AbortRunCommand)
	}{
		{name: "blank command id", mutate: func(command *AbortRunCommand) { command.CommandID = " \t\n" }},
		{name: "blank run id", mutate: func(command *AbortRunCommand) { command.RunID = " \t\n" }},
		{name: "negative revision", mutate: func(command *AbortRunCommand) { command.ExpectedRevision = -1 }},
		{name: "zero timestamp", mutate: func(command *AbortRunCommand) { command.At = 0 }},
		{name: "negative timestamp", mutate: func(command *AbortRunCommand) { command.At = -1 }},
		{name: "foreign fence", mutate: func(command *AbortRunCommand) { command.Fence.RunID = "other" }},
		{name: "empty claim token", mutate: func(command *AbortRunCommand) { command.Fence.ClaimToken = "" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := valid
			test.mutate(&command)
			store := &commandStoreStub{}
			result, err := NewAbortRunService(store, nil).AbortRun(context.Background(), command)
			if err == nil || !strings.Contains(err.Error(), "invalid abort run command") {
				t.Fatalf("AbortRun() error = %v", err)
			}
			if result != (RunCommandResult{}) || len(store.calls) != 0 {
				t.Fatalf("rejected command result/calls = %#v/%v", result, store.calls)
			}
		})
	}
}

func TestRunCommandServicesPropagateTransactionAndSignalFailures(t *testing.T) {
	transactionFailure := errors.New("transaction unavailable")
	for _, operation := range []string{"cancel", "abort"} {
		t.Run(operation+" transaction", func(t *testing.T) {
			store := &commandStoreStub{cancelErr: transactionFailure, abortErr: transactionFailure}
			var result RunCommandResult
			var err error
			if operation == "cancel" {
				result, err = NewCancelRunService(store, nil).CancelRun(context.Background(), CancelRunCommand{CommandID: "command", RunID: "run", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 0, At: 1})
			} else {
				result, err = NewAbortRunService(store, nil).AbortRun(context.Background(), AbortRunCommand{CommandID: "command", RunID: "run", ExpectedRevision: 0, At: 1, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}})
			}
			if !errors.Is(err, transactionFailure) || result != (RunCommandResult{}) {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			if len(store.calls) != 1 {
				t.Fatalf("store calls = %v", store.calls)
			}
		})
	}

	for _, operation := range []string{"cancel", "abort"} {
		t.Run(operation+" missing signaler", func(t *testing.T) {
			store := &commandStoreStub{
				cancelResult: RunCommandResult{Run: validCommandRun(t, domainexecution.Canceled), Revision: 2, WasApplied: true, SignalRequired: true},
				abortResult:  RunCommandResult{Run: validCommandRun(t, domainexecution.Aborted), Revision: 2, WasApplied: true, SignalRequired: true},
			}
			var result RunCommandResult
			var err error
			if operation == "cancel" {
				result, err = NewCancelRunService(store, nil).CancelRun(context.Background(), CancelRunCommand{CommandID: "command", RunID: "run", ExpectedStatus: domainexecution.Running, ExpectedRevision: 1, At: 2})
			} else {
				result, err = NewAbortRunService(store, nil).AbortRun(context.Background(), AbortRunCommand{CommandID: "command", RunID: "run", ExpectedRevision: 1, At: 2, Fence: domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}})
			}
			if !errors.Is(err, ErrRunSignalRetryable) || !result.WasApplied || result.Revision != 2 {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			if !strings.Contains(err.Error(), "cancellation signaler is unavailable") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestQueuedCancelReturnsWithoutSignaling(t *testing.T) {
	store := &commandStoreStub{cancelResult: RunCommandResult{
		Run: validCommandRun(t, domainexecution.Canceled), Revision: 2, WasApplied: true,
	}}
	result, err := NewCancelRunService(store, nil).CancelRun(context.Background(), CancelRunCommand{
		CommandID: "command", RunID: "run", ExpectedStatus: domainexecution.Queued, ExpectedRevision: 1, At: 2,
	})
	if err != nil || !result.WasApplied || result.SignalRequired {
		t.Fatalf("CancelRun() = (%#v, %v)", result, err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("calls = %v", store.calls)
	}
}

type countingQueueStore struct {
	result ReorderQueueResult
	err    error
	calls  int
}

func (store *countingQueueStore) Reorder(context.Context, ReorderQueueCommand) (ReorderQueueResult, error) {
	store.calls++
	return store.result, store.err
}

func TestReorderQueueRejectsEachInvalidCommandBeforeStore(t *testing.T) {
	valid := ReorderQueueCommand{CommandID: "command", ScopeID: "scope", ExpectedRevision: 0, RunIDs: []string{"a", "b"}}
	tests := []struct {
		name   string
		mutate func(*ReorderQueueCommand)
		target error
	}{
		{name: "blank command id", mutate: func(command *ReorderQueueCommand) { command.CommandID = " \t\n" }},
		{name: "blank scope id", mutate: func(command *ReorderQueueCommand) { command.ScopeID = " \t\n" }},
		{name: "negative revision", mutate: func(command *ReorderQueueCommand) { command.ExpectedRevision = -1 }},
		{name: "nil members", mutate: func(command *ReorderQueueCommand) { command.RunIDs = nil }},
		{name: "empty members", mutate: func(command *ReorderQueueCommand) { command.RunIDs = []string{} }},
		{name: "blank member", mutate: func(command *ReorderQueueCommand) { command.RunIDs = []string{"a", " \t\n"} }},
		{name: "duplicate member", mutate: func(command *ReorderQueueCommand) { command.RunIDs = []string{"a", "a"} }, target: ErrQueueMembershipConflict},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := valid
			command.RunIDs = append([]string(nil), valid.RunIDs...)
			test.mutate(&command)
			store := &countingQueueStore{}
			result, err := NewReorderQueueService(store).ReorderQueue(context.Background(), command)
			if err == nil {
				t.Fatal("ReorderQueue() error = nil")
			}
			if test.target != nil && !errors.Is(err, test.target) {
				t.Fatalf("ReorderQueue() error = %v, want %v", err, test.target)
			}
			if !reflect.DeepEqual(result, ReorderQueueResult{}) || store.calls != 0 {
				t.Fatalf("rejected command result/calls = %#v/%d", result, store.calls)
			}
		})
	}
}

func TestReorderQueueRejectsDependencyAndEveryMalformedAuthoritativeResult(t *testing.T) {
	command := ReorderQueueCommand{CommandID: "command", ScopeID: "scope", ExpectedRevision: 1, RunIDs: []string{"b", "a"}}
	dependencyFailure := errors.New("queue transaction failed")
	tests := []struct {
		name   string
		result ReorderQueueResult
		err    error
	}{
		{name: "dependency failure", err: dependencyFailure},
		{name: "wrong scope", result: ReorderQueueResult{ScopeID: "other", Revision: 2, RunIDs: []string{"b", "a"}}},
		{name: "wrong revision", result: ReorderQueueResult{ScopeID: "scope", Revision: 3, RunIDs: []string{"b", "a"}}},
		{name: "missing member", result: ReorderQueueResult{ScopeID: "scope", Revision: 2, RunIDs: []string{"b"}}},
		{name: "extra member", result: ReorderQueueResult{ScopeID: "scope", Revision: 2, RunIDs: []string{"b", "a", "c"}}},
		{name: "wrong order", result: ReorderQueueResult{ScopeID: "scope", Revision: 2, RunIDs: []string{"a", "b"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &countingQueueStore{result: test.result, err: test.err}
			result, err := NewReorderQueueService(store).ReorderQueue(context.Background(), command)
			if err == nil || !reflect.DeepEqual(result, ReorderQueueResult{}) || store.calls != 1 {
				t.Fatalf("ReorderQueue() = (%#v, %v), calls = %d", result, err, store.calls)
			}
			if test.err != nil && !errors.Is(err, test.err) {
				t.Fatalf("ReorderQueue() error = %v, want dependency error", err)
			}
		})
	}
}
