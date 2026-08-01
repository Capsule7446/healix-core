package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
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

func (f *sessionFactoryFixture) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry) (BrowserSession, error) {
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

func (r *entryRunnerFixture) RunEntry(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry, session BrowserSession) error {
	*r.events = append(*r.events, "run:"+entry.ExecutionID)
	r.seen = append(r.seen, session)
	return r.err
}

// unwrapFaultCause finds the *fault.Error inside err (even when it is nested
// under errors.Join, which errors.Unwrap's single-error form cannot see
// through) and returns its own private cause.
func unwrapFaultCause(t *testing.T, err error) error {
	t.Helper()
	var target *fault.Error
	if !errors.As(err, &target) {
		t.Fatalf("error = %v, want a *fault.Error in its chain", err)
	}
	cause := target.Unwrap()
	if cause == nil {
		t.Fatalf("error = %v, want its fault to carry a private cause", err)
	}
	return cause
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

func (f blockingFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	return blockingSession{closed: f.closed}, nil
}

type panicRunner struct {
	events *[]string
	value  any
}

func (r panicRunner) RunEntry(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry, _ BrowserSession) error {
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

func (f panicValidFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	return panicValidSession{events: f.events}, nil
}

type invalidSession struct {
	*sessionFixture
}

func (invalidSession) Valid() bool { return false }

type invalidSessionFactory struct{ session *sessionFixture }

func (factory invalidSessionFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.Entry) (BrowserSession, error) {
	return invalidSession{factory.session}, nil
}

// The executor now runs exactly one entry, so there is no empty-collection case
// to branch on: an instance with no entries is Scheduling's to recognise before
// it authorizes anything. What is left to pin is that one entry produces exactly
// one session, created before the runner and closed after it.
func TestEntryExecutorRunsOneEntryWithOneSession(t *testing.T) {
	fence := domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}
	events := []string{}
	factory := &sessionFactoryFixture{events: &events}
	runner := &entryRunnerFixture{events: &events}

	if err := mustEntryExecutor(t, factory, runner).Execute(context.Background(), fence, domainexecution.Entry{ExecutionID: "entry"}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(events, []string{"create:entry", "run:entry", "close:entry"}) {
		t.Fatalf("events = %v, want create then run then close, once each", events)
	}
	if len(factory.sessions) != 1 {
		t.Fatalf("created %d sessions for one entry, want exactly one", len(factory.sessions))
	}
	if len(runner.seen) != 1 || runner.seen[0] != factory.sessions[0] {
		t.Fatalf("the runner did not receive the session that was created for it: %#v", runner.seen)
	}
}

func TestEntryExecutorRejectsNormalInvalidSessionAndWrapsRunnerError(t *testing.T) {
	fence := domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}
	events := []string{}
	session := &sessionFixture{id: "entry", events: &events}
	runner := &entryRunnerFixture{events: &events}
	err := mustEntryExecutor(t, invalidSessionFactory{session}, runner).Execute(context.Background(), fence, domainexecution.Entry{ExecutionID: "entry"})
	if err == nil || !fault.IsCode(err, CodeEntryBrowserSessionAdapterContractViolation) || len(runner.seen) != 0 || !reflect.DeepEqual(events, []string{"close:entry"}) {
		t.Fatalf("invalid session error = %v, events = %v", err, events)
	}
	if cause := unwrapFaultCause(t, err); !strings.Contains(cause.Error(), "invalid session") {
		t.Fatalf("private cause = %v, want it to retain the session detail", cause)
	}

	cause := errors.New("runner failed")
	events = []string{}
	err = mustEntryExecutor(t, &sessionFactoryFixture{events: &events}, &entryRunnerFixture{events: &events, err: cause}).Execute(context.Background(), fence, domainexecution.Entry{ExecutionID: "entry"})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "execute entry entry") {
		t.Fatalf("wrapped runner error = %v", err)
	}
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

	err := executor.Execute(context.Background(), domainexecution.WorkerFence{RunID: "run"}, domainexecution.Entry{ExecutionID: "first"})

	// The fence's own error now propagates unwrapped instead of behind an uncoded
	// "execute entries" layer. Its identity check is still a bare error — that is
	// the remaining domain/execution migration, not this boundary's concern.
	if err == nil || !strings.Contains(err.Error(), "worker fence run id and claim token are required") {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(events, []string{}) {
		t.Fatalf("session events = %#v, want none", events)
	}
}

func TestEntryExecutorValidPanicClosesSynchronouslyAndStops(t *testing.T) {
	events := []string{}
	runnerCalled := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error {
		runnerCalled = true
		return nil
	})
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = mustEntryExecutor(t, panicValidFactory{events: &events}, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
	}()
	if recovered != "valid panic" || runnerCalled || !reflect.DeepEqual(events, []string{"create:first", "valid", "close"}) {
		t.Fatalf("panic/runner/events=%#v/%v/%v", recovered, runnerCalled, events)
	}
}

type nilSessionFactory struct{ runnerCalled *bool }

func (nilSessionFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.Entry) (BrowserSession, error) {
	return nil, nil
}

type typedNilFactory struct{ events *[]string }

type valueReceiverSession struct{}

func (valueReceiverSession) Valid() bool                 { return true }
func (valueReceiverSession) Close(context.Context) error { return nil }

func (f typedNilFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	var session *valueReceiverSession
	return session, nil
}

func TestEntryExecutorRejectsTypedNilSessionWithoutCloseOrNextEntry(t *testing.T) {
	events := []string{}
	called := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error {
		called = true
		return nil
	})
	err := mustEntryExecutor(t, typedNilFactory{events: &events}, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
	if err == nil || called || !reflect.DeepEqual(events, []string{"create:first"}) {
		t.Fatalf("error/called/events=%v/%v/%v", err, called, events)
	}
}

func TestEntryExecutorRejectsNilSessionBeforeRunner(t *testing.T) {
	called := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error {
		called = true
		return nil
	})
	err := mustEntryExecutor(t, nilSessionFactory{runnerCalled: &called}, runner).Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
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
				_ = executor.Execute(ctx, domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
			}()
			if recovered != "runner panic" || !reflect.DeepEqual(events, []string{"create:first", "run:first", "close:first"}) {
				t.Fatalf("panic/events=%#v/%#v", recovered, events)
			}
		})
	}
}

