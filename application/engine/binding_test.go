package engine

import "github.com/Capsule7446/healix-core/domain/parameter"

func literalBindingEqual(binding parameter.Binding, expected parameter.Value) bool {
	value, ok := binding.Literal()
	return ok && value.Equal(expected)
}

func literalNumber(binding parameter.Binding) string {
	value, ok := binding.Literal()
	if !ok {
		return ""
	}
	return value.Number()
}

func literalMultiSelect(binding parameter.Binding) []string {
	value, ok := binding.Literal()
	if !ok {
		return nil
	}
	return value.MultiSelect()
}
