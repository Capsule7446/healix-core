package parameter

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	// CodeNameInvalid 表示参数名称违反命名约束。
	CodeNameInvalid fault.Code = "PARAMETER_NAME_INVALID"
	// CodeValueInvalid 表示参数值无效。
	CodeValueInvalid fault.Code = "PARAMETER_VALUE_INVALID"
	// CodeConstraintUnsatisfied 表示参数值不满足约束。
	CodeConstraintUnsatisfied fault.Code = "PARAMETER_CONSTRAINT_UNSATISFIED"
	// CodeBindingUnresolvable 表示绑定合法但父作用域缺少所引用值。
	CodeBindingUnresolvable fault.Code = "PARAMETER_BINDING_UNRESOLVABLE"
	// CodeBindingInvalid 表示参数绑定结构或种类无效。
	CodeBindingInvalid fault.Code = "PARAMETER_BINDING_INVALID"
)

// 这些错误码不携带字段违规；每个错误只报告调用方已经持有的单个被拒值，额外拆分字段
// 不会提供新的可操作信息，也能避免在公开文本中泄露输入。

// nameInvalidError 构造参数名称无效错误。
func nameInvalidError() error {
	return mustParameterFault(fault.InvalidArgument, CodeNameInvalid, "parameter name is invalid")
}

// valueInvalidError 构造参数值无效错误。
func valueInvalidError() error {
	return mustParameterFault(fault.InvalidArgument, CodeValueInvalid, "parameter value is invalid")
}

// wrapValueInvalidError 将规范化失败保留为私有 cause，且 cause 不携带被拒参数输入。
func wrapValueInvalidError(cause error) error {
	return wrapParameterFault(cause, fault.InvalidArgument, CodeValueInvalid, "parameter value is invalid")
}

// constraintUnsatisfiedError 构造参数值不满足约束错误。
func constraintUnsatisfiedError() error {
	return mustParameterFault(fault.InvalidArgument, CodeConstraintUnsatisfied, "parameter value does not satisfy its constraint")
}

// bindingInvalidError 构造绑定结构无效错误，适用于空父引用名或零值/未知种类；调用方需
// 重新构造绑定。它不同于 CodeBindingUnresolvable，后者表示绑定合法但作用域缺少值。
func bindingInvalidError() error {
	return mustParameterFault(fault.InvalidArgument, CodeBindingInvalid, "parameter binding is invalid")
}

// bindingUnresolvableError 构造父作用域缺少绑定值的前置条件错误。
func bindingUnresolvableError() error {
	return mustParameterFault(fault.FailedPrecondition, CodeBindingUnresolvable, "parameter binding cannot be resolved")
}

// mustParameterFault 构造参数领域错误；构造失败表示程序契约错误并触发 panic。
func mustParameterFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// wrapParameterFault 构造带私有 cause 的参数领域错误，公开文本仅使用安全消息。
func wrapParameterFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
