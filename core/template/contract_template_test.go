package template

// Конформанс-раннер корпуса шаблонов (SPEC 103, фаза 3, D-047).
//
// Гоняет contract/corpus/template/**/<case>.template.json через движок
// подстановки и сравнивает с <case>.expected.json. Тот же корпус гоняет
// LxBox — расхождение между приложениями = баг движка, а не разница платформ
// (сами шаблоны при этом остаются разными, D-046).
//
// Формат кейса — contract/corpus/template/README.md:
//
//	<case>.template.json  {"vars": [...], "config": {...}, "_changed": "имя"}
//	<case>.vars.json      {"имя": "строка"}  (null = optional-var)
//	<case>.expected.json  {"load": "accept|reject|either", "config": {...},
//	                       "warnings": [...], "vars_after": {...}}
//
// Кейсы с "_changed" проверяют on_change (§4.6): переменная объявлена
// изменённой, ApplyOnChange прогоняется до подстановки, а "vars_after"
// фиксирует состояние переменных после каскада.
//
// Регенерация: go test ./core/template -run TestContractCorpusTemplate -update
// Это осознанный PR с ревью диффа, а не рутина (contract/corpus/README.md).

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var updateTemplateGolden = flag.Bool("update", false, "перегенерировать expected-файлы корпуса шаблонов")

const templateCorpusRelPath = "../../contract/corpus/template"

// corpusCase — разобранная тройка файлов кейса.
type corpusCase struct {
	name     string // относительный путь без суффикса, напр. "predicates/p1_bare_bool_true"
	template struct {
		Vars    []TemplateVar   `json:"vars"`
		Config  json.RawMessage `json:"config"`
		Changed string          `json:"_changed"`
	}
	vars     map[string]*string // nil-значение = optional-var (§5.1)
	expected corpusExpected
}

type corpusExpected struct {
	// Load — рубеж ЗАГРУЗКИ шаблона (контракт 0.7.2, D-077):
	// "accept" (или отсутствие поля) — валидатор обязан принять;
	// "reject" — валидатор обязан отвергнуть, при этом рантайм-ожидания
	// кейса продолжают проверяться толерантным прогоном (поле не выключает
	// проверку рантайма); "either" — вердикт намеренно не нормирован
	// (кандидаты unresolved/*: политика undeclared-имён на load не решена).
	Load      string            `json:"load,omitempty"`
	Config    json.RawMessage   `json:"config"`
	Warnings  []string          `json:"warnings"`
	VarsAfter map[string]string `json:"vars_after,omitempty"`
}

func loadCorpusCase(t *testing.T, base string) corpusCase {
	t.Helper()
	var c corpusCase
	c.name = base

	raw, err := os.ReadFile(base + ".template.json")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if err := json.Unmarshal(raw, &c.template); err != nil {
		t.Fatalf("parse template: %v", err)
	}

	rawVars, err := os.ReadFile(base + ".vars.json")
	if err != nil {
		t.Fatalf("read vars: %v", err)
	}
	if err := json.Unmarshal(rawVars, &c.vars); err != nil {
		t.Fatalf("parse vars: %v", err)
	}

	rawExp, err := os.ReadFile(base + ".expected.json")
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}
	if err := json.Unmarshal(rawExp, &c.expected); err != nil {
		t.Fatalf("parse expected: %v", err)
	}
	return c
}

// stateVars разворачивает vars.json в map состояния. Ключи со значением null
// (optional-var) в состояние НЕ попадают — их отсутствие и есть сигнал
// Dropped-каскада; при этом объявление переменной в шаблоне остаётся.
func (c corpusCase) stateVars() map[string]string {
	out := make(map[string]string, len(c.vars))
	for k, v := range c.vars {
		if v == nil {
			continue
		}
		out[k] = *v
	}
	return out
}

// nullVars — имена переменных, явно заданных как null (optional-var).
func (c corpusCase) nullVars() map[string]bool {
	out := make(map[string]bool)
	for k, v := range c.vars {
		if v == nil {
			out[k] = true
		}
	}
	return out
}

// runCorpusCase прогоняет кейс через движок и возвращает фактический результат.
func runCorpusCase(t *testing.T, c corpusCase) corpusExpected {
	t.Helper()
	target := LocalTarget()
	state := c.stateVars()

	// on_change (§4.6): применяется ДО подстановки, в контексте нового
	// значения изменённой переменной — как это делает UI при клике.
	if c.template.Changed != "" {
		ApplyOnChange(c.template.Changed, c.template.Vars, state, target)
	}

	// Значение null в vars.json = optional-var: объявление есть, значения нет.
	// Движок обязан дать Dropped-каскад, а не пустую строку.
	nulls := c.nullVars()
	resolved := ResolveTemplateVarsFor(c.template.Vars, state, nil, target)
	for name := range nulls {
		delete(resolved, name)
	}

	out, warnings, err := SubstituteVarsInJSONCanon(c.template.Config, c.template.Vars, resolved, target)
	if err != nil {
		// Канон роняет подстановку только на невалидном JSON — фикстура битая.
		t.Fatalf("подстановка: %v", err)
	}

	got := corpusExpected{Config: out, Warnings: normalizeWarnings(warnings)}
	if c.template.Changed != "" {
		got.VarsAfter = state
	}
	return got
}

