package execution

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/application/engine"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// outcomeRunner is an EntryRunner that reports whatever engine result the test
// hands it, so the executor's plumbing is observable on its own.
type outcomeRunner struct {
	result engine.EntryResult
	err    error
	calls  int
}

func (r *outcomeRunner) RunEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) (engine.EntryResult, error) {
	r.calls++
	return r.result, r.err
}

type outcomeSession struct{ closeErr error }

func (outcomeSession) Valid() bool { return true }

func (s outcomeSession) Close(context.Context) error { return s.closeErr }

type outcomeFactory struct {
	session BrowserSession
	err     error
}

func (f outcomeFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.Entry) (BrowserSession, error) {
	return f.session, f.err
}

type refusingAuthorizer struct{ err error }

func (a refusingAuthorizer) AuthorizeEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry) error {
	return a.err
}

func outcomeExecutor(t *testing.T, authorizer EntryAuthorizer, factory BrowserSessionFactory, runner EntryRunner) EntryExecutor {
	t.Helper()
	executor, err := NewEntryExecutor(authorizer, factory, runner, time.Second)
	if err != nil {
		t.Fatalf("new entry executor: %v", err)
	}
	return executor
}

func outcomeFence() domainexecution.WorkerFence {
	return domainexecution.WorkerFence{InstanceID: mustInstanceID("instance-1"), ClaimToken: "claim-1"}
}

func outcomeEntry() domainexecution.Entry {
	return domainexecution.Entry{ID: mustEntryID("entry-1")}
}

func TestEntryExecutorReturnsTheEngineOutcomeToTheCaller(t *testing.T) {
	want := engine.EntryResult{
		ExecutionOutcome: engine.OutcomeSucceeded,
		RecordingOutcome: engine.RecordingStopFailed,
		TimelineOutcome:  engine.TimelineComplete,
	}
	runner := &outcomeRunner{result: want}
	executor := outcomeExecutor(t, alwaysAuthorized{}, outcomeFactory{session: outcomeSession{}}, runner)

	outcome, err := executor.Execute(context.Background(), outcomeFence(), outcomeEntry())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !reflect.DeepEqual(outcome.Result, want) {
		t.Fatalf("engine result must reach the caller\n got %+v\nwant %+v", outcome.Result, want)
	}
	if outcome.FailureCode != "" {
		t.Fatalf("a successful entry must carry no failure code, got %q", outcome.FailureCode)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d want 1", runner.calls)
	}
}

func TestEntryExecutorReturnsTheEngineOutcomeAlongsideTheRunnerError(t *testing.T) {
	cause := errors.New("engine gave up")
	want := engine.EntryResult{
		ExecutionOutcome: engine.OutcomeFailed,
		RecordingOutcome: engine.RecordingSucceeded,
		TimelineOutcome:  engine.TimelineFinishFailed,
	}
	executor := outcomeExecutor(t, alwaysAuthorized{}, outcomeFactory{session: outcomeSession{}}, &outcomeRunner{result: want, err: cause})

	outcome, err := executor.Execute(context.Background(), outcomeFence(), outcomeEntry())
	if !errors.Is(err, cause) {
		t.Fatalf("runner error must reach the caller, got %v", err)
	}
	if !reflect.DeepEqual(outcome.Result, want) {
		t.Fatalf("a failed entry must still report what the engine observed\n got %+v\nwant %+v", outcome.Result, want)
	}
}

func TestEntryExecutorKeepsTheEngineOutcomeWhenTheSessionFailsToClose(t *testing.T) {
	closeErr := errors.New("browser refused to close")
	want := engine.EntryResult{
		ExecutionOutcome: engine.OutcomeSucceeded,
		RecordingOutcome: engine.RecordingSucceeded,
		TimelineOutcome:  engine.TimelineComplete,
	}
	executor := outcomeExecutor(t, alwaysAuthorized{}, outcomeFactory{session: outcomeSession{closeErr: closeErr}}, &outcomeRunner{result: want})

	outcome, err := executor.Execute(context.Background(), outcomeFence(), outcomeEntry())
	if !errors.Is(err, closeErr) {
		t.Fatalf("close failure must reach the caller, got %v", err)
	}
	if !reflect.DeepEqual(outcome.Result, want) {
		t.Fatalf("a teardown failure must not rewrite what the engine observed\n got %+v\nwant %+v", outcome.Result, want)
	}
}

