// 包插值拥有由执行和工作空间有界上下文共享的变量表达式语法。
package interpolation

import (
	"fmt"
	"strings"
)

type Resolver interface {
	Variable(name string) (string, bool)
}

func Expand(value string, resolver Resolver) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	if resolver == nil {
		return "", fmt.Errorf("variable resolver is required for %q", value)
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
			return "", fmt.Errorf("unterminated ${ in %q", value)
		}
		name := rest[start+2 : start+end]
		if err := validateName(name, value); err != nil {
			return "", err
		}
		resolved, ok := resolver.Variable(name)
		if !ok {
			return "", fmt.Errorf("undefined variable %q referenced in %q", name, value)
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
			return nil, fmt.Errorf("unterminated ${ in %q", value)
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
	if name == "" {
		return fmt.Errorf("empty variable reference ${} in %q", expression)
	}
	if strings.TrimSpace(name) != name || strings.ContainsAny(name, "\t\r\n {}$") {
		return fmt.Errorf("invalid variable name %q referenced in %q", name, expression)
	}
	return nil
}
