// 包插值拥有由执行和工作空间有界上下文共享的变量表达式语法。
package interpolation

import (
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// Resolver 提供变量名到字符串值的解析端口。
type Resolver interface {
	// Variable 按名称查找变量值；不存在时返回 false。
	Variable(name string) (string, bool)
}

const (
	// CodeResolverRequired 表示展开包含变量表达式但未提供解析器。
	CodeResolverRequired fault.Code = "INTERPOLATION_RESOLVER_REQUIRED"
	// CodeExpressionInvalid 表示变量表达式语法无效。
	CodeExpressionInvalid fault.Code = "INTERPOLATION_EXPRESSION_INVALID"
	// CodeVariableUndefined 表示引用的变量未定义。
	CodeVariableUndefined fault.Code = "INTERPOLATION_VARIABLE_UNDEFINED"
	// CodeExpansionTooLarge 表示展开结果超过大小上限。
	CodeExpansionTooLarge fault.Code = "INTERPOLATION_EXPANSION_TOO_LARGE"
)

// MaxExpansionBytes 限制一次 Expand 的结果字节数，并在写入前执行检查。
const MaxExpansionBytes = 1 << 20

// interpolationFault 构造插值领域错误；构造失败表示程序契约错误并触发 panic。
func interpolationFault(kind fault.Kind, code fault.Code, message string) error {
	err, constructionErr := fault.New(kind, code, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// Expand 解析并替换变量表达式，限制结果大小且不在公开错误中回显输入内容。
func Expand(value string, resolver Resolver) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	if resolver == nil {
		return "", interpolationFault(fault.FailedPrecondition, CodeResolverRequired, "variable resolver is required")
	}
	var out strings.Builder
	// 跟踪剩余预算而不是已写入长度，避免 written+len(chunk) 在极端输入下发生整数溢出，
	// 使检查错误地放行本应拒绝的写入。
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

// expansionTooLargeFault 构造展开超限错误，不把模板、变量名或解析值写入公开文本。
func expansionTooLargeFault() error {
	return interpolationFault(fault.ResourceExhausted, CodeExpansionTooLarge, "expanded value exceeds the size limit")
}

// Names 解析与 Expand 相同的变量语法，但不解析变量值。
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

// validateName 校验变量名的非空、无空白和无嵌套表达式约束。
func validateName(name, expression string) error {
	if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\t\r\n {}$") {
		return interpolationFault(fault.InvalidArgument, CodeExpressionInvalid, "variable expression is invalid")
	}
	return nil
}
