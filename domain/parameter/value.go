package parameter

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Type string

const (
	Text         Type = "TEXT"
	Number       Type = "NUMBER"
	Boolean      Type = "BOOLEAN"
	SingleSelect Type = "SINGLE_SELECT"
	MultiSelect  Type = "MULTI_SELECT"
)

// Value is a closed typed value. Only its constructors can create a valid value.
type Value struct {
	kind    Type
	text    string
	boolean bool
	multi   []string
}

type OptionalValue struct {
	present bool
	value   Value
}

func TextValue(value string) Value { return Value{kind: Text, text: value} }
func BooleanValue(value bool) Value {
	return Value{kind: Boolean, boolean: value}
}
func SingleSelectValue(value string) Value {
	return Value{kind: SingleSelect, text: value}
}
func MultiSelectValue(value []string) Value {
	return Value{kind: MultiSelect, multi: append([]string(nil), value...)}
}
func NewNumberValue(value string) (Value, error) {
	canonical, err := canonicalDecimal(value)
	if err != nil {
		return Value{}, err
	}
	return Value{kind: Number, text: canonical}, nil
}
func PresentValue(value Value) OptionalValue {
	return OptionalValue{present: true, value: value.Clone()}
}
func (v OptionalValue) IsPresent() bool      { return v.present }
func (v OptionalValue) Value() (Value, bool) { return v.value.Clone(), v.present }
func (v Value) Type() Type                   { return v.kind }
func (v Value) Text() string                 { return v.text }
func (v Value) Number() string               { return v.text }
func (v Value) Boolean() bool                { return v.boolean }
func (v Value) SingleSelect() string         { return v.text }
func (v Value) MultiSelect() []string        { return append([]string(nil), v.multi...) }

// MultiSelectMetrics reports immutable payload budgets without copying or exposing aliases.
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
func (v Value) Clone() Value { v.multi = append([]string(nil), v.multi...); return v }
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
func (v Value) Validate() error {
	switch v.kind {
	case Text, SingleSelect:
		if len(v.text) > MaxValueStringBytes {
			return fmt.Errorf("%s value exceeds maximum size", v.kind)
		}
		return nil
	case Number:
		canonical, err := canonicalDecimal(v.text)
		if err != nil || canonical != v.text {
			return errors.New("NUMBER value is not canonical")
		}
		return nil
	case Boolean:
		return nil
	case MultiSelect:
		for _, item := range v.multi {
			if len(item) > MaxValueStringBytes {
				return errors.New("MULTI_SELECT item exceeds maximum size")
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported parameter value type %q", v.kind)
	}
}

const MaxValueStringBytes = 64 * 1024

var decimalPattern = regexp.MustCompile(`^([+-]?)([0-9]+)(?:\.([0-9]*))?(?:[eE]([+-]?[0-9]+))?$`)

func canonicalDecimal(input string) (string, error) {
	if len(input) > MaxValueStringBytes {
		return "", errors.New("NUMBER exceeds maximum size")
	}
	match := decimalPattern.FindStringSubmatch(input)
	if match == nil {
		return "", fmt.Errorf("invalid NUMBER %q", input)
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
	var result string
	if scale <= 0 {
		if len(digits)-scale > MaxValueStringBytes {
			return "", errors.New("NUMBER canonical form exceeds maximum size")
		}
		result = digits + strings.Repeat("0", -scale)
	} else if scale >= len(digits) {
		if scale+2 > MaxValueStringBytes {
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
	return result, nil
}
