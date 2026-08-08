// Package fault 定义 Core 各上下文共享的安全、稳定业务错误契约。
package fault

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind 描述业务错误的一般处置策略。
type Kind string

const (
	// InvalidArgument 表示调用方提供的参数或命令形状无效。
	InvalidArgument Kind = "INVALID_ARGUMENT"
	// OutOfRange 表示参数超出允许范围。
	OutOfRange Kind = "OUT_OF_RANGE"
	// NotFound 表示请求的权威资源不存在。
	NotFound Kind = "NOT_FOUND"
	// AlreadyExists 表示请求创建的资源已存在。
	AlreadyExists Kind = "ALREADY_EXISTS"
	// Conflict 表示请求与当前权威状态发生冲突。
	Conflict Kind = "CONFLICT"
	// FailedPrecondition 表示当前前置条件不满足，重试请求本身不会改变结果。
	FailedPrecondition Kind = "FAILED_PRECONDITION"
	// ResourceExhausted 表示资源配额或容量已耗尽。
	ResourceExhausted Kind = "RESOURCE_EXHAUSTED"
	// Canceled 表示操作被取消。
	Canceled Kind = "CANCELED"
	// DeadlineExceeded 表示操作超过截止时间。
	DeadlineExceeded Kind = "DEADLINE_EXCEEDED"
	// Unavailable 表示依赖暂时不可用，通常可通过重试恢复。
	Unavailable Kind = "UNAVAILABLE"
	// Internal 表示系统内部错误或适配器契约违规。
	Internal Kind = "INTERNAL"
)

// Code 是由所属上下文拥有且稳定不变的业务错误标识。
type Code string

// Error 将错误码作为 error 字符串返回。
func (c Code) Error() string {
	return string(c)
}

// ParamKey 标识一个安全且与语言环境无关的参数。
type ParamKey string

