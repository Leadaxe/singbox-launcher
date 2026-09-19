// win7guard — страж конструкций, которых нет в тулчейне go1.20 (сборка Win7).
//
// Win7-сборка идёт отдельным модульным файлом go.win7.mod тулчейном
// Go 1.20.14 (`.github/workflows/ci.yml`, джоба `build-win7`), поэтому общий
// код не имеет права пользоваться тем, что появилось в Go 1.21+. Норма
// записана в AGENTS.md, раздел «Правила для всех агентов». Джоба `build-win7`
// запускается только на теге и ручном запуске с целью Win7 — без стража
// нарушение живёт в develop до самого выпуска.
//
// Что ловится:
//
//	import "slices" / "maps"    go1.21
//	builtin min / max / clear   go1.21
//	range по целому числу       go1.22
//	Request.PathValue           go1.22
//
// Разбор по AST, а не грепом, потому что запрещены не подстроки, а значения:
// `min` в комментарии, метод `p.clear()` и локальный хелпер `func min(...)`
// законны. Набор файлов считает go/build для windows/386 с ReleaseTags
// по go1.20 — файлы за `//go:build darwin` и `//go:build go1.22` в Win7-сборку
// не входят, и проверять их нельзя (`core/debugapi/pathparam_go122.go`
// использует PathValue законно).
//
// Ограничение: `range` по целому ловится только для литерала (`for range 10`).
// `for range n` требует вывода типов, то есть загрузки всего модуля обоими
// тулчейнами; цена не стоит покрытия — литерал даёт основную форму записи.
//
// Запуск из корня репозитория: go run ./tools/win7guard
package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// maxGoMinor — последняя минорная версия Go, доступная легаси-тулчейну.
const maxGoMinor = 20

var forbiddenImports = map[string]string{
	"slices": "go1.21",
	"maps":   "go1.21",
}

var forbiddenBuiltins = map[string]string{
	"min":   "go1.21",
	"max":   "go1.21",
	"clear": "go1.21",
}

var skipDirs = map[string]bool{
	"dist":         true,
	"temp":         true,
	"vendor":       true,
	"node_modules": true,
	"testdata":     true,
}

type finding struct {
	pos  token.Position
	what string
}

func main() {
	ctx := win7Context()
	fset := token.NewFileSet()

	dirs, err := packageDirs(".")
	if err != nil {
		fail(err)
	}

	var findings []finding
	scanned := 0

	for _, dir := range dirs {
		names, err := includedFiles(ctx, dir)
		if err != nil {
			fail(err)
		}
		parsed := make(map[string]*ast.File, len(names))
		for _, name := range names {
			path := filepath.Join(dir, name)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				fail(err)
			}
			parsed[path] = file
			scanned++
		}

		// Имена, объявленные в пакете, перекрывают builtin: такой вызов
		// собирается и go1.20 (см. slicesContains в ui/clash_api_tab_helpers.go).
		declared := declaredNames(parsed)
		for _, file := range parsed {
			findings = append(findings, inspect(fset, file, declared)...)
		}
	}

	if len(findings) == 0 {
		fmt.Printf("✅ win7guard: no go1.21+ constructs in the Win7 build set (%d files scanned)\n", scanned)
		return
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].pos.Filename != findings[j].pos.Filename {
			return findings[i].pos.Filename < findings[j].pos.Filename
		}
		return findings[i].pos.Line < findings[j].pos.Line
	})

	for _, f := range findings {
		fmt.Printf("❌ Win7 (go1.20) forbidden construct: %s at %s:%d\n", f.what, f.pos.Filename, f.pos.Line)
	}
	fmt.Println("   The Win7 build uses the Go 1.20 toolchain (go.win7.mod).")
	fmt.Println("   Allowed alternatives: a local helper (see ui/clash_api_tab_helpers.go:42),")
	fmt.Println("   or a build-tagged twin (see core/debugapi/pathparam_legacy.go).")
	os.Exit(1)
}

// win7Context повторяет условия сборки Win7: windows/386, cgo, тег desktop
// и релизные теги не выше go1.20.
func win7Context() *build.Context {
	ctx := build.Default
	ctx.GOOS = "windows"
	ctx.GOARCH = "386"
	ctx.CgoEnabled = true
	ctx.BuildTags = []string{"desktop"}

	ctx.ReleaseTags = nil
	for minor := 1; minor <= maxGoMinor; minor++ {
		ctx.ReleaseTags = append(ctx.ReleaseTags, fmt.Sprintf("go1.%d", minor))
	}
	return &ctx
}

func packageDirs(root string) ([]string, error) {
	var dirs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if path != root && (skipDirs[base] || strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_")) {
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	return dirs, err
}

// includedFiles отдаёт не-тестовые файлы каталога, попадающие в Win7-сборку.
func includedFiles(ctx *build.Context, dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Ошибка разбора build-тегов — забота компилятора, не стража.
		if match, err := ctx.MatchFile(dir, name); err != nil || !match {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func declaredNames(parsed map[string]*ast.File) map[string]bool {
	declared := map[string]bool{}
	note := func(id *ast.Ident) {
		if id != nil && forbiddenBuiltins[id.Name] != "" {
			declared[id.Name] = true
		}
	}

	for _, file := range parsed {
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncDecl:
				note(v.Name)
			case *ast.TypeSpec:
				note(v.Name)
			case *ast.ValueSpec:
				for _, id := range v.Names {
					note(id)
				}
			case *ast.Field:
				for _, id := range v.Names {
					note(id)
				}
			case *ast.AssignStmt:
				if v.Tok != token.DEFINE {
					return true
				}
				for _, lhs := range v.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						note(id)
					}
				}
			case *ast.RangeStmt:
				if id, ok := v.Key.(*ast.Ident); ok {
					note(id)
				}
				if id, ok := v.Value.(*ast.Ident); ok {
					note(id)
				}
			}
			return true
		})
	}
	return declared
}

func inspect(fset *token.FileSet, file *ast.File, declared map[string]bool) []finding {
	var found []finding
	add := func(n ast.Node, what string) {
		found = append(found, finding{pos: fset.Position(n.Pos()), what: what})
	}

	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if since := forbiddenImports[path]; since != "" {
			add(imp, fmt.Sprintf("import %q (%s)", path, since))
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CallExpr:
			switch fun := v.Fun.(type) {
			case *ast.Ident:
				if since := forbiddenBuiltins[fun.Name]; since != "" && !declared[fun.Name] {
					add(v, fmt.Sprintf("builtin %s(...) (%s)", fun.Name, since))
				}
			case *ast.SelectorExpr:
				if fun.Sel.Name == "PathValue" {
					add(v, "Request.PathValue (go1.22)")
				}
			}
		case *ast.RangeStmt:
			if lit, ok := v.X.(*ast.BasicLit); ok && lit.Kind == token.INT {
				add(v, "range over integer (go1.22)")
			}
		}
		return true
	})
	return found
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "win7guard: %v\n", err)
	os.Exit(2)
}