func TestEntryExecutorReportsNotStartedWhenTheEngineNeverRan(t *testing.T) {
	cases := map[string]struct {
		authorizer EntryAuthorizer
		factory    BrowserSessionFactory
		fence      domainexecution.WorkerFence
		wantCode   fault.Code
	}{
		"malformed fence": {
			authorizer: alwaysAuthorized{},
			factory:    outcomeFactory{session: outcomeSession{}},
			fence:      domainexecution.WorkerFence{InstanceID: mustInstanceID("instance-1")},
			wantCode:   domainexecution.CodeWorkerFenceInvalid,
		},
		"authorization refused": {
			authorizer: refusingAuthorizer{err: domainexecution.NewStaleWorkerFenceError()},
			factory:    outcomeFactory{session: outcomeSession{}},
			fence:      outcomeFence(),
			wantCode:   domainexecution.CodeWorkerFenceStale,
		},
		"session factory failed": {
			authorizer: alwaysAuthorized{},
			factory:    outcomeFactory{err: errors.New("no browser available")},
			fence:      outcomeFence(),
			wantCode:   CodeSchedulingAdapterUnavailable,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			runner := &outcomeRunner{}
			executor := outcomeExecutor(t, testCase.authorizer, testCase.factory, runner)

			outcome, err := executor.Execute(context.Background(), testCase.fence, outcomeEntry())
			if !fault.IsCode(err, testCase.wantCode) {
				t.Fatalf("want %q, got %v", testCase.wantCode, err)
			}
			if runner.calls != 0 {
				t.Fatalf("the runner must not run, calls=%d", runner.calls)
			}
			if outcome.Result != NotStartedEngineOutcome().Result {
				t.Fatalf("want a NOT_STARTED result, got %+v", outcome.Result)
			}
			if outcome.FailureCode != testCase.wantCode {
				t.Fatalf("outcome must carry the classified failure, got %q want %q", outcome.FailureCode, testCase.wantCode)
			}
		})
	}
}

// TestEntryExecutorOutcomeFeedsTheTerminalDecisionDirectly closes the loop the
// upstream contract asks for: the value Execute returns is the value
// DecideEntryCompletion consumes, with no host-side translation in between.
func TestEntryExecutorOutcomeFeedsTheTerminalDecisionDirectly(t *testing.T) {
	runner := &outcomeRunner{
		result: engine.EntryResult{
			ExecutionOutcome: engine.OutcomeFailed,
			RecordingOutcome: engine.RecordingDisabled,
			TimelineOutcome:  engine.TimelineDisabled,
		},
		err: errors.New("engine gave up"),
	}
	executor := outcomeExecutor(t, alwaysAuthorized{}, outcomeFactory{session: outcomeSession{}}, runner)

	outcome, err := executor.Execute(context.Background(), outcomeFence(), outcomeEntry())
	if err == nil {
		t.Fatalf("expected the runner failure")
	}

	decision, decideErr := DecideEntryCompletion(runningCompletionState(TerminalIntentCancel), outcome)
	if decideErr != nil {
		t.Fatalf("the executor outcome must be decidable as-is: %v", decideErr)
	}
	if decision.EntryStatus != domainexecution.EntryCanceled {
		t.Fatalf("want CANCELED, got %q", decision.EntryStatus)
	}
}

func TestNotStartedEngineOutcomeIsValidAndDecidable(t *testing.T) {
	outcome := NotStartedEngineOutcome()
	if err := outcome.Validate(); err != nil {
		t.Fatalf("the not-started outcome must be valid: %v", err)
	}
	decision, err := DecideEntryCompletion(runningCompletionState(TerminalIntentNone), outcome)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decision.EntryStatus != domainexecution.EntryFailed {
		t.Fatalf("an entry that never started and carries no intent is a failure, got %q", decision.EntryStatus)
	}
}
