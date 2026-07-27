package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

type sessionFixture struct {
	id     string
	events *[]string
	err    error
}

func (s *sessionFixture) Valid() bool { return s != nil }

func (s *sessionFixture) Close(context.Context) error {
	*s.events = append(*s.events, "close:"+s.id)
	return s.err
}

type sessionFactoryFixture struct {
	events    *[]string
	createErr error
	closeErr  error
	sessions  []*sessionFixture
}

func (f *sessionFactoryFixture) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.WorkflowEntry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	if f.createErr != nil {
		return nil, f.createErr
	}
	session := &sessionFixture{id: entry.ExecutionID, events: f.events, err: f.closeErr}
	f.sessions = append(f.sessions, session)
	return session, nil
}

type entryRunnerFixture struct {
	events *[]string
	err    error
	seen   []BrowserSession
}

func (r *entryRunnerFixture) RunEntry(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.WorkflowEntry, session BrowserSession) error {
	*r.events = append(*r.events, "run:"+entry.ExecutionID)
	r.seen = append(r.seen, session)
	return r.err
}

func mustEntryExecutor(t *testing.T, factory BrowserSessionFactory, runner EntryRunner) EntryExecutor {
	t.Helper()
	executor, err := NewEntryExecutor(factory, runner, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

type blockingSession struct{ closed chan struct{} }

func (s blockingSession) Valid() bool { return true }

func (s blockingSession) Close(ctx context.Context) error {
	defer close(s.closed)
	<-ctx.Done()
	return ctx.Err()
}

type blockingFactory struct {
	events *[]string
	closed chan struct{}
}

func (f blockingFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.WorkflowEntry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	return blockingSession{closed: f.closed}, nil
}

type panicRunner struct {
	events *[]string
	value  any
}

func (r panicRunner) RunEntry(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.WorkflowEntry, _ BrowserSession) error {
	*r.events = append(*r.events, "run:"+entry.ExecutionID)
	panic(r.value)
}

type panicValidSession struct{ events *[]string }

func (s panicValidSession) Valid() bool {
	*s.events = append(*s.events, "valid")
	panic("valid panic")
}

func (s panicValidSession) Close(context.Context) error {
	*s.events = append(*s.events, "close")
	return nil
}

type panicValidFactory struct{ events *[]string }

func (f panicValidFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.WorkflowEntry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	return panicValidSession{events: f.events}, nil
}

func TestEntryLifecyclePanicErrorReportsBothFailures(t *testing.T) {
	got := (EntryLifecyclePanic{RunnerPanic: "runner failed", ClosePanic: "close failed"}).Error()
	want := "entry runner panic: runner failed; browser close panic: close failed"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestEntryExecutorRejectsInvalidFenceBeforeAllocatingSession(t *testing.T) {
	events := []string{}
	executor := mustEntryExecutor(t, &sessionFactoryFixture{events: &events}, &entryRunnerFixture{events: &events})

	err := executor.Execute(context.Background(), domainexecution.WorkerFence{RunID: "run"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}})

	if err == nil || !strings.Contains(err.Error(), "execute entries") || !strings.Contains(err.Error(), "worker fence run id and claim token are required") {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(events, []string{}) {
		t.Fatalf("session events = %#v, want none", events)
	}
}

func TestEntryExecutorValidPanicClosesSynchronouslyAndStops(t *testing.T) {
	events := []string{}
	runnerCalled := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error {
		runnerCalled = true
		return nil
	})
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = mustEntryExecutor(t, panicValidFactory{events: &events}, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}})
	}()
	if recovered != "valid panic" || runnerCalled || !reflect.DeepEqual(events, []string{"create:first", "valid", "close"}) {
		t.Fatalf("panic/runner/events=%#v/%v/%v", recovered, runnerCalled, events)
	}
}

type nilSessionFactory struct{ runnerCalled *bool }

func (nilSessionFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry) (BrowserSession, error) {
	return nil, nil
}

type typedNilFactory struct{ events *[]string }

type valueReceiverSession struct{}

func (valueReceiverSession) Valid() bool                 { return true }
func (valueReceiverSession) Close(context.Context) error { return nil }

func (f typedNilFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.WorkflowEntry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	var session *valueReceiverSession
	return session, nil
}

