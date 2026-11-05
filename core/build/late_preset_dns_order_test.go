package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// SPEC 171 (D-135): DNS-слот пресета встаёт по оси маршрутных правил, а не в
// хвост dns.rules. Сценарий существующего пользователя: включён `fakeip`
// (1130, его правило `A/AAAA → fakeip` ловит всё), своё DNS-правило стоит
// первым, `tailscale` уже засеян; поздний дефолт `tunnel-ingress` (940)
// появляется и в состоянии, и в сборке до первого сохранения — и его
// DNS-reject закрытых пиров обязан стоять раньше fakeip, а правило
// пользователя — остаться первым.
func TestLatePresetDNSRuleTakesAxisPosition(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "bin", "wizard_template.json"))
	if err != nil {
		t.Skipf("боевой шаблон недоступен: %v", err)
	}
	td, err := template.ParseTemplateData(raw)
	if err != nil {
		t.Fatalf("шаблон не загрузился: %v", err)
	}
	specs := template.RuleOrderSpecs(td.Presets)
	if specs["tunnel-ingress"].Num != 940 || specs["fakeip"].Num != 1130 {
		t.Fatalf("ось шаблона изменилась: tunnel-ingress=%+v fakeip=%+v", specs["tunnel-ingress"], specs["fakeip"])
	}

	userRule := state.DNSRule{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{
		"domain_suffix": "home.lan", "server": "local_dns_resolver",
	}}
	st := &state.State{
		Rules:             []state.Rule{presetRule("fakeip", nil, true)},
		DNS:               state.DNSOptions{Rules: []state.DNSRule{userRule, {Kind: state.DNSRuleKindPreset, Ref: "fakeip", Enabled: true}}},
		LatePresetsSeeded: []string{"tailscale"},
	}
	st.Rules, st.LatePresetsSeeded = state.SeedLateDefaultRules(st.Rules, specs, st.LatePresetsSeeded)
	st.Rules = state.NormalizeRuleOrder(st.Rules, specs)
	if len(st.LatePresetsSeeded) != 3 || st.LatePresetsSeeded[1] != "tunnel-ingress" || st.LatePresetsSeeded[2] != "tunnel-routes" {
		t.Fatalf("отметки засева: %v", st.LatePresetsSeeded)
	}

	// (а) Состояние: синхронизация ставит новый слот между правилом
	// пользователя и fakeip, не трогая их.
	synced := state.DNSOptions{Rules: append([]state.DNSRule(nil), st.DNS.Rules...)}
	state.SyncDNSOptionsWithActivePresets(st.Rules, &synced, template.PresetLiteMap(td.Presets))
	gotRefs := make([]string, 0, len(synced.Rules))
	for _, r := range synced.Rules {
		if r.Kind == state.DNSRuleKindUser {
			gotRefs = append(gotRefs, "user")
		} else {
			gotRefs = append(gotRefs, r.Ref)
		}
	}
	// `tunnel-routes` (943, SPEC 180) — второй поздний дефолт с DNS-правилом:
	// его слот встаёт по той же оси, после `tunnel-ingress` и раньше fakeip.
	if len(gotRefs) != 4 || gotRefs[0] != "user" || gotRefs[1] != "tunnel-ingress" || gotRefs[2] != "tunnel-routes" || gotRefs[3] != "fakeip" {
		t.Fatalf("dns.rules состояния после синхронизации = %v, want [user tunnel-ingress tunnel-routes fakeip]", gotRefs)
	}

	// (б) Сборка на состоянии БЕЗ записи tunnel-ingress в dns.rules (до
	// первого сохранения визардом): reject закрытого пира раньше fakeip.
	cache := &ParsedCache{Endpoints: []json.RawMessage{
		json.RawMessage(`{"type":"wireguard","tag":"wg-home"}`),
	}}
	ctx := PresetMergeContext{
		Presets:     td.Presets,
		Rules:       st.Rules,
		DNS:         st.DNS,
		Target:      template.LocalTarget(),
		PresetNodes: CollectPresetNodes(cache, []string{"endpoints", "outbounds"}),
	}
	dns, err := MergePresetsIntoDNS(json.RawMessage(`{"servers":[{"tag":"local_dns_resolver","type":"local"}]}`), ctx)
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	_ = json.Unmarshal(dns, &d)
	userAt, rejectAt, fakeipAt := -1, -1, -1
	for i, r := range d.Rules {
		switch {
		case r["domain_suffix"] == "home.lan":
			userAt = i
		case r["action"] == "reject" && r["inbound"] != nil:
			rejectAt = i
		case r["query_type"] != nil && r["server"] != nil:
			if qt, _ := r["query_type"].([]interface{}); len(qt) == 2 && qt[0] == "A" {
				fakeipAt = i
			}
		}
	}
	if userAt < 0 || rejectAt < 0 || fakeipAt < 0 {
		t.Fatalf("в dns.rules нет ожидаемых правил (user=%d reject=%d fakeip=%d): %s", userAt, rejectAt, fakeipAt, dns)
	}
	if !(userAt < rejectAt && rejectAt < fakeipAt) {
		t.Fatalf("порядок dns.rules: user=%d reject=%d fakeip=%d, want user < reject < fakeip: %s", userAt, rejectAt, fakeipAt, dns)
	}
	if got, _ := json.Marshal(d.Rules[rejectAt]["inbound"]); string(got) != `["wg-home"]` {
		t.Errorf("reject inbound = %s, want [\"wg-home\"]", got)
	}
}

