package parameter

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxNameBytes is the maximum encoded UTF-8 byte length accepted for a name.
const MaxNameBytes = 64 * 1024

// ValidateName verifies that name is nonblank after trimming, valid UTF-8,
// contains no Unicode control or format runes, and does not exceed MaxNameBytes.
// It validates without normalizing or trimming the returned name.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if !utf8.ValidString(name) {
		return errors.New("name must be valid UTF-8")
	}
	if len(name) > MaxNameBytes {
		return errors.New("name exceeds byte limit")
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return errors.New("name contains control or format character")
		}
	}
	return nil
}
