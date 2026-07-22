package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

const (
	defaultCompletionHandlerTimeout = 5 * time.Second
	defaultCompletionChainTimeout   = 30 * time.Second
)

var (
	ErrStepTimelineStart  = errors.New("node: record step timeline start")
	ErrStepTimelineFinish = errors.New("node: record step timeline finish")
)

type TimelineMark struct {
	Offset   time.Duration
	Sequence uint64
}

type RecordingTimeline interface {
	Mark() TimelineMark
}

type StepExecutionRef struct {
	RunID      string
	NodeID     string
	Occurrence int
}

type StepBoundary string

const (
	StepBoundaryStarted  StepBoundary = "STARTED"
	StepBoundaryFinished StepBoundary = "FINISHED"
)

type StepOutcome string

const (
	StepOutcomeSucceeded StepOutcome = "SUCCEEDED"
	StepOutcomeFailed    StepOutcome = "FAILED"
	StepOutcomeCanceled  StepOutcome = "CANCELED"
)

type StepTimelineEvent struct {
	Step     StepExecutionRef
	Boundary StepBoundary
	Outcome  StepOutcome
	Mark     TimelineMark
}

func (e StepTimelineEvent) Validate() error {
	if e.Step.RunID == "" || e.Step.NodeID == "" || e.Step.Occurrence < 1 {
		return errors.New("step execution identity is invalid")
	}
	if e.Mark.Offset < 0 || e.Mark.Sequence < 1 {
		return errors.New("timeline mark is invalid")
	}
	switch e.Boundary {
	case StepBoundaryStarted:
		if e.Outcome != "" {
			return errors.New("started event cannot have outcome")
		}
	case StepBoundaryFinished:
		if e.Outcome != StepOutcomeSucceeded && e.Outcome != StepOutcomeFailed && e.Outcome != StepOutcomeCanceled {
			return errors.New("finished event requires terminal outcome")
		}
	default:
		return fmt.Errorf("unknown step boundary %q", e.Boundary)
	}
	return nil
}

type StepTimelineSink interface {
	RecordStepTimelineEvent(context.Context, StepTimelineEvent) error
}

type NodeOutcome string

const (
	NodeOutcomeSucceeded NodeOutcome = "SUCCEEDED"
	NodeOutcomeFailed    NodeOutcome = "FAILED"
	NodeOutcomeCanceled  NodeOutcome = "CANCELED"
	NodeOutcomeSkipped   NodeOutcome = "SKIPPED"
)

type NodeExecutionRef = StepExecutionRef

type ExecutionErrorSnapshot struct {
	Kind    string
	Message string
}

type NodeExecutionSnapshot struct {
	Execution   NodeExecutionRef
	NodeKind    string
	Outcome     NodeOutcome
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Error       *ExecutionErrorSnapshot
}

type ScreenshotOptions struct {
	FullPage bool
}

type ScreenshotArtifact struct {
	MediaType string
	Data      []byte
}

type ElementObservation struct {
	Exists     bool
	Visible    bool
	Text       string
	Attributes map[string]string
}

type ReadOnlyBrowser interface {
	CaptureScreenshot(context.Context, ScreenshotOptions) (ScreenshotArtifact, error)
	SnapshotDOM(context.Context) (heal.DOMSnapshot, error)
	ObserveElement(context.Context, fingerprint.NodeSpec, []string) (ElementObservation, error)
}

type runtimeReadOnlyBrowser struct {
	runtime    *Runtime
	screenshot ReadOnlyBrowser
}

func (b runtimeReadOnlyBrowser) CaptureScreenshot(ctx context.Context, options ScreenshotOptions) (ScreenshotArtifact, error) {
	artifact, err := b.screenshot.CaptureScreenshot(ctx, options)
	if err != nil {
		return ScreenshotArtifact{}, err
	}
	artifact.Data = append([]byte(nil), artifact.Data...)
	return artifact, nil
}

type copiedDOMSnapshot struct {
	source heal.DOMSnapshot
}

