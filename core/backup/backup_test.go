package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

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
		Connections: state.ConnectionsSection{
			Sources: []state.Source{
				{
					ID: "src-1", Type: state.SourceTypeSubscription, Enabled: true,
					URL: "https://example-1.com/sub", Label: "Main", MaxNodes: 200,
					Tag:           &state.TagSpec{Prefix: "[A] "},
					Update:        &state.UpdateSpec{IntervalHours: 12, AutoRefresh: &enabled},
					DisabledNodes: map[string]int64{testNodeHash: 1750000000},
					Skip:          []map[string]string{{"field": "tag", "contains": "trial"}},
					DetourTag:     "hop-1",
				},
				{
					ID: "src-2", Type: state.SourceTypeServer, Enabled: true,
					URI: "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s",
				},
			},
		},
		Rules: []state.Rule{
			mkPresetRule("traffic-processing", 0, true),
			mkInlineRule("Work", "proxy", 1000),
			mkInlineRule("Local", "direct", 1001),
		},
		Vars: []state.SettingVar{
			{Name: "log_level", Value: "debug"},     // переносимая
			{Name: "tun_interface", Value: "utun9"}, // непереносимая
		},
		ConfigParams: []state.ConfigParam{{Name: "final", Value: "proxy"}},
	}
}

func mkPresetRule(ref string, num int, enabled bool) state.Rule {
	body, _ := json.Marshal(state.PresetBody{Vars: map[string]string{"mode": "on"}})
	return state.Rule{Kind: state.RuleKindPreset, Ref: ref, Enabled: enabled, OrderNum: &num, Body: body}
}

func mkInlineRule(name, outbound string, num int) state.Rule {
	body, _ := json.Marshal(state.InlineBody{
		Name:     name,
		Match:    map[string]interface{}{"domain_suffix": []interface{}{"example.com"}},
		Outbound: outbound,
	})
	return state.Rule{Kind: state.RuleKindInline, Enabled: true, OrderNum: &num, Body: body}
}

// Инвариант §1: import(export(x)) == x в том же приложении.
func TestRoundTripLossless(t *testing.T) {
	src := mkState()
	b, err := Export(src, ExportOptions{AppVersion: "1.4.2", Platform: "darwin", Now: time.Unix(1750000000, 0)})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst := &state.State{}
	res, err := Import(dst, b, ImportOptions{
		Mode:           ImportReplace,
		KnownOutbounds: []string{"proxy", "hop-1"},
		KnownPresets:   []string{"traffic-processing"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(dst.Connections.Sources) != 2 {
		t.Fatalf("источников %d, ожидалось 2", len(dst.Connections.Sources))
	}
	sub := dst.Connections.Sources[0]
	if sub.URL != "https://example-1.com/sub" || sub.Label != "Main" || sub.MaxNodes != 200 {
		t.Errorf("подписка приехала искажённой: %+v", sub)
	}
	if sub.Tag == nil || sub.Tag.Prefix != "[A] " {
		t.Errorf("tag-политика потеряна: %+v", sub.Tag)
	}
	if sub.Update == nil || sub.Update.IntervalHours != 12 {
		t.Errorf("политика обновления потеряна: %+v", sub.Update)
	}
	if got := sub.DisabledNodes[testNodeHash]; got != 1750000000 {
		t.Errorf("отметка выключенной ноды потеряна: %v", sub.DisabledNodes)
	}
	// Непереносимые поля обязаны вернуться через extensions.launcher —
	// иначе round-trip на своей же машине теряет настройку.
	if len(sub.Skip) != 1 || sub.Skip[0]["contains"] != "trial" {
		t.Errorf("skip-фильтр потерян: %+v", sub.Skip)
	}
	if sub.DetourTag != "hop-1" {
		t.Errorf("detour потерян: %q", sub.DetourTag)
	}
	if sub.ID != "src-1" {
		t.Errorf("id источника потерян: %q", sub.ID)
	}

	if len(dst.Rules) != 3 {
		t.Fatalf("правил %d, ожидалось 3", len(dst.Rules))
	}
	if res.AppliedRules != 3 || res.AppliedSources != 2 {
		t.Errorf("счётчики: правил %d, источников %d", res.AppliedRules, res.AppliedSources)
	}
	for _, r := range dst.Rules {
		if !r.Enabled {
			t.Errorf("правило приехало выключенным без причины: %+v", r)
		}
	}
}

// Переменные: переносимая применяется, непереносимая — нет, и об этом
// говорится вслух.
func TestImportVarsPortableOnly(t *testing.T) {
	src := mkState()
	b, _ := Export(src, ExportOptions{})
	if _, ok := b.Vars["tun_interface"]; ok {
		t.Error("непереносимая переменная попала в бэкап")
	}
	if b.Vars["log_level"] != "debug" {
		t.Errorf("переносимая переменная потеряна: %v", b.Vars)
	}

	dst := &state.State{}
	b.Vars["tun_interface"] = "utun0" // как будто прислала другая сторона
	res, err := Import(dst, b, ImportOptions{})
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

// §3: route.final в никуда не применяется — иначе весь трафик уходит в
// несуществующий outbound.
func TestImportUnknownFinalNotApplied(t *testing.T) {
	b := &Backup{LxBackup: FormatVersion, Route: &Route{Final: "vpn-9"}}
	dst := &state.State{}
	res, err := Import(dst, b, ImportOptions{KnownOutbounds: []string{"proxy"}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	for _, p := range dst.ConfigParams {
		if p.Name == "final" {
			t.Errorf("final применён вопреки отсутствию цели: %q", p.Value)
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

// Чужой блоб extensions обязан пережить импорт и вернуться в экспорт:
// бэкап, побывавший на десктопе, не должен вернуться на телефон обеднённым.
func TestForeignExtensionsSurviveRoundTrip(t *testing.T) {
	foreign := json.RawMessage(`{"folders":["work","home"],"import_rules":[{"x":1}]}`)
	b := &Backup{
		LxBackup:   FormatVersion,
		Extensions: Extensions{AppLxBox: foreign},
	}
	dst := &state.State{}
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	back, err := Export(dst, ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	got, ok := back.Extensions[AppLxBox]
	if !ok {
		t.Fatal("чужой блоб extensions потерян при обратном экспорте")
	}
	if string(got) != string(foreign) {
		t.Errorf("чужой блоб изменён:\n  было %s\n  стало %s", foreign, got)
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
		var body state.InlineBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			t.Fatalf("тело правила: %v", err)
		}
		if r.OrderNum == nil {
			t.Fatalf("правило %q приехало без номера", body.Name)
		}
		all = append(all, ordered{body.Name, *r.OrderNum})
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
