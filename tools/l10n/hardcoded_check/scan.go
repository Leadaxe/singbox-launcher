// Сканер хардкод-литералов в display-позициях (SPEC 111, этап 5).
// Чистая логика без ввода-вывода — покрыта self-тестом scan_test.go.
// Ограничение: только stdlib и синтаксис go1.20 (win7-джоба гоняет
// `go get ./...` по всем пакетам модуля).
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"unicode"
)

// Positions: имя функции → позиции display-аргументов (display_positions.json).
type Positions map[string][]int

// Site — один хардкод-литерал в display-позиции.
type Site struct {
	File string
	Line int
	Text string
}

// ScanFile находит строковые литералы в display-позициях, не обёрнутые в
// locale.* и не исключённые комментарием l10n-exempt (на строке литерала
// или строкой выше).
//
// Литералы без единой буквы (глифы "?", "→", "···", пустые строки) не
// считаются: символам перевод не нужен. Значение из locale.* легально:
// NewLabel(locale.T("…")) — литерал аргумент локализатора, не виджета.
//
// Отличие от LxBox: в Go нет тернарного оператора, ветки if/switch
// присваивают переменные, а поток переменных сканер не отслеживает —
// осознанный зазор best-effort ratchet'а.
func ScanFile(fset *token.FileSet, filename string, src []byte, pos Positions) ([]Site, error) {
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	exempt := map[int]bool{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "l10n-exempt") {
				line := fset.Position(c.Pos()).Line
				exempt[line] = true
				exempt[line+1] = true
			}
		}
	}

	var sites []Site
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calleeName(call)
		positions, watched := pos[name]
		if !watched {
			return true
		}
		for _, p := range positions {
			if p >= len(call.Args) {
				continue
			}
			lit, ok := call.Args[p].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil || !hasLetter(text) {
				continue
			}
			line := fset.Position(lit.Pos()).Line
			if exempt[line] {
				continue
			}
			sites = append(sites, Site{File: filename, Line: line, Text: text})
		}
		return true
	})
	return sites, nil
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}
