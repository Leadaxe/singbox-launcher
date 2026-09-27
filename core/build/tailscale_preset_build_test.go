package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// LxBox §578: пресет tailscale из боевого шаблона даёт каждому узлу Tailscale
// в конфиге маршрут и MagicDNS через preferred_by; узел со skip_presets и
// узел, не попавший в кэш сборки, не обслуживаются; постоянных подсетей
// tailnet в выводе нет.
func TestTailscalePresetBuild_TwoNodes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "bin", "wizard_template.json"))
	if err != nil {
		t.Skipf("боевой шаблон недоступен: %v", err)
	}
	td, err := template.ParseTemplateData(raw)
	if err != nil {
		t.Fatalf("шаблон не загрузился: %v", err)
	}
	var ts *template.Preset
	for i := range td.Presets {
		if td.Presets[i].ID == "tailscale" {
			ts = &td.Presets[i]
		}
	}
	if ts == nil || ts.ForEach == nil || len(ts.ForEachDNSServers) != 1 {
		t.Fatalf("пресет tailscale не загружен как for_each: %+v", ts)
	}

	cache := &ParsedCache{
		Endpoints: []json.RawMessage{
			json.RawMessage(`{"type":"tailscale","tag":"home-ts","hostname":"a"}`),
			json.RawMessage(`{"type":"wireguard","tag":"wg"}`),
			json.RawMessage(`{"type":"tailscale","tag":"skip-ts"}`),
			json.RawMessage(`{"type":"tailscale","tag":"work-ts"}`),
		},
		SkipPresets: map[string]bool{"skip-ts": true},
	}
	ctx := PresetMergeContext{
		Presets:     []template.Preset{*ts},
		Rules:       []state.Rule{presetRule("tailscale", nil, true)},
		Target:      template.LocalTarget(),
		PresetNodes: collectPresetNodes(cache, []string{"endpoints", "outbounds"}),
	}
	route, err := MergePresetsIntoRoute(json.RawMessage(`{"rules":[]}`), ctx)
	if err != nil {
		t.Fatal(err)
	}
	dns, err := MergePresetsIntoDNS(json.RawMessage(`{"servers":[]}`), ctx)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	var d struct {
		Servers []map[string]interface{} `json:"servers"`
		Rules   []map[string]interface{} `json:"rules"`
	}
	_ = json.Unmarshal(route, &r)
	_ = json.Unmarshal(dns, &d)

	wantRoute := []string{
		`{"action":"resolve","preferred_by":["home-ts"],"server":"home-ts-dns"}`,
		`{"outbound":"home-ts","preferred_by":["home-ts"]}`,
		`{"action":"resolve","preferred_by":["work-ts"],"server":"work-ts-dns"}`,
		`{"outbound":"work-ts","preferred_by":["work-ts"]}`,
	}
	if len(r.Rules) != len(wantRoute) {
		t.Fatalf("route.rules = %s", route)
	}
	for i, w := range wantRoute {
		if got, _ := json.Marshal(r.Rules[i]); string(got) != w {
			t.Errorf("route.rules[%d] = %s, want %s", i, got, w)
		}
	}
	wantServers := []string{
		`{"endpoint":"home-ts","tag":"home-ts-dns","type":"tailscale"}`,
		`{"endpoint":"work-ts","tag":"work-ts-dns","type":"tailscale"}`,
	}
	if len(d.Servers) != 2 {
		t.Fatalf("dns.servers = %s", dns)
	}
	for i, w := range wantServers {
		if got, _ := json.Marshal(d.Servers[i]); string(got) != w {
			t.Errorf("dns.servers[%d] = %s, want %s", i, got, w)
		}
	}
	if len(d.Rules) != 2 {
		t.Fatalf("dns.rules = %s", dns)
	}
	if got, _ := json.Marshal(d.Rules[1]); string(got) != `{"preferred_by":["work-ts"],"server":"work-ts-dns"}` {
		t.Errorf("dns.rules[1] = %s", got)
	}
	all := string(route) + string(dns)
	for _, bad := range []string{"100.64.0.0/10", "fd7a:115c:a1e0", "ts.net", "skip-ts", "tailscale:"} {
		if strings.Contains(all, bad) {
			t.Errorf("вывод содержит %q: %s", bad, all)
		}
	}
}
