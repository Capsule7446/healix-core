package execution

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// InstanceID 标识一个执行实例。
type InstanceID struct{ value string }

// EntryID 标识实例中的一个执行入口。
type EntryID struct{ value string }

// InvocationPath 标识工作流调用树中的规范调用位置。
type InvocationPath struct{ value string }

// StepExecutionID 标识一次步骤执行。
type StepExecutionID struct{ value string }

// NewInstanceID 校验并创建实例 ID；错误表示值为空、含首尾空白或超过长度上限。
func NewInstanceID(value string) (InstanceID, error) {
	validated, err := validateExecutionIdentity("instance id", value)
	return InstanceID{value: validated}, err
}

// NewEntryID 校验并创建入口 ID；错误表示值为空、含首尾空白或超过长度上限。
func NewEntryID(value string) (EntryID, error) {
	validated, err := validateExecutionIdentity("entry id", value)
	return EntryID{value: validated}, err
}

// NewStepExecutionID 校验并创建步骤执行 ID；错误表示值为空、含首尾空白或超过长度上限。
func NewStepExecutionID(value string) (StepExecutionID, error) {
	validated, err := validateExecutionIdentity("step execution id", value)
	return StepExecutionID{value: validated}, err
}

// RootInvocationPath 将入口 ID 转为对应的根调用路径。
func RootInvocationPath(entry EntryID) InvocationPath {
	return InvocationPath{value: entry.value}
}

// ParseInvocationPath 校验并解析规范调用路径；错误表示路径或任一分段不符合编码规则。
func ParseInvocationPath(value string) (InvocationPath, error) {
	if _, err := validateInvocationPath(value); err != nil {
		return InvocationPath{}, err
	}
	return InvocationPath{value: value}, nil
}

// Child 在调用路径后追加带长度前缀的步骤分段，并返回新的路径值。
func (p InvocationPath) Child(stepID string) (InvocationPath, error) {
	if _, err := validateExecutionIdentity("workflow step id", stepID); err != nil {
		return InvocationPath{}, err
	}
	child := p.value + "/" + strconv.Itoa(len(stepID)) + ":" + stepID
	if _, err := validateInvocationPath(child); err != nil {
		return InvocationPath{}, err
	}
	return InvocationPath{value: child}, nil
}

// String 返回实例 ID 的规范字符串。
func (id InstanceID) String() string { return id.value }

// String 返回入口 ID 的规范字符串。
func (id EntryID) String() string { return id.value }

// String 返回步骤执行 ID 的规范字符串。
func (id StepExecutionID) String() string { return id.value }

// String 返回调用路径的规范字符串。
func (p InvocationPath) String() string { return p.value }

// Validate 校验实例 ID 的结构约束。
func (id InstanceID) Validate() error {
	_, err := validateExecutionIdentity("instance id", id.value)
	return err
}

// Validate 校验入口 ID 的结构约束。
func (id EntryID) Validate() error {
	_, err := validateExecutionIdentity("entry id", id.value)
	return err
}

// Validate 校验步骤执行 ID 的结构约束。
func (id StepExecutionID) Validate() error {
	_, err := validateExecutionIdentity("step execution id", id.value)
	return err
}

// Validate 校验调用路径的根 ID 和长度前缀分段。
func (p InvocationPath) Validate() error { _, err := validateInvocationPath(p.value); return err }

// validateExecutionIdentity 校验通用执行身份字符串的非空、长度和首尾空白约束。
func validateExecutionIdentity(kind, value string) (string, error) {
	if strings.TrimSpace(value) == "" || len(value) > MaxStringBytes {
		return "", fmt.Errorf("%s is invalid", kind)
	}
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", kind)
	}
	return value, nil
}

// validateInvocationPath 校验根调用 ID 及每个带长度前缀的工作流步骤分段。
func validateInvocationPath(value string) (string, error) {
	if _, err := validateExecutionIdentity("invocation path", value); err != nil {
		return "", err
	}
	rootEnd := strings.IndexByte(value, '/')
	if rootEnd < 0 {
		return value, nil
	}
	if _, err := validateExecutionIdentity("invocation root", value[:rootEnd]); err != nil {
		return "", err
	}
	position := rootEnd + 1
	for position < len(value) {
		remaining := value[position:]
		separator := strings.IndexByte(remaining, ':')
		if separator < 1 {
			return "", errors.New("invocation path has an invalid segment")
		}
		length, err := strconv.Atoi(remaining[:separator])
		if err != nil || length < 1 {
			return "", errors.New("invocation path has a noncanonical segment")
		}
		stepStart := position + separator + 1
		stepEnd := stepStart + length
		if stepEnd > len(value) {
			return "", errors.New("invocation path has a noncanonical segment")
		}
		if _, err := validateExecutionIdentity("workflow step id", value[stepStart:stepEnd]); err != nil {
			return "", err
		}
		if stepEnd == len(value) {
			return value, nil
		}
		if value[stepEnd] != '/' {
			return "", errors.New("invocation path has a noncanonical segment")
		}
		position = stepEnd + 1
	}
	return "", errors.New("invocation path has an invalid segment")
}
