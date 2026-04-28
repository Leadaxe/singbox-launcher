package build

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Канонические утилиты форматирования config.json — отделены от UI-слоя
// чтобы их мог использовать `core/build.BuildConfig` без обратного импорта
// в `ui/wizard/business`.
//
// Исторически жили в `ui/wizard/business/{formatting.go,create_config.go}`;
// перемещены в core (SPEC 045 phase 5.3 порт). Wizard-side всё ещё имеет
// обёртки-алиасы на эти функции для back-compat существующих тестов и
// callsites — обёртки удаляются, когда wizard-слой целиком переключится
// на BuildConfig (фаза 5.2 SPEC 045).
//
// Все функции — pure (без I/O, без shared state, без глобальных мьютексов).

// IndentBase — базовый отступ JSON-вывода (2 пробела).
const IndentBase = "  "

// Indent возвращает строку отступа уровня level (level≤0 → пусто).
//
//	Indent(0) = ""
//	Indent(1) = "  " (2 пробела)
//	Indent(2) = "    " (4 пробела)
func Indent(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat(IndentBase, level)
}

// IndentMultiline ставит indent перед каждой строкой text.
// Пустой text → возвращает indent (одна пустая строка с отступом).
func IndentMultiline(text, indent string) string {
	if text == "" {
		return indent
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// FormatSectionJSON делает pretty-print JSON-секции с указанным уровнем
// исходного отступа (indentLevel — количество пробелов перед открывающей
// скобкой; внутри блока всегда +IndentBase).
//
// Соответствует поведению старого `ui/wizard/business.FormatSectionJSON` —
// порядок ключей сохраняется как у `json.Indent`.
func FormatSectionJSON(raw json.RawMessage, indentLevel int) (string, error) {
	var buf bytes.Buffer
	prefix := strings.Repeat(" ", indentLevel)
	if err := json.Indent(&buf, raw, prefix, IndentBase); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatCompactJSON убирает межтокенный whitespace из JSON.
// (Старая `formatCompactJSON` в wizard принимала также indent-параметр, но
// не использовала его — здесь убран.)
func FormatCompactJSON(raw json.RawMessage) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", err
	}
	return buf.String(), nil
}
