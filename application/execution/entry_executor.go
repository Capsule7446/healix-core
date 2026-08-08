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

// entryExecutorConfigurationInvalidError 将执行器配置校验失败包装为前置条件错误。
func entryExecutorConfigurationInvalidError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.FailedPrecondition, CodeEntryExecutorConfigurationInvalid, "entry executor configuration is invalid")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// classifySchedulingAdapterFailure 为未分类的浏览器会话工厂失败补上注册错误码，并让适配器已分类的
// 错误原样通过，避免在边界掩盖宿主适配器已产生的错误码。
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

// entryBrowserSessionAdapterContractViolationError 构造浏览器会话适配器返回非法结果时的内部错误。
func entryBrowserSessionAdapterContractViolationError(cause error) error {
	err, constructionErr := fault.Wrap(cause, fault.Internal, CodeEntryBrowserSessionAdapterContractViolation, "browser session adapter returned an invalid outcome")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// BrowserSession 表示由宿主拥有的、不透明的浏览器/进程生命周期。
// Close 必须阻塞到清理完成或传入 Context 结束；宿主实现必须协作遵守截止时间。EntryExecutor
// 不会异步放弃 Close，也不会先启动下一个 Entry。
type BrowserSession interface {
	// Valid 报告会话是否可用于运行入口。
	Valid() bool
	// Close 清理浏览器会话；应遵守传入 Context 的取消和截止时间。
	Close(context.Context) error
}

// BrowserSessionFactory 定义按工作线程权威和入口创建浏览器会话的端口。
type BrowserSessionFactory interface {
	// Create 创建入口执行所需的浏览器会话。
	Create(context.Context, domainexecution.WorkerFence, domainexecution.Entry) (BrowserSession, error)
}

// EntryRunner 定义在浏览器会话中运行完整顶层入口的端口。
type EntryRunner interface {
	// RunEntry 执行完整的顶层入口；嵌套工作流引用通过 runner 的执行上下文复用同一会话。
	//
	// 它必须在返回错误的同时返回引擎观测结果：中途失败的运行仍有终态决策所需的结果；若只返回
	// 错误，调用方将无法区分取消运行与崩溃运行。
	RunEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry, BrowserSession) (engine.EntryResult, error)
}

// EntryAuthorizer 在为入口创建任何宿主资源前，校验工作线程是否仍持有 fence 指定的权威。
// 执行器不能依赖 engine.RunProgram 自身的校验：该校验在宿主 EntryRunner 内执行，而 EntryRunner
// 只有在浏览器已创建后才会到达。
type EntryAuthorizer interface {
	// AuthorizeEntry 校验工作线程是否获授权运行指定入口。
	AuthorizeEntry(context.Context, domainexecution.WorkerFence, domainexecution.Entry) error
}

// EntryLifecyclePanic 在同一 Entry 生命周期中 runner 执行和清理都 panic 时保留两次 panic。
type EntryLifecyclePanic struct {
	RunnerPanic any
	ClosePanic  any
}

// Error 返回同时发生 runner panic 和浏览器关闭 panic 的摘要文本。
func (p EntryLifecyclePanic) Error() string {
	return fmt.Sprintf("entry runner panic: %v; browser close panic: %v", p.RunnerPanic, p.ClosePanic)
}

// EntryExecutor 在授权后创建会话、运行单个入口并按顺序关闭会话。
type EntryExecutor struct {
	authorizer   EntryAuthorizer
	factory      BrowserSessionFactory
	runner       EntryRunner
	closeTimeout time.Duration
}

// NewEntryExecutor 校验授权器、会话工厂、runner 和关闭超时，并构造入口执行器。
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

