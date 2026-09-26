package config

// Линтер ссылок реестра на код лаунчера (SPEC 142, волна 8, находки D1–D4).
//
// Реестр говорит, ГДЕ правило исполняется: `refs.go`, `impl` (в том числе
// `mapper[].impl`), `go` у кодов warnings.json. Пока эти ссылки никто не
// проверял, они протухали молча: рукописные парсеры снимались, а реестр
// ещё годами отправлял читателя в удалённые файлы и функции.
//
// Что проверяется во ВСЕХ строках реестра (`refs.go`, `impl`, `go`, `note`,
// описания) — кроме поддеревьев `dart`:
//   - путь от корня репозитория (`core/…`, `ui/…`, `internal/…`,
//     `contract/…` и т. п.) к файлу `.go`/`.json` существует;
//   - `путь.go:Имя` — имя объявлено в этом файле: функция, тип, переменная,
//     константа, метод (`Имя` или `Тип.Метод`) либо поле структуры
//     (`Тип.Поле`);
//   - номера строк не ставятся (`путь.go:123` — ошибка): они протухают с
//     первой же правкой файла;
//   - голое имя файла (`foo.go` без каталога) запрещено: его нельзя ни
//     проверить, ни отличить от файла апстрима. Файл апстрима пишется
//     путём от корня своего репозитория (`common/tls/utls_client.go`), а
//     если его путь начинается с каталога лаунчера — с префиксом репозитория
//     (`3x-ui:internal/sub/service.go`).
//
// Dart-ссылки (`refs.dart`, ключ `dart`) не проверяются — это код LxBox.
//
// Go 1.20-совместимо (память win7-build-go120).

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const registryRefsRepoRoot = "../.."

// registryRefPathRe — путь к файлу лаунчера от корня репозитория и
// необязательный хвост `:Имя` (или `:123` — его линтер отвергает).
// Слева не должно быть слова, `/`, `:` и `.`: так `3x-ui:internal/…` и
// `common/tls/…` не принимаются за наши пути.
var registryRefPathRe = regexp.MustCompile(
	`(?:^|[^\w/:.-])((?:api|build|cmd|contract|core|internal|scripts|tools|ui)/[\w./-]*\w\.(?:go|json))(?::([A-Za-z_][\w.]*|\d[\d,-]*))?`)

// registryRefBareRe — голое имя Go-файла без каталога.
var registryRefBareRe = regexp.MustCompile(`(?:^|[^\w/.:-])(\w+\.go)\b`)

func TestRegistryCodeRefsResolve(t *testing.T) {
	var files []string
	err := filepath.Walk(registryBodyDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход реестра: %v", err)
	}
	sort.Strings(files)

	decls := map[string]map[string]bool{}
	var problems []string
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var doc interface{}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		rel, _ := filepath.Rel(registryBodyDir, file)
		for _, ref := range collectRegistryCodeRefs(doc, "") {
			for _, p := range checkRegistryCodeRef(ref.text, decls) {
				problems = append(problems, rel+" "+ref.path+": "+p)
			}
		}
	}
	for _, p := range problems {
		t.Error(p)
	}
}

type registryCodeRef struct{ path, text string }

// collectRegistryCodeRefs собирает все строки реестра; ключ dart
// пропускается целиком. Ссылка на код бывает не только в refs/impl/go:
// заметки и описания тоже называют файлы, и протухают они так же.
func collectRegistryCodeRefs(node interface{}, path string) []registryCodeRef {
	var out []registryCodeRef
	switch v := node.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "dart" {
				continue
			}
			out = append(out, collectRegistryCodeRefs(v[k], path+"/"+k)...)
		}
	case []interface{}:
		for i, item := range v {
			out = append(out, collectRegistryCodeRefs(item, path+"/"+strconv.Itoa(i))...)
		}
	case string:
		out = append(out, registryCodeRef{path: path, text: v})
	}
	return out
}

func checkRegistryCodeRef(text string, decls map[string]map[string]bool) []string {
	var problems []string
	for _, m := range registryRefPathRe.FindAllStringSubmatch(text, -1) {
		file, tail := m[1], m[2]
		full := filepath.Join(registryRefsRepoRoot, filepath.FromSlash(file))
		if _, err := os.Stat(full); err != nil {
			problems = append(problems, "нет файла "+file)
			continue
		}
		if tail == "" {
			continue
		}
		if tail[0] >= '0' && tail[0] <= '9' {
			problems = append(problems, "номер строки в ссылке "+file+":"+tail+" — пишите имя")
			continue
		}
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		names, ok := decls[full]
		if !ok {
			names = goFileDecls(full)
			decls[full] = names
		}
		ident := strings.TrimRight(tail, ".")
		if !names[ident] {
			problems = append(problems, "в "+file+" не объявлено "+ident)
		}
	}
	for _, m := range registryRefBareRe.FindAllStringSubmatch(text, -1) {
		problems = append(problems, "голое имя файла "+m[1]+" — пишите путь от корня репозитория")
	}
	return problems
}

// goFileDecls — имена, которые файл объявляет: верхний уровень, методы
// (`Метод` и `Тип.Метод`) и поля структур (`Тип.Поле`).
func goFileDecls(path string) map[string]bool {
	names := map[string]bool{}
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		names["\x00"+err.Error()] = true
		return names
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			names[d.Name.Name] = true
			if d.Recv != nil && len(d.Recv.List) > 0 {
				if recv := receiverTypeName(d.Recv.List[0].Type); recv != "" {
					names[recv+"."+d.Name.Name] = true
				}
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					names[s.Name.Name] = true
					if st, ok := s.Type.(*ast.StructType); ok {
						for _, fld := range st.Fields.List {
							for _, n := range fld.Names {
								names[s.Name.Name+"."+n.Name] = true
							}
						}
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names[n.Name] = true
					}
				}
			}
		}
	}
	return names
}

func receiverTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	}
	return ""
}
