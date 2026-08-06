package execution

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Capsule7446/healix-core/application/engine"
	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const (
	// CodeEntryExecutorConfigurationInvalid covers NewEntryExecutor's own
	// constructor checks: none of these are a caller argument distinct from the
	// executor's own configuration, so the remediation is always to repair that
	// configuration before construction, hence FAILED_PRECONDITION.
	CodeEntryExecutorConfigurationInvalid fault.Code = "EXECUTION_ENTRY_EXECUTOR_CONFIGURATION_INVALID"
	// CodeSchedulingAdapterUnavailable covers a browser session factory failure:
	// the host adapter, not the caller, needs to become reachable again.
	CodeSchedulingAdapterUnavailable fault.Code = "EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE"
	// CodeEntryBrowserSessionAdapterContractViolation covers a nil or invalid
	// session returned by the host factory: the factory itself violated its
	// contract, which has no caller remediation.
	CodeEntryBrowserSessionAdapterContractViolation fault.Code = "EXECUTION_ENTRY_BROWSER_SESSION_ADAPTER_CONTRACT_VIOLATION"
)

func entryExecutorConfigurationInvalidError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.FailedPrecondition, CodeEntryExecutorConfigurationInvalid, "entry executor configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// classifySchedulingAdapterFailure gives a bare browser session factory failure
// its registered code, and lets an already-classified failure through
// unchanged so this boundary never buries a code the host adapter already
// produced.
func classifySchedulingAdapterFailure(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	err, constructionErr := fault.Wrap(cause, fault.Unavailable, CodeSchedulingAdapterUnavailable, "scheduling adapter is unavailable")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func entryBrowserSessionAdapterContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.Internal, CodeEntryBrowserSessionAdapterContractViolation, "browser session adapter returned an invalid outcome")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// BrowserSession is an opaque Host-owned browser/process lifetime.
// Close must block until cleanup completes or the supplied context ends, and
// Host implementations must cooperatively honor its deadline. EntryExecutor
// never abandons Close asynchronously and never starts the next Entry first.
type BrowserSession interface {
	Valid() bool
	Close(context.Context) error
}

type BrowserSessionFactory interface {
	Create(context.Context, domainexecution.WorkerFence, domainexecution.Entry) (BrowserSession, error)
}

type EntryRunner interface {
	// RunEntry executes the complete top-level entry. Nested workflow references
	// receive this same session through the runner's execution context.
	//
	// It returns what the engine observed alongside any failure, and must report
	// both: a run that failed part-way still has a result the terminal decision
	// needs, and returning only the error would leave the caller unable to tell
	// a canceled run from a crashed one.
	RunEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) (engine.EntryResult, error)
}

// EntryAuthorizer answers whether this worker still holds the authority named
// by the fence, before any Host resource is created for the entry. The
// executor cannot rely on engine.RunProgram's own verification: that runs
// inside the Host's EntryRunner, which is reached only after a browser
// already exists.
type EntryAuthorizer interface {
	AuthorizeEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry) error
}

// EntryLifecyclePanic retains both panics when runner execution and cleanup
// panic during the same Entry lifecycle.
type EntryLifecyclePanic struct {
	RunnerPanic any
	ClosePanic  any
}

func (p EntryLifecyclePanic) Error() string {
	return fmt.Sprintf("entry runner panic: %v; browser close panic: %v", p.RunnerPanic, p.ClosePanic)
}

type EntryExecutor struct {
	authorizer   EntryAuthorizer
	factory      BrowserSessionFactory
	runner       EntryRunner
	closeTimeout time.Duration
}

func NewEntryExecutor(authorizer EntryAuthorizer, factory BrowserSessionFactory, runner EntryRunner, closeTimeout time.Duration) (EntryExecutor, error) {
	if isNilInterfaceValue(authorizer) {
		return EntryExecutor{}, entryExecutorConfigurationInvalidError(errors.New("entry authorizer is required"))
	}
	if isNilInterfaceValue(factory) {
		return EntryExecutor{}, entryExecutorConfigurationInvalidError(errors.New("browser session factory is required"))
	}
	if isNilInterfaceValue(runner) {
		return EntryExecutor{}, entryExecutorConfigurationInvalidError(errors.New("entry runner is required"))
	}
	if closeTimeout <= 0 {
		return EntryExecutor{}, entryExecutorConfigurationInvalidError(errors.New("browser session close timeout must be positive"))
	}
	return EntryExecutor{authorizer: authorizer, factory: factory, runner: runner, closeTimeout: closeTimeout}, nil
}