func (s copiedDOMSnapshot) Candidates(ctx context.Context) ([]heal.SnapshotCandidate, error) {
	candidates, err := s.source.Candidates(ctx)
	if err != nil {
		return nil, err
	}
	copied := make([]heal.SnapshotCandidate, len(candidates))
	for i, candidate := range candidates {
		copied[i] = candidate
		copied[i].Fingerprint.Attributes = cloneStringMap(candidate.Fingerprint.Attributes)
		copied[i].Fingerprint.Path = append([]string(nil), candidate.Fingerprint.Path...)
		copied[i].Fingerprint.Framework = append(fingerprint.FrameworkStack(nil), candidate.Fingerprint.Framework...)
	}
	return copied, nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (b runtimeReadOnlyBrowser) SnapshotDOM(ctx context.Context) (heal.DOMSnapshot, error) {
	snapshot, err := b.runtime.Driver.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return copiedDOMSnapshot{source: snapshot}, nil
}

func (b runtimeReadOnlyBrowser) ObserveElement(ctx context.Context, spec fingerprint.NodeSpec, attributes []string) (ElementObservation, error) {
	element, err := b.runtime.locator().Locate(ctx, spec)
	if err != nil {
		return ElementObservation{}, err
	}
	reader := b.runtime.reader()
	exists, err := reader.Exists(ctx, element)
	if err != nil {
		return ElementObservation{}, err
	}
	visible, err := reader.Visible(ctx, element)
	if err != nil {
		return ElementObservation{}, err
	}
	text, err := reader.Text(ctx, element)
	if err != nil {
		return ElementObservation{}, err
	}
	observed := make(map[string]string, len(attributes))
	for _, name := range append([]string(nil), attributes...) {
		value, ok, err := reader.Attribute(ctx, element, name)
		if err != nil {
			return ElementObservation{}, err
		}
		if ok {
			observed[name] = value
		}
	}
	return ElementObservation{Exists: exists, Visible: visible, Text: text, Attributes: observed}, nil
}

func (rt *Runtime) completionBrowser() ReadOnlyBrowser {
	if rt.ReadOnlyBrowser == nil {
		return nil
	}
	return runtimeReadOnlyBrowser{runtime: rt, screenshot: rt.ReadOnlyBrowser}
}

type NodeCompletionContext struct {
	Snapshot NodeExecutionSnapshot
	Browser  ReadOnlyBrowser
}

type NodeCompletionHandler interface {
	Name() string
	Handle(context.Context, NodeCompletionContext) error
}

type CompletionHandlerResult struct {
	HandlerName string
	StartedAt   time.Time
	CompletedAt time.Time
	Error       *ExecutionErrorSnapshot
}

type NodeCompletionOptions struct {
	HandlerTimeout time.Duration
	ChainTimeout   time.Duration
}

type NodeCompletionChain struct {
	handlers []NodeCompletionHandler
	options  NodeCompletionOptions
}

func NewNodeCompletionChain(options NodeCompletionOptions, handlers ...NodeCompletionHandler) (*NodeCompletionChain, error) {
	copied := append([]NodeCompletionHandler(nil), handlers...)
	for i, handler := range copied {
		if handler == nil || handler.Name() == "" {
			return nil, fmt.Errorf("completion handler %d requires a name", i)
		}
	}
	if options.HandlerTimeout < 0 || options.ChainTimeout < 0 {
		return nil, errors.New("completion timeouts cannot be negative")
	}
	return &NodeCompletionChain{handlers: copied, options: options}, nil
}

func (c *NodeCompletionChain) run(ctx context.Context, input NodeCompletionContext) []CompletionHandlerResult {
	if c == nil || len(c.handlers) == 0 {
		return nil
	}
	chainTimeout := c.options.ChainTimeout
	if chainTimeout == 0 {
		chainTimeout = defaultCompletionChainTimeout
	}
	chainCtx, cancelChain := context.WithTimeout(context.WithoutCancel(ctx), chainTimeout)
	defer cancelChain()
	results := make([]CompletionHandlerResult, 0, len(c.handlers))
	for _, handler := range c.handlers {
		if chainCtx.Err() != nil {
			break
		}
		handlerTimeout := c.options.HandlerTimeout
		if handlerTimeout == 0 {
			handlerTimeout = defaultCompletionHandlerTimeout
		}
		handlerCtx, cancelHandler := context.WithTimeout(chainCtx, handlerTimeout)
		startedAt := time.Now()
		err := handler.Handle(handlerCtx, input)
		completedAt := time.Now()
		cancelHandler()
		results = append(results, CompletionHandlerResult{HandlerName: handler.Name(), StartedAt: startedAt, CompletedAt: completedAt, Error: snapshotError(err)})
	}
	return results
}

type NodeCompletionObservation struct {
	Execution NodeExecutionRef
	Results   []CompletionHandlerResult
}

type NodeCompletionObserver interface {
	RecordNodeCompletion(context.Context, NodeCompletionObservation) error
}

type LeafCompletionError struct {
	NodeErr     error
	TimelineErr error
}

func (e *LeafCompletionError) Error() string {
	return errors.Join(e.NodeErr, e.TimelineErr).Error()
}

func (e *LeafCompletionError) Unwrap() []error {
	return []error{e.NodeErr, e.TimelineErr}
}

func LeafExecutionError(err error) error {
	var completionErr *LeafCompletionError
	if errors.As(err, &completionErr) {
		return completionErr.NodeErr
	}
	return err
}

type leafLifecycle struct {
	runtime     *Runtime
	execution   StepExecutionRef
	nodeKind    string
	startedAt   time.Time
	startedMark TimelineMark
	nodeOutcome NodeOutcome
}

func (rt *Runtime) beginLeafLifecycle(ctx context.Context, nodeID, nodeKind string, occurrence int) (*leafLifecycle, error) {
	lifecycle := &leafLifecycle{
		runtime:   rt,
		execution: StepExecutionRef{RunID: rt.RunID, NodeID: nodeID, Occurrence: occurrence},
		nodeKind:  nodeKind,
		startedAt: time.Now(),
	}
	if rt.StepTimeline == nil {
		return lifecycle, nil
	}
	if rt.Timeline == nil {
		return nil, errors.New("node: recording timeline is required when step timeline is enabled")
	}
	lifecycle.startedMark = rt.Timeline.Mark()
	event := StepTimelineEvent{Step: lifecycle.execution, Boundary: StepBoundaryStarted, Mark: lifecycle.startedMark}
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("validate step timeline start: %w", err)
	}
	if err := rt.StepTimeline.RecordStepTimelineEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("%w for %s/%d: %v", ErrStepTimelineStart, nodeID, occurrence, err)
	}
	return lifecycle, nil
}

