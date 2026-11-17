package jsonfmt

import "strings"

// StripComments снимает комментарии с JSON-текста (контракт 1.1.172,
// BACKUP.md §2 `origin`): `//` и `#` до конца строки, `/* … */` — вне
// строковых литералов. Внутри строки (`"…"`, с учётом `\`-экранирования)
// ничего комментарием не считается: `"path": "/a//b#c"` остаётся как есть.
//
// Комментарий заменяется одним пробелом, перевод строки после строчного
// комментария сохраняется — соседние лексемы не склеиваются, номера строк
// в ошибке разбора остаются прежними. Незакрытый `/*` съедает текст до
// конца. Текст без комментариев возвращается тем же.
//
// Одна функция на все места, которые разбирают JSON-исходник узла: вид
// исходника, авторское тело, материализация, Regen, ключ дедупа импорта.
// Разные реализации разошлись бы на кавычке внутри комментария.
func StripComments(s string) string {
	if !strings.ContainsAny(s, "/#") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			b.WriteByte(c)
			switch c {
			case '\\':
				if i+1 < len(s) {
					i++
					b.WriteByte(s[i])
				}
			case '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			b.WriteByte(c)
		case c == '#' || (c == '/' && i+1 < len(s) && s[i+1] == '/'):
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
			if i < len(s) {
				b.WriteByte('\n')
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				i = len(s)
			} else {
				i += 2 + end + 1
			}
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
