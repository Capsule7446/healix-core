package scheduling

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const claimReleaseTimeout = 5 * time.Second

const (
	// CodeSchedulingDependencyRequired 表示调度协调器缺少必需端口。
	CodeSchedulingDependencyRequired fault.Code = "EXECUTION_SCHEDULING_DEPENDENCY_REQUIRED"
	// CodeSchedulingClaimInvalid 表示领取快照、fence 或摘要身份不一致。
	CodeSchedulingClaimInvalid fault.Code = "EXECUTION_SCHEDULING_CLAIM_INVALID"
	// CodeSchedulingAdapterUnavailable 表示领取、释放、状态读取或决策写入端口失败；需要恢复可达的是
	// 宿主适配器，处理方式是重试而非修改调用参数。
	CodeSchedulingAdapterUnavailable fault.Code = "EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE"
)

// classifySchedulingAdapterFailure 为未分类的调度端口失败补上注册错误码，并让已分类错误（例如
// DecideAdvance 自身的 EXECUTION_ENTRY_STATES_INVALID）原样通过，避免在边界掩盖依赖已产生的错误码。
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

// schedulingClaimInvalidError 构造领取快照或 fence 身份无效的前置条件错误。
func schedulingClaimInvalidError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeSchedulingClaimInvalid,
		"scheduling claim is invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// schedulingDependencyRequiredError 构造调度端口缺失的前置条件错误。
func schedulingDependencyRequiredError() error {
	err, constructionErr := fault.New(fault.FailedPrecondition, CodeSchedulingDependencyRequired, "execution scheduling dependency is required")
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// isNilPort 识别直接为 nil 或承载 typed nil 的调度端口。
func isNilPort(port any) bool {
	if port == nil {
		return true
	}
	value := reflect.ValueOf(port)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Claim 保存调度器领取的实例快照及其工作线程 fence。
type Claim struct {
	Snapshot execution.InstanceSnapshot
	Fence    execution.WorkerFence
}

// ClaimSource 定义领取下一个实例以及释放领取的端口。
type ClaimSource interface {
	// ClaimNext 按 worker ID 和时间领取下一个实例；无可领取实例时返回 found=false。
	ClaimNext(context.Context, string, int64) (Claim, bool, error)
	// Release 释放本次领取。
	Release(context.Context, Claim) error
}

// EntryStateReader 定义读取领取实例入口状态的端口。
type EntryStateReader interface {
	// LoadEntryStates 读取领取对应实例的全部入口状态。
	LoadEntryStates(context.Context, Claim) ([]EntryState, error)
}

// ApplyDecisionResult 保存决策写入是否应用及写入后确认的 fence。
type ApplyDecisionResult struct {
	Fence   execution.WorkerFence
	Applied bool
}

// DecisionWriter 定义以原子 fence 应用完整入口状态决策的端口。
type DecisionWriter interface {
	// ApplyDecision 原子执行 fence 并应用一个纯 Decision 产生的全部入口状态写入。Transitions 是
	// 完整列表；NextEntryID 是正在启动入口的快捷引用（其 Pending→Running 转换包含在 Transitions 中）。
	ApplyDecision(context.Context, Claim, Decision, int64) (ApplyDecisionResult, error)
}

// Coordinator 编排领取、读取入口状态、计算决策、原子写入和释放领取。
type Coordinator struct {
	claims ClaimSource
	states EntryStateReader
	writer DecisionWriter
}

// NewCoordinator 构造调度协调器；依赖缺失时由 ProcessNext 返回配置错误。
func NewCoordinator(claims ClaimSource, states EntryStateReader, writer DecisionWriter) Coordinator {
	return Coordinator{claims: claims, states: states, writer: writer}
}

// ProcessNext 领取并处理一个实例；返回是否成功领取以及处理或释放过程中的错误。
func (c Coordinator) ProcessNext(ctx context.Context, workerID string, occurredAt int64) (claimed bool, resultErr error) {
	if isNilPort(c.claims) || isNilPort(c.states) || isNilPort(c.writer) {
		return false, schedulingDependencyRequiredError()
	}
	claim, found, err := c.claims.ClaimNext(ctx, workerID, occurredAt)
	if err != nil {
		return false, classifySchedulingAdapterFailure(err)
	}
	if !found {
		return false, nil
	}
	defer func() {
		releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), claimReleaseTimeout)
		defer cancel()
		if err := c.claims.Release(releaseContext, claim); err != nil {
			resultErr = errors.Join(resultErr, classifySchedulingAdapterFailure(err))
		}
	}()
	if claim.Fence.Validate() != nil || claim.Fence.InstanceID != claim.Snapshot.InstanceID() || claim.Snapshot.Digest() == "" {
		return true, schedulingClaimInvalidError()
	}
	states, err := c.states.LoadEntryStates(ctx, claim)
	if err != nil {
		return true, classifySchedulingAdapterFailure(err)
	}
	decision, err := DecideAdvance(claim.Snapshot, states)
	if err != nil {
		// DecideAdvance 已返回 EXECUTION_ENTRY_STATES_INVALID。
		return true, err
	}
	if decision.NextEntryID.Validate() != nil && len(decision.Transitions) == 0 && decision.FinalStatus == nil {
		return true, nil
	}
	applied, err := c.writer.ApplyDecision(ctx, claim, decision, occurredAt)
	if err != nil {
		return true, classifySchedulingAdapterFailure(err)
	}
	if !applied.Applied || applied.Fence != claim.Fence {
		return true, execution.NewStaleWorkerFenceError()
	}
	return true, nil
}
