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
	CodeExpansionTooLarge fault.Code = "INTERPOLATION_EXPANSION_TOO_LARGE"
)

// MaxExpansionBytes bounds one Expand result.
//
// Interpolation is the one place in Core where a small input legitimately
// produces a larger output, and the multiplier is attacker-chosen: a template
// is N occurrences of ${x} and a variable is M bytes, so the result is N*M
// while neither input on its own looks unusual. A 64 KiB template against a
// 64 KiB value reaches exactly 1 GiB. Nothing upstream can catch that — the
// individual sizes are fine and only their product is not — so the ceiling
// has to live here, and it has to be enforced before the bytes are written
// rather than after, or the refusal costs the same memory as the attack.
const MaxExpansionBytes = 1 << 20

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
	// Track what is left of the budget rather than what has been written. The
	// natural spelling — written+len(chunk) > MaxExpansionBytes — can overflow
	// int on adversarial sizes and wrap negative, which reads as "still under
	// budget" and admits exactly the write the check exists to stop.
	remaining := MaxExpansionBytes
	write := func(chunk string) bool {
		if len(chunk) > remaining {
			return false
		}
		remaining -= len(chunk)
		out.WriteString(chunk)
		return true
	}
	rest := value
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			if !write(rest) {
				return "", expansionTooLargeFault()
			}
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
		if !write(rest[:start]) || !write(resolved) {
			return "", expansionTooLargeFault()
		}
		rest = rest[start+end+1:]
	}
}

// expansionTooLargeFault keeps the template, the variable name, and the
// resolved value out of the public text. All three are caller input and the
// last one is the most likely to carry a token or a URL.
func expansionTooLargeFault() error {
	return interpolationFault(fault.ResourceExhausted, CodeExpansionTooLarge, "expanded value exceeds the size limit")
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
