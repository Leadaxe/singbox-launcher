package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
	if len(st.LatePresetsSeeded) != 2 || st.LatePresetsSeeded[1] != "tunnel-ingress" {
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
	if len(gotRefs) != 3 || gotRefs[0] != "user" || gotRefs[1] != "tunnel-ingress" || gotRefs[2] != "fakeip" {
		t.Fatalf("dns.rules состояния после синхронизации = %v, want [user tunnel-ingress fakeip]", gotRefs)
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