type EntryRunnerFunc func(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error

func (f EntryRunnerFunc) RunEntry(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.Entry, session BrowserSession) error {
	return f(ctx, fence, entry, session)
}

type panicCloseSession struct{}

func (panicCloseSession) Valid() bool                 { return true }
func (panicCloseSession) Close(context.Context) error { panic("close panic") }

type panicCloseFactory struct{}

func (panicCloseFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.Entry) (BrowserSession, error) {
	return panicCloseSession{}, nil
}

func TestEntryExecutorRetainsRunnerAndClosePanics(t *testing.T) {
	executor := mustEntryExecutor(t, panicCloseFactory{}, panicRunner{events: &[]string{}, value: "runner panic"})
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = executor.Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
	}()
	joined, ok := recovered.(EntryLifecyclePanic)
	if !ok || joined.RunnerPanic != "runner panic" || joined.ClosePanic != "close panic" {
		t.Fatalf("panic=%#v", recovered)
	}
}

func TestEntryExecutorPropagatesClosePanicAfterSuccessfulRunner(t *testing.T) {
	executor := mustEntryExecutor(t, panicCloseFactory{}, EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error {
		return nil
	}))
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = executor.Execute(context.Background(), domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
	}()
	if recovered != "close panic" {
		t.Fatalf("panic = %#v", recovered)
	}
}

type observedInvalidSession struct{ events *[]string }

func (session observedInvalidSession) Valid() bool { return false }
func (session observedInvalidSession) Close(context.Context) error {
	*session.events = append(*session.events, "close")
	return nil
}

type observedInvalidSessionFactory struct{ events *[]string }

func (factory observedInvalidSessionFactory) Create(context.Context, domainexecution.WorkerFence, domainexecution.Entry) (BrowserSession, error) {
	*factory.events = append(*factory.events, "create")
	return observedInvalidSession{events: factory.events}, nil
}

