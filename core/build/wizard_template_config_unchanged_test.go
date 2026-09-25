package build

// «config.json на штатном шаблоне не изменился» (SPEC 143, волна 3).
//
// Перевод тел пресетов и шаблонных DNS-серверов на канонический обходчик
// меняет политику unresolved: вместо «фрагмент выпал» — «выпал ключ» плюс
// гейты валидности. На штатном шаблоне это не должно менять ни байта конфига,
// кроме того, что SPEC 143 предписывает прямо. Эталоны сняты с develop ДО
// перевода (волны 0–2 слиты), по наборам разведки RECON §2: таргеты ×
// состояния настроек × все пресеты шаблона включены.
//
// Перегенерация эталонов — только осознанно:
//
//	go test ./core/build -run TestWizardTemplateConfigUnchanged -count=1 -update

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

var updateWizardTemplateGolden = flag.Bool("update", false,
	"перегенерировать testdata/wizard_template_config/*.json")

const wizardTemplateGoldenDir = "testdata/wizard_template_config"

// wizardTemplateCase — один набор: глобальные переменные состояния,
// переменные пресетов по id и правка шаблона перед разбором (nil — штатный).
type wizardTemplateCase struct {
	name       string
	vars       map[string]string
	presetVars map[string]map[string]string
	patch      func(root map[string]interface{})
}

func wizardTemplateCases() []wizardTemplateCase {
	return []wizardTemplateCase{
		{name: "defaults"},
		{name: "tun_off_proxy_in", vars: map[string]string{
			"tun": "false", "enable_proxy_in": "true",
			"proxy_in_auth_enabled": "true", "proxy_in_username": "user",
			"tls_fragment": "true",
		}},
		{name: "ipv6_gateway_lists", vars: map[string]string{
			"ipv6_enabled": "true", "gateway_mode": "true",
			"gateway_include_interface": "en0\nen1",
			"tun_address_exclude":       "10.0.0.0/8\n192.168.0.0/16",
		}},
		{name: "junk_ints", vars: map[string]string{
			"tun_mtu": "abc", "proxy_in_listen_port": "70000", "urltest_tolerance": "-5",
			"enable_proxy_in": "true",
		}},
		{name: "empty_mtu", vars: map[string]string{"tun_mtu": ""}},
		// Выключенные глобали и переменные пресетов (PLAN «Риски»): DNS
		// override снят, sniff/resolve выключены, resolve_strategy пуст.
		{name: "globals_off", vars: map[string]string{"resolve_strategy": ""},
			presetVars: map[string]map[string]string{
				"russian":            {"use_dns_override": "false", "geoip_enabled": "false"},
				"traffic-processing": {"sniff_enabled": "false", "resolve_enabled": "false"},
				"fakeip":             {"force": "false"},
			}},
		{name: "presets_nondefault", presetVars: map[string]map[string]string{
			"traffic-processing": {"hijack_dns_enabled": "false", "sniff_timeout": "300ms"},
			"russian":            {"dns_ip": "77.88.8.1", "out": "proxy-out"},
			"split-all-traffic":  {"out_b": "direct-out"},
		}},
		// Ловушка RECON §2: пустая глобаль, на которую ссылается тело пресета
		// ("strategy": "@resolve_strategy" в traffic-processing). В штатном
		// шаблоне у неё непустое умолчание, поэтому пустоту даёт правка
		// шаблона: умолчание снято.
		{name: "resolve_strategy_unset", patch: func(root map[string]interface{}) {
			vars, _ := root["vars"].([]interface{})
			for _, raw := range vars {
				if v, ok := raw.(map[string]interface{}); ok && v["name"] == "resolve_strategy" {
					delete(v, "default_value")
				}
			}
		}},
	}
}

func wizardTemplateTargets() []template.TargetSpec {
	return []template.TargetSpec{
		{GOOS: "darwin", GOARCH: "arm64", Target: "local"},
		{GOOS: "windows", GOARCH: "amd64", Target: "local"},
		{GOOS: "windows", GOARCH: "386", Target: "local"},
		{GOOS: "linux", GOARCH: "amd64", Target: "remote"},
	}
}