// SPEC 180 (контракт 1.1.161, D-144): пресет `tunnel-routes` на боевом
// шаблоне. Узел openvpn-client с полем записи own_routes получает DNS-сервер
// типа `openvpn` и DNS-правило `preferred_by` — раньше `A/AAAA → fakeip`,
// иначе присланные сервером домены получили бы поддельный адрес. Узел без
// поля пресет не обслуживает.
func TestTunnelRoutesDNSRuleBeforeFakeIP(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "bin", "wizard_template.json"))
	if err != nil {
		t.Skipf("боевой шаблон недоступен: %v", err)
	}
	td, err := template.ParseTemplateData(raw)
	if err != nil {
		t.Fatalf("шаблон не загрузился: %v", err)
	}
	specs := template.RuleOrderSpecs(td.Presets)
	if specs["tunnel-routes"].Num != 943 || !specs["tunnel-routes"].DefaultEnabled {
		t.Fatalf("пресет tunnel-routes в шаблоне: %+v", specs["tunnel-routes"])
	}
	st := &state.State{
		Rules: []state.Rule{presetRule("fakeip", nil, true)},
		DNS:   state.DNSOptions{Rules: []state.DNSRule{{Kind: state.DNSRuleKindPreset, Ref: "fakeip", Enabled: true}}},
	}
	st.Rules, _ = state.SeedLateDefaultRules(st.Rules, specs, nil)
	st.Rules = state.NormalizeRuleOrder(st.Rules, specs)

	cache := &ParsedCache{
		Endpoints: []json.RawMessage{
			json.RawMessage(`{"type":"openvpn-client","tag":"office-ovpn"}`),
			json.RawMessage(`{"type":"openvpn-client","tag":"plain-ovpn"}`),
		},
		RecordFlags: map[string]configtypes.NodeRecordFlags{"office-ovpn": {OwnRoutes: true}},
	}
	ctx := PresetMergeContext{
		Presets:     td.Presets,
		Rules:       st.Rules,
		DNS:         st.DNS,
		Target:      template.LocalTarget(),
		PresetNodes: CollectPresetNodes(cache, []string{"endpoints", "outbounds"}),
	}
	dns, err := MergePresetsIntoDNS(json.RawMessage(`{"servers":[{"tag":"local_dns_resolver","type":"local"}]}`), ctx)
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Servers []map[string]interface{} `json:"servers"`
		Rules   []map[string]interface{} `json:"rules"`
	}
	_ = json.Unmarshal(dns, &d)
	server := false
	for _, s := range d.Servers {
		if s["tag"] == "plain-ovpn-dns" {
			t.Errorf("узел без own_routes получил DNS-сервер: %s", dns)
		}
		if s["tag"] == "office-ovpn-dns" {
			server = s["type"] == "openvpn" && s["endpoint"] == "office-ovpn"
		}
	}
	if !server {
		t.Fatalf("нет DNS-сервера {type: openvpn, tag: office-ovpn-dns, endpoint: office-ovpn}: %s", dns)
	}
	ownAt, fakeipAt := -1, -1
	for i, r := range d.Rules {
		if pb, _ := json.Marshal(r["preferred_by"]); string(pb) == `["office-ovpn-dns"]` && r["server"] == "office-ovpn-dns" {
			ownAt = i
		}
		if qt, _ := r["query_type"].([]interface{}); len(qt) == 2 && qt[0] == "A" && r["server"] != nil {
			fakeipAt = i
		}
	}
	if ownAt < 0 || fakeipAt < 0 || ownAt > fakeipAt {
		t.Fatalf("порядок dns.rules: tunnel-routes=%d fakeip=%d, want tunnel-routes < fakeip: %s", ownAt, fakeipAt, dns)
	}
}