func (l *leafLifecycle) MarkSkipped() {
	l.nodeOutcome = NodeOutcomeSkipped
}

func (l *leafLifecycle) Complete(ctx context.Context, nodeErr error) error {
	completedAt := time.Now()
	outcome, stepOutcome := lifecycleOutcome(ctx, nodeErr)
	if l.nodeOutcome == NodeOutcomeSkipped && nodeErr == nil {
		outcome = NodeOutcomeSkipped
	}
	var timelineErr error
	if l.runtime.StepTimeline != nil {
		mark := l.runtime.Timeline.Mark()
		event := StepTimelineEvent{Step: l.execution, Boundary: StepBoundaryFinished, Outcome: stepOutcome, Mark: mark}
		if mark.Offset < l.startedMark.Offset {
			timelineErr = errors.New("finished timeline mark precedes started mark")
		} else if err := event.Validate(); err != nil {
			timelineErr = err
		} else {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
			timelineErr = l.runtime.StepTimeline.RecordStepTimelineEvent(cleanupCtx, event)
			cancel()
		}
		if timelineErr != nil {
			timelineErr = fmt.Errorf("%w for %s/%d: %v", ErrStepTimelineFinish, l.execution.NodeID, l.execution.Occurrence, timelineErr)
		}
	}
	snapshot := NodeExecutionSnapshot{
		Execution:   l.execution,
		NodeKind:    l.nodeKind,
		Outcome:     outcome,
		StartedAt:   l.startedAt,
		CompletedAt: completedAt,
		Duration:    completedAt.Sub(l.startedAt),
		Error:       snapshotError(nodeErr),
	}
	results := l.runtime.CompletionChain.run(ctx, NodeCompletionContext{Snapshot: snapshot, Browser: l.runtime.completionBrowser()})
	if l.runtime.CompletionObserver != nil && len(results) > 0 {
		observation := NodeCompletionObservation{Execution: l.execution, Results: append([]CompletionHandlerResult(nil), results...)}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
		_ = l.runtime.CompletionObserver.RecordNodeCompletion(cleanupCtx, observation)
		cancel()
	}
	if timelineErr != nil {
		return &LeafCompletionError{NodeErr: nodeErr, TimelineErr: timelineErr}
	}
	return nodeErr
}

func lifecycleOutcome(ctx context.Context, err error) (NodeOutcome, StepOutcome) {
	if err == nil {
		return NodeOutcomeSucceeded, StepOutcomeSucceeded
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NodeOutcomeCanceled, StepOutcomeCanceled
	}
	return NodeOutcomeFailed, StepOutcomeFailed
}

func snapshotError(err error) *ExecutionErrorSnapshot {
	if err == nil {
		return nil
	}
	return &ExecutionErrorSnapshot{Kind: string(errorKind(err)), Message: err.Error()}
}