// normalizeWarnings приводит список к сравнимому виду: пустой список вместо
// nil, отсортированный, без дублей (порядок warning'ов не нормируется).
func normalizeWarnings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, w := range in {
		w = strings.TrimSpace(w)
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// jsonEqual сравнивает два JSON-дерева по значению, не по байтам (CANON §7).
func jsonEqual(a, b json.RawMessage) bool {
	var x, y interface{}
	if err := json.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func TestContractCorpusTemplate(t *testing.T) {
	root := templateCorpusRelPath
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skipf("корпус шаблонов не найден: %s", root)
	}

	var bases []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".template.json") {
			bases = append(bases, strings.TrimSuffix(path, ".template.json"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход корпуса: %v", err)
	}
	if len(bases) == 0 {
		t.Fatal("корпус шаблонов пуст")
	}
	sort.Strings(bases)

	var mismatches []string
	for _, base := range bases {
		name, _ := filepath.Rel(root, base)
		t.Run(name, func(t *testing.T) {
			c := loadCorpusCase(t, base)

			// Рубеж загрузки (поле "load" в expected, D-077). Проверяется
			// ДО рантайма и независимо от него: reject-кейс обязан и
			// отвергаться валидатором, и выполнять рантайм-ожидания в
			// толерантном прогоне ниже.
			loadErr := ValidateWizardTemplate(c.template.Vars, nil, c.template.Config)
			switch c.expected.Load {
			case "", "accept":
				if loadErr != nil {
					t.Errorf("load: валидатор отверг шаблон, ожидался accept: %v", loadErr)
				}
			case "reject":
				if loadErr == nil {
					t.Errorf("load: валидатор принял шаблон, ожидался reject")
				}
			case "either":
				// вердикт намеренно не нормирован
			default:
				t.Errorf("load: неизвестное значение %q (accept|reject|either)", c.expected.Load)
			}

			got := runCorpusCase(t, c)

			if *updateTemplateGolden {
				got.Load = c.expected.Load // load нормируется руками, -update его не трогает
				writeGolden(t, base+".expected.json", got)
				return
			}

			ok := jsonEqual(got.Config, c.expected.Config) &&
				reflect.DeepEqual(got.Warnings, normalizeWarnings(c.expected.Warnings))
			if ok && c.template.Changed != "" {
				ok = reflect.DeepEqual(got.VarsAfter, c.expected.VarsAfter)
			}
			if !ok {
				mismatches = append(mismatches, name)
				t.Errorf("расхождение с контрактом\n  config   получено: %s\n  config   ожидалось: %s\n  warnings получено: %v\n  warnings ожидалось: %v\n  vars_after получено: %v\n  vars_after ожидалось: %v",
					got.Config, c.expected.Config,
					got.Warnings, normalizeWarnings(c.expected.Warnings),
					got.VarsAfter, c.expected.VarsAfter)
			}
		})
	}

	if len(mismatches) > 0 {
		t.Logf("СВОДКА: %d из %d кейсов расходятся с контрактом", len(mismatches), len(bases))
		for _, m := range mismatches {
			t.Logf("  ✗ %s", m)
		}
	}
}

func writeGolden(t *testing.T, path string, exp corpusExpected) {
	t.Helper()
	if exp.Warnings == nil {
		exp.Warnings = []string{}
	}
	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
}

// TestContractCorpusDeps — конформанс-раннер раздела corpus/template/deps/
// (SPEC 107 §8.1). Формат кейса — пара файлов:
//
//	<case>.cond.json      условие языка (§5.1) в любой форме
//	<case>.expected.json  {"deps": ["a", "b", …]}  — отсортированные имена
//
// Тот же набор гоняет Dart: извлечение зависимостей нормативно, потому что на
// нём стоит реактивный пересчёт — разъехавшиеся deps означают, что на одной
// платформе строка не обновится при изменении переменной, а на другой
// обновится.
func TestContractCorpusDeps(t *testing.T) {
	root := filepath.Join(templateCorpusRelPath, "deps")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skipf("раздел deps не найден: %s", root)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("чтение %s: %v", root, err)
	}

	cases := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".cond.json") {
			continue
		}
		cases++
		base := strings.TrimSuffix(name, ".cond.json")
		t.Run(base, func(t *testing.T) {
			condRaw, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatalf("read cond: %v", err)
			}
			expRaw, err := os.ReadFile(filepath.Join(root, base+".expected.json"))
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			var exp struct {
				Deps []string `json:"deps"`
			}
			if err := json.Unmarshal(expRaw, &exp); err != nil {
				t.Fatalf("parse expected: %v", err)
			}

			got := CondDepsJSON(condRaw)
			if got == nil {
				got = []string{}
			}
			if exp.Deps == nil {
				exp.Deps = []string{}
			}
			if !reflect.DeepEqual(got, exp.Deps) {
				t.Errorf("deps расходятся\n  получено:  %v\n  ожидалось: %v\n  условие:   %s",
					got, exp.Deps, condRaw)
			}
		})
	}
	if cases == 0 {
		t.Fatal("раздел deps пуст")
	}
}
