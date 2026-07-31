// Package fault defines safe, stable business-fault contracts shared by Core contexts.
package fault

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind describes the general remediation strategy for a business fault.
type Kind string

const (
	InvalidArgument    Kind = "INVALID_ARGUMENT"
	OutOfRange         Kind = "OUT_OF_RANGE"
	NotFound           Kind = "NOT_FOUND"
	AlreadyExists      Kind = "ALREADY_EXISTS"
	Conflict           Kind = "CONFLICT"
	FailedPrecondition Kind = "FAILED_PRECONDITION"
	ResourceExhausted  Kind = "RESOURCE_EXHAUSTED"
	Canceled           Kind = "CANCELED"
	DeadlineExceeded   Kind = "DEADLINE_EXCEEDED"
	Unavailable        Kind = "UNAVAILABLE"
	Internal           Kind = "INTERNAL"
)

// Code is a context-owned, stable business-fault identifier.
type Code string

func (c Code) Error() string {
	return string(c)
}

// ParamKey identifies a safe, locale-neutral parameter.
type ParamKey string

var (
	codePattern     = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,62}$`)
	paramKeyPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
	fieldPattern    = regexp.MustCompile(`^[a-z][A-Za-z0-9.]{0,126}$`)
)

// MaxViolations bounds how many violations one aggregate envelope may carry.
// It is exported so that every context can truncate deterministically instead of
// letting construction fail on hostile input.
const MaxViolations = 32

const (
	maxMessageLength = 512
	maxParams        = 16
	maxParamValueLen = 256
)

// Param is an immutable safe parameter value.
type Param struct {
	key   ParamKey
	value string
}

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

func (p Param) Key() ParamKey { return p.key }
func (p Param) Value() string { return p.value }

// Violation describes one safe field-level validation failure.
type Violation struct {
	code    Code
	field   string
	message string
	params  []Param
}

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

func (v Violation) Code() Code      { return v.code }
func (v Violation) Field() string   { return v.field }
func (v Violation) Message() string { return v.message }
func (v Violation) Params() []Param { return cloneParams(v.params) }

// Option adds safe structured details to an Error.
type Option func(*faultOptions) error

type faultOptions struct {
	params     []Param
	violations []Violation
}

func WithParams(params ...Param) Option {
	return func(values *faultOptions) error {
		values.params = append(values.params, params...)
		return nil
	}
}

func WithViolations(violations ...Violation) Option {
	return func(values *faultOptions) error {
		values.violations = append(values.violations, violations...)
		return nil
	}
}

// Error is a safe business fault. Its cause is deliberately only available through Unwrap.
type Error struct {
	kind       Kind
	code       Code
	message    string
	params     []Param
	violations []Violation
	cause      error
}

func New(kind Kind, code Code, safeMessage string, options ...Option) (*Error, error) {
	return construct(nil, kind, code, safeMessage, options...)
}

func Wrap(cause error, kind Kind, code Code, safeMessage string, options ...Option) (*Error, error) {
	return construct(cause, kind, code, safeMessage, options...)
}

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
	if err := validateViolations(values.violations); err != nil {
		return nil, err
	}
	return &Error{kind: kind, code: code, message: safeMessage, params: cloneParams(values.params), violations: cloneViolations(values.violations), cause: cause}, nil
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return string(e.code) + ": " + e.message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is matches another fault only by stable code.
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

func (e *Error) Kind() Kind {
	if e == nil {
		return ""
	}
	return e.kind
}
func (e *Error) Code() Code {
	if e == nil {
		return ""
	}
	return e.code
}
func (e *Error) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}
func (e *Error) Params() []Param {
	if e == nil {
		return nil
	}
	return cloneParams(e.params)
}
func (e *Error) Violations() []Violation {
	if e == nil {
		return nil
	}
	return cloneViolations(e.violations)
}

// Descriptor is a cause-free immutable snapshot of an Error.
type Descriptor struct {
	kind       Kind
	code       Code
	message    string
	params     []Param
	violations []Violation
}

func (d Descriptor) Kind() Kind              { return d.kind }
func (d Descriptor) Code() Code              { return d.code }
func (d Descriptor) Message() string         { return d.message }
func (d Descriptor) Params() []Param         { return cloneParams(d.params) }
func (d Descriptor) Violations() []Violation { return cloneViolations(d.violations) }

func CodeOf(err error) (Code, bool) {
	fault, ok := find(err)
	if !ok {
		return "", false
	}
	return fault.code, true
}
func KindOf(err error) (Kind, bool) {
	fault, ok := find(err)
	if !ok {
		return "", false
	}
	return fault.kind, true
}
func IsCode(err error, code Code) bool {
	if validateCode(code) != nil {
		return false
	}
	return errors.Is(err, code)
}
func Describe(err error) (Descriptor, bool) {
	fault, ok := find(err)
	if !ok {
		return Descriptor{}, false
	}
	return Descriptor{kind: fault.kind, code: fault.code, message: fault.message, params: cloneParams(fault.params), violations: cloneViolations(fault.violations)}, true
}

func find(err error) (*Error, bool) {
	var fault *Error
	if !errors.As(err, &fault) || fault == nil {
		return nil, false
	}
	return fault, true
}
func validateKind(kind Kind) error {
	switch kind {
	case InvalidArgument, OutOfRange, NotFound, AlreadyExists, Conflict, FailedPrecondition, ResourceExhausted, Canceled, DeadlineExceeded, Unavailable, Internal:
		return nil
	default:
		return errors.New("unsupported fault kind")
	}
}
func validateCode(code Code) error {
	if !codePattern.MatchString(string(code)) {
		return errors.New("invalid fault code")
	}
	return nil
}
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

func containsUnsafePublicText(value string) bool {
	if !utf8.ValidString(value) {
		return true
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) || character == ' ' || character == ' ' {
			return true
		}
	}
	return false
}
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
func cloneParams(params []Param) []Param { return append([]Param(nil), params...) }
func cloneViolations(violations []Violation) []Violation {
	result := make([]Violation, len(violations))
	for index, violation := range violations {
		result[index] = violation
		result[index].params = cloneParams(violation.params)
	}
	return result
}
