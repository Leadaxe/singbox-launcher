package config

// Конформанс-раннер корпуса ФОРМЫ (контракт 1.1.122, REGISTRY_DESIGN §3;
// общие секции, варианты, fixed, массив [0] — 1.1.123, SPEC 168 §2.4):
// contract/corpus/form/<scheme>_form.json — тело узла → видимые поля,
// обязательные (и незаполненные) поля, активный вариант групп выбора,
// нарушения диапазона формы. Формат — contract/corpus/form/README.md.
//
// Вычисление — registry.FormState: чистая функция реестра без Fyne, её же
// читает форма лаунчера. Сравнение строгое: порядок путей — order реестра.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/registry"
)

type formCorpusRange struct {
	Path  string  `json:"path"`
	Value float64 `json:"value"`
}

type formCorpusExpect struct {
	// Body — тело после записи `write` (контракт 1.1.123); без write не
	// сверяется.
	Body       map[string]interface{} `json:"body,omitempty"`
	Visible    []string               `json:"visible"`
	Required   []string               `json:"required"`
	Missing    []string               `json:"missing"`
	Choices    map[string]string      `json:"choices"`
	OutOfRange []formCorpusRange      `json:"out_of_range"`
}

type formCorpusFile struct {
	V      int    `json:"v"`
	Scheme string `json:"scheme"`
	Cases  []struct {
		Name    string                 `json:"name"`
		Comment string                 `json:"comment"`
		Body    map[string]interface{} `json:"body"`
		Write   *formCorpusWrite       `json:"write"`
		Expect  formCorpusExpect       `json:"expect"`
	} `json:"cases"`
}

// formCorpusWrite — что форма пишет в тело перед вычислением (контракт
// 1.1.123, `form.implies`): set — путь поля формы → значение, clear — пути
// очищенных полей. Запись — SetFormValue/DeleteFormValue и ApplyFormImplies
// поля.
type formCorpusWrite struct {
	Set   map[string]interface{} `json:"set"`
	Clear []string               `json:"clear"`
}

func TestContractCorpusForm(t *testing.T) {
	root := filepath.Join(contractCorpusRelPath, "form")
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatalf("корпус формы пуст: %s", root)
	}
	reg := registry.MustGet()
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var f formCorpusFile
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatalf("разбор %s: %v", path, err)
		}
		if len(f.Cases) == 0 {
			t.Errorf("%s: нет кейсов", filepath.Base(path))
		}
		for _, c := range f.Cases {
			c := c
			t.Run(strings.TrimSuffix(filepath.Base(path), ".json")+"/"+c.Name, func(t *testing.T) {
				body := c.Body
				if c.Write != nil {
					body = applyFormCorpusWrite(t, reg, f.Scheme, body, c.Write)
				}
				st, ok := reg.FormState(f.Scheme, body)
				if !ok {
					t.Fatalf("у схемы %q нет формы в реестре", f.Scheme)
				}
				got := formCorpusExpect{
					Visible:  st.Visible,
					Required: st.Required,
					Missing:  st.Missing,
					Choices:  st.Choices,
				}
				if c.Write != nil {
					got.Body = body
				}
				for _, r := range st.OutOfRange {
					got.OutOfRange = append(got.OutOfRange, formCorpusRange{Path: r.Path, Value: r.Value})
				}
				gb, _ := json.Marshal(normalizeFormExpect(got))
				wb, _ := json.Marshal(normalizeFormExpect(c.Expect))
				if string(gb) != string(wb) {
					t.Errorf("расхождение с контрактом\n got %s\nwant %s", gb, wb)
				}
			})
		}
	}
}

// applyFormCorpusWrite пишет в тело то, что набрала бы форма: значения set и
// снятие clear по полям формы схемы, вместе с их `form.implies`.
func applyFormCorpusWrite(t *testing.T, reg *registry.Registry, scheme string, body map[string]interface{}, w *formCorpusWrite) map[string]interface{} {
	t.Helper()
	out := map[string]interface{}{}
	if body != nil {
		raw, _ := json.Marshal(body)
		_ = json.Unmarshal(raw, &out)
	}
	field := func(path string) *registry.Field {
		var found *registry.Field
		for _, ff := range reg.FormFields(scheme) {
			if ff.Path == path {
				if ff.Variant == "" {
					return ff.Field
				}
				if found == nil {
					found = ff.Field
				}
			}
		}
		if found == nil {
			t.Fatalf("write: %q — не поле формы схемы %s", path, scheme)
		}
		return found
	}
	keys := make([]string, 0, len(w.Set))
	for p := range w.Set {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		f := field(p)
		registry.SetFormValue(out, p, w.Set[p])
		registry.ApplyFormImplies(f, out, true)
	}
	for _, p := range w.Clear {
		f := field(p)
		registry.DeleteFormValue(out, p)
		registry.ApplyFormImplies(f, out, false)
	}
	return out
}

// normalizeFormExpect — пустой список и отсутствие списка одно и то же.
func normalizeFormExpect(e formCorpusExpect) formCorpusExpect {
	if e.Visible == nil {
		e.Visible = []string{}
	}
	if e.Required == nil {
		e.Required = []string{}
	}
	if e.Missing == nil {
		e.Missing = []string{}
	}
	if e.Choices == nil {
		e.Choices = map[string]string{}
	}
	if e.OutOfRange == nil {
		e.OutOfRange = []formCorpusRange{}
	}
	return e
}
