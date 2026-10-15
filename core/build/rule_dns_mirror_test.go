package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"singbox-launcher/core/backup"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// TestRuleDNSMirror — DNS-зеркало собственного правила (SPEC 172, контракт
// 1.1.130, TEMPLATE_LANG §6.11): поле `rules[].dns` проходит бэкап и диск без
// потерь, а сборка выпускает зеркала по норме — матчер без IP, гейт, пустой
// матчер, глушилка перед зеркалом, место перед слотом пресета с бо́льшим
// номером, выключенный сервер принудительно, удалённый — с предупреждением.
func TestRuleDNSMirror(t *testing.T) {
	t.Run("круг данных", func(t *testing.T) {
		file := []byte(`{
  "lx_backup": 2,
  "exported_by": {"app": "lxbox", "version": "test", "platform": "android"},
  "exported_at": "2026-10-16T00:00:00Z",
  "rules": [
    {"kind": "inline", "name": "corp", "enabled": true, "num": 1000,
     "body": {"domain_suffix": ["corp.example"], "outbound": "direct-out"},
     "dns": {"enabled": true, "serverTag": "cf_doh"}},
    {"kind": "srs", "name": "lists", "enabled": true, "num": 1010,
     "refs": ["https://example.com/lists.srs"], "body": {"outbound": "direct-out"},
     "dns": {"enabled": false, "serverTag": "google_udp", "forceIpv4": true}}
  ]
}`)
		f, _, err := backup.Parse(file)
		if err != nil {
			t.Fatal(err)
		}
		s := &state.State{}
		if _, err := backup.ImportFile(s, f, backup.ImportOptions{}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "state.json")
		if err := s.Save(path); err != nil {
			t.Fatal(err)
		}
		loaded, err := state.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]*state.RuleDNS{
			"corp":  {Enabled: true, ServerTag: "cf_doh"},
			"lists": {ServerTag: "google_udp", ForceIPv4: true},
		}
		for _, r := range loaded.Rules {
			if !reflect.DeepEqual(r.DNS, want[r.Name]) {
				t.Errorf("state.json: %s.dns = %+v, want %+v", r.Name, r.DNS, want[r.Name])
			}
		}
		for _, cr := range loaded.CustomRules {
			if !reflect.DeepEqual(cr.DNS, want[cr.Label]) {
				t.Errorf("вид Конфигуратора: %s.dns = %+v, want %+v", cr.Label, cr.DNS, want[cr.Label])
			}
		}
		out, _, err := backup.Export10(loaded, backup.ExportOptions{})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(out.Rules)
		var rules []struct {
			Name string          `json:"name"`
			DNS  json.RawMessage `json:"dns"`
		}
		_ = json.Unmarshal(raw, &rules)
		wantJSON := map[string]string{
			"corp":  `{"enabled":true,"serverTag":"cf_doh"}`,
			"lists": `{"enabled":false,"serverTag":"google_udp","forceIpv4":true}`,
		}
		for _, r := range rules {
			if string(r.DNS) != wantJSON[r.Name] {
				t.Errorf("экспорт: %s.dns = %s, want %s", r.Name, r.DNS, wantJSON[r.Name])
			}
		}
	})

	t.Run("сборка", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "bin", "wizard_template.json"))
		if err != nil {
			t.Skipf("боевой шаблон недоступен: %v", err)
		}
		td, err := template.ParseTemplateData(raw)
		if err != nil {
			t.Fatalf("шаблон не загрузился: %v", err)
		}
		if n := template.RuleOrderSpecs(td.Presets)["fakeip"].Num; n != 1130 {
			t.Fatalf("ось шаблона изменилась: fakeip=%d", n)
		}

		num := func(n int) *int { return &n }
		rule := func(r state.Rule, n int, d *state.RuleDNS) state.Rule {
			r.Enabled, r.Num, r.DNS = true, num(n), d
			return r
		}
		corp := rule(state.NewInlineRule("corp", map[string]interface{}{
			"domain_suffix": []interface{}{"corp.example"}, "ip_cidr": []interface{}{"10.0.0.0/8"},
		}, "direct-out"), 1000, &state.RuleDNS{Enabled: true, ServerTag: "cf_doh"})
		lists := rule(state.NewSrsRule("lists", []string{"https://example.com/lists.srs"}, "direct-out"), 1010,
			&state.RuleDNS{Enabled: true, ServerTag: "local_dns_resolver", ForceIPv4: true})
		ports := rule(state.NewInlineRule("ports", map[string]interface{}{
			"domain_suffix": []interface{}{"ports.example"}, "port": []interface{}{443},
		}, "direct-out"), 1020, &state.RuleDNS{Enabled: true, ServerTag: "local_dns_resolver"})
		ipOnly := rule(state.NewInlineRule("ip-only", map[string]interface{}{
			"ip_cidr": []interface{}{"192.0.2.0/24"},
		}, "direct-out"), 1030, &state.RuleDNS{Enabled: true, ServerTag: "local_dns_resolver", ForceIPv4: true})
		gone := rule(state.NewSrsRule("gone", []string{"https://example.com/gone.srs"}, "direct-out"), 1200,
			&state.RuleDNS{Enabled: true, ServerTag: "deleted_server"})
		fakeip := presetRule("fakeip", nil, true)
		fakeip.Num = num(1130)

		var warns []template.TemplateWarning
		ctx := PresetMergeContext{
			Presets: td.Presets,
			Rules:   []state.Rule{corp, lists, ports, ipOnly, fakeip, gone},
			DNS: state.DNSOptions{
				Servers: []state.DNSServer{{Kind: state.DNSServerKindUser, Tag: "cf_doh", Enabled: false,
					Body: map[string]interface{}{"type": "https", "server": "1.1.1.1"}}},
				Rules: []state.DNSRule{
					{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{"domain_suffix": "home.lan", "server": "local_dns_resolver"}},
					{Kind: state.DNSRuleKindPreset, Ref: "fakeip", Enabled: true},
				},
			},
			Target: template.LocalTarget(),
			SrsCachedPaths: map[string][]string{
				state.StableRuleID(lists): {"/tmp/lists.srs"},
				state.StableRuleID(gone):  {"/tmp/gone.srs"},
			},
			templateWarnings: &warns,
		}
		dns, err := MergePresetsIntoDNS(json.RawMessage(`{"servers":[{"tag":"local_dns_resolver","type":"local"}]}`), ctx)
		if err != nil {
			t.Fatal(err)
		}
		var d struct {
			Servers []map[string]interface{} `json:"servers"`
			Rules   []json.RawMessage        `json:"rules"`
		}
		if err := json.Unmarshal(dns, &d); err != nil {
			t.Fatal(err)
		}

		// Первые четыре — правило пользователя и пары зеркал перед слотом
		// fakeip (1130): corp без ip_cidr, у lists глушилка раньше зеркала.
		// ports (гейт), ip-only (пустой матчер) и gone (сервер удалён) не
		// выпускаются — после fakeip чужих правил нет.
		wantHead := []string{
			`{"domain_suffix":"home.lan","server":"local_dns_resolver"}`,
			`{"domain_suffix":["corp.example"],"server":"cf_doh"}`,
			`{"action":"predefined","ip_version":6,"rcode":"NOERROR","rule_set":"user:lists"}`,
			`{"rule_set":"user:lists","server":"local_dns_resolver"}`,
		}
		if len(d.Rules) <= len(wantHead) {
			t.Fatalf("dns.rules = %s", dns)
		}
		for i, w := range wantHead {
			if got := canonJSON(t, d.Rules[i]); got != w {
				t.Errorf("dns.rules[%d] = %s, want %s", i, got, w)
			}
		}
		for _, r := range d.Rules[len(wantHead):] {
			var m map[string]interface{}
			_ = json.Unmarshal(r, &m)
			if m["query_type"] == nil {
				t.Errorf("после зеркал ожидались только правила fakeip, есть %s", r)
			}
		}

		forced := false
		for _, s := range d.Servers {
			if s["tag"] == "cf_doh" {
				forced = true
			}
		}
		if !forced {
			t.Errorf("выключенный cf_doh, на который ссылается зеркало, не включён в dns.servers")
		}

		dropped := false
		for _, w := range warns {
			if w.Code == WarnTemplateFragmentDropped && w.Params["owner"] == "gone" && w.Params["reason"] == "server" {
				dropped = true
			}
		}
		if !dropped {
			t.Errorf("нет предупреждения о зеркале gone на удалённый сервер: %+v", warns)
		}
	})
}

func canonJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
