package scheduling

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestRunCommandErrorsExposeStableContracts(t *testing.T) {
	cause := errors.New("dependency failed: secret-token")
	tests := []struct {
		name    string
		err     error
		code    fault.Code
		kind    fault.Kind
		message string
		cause   error
	}{
		{name: "command identity", err: runCommandConflictError(), code: CodeInstanceCommandIdentityConflict, kind: fault.Conflict, message: "instance command identity conflicts with an existing request"},
		{name: "run identity", err: runIdentityConflictError(), code: CodeInstanceIdentityConflict, kind: fault.Conflict, message: "instance identity conflicts with the authoritative state"},
		{name: "run revision", err: runRevisionConflictError(), code: CodeInstanceRevisionConflict, kind: fault.Conflict, message: "instance revision conflicts with current state"},
		{name: "run status", err: runStatusConflictError(), code: CodeInstanceStatusConflict, kind: fault.Conflict, message: "instance status conflicts with current state"},
		{name: "queue revision", err: queueRevisionConflictError(), code: CodeQueueRevisionConflict, kind: fault.Conflict, message: "queue revision conflicts with current state"},
		{name: "queue membership", err: queueMembershipConflictError(), code: CodeQueueMembershipConflict, kind: fault.Conflict, message: "queue membership conflicts with the authoritative state"},
		{name: "adapter contract", err: runAdapterContractViolationError(cause), code: CodeInstanceAdapterContractViolation, kind: fault.Internal, message: "instance command adapter returned an invalid authoritative result", cause: cause},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor, ok := fault.Describe(test.err)
			if !ok || descriptor.Code() != test.code || descriptor.Kind() != test.kind || descriptor.Message() != test.message || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
				t.Fatalf("descriptor = %#v, ok = %v", descriptor, ok)
			}
			if test.cause != nil && !errors.Is(test.err, test.cause) {
				t.Fatalf("errors.Is(%v, cause) = false", test.err)
			}
			if strings.Contains(test.err.Error(), "secret-token") || strings.Contains(test.err.Error(), "dependency failed") || strings.Contains(test.err.Error(), "command-sensitive-id") || strings.Contains(test.err.Error(), "run-sensitive-id") || strings.Contains(test.err.Error(), "scope-sensitive-id") {
				t.Fatalf("public error leaked sensitive detail: %q", test.err.Error())
			}
		})
	}
}

func TestRunSignalRetryableErrorPreservesCauseAndRedactsPublicDetails(t *testing.T) {
	cause := errors.New("adapter failure: secret-token")
	err := runSignalRetryableError(cause)
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Code() != CodeInstanceSignalRetryable || descriptor.Kind() != fault.Unavailable {
		t.Fatalf("fault descriptor = %#v, ok = %v", descriptor, ok)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, cause) = false", err)
	}
	if got := err.Error(); got != "EXECUTION_INSTANCE_SIGNAL_RETRYABLE: execution cancellation signal must be retried" || strings.Contains(got, "secret-token") {
		t.Fatalf("public retry error = %q", got)
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
			if !fault.IsCode(err, CodeCancelInstanceCommandInvalid) {
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
			if !fault.IsCode(err, CodeAbortInstanceCommandInvalid) {
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
			if !fault.IsCode(err, CodeInstanceSignalRetryable) || !result.WasApplied || result.Revision != 2 {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			if got := err.Error(); got != "EXECUTION_INSTANCE_SIGNAL_RETRYABLE: execution cancellation signal must be retried" {
				t.Fatalf("public error = %q", got)
			}
			if strings.Contains(err.Error(), "cancellation signaler is unavailable") {
				t.Fatalf("public error leaked internal details: %v", err)
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
		{name: "duplicate member", mutate: func(command *ReorderQueueCommand) { command.RunIDs = []string{"a", "a"} }, target: CodeQueueMembershipConflict},
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
			if test.target == nil && !fault.IsCode(err, CodeReorderQueueCommandInvalid) {
				t.Fatalf("ReorderQueue() error = %v", err)
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
			if test.err == nil && !fault.IsCode(err, CodeInstanceAdapterContractViolation) {
				t.Fatalf("ReorderQueue() malformed-result error = %v", err)
			}
		})
	}
}

func TestInvalidRunCommandFaultsExposeSafeStableContracts(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    fault.Code
		message string
	}{
		{name: "cancel", err: cancelRunCommandInvalidError(errors.New("command=command-secret value=credential-secret")), code: CodeCancelInstanceCommandInvalid, message: "cancel instance command is invalid"},
		{name: "abort", err: abortRunCommandInvalidError(errors.New("fence=claim-secret value=credential-secret")), code: CodeAbortInstanceCommandInvalid, message: "abort instance command is invalid"},
		{name: "reorder", err: reorderQueueCommandInvalidError(errors.New("scope=scope-secret run=run-secret")), code: CodeReorderQueueCommandInvalid, message: "reorder queue command is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor, ok := fault.Describe(test.err)
			if !ok || descriptor.Code() != test.code || descriptor.Kind() != fault.InvalidArgument || descriptor.Message() != test.message || len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 {
				t.Fatalf("descriptor/error = %#v/%v", descriptor, test.err)
			}
			for _, sensitive := range []string{"command-secret", "credential-secret", "claim-secret", "scope-secret", "run-secret"} {
				if strings.Contains(test.err.Error(), sensitive) {
					t.Fatalf("public error leaked %q: %q", sensitive, test.err.Error())
				}
			}
		})
	}
}
