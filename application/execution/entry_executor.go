package execution

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
)

// BrowserSession is an opaque Host-owned browser/process lifetime.
// Close must block until cleanup completes or the supplied context ends, and
// Host implementations must cooperatively honor its deadline. EntryExecutor
// never abandons Close asynchronously and never starts the next Entry first.
type BrowserSession interface {
	Valid() bool
	Close(context.Context) error
}

type BrowserSessionFactory interface {
	Create(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry) (BrowserSession, error)
}

type EntryRunner interface {
	// RunEntry executes the complete top-level entry. Nested workflow references
	// receive this same session through the runner's execution context.
	RunEntry(context.Context, domainexecution.WorkerFence, domainexecution.WorkflowEntry, BrowserSession) error
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
		return EntryExecutor{}, errors.New("browser session factory is required")
	}
	if isNilInterfaceValue(runner) {
		return EntryExecutor{}, errors.New("entry runner is required")
	}
	if closeTimeout <= 0 {
		return EntryExecutor{}, errors.New("browser session close timeout must be positive")
	}
	return EntryExecutor{factory: factory, runner: runner, closeTimeout: closeTimeout}, nil
}

func (e EntryExecutor) Execute(ctx context.Context, fence domainexecution.WorkerFence, entries []domainexecution.WorkflowEntry) error {
	// The fence returns its own classified fault; an uncoded wrapper here would
	// hide that classification behind an unclassified outer error.
	if err := fence.Validate(); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := e.executeEntry(ctx, fence, entry); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (e EntryExecutor) executeEntry(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.WorkflowEntry) (result error) {
	session, err := e.factory.Create(ctx, fence, entry)
	if err != nil {
		if !isNilBrowserSession(session) {
			closeErr := e.closeSession(ctx, session)
			return errors.Join(fmt.Errorf("create browser session for entry %s: %w", entry.ExecutionID, err), wrapEntryError("close partial browser session for", entry.ExecutionID, closeErr))
		}
		return fmt.Errorf("create browser session for entry %s: %w", entry.ExecutionID, err)
	}
	if isNilBrowserSession(session) {
		return fmt.Errorf("create browser session for entry %s: Host returned nil session", entry.ExecutionID)
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
		return fmt.Errorf("create browser session for entry %s: Host returned invalid session", entry.ExecutionID)
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
