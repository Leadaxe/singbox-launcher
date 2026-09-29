package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muhammadmuzzammil1998/jsonc"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// Набор, не попавший в конфиг (remote .srs не скачан), снимает правило
// целиком, что бы в нём ни осталось, — иначе соседние поля (`network`,
// `server`) расширяли бы правило до всего трафика (SPEC 153). Частично
// уцелевший список наборов правило сохраняет. Route и DNS, пресеты и
// пользовательские правила; выпадение — template_fragment_dropped.
func TestDanglingRuleSetDropsRule(t *testing.T) {
	t.Run("wizard_template", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "bin", "wizard_template.json"))
		if err != nil {
			t.Skipf("боевой шаблон недоступен: %v", err)
		}
		// Кэша .srs нет: remote-наборы games и ru-blocked пропускаются.
		target := template.TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: "local"}.Normalized()
		res := buildWizardTemplateCase(t, raw, wizardTemplateCase{name: "defaults"}, target)

		var cfg struct {
			Route struct {
				Rules []map[string]interface{} `json:"rules"`
			} `json:"route"`
		}
		if err := json.Unmarshal(jsonc.ToJSON(res.ConfigJSON), &cfg); err != nil {
			t.Fatalf("config: %v", err)
		}
		for _, r := range cfg.Route.Rules {
			_, hasOut := r["outbound"]
			if hasOut && len(r) == 2 && r["network"] != nil {
				t.Errorf("правило на весь TCP/UDP осталось в конфиге: %v", r)
			}
		}
		for _, id := range []string{"games", "ru-blocked"} {
			if !hasFragmentDropped(res.TemplateWarnings, id, fragmentKindRule) {
				t.Errorf("нет template_fragment_dropped для %s: %v", id, res.TemplateWarnings)
			}
		}
	})

	t.Run("merge", func(t *testing.T) {
		raw := []byte(`{
			"id": "p",
			"label": "P",
			"rule_set": [
				{"tag": "inl", "type": "inline", "rules": [{"domain_suffix": ["a.example"]}]},
				{"tag": "rem", "type": "remote", "format": "binary", "url": "https://example.com/rem.srs"}
			],
			"rules": [
				{"rule_set": "rem", "network": ["tcp", "udp"], "outbound": "direct-out"},
				{"rule_set": ["inl", "rem"], "network": ["tcp", "udp"], "outbound": "direct-out"}
			],
			"dns_rules": [
				{"rule_set": "rem", "server": "remote"},
				{"rule_set": ["inl", "rem"], "server": "remote"}
			]
		}`)
		var p template.Preset
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var warns []template.TemplateWarning
		ctx := PresetMergeContext{
			Presets: []template.Preset{p},
			Rules:   []corestate.Rule{presetRule("p", nil, true)},
			DNS: corestate.DNSOptions{Rules: []corestate.DNSRule{
				{Kind: corestate.DNSRuleKindPreset, Ref: "p", Enabled: true},
				{Kind: corestate.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{
					"rule_set": "x", "server": "y",
				}},
				{Kind: corestate.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{
					"rule_set": []interface{}{"p:inl", "x"}, "server": "y",
				}},
			}},
			Target:           template.TargetSpec{GOOS: "darwin", GOARCH: "arm64"}.Normalized(),
			templateWarnings: &warns,
		}

		route, err := MergePresetsIntoRoute(json.RawMessage(`{}`), ctx)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		var r struct {
			Rules []interface{} `json:"rules"`
		}
		_ = json.Unmarshal(route, &r)
		gotRoute, _ := json.Marshal(r.Rules)
		wantRoute := `[{"network":["tcp","udp"],"outbound":"direct-out","rule_set":["p:inl"]}]`
		if string(gotRoute) != wantRoute {
			t.Errorf("route.rules:\n got  %s\n want %s", gotRoute, wantRoute)
		}

		dnsRaw := json.RawMessage(`{"servers": [
			{"tag": "remote", "type": "udp", "server": "192.0.2.1"},
			{"tag": "y", "type": "udp", "server": "192.0.2.2"}
		]}`)
		dns, err := MergePresetsIntoDNS(dnsRaw, ctx)
		if err != nil {
			t.Fatalf("dns: %v", err)
		}
		var d struct {
			Rules []interface{} `json:"rules"`
		}
		_ = json.Unmarshal(dns, &d)
		gotDNS, _ := json.Marshal(d.Rules)
		wantDNS := `[{"rule_set":["p:inl"],"server":"remote"},{"rule_set":["p:inl"],"server":"y"}]`
		if string(gotDNS) != wantDNS {
			t.Errorf("dns.rules:\n got  %s\n want %s", gotDNS, wantDNS)
		}

		if !hasFragmentDropped(warns, "p", fragmentKindRule) {
			t.Errorf("нет template_fragment_dropped для route.rules пресета: %v", warns)
		}
		if !hasFragmentDropped(warns, "p", fragmentKindDNSRule) {
			t.Errorf("нет template_fragment_dropped для dns.rules пресета: %v", warns)
		}
		if !hasFragmentDropped(warns, "dns_options", fragmentKindDNSRule) {
			t.Errorf("нет template_fragment_dropped для пользовательского DNS-правила: %v", warns)
		}
	})
}

func hasFragmentDropped(ws []template.TemplateWarning, owner, kind string) bool {
	for _, w := range ws {
		if w.Code == WarnTemplateFragmentDropped && w.Params["owner"] == owner &&
			w.Params["kind"] == kind && strings.Contains(w.Params["reason"], "rule_set") {
			return true
		}
	}
	return false
}
