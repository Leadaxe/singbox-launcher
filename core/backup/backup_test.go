package backup

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

// readRegistryVars возвращает {имя: portable} из contract/registry/vars.json.
func readRegistryVars() (map[string]bool, error) {
	data, err := os.ReadFile(filepath.Join("..", "..", "contract", "registry", "vars.json"))
	if err != nil {
		return nil, err
	}
	var file struct {
		Vars map[string]struct {
			Portable bool `json:"portable"`
		} `json:"vars"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(file.Vars))
	for name, v := range file.Vars {
		out[name] = v.Portable
	}
	return out, nil
}

// Список переносимых переменных — зеркало реестра. Разъехавшийся список
// означает, что бэкап либо теряет настройку, либо тащит на чужую машину
// значение, которое там значит другое.
func TestPortableVarsMatchRegistry(t *testing.T) {
	raw, err := readRegistryVars()
	if err != nil {
		t.Skipf("реестр недоступен: %v", err)
	}
	for name, portable := range raw {
		got := IsPortableVar(name)
		if got != portable {
			t.Errorf("%s: код считает portable=%v, реестр — %v", name, got, portable)
		}
	}
}

// testNodeHash — identity-хеш формата схемы (64 hex): отметки выключенных
// нод переносятся только по хешу (BACKUP.md §4).
const testNodeHash = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"

func mkState() *state.State {
	enabled := true
	return &state.State{
		Sources: []state.Source{
			{
				ID: "src-1",
				Node: state.Node{
					Kind: state.SourceKindSubscription, Enabled: true,
					Detour: &state.NodeLink{Tag: "hop-1"},
				},
				URL: "https://example-1.com/sub", Name: "Main", MaxNodes: 200,
				TagPolicy:       &state.TagPolicy{Prefix: "[A] "},
				Update:          &state.UpdateSpec{IntervalHours: 12, AutoRefresh: &enabled},
				PendingDisabled: []string{testNodeHash},
				Skip:            []map[string]string{{"field": "tag", "contains": "trial"}},
			},
			{
				ID: "src-2", Node: state.Node{
					Kind: state.SourceKindServer, Enabled: true, Tag: "s",
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s"},
				},
			},
		},
		Rules: []state.Rule{
			mkPresetRule("traffic-processing", 0, true),
			mkInlineRule("Work", "proxy", 1000),
			mkInlineRule("Local", "direct", 1001),
			// Самостоятельный эффект правила (не цель): `action` без
			// `outbound` обязан пережить круг export→import, иначе правило
			// приезжает обратно матчером без действия.
			mkEffectRule("Sniff", "sniff", 1002),
		},
		Vars: []state.SettingVar{
			{Name: "log_level", Value: "debug"},     // переносимая
			{Name: "tun_interface", Value: "utun9"}, // непереносимая
			{Name: "route_final", Value: "proxy"},   // непереносимая: едет секцией route.final
		},
	}
}

func mkPresetRule(ref string, num int, enabled bool) state.Rule {
	r := state.NewPresetRule(ref, map[string]string{"mode": "on"})
	r.Enabled = enabled
	r.Num = &num
	return r
}

// mkEffectRule — inline-правило с самостоятельным sing-box `action`
// (`sniff`/`hijack-dns`/`resolve`) и без цели: ключ живёт в теле как матчер-
// эффект, а не как цель.
func mkEffectRule(name, action string, num int) state.Rule {
	r := state.NewInlineRule(name, map[string]interface{}{
		"inbound": "tun-in",
		"action":  action,
	}, "")
	r.Enabled = true
	r.Num = &num
	return r
}

func mkInlineRule(name, outbound string, num int) state.Rule {
	r := state.NewInlineRule(name,
		map[string]interface{}{"domain_suffix": []interface{}{"example.com"}}, outbound)
	r.Enabled = true
	r.Num = &num
	return r
}

// importedAs — итог импорта одной и той же настройки одним из входов.
type importedAs struct {
	// format — «1.0» или «0.12»: имя подтеста и строки ошибки.
	format string
	state  *state.State
	res    *ImportResult
	// warns — предупреждения разбора и импорта вместе.
	warns []Warning
}

// importBothFormats — одна и та же настройка ДВУМЯ входами импорта: файл,
// который лаунчер пишет сейчас (Export10 → байты → Parse), и файл 0.12,
// который прежний писатель снимал с того же состояния.
//
// Писателя 0.12 больше нет (D-110), а читатель живёт всегда: такие файлы у
// пользователей на руках. Поэтому второй вход — сырой JSON, снятый прежним
// писателем с того же состояния, а не выдуманный руками: проверяется чтение
// того, что действительно выпущено. Слияние у входов одно (import.go), и
// утверждения сценария обязаны держаться на обоих.
func importBothFormats(t *testing.T, s *state.State, legacy012 string, opts ImportOptions) []importedAs {
	t.Helper()
	inputs := []struct {
		format string
		want   FileFormat
		raw    []byte
	}{
		{"1.0", FileFormat10, fixedExport10(t, s)},
		{"0.12", FileFormatLegacy, []byte(legacy012)},
	}
	out := make([]importedAs, 0, len(inputs))
	for _, in := range inputs {
		f, parseWarns, err := Parse(in.raw)
		if err != nil {
			t.Fatalf("%s: Parse: %v", in.format, err)
		}
		// Иначе оба прогона могли бы пройти одним читателем, и legacy-вход
		// остался бы без проверки при зелёном тесте.
		if f.Format != in.want {
			t.Fatalf("%s: файл прочитан не своим входом (%v)", in.format, f.Format)
		}
		dst := &state.State{}
		res, err := ImportFile(dst, f, opts)
		if err != nil {
			t.Fatalf("%s: Import: %v", in.format, err)
		}
		out = append(out, importedAs{
			format: in.format, state: dst, res: res,
			warns: append(parseWarns, res.Warnings...),
		})
	}
	return out
}

// legacyMkState012 — файл 0.12, который прежний писатель снимал с mkState().
const legacyMkState012 = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.4.2", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "subscriptions": [{
    "id": "src-1",
    "url": "https://example-1.com/sub",
    "label": "Main",
    "max_nodes": 200,
    "tag": {"prefix": "[A] "},
    "update": {"interval_hours": 12, "auto": true},
    "disabled": {"a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90": 0},
    "skip": [{"contains": "trial", "field": "tag"}],
    "detour_node_tag": "hop-1",
    "detour_node_label": "hop-1"
  }],
  "servers": [{
    "id": "src-2",
    "uri": "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s",
    "node_tag": "s"
  }],
  "rules": [
    {"kind": "preset", "num": 0, "ref": "traffic-processing", "vars": {"mode": "on"}},
    {"kind": "inline", "name": "Work", "num": 1000, "outbound": "proxy",
     "match": {"domain_suffix": ["example.com"]}},
    {"kind": "inline", "name": "Local", "num": 1001, "outbound": "direct",
     "match": {"domain_suffix": ["example.com"]}},
    {"kind": "inline", "name": "Sniff", "num": 1002,
     "match": {"action": "sniff", "inbound": "tun-in"}}
  ],
  "vars": {"log_level": "debug"},
  "route": {"final": "proxy"}
}`

// Инвариант §1: import(export(x)) == x в том же приложении — и та же
// настройка обязана получиться из файла 0.12, снятого с неё прежним
// лаунчером (legacy-вход).
func TestRoundTripLossless(t *testing.T) {
	inputs := importBothFormats(t, mkState(), legacyMkState012, ImportOptions{
		KnownOutbounds: []string{"proxy", "hop-1"},
		KnownPresets:   []string{"traffic-processing"},
	})
	for _, in := range inputs {
		t.Run(in.format, func(t *testing.T) {
			dst, res := in.state, in.res
			if len(dst.Sources) != 2 {
				t.Fatalf("источников %d, ожидалось 2", len(dst.Sources))
			}
			sub := dst.Sources[0]
			if sub.URL != "https://example-1.com/sub" || sub.Name != "Main" || sub.MaxNodes != 200 {
				t.Errorf("подписка приехала искажённой: %+v", sub)
			}
			if sub.TagPolicy == nil || sub.TagPolicy.Prefix != "[A] " {
				t.Errorf("tag-политика потеряна: %+v", sub.TagPolicy)
			}
			if sub.Update == nil || sub.Update.IntervalHours != 12 {
				t.Errorf("политика обновления потеряна: %+v", sub.Update)
			}
			// SPEC 118 W5: отметка выключения едет по СЫРОМУ тегу узла; узлов у
			// импортированной подписки ещё нет (nodes[] в контракт не едут),
			// поэтому она ждёт первого достоверного fetch в PendingDisabled
			// (вердикт O2).
			if len(sub.PendingDisabled) != 1 || sub.PendingDisabled[0] != testNodeHash {
				t.Errorf("отметка выключенной ноды потеряна: %v", sub.PendingDisabled)
			}
			// Прежде эти поля ездили карманом extensions.launcher; теперь они —
			// обычные поля записи, и roundtrip на своей же машине обязан их
			// вернуть.
			if len(sub.Skip) != 1 || sub.Skip[0]["contains"] != "trial" {
				t.Errorf("skip-фильтр потерян: %+v", sub.Skip)
			}
			if sub.Detour == nil || sub.Detour.Tag != "hop-1" {
				t.Errorf("detour потерян: %+v", sub.Detour)
			}
			if sub.ID != "src-1" {
				t.Errorf("id источника потерян: %q", sub.ID)
			}

			if len(dst.Rules) != 4 {
				t.Fatalf("правил %d, ожидалось 4", len(dst.Rules))
			}
			if res.AppliedRules != 4 || res.AppliedSources != 2 {
				t.Errorf("счётчики: правил %d, источников %d", res.AppliedRules, res.AppliedSources)
			}
			// Самостоятельный `action` — эффект правила, а не цель: его нельзя
			// потерять ни в теле 1.0, ни в `match` файла 0.12.
			effectBody, err := dst.Rules[3].BodyMap()
			if err != nil {
				t.Fatalf("тело правила-эффекта: %v", err)
			}
			if effectBody["action"] != "sniff" || effectBody["inbound"] != "tun-in" {
				t.Errorf("самостоятельный action потерян на круге бэкапа: %v", effectBody)
			}
			for _, r := range dst.Rules {
				if !r.Enabled {
					t.Errorf("правило приехало выключенным без причины: %+v", r)
				}
			}
		})
	}
}

// Переменные: переносимая применяется, непереносимая — нет, и об этом
// говорится вслух.
func TestImportVarsPortableOnly(t *testing.T) {
	src := mkState()
	b, _, err := Export10(src, ExportOptions{})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	if _, ok := b.Vars["tun_interface"]; ok {
		t.Error("непереносимая переменная попала в бэкап")
	}
	if b.Vars["log_level"] != "debug" {
		t.Errorf("переносимая переменная потеряна: %v", b.Vars)
	}

	dst := &state.State{}
	b.Vars["tun_interface"] = "utun0" // как будто прислала другая сторона
	res, err := Import10(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	for _, v := range dst.Vars {
		if v.Name == "tun_interface" {
			t.Error("непереносимая переменная применена")
		}
	}
	if !hasWarn(res.Warnings, WarnBackupVarSkipped) {
		t.Errorf("пропуск переменной не назван: %v", res.Warnings)
	}
}

// §3: правило с несуществующей целью импортируется ВЫКЛЮЧЕННЫМ.
// Включённое правило с мёртвым outbound роняет весь конфиг ядра.
func TestImportUnknownOutboundDisablesRule(t *testing.T) {
	b := &Backup{
		LxBackup: FormatVersion,
		Rules: []Rule{
			{Kind: RuleInline, Name: "Ghost", Outbound: "vpn-3",
				Match: json.RawMessage(`{"domain_suffix":["x.com"]}`)},
		},
	}
	dst := &state.State{}
	res, err := Import(dst, b, ImportOptions{KnownOutbounds: []string{"proxy", "direct"}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(dst.Rules) != 1 {
		t.Fatalf("правило потеряно: %d", len(dst.Rules))
	}
	if dst.Rules[0].Enabled {
		t.Error("правило с несуществующим outbound приехало ВКЛЮЧЁННЫМ — конфиг ядра упадёт")
	}
	if !hasWarn(res.Warnings, WarnBackupUnknownOutbound) {
		t.Errorf("не названо: %v", res.Warnings)
	}
}

// §3 для входа 1.0: та же проверка целей на записях формы v8.
//
// Форма записи здесь другая (цель живёт КЛЮЧОМ sing-box внутри body, а не
// полем `outbound` записи бэкапа), и читается она видом — DecodeBody. Пока
// разбор вида шёл по значениям, а DecodeBody возвращает указатели, switch
// молча проваливался в default и отдавал «цели нет» ДЛЯ ЛЮБОГО правила:
// проверка §9 п. 7 в пути 1.0 не работала вовсе, правило приезжало включённым
// и роняло config.json целиком. Проверяются ОБА вида с целью — inline и srs.
func TestImport10UnknownOutboundDisablesRule(t *testing.T) {
	ghost := state.NewInlineRule("Ghost",
		map[string]interface{}{"domain_suffix": []interface{}{"x.com"}}, "vpn-3")
	ghost.Enabled = true
	ghostSrs := state.NewSrsRule("GhostSrs", []string{"https://example.com/a.srs"}, "vpn-9")
	ghostSrs.Enabled = true
	alive := state.NewInlineRule("Alive",
		map[string]interface{}{"domain_suffix": []interface{}{"y.com"}}, "proxy")
	alive.Enabled = true

	b := &Backup10{LxBackup: FormatVersion10, Rules: []state.Rule{ghost, ghostSrs, alive}}
	dst := &state.State{}
	res, err := Import10(dst, b, ImportOptions{KnownOutbounds: []string{"proxy", "direct"}})
	if err != nil {
		t.Fatalf("Import10: %v", err)
	}
	if len(dst.Rules) != 3 {
		t.Fatalf("правил %d, ожидалось 3", len(dst.Rules))
	}
	for i, name := range []string{"Ghost", "GhostSrs"} {
		if dst.Rules[i].Enabled {
			t.Errorf("%s: правило с несуществующим outbound приехало ВКЛЮЧЁННЫМ — конфиг ядра упадёт", name)
		}
	}
	if !dst.Rules[2].Enabled {
		t.Error("Alive: правило с существующей целью выключено")
	}
	if !hasWarn(res.Warnings, WarnBackupUnknownOutbound) {
		t.Errorf("не названо: %v", res.Warnings)
	}
}

// Норма «одно правило — одно тело» (D-111) на входе 1.0: запись с массивом в
// `body` обязана дать РОВНО то состояние, что те же правила, развёрнутые в
// файле подряд, — тогда и config.json из них байт-в-байт тот же. Заодно
// проверяется, что части — обычные записи: у каждой своя проверка цели
// (выключена только часть с мёртвой целью), `id` другой стороны остаётся у
// первой, не-объект отбрасывается прежним кодом, соседние записи не
// перемешиваются с частями по оси.
func TestImport10RuleBodyArrayEqualsExpandedRecords(t *testing.T) {
	num := func(n int) *int { return &n }
	rule := func(name, id string, n int, body string) state.Rule {
		return state.Rule{Kind: state.RuleKindInline, ID: id, Name: name, Enabled: true, Num: num(n), Body: json.RawMessage(body)}
	}
	const (
		first  = `{"domain_suffix":["m.example-1.com"],"outbound":"vpn-3"}`
		second = `{"ip_cidr":["203.0.113.0/24"],"action":"reject","method":"drop"}`
	)
	arrayFile := &Backup10{LxBackup: FormatVersion10, Rules: []state.Rule{
		rule("After", "", 1002, `{"domain_suffix":["a.example-1.com"],"outbound":"proxy"}`),
		rule("Mixed", "lx-1", 1001, `[`+first+`,`+second+`,42]`),
	}}
	expandedFile := &Backup10{LxBackup: FormatVersion10, Rules: []state.Rule{
		rule("After", "", 1002, `{"domain_suffix":["a.example-1.com"],"outbound":"proxy"}`),
		rule("Mixed", "lx-1", 1001, first),
		rule("Mixed #2", "", 1001, second),
	}}
	opts := ImportOptions{KnownOutbounds: []string{"proxy", "direct"}}

	fromArray := &state.State{}
	res, err := Import10(fromArray, arrayFile, opts)
	if err != nil {
		t.Fatalf("Import10 (массив): %v", err)
	}
	fromExpanded := &state.State{}
	if _, err := Import10(fromExpanded, expandedFile, opts); err != nil {
		t.Fatalf("Import10 (развёрнутые записи): %v", err)
	}

	if !reflect.DeepEqual(fromArray.Rules, fromExpanded.Rules) {
		t.Fatalf("массив в body дал не то же состояние, что развёрнутые записи:\n%+v\n%+v", fromArray.Rules, fromExpanded.Rules)
	}
	names := make([]string, 0, len(fromArray.Rules))
	for _, r := range fromArray.Rules {
		names = append(names, r.Name)
	}
	if want := []string{"Mixed", "Mixed #2", "After"}; !equalStrings(names, want) {
		t.Errorf("порядок оси %v, ожидался %v", names, want)
	}
	if fromArray.Rules[0].Enabled || !fromArray.Rules[1].Enabled {
		t.Errorf("проверка цели не по частям: enabled %v / %v", fromArray.Rules[0].Enabled, fromArray.Rules[1].Enabled)
	}
	if !hasWarn(res.Warnings, WarnBackupUnknownField) || !hasWarn(res.Warnings, WarnBackupUnknownOutbound) {
		t.Errorf("не названы отброшенный элемент и мёртвая цель: %v", res.Warnings)
	}
}

// §3: route.final в никуда не применяется — иначе весь трафик уходит в
// несуществующий outbound.
func TestImportUnknownFinalNotApplied(t *testing.T) {
	b := &Backup{LxBackup: FormatVersion, Route: &Route{Final: "vpn-9"}}
	dst := &state.State{}
	res, err := Import(dst, b, ImportOptions{KnownOutbounds: []string{"proxy"}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	for _, v := range dst.Vars {
		if v.Name == "route_final" {
			t.Errorf("final применён вопреки отсутствию цели: %q", v.Value)
		}
	}
	if !hasWarn(res.Warnings, WarnBackupFinalDropped) {
		t.Errorf("не названо: %v", res.Warnings)
	}
}

// Зарезервированные литералы существуют всегда — объявлять их не нужно.
func TestReservedOutboundsAlwaysKnown(t *testing.T) {
	for _, tag := range []string{"direct", "block", "reject", "drop"} {
		b := &Backup{LxBackup: FormatVersion, Rules: []Rule{
			{Kind: RuleInline, Name: "R", Outbound: tag, Match: json.RawMessage(`{}`)},
		}}
		dst := &state.State{}
		res, err := Import(dst, b, ImportOptions{KnownOutbounds: []string{"proxy"}})
		if err != nil {
			t.Fatalf("Import(%s): %v", tag, err)
		}
		if !dst.Rules[0].Enabled {
			t.Errorf("правило с литералом %q выключено", tag)
		}
		if hasWarn(res.Warnings, WarnBackupUnknownOutbound) {
			t.Errorf("литерал %q принят за неизвестную цель", tag)
		}
	}
}

// П3: чужой блоб extensions больше НЕ провозится — он отбрасывается с
// warning'ом при разборе файла. Провоз непонятого создавал состояние-призрак,
// которое протухало, когда каноническую часть правили на другой стороне.
func TestForeignExtensionsDroppedWithWarning(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox","version":"2.0.0"},` +
		`"exported_at":"2026-08-22T00:00:00Z",` +
		`"extensions":{"lxbox":{"folders":["work"]}}}`)
	b, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !hasWarn(warns, WarnBackupExtensionsDropped) {
		t.Fatalf("отброшенный extensions не назван: %v", warns)
	}
	dst := &state.State{}
	if _, err := ImportFile(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	back, _, err := Export10(dst, ExportOptions{})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	out, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "extensions") {
		t.Fatalf("extensions вернулся в экспорт — карман провоза не закрыт: %s", out)
	}
}

// Мажорная версия формата: новее — отказ с понятным текстом, не паника и
// не тихий импорт половины полей.
func TestImportRejectsNewerMajor(t *testing.T) {
	b := &Backup{LxBackup: FormatVersion + 1}
	_, err := Import(&state.State{}, b, ImportOptions{})
	if err == nil {
		t.Fatal("бэкап новее поддерживаемого принят")
	}
}

// Ось порядка перенумеровывается, но ОТНОСИТЕЛЬНЫЙ порядок сохраняется:
// абсолютные номера у сторон свои, важен лишь порядок следования.
func TestImportRenumbersPreservingOrder(t *testing.T) {
	n := func(v int) *float64 { f := float64(v); return &f }
	b := &Backup{LxBackup: FormatVersion, Rules: []Rule{
		{Kind: RuleInline, Name: "third", Num: n(9000), Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "first", Num: n(10), Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "second", Num: n(500), Match: json.RawMessage(`{}`)},
	}}
	dst := &state.State{}
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	type ordered struct {
		name string
		num  int
	}
	all := make([]ordered, 0, len(dst.Rules))
	for _, r := range dst.Rules {
		if r.Num == nil {
			t.Fatalf("правило %q приехало без номера", r.Name)
		}
		all = append(all, ordered{r.Name, *r.Num})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].num < all[j].num })
	want := []string{"first", "second", "third"}
	for i, w := range want {
		if all[i].name != w {
			t.Fatalf("порядок нарушен: получено %v, ожидалось %v", all, want)
		}
	}
	// Номера переписаны в свою зону, а не оставлены чужими.
	if all[0].num != state.UserRuleNumStart {
		t.Errorf("нумерация не переписана: первый номер %d, ожидался %d",
			all[0].num, state.UserRuleNumStart)
	}
}

// SPEC 106-B: подключение оси к визарду не должно ломать импорт. Номера,
// проставленные renumberImportedRules, — уже разметка: NormalizeRuleOrder на
// первой же загрузке обязан оставить их и порядок как есть, а не пере-размечать.
func TestNormalizeKeepsImportedOrder(t *testing.T) {
	n := func(v int) *float64 { f := float64(v); return &f }
	b := &Backup{LxBackup: FormatVersion, Rules: []Rule{
		{Kind: RuleInline, Name: "third", Num: n(9000), Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "first", Num: n(10), Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "second", Num: n(500), Match: json.RawMessage(`{}`)},
	}}
	dst := &state.State{}
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	names := func(rules []state.Rule) []string {
		out := make([]string, 0, len(rules))
		for _, r := range rules {
			out = append(out, r.Name)
		}
		return out
	}

	// Шаблон пустой: у импорта нет пресетов, seed'ить нечего.
	normalized := state.NormalizeRuleOrder(dst.Rules, map[string]state.RuleOrderSpec{})
	got := names(normalized)
	want := []string{"first", "second", "third"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("normalize переставил импортированные правила: %v, ожидалось %v", got, want)
		}
	}
	for i, r := range normalized {
		if r.Num == nil {
			t.Fatalf("правило %d потеряло номер после normalize", i)
		}
		if *r.Num != state.UserRuleNumStart+i {
			t.Errorf("номер правила %q = %d, ожидался %d (импортные номера переписаны)",
				got[i], *r.Num, state.UserRuleNumStart+i)
		}
	}

	// Идемпотентность: второй проход ничего не меняет.
	again := state.NormalizeRuleOrder(normalized, map[string]state.RuleOrderSpec{})
	for i, name := range names(again) {
		if name != want[i] {
			t.Fatalf("повторный normalize переставил правила: %v", names(again))
		}
	}
}

// Чужой kind не роняет импорт: остальные правила обязаны приехать.
func TestImportUnknownKindSkipsOnlyThatRule(t *testing.T) {
	b := &Backup{LxBackup: FormatVersion, Rules: []Rule{
		{Kind: RuleJSON, Name: "raw-lxbox"},
		{Kind: RuleInline, Name: "ok", Outbound: "direct", Match: json.RawMessage(`{}`)},
	}}
	dst := &state.State{}
	res, err := Import(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("одно чужое правило уронило весь импорт: %v", err)
	}
	if len(dst.Rules) != 1 {
		t.Fatalf("применено %d правил, ожидалось 1", len(dst.Rules))
	}
	if !hasWarn(res.Warnings, WarnBackupUnknownField) {
		t.Errorf("пропуск чужого правила не назван: %v", res.Warnings)
	}
}

func hasWarn(list []Warning, code string) bool {
	for _, w := range list {
		if w.Code == code {
			return true
		}
	}
	return false
}

// TestRoundTripChainSources — цепочки (SPEC 110) едут записью sources[] вида
// chain: настройки маршрута в теле, позиции в hops, — и переживают
// экспорт→импорт; блоба extensions.launcher нет (BACKUP.md §2). Чтение
// секции chains[] файлов 0.12 держит корпус (chains_roundtrip).
func TestRoundTripChainSources(t *testing.T) {
	stripOff := false
	s := &state.State{}
	s.Sources = []state.Source{
		{Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true}, URL: "https://example.com/sub"},
		{
			// SPEC 118 W5: цепочка — узел канона: настройки маршрута в body,
			// позиции отдельным полем hops.
			Node: state.Node{
				Kind: state.SourceKindChain, Enabled: true, Tag: "chain-1",
				Body: configtypes.ChainBody(&configtypes.SourceChain{
					IdleTimeout:  "0s",
					StripEvasion: &stripOff,
					Strip:        map[string]bool{"tls.utls": false},
					// null-значение — RFC 7396 (удаление ключа), обязано
					// пережить перенос как есть.
					Rewrite: map[string]interface{}{
						"vless": map[string]interface{}{"flow": nil},
					},
				}),
				Hops: []state.NodeLink{{Tag: "warp"}, {Tag: "vpn ②"}},
			},
			Label: "chain-1",
		},
	}

	b, _, err := Export10(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	chains := 0
	for _, rec := range b.Sources {
		if rec.Kind == state.SourceKindChain {
			chains++
			if rec.Tag != "chain-1" {
				t.Fatalf("тег цепочки в файле %q, ожидался chain-1", rec.Tag)
			}
		}
	}
	if chains != 1 {
		t.Fatalf("цепочек в sources[] %d, ожидалась одна: %+v", chains, b.Sources)
	}

	restored := &state.State{}
	if _, err := Import10(restored, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	var chain *state.Source
	for i := range restored.Sources {
		if restored.Sources[i].Kind == state.SourceKindChain {
			chain = &restored.Sources[i]
		}
	}
	if chain == nil {
		t.Fatal("цепочка потеряна на roundtrip")
	}
	if chain.NodeTagOrLabel() != "chain-1" || len(chain.Hops) != 2 {
		t.Fatalf("состав цепочки искажён: %+v", chain)
	}
	if string(chain.Body) != string(s.Sources[1].Body) {
		t.Fatalf("тело цепочки искажено: %s, ожидалось %s", chain.Body, s.Sources[1].Body)
	}
	if chain.Hops[0].Tag != "warp" || chain.Hops[1].Tag != "vpn ②" {
		t.Fatalf("позиции цепочки искажены: %+v", chain.Hops)
	}
}

// П4: legacy-развилки чтения нет. Файл релизов v1.5.0–v1.5.1 нёс цепочки
// блобом extensions.launcher; теперь этот карман — обычное неизвестное поле:
// отбрасывается с warning'ом, цепочки из него не материализуются. Цена
// разрыва задокументирована (BACKUP.md §10), молчания нет.
func TestLegacyExtensionsChainsNotRead(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"launcher","version":"1.5.1"},` +
		`"exported_at":"2026-08-22T00:00:00Z","extensions":{"launcher":{"chains":` +
		`[{"type":"chain","label":"old-relay","enabled":true,"chain":{"hops":["a","b"]}}]}}}`)
	b, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !hasWarn(warns, WarnBackupExtensionsDropped) {
		t.Fatalf("legacy-блоб отброшен молча: %v", warns)
	}
	restored := &state.State{}
	if _, err := ImportFile(restored, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	for _, src := range restored.Sources {
		if src.Kind == state.SourceKindChain {
			t.Fatalf("цепочка прочитана из упразднённого кармана: %+v", src)
		}
	}
}

// TestImportChainTagBusy — занятый тег: первая запись побеждает, вторая
// пропускается, и это ВСЕГДА предъявляется warning'ом — молчаливое «одна
// победила» скрыло бы случайных тёзок (BACKUP.md §4).
//
// Тёзки проверяются ВНУТРИ одного файла: режим импорта один — replace, и
// коллизия «своя против приехавшей» в нём невозможна по построению.
func TestImportChainTagBusy(t *testing.T) {
	b := &Backup{
		LxBackup: FormatVersion,
		Chains: []Chain{
			{Tag: "relay", Chain: &configtypes.SourceChain{Hops: []string{"first-1", "first-2"}}},
			{Tag: "relay", Chain: &configtypes.SourceChain{Hops: []string{"second-1", "second-2"}}},
		},
	}

	s := &state.State{}
	res, err := Import(s, b, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarn(res.Warnings, WarnBackupChainExists) {
		t.Fatal("занятый тег не предъявлен warning'ом")
	}
	count := 0
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindChain {
			count++
			if len(src.Hops) == 0 || src.Hops[0].Tag != "first-1" {
				t.Fatalf("вторая запись перезаписала первую: %+v", src.Hops)
			}
		}
	}
	if count != 1 {
		t.Fatalf("цепочек %d, ожидалась одна", count)
	}
}

// TestRoundTripDNSSection — DNS-секция применяется на импорте (раньше
// экспортировалась и молча игнорировалась). Оба входа: у 1.0 тег и тело
// лежат `tag`/`body`, у файла 0.12 — `name`/`value`, и итог обязан совпасть.
func TestRoundTripDNSSection(t *testing.T) {
	s := &state.State{}
	s.DNS.Final = "dns_shield"
	s.DNS.Strategy = "ipv4_only"
	s.DNS.Servers = []state.DNSServer{
		{Kind: state.DNSServerKindTemplate, Tag: "google_dot", Enabled: true},
		{Kind: state.DNSServerKindUser, Tag: "my_dns", Enabled: true,
			Body: map[string]interface{}{"type": "udp", "server": "10.0.0.1"}},
	}
	s.DNS.Rules = []state.DNSRule{
		{Kind: state.DNSRuleKindUser, Enabled: false,
			Body: map[string]interface{}{"domain_suffix": "example.com", "server": "my_dns"}},
	}

	// Файл 0.12, который прежний писатель снимал с этого состояния.
	const legacy = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "dns": {
    "servers": [
      {"kind": "template", "name": "google_dot"},
      {"kind": "user", "name": "my_dns", "value": {"server": "10.0.0.1", "type": "udp"}}
    ],
    "rules": [
      {"kind": "user", "enabled": false, "value": {"domain_suffix": "example.com", "server": "my_dns"}}
    ],
    "final": "dns_shield",
    "strategy": "ipv4_only"
  }
}`

	for _, in := range importBothFormats(t, s, legacy, ImportOptions{}) {
		restored := in.state
		if restored.DNS.Final != "dns_shield" || restored.DNS.Strategy != "ipv4_only" {
			t.Fatalf("%s: final/strategy потеряны: %q %q", in.format, restored.DNS.Final, restored.DNS.Strategy)
		}
		if len(restored.DNS.Servers) != 2 {
			t.Fatalf("%s: servers: %+v", in.format, restored.DNS.Servers)
		}
		if restored.DNS.Servers[0].Tag != "google_dot" || restored.DNS.Servers[1].Tag != "my_dns" {
			t.Errorf("%s: теги серверов: %+v", in.format, restored.DNS.Servers)
		}
		if restored.DNS.Servers[1].Body["server"] != "10.0.0.1" {
			t.Fatalf("%s: тело user-сервера потеряно: %+v", in.format, restored.DNS.Servers[1])
		}
		if len(restored.DNS.Rules) != 1 || restored.DNS.Rules[0].Enabled {
			t.Fatalf("%s: rules: %+v", in.format, restored.DNS.Rules)
		}
	}
}

// TestRoundTripLocalOutbounds удалён вместе с предметом: локальных
// Направлений источника в модели v7 нет (SPEC 118 W5). Их наследник —
// FolderReplace, и его перенос проверяет корпус контракта (fold ⇄ replace).

// TestRoundTripWarpAccounts — warp[] едет и возвращается.
func TestRoundTripWarpAccounts(t *testing.T) {
	s := &state.State{}
	s.WarpAccounts = &state.WarpAccountsSection{
		WG: &state.WarpWGAccount{PrivateKey: "priv", PeerPublic: "pub", ClientV4: "172.16.0.2"},
	}
	b, _, err := Export10(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Warp) != 1 {
		t.Fatalf("warp не экспортирован: %v", b.Warp)
	}
	restored := &state.State{}
	if _, err := Import10(restored, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	if restored.WarpAccounts == nil || restored.WarpAccounts.WG == nil ||
		restored.WarpAccounts.WG.PrivateKey != "priv" {
		t.Fatalf("warp потерян: %+v", restored.WarpAccounts)
	}
}

// П3 на уровне записи: extensions ВНУТРИ подписки тоже отбрасывается — и
// одним общим warning'ом с корневым, а не отдельной строкой на каждую запись.
func TestPerEntityForeignExtensionsDropped(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox","version":"2.0.0"},` +
		`"exported_at":"2026-08-22T00:00:00Z","subscriptions":[{"url":"https://example.com/sub",` +
		`"extensions":{"lxbox":{"import_rules":true}}}]}`)
	b, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	n := 0
	for _, w := range warns {
		if w.Code == WarnBackupExtensionsDropped {
			n++
			if !strings.Contains(w.Detail, "https://example.com/sub") {
				t.Errorf("warning не называет затронутую запись: %q", w.Detail)
			}
		}
	}
	if n != 1 {
		t.Fatalf("warning'ов об extensions %d, ожидался ровно один на файл: %v", n, warns)
	}
	s := &state.State{}
	if _, err := ImportFile(s, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	out, _, err := Export10(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	enc, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "import_rules") {
		t.Fatalf("mobile-блоб записи провезён вопреки П3: %s", enc)
	}
}

// Ссылка detour на КОРНЕВОЙ узел переносится корневой формой `{tag}`
// (NODE_LINK.md §2 п. 5): id корневого узла адресом не является.
//
// Входов два, и форма ссылки у них разная: у 1.0 — объект `detour{tag}`, у
// файла 0.12 — плоская тройня `detour_node_*`, где `detour_node_source_id`
// несёт id СЕРВЕРА (в 0.12 сервер был сам себе источником). Итог импорта
// обязан совпасть: `{tag: тег узла здесь}` без folder_id. Прежде 0.12
// ввозил id сервера в folder_id, и на сборке такая ссылка не разрешалась
// никогда («the referenced source is gone», §7.4).
func TestRoundTripDetourNodeRef(t *testing.T) {
	s := &state.State{}
	s.Sources = []state.Source{
		{
			ID: "01WARP00000000000000000",
			Node: state.Node{
				Kind: state.SourceKindServer, Enabled: true, Tag: "🔥🎭 WARP (MASQUE)",
				Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "vless://u@h:443"},
			},
			Label: "WARP hop",
		},
		{
			ID: "01PROTON0000000000000000",
			Node: state.Node{
				Kind: state.SourceKindSubscription, Enabled: true,
				Detour: &state.NodeLink{Tag: "🔥🎭 WARP (MASQUE)"},
			},
			URL: "https://example.com/sub",
		},
	}

	// Ссылка едет ОБЩИМ полем записи, а не карманом: extensions больше не
	// существует (П3), и в файле 1.0 это объект `detour` самой подписки.
	var doc struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(fixedExport10(t, s), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sources) != 2 {
		t.Fatalf("источников в файле %d, ожидалось 2", len(doc.Sources))
	}
	var link map[string]string
	if err := json.Unmarshal(doc.Sources[1]["detour"], &link); err != nil {
		t.Fatalf("detour подписки не объектом: %s (%v)", doc.Sources[1]["detour"], err)
	}
	if _, has := link["folder_id"]; has || link["tag"] != "🔥🎭 WARP (MASQUE)" {
		t.Fatalf("ссылка в файле 1.0 = %v, ожидалась корневая форма без folder_id", link)
	}

	// Файл 0.12, который прежний писатель снимал с состояния, где ссылка
	// адресовала сервер его id.
	const legacy = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "subscriptions": [{
    "id": "01PROTON0000000000000000",
    "url": "https://example.com/sub",
    "detour_node_source_id": "01WARP00000000000000000",
    "detour_node_tag": "🔥🎭 WARP (MASQUE)",
    "detour_node_label": "🔥🎭 WARP (MASQUE)"
  }],
  "servers": [{
    "id": "01WARP00000000000000000",
    "uri": "vless://u@h:443",
    "node_tag": "🔥🎭 WARP (MASQUE)"
  }]
}`

	for _, in := range importBothFormats(t, s, legacy, ImportOptions{}) {
		var hop, dep *state.Source
		for i := range in.state.Sources {
			switch in.state.Sources[i].Kind {
			case state.SourceKindServer:
				hop = &in.state.Sources[i]
			case state.SourceKindSubscription:
				dep = &in.state.Sources[i]
			}
		}
		if hop == nil || dep == nil {
			t.Fatalf("%s: источники не восстановились: %+v", in.format, in.state.Sources)
		}
		want := state.NodeLink{Tag: hop.NodeTagOrLabel()}
		if dep.Detour == nil || *dep.Detour != want {
			t.Fatalf("%s: ссылка после импорта = %+v, ожидалась корневая %+v", in.format, dep.Detour, want)
		}
	}
}

// Переходная форма: ссылка только тегом (dev-состояния между SPEC 112 и
// 112-A). Она обязана переехать как есть — на приёмнике её разрешит
// глобальный поиск по финальному тегу.
func TestRoundTripDetourNodeTagOnlyRef(t *testing.T) {
	s := &state.State{}
	s.Sources = []state.Source{{
		Node: state.Node{
			Kind: state.SourceKindSubscription, Enabled: true,
			Detour: &state.NodeLink{Tag: "🔥🎭 WARP (MASQUE)"},
		},
		URL: "https://example.com/sub",
	}}

	// Файл 0.12, который прежний писатель снимал с этого состояния.
	const legacy = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "subscriptions": [{
    "url": "https://example.com/sub",
    "detour_node_tag": "🔥🎭 WARP (MASQUE)",
    "detour_node_label": "🔥🎭 WARP (MASQUE)"
  }]
}`

	for _, in := range importBothFormats(t, s, legacy, ImportOptions{}) {
		got := in.state.Sources[0]
		if got.Detour == nil || got.Detour.Tag != "🔥🎭 WARP (MASQUE)" || got.Detour.FolderID != "" {
			t.Fatalf("%s: ссылка корневого пространства искажена: %+v", in.format, got.Detour)
		}
	}
}

// TestLegacyDetourNodeHashNotExported удалён вместе с предметом: поля
// detour_node_hash в модели v7 нет вовсе (SPEC 118 W5), и «вывезти» его
// стало невыразимо по построению. Что хеш не пишется в файл — по-прежнему
// держит TestRoundTripDetourNodeRef.

// Старый файл, где ссылка лежала хешем в extensions: общие поля читаются,
// карман отбрасывается с warning'ом, ссылка по хешу теряется — это
// задокументированная цена разрыва (П4), а не молчаливая потеря.
func TestLegacyDetourNodeHashFileReadsWithWarning(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"launcher","version":"1.5.3"},` +
		`"exported_at":"2026-08-22T00:00:00Z","subscriptions":[{"url":"https://example.com/sub",` +
		`"label":"Main","max_nodes":150,"extensions":{"launcher":{"id":"src-1",` +
		`"detour_node_hash":"62bff800aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
		`"detour_node_label":"WARP hop"}}}]}`)
	b, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !hasWarn(warns, WarnBackupExtensionsDropped) {
		t.Fatalf("потеря кармана не названа: %v", warns)
	}
	dst := &state.State{}
	if _, err := ImportFile(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("старый файл уронил импорт: %v", err)
	}
	if len(dst.Sources) != 1 {
		t.Fatalf("источников %d, ожидался 1", len(dst.Sources))
	}
	src := dst.Sources[0]
	// Общие поля применились...
	if src.URL != "https://example.com/sub" || src.Name != "Main" || src.MaxNodes != 150 {
		t.Errorf("общие поля старого файла не применились: %+v", src)
	}
	// ...а содержимое кармана не применилось и не осело в состоянии.
	// ID при этом НЕ пуст: SPEC 117 (Р3) — импорт как создатель Source
	// минтит свежий ULID, но именно свежий, а не id из кармана.
	if src.Detour != nil || src.ID == "src-1" {
		t.Errorf("содержимое extensions просочилось в состояние: %+v", src)
	}
	if len(src.ID) != 26 {
		t.Errorf("импорт обязан выдать источнику свежий ULID (Р3): id=%q", src.ID)
	}
}
