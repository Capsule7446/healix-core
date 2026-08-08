package node

import (
	"context"
	"errors"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// transientDriverFault 将驱动临时不可用原因包装为可重试的 node 错误。
func transientDriverFault(cause error) error {
	if cause == nil {
		return nil
	}
	return mustWrapNodeFault(cause, fault.Unavailable, CodeTransientDriver, "node driver is temporarily unavailable")
}

// classifyNodeFault 将取消、超时、未找到和未知驱动原因映射为稳定 node 错误码。
func classifyNodeFault(cause error) error {
	if cause == nil {
		return nil
	}
	if _, ok := fault.CodeOf(cause); ok {
		return cause
	}
	switch {
	case errors.Is(cause, context.Canceled):
		return mustWrapNodeFault(cause, fault.Canceled, CodeCanceled, "node operation was canceled")
	case errors.Is(cause, context.DeadlineExceeded):
		return mustWrapNodeFault(cause, fault.DeadlineExceeded, CodeTimeout, "node operation timed out")
	case fault.IsCode(cause, CodeElementNotFound):
		return mustWrapNodeFault(cause, fault.NotFound, CodeElementNotFound, "element was not found")
	default:
		return mustWrapNodeFault(cause, fault.Internal, CodeOperationFailed, "node operation failed")
	}
}

// nodeFaultDetails 返回错误的稳定 Kind 和 Code；未分类错误先执行 node 分类。
func nodeFaultDetails(err error) (fault.Kind, fault.Code) {
	if err == nil {
		return "", ""
	}
	if kind, ok := fault.KindOf(err); ok {
		code, _ := fault.CodeOf(err)
		return kind, code
	}
	classified := classifyNodeFault(err)
	kind, _ := fault.KindOf(classified)
	code, _ := fault.CodeOf(classified)
	return kind, code
}

// nodeFaultKind 返回错误的稳定 Kind。
func nodeFaultKind(err error) fault.Kind {
	kind, _ := nodeFaultDetails(err)
	return kind
}

// nodeFaultCode 返回错误的稳定 Code。
func nodeFaultCode(err error) fault.Code {
	_, code := nodeFaultDetails(err)
	return code
}

// mustWrapNodeFault 构造带 cause 的 node 领域错误；构造失败表示程序契约错误并触发 panic。
func mustWrapNodeFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	wrapped, err := fault.Wrap(cause, kind, code, message, options...)
	if err != nil {
		panic(err)
	}
	return wrapped
}

// classifyStepPhaseTransitionInvalid 分类步骤阶段机和叶生命周期入口错误；已分类错误原样
// 通过，其他阶段入口错误归入阶段迁移错误码且不回显节点 ID 或阶段名。
func classifyStepPhaseTransitionInvalid(cause error) error {
	if cause == nil {
		return nil
	}
	if _, ok := fault.CodeOf(cause); ok {
		return cause
	}
	return stepPhaseTransitionInvalidError(cause)
}

// stepPhaseTransitionInvalidError 构造步骤阶段迁移无效错误。
func stepPhaseTransitionInvalidError(cause error) error {
	return mustWrapNodeFault(cause, fault.FailedPrecondition, CodeStepPhaseTransitionInvalid, "step phase transition is invalid")
}

// stepConfigurationInvalidError 构造不带独立 cause 的步骤配置聚合错误，详情由违规项承载。
func stepConfigurationInvalidError(violations ...fault.Violation) error {
	return mustNodeFault(fault.InvalidArgument, CodeStepConfigurationInvalid, "step configuration is invalid", fault.WithViolations(violations...))
}

// wrapStepConfigurationInvalidError 构造步骤配置聚合错误，并将底层 Go 错误保留为私有 cause，
// 避免被拒值（如 URL scheme 或未知种类）进入公开文本。
func wrapStepConfigurationInvalidError(cause error, violations ...fault.Violation) error {
	return mustWrapNodeFault(cause, fault.InvalidArgument, CodeStepConfigurationInvalid, "step configuration is invalid", fault.WithViolations(violations...))
}

// healingRefusedError 表示自愈策略自身拒绝（安全拒绝或没有达到审核阈值的候选），不表示
// 自愈适配器失败。
func healingRefusedError(cause error) error {
	return mustWrapNodeFault(cause, fault.FailedPrecondition, CodeHealingRefused, "healing was refused")
}

// evidenceRecordFailedError 表示 Facts 端口记录失败，包括事件、样本或决策暂存；它与只分类
// Driver 端口失败的 CodeTransientDriver 不同。
func evidenceRecordFailedError(cause error) error {
	return mustWrapNodeFault(cause, fault.Unavailable, CodeEvidenceRecordFailed, "execution evidence could not be recorded")
}

// mustViolation 构造已知有效的字段违规；内部构造失败表示程序契约错误并触发 panic。
func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}
