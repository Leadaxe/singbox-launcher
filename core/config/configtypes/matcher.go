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

	// Negation regex: !/regex/i (case-insensitive) или !/regex/ (case-sensitive).
	// Флаг i опционален: раньше «/🔥/» без i молча проваливался в literal-ветку
	// («значение == "/🔥/"»), селектор собирался пустым и выпадал из конфига.
	if strings.HasPrefix(pattern, "!/") {
		if regexStr, ci, ok := trimRegexPattern(strings.TrimPrefix(pattern, "!")); ok {
			re, err := compileFilterRegex(regexStr, ci)
			if err != nil {
				debuglog.WarnLog("Parser: Invalid regex pattern %s: %v", pattern, err)
				return false
			}
			return !re.MatchString(value)
		}
	}

	// Regex: /regex/i (case-insensitive) или /regex/ (case-sensitive).
	if strings.HasPrefix(pattern, "/") {
		if regexStr, ci, ok := trimRegexPattern(pattern); ok {
			re, err := compileFilterRegex(regexStr, ci)
			if err != nil {
				debuglog.WarnLog("Parser: Invalid regex pattern %s: %v", pattern, err)
				return false
			}
			return re.MatchString(value)
		}
	}

	// Literal match (case-sensitive)
	return value == pattern
}

// trimRegexPattern распознаёт /regex/i и /regex/: возвращает тело регэкспа,
// признак case-insensitive и ok=false, если строка не является /…/-формой
// (тогда caller падает в literal-ветку).
func trimRegexPattern(pattern string) (regexStr string, caseInsensitive bool, ok bool) {
	if !strings.HasPrefix(pattern, "/") {
		return "", false, false
	}
	if strings.HasSuffix(pattern, "/i") && len(pattern) > len("//i") {
		return pattern[1 : len(pattern)-2], true, true
	}
	if strings.HasSuffix(pattern, "/") && len(pattern) > len("//") {
		return pattern[1 : len(pattern)-1], false, true
	}
	return "", false, false
}

func compileFilterRegex(regexStr string, caseInsensitive bool) (*regexp.Regexp, error) {
	if caseInsensitive {
		return regexp.Compile("(?i)" + regexStr)
	}
	return regexp.Compile(regexStr)
}

// PatternCompiles сообщает, разбирается ли паттерн фильтра.
//
// Нужна вызывающим, которые обязаны отличить «не совпало» от «выражение
// некорректно»: MatchesPattern возвращает false в обоих случаях, и для
// фильтра Направления это разница между «узлов нет» и «фильтра нет»
// (SPEC 104 §3.5).
//
// Литеральные формы всегда корректны — компилировать в них нечего.
func PatternCompiles(pattern string) bool {
	body := strings.TrimPrefix(pattern, "!")
	regexStr, ci, ok := trimRegexPattern(body)
	if !ok {
		return true // литерал или !литерал
	}
	_, err := compileFilterRegex(regexStr, ci)
	return err == nil
}
