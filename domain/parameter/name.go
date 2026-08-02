package parameter

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxNameBytes is the maximum encoded UTF-8 byte length accepted for a name.
const MaxNameBytes = 64 * 1024

// ValidateName verifies that name is nonblank after trimming, valid UTF-8,
// contains no Unicode control or format runes, and does not exceed MaxNameBytes.
// It validates without normalizing or trimming the returned name.
// The rejected name never enters the fault: a name is caller input, and the
// caller already knows which name it supplied.
func ValidateName(name string) error {
	if !isValidName(name) {
		return nameInvalidError()
	}
	return nil
}

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
