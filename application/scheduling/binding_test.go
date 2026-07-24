package scheduling

import "github.com/Capsule7446/healix-core/domain/parameter"

func literalBindingEqual(binding parameter.Binding, expected parameter.Value) bool {
	value, ok := binding.Literal()
	return ok && value.Equal(expected)
}