// Execute runs exactly one authorized entry and reports what its engine
// observed.
//
// It used to take a slice and loop, which put the order entries run in, and the
// decision to stop after a failure, inside the executor. Both belong to
// Scheduling: it is the only component that can see the whole instance, and it
// is the one that commits terminal state. An executor that also sequenced meant
// two components could disagree about what ran, with no way to tell which was
// right after the fact.
//
// The returned EngineOutcome is always decidable — it feeds
// DecideEntryCompletion as-is, with no host-side translation in between. Every
// return path fills it, including the ones where the engine never started, so a
// caller can commit a terminal state and release the lease from the return
// value alone. The error is still returned separately: the outcome says what
// happened, the error says why, and a host needs both for the audit trail.
func (e EntryExecutor) Execute(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.Entry) (EngineOutcome, error) {
	// The fence returns its own classified fault; an uncoded wrapper here would
	// hide that classification behind an unclassified outer error.
	if err := fence.Validate(); err != nil {
		return engineOutcomeFor(NotStartedEngineOutcome().Result, err), err
	}
	if err := e.authorizer.AuthorizeEntry(ctx, fence, entry); err != nil {
		return engineOutcomeFor(NotStartedEngineOutcome().Result, err), err
	}
	engineResult, err := e.executeEntry(ctx, fence, entry)
	return engineOutcomeFor(engineResult, err), err
}

// engineOutcomeFor pairs what the engine observed with the classification of
// whatever failure accompanied it. The code is copied out of the error rather
// than left for the caller to extract, because the outcome is what gets
// persisted: a host that had to re-inspect the error to fill an audit field
// would be free to fill it differently.
func engineOutcomeFor(result engine.EntryResult, err error) EngineOutcome {
	outcome := EngineOutcome{Result: result}
	// An unclassified failure leaves the field empty rather than inventing a
	// code: a blank audit entry is honest, a guessed one is not.
	if code, classified := fault.CodeOf(err); classified {
		outcome.FailureCode = code
	}
	return outcome
}

func (e EntryExecutor) executeEntry(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.Entry) (engineResult engine.EntryResult, result error) {
	// Every early return below reports NOT_STARTED rather than a zero result: an
	// empty result is outside the engine vocabulary and DecideEntryCompletion
	// would refuse it, stranding the entry RUNNING with an unreleasable lease.
	engineResult = NotStartedEngineOutcome().Result

	session, err := e.factory.Create(ctx, fence, entry)
	if err != nil {
		// No execution id in either wrap: classifySchedulingAdapterFailure passes an
		// already-classified cause through unchanged, and an outer wrapper that
		// still echoed the id would leak it even then.
		if !isNilBrowserSession(session) {
			closeErr := e.closeSession(ctx, session)
			var joined error = fmt.Errorf("create browser session: %w", err)
			if closeErr != nil {
				joined = errors.Join(joined, fmt.Errorf("close partial browser session: %w", closeErr))
			}
			return engineResult, classifySchedulingAdapterFailure(joined)
		}
		return engineResult, classifySchedulingAdapterFailure(fmt.Errorf("create browser session: %w", err))
	}
	if isNilBrowserSession(session) {
		return engineResult, entryBrowserSessionAdapterContractViolationError(errors.New("host returned a nil session"))
	}
	var runnerPanic any
	defer func() {
		if recovered := recover(); recovered != nil {
			runnerPanic = recovered
		}
		closeContext, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), e.closeTimeout)
		var closeErr error
		var closePanic any
		func() {
			defer func() { closePanic = recover() }()
			closeErr = session.Close(closeContext)
		}()
		cancelClose()
		if runnerPanic != nil {
			if closePanic != nil {
				panic(EntryLifecyclePanic{RunnerPanic: runnerPanic, ClosePanic: closePanic})
			}
			panic(runnerPanic)
		}
		if closePanic != nil {
			panic(closePanic)
		}
		result = errors.Join(result, wrapEntryError("close browser session for", entry.ID.String(), closeErr))
	}()
	if !session.Valid() {
		return engineResult, entryBrowserSessionAdapterContractViolationError(errors.New("host returned an invalid session"))
	}
	// Assigned before the error is wrapped, and to the named return, so a
	// teardown failure joined in by the deferred close cannot rewrite what the
	// engine reported.
	engineResult, runErr := e.runner.RunEntry(ctx, fence, entry, session)
	result = wrapEntryError("execute", entry.ID.String(), runErr)
	return engineResult, result
}

func (e EntryExecutor) closeSession(ctx context.Context, session BrowserSession) error {
	closeContext, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), e.closeTimeout)
	defer cancelClose()
	return session.Close(closeContext)
}

func isNilInterfaceValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func isNilBrowserSession(session BrowserSession) bool {
	return isNilInterfaceValue(session)
}

func wrapEntryError(operation, executionID string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s entry %s: %w", operation, executionID, err)
}
