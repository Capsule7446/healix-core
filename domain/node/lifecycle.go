package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainexecution "github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

const (
	// defaultCompletionHandlerTimeout 是单个完成处理器的默认超时。
	defaultCompletionHandlerTimeout = 5 * time.Second
	// defaultCompletionChainTimeout 是完成处理器链的默认总超时。
	defaultCompletionChainTimeout = 30 * time.Second
)

// TimelineMark 标识时间线中的偏移和单调序列号。
type TimelineMark struct {
	Offset   time.Duration
	Sequence uint64
}

// RecordingTimeline 提供读取当前时间线标记的端口。
type RecordingTimeline interface {
	// Mark 返回当前时间线标记。
	Mark() TimelineMark
}

// StepExecutionRef 标识实例中某个节点执行及其 Occurrence。
type StepExecutionRef struct {
	InstanceID domainexecution.InstanceID
	NodeID     string
	Occurrence int
}

// StepBoundary 标识步骤时间线事件是开始还是完成边界。
type StepBoundary string

const (
	// StepBoundaryStarted 表示步骤开始边界。
	StepBoundaryStarted StepBoundary = "STARTED"
	// StepBoundaryFinished 表示步骤完成边界。
	StepBoundaryFinished StepBoundary = "FINISHED"
)

// StepOutcome 标识完成边界的终态结果。
type StepOutcome string

const (
	// StepOutcomeSucceeded 表示步骤成功完成。
	StepOutcomeSucceeded StepOutcome = "SUCCEEDED"
	// StepOutcomeFailed 表示步骤失败完成。
	StepOutcomeFailed StepOutcome = "FAILED"
	// StepOutcomeCanceled 表示步骤因取消完成。
	StepOutcomeCanceled StepOutcome = "CANCELED"
)

// StepTimelineEvent 记录步骤边界、终态结果和时间线标记。
type StepTimelineEvent struct {
	Step     StepExecutionRef
	Boundary StepBoundary
	Outcome  StepOutcome
	Mark     TimelineMark
}

