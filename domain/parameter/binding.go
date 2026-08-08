package parameter

import (
	"strings"
)

// BindingKind 标识参数绑定是字面量还是父作用域引用。
type BindingKind string

const (
	// LiteralBindingKind 表示绑定直接携带参数值。
	LiteralBindingKind BindingKind = "LITERAL"
	// ParentReferenceBindingKind 表示绑定引用父作用域参数名。
	ParentReferenceBindingKind BindingKind = "PARENT_REFERENCE"
)

// Constraint 描述参数值必须满足的类型和可选项约束。
type Constraint struct {
	Type    Type
	Options []string
}

// Validate 校验值的类型、选项和自身合法性，不回显调用方提供的类型或选项；值自身已分类
// 的错误保持原样传播。
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

// Binding 保存字面量值或父作用域参数名；内部值通过 Clone 隔离所有权。
type Binding struct {
	kind       BindingKind
	literal    Value
	parentName string
}

// LiteralBinding 创建携带值深拷贝的字面量绑定。
func LiteralBinding(value Value) Binding {
	return Binding{kind: LiteralBindingKind, literal: value.Clone()}
}

// ParentReferenceBinding 创建引用父作用域参数名的绑定。
func ParentReferenceBinding(name string) Binding {
	return Binding{kind: ParentReferenceBindingKind, parentName: name}
}

// Kind 返回绑定种类。
func (b Binding) Kind() BindingKind { return b.kind }

// Literal 返回字面量值的深拷贝及其种类标志。
func (b Binding) Literal() (Value, bool) { return b.literal.Clone(), b.kind == LiteralBindingKind }

// ParentName 返回父作用域参数名及其种类标志。
func (b Binding) ParentName() (string, bool) {
	return b.parentName, b.kind == ParentReferenceBindingKind
}

// Clone 返回绑定的深拷贝，避免共享字面量内部引用。
func (b Binding) Clone() Binding { b.literal = b.literal.Clone(); return b }

// Resolve 在父作用域中解析绑定并返回值深拷贝；缺少父值或绑定无效时返回分类错误。
func (b Binding) Resolve(parent map[string]Value) (Value, error) {
	switch b.kind {
	case LiteralBindingKind:
		if err := b.literal.Validate(); err != nil {
			return Value{}, err
		}
		return b.literal.Clone(), nil
	case ParentReferenceBindingKind:
		// 空父参数名表示绑定本身格式错误；修复方式是重新构造绑定，而不是补充作用域值。
		if strings.TrimSpace(b.parentName) == "" {
			return Value{}, bindingInvalidError()
		}
		// 只有此处绑定本身合法而父作用域确实缺少值。父参数名和作用域映射均由调用方提供，
		// 因此不会回显到错误文本。
		value, exists := parent[b.parentName]
		if !exists {
			return Value{}, bindingUnresolvableError()
		}
		return value.Clone(), nil
	default:
		// 零值或未知种类表示调用方没有构造出可用绑定。
		return Value{}, bindingInvalidError()
	}
}
