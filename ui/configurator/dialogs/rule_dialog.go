// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл rule_dialog.go — утилита полей окна правила: ParseLines разбирает
// многострочный текст поля в список строк.
package dialogs

import "strings"

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
