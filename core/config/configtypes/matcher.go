// Package configtypes: matcher.go — shared pattern-matching for node filters.
//
// MatchesPattern is the single implementation used by both selector-scope
// filters (core/config/outbound_filter.go) and subscription skip-filters
// (core/config/subscription/node_parser.go). Both packages import
// configtypes as a leaf package, so hosting the helper here keeps a single
// source of truth and the exact matching semantics:
//   - literal           → value == pattern
//   - !literal          → value != literal
//   - /regex/i          → case-insensitive regex match
//   - !/regex/i         → case-insensitive regex non-match
//
// Invalid regex patterns are logged via debuglog.WarnLog and treated as
// non-matching (return false).
package configtypes

import (
	"regexp"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// MatchesPattern matches value against pattern: literal, !literal, /regex/i,
// !/regex/i. Regex forms are case-insensitive; literal forms are
// case-sensitive. An invalid regex pattern is logged and treated as a
// non-match (false).
func MatchesPattern(value, pattern string) bool {
	// Negation literal: !literal
	if strings.HasPrefix(pattern, "!") && !strings.HasPrefix(pattern, "!/") {
		literal := strings.TrimPrefix(pattern, "!")
		return value != literal
	}

	// Negation regex: !/regex/i
	if strings.HasPrefix(pattern, "!/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "!/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			debuglog.WarnLog("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return !re.MatchString(value)
	}

	// Regex: /regex/i
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			debuglog.WarnLog("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return re.MatchString(value)
	}

	// Literal match (case-sensitive)
	return value == pattern
}