func TestEntryExecutorRejectsTypedNilSessionWithoutCloseOrNextEntry(t *testing.T) {
	events := []string{}
	called := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error {
		called = true
		return nil
	})
	err := mustEntryExecutor(t, typedNilFactory{events: &events}, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}})
	if err == nil || called || !reflect.DeepEqual(events, []string{"create:first"}) {
		t.Fatalf("error/called/events=%v/%v/%v", err, called, events)
	}
}

func TestEntryExecutorRejectsNilSessionBeforeRunner(t *testing.T) {
	called := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error {
		called = true
		return nil
	})
	err := mustEntryExecutor(t, nilSessionFactory{runnerCalled: &called}, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}})
	if err == nil || called {
		t.Fatalf("error/called=%v/%v", err, called)
	}
}

func TestEntryExecutorRunnerPanicClosesSynchronouslyAndStops(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "canceled"}[canceled], func(t *testing.T) {
			events := []string{}
			factory := &sessionFactoryFixture{events: &events}
			executor := mustEntryExecutor(t, factory, panicRunner{events: &events, value: "runner panic"})
			ctx := context.Background()
			if canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				_ = executor.Execute(ctx, domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}})
			}()
			if recovered != "runner panic" || !reflect.DeepEqual(events, []string{"create:first", "run:first", "close:first"}) {
				t.Fatalf("panic/events=%#v/%#v", recovered, events)
			}
		})
	}
}

type EntryRunnerFunc func(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error

func (f EntryRunnerFunc) RunEntry(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.WorkflowEntry, session BrowserSession) error {
	return f(ctx, fence, entry, session)
}

type panicCloseSession struct{}

func (panicCloseSession) Valid() bool                 { return true }
func (panicCloseSession) Close(context.Context) error { panic("close panic") }

type panicCloseFactory struct{}

func (panicCloseFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry) (BrowserSession, error) {
	return panicCloseSession{}, nil
}

func TestEntryExecutorRetainsRunnerAndClosePanics(t *testing.T) {
	executor := mustEntryExecutor(t, panicCloseFactory{}, panicRunner{events: &[]string{}, value: "runner panic"})
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = executor.Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}})
	}()
	joined, ok := recovered.(EntryLifecyclePanic)
	if !ok || joined.RunnerPanic != "runner panic" || joined.ClosePanic != "close panic" {
		t.Fatalf("panic=%#v", recovered)
	}
}

func TestEntryExecutorPropagatesClosePanicAfterSuccessfulRunner(t *testing.T) {
	executor := mustEntryExecutor(t, panicCloseFactory{}, EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error {
		return nil
	}))
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = executor.Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}})
	}()
	if recovered != "close panic" {
		t.Fatalf("panic = %#v", recovered)
	}
}

type invalidSession struct{ events *[]string }

func (session invalidSession) Valid() bool { return false }
func (session invalidSession) Close(context.Context) error {
	*session.events = append(*session.events, "close")
	return nil
}

type invalidSessionFactory struct{ events *[]string }

func (factory invalidSessionFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry) (BrowserSession, error) {
	*factory.events = append(*factory.events, "create")
	return invalidSession{events: factory.events}, nil
}

func TestEntryExecutorRejectsInvalidSessionAndClosesBeforeStopping(t *testing.T) {
	events := []string{}
	runnerCalled := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error {
		runnerCalled = true
		return nil
	})
	err := mustEntryExecutor(t, invalidSessionFactory{events: &events}, runner).Execute(
		context.Background(),
		domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"},
		[]domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}},
	)
	if err == nil || !strings.Contains(err.Error(), "Host returned invalid session") || runnerCalled {
		t.Fatalf("error/runner = %v/%v", err, runnerCalled)
	}
	if !reflect.DeepEqual(events, []string{"create", "close"}) {
		t.Fatalf("events = %v", events)
	}
}

type partialSessionFactory struct {
	events    *[]string
	createErr error
	closeErr  error
}

func (f partialSessionFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.WorkflowEntry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	return &sessionFixture{id: entry.ExecutionID, events: f.events, err: f.closeErr}, f.createErr
}

func TestEntryExecutorClosesPartialSessionWhenCreateFails(t *testing.T) {
	createFailure := errors.New("create failed")
	closeFailure := errors.New("close failed")
	events := []string{}
	runnerCalled := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error {
		runnerCalled = true
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mustEntryExecutor(t, partialSessionFactory{events: &events, createErr: createFailure, closeErr: closeFailure}, runner).Execute(ctx, domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}})
	if !errors.Is(err, createFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("error = %v", err)
	}
	if runnerCalled || !reflect.DeepEqual(events, []string{"create:first", "close:first"}) {
		t.Fatalf("runner/events = %v/%#v", runnerCalled, events)
	}
}

