package backup

// Импорт на раскладке живого состояния владельца (хвосты 1.6.0): ось порядка
// правил встаёт номерами файла, DNS-дубли ищутся только у приёмника.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"singbox-launcher/core/build"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

func axisInline(name string, num int) state.Rule {
	r := state.NewInlineRule(name,
		map[string]interface{}{"domain_suffix": []interface{}{name + ".example"}}, "direct-out")
	r.Enabled = true
	r.Num = &num
	return r
}

func axisOf(rules []state.Rule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		id := r.Name
		if r.Kind == state.RuleKindPreset {
			id = r.Ref
		}
		num := "-"
		if r.Num != nil {
			num = fmt.Sprint(*r.Num)
		}
		out = append(out, id+"@"+num)
	}
	return out
}

func dnsRulesWith(s *state.State, needle string) int {
	n := 0
	for _, r := range s.DNS.Rules {
		raw, _ := json.Marshal(r.Body)
		if strings.Contains(string(raw), needle) {
			n++
		}
	}
	return n
}

func dnsServersTagged(s *state.State, tag string) int {
	n := 0
	for _, srv := range s.DNS.Servers {
		if srv.Tag == tag {
			n++
		}
	}
	return n
}

// Импорт своего файла в пустое состояние и в состояние с другой осью.
//
// Ось: системная голова (0), якоря шаблона (950, 955) и правило узла (945)
// остаются в зоне ниже 1000, дырка пользовательской зоны не схлопывается,
// перехватчики (1120, 1130) остаются перехватчиками, неразмеченное правило
// встаёт в хвост. Сборка ставит голову и якоря ПЕРЕД route.rules шаблона —
// сплошная нумерация с 1000 уводила их за шаблонные правила, и sniff переставал
// быть первым.
//
// DNS: одинаковые правила и серверы внутри файла ввозятся все (файл — снимок
// состояния), совпавшие с записью приёмника пропускаются.
func TestImportKeepsAxisZonesAndFileDNSDuplicates(t *testing.T) {
	ntc := func() state.DNSRule {
		return state.DNSRule{Kind: state.DNSRuleKindUser, Enabled: true,
			Body: map[string]interface{}{"domain_suffix": []interface{}{"ntc.party"}, "server": "home"}}
	}
	tracker := func() state.DNSRule {
		return state.DNSRule{Kind: state.DNSRuleKindUser, Enabled: true,
			Body: map[string]interface{}{"domain_suffix": []interface{}{"tracker.example"}, "server": "home"}}
	}
	home := func() state.DNSServer {
		return state.DNSServer{Kind: state.DNSServerKindUser, Tag: "home", Enabled: true,
			Body: map[string]interface{}{"type": "udp", "server": "192.0.2.53"}}
	}

	src := stateWithSections(t, "ts-dns", 945)
	unmarked := axisInline("unmarked", 0)
	unmarked.Num = nil
	src.Rules = []state.Rule{
		mkPresetRule("traffic-processing", 0, true),
		mkPresetRule("private-ips", 950, true),
		mkPresetRule("local-lan-domains", 955, true),
		axisInline("mine-a", 1000),
		axisInline("mine-b", 1001),
		axisInline("mine-c", 1003),
		mkPresetRule("russian", 1120, true),
		mkPresetRule("fakeip", 1130, false),
		unmarked,
	}
	src.DNS.Servers = []state.DNSServer{home(), home()}
	src.DNS.Rules = []state.DNSRule{ntc(), ntc(), tracker(), tracker()}

	wantAxis := []string{
		"traffic-processing@0", "private-ips@950", "local-lan-domains@955",
		"mine-a@1000", "mine-b@1001", "mine-c@1003",
		"russian@1120", "fakeip@1130", "unmarked@1131",
	}
	checkAxis := func(stage string, s *state.State) {
		t.Helper()
		if got := axisOf(s.Rules); !equalStrings(got, wantAxis) {
			t.Errorf("%s: ось %v, ожидалась %v", stage, got, wantAxis)
		}
		sec := nodeSectionsOf(t, s)
		if sec == nil || len(sec.Rules) != 1 || sec.Rules[0].Num == nil || *sec.Rules[0].Num != 945 {
			t.Errorf("%s: правило узла ушло со своего места 945: %+v", stage, sec)
		}
	}

	// 1) В пустое состояние: ось и DNS воспроизводятся целиком.
	empty, _ := exportParseImport10(t, src, state.New())
	checkAxis("в пустое", empty)
	if n := dnsRulesWith(empty, "ntc.party"); n != 2 {
		t.Errorf("в пустое: правил ntc.party %d, в файле 2 — одинаковые правила файла схлопнуты", n)
	}
	if n := dnsRulesWith(empty, "tracker.example"); n != 2 {
		t.Errorf("в пустое: правил tracker.example %d, в файле 2", n)
	}
	if n := dnsServersTagged(empty, "home"); n != 2 {
		t.Errorf("в пустое: серверов home %d, в файле 2 — одинаковые серверы файла схлопнуты", n)
	}

	// 2) В состояние со своей осью и своими DNS-записями: правила замещаются
	// файлом, DNS-запись файла, совпавшая с локальной, пропускается — оба
	// экземпляра; несовпавшая пара ввозится парой.
	recv := state.New()
	recv.Rules = []state.Rule{axisInline("local-only", 1000)}
	recv.DNS.Servers = []state.DNSServer{home()}
	recv.DNS.Rules = []state.DNSRule{ntc()}
	exportParseImport10(t, src, recv)
	checkAxis("в непустое", recv)
	if n := dnsRulesWith(recv, "ntc.party"); n != 1 {
		t.Errorf("в непустое: правил ntc.party %d, ожидалось 1 — своё правило приёмника", n)
	}
	if n := dnsRulesWith(recv, "tracker.example"); n != 2 {
		t.Errorf("в непустое: правил tracker.example %d, ожидалось 2", n)
	}
	if n := dnsServersTagged(recv, "home"); n != 1 {
		t.Errorf("в непустое: серверов home %d, ожидался 1 — свой сервер приёмника", n)
	}

	// 3) Новое правило после импорта встаёт перед перехватчиками, как до него.
	if next := state.NextUserRuleNum(recv.Rules); next != 1004 {
		t.Errorf("новое правило после импорта получит %d, ожидалось 1004 (перед перехватчиком 1120)", next)
	}

	// 4) Сборка: голова и якоря ниже 1000 — перед route.rules шаблона.
	num := func(v int) *int { return &v }
	notSortable := false
	presets := []template.Preset{
		{ID: "traffic-processing", Num: num(0), Sortable: &notSortable,
			Rules: []map[string]interface{}{{"inbound": "tun-in", "action": "sniff"}}},
		{ID: "private-ips", Num: num(950),
			Rules: []map[string]interface{}{{"ip_is_private": true, "outbound": "direct-out"}}},
		{ID: "local-lan-domains", Num: num(955),
			Rules: []map[string]interface{}{{"domain_suffix": ".lan", "outbound": "direct-out"}}},
		{ID: "russian", Num: num(1120),
			Rules: []map[string]interface{}{{"domain_suffix": ".ru", "outbound": "direct-out"}}},
		{ID: "fakeip", Num: num(1130),
			Rules: []map[string]interface{}{{"ip_cidr": "198.18.0.0/15", "outbound": "direct-out"}}},
	}
	route, err := build.MergePresetsIntoRoute(
		json.RawMessage(`{"rules":[{"domain_suffix":["template.example"],"outbound":"direct-out"}]}`),
		build.PresetMergeContext{Presets: presets, Rules: recv.Rules})
	if err != nil {
		t.Fatalf("MergePresetsIntoRoute: %v", err)
	}
	var parsed struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	if err := json.Unmarshal(route, &parsed); err != nil {
		t.Fatalf("route: %v", err)
	}
	var got []string
	for _, r := range parsed.Rules {
		raw, _ := json.Marshal(r)
		s := string(raw)
		switch {
		case r["action"] == "sniff":
			got = append(got, "traffic-processing")
		case r["ip_is_private"] == true:
			got = append(got, "private-ips")
		case strings.Contains(s, `".lan"`):
			got = append(got, "local-lan-domains")
		case strings.Contains(s, "template.example"):
			got = append(got, "template")
		case strings.Contains(s, `".ru"`):
			got = append(got, "russian")
		default:
			ds, _ := r["domain_suffix"].([]interface{})
			if len(ds) == 1 {
				got = append(got, strings.TrimSuffix(fmt.Sprint(ds[0]), ".example"))
			} else {
				got = append(got, s)
			}
		}
	}
	want := []string{"traffic-processing", "private-ips", "local-lan-domains", "template",
		"mine-a", "mine-b", "mine-c", "russian", "unmarked"}
	if !equalStrings(got, want) {
		t.Errorf("route.rules после импорта %v, ожидалось %v", got, want)
	}
}