// Validate 校验步骤身份、时间线标记及开始/完成边界的结果组合。
func (e StepTimelineEvent) Validate() error {
	if e.Step.InstanceID.Validate() != nil || e.Step.NodeID == "" || e.Step.Occurrence < 1 {
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

// StepTimelineSink 持久化步骤时间线事件。
type StepTimelineSink interface {
	// RecordStepTimelineEvent 记录一个步骤时间线事件。
	RecordStepTimelineEvent(context.Context, StepTimelineEvent) error
}

// NodeOutcome 标识节点执行的生命周期结果。
type NodeOutcome string

const (
	// NodeOutcomeSucceeded 表示节点成功完成。
	NodeOutcomeSucceeded NodeOutcome = "SUCCEEDED"
	// NodeOutcomeFailed 表示节点失败完成。
	NodeOutcomeFailed NodeOutcome = "FAILED"
	// NodeOutcomeCanceled 表示节点因取消完成。
	NodeOutcomeCanceled NodeOutcome = "CANCELED"
	// NodeOutcomeSkipped 表示节点被跳过。
	NodeOutcomeSkipped NodeOutcome = "SKIPPED"
)

// ExecutionErrorSnapshot 保存安全的错误 Kind、Code 和 Message 快照。
type ExecutionErrorSnapshot struct {
	Kind    fault.Kind
	Code    fault.Code
	Message string
}

// NodeExecutionSnapshot 保存节点执行身份、结果、时间和安全错误快照。
type NodeExecutionSnapshot struct {
	Execution   StepExecutionRef
	NodeKind    string
	Outcome     NodeOutcome
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Error       *ExecutionErrorSnapshot
}

// ScreenshotOptions 配置截图是否包含完整页面。
type ScreenshotOptions struct {
	FullPage bool
}

// ScreenshotArtifact 保存截图媒体类型和数据副本。
type ScreenshotArtifact struct {
	MediaType string
	Data      []byte
}

// ElementObservation 保存元素存在性、可见性、文本和属性观测。
type ElementObservation struct {
	Exists     bool
	Visible    bool
	Text       string
	Attributes map[string]string
}

// ReadOnlyBrowser 提供完成处理器使用的只读浏览器端口。
type ReadOnlyBrowser interface {
	// CaptureScreenshot 捕获截图并返回其工件。
	CaptureScreenshot(context.Context, ScreenshotOptions) (ScreenshotArtifact, error)
	// SnapshotDOM 获取只读 DOM 候选快照。
	SnapshotDOM(context.Context) (heal.DOMSnapshot, error)
	// ObserveElement 读取目标元素的存在性、可见性、文本和指定属性。
	ObserveElement(context.Context, fingerprint.ElementTargetSpec, []string) (ElementObservation, error)
}

// runtimeReadOnlyBrowser 将 Runtime 的驱动和只读浏览器端口适配为 ReadOnlyBrowser。
type runtimeReadOnlyBrowser struct {
	runtime    *Runtime
	screenshot ReadOnlyBrowser
}

// CaptureScreenshot 委托只读浏览器截图并复制返回的数据字节。
func (b runtimeReadOnlyBrowser) CaptureScreenshot(ctx context.Context, options ScreenshotOptions) (ScreenshotArtifact, error) {
	artifact, err := b.screenshot.CaptureScreenshot(ctx, options)
	if err != nil {
		return ScreenshotArtifact{}, err
	}
	artifact.Data = append([]byte(nil), artifact.Data...)
	return artifact, nil
}

// copiedDOMSnapshot 保存候选指纹深拷贝，避免完成处理器修改运行时数据。
type copiedDOMSnapshot struct {
	candidates []heal.SnapshotCandidate
}

// Candidates 返回候选快照的深拷贝切片。
func (s copiedDOMSnapshot) Candidates(context.Context) ([]heal.SnapshotCandidate, error) {
	return cloneSnapshotCandidates(s.candidates), nil
}

// cloneSnapshotCandidates 深复制候选切片及其指纹数据。
func cloneSnapshotCandidates(source []heal.SnapshotCandidate) []heal.SnapshotCandidate {
	copied := make([]heal.SnapshotCandidate, len(source))
	for i, candidate := range source {
		copied[i] = candidate
		copied[i].Fingerprint = candidate.Fingerprint.Clone()
	}
	return copied
}

// SnapshotDOM 获取驱动 DOM 候选并返回隔离所有权的快照。
func (b runtimeReadOnlyBrowser) SnapshotDOM(ctx context.Context) (heal.DOMSnapshot, error) {
	snapshot, err := b.runtime.Driver.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	candidates, err := snapshot.Candidates(ctx)
	if err != nil {
		return nil, err
	}
	return copiedDOMSnapshot{candidates: cloneSnapshotCandidates(candidates)}, nil
}

// ObserveElement 通过定位器和只读读取器观测元素，并复制属性名称和值到结果映射。
func (b runtimeReadOnlyBrowser) ObserveElement(ctx context.Context, spec fingerprint.ElementTargetSpec, attributes []string) (ElementObservation, error) {
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

// completionBrowser 返回完成处理器可用的只读浏览器适配器；端口缺失时返回 nil。
func (rt *Runtime) completionBrowser() ReadOnlyBrowser {
	if rt.ReadOnlyBrowser == nil {
		return nil
	}
	return runtimeReadOnlyBrowser{runtime: rt, screenshot: rt.ReadOnlyBrowser}
}

// NodeCompletionContext 携带节点执行快照和可选只读浏览器端口。
type NodeCompletionContext struct {
	Snapshot NodeExecutionSnapshot
	Browser  ReadOnlyBrowser
}

// NodeCompletionHandler 在节点完成后处理只读上下文。
type NodeCompletionHandler interface {
	// Name 返回处理器稳定名称。
	Name() string
	// Handle 处理节点完成上下文并返回错误。
	Handle(context.Context, NodeCompletionContext) error
}

// CompletionHandlerResult 保存处理器名称、执行时间和安全错误快照。
type CompletionHandlerResult struct {
	HandlerName string
	StartedAt   time.Time
	CompletedAt time.Time
	Error       *ExecutionErrorSnapshot
}

// NodeCompletionOptions 配置单个处理器和整个完成链的超时。
type NodeCompletionOptions struct {
	HandlerTimeout time.Duration
	ChainTimeout   time.Duration
}

// NodeCompletionChain 按注册顺序运行节点完成处理器。
type NodeCompletionChain struct {
	handlers []NodeCompletionHandler
	options  NodeCompletionOptions
}

// NewNodeCompletionChain 复制并校验处理器列表及超时配置，创建完成链。
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

// HasHandlers 判断完成链是否非 nil 且包含处理器。
func (c *NodeCompletionChain) HasHandlers() bool {
	return c != nil && len(c.handlers) > 0
}

// run 在总超时和单处理器超时约束下按顺序运行完成处理器，并复制错误快照输入；父 context
// 已取消时使用不携带取消信号的派生上下文，以便完成清理仍受链超时约束地执行。
func (c *NodeCompletionChain) run(ctx context.Context, input NodeCompletionContext) []CompletionHandlerResult {
	if c == nil || len(c.handlers) == 0 {
		return nil
	}
	chainTimeout := c.options.ChainTimeout
	if chainTimeout == 0 {
		chainTimeout = defaultCompletionChainTimeout
	}
	chainParent := ctx
	if ctx.Err() != nil {
		chainParent = context.WithoutCancel(ctx)
	}
	chainCtx, cancelChain := context.WithTimeout(chainParent, chainTimeout)
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
		handlerInput := input
		handlerInput.Snapshot.Error = cloneErrorSnapshot(input.Snapshot.Error)
		err := handler.Handle(handlerCtx, handlerInput)
		completedAt := time.Now()
		cancelHandler()
		results = append(results, CompletionHandlerResult{HandlerName: handler.Name(), StartedAt: startedAt, CompletedAt: completedAt, Error: snapshotError(err)})
	}
	return results
}

// NodeCompletionObservation 保存节点执行身份和所有处理器结果。
type NodeCompletionObservation struct {
	Execution StepExecutionRef
	Results   []CompletionHandlerResult
}

// NodeCompletionObserver 持久化节点完成观测。
type NodeCompletionObserver interface {
	// RecordNodeCompletion 记录节点完成观测。
	RecordNodeCompletion(context.Context, NodeCompletionObservation) error
}

// leafCompletionError 聚合节点错误、时间线错误和完成观测错误。
type leafCompletionError struct {
	fault          error
	nodeErr        error
	timelineErr    error
	observationErr error
}

// Error 返回聚合错误的安全主错误文本。
func (e *leafCompletionError) Error() string {
	return e.fault.Error()
}

// Unwrap 返回所有非 nil 子错误，供 errors.Is/As 遍历。
func (e *leafCompletionError) Unwrap() []error {
	return nonNilErrors(e.fault, e.nodeErr, e.timelineErr, e.observationErr)
}

// newLeafCompletionError 构造携带安全主错误和各完成阶段子错误的聚合错误。
func newLeafCompletionError(nodeErr, timelineErr, observationErr error) error {
	return &leafCompletionError{
		fault: mustNodeFault(
			fault.Internal,
			CodeLeafCompletionFailed,
			"leaf execution completion failed",
		),
		nodeErr:        nodeErr,
		timelineErr:    timelineErr,
		observationErr: observationErr,
	}
}

// LeafExecutionError 从聚合完成错误中提取原始节点错误，移除时间线和观测副作用错误。
func LeafExecutionError(err error) error {
	if !containsLeafCompletionError(err) {
		return err
	}
	return withoutLeafCompletionEffects(err)
}

// containsLeafCompletionError 递归判断错误链或多错误聚合中是否包含叶完成错误。
func containsLeafCompletionError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*leafCompletionError); ok {
		return true
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range multi.Unwrap() {
			if containsLeafCompletionError(child) {
				return true
			}
		}
		return false
	}
	return containsLeafCompletionError(errors.Unwrap(err))
}

