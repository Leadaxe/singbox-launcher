package backup

// Конформанс-раннер корпуса LX Backup (SPEC 103, фаза 4).
//
// Гоняет contract/corpus/backup/*.backup.json через Import и сверяет с
// <case>.expected.json. Тот же набор обязана проходить сторона LxBox: перенос
// настроек между приложениями имеет смысл ровно настолько, насколько обе
// стороны одинаково понимают битую ссылку, непереносимую переменную и чужой
// блок extensions.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

const backupCorpusRelPath = "../../contract/corpus/backup"

// corpusExpectation — форма <case>.expected.json.
type corpusExpectation struct {
	Rules []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	} `json:"rules"`
	Vars              map[string]string `json:"vars"`
	Warnings          []string          `json:"warnings"`
	RouteFinalApplied *bool             `json:"route_final_applied"`
	// ForeignKeptOtherApp — импортёр обязан сохранить блоб extensions ДРУГОГО
	// приложения. Ожидание сформулировано относительно импортёра, а не по
	// имени приложения: для лаунчера чужой — lxbox, для LxBox — launcher, и
	// фикстура остаётся одна на обе стороны.
	ForeignKeptOtherApp bool     `json:"foreign_extensions_kept_other_app"`
	DisabledHashes      []string `json:"disabled_hashes"`
}

func TestBackupCorpus(t *testing.T) {
	entries, err := os.ReadDir(backupCorpusRelPath)
	if err != nil {
		t.Skipf("корпус бэкапов недоступен: %v", err)
	}

	var cases []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".backup.json") {
			cases = append(cases, strings.TrimSuffix(e.Name(), ".backup.json"))
		}
	}
	sort.Strings(cases)
	if len(cases) == 0 {
		t.Skip("корпус пуст")
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(backupCorpusRelPath, name+".backup.json"))
			if err != nil {
				t.Fatalf("чтение кейса: %v", err)
			}
			expRaw, err := os.ReadFile(filepath.Join(backupCorpusRelPath, name+".expected.json"))
			if err != nil {
				t.Fatalf("чтение ожиданий: %v", err)
			}
			var exp corpusExpectation
			if err := json.Unmarshal(expRaw, &exp); err != nil {
				t.Fatalf("разбор ожиданий: %v", err)
			}

			b, parseWarns, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			dst := &state.State{}
			res, err := Import(dst, b, ImportOptions{
				Mode: ImportReplace,
				// Принимающая сторона знает эти цели; всё прочее —
				// символическая ссылка в никуда.
				KnownOutbounds: []string{"proxy", "direct"},
			})
			if err != nil {
				t.Fatalf("Import: %v", err)
			}

			gotWarns := warnCodes(append(parseWarns, res.Warnings...))
			wantWarns := append([]string(nil), exp.Warnings...)
			sort.Strings(wantWarns)
			if !equalStrings(gotWarns, wantWarns) {
				t.Errorf("коды предупреждений: получено %v, ожидалось %v", gotWarns, wantWarns)
			}

			checkRules(t, dst, exp)
			checkVars(t, dst, exp)
			checkRouteFinal(t, dst, exp)
			checkForeignExtensions(t, dst, exp)
			checkDisabledHashes(t, dst, exp)
		})
	}
}

func checkRules(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(dst.Rules) != len(exp.Rules) {
		t.Fatalf("правил %d, ожидалось %d", len(dst.Rules), len(exp.Rules))
	}
	// Сравниваем в порядке оси, а не в порядке файла: импортёр
	// перенумеровывает, сохраняя относительный порядок.
	type got struct {
		name    string
		enabled bool
		num     int
	}
	all := make([]got, 0, len(dst.Rules))
	for _, r := range dst.Rules {
		name := ruleName(r)
		num := 0
		if r.OrderNum != nil {
			num = *r.OrderNum
		}
		all = append(all, got{name, r.Enabled, num})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].num < all[j].num })

	for i, want := range exp.Rules {
		if all[i].name != want.Name {
			t.Errorf("правило %d: имя %q, ожидалось %q", i, all[i].name, want.Name)
		}
		if all[i].enabled != want.Enabled {
			t.Errorf("правило %q: enabled=%v, ожидалось %v", want.Name, all[i].enabled, want.Enabled)
		}
	}
}

func checkVars(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.Vars == nil {
		return
	}
	have := map[string]string{}
	for _, v := range dst.Vars {
		have[v.Name] = v.Value
	}
	for name, want := range exp.Vars {
		if have[name] != want {
			t.Errorf("переменная %s = %q, ожидалось %q", name, have[name], want)
		}
	}
	for name := range have {
		if _, ok := exp.Vars[name]; !ok {
			t.Errorf("применена переменная %s, которой не должно быть", name)
		}
	}
}

func checkRouteFinal(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.RouteFinalApplied == nil {
		return
	}
	applied := false
	for _, p := range dst.ConfigParams {
		if p.Name == "final" && p.Value != "" {
			applied = true
		}
	}
	if applied != *exp.RouteFinalApplied {
		t.Errorf("route.final применён=%v, ожидалось %v", applied, *exp.RouteFinalApplied)
	}
}

func checkForeignExtensions(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if !exp.ForeignKeptOtherApp {
		return
	}
	// Своё приложение блоб применяет полями, чужое — хранит нетронутым.
	blob, ok := dst.ForeignBackupExtensions[AppLxBox]
	if !ok || len(blob) == 0 {
		t.Errorf("блоб extensions.%s не сохранён — при обратном экспорте данные пропадут", AppLxBox)
	}
	if _, wrong := dst.ForeignBackupExtensions[AppLauncher]; wrong {
		t.Errorf("собственный блоб extensions.%s положен в чужие — он должен применяться полями", AppLauncher)
	}
}

func checkDisabledHashes(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(exp.DisabledHashes) == 0 {
		return
	}
	found := map[string]bool{}
	for _, src := range dst.Connections.Sources {
		for hash := range src.DisabledNodes {
			found[hash] = true
		}
	}
	for _, want := range exp.DisabledHashes {
		if !found[want] {
			t.Errorf("отметка выключенной ноды %s не перенесена", want)
		}
	}
}

func ruleName(r state.Rule) string {
	switch r.Kind {
	case state.RuleKindInline:
		var body state.InlineBody
		if err := json.Unmarshal(r.Body, &body); err == nil {
			return body.Name
		}
	case state.RuleKindSrs:
		var body state.SrsBody
		if err := json.Unmarshal(r.Body, &body); err == nil {
			return body.Name
		}
	case state.RuleKindPreset:
		return r.Ref
	}
	return string(r.Kind)
}

func warnCodes(warns []Warning) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range warns {
		if seen[w.Code] {
			continue
		}
		seen[w.Code] = true
		out = append(out, w.Code)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
