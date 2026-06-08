package subscription

import (
	"strings"
	"unicode/utf8"
)

// utf8_utils.go consolidates the duplicated UTF-8 validate/repair and
// control-character logic that previously lived in node_parser.go
// (validateAndFixUTF8 / validateAndFixUTF8Bytes / sanitizeForDisplay) and
// meta.go (hasControlChars). Behavior is preserved exactly; callers are
// thin wrappers / aliases over these helpers.

// FixUTF8String validates and fixes invalid UTF-8 in a string.
// Returns the fixed string and true if valid, or the original string and
// false if the input cannot be repaired into valid UTF-8.
func FixUTF8String(s string) (string, bool) {
	if utf8.ValidString(s) {
		return s, true
	}
	fixed := strings.ToValidUTF8(s, "")
	if utf8.ValidString(fixed) {
		return fixed, true
	}
	return s, false
}

// FixUTF8Bytes validates and fixes invalid UTF-8 in bytes.
// Returns the fixed string and true if valid, or an empty string and false
// if the input cannot be repaired into valid UTF-8.
func FixUTF8Bytes(b []byte) (string, bool) {
	if utf8.Valid(b) {
		return string(b), true
	}
	fixed := strings.ToValidUTF8(string(b), "")
	if utf8.ValidString(fixed) {
		return fixed, true
	}
	return "", false
}

// HasControlChars reports whether s contains any C0 control character
// (U+0000..U+001F) or DEL (U+007F), excluding the common whitespace
// characters tab, newline, and carriage return.
func HasControlChars(s string) bool {
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0x7F {
			return true
		}
	}
	return false
}
