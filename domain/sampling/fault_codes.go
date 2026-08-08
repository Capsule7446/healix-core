package sampling

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	// CodeSessionInputInvalid 表示采样会话输入校验失败。
	CodeSessionInputInvalid fault.Code = "SAMPLING_SESSION_INPUT_INVALID"
	// CodeSessionStateInvalid 表示采样会话状态不允许当前操作。
	CodeSessionStateInvalid fault.Code = "SAMPLING_SESSION_STATE_INVALID"
	// CodeCaptureInvalid 表示采样捕获数据校验失败。
	CodeCaptureInvalid fault.Code = "SAMPLING_CAPTURE_INVALID"
	// CodeDraftInvalid 表示未发布草稿校验失败。
	CodeDraftInvalid fault.Code = "SAMPLING_DRAFT_INVALID"
	// CodeDraftStepNotFound 表示未找到草稿步骤。
	CodeDraftStepNotFound fault.Code = "SAMPLING_DRAFT_STEP_NOT_FOUND"
	// CodeDraftElementTargetNotFound 表示未找到草稿元素目标。
	CodeDraftElementTargetNotFound fault.Code = "SAMPLING_DRAFT_NODE_NOT_FOUND"
	// CodeDraftElementTargetInUse 表示草稿元素目标仍被步骤引用。
	CodeDraftElementTargetInUse fault.Code = "SAMPLING_DRAFT_NODE_IN_USE"
	// CodeDraftIndexOutOfRange 表示草稿步骤索引越界。
	CodeDraftIndexOutOfRange fault.Code = "SAMPLING_DRAFT_INDEX_OUT_OF_RANGE"
	// CodePublicationMappingInvalid 表示采样发布映射校验失败。
	CodePublicationMappingInvalid fault.Code = "SAMPLING_PUBLICATION_MAPPING_INVALID"
	// CodeWorkspaceInvalid 表示采样工作区校验失败。
	CodeWorkspaceInvalid fault.Code = "SAMPLING_WORKSPACE_INVALID"
	// CodeInternal 表示采样操作发生内部错误。
	CodeInternal fault.Code = "SAMPLING_INTERNAL"
)

// sessionInputInvalidError 创建带违规列表的会话输入错误。
func sessionInputInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeSessionInputInvalid, "sampling session input is invalid", fault.WithViolations(capViolations(violations)...))
}

// wrapSessionInputInvalidError 包装会话输入错误，并将解析错误保留为私有 cause。
func wrapSessionInputInvalidError(cause error, violations []fault.Violation) error {
	return wrapSamplingFault(cause, fault.InvalidArgument, CodeSessionInputInvalid, "sampling session input is invalid", fault.WithViolations(capViolations(violations)...))
}

// sessionStateInvalidError 创建表示会话状态不允许操作的错误。
func sessionStateInvalidError() error {
	return mustSamplingFault(fault.FailedPrecondition, CodeSessionStateInvalid, "sampling session state does not allow this operation")
}

// captureInvalidError 创建带违规列表的采样捕获错误。
func captureInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeCaptureInvalid, "sampling capture is invalid", fault.WithViolations(capViolations(violations)...))
}

// draftInvalidError 创建带违规列表的草稿校验错误。
func draftInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeDraftInvalid, "sampling draft is invalid", fault.WithViolations(capViolations(violations)...))
}

// draftStepNotFoundError 创建未找到草稿步骤的错误。
func draftStepNotFoundError() error {
	return mustSamplingFault(fault.NotFound, CodeDraftStepNotFound, "sampling draft step was not found")
}

// draftElementTargetNotFoundError 创建未找到草稿元素目标的错误。
func draftElementTargetNotFoundError() error {
	return mustSamplingFault(fault.NotFound, CodeDraftElementTargetNotFound, "unpublished element target was not found")
}

// draftElementTargetInUseError 创建草稿元素目标仍被引用的错误。
func draftElementTargetInUseError() error {
	return mustSamplingFault(fault.FailedPrecondition, CodeDraftElementTargetInUse, "unpublished element target is still referenced")
}

// draftIndexOutOfRangeError 创建草稿索引越界的错误。
func draftIndexOutOfRangeError() error {
	return mustSamplingFault(fault.OutOfRange, CodeDraftIndexOutOfRange, "sampling draft index is out of range")
}

// publicationMappingInvalidError 创建带违规列表的发布映射错误。
func publicationMappingInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodePublicationMappingInvalid, "sampling publication mapping is invalid", fault.WithViolations(capViolations(violations)...))
}

// workspaceInvalidError 创建带违规列表的工作区错误。
func workspaceInvalidError(violations []fault.Violation) error {
	return mustSamplingFault(fault.InvalidArgument, CodeWorkspaceInvalid, "sampling workspace is invalid", fault.WithViolations(capViolations(violations)...))
}

// internalError 创建采样内部错误。
func internalError() error {
	return mustSamplingFault(fault.Internal, CodeInternal, "sampling operation could not be completed")
}

// wrapInternalError 包装采样内部错误并保留私有 cause。
func wrapInternalError(cause error) error {
	return wrapSamplingFault(cause, fault.Internal, CodeInternal, "sampling operation could not be completed")
}

// capViolations 将违规列表限制为 fault.MaxViolations 项，并保留其前缀顺序。
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

// mustViolation 构造违规值；构造失败表示代码契约错误并触发 panic。
func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}

// mustSamplingFault 构造采样域错误；构造失败表示代码契约错误并触发 panic。
func mustSamplingFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// wrapSamplingFault 包装 cause 构造采样域错误；构造失败表示代码契约错误并触发 panic。
func wrapSamplingFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