// withoutLeafCompletionEffects 从错误链移除叶完成副作用并保留原始节点错误结构。
func withoutLeafCompletionEffects(err error) error {
	if err == nil {
		return nil
	}
	if completionErr, ok := err.(*leafCompletionError); ok {
		return completionErr.nodeErr
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		children := multi.Unwrap()
		transformed := make([]error, 0, len(children))
		for _, child := range children {
			if executionErr := withoutLeafCompletionEffects(child); executionErr != nil {
				transformed = append(transformed, executionErr)
			}
		}
		return errors.Join(transformed...)
	}
	if child := errors.Unwrap(err); child != nil && containsLeafCompletionError(child) {
		return withoutLeafCompletionEffects(child)
	}
	return err
}

// stepTimelineStartError 包装步骤时间线开始记录失败。
func stepTimelineStartError(cause error) error {
	return wrapNodeFault(cause, CodeStepTimelineStartFailed, "step timeline start could not be recorded")
}

// stepTimelineFinishError 包装步骤时间线完成记录失败。
func stepTimelineFinishError(cause error) error {
	return wrapNodeFault(cause, CodeStepTimelineFinishFailed, "step timeline finish could not be recorded")
}

// nodeCompletionObservationError 包装节点完成观测记录失败。
func nodeCompletionObservationError(cause error) error {
	return wrapNodeFault(cause, CodeNodeCompletionObservation, "node completion observation could not be recorded")
}