func TestEntryExecutorRejectsInvalidSessionAndClosesBeforeStopping(t *testing.T) {
	events := []string{}
	runnerCalled := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error {
		runnerCalled = true
		return nil
	})
	err := mustEntryExecutor(t, observedInvalidSessionFactory{events: &events}, runner).Execute(
		context.Background(),
		domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"},
		domainexecution.Entry{ExecutionID: "first"},
	)
	if err == nil || !fault.IsCode(err, CodeEntryBrowserSessionAdapterContractViolation) || runnerCalled {
		t.Fatalf("error/runner = %v/%v", err, runnerCalled)
	}
	descriptor, ok := fault.Describe(err)
	if !ok || strings.Contains(descriptor.Message(), "invalid session") {
		t.Fatalf("public message = %#v (ok=%v), must not carry the session detail", descriptor, ok)
	}
	if cause := unwrapFaultCause(t, err); !strings.Contains(cause.Error(), "invalid session") {
		t.Fatalf("private cause = %v, want it to retain the session detail", cause)
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

func (f partialSessionFactory) Create(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry) (BrowserSession, error) {
	*f.events = append(*f.events, "create:"+entry.ExecutionID)
	return &sessionFixture{id: entry.ExecutionID, events: f.events, err: f.closeErr}, f.createErr
}

func TestEntryExecutorClosesPartialSessionWhenCreateFails(t *testing.T) {
	createFailure := errors.New("create failed")
	closeFailure := errors.New("close failed")
	events := []string{}
	runnerCalled := false
	runner := EntryRunnerFunc(func(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error {
		runnerCalled = true
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mustEntryExecutor(t, partialSessionFactory{events: &events, createErr: createFailure, closeErr: closeFailure}, runner).Execute(ctx, domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
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
			_, err := NewEntryExecutor(factory, runner, timeout)
			if err == nil || !fault.IsCode(err, CodeEntryExecutorConfigurationInvalid) {
				t.Fatalf("NewEntryExecutor() error = %v, want code %s", err, CodeEntryExecutorConfigurationInvalid)
			}
			descriptor, ok := fault.Describe(err)
			if !ok || strings.Contains(descriptor.Message(), "close timeout must be positive") {
				t.Fatalf("public message = %#v (ok=%v), must not carry the detail", descriptor, ok)
			}
			if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), "close timeout must be positive") {
				t.Fatalf("private cause = %v, want it to retain the detail", cause)
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
	err = executor.Execute(ctx, domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})
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

// Each entry gets its own browser session, created and closed inside the one
// Execute call. Serialising several entries is Scheduling's job now, so what is
// left to pin here is that two separate calls never share a session — the
// property that made serial execution safe in the first place.
func TestEntryExecutorGivesEachEntryItsOwnSession(t *testing.T) {
	events := []string{}
	factory := &sessionFactoryFixture{events: &events}
	runner := &entryRunnerFixture{events: &events}
	executor := mustEntryExecutor(t, factory, runner)
	fence := domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}

	for _, id := range []string{"first", "second"} {
		if err := executor.Execute(context.Background(), fence, domainexecution.Entry{ExecutionID: id}); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{"create:first", "run:first", "close:first", "create:second", "run:second", "close:second"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if len(runner.seen) != 2 || runner.seen[0] == runner.seen[1] {
		t.Fatalf("the two entries shared a browser session: %#v", runner.seen)
	}
}

// Cancellation during an entry no longer becomes the executor's error: the
// entry itself completed, and whether the instance continues is Scheduling's
// decision. What must still hold is that the session is closed regardless.
func TestEntryExecutorClosesTheSessionEvenWhenTheContextIsCancelledMidEntry(t *testing.T) {
	events := []string{}
	factory := &sessionFactoryFixture{events: &events}
	ctx, cancel := context.WithCancel(context.Background())
	runner := EntryRunnerFunc(func(_ context.Context, _ domainexecution.WorkerFence, entry domainexecution.Entry, _ BrowserSession) error {
		events = append(events, "run:"+entry.ExecutionID)
		cancel()
		return nil
	})

	err := mustEntryExecutor(t, factory, runner).Execute(ctx, domainexecution.WorkerFence{RunID: "run", ClaimToken: "claim"}, domainexecution.Entry{ExecutionID: "first"})

	if err != nil {
		t.Fatalf("Execute() error = %v; the entry finished, so sequencing after cancellation belongs to Scheduling", err)
	}
	if want := []string{"create:first", "run:first", "close:first"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}
