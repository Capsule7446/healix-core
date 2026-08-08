package parameter

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Type 标识参数值的封闭类型集合。
type Type string

const (
	// Text 表示文本参数值。
	Text Type = "TEXT"
	// Number 表示规范十进制参数值。
	Number Type = "NUMBER"
	// Boolean 表示布尔参数值。
	Boolean Type = "BOOLEAN"
	// SingleSelect 表示单选参数值。
	SingleSelect Type = "SINGLE_SELECT"
	// MultiSelect 表示多选参数值。
	MultiSelect Type = "MULTI_SELECT"
)

// Value 是类型封闭的参数值；只有本包构造器能创建通过 Validate 的有效值。
type Value struct {
	kind    Type
	text    string
	boolean bool
	multi   []string
}

// OptionalValue 表示可缺省的参数值，并区分缺省与零值内容。
type OptionalValue struct {
	present bool
	value   Value
}

// TextValue 创建文本参数值。
func TextValue(value string) Value { return Value{kind: Text, text: value} }

// BooleanValue 创建布尔参数值。
func BooleanValue(value bool) Value {
	return Value{kind: Boolean, boolean: value}
}

// SingleSelectValue 创建单选参数值。
func SingleSelectValue(value string) Value {
	return Value{kind: SingleSelect, text: value}
}

// MultiSelectValue 创建多选参数值并复制输入切片。
func MultiSelectValue(value []string) Value {
	return Value{kind: MultiSelect, multi: append([]string(nil), value...)}
}

// NewNumberValue 解析并创建规范十进制参数值；输入无效或规范结果超限时返回安全错误。
func NewNumberValue(value string) (Value, error) {
	canonical, err := canonicalDecimal(value)
	if err != nil {
		return Value{}, wrapValueInvalidError(err)
	}
	return Value{kind: Number, text: canonical}, nil
}

// PresentValue 创建携带值深拷贝的已存在 OptionalValue。
func PresentValue(value Value) OptionalValue {
	return OptionalValue{present: true, value: value.Clone()}
}

// IsPresent 判断 OptionalValue 是否携带值。
func (v OptionalValue) IsPresent() bool { return v.present }

// Value 返回 OptionalValue 中的值深拷贝及存在标志。
func (v OptionalValue) Value() (Value, bool) { return v.value.Clone(), v.present }

// Type 返回参数值类型；零值 Value 返回空类型。
func (v Value) Type() Type { return v.kind }

// Text 返回文本或选择值的字符串载荷。
func (v Value) Text() string { return v.text }

// Number 返回规范十进制字符串载荷。
func (v Value) Number() string { return v.text }

// Boolean 返回布尔载荷；非布尔零值返回 false。
func (v Value) Boolean() bool { return v.boolean }

// SingleSelect 返回单选字符串载荷。
func (v Value) SingleSelect() string { return v.text }

// MultiSelect 返回多选载荷切片的副本。
func (v Value) MultiSelect() []string { return append([]string(nil), v.multi...) }

// MultiSelectMetrics 返回多选项数量、总字节数和最大项字节数，不复制或暴露内部切片；非多选值返回 ok=false。
func (v Value) MultiSelectMetrics() (count, totalBytes, maxItemBytes int, ok bool) {
	if v.kind != MultiSelect {
		return 0, 0, 0, false
	}
	for _, item := range v.multi {
		if len(item) > maxItemBytes {
			maxItemBytes = len(item)
		}
		if len(item) > int(^uint(0)>>1)-totalBytes {
			return len(v.multi), 0, maxItemBytes, false
		}
		totalBytes += len(item)
	}
	return len(v.multi), totalBytes, maxItemBytes, true
}

// Clone 返回参数值深拷贝；多选切片与源值不共享。
func (v Value) Clone() Value { v.multi = append([]string(nil), v.multi...); return v }

// Equal 比较两个参数值的类型、标量载荷、布尔值和多选项顺序是否完全相等。
func (v Value) Equal(other Value) bool {
	if v.kind != other.kind || v.text != other.text || v.boolean != other.boolean || len(v.multi) != len(other.multi) {
		return false
	}
	for i := range v.multi {
		if v.multi[i] != other.multi[i] {
			return false
		}
	}
	return true
}

// Validate 为每种拒绝返回一个安全错误码，不将值或类型写入公开文本；调用方可通过 Type
// 读取自身类型。
func (v Value) Validate() error {
	switch v.kind {
	case Text, SingleSelect:
		if len(v.text) > MaxValueStringBytes {
			return valueInvalidError()
		}
		return nil
	case Number:
		canonical, err := canonicalDecimal(v.text)
		if err != nil || canonical != v.text {
			return valueInvalidError()
		}
		return nil
	case Boolean:
		return nil
	case MultiSelect:
		for _, item := range v.multi {
			if len(item) > MaxValueStringBytes {
				return valueInvalidError()
			}
		}
		return nil
	default:
		return valueInvalidError()
	}
}

// MaxValueStringBytes 限制文本、选择值和规范数字字符串的字节长度。
const MaxValueStringBytes = 64 * 1024

// decimalPattern 匹配带可选符号、小数和指数的十进制输入。
var decimalPattern = regexp.MustCompile(`^([+-]?)([0-9]+)(?:\.([0-9]*))?(?:[eE]([+-]?[0-9]+))?$`)

// canonicalDecimal 将十进制输入规范化为无指数、无多余零的字符串，并执行长度/指数上限检查。
func canonicalDecimal(input string) (string, error) {
	if len(input) > MaxValueStringBytes {
		return "", errors.New("NUMBER exceeds maximum size")
	}
	match := decimalPattern.FindStringSubmatch(input)
	if match == nil {
		// 即使私有 cause 也不携带被拒输入，因为宿主可能记录 cause，而参数值不得进入日志。
		return "", errors.New("NUMBER format is invalid")
	}
	exponent := 0
	if match[4] != "" {
		parsed, err := strconv.Atoi(match[4])
		if err != nil || parsed > 100000 || parsed < -100000 {
			return "", errors.New("NUMBER exponent is out of range")
		}
		exponent = parsed
	}
	digits := strings.TrimLeft(match[2]+match[3], "0")
	if digits == "" {
		return "0", nil
	}
	scale := len(match[3]) - exponent
	// 负号最终会占用一个字节，因此必须计入规范结果的长度预算。
	budget := MaxValueStringBytes
	if match[1] == "-" {
		budget--
	}
	var result string
	if scale <= 0 {
		if len(digits)-scale > budget {
			return "", errors.New("NUMBER canonical form exceeds maximum size")
		}
		result = digits + strings.Repeat("0", -scale)
	} else if scale >= len(digits) {
		if scale+2 > budget {
			return "", errors.New("NUMBER canonical form exceeds maximum size")
		}
		result = "0." + strings.Repeat("0", scale-len(digits)) + digits
	} else {
		result = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	}
	if strings.Contains(result, ".") {
		result = strings.TrimRight(strings.TrimRight(result, "0"), ".")
	}
	if result == "" {
		result = "0"
	}
	if match[1] == "-" && result != "0" {
		result = "-" + result
	}
	// 构造器不得返回会被自身 Validate 拒绝的结果。
	if len(result) > MaxValueStringBytes {
		return "", errors.New("NUMBER canonical form exceeds maximum size")
	}
	return result, nil
}
