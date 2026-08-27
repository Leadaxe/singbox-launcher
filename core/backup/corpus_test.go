package backup

// Конформанс-раннер корпуса LX Backup (контракт 0.11.0).
//
// Гоняет contract/corpus/backup/*.backup.json через Import и сверяет с
// <case>.expected.json. Тот же набор обязана проходить сторона LxBox: перенос
// настроек между приложениями имеет смысл ровно настолько, насколько обе
// стороны одинаково понимают битую ссылку, непереносимую переменную и
// упразднённый карман extensions.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
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
	// ExtensionsDropped — файл несёт упразднённый механизм extensions
	// (схема 0.10.x). Импортёр обязан его ОТБРОСИТЬ и назвать одним
	// warning'ом на файл (BACKUP_PRINCIPLES.md П3/П4), а не провозить:
	// провоз непонятого создавал состояние-призрак. Ожидание сформулировано
	// относительно импортёра и одинаково для обеих сторон.
	ExtensionsDropped bool     `json:"extensions_dropped"`
	DisabledHashes    []string `json:"disabled_hashes"`

	// Directions — Направления, которые импорт обязан СОЗДАТЬ (SPEC 104,
	// схема v1.1). Проверяется каноническая форма, а не внутренняя: она и
	// есть предмет договорённости между приложениями.
	Directions []struct {
		Tag           string `json:"tag"`
		Label         string `json:"label"`
		Filter        string `json:"filter"`
		Invert        bool   `json:"invert"`
		IncludeDirect bool   `json:"include_direct"`
		IncludeBlock  bool   `json:"include_block"`
		HasAuto       bool   `json:"has_auto"`
	} `json:"directions"`

	// Chains — цепочки после импорта (SPEC 110). Список ИСЧЕРПЫВАЮЩИЙ:
	// запись, пропущенная по занятому тегу, не должна материализоваться
	// второй копией. chain сверяется deep-equal канона — включая
	// null-значения rewrite (RFC 7396: null удаляет ключ и обязан пережить
	// перенос как есть). label, если задан, проверяется через re-export:
	// это общее поле схемы, и обе стороны обязаны вернуть его на место.
	//
	// Enabled — указатель, а не bool: умолчание схемы true, и отсутствие
	// ключа в ожиданиях обязано значить «не проверяем», а не «ожидаем
	// false». Обычный bool сделал бы нулевое значение требованием
	// выключенности и провалил бы все кейсы без этого поля.
	Chains []struct {
		Tag     string          `json:"tag"`
		Label   string          `json:"label"`
		Enabled *bool           `json:"enabled"`
		Chain   json.RawMessage `json:"chain"`
	} `json:"chains"`
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
			checkExtensionsDropped(t, dst, exp)
			checkDisabledHashes(t, dst, exp)
			checkDirections(t, dst, exp)
			checkChains(t, dst, exp)
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

// checkExtensionsDropped — упразднённый карман не возвращается в экспорт.
//
// Warning об этом уже сверен общим сравнением кодов; здесь проверяется вторая
// половина П1: состояние после импорта неотличимо от настроенного руками, то
// есть следа от extensions в нём нет и повторный экспорт его не воскрешает.
func checkExtensionsDropped(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if !exp.ExtensionsDropped {
		return
	}
	back, err := Export(dst, ExportOptions{AppVersion: "corpus"})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	raw, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "extensions") {
		t.Errorf("extensions вернулся в экспорт — карман провоза не закрыт: %s", raw)
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

// checkChains проверяет цепочки после импорта (SPEC 110).
//
// Сверяется канон chain (deep-equal, включая null внутри rewrite) и число
// записей; label — через re-export: он общее поле схемы, у лаунчера живёт в
// Source.Label и обязан вернуться на место (П1).
func checkChains(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(exp.Chains) == 0 {
		return
	}
	byTag := map[string]state.Source{}
	count := 0
	for _, src := range dst.Connections.Sources {
		if src.Type == state.SourceTypeChain {
			byTag[src.NodeTagOrLabel()] = src
			count++
		}
	}
	if count != len(exp.Chains) {
		t.Fatalf("цепочек %d, ожидалось %d", count, len(exp.Chains))
	}

	needExport := false
	for _, want := range exp.Chains {
		src, ok := byTag[want.Tag]
		if !ok {
			t.Fatalf("цепочка %q не создана импортом", want.Tag)
		}
		gotRaw, err := json.Marshal(src.Chain)
		if err != nil {
			t.Fatalf("%s: marshal канона: %v", want.Tag, err)
		}
		if !jsonDeepEqual(gotRaw, want.Chain) {
			t.Errorf("%s: канон цепочки искажён: %s, ожидалось %s", want.Tag, gotRaw, want.Chain)
		}
		// Выключенность — состояние записи, а не канона: enabled живёт в
		// обвязке chains[], и умолчание схемы (отсутствие ключа = true)
		// стороны обязаны читать одинаково.
		if want.Enabled != nil && src.Enabled != *want.Enabled {
			t.Errorf("%s: enabled=%v, ожидалось %v", want.Tag, src.Enabled, *want.Enabled)
		}
		if want.Label != "" {
			needExport = true
		}
	}
	if !needExport {
		return
	}
	b, err := Export(dst, ExportOptions{AppVersion: "corpus"})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	exported := map[string]Chain{}
	for _, c := range b.Chains {
		exported[c.Tag] = c
	}
	for _, want := range exp.Chains {
		if want.Label == "" {
			continue
		}
		if got := exported[want.Tag].Label; got != want.Label {
			t.Errorf("%s: label в re-export %q, ожидалось %q — подпись потеряна", want.Tag, got, want.Label)
		}
	}
}

// jsonDeepEqual сравнивает два JSON-фрагмента структурно, без чувствительности
// к порядку ключей и пробелам.
func jsonDeepEqual(a, b json.RawMessage) bool {
	var av, bv interface{}
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// checkDirections проверяет Направления, созданные импортом (SPEC 104).
//
// Сверяется каноническая форма, а не внутренняя структура: именно о ней
// договорились стороны, и раннер LxBox читает те же ожидания.
func checkDirections(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(exp.Directions) == 0 {
		return
	}
	byTag := make(map[string]configtypes.Direction, len(dst.Connections.Outbounds))
	for _, d := range dst.Connections.Outbounds {
		byTag[d.Tag] = d
	}
	for _, want := range exp.Directions {
		got, ok := byTag[want.Tag]
		if !ok {
			t.Fatalf("направление %q не создано импортом", want.Tag)
		}
		body, invert := configtypes.DirectionFilterTag(got.Filters)
		if body != want.Filter || invert != want.Invert {
			t.Errorf("%s: отбор (%q, инверсия=%v), ожидалось (%q, %v)",
				want.Tag, body, invert, want.Filter, want.Invert)
		}
		hasDirect, hasBlock := false, false
		for _, tag := range got.AddOutbounds {
			switch tag {
			case "direct-out":
				hasDirect = true
			case "block-out":
				hasBlock = true
			}
		}
		if hasDirect != want.IncludeDirect || hasBlock != want.IncludeBlock {
			t.Errorf("%s: опции (direct=%v, block=%v), ожидалось (%v, %v)",
				want.Tag, hasDirect, hasBlock, want.IncludeDirect, want.IncludeBlock)
		}
		if (got.Auto != nil) != want.HasAuto {
			t.Errorf("%s: автовыбор=%v, ожидалось %v", want.Tag, got.Auto != nil, want.HasAuto)
		}
	}
}