// TestWizardTemplateConfigUnchanged — config.json штатного шаблона побайтно
// равен эталону на каждом наборе.
func TestWizardTemplateConfigUnchanged(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "bin", "wizard_template.json"))
	if err != nil {
		t.Skipf("боевой шаблон недоступен: %v", err)
	}
	if *updateWizardTemplateGolden {
		if err := os.MkdirAll(wizardTemplateGoldenDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range wizardTemplateCases() {
		for _, target := range wizardTemplateTargets() {
			target := target.Normalized()
			name := tc.name + "__" + target.GOOS + "_" + target.GOARCH + "_" + target.TargetOrLocal()
			t.Run(name, func(t *testing.T) {
				res := buildWizardTemplateCase(t, raw, tc, target)
				path := filepath.Join(wizardTemplateGoldenDir, name+".json")
				if *updateWizardTemplateGolden {
					if err := os.WriteFile(path, res.ConfigJSON, 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				for _, w := range res.TemplateWarnings {
					t.Logf("template warning: %s %v", w.Code, w.Params)
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("эталон %s: %v", path, err)
				}
				if bytes.Equal(res.ConfigJSON, want) {
					return
				}
				idx := firstDiffByte(res.ConfigJSON, want)
				t.Errorf("config.json разошёлся с эталоном на байте %d\n%s",
					idx, contextDiff(res.ConfigJSON, want, idx, 160))
			})
		}
	}
}

// wizardTemplateCache — минимальный кэш узлов: цели, на которые ссылаются
// пресеты по умолчанию (proxy-out, «ru VPN 🇷🇺»). Без них чистка висячих
// outbound снимала бы половину правил пресетов, и эталон не видел бы их тел.
func wizardTemplateCache() *ParsedCache {
	return &ParsedCache{Outbounds: []json.RawMessage{
		json.RawMessage(`{"type":"vless","tag":"node-a","server":"198.51.100.1","server_port":443,"uuid":"00000000-0000-0000-0000-000000000001"}`),
		json.RawMessage(`{"type":"selector","tag":"proxy-out","outbounds":["node-a","direct-out"]}`),
		json.RawMessage(`{"type":"selector","tag":"ru VPN 🇷🇺","outbounds":["node-a"]}`),
	}}
}

// buildWizardTemplateCase собирает конфиг по прод-схеме
// core.buildContextFromState: шаблон через ParseTemplateData, все пресеты
// шаблона включены (переменные — из набора, остальное по умолчанию), записи
// DNS синхронизированы с пресетами, кэш узлов — wizardTemplateCache.
func buildWizardTemplateCase(t *testing.T, raw []byte, tc wizardTemplateCase, target template.TargetSpec) Result {
	t.Helper()
	if tc.patch != nil {
		var root map[string]interface{}
		if err := json.Unmarshal(raw, &root); err != nil {
			t.Fatal(err)
		}
		tc.patch(root)
		patched, err := json.Marshal(root)
		if err != nil {
			t.Fatal(err)
		}
		raw = patched
	}
	td, err := template.ParseTemplateData(raw)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}

	// Секреты фиксированы: иначе сборка материализует их заново на каждом
	// прогоне, и эталон не повторится.
	vars := map[string]string{
		"clash_secret":      "golden-secret",
		"proxy_in_password": "golden-password",
	}
	for k, v := range tc.vars {
		vars[k] = v
	}

	rules := make([]corestate.Rule, 0, len(td.Presets))
	presetByID := make(map[string]corestate.PresetLite, len(td.Presets))
	ids := make([]string, 0, len(td.Presets))
	for i := range td.Presets {
		p := &td.Presets[i]
		ids = append(ids, p.ID)
		presetByID[p.ID] = p
		rules = append(rules, presetRule(p.ID, tc.presetVars[p.ID], true))
	}
	if len(ids) < 16 {
		sort.Strings(ids)
		t.Fatalf("пресетов в шаблоне %d (%v), ожидалось не меньше 16", len(ids), ids)
	}
	var dns corestate.DNSOptions
	corestate.SyncDNSOptionsWithActivePresets(rules, &dns, presetByID)

	ctx := BuildContext{
		Template: td,
		Vars:     vars,
		Cache:    wizardTemplateCache(),
		DNS:      DNSConfig{Final: vars["dns_final"], Strategy: vars["dns_strategy"]},
		Target:   target,
		Preset: PresetMergeContext{
			Target:              target,
			Presets:             td.Presets,
			Rules:               rules,
			DNS:                 dns,
			TemplateDNSDefaults: parseGoldenTemplateDNSDefaults(td),
			GlobalVars:          vars,
			TemplateVars:        td.Vars,
			DNSServerVars:       td.DNSServerVars,
		},
	}
	res, err := BuildConfig(ctx)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	return res
}
