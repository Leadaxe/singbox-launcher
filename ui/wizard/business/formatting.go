package business

import "strings"

// Константы для форматирования конфигурации
const (
	// IndentBase - базовый отступ (2 пробела)
	IndentBase = "  "
)

// Indent возвращает строку отступа для указанного уровня вложенности.
// Использует IndentBase как базовую единицу.
//
// Примеры:
//
//	Indent(0) = ""
//	Indent(1) = "  " (2 пробела)
//	Indent(2) = "    " (4 пробела)
//	Indent(3) = "      " (6 пробелов)
func Indent(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat(IndentBase, level)
}
