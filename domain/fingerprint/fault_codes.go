package fingerprint

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	// CodeSelectorInvalid 表示元素选择器不符合定位器约束。
	CodeSelectorInvalid fault.Code = "FINGERPRINT_SELECTOR_INVALID"
	// CodeElementTargetSpecInvalid 表示元素目标规格不符合共享身份和定位器约束。
	CodeElementTargetSpecInvalid fault.Code = "FINGERPRINT_ELEMENT_TARGET_SPEC_INVALID"
	// CodeDescriptorInvalid 表示元素指纹描述符不符合身份不变量。
	CodeDescriptorInvalid fault.Code = "FINGERPRINT_DESCRIPTOR_INVALID"
	// CodeFrameworkStackInvalid 表示框架栈不符合框架种类、置信度或唯一性约束。
	CodeFrameworkStackInvalid fault.Code = "FINGERPRINT_FRAMEWORK_STACK_INVALID"
	// CodeFrameworkDetectorFailed 表示框架检测端口未能完成检测。
	CodeFrameworkDetectorFailed fault.Code = "FINGERPRINT_FRAMEWORK_DETECTOR_FAILED"
)

// selectorInvalidError 构造选择器无效错误。
func selectorInvalidError() error {
	return mustFingerprintFault(fault.InvalidArgument, CodeSelectorInvalid, "element selector is invalid")
}

// elementTargetSpecInvalidError 构造元素目标规格无效错误，并附带有序字段违规。
func elementTargetSpecInvalidError(violations []fault.Violation) error {
	return mustFingerprintFault(fault.InvalidArgument, CodeElementTargetSpecInvalid, "element target spec is invalid", fault.WithViolations(capViolations(violations)...))
}

// descriptorInvalidError 构造元素指纹描述符无效错误，并附带有序字段违规。
func descriptorInvalidError(violations []fault.Violation) error {
	return mustFingerprintFault(fault.InvalidArgument, CodeDescriptorInvalid, "element fingerprint descriptor is invalid", fault.WithViolations(capViolations(violations)...))
}

// frameworkStackInvalidError 构造框架栈无效错误，并附带有序字段违规。
func frameworkStackInvalidError(violations []fault.Violation) error {
	return mustFingerprintFault(fault.InvalidArgument, CodeFrameworkStackInvalid, "framework stack is invalid", fault.WithViolations(capViolations(violations)...))
}

// frameworkDetectorFailedError 将检测器原始错误保留为私有 cause。
// 宿主提供的检测器可能报告页面 URL 或 DOM 片段，因此其文本不得进入公开错误文本。
func frameworkDetectorFailedError(cause error) error {
	return wrapFingerprintFault(cause, fault.Internal, CodeFrameworkDetectorFailed, "framework detection could not be completed")
}

// capViolations 在聚合违规项超过封套上限时保留确定性的前缀，避免不可信输入将校验变成 panic。
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

// mustViolation 构造已知有效的字段违规；内部构造失败表示程序契约错误并触发 panic。
func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}

// mustFingerprintFault 使用显式 Kind 构造指纹领域错误，避免错误码注册 Kind 不同却被
// 静默归类为 fault.InvalidArgument。
func mustFingerprintFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// wrapFingerprintFault 构造带私有 cause 的指纹领域错误，公开文本只使用安全消息。
func wrapFingerprintFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// joinField 将字段名拼接为相对于当前聚合的逻辑字段路径。
func joinField(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
