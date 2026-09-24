// Сканер обходов типов internal/paths (SPEC 135 §8).
// Чистая логика без ввода-вывода — покрыта self-тестом scan_test.go.
// Ограничение: только stdlib и синтаксис go1.20 (win7-джоба гоняет
// `go get ./...` тулчейном go 1.20.14 по всем пакетам модуля).
package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strings"
)

// Finding — одна находка стража: позиция, правило, фрагмент исходника.
type Finding struct {
	Pos  token.Position
	Rule string
	Frag string
}

// allowPaths — allowlist по пути файла относительно корня модуля (прямые
// слэши). Каждая строка обоснована отдельно: это не бланковое исключение
// пакета, а конкретный файл, за которым видна ровно одна причина.
var allowFiles = map[string]string{
	"internal/platform/glstate.go":         "Mesa: единственная разрешённая запись в AppDir (opengl32.dll обязан лежать рядом с exe)",
	"internal/platform/glprobe_windows.go": "Mesa: пробник и установщик рендерера работают с тем же AppDir, что и glstate.go",
	"internal/paths/paths.go":              "ProbeWritable — честная проба записи, принимает произвольный dir, не привязана к DataDir/LogDir",
	"internal/paths/copytree.go":           "копировщик раскладки: читает AppDir, пишет в DataDir — легитимный источник для миграции",
	"internal/paths/migrate.go":            "миграция унаследованной раскладки: читает AppDir, пишет в DataDir",
}

// allowDirs — каталоги, исключённые целиком (не код приложения).
var allowDirPrefixes = []string{"tools/", "vendor/", ".claude/", "contract/"}

// isAllowed — путь файла (относительно корня модуля, ToSlash) в allowlist.
func isAllowed(relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	if strings.HasSuffix(relPath, "_test.go") {
		return true
	}
	if _, ok := allowFiles[relPath]; ok {
		return true
	}
	for _, prefix := range allowDirPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}
	return false
}

// writeFuncs — вызовы os./ioutil., которые считаются прямой записью/модификацией
// на диске. Индекс — позиции аргументов, исходный текст которых проверяется
// на признак AppDir (обычно первый аргумент — путь назначения).
var writeFuncs = map[string][]int{
	"WriteFile": {0},
	"Create":    {0},
	"OpenFile":  {0},
	"Mkdir":     {0},
	"MkdirAll":  {0},
	"Rename":    {0, 1},
	"Remove":    {0},
	"RemoveAll": {0},
	"Chmod":     {0},
	"Symlink":   {0, 1},
	"Link":      {0, 1},
	"Truncate":  {0},
}

// containsAppToken — грубая, регистронезависимая проверка «App» как
// отдельной лексемы: границы — не-буква/цифра ИЛИ смена регистра на
// заглавную (camelCase-граница: appDir, AppDir, layout.App). Так ловятся
// appDir/AppDir/.App, но не application/appendix (после "app" там строчная
// буква без границы). Ложные срабатывания допустимы (по ТЗ они редки) —
// важно не пропустить обход.
func containsAppToken(src string) bool {
	lower := strings.ToLower(src)
	idx := 0
	for {
		i := strings.Index(lower[idx:], "app")
		if i < 0 {
			return false
		}
		pos := idx + i
		before := byte(0)
		if pos > 0 {
			before = src[pos-1]
		}
		afterIsBoundary := pos+3 >= len(src)
		if !afterIsBoundary {
			after := src[pos+3]
			afterIsBoundary = !isAlnum(after) || (after >= 'A' && after <= 'Z')
		}
		if !isAlnum(before) && afterIsBoundary {
			return true
		}
		idx = pos + 3
		if idx >= len(lower) {
			return false
		}
	}
}

func isAlnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// exprSource — исходный текст выражения через go/printer (позиции узла могут
// не совпадать с обрезкой по строке файла, если выражение многострочное).
func exprSource(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

// ScanSource разбирает один файл и возвращает находки. relPath — путь
// относительно корня модуля (для allowlist и отчёта), src — содержимое файла.
func ScanSource(fset *token.FileSet, relPath string, src []byte) ([]Finding, error) {
	if isAllowed(relPath) {
		return nil, nil
	}

	f, err := parser.ParseFile(fset, relPath, src, 0)
	if err != nil {
		return nil, err
	}

	var findings []Finding

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		// Правило 1: конверсия-обход paths.DataDir(X) / paths.LogDir(X).
		if pkgIdent.Name == "paths" && (sel.Sel.Name == "DataDir" || sel.Sel.Name == "LogDir") {
			if len(call.Args) > 0 {
				frag := exprSource(fset, call.Args[0])
				if containsAppToken(frag) {
					findings = append(findings, Finding{
						Pos:  fset.Position(call.Pos()),
						Rule: "paths-conversion: конверсия AppDir в " + sel.Sel.Name + " через " + exprSource(fset, call),
						Frag: exprSource(fset, call),
					})
				}
			}
		}

		// Правило 2: прямая запись os./ioutil. от AppDir.
		if pkgIdent.Name == "os" || pkgIdent.Name == "ioutil" {
			if argPositions, known := writeFuncs[sel.Sel.Name]; known {
				for _, argIdx := range argPositions {
					if argIdx >= len(call.Args) {
						continue
					}
					frag := exprSource(fset, call.Args[argIdx])
					if containsAppToken(frag) {
						findings = append(findings, Finding{
							Pos:  fset.Position(call.Pos()),
							Rule: "paths-direct-write: " + pkgIdent.Name + "." + sel.Sel.Name + " от AppDir: " + exprSource(fset, call),
							Frag: exprSource(fset, call),
						})
						break
					}
				}
			}
		}

		return true
	})

	return findings, nil
}
