package dialogs

import (
	"strings"
)

const (
	RuleTypeIP     = "IP Addresses (CIDR)"
	RuleTypeDomain = "Domains/URLs"
)

// ExtractStringArray extracts []string from interface{} (supports []interface{} and []string).
func ExtractStringArray(val interface{}) []string {
	if arr, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	if arr, ok := val.([]string); ok {
		return arr
	}
	return nil
}

// ParseLines parses multiline text, removing empty lines.
func ParseLines(text string, preserveOriginal bool) []string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if preserveOriginal {
				result = append(result, line) // Preserve original (with spaces)
			} else {
				result = append(result, trimmed) // Preserve trimmed version
			}
		}
	}
	return result
}
