package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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
	// Прежде эти поля ездили карманом extensions.launcher; теперь они —
	// обычные поля записи, и roundtrip на своей же машине обязан их вернуть.
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
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	back, err := Export(dst, ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
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
			var body state.InlineBody
			_ = json.Unmarshal(r.Body, &body)
			out = append(out, body.Name)
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
		if r.OrderNum == nil {
			t.Fatalf("правило %d потеряло номер после normalize", i)
		}
		if *r.OrderNum != state.UserRuleNumStart+i {
			t.Errorf("номер правила %q = %d, ожидался %d (импортные номера переписаны)",
				got[i], *r.OrderNum, state.UserRuleNumStart+i)
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

// TestRoundTripChainSources — цепочки (SPEC 110, схема v1.2) едут корневой
// секцией chains[] со всеми полями канона и переживают экспорт→импорт;
// блоб extensions.launcher больше не пишется (BACKUP.md §2).
func TestRoundTripChainSources(t *testing.T) {
	stripOff := false
	s := &state.State{}
	s.Connections.Sources = []state.Source{
		{Type: state.SourceTypeSubscription, URL: "https://example.com/sub", Enabled: true},
		{
			Type:    state.SourceTypeChain,
			Label:   "chain-1",
			Enabled: true,
			Chain: &configtypes.SourceChain{
				Hops:         []string{"warp", "vpn ②"},
				IdleTimeout:  "0s",
				StripEvasion: &stripOff,
				Strip:        map[string]bool{"tls.utls": false},
				// null-значение — RFC 7396 (удаление ключа), обязано
				// пережить перенос как есть.
				Rewrite: map[string]interface{}{
					"vless": map[string]interface{}{"flow": nil},
				},
			},
		},
	}

	b, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Chains) != 1 || b.Chains[0].Tag != "chain-1" {
		t.Fatalf("секция chains[] не собрана: %+v", b.Chains)
	}

	restored := &state.State{}
	if _, err := Import(restored, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	var chain *state.Source
	for i := range restored.Connections.Sources {
		if restored.Connections.Sources[i].Type == state.SourceTypeChain {
			chain = &restored.Connections.Sources[i]
		}
	}
	if chain == nil {
		t.Fatal("цепочка потеряна на roundtrip")
	}
	if chain.NodeTagOrLabel() != "chain-1" || chain.Chain == nil {
		t.Fatalf("состав цепочки искажён: %+v", chain)
	}
	want, _ := json.Marshal(s.Connections.Sources[1].Chain)
	got, _ := json.Marshal(chain.Chain)
	if string(want) != string(got) {
		t.Fatalf("канон цепочки искажён: %s, ожидалось %s", got, want)
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
	if _, err := Import(restored, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	for _, src := range restored.Connections.Sources {
		if src.Type == state.SourceTypeChain {
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
	for _, src := range s.Connections.Sources {
		if src.Type == state.SourceTypeChain {
			count++
			if src.Chain == nil || src.Chain.Hops[0] != "first-1" {
				t.Fatalf("вторая запись перезаписала первую: %+v", src.Chain)
			}
		}
	}
	if count != 1 {
		t.Fatalf("цепочек %d, ожидалась одна", count)
	}
}

// TestRoundTripDNSSection — DNS-секция применяется на импорте (раньше
// экспортировалась и молча игнорировалась).
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

	b, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	restored := &state.State{}
	if _, err := Import(restored, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}

	if restored.DNS.Final != "dns_shield" || restored.DNS.Strategy != "ipv4_only" {
		t.Fatalf("final/strategy потеряны: %q %q", restored.DNS.Final, restored.DNS.Strategy)
	}
	if len(restored.DNS.Servers) != 2 {
		t.Fatalf("servers: %+v", restored.DNS.Servers)
	}
	if restored.DNS.Servers[1].Body["server"] != "10.0.0.1" {
		t.Fatalf("тело user-сервера потеряно: %+v", restored.DNS.Servers[1])
	}
	if len(restored.DNS.Rules) != 1 || restored.DNS.Rules[0].Enabled {
		t.Fatalf("rules: %+v", restored.DNS.Rules)
	}
}

// TestRoundTripLocalOutbounds — локальные outbound'ы подписки: экспорт писал
// их в extensions.launcher с самого начала, а импорт не читал.
func TestRoundTripLocalOutbounds(t *testing.T) {
	s := &state.State{}
	s.Connections.Sources = []state.Source{{
		Type:    state.SourceTypeSubscription,
		URL:     "https://example.com/sub",
		Enabled: true,
		Outbounds: []configtypes.Direction{
			{Tag: "local-select", Type: "selector"},
		},
	}}

	b, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	restored := &state.State{}
	if _, err := Import(restored, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	src := restored.Connections.Sources[0]
	if len(src.Outbounds) != 1 || src.Outbounds[0].Tag != "local-select" {
		t.Fatalf("локальные outbound'ы потеряны: %+v", src.Outbounds)
	}
}

// TestRoundTripWarpAccounts — warp[] едет и возвращается.
func TestRoundTripWarpAccounts(t *testing.T) {
	s := &state.State{}
	s.WarpAccounts = &state.WarpAccountsSection{
		WG: &state.WarpWGAccount{PrivateKey: "priv", PeerPublic: "pub", ClientV4: "172.16.0.2"},
	}
	b, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Warp) != 1 {
		t.Fatalf("warp не экспортирован: %v", b.Warp)
	}
	restored := &state.State{}
	if _, err := Import(restored, b, ImportOptions{}); err != nil {
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
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	out, err := Export(s, ExportOptions{AppVersion: "test"})
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

// SPEC 112-A — ссылка detour-на-узел переносится ОБЪЕКТОМ: id источника-цели
// плюс identity-тег узла. Обе половины обязаны пережить roundtrip, включая
// сами id источников: без них ссылка на приёмнике мертва.
func TestRoundTripDetourNodeRef(t *testing.T) {
	s := &state.State{}
	s.Connections.Sources = []state.Source{
		{
			ID:      "01WARP00000000000000000",
			Type:    state.SourceTypeServer,
			URI:     "vless://u@h:443",
			NodeTag: "🔥🎭 WARP (MASQUE)",
			Label:   "WARP hop",
			Enabled: true,
		},
		{
			ID:                 "01PROTON0000000000000000",
			Type:               state.SourceTypeSubscription,
			URL:                "https://example.com/sub",
			Enabled:            true,
			DetourNodeSourceID: "01WARP00000000000000000",
			DetourNodeTag:      "🔥🎭 WARP (MASQUE)",
			DetourNodeLabel:    "WARP hop",
		},
	}

	b, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	// Ссылка едет ОБЩИМИ полями записи, а не карманом: extensions больше
	// не существует (П3), и ключи обязаны лежать прямо в subscriptions[0].
	raw, err := json.Marshal(b.Subscriptions[0])
	if err != nil {
		t.Fatal(err)
	}
	var ext map[string]interface{}
	if err := json.Unmarshal(raw, &ext); err != nil {
		t.Fatal(err)
	}
	if ext["detour_node_source_id"] != "01WARP00000000000000000" {
		t.Fatalf("detour_node_source_id не выехал в бэкап: %v", ext)
	}
	if ext["detour_node_tag"] != "🔥🎭 WARP (MASQUE)" {
		t.Fatalf("detour_node_tag не выехал в бэкап: %v", ext)
	}
	if _, stale := ext["detour_node_hash"]; stale {
		t.Errorf("упразднённый detour_node_hash не должен писаться: %v", ext)
	}

	restored := &state.State{}
	if _, err := Import(restored, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	var hop, dep *state.Source
	for i := range restored.Connections.Sources {
		switch restored.Connections.Sources[i].Type {
		case state.SourceTypeServer:
			hop = &restored.Connections.Sources[i]
		case state.SourceTypeSubscription:
			dep = &restored.Connections.Sources[i]
		}
	}
	if hop == nil || dep == nil {
		t.Fatalf("источники не восстановились: %+v", restored.Connections.Sources)
	}
	// Ключ вопроса из ТЗ: id источника-цели обязан пережить roundtrip, иначе
	// ссылка на приёмнике указывает в никуда.
	if hop.ID != "01WARP00000000000000000" {
		t.Fatalf("id источника-цели потерян: %q", hop.ID)
	}
	if dep.DetourNodeSourceID != hop.ID {
		t.Fatalf("DetourNodeSourceID после импорта = %q, ожидался %q", dep.DetourNodeSourceID, hop.ID)
	}
	if dep.DetourNodeTag != "🔥🎭 WARP (MASQUE)" {
		t.Fatalf("DetourNodeTag после импорта = %q", dep.DetourNodeTag)
	}
}

// Переходная форма: ссылка только тегом (dev-состояния между SPEC 112 и
// 112-A). Она обязана переехать как есть — на приёмнике её разрешит
// глобальный поиск по финальному тегу.
func TestRoundTripDetourNodeTagOnlyRef(t *testing.T) {
	s := &state.State{}
	s.Connections.Sources = []state.Source{{
		Type:            state.SourceTypeSubscription,
		URL:             "https://example.com/sub",
		Enabled:         true,
		DetourNodeTag:   "🔥🎭 WARP (MASQUE)",
		DetourNodeLabel: "WARP hop",
	}}

	b, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	restored := &state.State{}
	if _, err := Import(restored, b, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	got := restored.Connections.Sources[0]
	if got.DetourNodeTag != "🔥🎭 WARP (MASQUE)" || got.DetourNodeSourceID != "" {
		t.Fatalf("tag-only ссылка искажена: source_id=%q tag=%q", got.DetourNodeSourceID, got.DetourNodeTag)
	}
}

// Упразднённый detour_node_hash в схему 0.11.0 не входит и в файл не
// пишется. Источник, ещё не прошедший миграцию, отдаёт пустую ссылку: её
// восстановит первая же сборка из своего состояния, а вывоз протухающего хеша
// вернул бы в формат ровно то, ради чего он снесён (BACKUP.md §6).
func TestLegacyDetourNodeHashNotExported(t *testing.T) {
	s := &state.State{}
	s.Connections.Sources = []state.Source{{
		Type:            state.SourceTypeSubscription,
		URL:             "https://example.com/sub",
		Enabled:         true,
		DetourNodeHash:  "62bff800aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DetourNodeLabel: "🔥🎭 WARP (MASQUE)",
	}}

	b, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "62bff800") || strings.Contains(string(raw), "detour_node_hash") {
		t.Fatalf("упразднённый хеш уехал в файл: %s", raw)
	}
	// Подпись без самой ссылки — тоже мусор: показывать нечего, а поле
	// сделало бы два экспорта разными на неотличимых состояниях.
	if strings.Contains(string(raw), "detour_node_label") {
		t.Fatalf("осиротевшая подпись ссылки уехала в файл: %s", raw)
	}
}

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
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("старый файл уронил импорт: %v", err)
	}
	if len(dst.Connections.Sources) != 1 {
		t.Fatalf("источников %d, ожидался 1", len(dst.Connections.Sources))
	}
	src := dst.Connections.Sources[0]
	// Общие поля применились...
	if src.URL != "https://example.com/sub" || src.Label != "Main" || src.MaxNodes != 150 {
		t.Errorf("общие поля старого файла не применились: %+v", src)
	}
	// ...а содержимое кармана не применилось и не осело в состоянии.
	if src.DetourNodeHash != "" || src.DetourNodeTag != "" || src.ID != "" {
		t.Errorf("содержимое extensions просочилось в состояние: %+v", src)
	}
}