var (
	codePattern     = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,62}$`)
	paramKeyPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
	fieldPattern    = regexp.MustCompile(`^[a-z][A-Za-z0-9.]{0,126}$`)
)

// MaxViolations 限制单个聚合错误封套最多携带的违规项数量。
// 该常量导出后，各上下文可确定性地截断恶意输入，而不是让构造过程失败。
const MaxViolations = 32

const (
	// maxMessageLength 限制公开错误消息的字节长度。
	maxMessageLength = 512
	// maxParams 限制单个错误或违规项携带的参数数量。
	maxParams = 16
	// maxParamValueLen 限制单个参数值的字节长度。
	maxParamValueLen = 256
)

// Param 是不可变且安全的参数值。
type Param struct {
	key   ParamKey
	value string
}

// NewParam 校验并创建参数；结果不共享可变输入，错误表示键或值违反安全约束。
func NewParam(key ParamKey, value string) (Param, error) {
	if !paramKeyPattern.MatchString(string(key)) {
		return Param{}, errors.New("invalid fault parameter key")
	}
	if len(value) > maxParamValueLen {
		return Param{}, errors.New("fault parameter value exceeds maximum length")
	}
	if containsUnsafePublicText(value) {
		return Param{}, errors.New("fault parameter value contains control characters")
	}
	return Param{key: key, value: value}, nil
}

// Key 返回参数键。
func (p Param) Key() ParamKey { return p.key }

// Value 返回参数值。
func (p Param) Value() string { return p.value }

// Violation 描述一项安全的字段级校验失败。
type Violation struct {
	code    Code
	field   string
	message string
	params  []Param
}

// NewViolation 校验并创建字段级违规；参数切片会被复制，调用方保留原切片所有权。
func NewViolation(code Code, field, message string, params ...Param) (Violation, error) {
	if err := validateCode(code); err != nil {
		return Violation{}, err
	}
	if !fieldPattern.MatchString(field) {
		return Violation{}, errors.New("invalid violation field")
	}
	if err := validateMessage(message); err != nil {
		return Violation{}, err
	}
	if err := validateParams(params); err != nil {
		return Violation{}, err
	}
	return Violation{code: code, field: field, message: message, params: cloneParams(params)}, nil
}

// Code 返回字段级违规原因码。
func (v Violation) Code() Code { return v.code }

// Field 返回发生违规的字段路径。
func (v Violation) Field() string { return v.field }

// Message 返回不含不安全控制字符的字段级安全消息。
func (v Violation) Message() string { return v.message }

// Params 返回参数副本，避免调用方修改违规内部状态。
func (v Violation) Params() []Param { return cloneParams(v.params) }

// Option 向 Error 添加安全的结构化细节。
type Option func(*faultOptions) error

// faultOptions 收集构造 Error 时附加的参数和字段级违规。
type faultOptions struct {
	params     []Param
	violations []Violation
}

// WithParams 返回把安全参数附加到 Error 的构造选项；选项执行时会追加而不接管输入切片。
func WithParams(params ...Param) Option {
	return func(values *faultOptions) error {
		values.params = append(values.params, params...)
		return nil
	}
}

// WithViolations 返回把字段级违规附加到 Error 的构造选项；最终构造时超过 MaxViolations 的尾部会被截断。
func WithViolations(violations ...Violation) Option {
	return func(values *faultOptions) error {
		values.violations = append(values.violations, violations...)
		return nil
	}
}

// Error 是安全的业务错误。其 cause 只通过 Unwrap 暴露，默认格式化不会泄露私有原因内容。
type Error struct {
	kind       Kind
	code       Code
	message    string
	params     []Param
	violations []Violation
	cause      error
}

// New 校验并构造不带 cause 的安全业务错误；返回错误表示 kind、code、消息或选项无效。
func New(kind Kind, code Code, safeMessage string, options ...Option) (*Error, error) {
	return construct(nil, kind, code, safeMessage, options...)
}

// Wrap 校验并构造带 cause 的安全业务错误；cause 仅可通过 Unwrap 读取。
func Wrap(cause error, kind Kind, code Code, safeMessage string, options ...Option) (*Error, error) {
	return construct(cause, kind, code, safeMessage, options...)
}

// construct 执行 New 和 Wrap 共用的字段、选项、违规数量校验，并复制所有可变切片。
func construct(cause error, kind Kind, code Code, safeMessage string, options ...Option) (*Error, error) {
	if err := validateKind(kind); err != nil {
		return nil, err
	}
	if err := validateCode(code); err != nil {
		return nil, err
	}
	if err := validateMessage(safeMessage); err != nil {
		return nil, err
	}
	values := faultOptions{}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("fault option is required")
		}
		if err := option(&values); err != nil {
			return nil, err
		}
	}
	if err := validateParams(values.params); err != nil {
		return nil, err
	}
	// 此处选择截断而非拒绝是有意的。违规项来自不可信输入，超过上限是恶意负载的常见
	// 结果，并非编程错误。让构造失败会把该负载变成构造错误，而各上下文使用的必须构造
	// 惯用法又会将其转为 panic。保留确定性的前缀会降低报告完整度而非拖垮进程；调用方
	// 若已自行截断则不受影响。
	if len(values.violations) > MaxViolations {
		values.violations = values.violations[:MaxViolations]
	}
	if err := validateViolations(values.violations); err != nil {
		return nil, err
	}
	return &Error{kind: kind, code: code, message: safeMessage, params: cloneParams(values.params), violations: cloneViolations(values.violations), cause: cause}, nil
}

// Error 返回由稳定错误码和安全消息组成的公开字符串；nil 接收者返回 "<nil>"。
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return string(e.code) + ": " + e.message
}

// Unwrap 返回私有 cause 以支持 errors.Is/As；nil 接收者返回 nil。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Format 让所有格式化动词都停留在安全公开表面。没有它，%#v 和 %+v 的默认结构体格式
// 化会反射未导出字段并打印私有 cause；cause 是错误中唯一允许携带令牌、URL 或页面内容
// 的部分。宿主若用 %#v 写日志，就会发布 Unwrap 原本用于隔离的内容。确实需要 cause
// 的调用方必须通过 Unwrap 显式请求。
func (e *Error) Format(state fmt.State, verb rune) {
	if e == nil {
		_, _ = io.WriteString(state, "<nil>")
		return
	}
	switch verb {
	case 'q':
		_, _ = io.WriteString(state, strconv.Quote(e.Error()))
	default:
		_, _ = io.WriteString(state, e.Error())
	}
}

// Is 仅按稳定错误码匹配另一个业务错误或 Code。
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	if code, ok := target.(Code); ok {
		return e.code == code
	}
	other, ok := target.(*Error)
	return ok && other != nil && e.code == other.code
}

// Kind 返回错误种类；nil 接收者返回空 Kind。
func (e *Error) Kind() Kind {
	if e == nil {
		return ""
	}
	return e.kind
}

// Code 返回稳定错误码；nil 接收者返回空 Code。
func (e *Error) Code() Code {
	if e == nil {
		return ""
	}
	return e.code
}

// Message 返回安全错误消息；nil 接收者返回空字符串。
func (e *Error) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// Params 返回参数的深拷贝；nil 接收者返回 nil。
func (e *Error) Params() []Param {
	if e == nil {
		return nil
	}
	return cloneParams(e.params)
}

// Violations 返回违规项及其参数的深拷贝；nil 接收者返回 nil。
func (e *Error) Violations() []Violation {
	if e == nil {
		return nil
	}
	return cloneViolations(e.violations)
}

// Descriptor 是不携带 cause 的 Error 不可变快照。
type Descriptor struct {
	kind       Kind
	code       Code
	message    string
	params     []Param
	violations []Violation
}

// Kind 返回快照中的错误种类。
func (d Descriptor) Kind() Kind { return d.kind }

// Code 返回快照中的稳定错误码。
func (d Descriptor) Code() Code { return d.code }

// Message 返回快照中的安全错误消息。
func (d Descriptor) Message() string { return d.message }

// Params 返回快照参数的深拷贝。
func (d Descriptor) Params() []Param { return cloneParams(d.params) }

// Violations 返回快照违规项及其参数的深拷贝。
func (d Descriptor) Violations() []Violation { return cloneViolations(d.violations) }

// CodeOf 返回错误链边界业务错误的稳定码；找不到业务错误时返回 false。
func CodeOf(err error) (Code, bool) {
	fault, ok := find(err)
	if !ok {
		return "", false
	}
	return fault.code, true
}

// KindOf 返回错误链边界业务错误的种类；找不到业务错误时返回 false。
func KindOf(err error) (Kind, bool) {
	fault, ok := find(err)
	if !ok {
		return "", false
	}
	return fault.kind, true
}

// IsCode 查询 code 是否出现在错误链的任意位置，这与 CodeOf、KindOf 和 Describe 回答
// 的问题不同。
//
// 那三个函数报告边界错误，即宿主用于路由和呈现的唯一分类。IsCode 会遍历整条链，
// 因而嵌套在其中的错误也会使它返回 true。这一区别是有意且承载业务语义的：一次叶子
// 完成可能同时因多个原因失败，调用方会包装每个贡献错误，使宿主能够查询全部原因，
// 同时保留一个用于行动的主错误码。
//
// 这一区别必须明确，因为两者可能不一致；宿主若用 CodeOf 分类却用 IsCode 分支，同一
// 错误会被路由到两条路径。使用 CodeOf 或 Describe 判断错误“是什么”，使用 IsCode
// 查询某个失败是否“参与其中”。
func IsCode(err error, code Code) bool {
	if validateCode(code) != nil {
		return false
	}
	return errors.Is(err, code)
}

// Describe 提取错误链边界业务错误的不可变公开快照；找不到业务错误时返回 false。
func Describe(err error) (Descriptor, bool) {
	fault, ok := find(err)
	if !ok {
		return Descriptor{}, false
	}
	return Descriptor{kind: fault.kind, code: fault.code, message: fault.message, params: cloneParams(fault.params), violations: cloneViolations(fault.violations)}, true
}

// find 从错误链提取首个业务 Error；找不到或提取到 nil 时返回 false。
func find(err error) (*Error, bool) {
	var fault *Error
	if !errors.As(err, &fault) || fault == nil {
		return nil, false
	}
	return fault, true
}

// validateKind 校验错误种类是否属于稳定的 Kind 词汇。
func validateKind(kind Kind) error {
	switch kind {
	case InvalidArgument, OutOfRange, NotFound, AlreadyExists, Conflict, FailedPrecondition, ResourceExhausted, Canceled, DeadlineExceeded, Unavailable, Internal:
		return nil
	default:
		return errors.New("unsupported fault kind")
	}
}

// validateCode 校验错误码是否符合公开错误码格式。
func validateCode(code Code) error {
	if !codePattern.MatchString(string(code)) {
		return errors.New("invalid fault code")
	}
	return nil
}

// validateMessage 校验安全消息非空、长度受限且只含可公开打印字符。
func validateMessage(message string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("fault message is required")
	}
	if len(message) > maxMessageLength {
		return errors.New("fault message exceeds maximum length")
	}
	if containsUnsafePublicText(message) {
		return errors.New("fault message contains control characters")
	}
	return nil
}

// containsUnsafePublicText 拒绝可能导致公开文本渲染结果偏离字面内容的字符串。
//
// 过去手工列出两个分隔符只覆盖 U+2028 和 U+2029，其他 Unicode 空格分隔符、组合标记、
// 变体选择符和默认可忽略字符都能通过。单独的 U+3164 韩文填充字符渲染为空，也曾被视为
// 安全英文。现在改为正面定义规则：公开文本必须是可打印 ASCII，错误码注册表中的所有
// 安全消息都满足这一点。调用方需要的其他内容应放入私有 cause，其中渲染不会泄露它。
func containsUnsafePublicText(value string) bool {
	if !utf8.ValidString(value) {
		return true
	}
	for _, character := range value {
		if character < 0x20 || character > 0x7E {
			return true
		}
	}
	return false
}

// validateViolations 校验违规数量、字段级错误码、字段路径、安全消息和参数。
func validateViolations(violations []Violation) error {
	if len(violations) > MaxViolations {
		return errors.New("fault violations exceed maximum count")
	}
	for _, violation := range violations {
		if err := validateCode(violation.code); err != nil {
			return err
		}
		if !fieldPattern.MatchString(violation.field) {
			return errors.New("invalid violation field")
		}
		if err := validateMessage(violation.message); err != nil {
			return err
		}
		if err := validateParams(violation.params); err != nil {
			return err
		}
	}
	return nil
}

// validateParams 校验参数数量、键值安全性及键的唯一性。
func validateParams(params []Param) error {
	if len(params) > maxParams {
		return errors.New("fault parameters exceed maximum count")
	}
	seen := make(map[ParamKey]struct{}, len(params))
	for _, param := range params {
		if !paramKeyPattern.MatchString(string(param.key)) {
			return errors.New("invalid fault parameter key")
		}
		if len(param.value) > maxParamValueLen {
			return errors.New("fault parameter value exceeds maximum length")
		}
		if containsUnsafePublicText(param.value) {
			return errors.New("fault parameter value contains control characters")
		}
		if _, duplicate := seen[param.key]; duplicate {
			return errors.New("duplicate fault parameter key")
		}
		seen[param.key] = struct{}{}
	}
	return nil
}

// cloneParams 复制参数切片；nil 输入保持 nil，结果由调用方独占切片所有权。
func cloneParams(params []Param) []Param { return append([]Param(nil), params...) }

// cloneViolations 复制违规切片及每项的参数切片，隔离内部所有权。
func cloneViolations(violations []Violation) []Violation {
	result := make([]Violation, len(violations))
	for index, violation := range violations {
		result[index] = violation
		result[index].params = cloneParams(violation.params)
	}
	return result
}
