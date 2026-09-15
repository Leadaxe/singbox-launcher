package presentation

// Восстановление настроек на новой машине не уводит DNS мимо VPN (релиз 1.6.0).
//
// Дефект на копии живых данных: после импорта бэкапа в пустое состояние
// финальный DNS google_udp, настроенный через proxy-out, собирался без detour —
// DNS шёл напрямую. Причины две, по входу на каждую:
//
//   - оба входа: канал запроса шаблонного DNS-сервера живёт в переменной
//     `dns_google_udp_outbound`, а её не было в списке переносимых — в файл
//     она не ехала (registry/vars.json, core/backup/portable_vars.go);
//   - UI-вход: визард новой машины — сид шаблона, и файл сливался в него.
//     DNS-серверы шаблона уже стояли выключенными умолчаниями, «своё
//     сильнее» оставляло их такими, и финальный DNS заменялся системным
//     резолвером; Направления шаблона вытесняли Направления файла.
//
// Заодно UI-вход обязан сохранить цель правила на системный тег шаблона: сброс
// «осиротевших» целей при загрузке переводил block-out на direct-out.

import (
	"encoding/json"
	"path/filepath"
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

	// ── Машина-источник: финальный DNS google_udp через proxy-out ──────
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
	}); err != nil {
		t.Fatalf("ExportFile: %v", err)
	}
	file, _, err := backup.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	varValue := func(s *corestate.State, name string) string {
		for _, v := range s.Vars {
			if v.Name == name {
				return v.Value
			}
		}
		return ""
	}
	// checkDNSRoute — собранный сервер google_udp включён и идёт через VPN.
	checkDNSRoute := func(stage string, s *corestate.State) {
		t.Helper()
		if got := varValue(s, "dns_final"); got != "google_udp" {
			t.Errorf("%s: dns_final %q, ожидался google_udp", stage, got)
		}
		vars := map[string]string{}
		for _, v := range s.Vars {
			vars[v.Name] = v.Value
		}
		resolved := build.ResolveDNS(s, td, vars, target)
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
		}
		if !found {
			t.Errorf("%s: сервера google_udp нет среди собранных", stage)
		}
	}

	// ── Вход Debug API: POST /backup/import в состояние, которого нет ───
	// (backupImportWith: state.New(); Направлений у приёмника нет — список
	// KnownOutbounds пуст, системные теги едут из шаблона).
	opts := backup.ImportOptions{BlockTag: td.DirectionBlockTag(), SystemTags: td.SystemOutboundTags()}
	for _, p := range td.Presets {
		opts.KnownPresets = append(opts.KnownPresets, p.ID)
	}
	empty := corestate.New()
	if _, err := backup.ImportFile(empty, file, opts); err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	checkDNSRoute("debug API", empty)

	// ── Вход UI: визард новой машины (configurator.go, state.json нет) ───
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
}
