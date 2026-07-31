package parameter

import (
	"strings"
)

type BindingKind string

const (
	LiteralBindingKind         BindingKind = "LITERAL"
	ParentReferenceBindingKind BindingKind = "PARENT_REFERENCE"
)

type Constraint struct {
	Type    Type
	Options []string
}

// Validate rejects without echoing the constraint type, the value type, or any
// option: the caller supplied all three. A value that fails its own validation
// keeps that value's code rather than being re-wrapped, so the host is not forced
// to unwrap to learn the value itself was malformed.
func (c Constraint) Validate(value Value) error {
	switch c.Type {
	case Text, Number, Boolean, SingleSelect, MultiSelect:
	default:
		return constraintUnsatisfiedError()
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Type() != c.Type {
		return constraintUnsatisfiedError()
	}
	allowed := func(candidate string) bool {
		for _, option := range c.Options {
			if option == candidate {
				return true
			}
		}
		return false
	}
	if c.Type == SingleSelect && !allowed(value.SingleSelect()) {
		return constraintUnsatisfiedError()
	}
	if c.Type == MultiSelect {
		for _, item := range value.MultiSelect() {
			if !allowed(item) {
				return constraintUnsatisfiedError()
			}
		}
	}
	return nil
}

type Binding struct {
	kind       BindingKind
	literal    Value
	parentName string
}

func LiteralBinding(value Value) Binding {
	return Binding{kind: LiteralBindingKind, literal: value.Clone()}
}
func ParentReferenceBinding(name string) Binding {
	return Binding{kind: ParentReferenceBindingKind, parentName: name}
}
func (b Binding) Kind() BindingKind      { return b.kind }
func (b Binding) Literal() (Value, bool) { return b.literal.Clone(), b.kind == LiteralBindingKind }
func (b Binding) ParentName() (string, bool) {
	return b.parentName, b.kind == ParentReferenceBindingKind
}
func (b Binding) Clone() Binding { b.literal = b.literal.Clone(); return b }
func (b Binding) Resolve(parent map[string]Value) (Value, error) {
	switch b.kind {
	case LiteralBindingKind:
		if err := b.literal.Validate(); err != nil {
			return Value{}, err
		}
		return b.literal.Clone(), nil
	case ParentReferenceBindingKind:
		// The parent parameter name is never echoed: it is caller-declared, and the
		// parent scope map is the caller's too, so it can locate the gap itself.
		if strings.TrimSpace(b.parentName) == "" {
			return Value{}, bindingUnresolvableError()
		}
		value, exists := parent[b.parentName]
		if !exists {
			return Value{}, bindingUnresolvableError()
		}
		return value.Clone(), nil
	default:
		return Value{}, bindingUnresolvableError()
	}
}
