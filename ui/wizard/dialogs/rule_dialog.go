// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл rule_dialog.go содержит утилиты для работы с правилами в диалогах:
//   - ExtractStringArray - извлечение массива строк из interface{} (поддержка []interface{} и []string)
//   - ParseLines - парсинг многострочного текста в массив строк (разделение по переносу строки)
//   - Константы типов правил (RuleTypeIP, RuleTypeDomain)
//
// Эти утилиты используются в add_rule_dialog.go для обработки ввода пользователя
// (например, ввод доменов или IP-адресов в многострочном текстовом поле).
//
// Утилиты для диалогов - это вспомогательные функции, отдельные от основной логики диалогов.
//
// Используется в:
//   - dialogs/add_rule_dialog.go - ExtractStringArray и ParseLines вызываются при сохранении правила
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
