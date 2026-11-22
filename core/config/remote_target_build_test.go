// SPEC 186: сборка конфига для чужой машины берёт входы от ЦЕЛИ (её ОС, её
// переменные Settings), а не от компьютера, на котором запущен лаунчер. Тест
// не зависит от ОС запуска: обе цели задаются явно.
package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/paths"
)

func TestRemoteTargetBuildUsesTargetInputs(t *testing.T) {
	linux := template.TargetSpec{GOOS: "linux", GOARCH: "arm64", Target: template.TargetRemote, MachineID: "router"}.Normalized()

	// ── 1. Гейт узла: kTLS (платформа linux в реестре) остаётся у цели linux
	// и снимается у цели darwin — при любой ОС запуска.
	body := json.RawMessage(`{"type":"vless","server":"a.example.com","server_port":443,` +
		`"uuid":"11111111-2222-4333-8444-555555555555",` +
		`"tls":{"enabled":true,"server_name":"a.example.com","kernel_tx":true}}`)
	emit := func(goos string) string {
		t.Helper()
		pc := &ParserConfig{}
		pc.ParserConfig.Version = ParserConfigVersion
		pc.ParserConfig.Proxies = []ProxySource{{
			ID:    "src",
			Label: "n1",
			Canonical: &configtypes.CanonicalSource{Nodes: []configtypes.CanonicalNode{{
				Kind:       "server",
				Tag:        "n1",
				Enabled:    true,
				Body:       body,
				OriginKind: "uri",
				OriginRaw:  "vless://11111111-2222-4333-8444-555555555555@a.example.com:443?security=tls&sni=a.example.com#n1",
			}}},
		}}
		pc.BuildTarget = &BuildTarget{GOOS: goos}
		res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, DirectionBuildOptions{})
		if err != nil {
			t.Fatalf("GenerateOutboundsFromParserConfig(%s): %v", goos, err)
		}
		return strings.Join(res.OutboundsJSON, "\n")
	}
	if out := emit("linux"); !strings.Contains(out, `"kernel_tx"`) {
		t.Errorf("target linux: tls.kernel_tx dropped from the node:\n%s", out)
	}
	if out := emit("darwin"); strings.Contains(out, `"kernel_tx"`) {
		t.Errorf("target darwin: tls.kernel_tx kept in the node:\n%s", out)
	}

	// ── 2. @vars: значение профиля машины, а не локального state.json.
	dataDir := newTestLayout(t)
	writeTemplateFile(t, dataDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "5m"},
			map[string]interface{}{"name": "urltest_tolerance", "type": "int", "default_value": "100"},
		},
	})
	writeStateFile(t, dataDir, map[string]interface{}{
		"vars": []interface{}{map[string]string{"name": "urltest_interval", "value": "1m"}},
	})
	layout := paths.Layout{Data: paths.DataDir(dataDir)}
	machine := &BuildTarget{GOOS: linux.GOOS, SettingsVars: map[string]string{"urltest_interval": "7m", "urltest_tolerance": ""}}
	pc := &ParserConfig{BuildTarget: machine}
	pc.ParserConfig.Outbounds = []Direction{{
		Tag:     "auto",
		Options: map[string]interface{}{"interval": "@urltest_interval", "tolerance": "@urltest_tolerance"},
	}}
	SubstituteParserConfigPlaceholders(pc, BuildVarSubstituterFor(layout, pc.BuildTarget.SettingsVars))
	opts := pc.ParserConfig.Outbounds[0].Options
	if opts["interval"] != "7m" {
		t.Errorf("interval = %v, want the machine's own 7m (local state.json says 1m)", opts["interval"])
	}
	if opts["tolerance"] != 100 {
		t.Errorf("tolerance = %v (%T), want template default int 100 for an empty machine value", opts["tolerance"], opts["tolerance"])
	}

	// ── 3. Пресеты: отбор по ОС цели, а не лаунчера.
	raw, err := os.ReadFile("../../bin/wizard_template.json")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("parse template: %v", err)
	}
	var presets []map[string]interface{}
	if err := json.Unmarshal(root["presets"], &presets); err != nil || len(presets) == 0 {
		t.Fatalf("template presets: %v (n=%d)", err, len(presets))
	}
	clone := func(id string, platforms ...string) map[string]interface{} {
		cp := make(map[string]interface{}, len(presets[0])+2)
		for k, v := range presets[0] {
			cp[k] = v
		}
		cp["id"] = id
		cp["platforms"] = platforms
		return cp
	}
	presets = append(presets, clone("spec186_linux_only", "linux"), clone("spec186_darwin_only", "darwin"))
	root["presets"], _ = json.Marshal(presets)
	patched, _ := json.Marshal(root)
	td, err := template.ParseTemplateData(patched)
	if err != nil {
		t.Fatalf("ParseTemplateData: %v", err)
	}
	has := func(list []template.Preset, id string) bool {
		for i := range list {
			if list[i].ID == id {
				return true
			}
		}
		return false
	}
	forLinux := td.ForTarget(linux).Presets
	if !has(forLinux, "spec186_linux_only") {
		t.Errorf("target linux: linux-only preset filtered out")
	}
	if has(forLinux, "spec186_darwin_only") {
		t.Errorf("target linux: darwin-only preset kept")
	}
}
