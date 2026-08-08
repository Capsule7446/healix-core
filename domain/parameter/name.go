package parameter

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxNameBytes 是参数名称允许的最大 UTF-8 编码字节长度。
const MaxNameBytes = 64 * 1024

// ValidateName 校验名称非空、UTF-8 有效、不含 Unicode 控制/格式字符且不超过 MaxNameBytes；
// 校验不会规范化或修剪名称，被拒名称也不会进入错误。
func ValidateName(name string) error {
	if !isValidName(name) {
		return nameInvalidError()
	}
	return nil
}

// isValidName 执行参数名称的具体空白、编码、长度和字符类别检查。
func isValidName(name string) bool {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || len(name) > MaxNameBytes {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
	}
	return true
}
