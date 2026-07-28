package parameter

import (
	"fmt"
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

func (c Constraint) Validate(value Value) error {
	switch c.Type {
	case Text, Number, Boolean, SingleSelect, MultiSelect:
	default:
		return fmt.Errorf("unsupported parameter constraint type %q", c.Type)
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Type() != c.Type {
		return fmt.Errorf("expected %s, got %s", c.Type, value.Type())
	}
	allowed := func(s string) bool {
		for _, option := range c.Options {
			if option == s {
				return true
			}
		}
		return false
	}
	if c.Type == SingleSelect && !allowed(value.SingleSelect()) {
		return fmt.Errorf("single-select value is not an option")
	}
	if c.Type == MultiSelect {
		for _, item := range value.MultiSelect() {
			if !allowed(item) {
				return fmt.Errorf("multi-select value is not an option")
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
		if strings.TrimSpace(b.parentName) == "" {
			return Value{}, fmt.Errorf("parent parameter reference name is required")
		}
		value, exists := parent[b.parentName]
		if !exists {
			return Value{}, fmt.Errorf("parent parameter %q is missing", b.parentName)
		}
		return value.Clone(), nil
	default:
		return Value{}, fmt.Errorf("unsupported parameter binding kind %q", b.kind)
	}
}