func TestNewEntryExecutorRejectsMissingDependencies(t *testing.T) {
	factory := &sessionFactoryFixture{events: &[]string{}}
	runner := &entryRunnerFixture{events: &[]string{}}
	var typedNilFactory *sessionFactoryFixture
	var typedNilRunner *entryRunnerFixture
	for _, test := range []struct {
		name    string
		factory BrowserSessionFactory
		runner  EntryRunner
	}{
		{name: "factory", runner: runner},
		{name: "runner", factory: factory},
		{name: "typed nil factory", factory: typedNilFactory, runner: runner},
		{name: "typed nil runner", factory: factory, runner: typedNilRunner},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewEntryExecutor(test.factory, test.runner, time.Second); err == nil {
				t.Fatalf("missing %s accepted", test.name)
			}
		})
	}
	for _, timeout := range []time.Duration{0, -time.Nanosecond} {
		t.Run("nonpositive timeout "+timeout.String(), func(t *testing.T) {
			if _, err := NewEntryExecutor(factory, runner, timeout); err == nil || !strings.Contains(err.Error(), "close timeout must be positive") {
				t.Fatalf("NewEntryExecutor() error = %v", err)
			}
		})
	}
}

func TestEntryExecutorBoundsCancellationIndependentClose(t *testing.T) {
	if _, err := NewEntryExecutor(nil, nil, 0); err == nil {
		t.Fatal("zero close timeout accepted")
	}
	events := []string{}
	closed := make(chan struct{})
	factory := blockingFactory{events: &events, closed: closed}
	runner := &entryRunnerFixture{events: &events, err: context.Canceled}
	executor, err := NewEntryExecutor(factory, runner, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = executor.Execute(ctx, domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("execution/close errors=%v", err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("close did not finish")
	}
	if !reflect.DeepEqual(events, []string{"create:first", "run:first"}) {
		t.Fatalf("next session opened after close timeout: %#v", events)
	}
}

func TestEntryExecutorUsesFreshSerialBrowserSessions(t *testing.T) {
	events := []string{}
	factory := &sessionFactoryFixture{events: &events}
	runner := &entryRunnerFixture{events: &events}
	entries := []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}}
	err := mustEntryExecutor(t, factory, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, entries)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create:first", "run:first", "close:first", "create:second", "run:second", "close:second"}
	if !reflect.DeepEqual(events, want) || len(runner.seen) != 2 || runner.seen[0] == runner.seen[1] {
		t.Fatalf("events/sessions=%#v/%#v", events, runner.seen)
	}
}

func TestEntryExecutorClosesBeforeStoppingOnEveryFailure(t *testing.T) {
	runFailure := errors.New("execution failed")
	closeFailure := errors.New("close failed")
	createFailure := errors.New("create failed")
	tests := []struct {
		name      string
		createErr error
		runErr    error
		closeErr  error
		want      []string
		targets   []error
	}{
		{"create", createFailure, nil, nil, []string{"create:first"}, []error{createFailure}},
		{"execution", nil, runFailure, nil, []string{"create:first", "run:first", "close:first"}, []error{runFailure}},
		{"close", nil, nil, closeFailure, []string{"create:first", "run:first", "close:first"}, []error{closeFailure}},
		{"execution and close", nil, runFailure, closeFailure, []string{"create:first", "run:first", "close:first"}, []error{runFailure, closeFailure}},
		{"cancel", nil, context.Canceled, nil, []string{"create:first", "run:first", "close:first"}, []error{context.Canceled}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			factory := &sessionFactoryFixture{events: &events, createErr: test.createErr, closeErr: test.closeErr}
			runner := &entryRunnerFixture{events: &events, err: test.runErr}
			err := mustEntryExecutor(t, factory, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, []domainexecution.WorkflowEntry{{ExecutionID: "first"}, {ExecutionID: "second"}})
			if !reflect.DeepEqual(events, test.want) {
				t.Fatalf("events=%#v", events)
			}
			for _, target := range test.targets {
				if !errors.Is(err, target) {
					t.Fatalf("error=%v missing %v", err, target)
				}
			}
		})
	}
}
