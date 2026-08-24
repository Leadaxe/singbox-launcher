// Сканер вызовов слоя локализации (SPEC 111, этап 4).
// Чистая логика без ввода-вывода — покрыта self-тестом scan_test.go.
// Ограничение: только stdlib и синтаксис go1.20 (win7-джоба гоняет
// `go get ./...` тулчейном go 1.20.14 по всем пакетам модуля).
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// Usage накапливает факты использования одного ключа по всем файлам.
type Usage struct {
	TFamily      bool  // locale.T / Tf / TN / TfN
	PluralFamily bool  // locale.Plural / PluralN
	Forms        []int // использованные индексы special-форм (>0)
	Sites        []string
}

// ScanResult — итог обхода дерева исходников.
type ScanResult struct {
	Used    map[string]*Usage // ключ → использование
	Dynamic []string          // сайты с нелитеральным ключом (file:line)
}

// funcInfo описывает функции locale.* : позиция аргумента-ключа,
// позиция аргумента-формы (-1 если нет) и семейство.
type funcInfo struct {
	keyArg  int
	formArg int
	plural  bool
}

var localeFuncs = map[string]funcInfo{
	"T":       {keyArg: 0, formArg: -1},
	"Tf":      {keyArg: 0, formArg: -1},
	"TN":      {keyArg: 1, formArg: 0},
	"TfN":     {keyArg: 1, formArg: 0},
	"Plural":  {keyArg: 0, formArg: -1, plural: true},
	"PluralN": {keyArg: 1, formArg: 0, plural: true},
}

// Helpers: имя функции → позиции аргументов, являющихся ключами
// (внутри хелпер зовёт locale.T). Читается из l10n_helpers.json.
type Helpers map[string][]int

// ScanSource разбирает один файл и дописывает находки в res.
// Строковый литерал на строке с комментарием l10n-key считается
// использованным ключом (для ключей, уходящих в locale.T через переменные
// и функции, которые сканер не резолвит).
func ScanSource(fset *token.FileSet, filename string, src []byte, helpers Helpers, res *ScanResult) error {
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return err
	}

	// Строковые константы файла: locale.T(constName) резолвится по ним
	// (длинные тексты выносятся в const рядом с использованием).
	consts := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, name := range vs.Names {
				if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
					if v, err := strconv.Unquote(bl.Value); err == nil {
						consts[name.Name] = v
					}
				}
			}
		}
	}

	marker := map[int]bool{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "l10n-key") {
				marker[fset.Position(c.Pos()).Line] = true
			}
		}
	}

	use := func(key string, fi funcInfo, form int, pos token.Position) {
		u := res.Used[key]
		if u == nil {
			u = &Usage{}
			res.Used[key] = u
		}
		if fi.plural {
			u.PluralFamily = true
		} else {
			u.TFamily = true
		}
		if form > 0 {
			u.Forms = append(u.Forms, form)
		}
		u.Sites = append(u.Sites, pos.String())
	}

	ast.Inspect(f, func(n ast.Node) bool {
		// Маркированные литералы: строка маркера или следующая за ней.
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			// Маркер действует только на своей строке: правило «строкой выше»
			// цепляло соседние wire-литералы (Status: "downloading").
			line := fset.Position(lit.Pos()).Line
			if marker[line] {
				if key, err := strconv.Unquote(lit.Value); err == nil {
					// Семейство по маркеру неизвестно — засчитываем как
					// T: usage-conflict для маркированных ключей не ловим.
					use(key, funcInfo{}, 0, fset.Position(lit.Pos()))
				}
			}
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// locale.<Fn>(...)
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "locale" {
				fi, known := localeFuncs[sel.Sel.Name]
				if !known {
					return true
				}
				form := 0
				if fi.formArg >= 0 && fi.formArg < len(call.Args) {
					if bl, ok := call.Args[fi.formArg].(*ast.BasicLit); ok && bl.Kind == token.INT {
						form, _ = strconv.Atoi(bl.Value)
					}
				}
				if fi.keyArg < len(call.Args) {
					switch arg := call.Args[fi.keyArg].(type) {
					case *ast.BasicLit:
						if arg.Kind == token.STRING {
							if key, err := strconv.Unquote(arg.Value); err == nil {
								use(key, fi, form, fset.Position(arg.Pos()))
								return true
							}
						}
					case *ast.Ident:
						if key, ok := consts[arg.Name]; ok {
							use(key, fi, form, fset.Position(arg.Pos()))
							return true
						}
					}
				}
				res.Dynamic = append(res.Dynamic, fset.Position(call.Pos()).String())
				return true
			}
		}

		// Зарегистрированный хелпер: его аргументы-ключи.
		name := calleeName(call)
		if positions, ok := helpers[name]; ok {
			for _, p := range positions {
				if p < len(call.Args) {
					if bl, ok := call.Args[p].(*ast.BasicLit); ok && bl.Kind == token.STRING {
						if key, err := strconv.Unquote(bl.Value); err == nil && key != "" {
							use(key, funcInfo{}, 0, fset.Position(bl.Pos()))
						}
					}
				}
			}
		}
		return true
	})
	return nil
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
