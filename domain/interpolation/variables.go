// 包插值拥有由执行和工作空间有界上下文共享的变量表达式语法。
package interpolation

import (
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

type Resolver interface {
	Variable(name string) (string, bool)
}

const (
	CodeResolverRequired  fault.Code = "INTERPOLATION_RESOLVER_REQUIRED"
	CodeExpressionInvalid fault.Code = "INTERPOLATION_EXPRESSION_INVALID"
	CodeVariableUndefined fault.Code = "INTERPOLATION_VARIABLE_UNDEFINED"
)

func interpolationFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

func Expand(value string, resolver Resolver) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	if resolver == nil {
		return "", interpolationFault(fault.FailedPrecondition, CodeResolverRequired, "variable resolver is required")
	}
	var out strings.Builder
	rest := value
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			out.WriteString(rest)
			return out.String(), nil
		}
		end := strings.Index(rest[start:], "}")
		if end < 0 {
			return "", interpolationFault(fault.InvalidArgument, CodeExpressionInvalid, "variable expression is invalid")
		}
		name := rest[start+2 : start+end]
		if err := validateName(name, value); err != nil {
			return "", err
		}
		resolved, ok := resolver.Variable(name)
		if !ok {
			return "", interpolationFault(fault.NotFound, CodeVariableUndefined, "referenced variable is not defined")
		}
		out.WriteString(rest[:start])
		out.WriteString(resolved)
		rest = rest[start+end+1:]
	}
}

// Names 解析与 Expand 相同的语法，但不解析值。
func Names(value string) ([]string, error) {
	if !strings.Contains(value, "${") {
		return nil, nil
	}
	var names []string
	rest := value
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			return names, nil
		}
		end := strings.Index(rest[start:], "}")
		if end < 0 {
			return nil, interpolationFault(fault.InvalidArgument, CodeExpressionInvalid, "variable expression is invalid")
		}
		name := rest[start+2 : start+end]
		if err := validateName(name, value); err != nil {
			return nil, err
		}
		names = append(names, name)
		rest = rest[start+end+1:]
	}
}

func validateName(name, expression string) error {
	if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\t\r\n {}$") {
		return interpolationFault(fault.InvalidArgument, CodeExpressionInvalid, "variable expression is invalid")
	}
	return nil
}
