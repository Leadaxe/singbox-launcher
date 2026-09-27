package config

// Тест по исходникам (контракт 1.1.87, PARSING_PRINCIPLES §10; LxBox §577
// раздел 2): шаги сборки, меняющие тело узла по правилам реестра, проходят
// через точку решения nodeflow.Decide и не пишут в карту тела напрямую.
//
// Опись шагов — SPEC 146 §3. Шаг, исчезнувший из исходников, — ошибка:
// переименование не должно тихо выводить шаг из-под проверки.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// authoredEditSteps — опись шагов сборки, правящих тело по реестру.
var authoredEditSteps = []string{
	"repairBodyForBuild",
	"generateRawNodeJSON",
	"yieldBodyToDetour",
	"yieldToBuildDetour",
	"yieldChainDetour",
	"materializeBody",
	"sanitizeStoredNodeBody",
}

// decisionEntries — вызовы nodeflow, которые сами идут через Decide:
// Decide — точка решения, AuthoredResult и RepairsFor зовут её по каждой
// записи.
var decisionEntries = map[string]bool{
	"Decide":         true,
	"AuthoredResult": true,
	"RepairsFor":     true,
}

func TestAuthoredEditStepsGoThroughDecide(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*ast.FuncDecl{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Body != nil {
				funcs[fd.Name.Name] = fd
			}
		}
	}

	// Граф вызовов пакета: функция доходит до точки решения, если зовёт
	// nodeflow.{Decide,AuthoredResult,RepairsFor} сама или через функцию
	// пакета, которая доходит.
	calls := map[string][]string{}
	direct := map[string]bool{}
	for name, fd := range funcs {
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := ce.Fun.(type) {
			case *ast.Ident:
				calls[name] = append(calls[name], fn.Name)
			case *ast.SelectorExpr:
				if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == "nodeflow" && decisionEntries[fn.Sel.Name] {
					direct[name] = true
				}
			}
			return true
		})
	}
	reaches := map[string]bool{}
	var visit func(name string, seen map[string]bool) bool
	visit = func(name string, seen map[string]bool) bool {
		if r, ok := reaches[name]; ok {
			return r
		}
		if seen[name] {
			return false
		}
		seen[name] = true
		r := direct[name]
		for _, c := range calls[name] {
			if r {
				break
			}
			if _, local := funcs[c]; local && visit(c, seen) {
				r = true
			}
		}
		reaches[name] = r
		return r
	}

	for _, step := range authoredEditSteps {
		fd, ok := funcs[step]
		if !ok {
			t.Errorf("шаг %s не найден в исходниках core/config: обновите опись (SPEC 146 §3)", step)
			continue
		}
		if !visit(step, map[string]bool{}) {
			t.Errorf("шаг %s не проходит через nodeflow.Decide", step)
		}
		for _, w := range directBodyWrites(fd) {
			t.Errorf("шаг %s пишет в карту тела напрямую (%s): правка тела по реестру идёт только через nodeflow.Decide",
				step, fset.Position(w).String())
		}
	}
}

// directBodyWrites — позиции прямой записи в карту в теле функции:
// присваивание по индексу `m[k] = v` и встроенный `delete(m, k)`.
func directBodyWrites(fd *ast.FuncDecl) []token.Pos {
	var out []token.Pos
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if _, ok := lhs.(*ast.IndexExpr); ok {
					out = append(out, lhs.Pos())
				}
			}
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "delete" && id.Obj == nil {
				out = append(out, x.Pos())
			}
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
