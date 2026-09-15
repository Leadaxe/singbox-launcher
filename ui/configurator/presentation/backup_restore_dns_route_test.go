package presentation

// Восстановление настроек не уводит DNS мимо VPN и не теряет выбор адреса
// (релиз 1.6.0; D-117 → SPEC 129 / D-118).
//
// Значения переменных шаблонного DNS-сервера живут в записи
// `dns.servers[kind=template].vars`. Тест проходит весь путь пользователя:
// лаунчер → файл → лаунчер, тремя входами (Debug API в пустое, UI новой машины,
// UI с уже настроенным состоянием), и проверяет то, что увидит ядро:
//
//   - источник записан ДО SPEC 129 и после обновления не пересохранён —
//     значения в корневых `dns_<tag>_<var>`; экспорт обязан перенести их в
//     запись (ловушка Л2), и корневых `dns_*` в файле быть не должно;
//   - маршрут google_udp через proxy-out и выбранный адрес 8.8.4.4 (П-2)
//     переживают оба входа;
//   - UI-вход в непустое состояние (Л13): приёмник тоже в старой форме —
//     загрузка обязана перенести его значения ДО фильтра сирот (Л1), наложение
//     файла — заместить только названные файлом имена.
//
// Заодно UI-вход обязан сохранить цель правила на системный тег шаблона: сброс
// «осиротевших» целей при загрузке переводил block-out на direct-out.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/backup"
	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func TestBackupRestoreKeepsDNSRouteOnNewMachine(t *testing.T) {
	root := findRepoRootForTemplate(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}
	target := wizardtemplate.LocalTarget()
	varMap := func(s *corestate.State) map[string]string {
		out := map[string]string{}
		for _, v := range s.Vars {
			out[v.Name] = v.Value
		}
		return out
	}
	declsFor := func(s *corestate.State) *corestate.RecordVarDecls {
		return wizardtemplate.RecordVarDeclsFor(td, varMap(s), target)
	}

	// ── Машина-источник, форма до SPEC 129: финальный DNS google_udp через
	// proxy-out, адрес — вторичный 8.8.4.4 ────────────────────────────────
	src := corestate.New()
	src.Directions = []config.Direction{{Tag: "proxy-out", Ref: config.RefTemplate}}
	src.DNS.Servers = []corestate.DNSServer{
		{Kind: corestate.DNSServerKindTemplate, Tag: "local_dns_resolver", Enabled: true},
		{Kind: corestate.DNSServerKindTemplate, Tag: "direct_dns_resolver", Enabled: true},
		{Kind: corestate.DNSServerKindTemplate, Tag: "google_udp", Enabled: true},
	}
	src.Vars = []corestate.SettingVar{
		{Name: "dns_final", Value: "google_udp"},
		{Name: "dns_google_udp_outbound", Value: "proxy-out"},
		{Name: "dns_google_udp_dns_ip", Value: "8.8.4.4"},
		{Name: "route_final", Value: "proxy-out"},
	}
	blockAds := corestate.NewInlineRule("block ads",
		map[string]interface{}{"domain_suffix": []interface{}{"ads.example-1.com"}}, "block-out")
	blockAds.Enabled = true
	num := corestate.UserRuleNumStart
	blockAds.Num = &num
	src.Rules = []corestate.Rule{blockAds}

	path := filepath.Join(t.TempDir(), "backup.json")
	if _, err := backup.ExportFile(path, src, backup.ExportOptions{
		AppVersion: "test",
		Directions: build.ResolveDirections(src.Directions, td, target),
		BlockTag:   td.DirectionBlockTag(),
		RecordVars: declsFor(src),
	}); err != nil {
		t.Fatalf("ExportFile: %v", err)
	}

	// Файл: значения — в записи сервера, корневых склеенных имён нет.
	rawFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var onDisk struct {
		Vars map[string]string `json:"vars"`
		DNS  struct {
			Servers []corestate.DNSServer `json:"servers"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(rawFile, &onDisk); err != nil {
		t.Fatalf("parse file: %v", err)
	}
	for name := range onDisk.Vars {
		if strings.HasPrefix(name, "dns_google_") {
			t.Errorf("файл: корневое %s — значение переменной сервера обязано ехать записью", name)
		}
	}
	fileVars := map[string]string(nil)
	for _, srv := range onDisk.DNS.Servers {
		if srv.Kind == corestate.DNSServerKindTemplate && srv.Tag == "google_udp" {
			fileVars = srv.Vars
		}
	}
	if fileVars["outbound"] != "proxy-out" || fileVars["dns_ip"] != "8.8.4.4" || len(fileVars) != 2 {
		t.Errorf("файл: google_udp.vars %v, ожидались outbound=proxy-out и dns_ip=8.8.4.4", fileVars)
	}

	file, _, err := backup.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// checkDNSRoute — собранный сервер google_udp включён, идёт через VPN на
	// выбранный адрес, и склеенных корневых имён в состоянии не осталось.
	checkDNSRoute := func(stage string, s *corestate.State) {
		t.Helper()
		vars := varMap(s)
		if got := vars["dns_final"]; got != "google_udp" {
			t.Errorf("%s: dns_final %q, ожидался google_udp", stage, got)
		}
		for name := range vars {
			if strings.HasPrefix(name, "dns_google_") {
				t.Errorf("%s: корневое %s осталось в состоянии", stage, name)
			}
		}
		resolved := build.ResolveDNS(s, td, wizardtemplate.VarValuesFor(td.Vars, vars, nil, target), target)
		found := false
		for _, srv := range resolved.Servers {
			if srv.Tag != "google_udp" {
				continue
			}
			found = true
			if !srv.Enabled {
				t.Errorf("%s: google_udp выключен — финальный DNS уйдёт на системный резолвер", stage)
			}
			if detour, _ := srv.Body["detour"].(string); detour != "proxy-out" {
				t.Errorf("%s: google_udp.detour %q, ожидался proxy-out — DNS мимо VPN", stage, detour)
			}
			if addr, _ := srv.Body["server"].(string); addr != "8.8.4.4" {
				t.Errorf("%s: google_udp.server %q, ожидался выбранный 8.8.4.4", stage, addr)
			}
		}
		if !found {
			t.Errorf("%s: сервера google_udp нет среди собранных", stage)
		}
	}

	// ── Вход Debug API: POST /backup/import в состояние, которого нет ───
	// (backupImportWith: state.New(); Направлений у приёмника нет — список
	// KnownOutbounds пуст, системные теги едут из шаблона).
	empty := corestate.New()
	opts := backup.ImportOptions{
		BlockTag:   td.DirectionBlockTag(),
		SystemTags: td.SystemOutboundTags(),
		RecordVars: declsFor(empty),
	}
	for _, p := range td.Presets {
		opts.KnownPresets = append(opts.KnownPresets, p.ID)
	}
	if _, err := backup.ImportFile(empty, file, opts); err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	checkDNSRoute("debug API", empty)

	// newWizard — визард новой машины (configurator.go, state.json нет).
	newWizard := func() *WizardPresenter {
		t.Helper()
		m := wizardmodels.NewWizardModel()
		m.TemplateData = td
		m.ExecDir = t.TempDir()
		p := NewWizardPresenter(m, &GUIState{}, nil)
		loaded, parserJSON, _, err := wizardbusiness.LoadConfigFromFile(nil, td)
		if err != nil || !loaded {
			t.Fatalf("LoadConfigFromFile: loaded=%v err=%v", loaded, err)
		}
		var parsed config.ParserConfig
		if err := json.Unmarshal([]byte(parserJSON), &parsed); err != nil {
			t.Fatalf("parser_config: %v", err)
		}
		m.GlobalOutbounds = append([]config.Direction(nil), parsed.ParserConfig.Outbounds...)
		p.InitializeTemplateState()
		wizardbusiness.ApplyWizardDNSTemplate(m)
		wizardbusiness.ApplyDNSVarsFromSettingsToModel(m)
		return p
	}

	// ── Вход UI: визард новой машины ────────────────────────────────────
	p := newWizard()
	if p.HasUnsavedChanges() {
		t.Fatal("предусловие: визард новой машины открылся с несохранёнными правками")
	}
	res, stage, err := p.ImportBackupFile(file, true)
	if err != nil {
		t.Fatalf("ImportBackupFile (шаг %d): %v", stage, err)
	}
	for _, w := range res.Warnings {
		if w.Code == backup.WarnBackupDirectionExists {
			t.Errorf("UI: на новой машине Направление файла вытеснено сидом шаблона: %s", w.Detail)
		}
	}
	saved := p.CreateStateFromModel("", "")
	checkDNSRoute("UI", saved)

	var rule *corestate.Rule
	for i := range saved.Rules {
		if saved.Rules[i].Name == "block ads" {
			rule = &saved.Rules[i]
		}
	}
	if rule == nil {
		t.Fatal("UI: правило block ads потеряно")
	}
	body, err := rule.DecodeBody()
	if err != nil {
		t.Fatalf("UI: DecodeBody: %v", err)
	}
	if ib, ok := body.(*corestate.InlineBody); !ok || ib.Outbound != "block-out" || !rule.Enabled {
		t.Errorf("UI: правило на системный тег шаблона после загрузки %+v (enabled=%v), ожидалась цель block-out", body, rule.Enabled)
	}

	// ── Вход UI в настроенное состояние (Л13) ──────────────────────────
	// Приёмник записан до SPEC 129: свой адрес google_udp (v6) и свой адрес
	// cloudflare_dot, которого файл не касается.
	receiver := corestate.New()
	receiver.Directions = []config.Direction{{Tag: "proxy-out", Ref: config.RefTemplate}}
	receiver.DNS.Servers = []corestate.DNSServer{
		{Kind: corestate.DNSServerKindTemplate, Tag: "local_dns_resolver", Enabled: true},
		{Kind: corestate.DNSServerKindTemplate, Tag: "direct_dns_resolver", Enabled: true},
		{Kind: corestate.DNSServerKindTemplate, Tag: "google_udp", Enabled: true},
		{Kind: corestate.DNSServerKindTemplate, Tag: "cloudflare_dot", Enabled: true},
	}
	receiver.Vars = []corestate.SettingVar{
		{Name: "dns_final", Value: "google_udp"},
		{Name: "dns_google_udp_dns_ip", Value: "2001:4860:4860::8888"},
		{Name: "dns_cloudflare_dot_dns_ip", Value: "1.0.0.1"},
	}
	p = newWizard()
	if err := p.LoadState(receiver); err != nil {
		t.Fatalf("LoadState приёмника: %v", err)
	}
	if got := p.Model().DNSTemplateVars["cloudflare_dot"]["dns_ip"]; got != "1.0.0.1" {
		t.Fatalf("загрузка: cloudflare_dot.dns_ip %q — корневое значение потеряно фильтром сирот (Л1)", got)
	}
	if _, stage, err := p.ImportBackupFile(file, false); err != nil {
		t.Fatalf("ImportBackupFile в настроенное (шаг %d): %v", stage, err)
	}
	merged := p.CreateStateFromModel("", "")
	checkDNSRoute("UI в настроенное", merged)
	for _, srv := range merged.DNS.Servers {
		if srv.Kind == corestate.DNSServerKindTemplate && srv.Tag == "cloudflare_dot" {
			if srv.Vars["dns_ip"] != "1.0.0.1" {
				t.Errorf("UI в настроенное: cloudflare_dot.vars %v — имя, которого файл не называл, затёрто", srv.Vars)
			}
		}
	}
}
