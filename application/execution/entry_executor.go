package execution

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

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
	RunEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) error
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
	factory      BrowserSessionFactory
	runner       EntryRunner
	closeTimeout time.Duration
}

func NewEntryExecutor(factory BrowserSessionFactory, runner EntryRunner, closeTimeout time.Duration) (EntryExecutor, error) {
	if isNilInterfaceValue(factory) {
		return EntryExecutor{}, entryExecutorConfigurationInvalidError(errors.New("browser session factory is required"))
	}
	if isNilInterfaceValue(runner) {
		return EntryExecutor{}, entryExecutorConfigurationInvalidError(errors.New("entry runner is required"))
	}
	if closeTimeout <= 0 {
		return EntryExecutor{}, entryExecutorConfigurationInvalidError(errors.New("browser session close timeout must be positive"))
	}
	return EntryExecutor{factory: factory, runner: runner, closeTimeout: closeTimeout}, nil
}

// Execute runs exactly one authorized entry.
//
// It used to take a slice and loop, which put the order entries run in, and the
// decision to stop after a failure, inside the executor. Both belong to
// Scheduling: it is the only component that can see the whole instance, and it
// is the one that commits terminal state. An executor that also sequenced meant
// two components could disagree about what ran, with no way to tell which was
// right after the fact.
func (e EntryExecutor) Execute(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.Entry) error {
	// The fence returns its own classified fault; an uncoded wrapper here would
	// hide that classification behind an unclassified outer error.
	if err := fence.Validate(); err != nil {
		return err
	}
	return e.executeEntry(ctx, fence, entry)
}

func (e EntryExecutor) executeEntry(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.Entry) (result error) {
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
			return classifySchedulingAdapterFailure(joined)
		}
		return classifySchedulingAdapterFailure(fmt.Errorf("create browser session: %w", err))
	}
	if isNilBrowserSession(session) {
		return entryBrowserSessionAdapterContractViolationError(errors.New("host returned a nil session"))
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
		result = errors.Join(result, wrapEntryError("close browser session for", entry.ExecutionID, closeErr))
	}()
	if !session.Valid() {
		return entryBrowserSessionAdapterContractViolationError(errors.New("host returned an invalid session"))
	}
	result = wrapEntryError("execute", entry.ExecutionID, e.runner.RunEntry(ctx, fence, entry, session))
	return result
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