// Execute 运行恰好一个已授权入口，并报告其引擎观测结果。
//
// 执行器只负责单个入口；入口顺序和失败后的停止决策属于 Scheduling，因为只有 Scheduling 能看到
// 完整实例并提交终态。若执行器同时负责排序，两个组件可能对实际运行内容产生分歧，事后无法判断
// 哪一方正确。
//
// 返回的 EngineOutcome 始终可用于决策，可原样传给 DecideEntryCompletion，中间无需宿主翻译。每条
// 返回路径都会填充它，包括引擎尚未启动的路径，使调用方仅凭返回值即可提交终态并释放租约。错误仍
// 单独返回：结果说明发生了什么，错误说明原因，宿主审计链需要二者。
func (e EntryExecutor) Execute(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.Entry) (EngineOutcome, error) {
	// fence 校验返回自身的分类错误；此处不包裹，避免未分类外层错误掩盖原分类。
	if err := fence.Validate(); err != nil {
		return engineOutcomeFor(NotStartedEngineOutcome().Result, err), err
	}
	if err := e.authorizer.AuthorizeEntry(ctx, fence, entry); err != nil {
		return engineOutcomeFor(NotStartedEngineOutcome().Result, err), err
	}
	engineResult, err := e.executeEntry(ctx, fence, entry)
	return engineOutcomeFor(engineResult, err), err
}

// engineOutcomeFor 将引擎观测结果与伴随错误的分类配对。错误码在此处提取并写入结果，而不是留给
// 调用方自行提取，因为持久化的是结果；若宿主需重新检查错误填写审计字段，不同宿主可能填写不同值。
func engineOutcomeFor(result engine.EntryResult, err error) EngineOutcome {
	outcome := EngineOutcome{Result: result}
	// 未分类失败保持字段为空而不臆造错误码：空审计项是诚实的，猜测出的错误码不是。
	if code, classified := fault.CodeOf(err); classified {
		outcome.FailureCode = code
	}
	return outcome
}

// executeEntry 创建并校验浏览器会话，运行入口，并在任何返回路径按顺序关闭会话；同时保留 runner
// 和关闭阶段的 panic 语义。
func (e EntryExecutor) executeEntry(ctx context.Context, fence domainexecution.WorkerFence, entry domainexecution.Entry) (engineResult engine.EntryResult, result error) {
	// 以下每条提前返回都报告 NOT_STARTED，而不是零结果：空结果不属于引擎词汇，
	// DecideEntryCompletion 会拒绝它，使入口滞留 RUNNING 并留下无法释放的租约。
	engineResult = NotStartedEngineOutcome().Result

	session, err := e.factory.Create(ctx, fence, entry)
	if err != nil {
		// 两层包装都不包含执行 ID：classifySchedulingAdapterFailure 会原样传递已分类 cause，
		// 外层若仍回显 ID，即使如此也会泄露身份。
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
	// 注册的 defer 始终先捕获 runner panic，再使用脱离父取消的独立超时调用 Close；无论 runner
	// 是否 panic，Close 都会执行。两个阶段同时 panic 时以 EntryLifecyclePanic 保留二者；仅一个
	// 阶段 panic 时原样重新抛出。没有 panic 时，Close 错误加入命名返回错误而不改写引擎结果。
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
	// 在包装错误前写入命名返回值，使延迟关闭加入的清理失败不会改写引擎已报告的结果。
	engineResult, runErr := e.runner.RunEntry(ctx, fence, entry, session)
	result = wrapEntryError("execute", entry.ID.String(), runErr)
	return engineResult, result
}

// closeSession 使用脱离父取消状态的关闭超时清理浏览器会话。
func (e EntryExecutor) closeSession(ctx context.Context, session BrowserSession) error {
	closeContext, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), e.closeTimeout)
	defer cancelClose()
	return session.Close(closeContext)
}

// isNilInterfaceValue 识别直接为 nil 或承载 typed nil 的接口值。
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

// isNilBrowserSession 复用接口 nil 检查判断浏览器会话是否缺失。
func isNilBrowserSession(session BrowserSession) bool {
	return isNilInterfaceValue(session)
}

// wrapEntryError 为入口操作错误添加操作名和入口 ID。
func wrapEntryError(operation, executionID string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s entry %s: %w", operation, executionID, err)
}
