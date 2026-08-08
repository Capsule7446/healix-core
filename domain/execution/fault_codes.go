package execution

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	// CodePlanUnsealed 表示执行计划尚未封存，不能作为实例快照使用。
	CodePlanUnsealed fault.Code = "EXECUTION_PLAN_UNSEALED"
	// CodeStatusTransitionInvalid 表示入口状态迁移不符合领域规则。
	CodeStatusTransitionInvalid fault.Code = "EXECUTION_STATUS_TRANSITION_INVALID"
	// CodeInstanceStatusTransitionInvalid 表示实例状态迁移不符合领域规则。
	CodeInstanceStatusTransitionInvalid fault.Code = "EXECUTION_INSTANCE_STATUS_TRANSITION_INVALID"
	// CodeWorkerFenceStale 表示 worker fence 已过期或不再是权威 fence。
	CodeWorkerFenceStale fault.Code = "EXECUTION_WORKER_FENCE_STALE"
	// CodeWorkerFenceInvalid 表示 worker fence 的身份或内容无效。
	CodeWorkerFenceInvalid fault.Code = "EXECUTION_WORKER_FENCE_INVALID"

	// CodeCreateInstancePlanInvalid 表示创建实例计划校验失败。
	CodeCreateInstancePlanInvalid fault.Code = "EXECUTION_CREATE_INSTANCE_PLAN_INVALID"
	// CodeCreateInstanceStepShapeInvalid 表示创建实例步骤树形状校验失败。
	CodeCreateInstanceStepShapeInvalid fault.Code = "EXECUTION_CREATE_INSTANCE_STEP_SHAPE_INVALID"
	// CodeCreateInstanceSnapshotInvalid 表示创建实例快照形状校验失败。
	CodeCreateInstanceSnapshotInvalid fault.Code = "EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID"
	// CodeEnvironmentSnapshotInvalid 表示实例环境、截图或自愈策略快照校验失败。
	CodeEnvironmentSnapshotInvalid fault.Code = "EXECUTION_ENVIRONMENT_SNAPSHOT_INVALID"

	// CodeCreateInstanceSnapshotConflict 表示创建实例快照与权威实例状态冲突；其注册字符串
	// 与 application/scheduling 的同名契约保持一致。
	CodeCreateInstanceSnapshotConflict fault.Code = "EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT"
)

// mustExecutionFault 构造执行领域错误；构造失败表示程序契约错误并触发 panic。
func mustExecutionFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// wrapExecutionFault 构造带私有 cause 的执行领域错误，公开文本仅使用安全消息。
func wrapExecutionFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// classifyCreateInstancePlan 为未分类的执行计划校验失败补上注册错误码，并让已分类的
// 步骤形状、指纹规格或参数错误原样通过，避免再次包装而掩盖宿主需要分支处理的错误码。
func classifyCreateInstancePlan(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return wrapExecutionFault(cause, fault.InvalidArgument, CodeCreateInstancePlanInvalid, "create-instance plan is invalid")
}

// classifyCreateInstanceSnapshot 为封存、恢复和创建实例共用的快照边界分类未编码错误，
// 将其归入 EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID，并让计划、步骤形状或环境错误原样通过。
func classifyCreateInstanceSnapshot(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return wrapExecutionFault(cause, fault.InvalidArgument, CodeCreateInstanceSnapshotInvalid, "create-instance snapshot is invalid")
}

// wrapOrPropagate 保持已分类 cause 不变，否则对未分类 cause 应用 annotate；调用方可借此
// 为私有 cause 附加身份，而不会再次包装并掩盖其他上下文已有的错误码。
func wrapOrPropagate(cause error, annotate func(error) error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return annotate(cause)
}

// stepShapeInvalidError 构造工作流快照步骤树的聚合校验错误，以有序违规项承载全部形状
// 失败，并在构造时再次执行违规数量上限保护。
func stepShapeInvalidError(violations []fault.Violation) error {
	return mustExecutionFault(fault.InvalidArgument, CodeCreateInstanceStepShapeInvalid, "create-instance step shape is invalid", fault.WithViolations(capViolations(violations)...))
}

// environmentSnapshotInvalidError 构造实例环境、截图和自愈策略快照的聚合校验错误。
func environmentSnapshotInvalidError(violations []fault.Violation) error {
	return mustExecutionFault(fault.InvalidArgument, CodeEnvironmentSnapshotInvalid, "environment snapshot is invalid", fault.WithViolations(capViolations(violations)...))
}

// createInstanceSnapshotConflictError 构造与 application/scheduling 共享注册字符串和安全
// 消息的创建实例快照冲突错误。
func createInstanceSnapshotConflictError() error {
	return mustExecutionFault(fault.Conflict, CodeCreateInstanceSnapshotConflict, "create-instance snapshot conflicts with the authoritative instance")
}

// capViolations 在聚合违规项超过封套上限时保留确定性的前缀，避免不可信输入将校验变成 panic。
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

// atCap 判断违规封套是否已满，使收集遍历在达到上限后停止并保留输入顺序前缀。
func atCap(violations []fault.Violation) bool {
	return len(violations) >= fault.MaxViolations
}

// mustViolation 构造已知有效的字段违规；内部构造失败表示程序契约错误并触发 panic。
func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}