// wrapNodeFault 构造带私有 cause 的 node 内部错误，公开文本仅使用安全消息。
func wrapNodeFault(cause error, code fault.Code, message string) error {
	err, constructionErr := fault.Wrap(cause, fault.Internal, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// mustNodeFault 构造 node 领域错误；构造失败表示程序契约错误并触发 panic。
func mustNodeFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// nonNilErrors 过滤错误列表中的 nil 项并返回新的切片。
func nonNilErrors(values ...error) []error {
	result := make([]error, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, value)
		}
	}
	return result
}

// leafLifecycle 保存叶节点执行期间的时间线、结果和完成处理状态。
type leafLifecycle struct {
	runtime     *Runtime
	execution   StepExecutionRef
	nodeKind    string
	startedAt   time.Time
	startedMark TimelineMark
	nodeOutcome NodeOutcome
}

// beginLeafLifecycle 创建叶生命周期并记录开始时间线；端口缺失或记录失败时返回分类错误。
func (rt *Runtime) beginLeafLifecycle(ctx context.Context, nodeID, nodeKind string, occurrence int) (*leafLifecycle, error) {
	lifecycle := &leafLifecycle{
		runtime:   rt,
		execution: StepExecutionRef{InstanceID: rt.InstanceID, NodeID: nodeID, Occurrence: occurrence},
		nodeKind:  nodeKind,
		startedAt: time.Now(),
	}
	if rt.StepTimeline == nil {
		rt.leafExecutionStarted = true
		return lifecycle, nil
	}
	if rt.Timeline == nil {
		return nil, stepPhaseTransitionInvalidError(errors.New("node: recording timeline is required when step timeline is enabled"))
	}
	lifecycle.startedMark = rt.Timeline.Mark()
	event := StepTimelineEvent{Step: lifecycle.execution, Boundary: StepBoundaryStarted, Mark: lifecycle.startedMark}
	if err := event.Validate(); err != nil {
		// 开始边界事件校验失败与完成边界一样归入阶段迁移错误码，保持两条路径分类一致。
		return nil, stepTimelineStartError(err)
	}
	if err := rt.StepTimeline.RecordStepTimelineEvent(ctx, event); err != nil {
		return nil, stepTimelineStartError(err)
	}
	rt.leafExecutionStarted = true
	return lifecycle, nil
}

// MarkSkipped 将叶节点结果标记为跳过，供 Complete 生成对应 NodeOutcome。
func (l *leafLifecycle) MarkSkipped() {
	l.nodeOutcome = NodeOutcomeSkipped
}

// Complete 记录完成时间线、运行完成处理链和观测；终态记录使用独立短超时上下文，并聚合
// 所有副作用错误而不丢失原始节点错误。
func (l *leafLifecycle) Complete(ctx context.Context, nodeErr error) error {
	completedAt := time.Now()
	outcome, stepOutcome := lifecycleOutcome(nodeErr)
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
			timelineErr = stepTimelineFinishError(timelineErr)
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
	var observationErr error
	if l.runtime.CompletionObserver != nil && len(results) > 0 {
		observation := NodeCompletionObservation{Execution: l.execution, Results: append([]CompletionHandlerResult(nil), results...)}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalEventTimeout)
		if err := l.runtime.CompletionObserver.RecordNodeCompletion(cleanupCtx, observation); err != nil {
			observationErr = nodeCompletionObservationError(err)
		}
		cancel()
	}
	if timelineErr != nil || observationErr != nil {
		return newLeafCompletionError(nodeErr, timelineErr, observationErr)
	}
	return nodeErr
}

// lifecycleOutcome 将节点错误映射为节点结果和步骤终态结果。
func lifecycleOutcome(err error) (NodeOutcome, StepOutcome) {
	if err == nil {
		return NodeOutcomeSucceeded, StepOutcomeSucceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NodeOutcomeCanceled, StepOutcomeCanceled
	}
	return NodeOutcomeFailed, StepOutcomeFailed
}

// cloneErrorSnapshot 深复制可选错误快照；nil 输入保持 nil。
func cloneErrorSnapshot(source *ExecutionErrorSnapshot) *ExecutionErrorSnapshot {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

// snapshotError 将错误分类为安全的 Kind/Code/Message 快照；无法描述时返回 nil。
func snapshotError(err error) *ExecutionErrorSnapshot {
	if err == nil {
		return nil
	}
	classified := classifyNodeFault(err)
	descriptor, ok := fault.Describe(classified)
	if !ok {
		return nil
	}
	return &ExecutionErrorSnapshot{Kind: descriptor.Kind(), Code: descriptor.Code(), Message: descriptor.Message()}
}
