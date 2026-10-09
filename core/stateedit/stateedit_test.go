package stateedit

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// incidentState — состояние с правилами, DNS и Направлением, на которое
// ссылается всё, что удаление сервера обязано почистить или назвать.
func incidentState(t *testing.T) *state.State {
	t.Helper()
	st := state.New()
	st.Directions = []configtypes.Direction{{Tag: "proxy-out", Type: "selector", AddOutbounds: []string{"direct-out", "old-node"}}}
	folder := state.NewFolderSource("Work")
	folder.Nodes = []state.Node{{Kind: state.SourceKindServer, Tag: "f1", Enabled: true,
		Body: json.RawMessage(`{"type":"socks","server":"5.6.7.8","server_port":1080}`), Detour: &state.NodeLink{Tag: "old-node"}}}
	st.Sources = []state.Source{
		state.NewServerSource("old-node", json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)),
		state.NewChainSource("ch", []state.NodeLink{{Tag: "old-node"}}),
		folder,
	}
	n1000, n1001 := 1000, 1001
	st.Rules = []state.Rule{
		{Kind: state.RuleKindInline, Name: "r1", Enabled: true, Num: &n1000, Body: json.RawMessage(`{"domain_suffix":["a.com"],"outbound":"old-node"}`)},
		{Kind: state.RuleKindInline, Name: "r2", Enabled: true, Num: &n1001, Body: json.RawMessage(`{"domain_suffix":["b.com"],"outbound":"proxy-out"}`)},
	}
	st.DNS.Servers = []state.DNSServer{
		{Kind: state.DNSServerKindUser, Tag: "cf", Enabled: true, Body: map[string]interface{}{"type": "udp", "server": "1.1.1.1"}},
		{Kind: state.DNSServerKindUser, Tag: "g", Enabled: true, Body: map[string]interface{}{"type": "udp", "server": "8.8.8.8"}},
	}
	st.DNS.Rules = []state.DNSRule{{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{"domain_suffix": []interface{}{"c.com"}, "server": "cf"}}}
	st.Vars = []state.SettingVar{{Name: "dns_final", Value: "cf"}}
	return st
}

const wgBlock = `[Interface]
PrivateKey = 0GCSi+xv9uacc7rK5S8WmwNlf/eqD/6+I34xw6+iDnU=
Address = 10.2.0.2/32

[Peer]
# NL-1
PublicKey = 0q5TxQQMNVQ6wEcLqnHa20G0DP/fpk8YdgLJUYApfTo=
AllowedIPs = 0.0.0.0/0
Endpoint = 149.22.88.129:51820`

// TestStateEditIncidentScenario — сценарий инцидента 08.10.2026 через
// операции состояния: добавление узла не трогает правила, DNS и Направления;
// удаление узла снимает NodeLink и include, а цели правил только называет;
// правила и DNS-записи добавляются и удаляются по одной.
func TestStateEditIncidentScenario(t *testing.T) {
	st := incidentState(t)
	rulesBefore := append([]state.Rule(nil), st.Rules...)
	dnsBefore := len(st.DNS.Servers)
	dnsRulesBefore := len(st.DNS.Rules)
	dirsBefore := append([]string(nil), st.Directions[0].AddOutbounds...)

	link := "vless://2ee2a715-d541-416a-8713-d66567448c2e@91.98.155.240:443?encryption=none&security=none&type=grpc#old-node"
	input := link + "\nhttps://sub.example/token123\n" + wgBlock
	res, err := AddServers(st, nil, AddServersRequest{Input: input})
	if err != nil {
		t.Fatalf("AddServers: %v", err)
	}
	if len(res.Added) != 2 {
		t.Fatalf("added = %+v, want vless + wg", res.Added)
	}
	if res.Added[0].Tag != "old-node-2" {
		t.Errorf("root tag not uniquified: %q", res.Added[0].Tag)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != SkipSubscriptionURL || res.Skipped[0].Line != 2 {
		t.Errorf("skipped = %+v, want subscription_url at line 2", res.Skipped)
	}
	if !reflect.DeepEqual(st.Rules, rulesBefore) || len(st.DNS.Servers) != dnsBefore || len(st.DNS.Rules) != dnsRulesBefore {
		t.Fatal("adding a server touched rules or DNS")
	}
	if !reflect.DeepEqual(st.Directions[0].AddOutbounds, dirsBefore) {
		t.Fatal("adding a server touched directions")
	}
	again, err := AddServers(st, nil, AddServersRequest{Input: link})
	if err != nil {
		t.Fatalf("AddServers again: %v", err)
	}
	if len(again.Added) != 0 || len(again.Skipped) != 1 || again.Skipped[0].Reason != SkipDuplicate {
		t.Errorf("re-add: %+v, want one duplicate", again)
	}

	del, err := DeleteServer(st, "old-node", "")
	if err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	for i := range st.Sources {
		src := &st.Sources[i]
		if src.Kind == state.SourceKindChain && len(src.Hops) != 0 {
			t.Errorf("chain hop to deleted node kept: %+v", src.Hops)
		}
		if src.Kind == state.SourceKindFolder && src.Nodes[0].Detour != nil {
			t.Error("folder node detour to deleted node kept")
		}
	}
	if len(del.LinksRemoved) != 2 {
		t.Errorf("links removed = %v, want hop + detour", del.LinksRemoved)
	}
	if got := st.Directions[0].AddOutbounds; !reflect.DeepEqual(got, []string{"direct-out"}) {
		t.Errorf("direction include = %v, want [direct-out]", got)
	}
	if len(del.Dangling) != 1 || !strings.Contains(del.Dangling[0], `"r1"`) {
		t.Errorf("dangling = %v, want rule r1", del.Dangling)
	}
	if _, err := DeleteServer(st, "old-node", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v, want ErrNotFound", err)
	}

	var fe *FieldError
	_, err = AddRule(st, nil, state.Rule{Kind: state.RuleKindInline, Name: "bad", Body: json.RawMessage(`{"domain":["x"],"outbound":"nowhere"}`)})
	if !errors.As(err, &fe) || fe.Field != "body.outbound" {
		t.Fatalf("unknown outbound: %v, want FieldError body.outbound", err)
	}
	num, err := AddRule(st, nil, state.Rule{Kind: state.RuleKindInline, Name: "ok", Enabled: true, Body: json.RawMessage(`{"domain":["x"],"outbound":"proxy-out"}`)})
	if err != nil || num != 1002 {
		t.Fatalf("AddRule: num=%d err=%v, want 1002", num, err)
	}
	for i := 1; i < len(st.Rules); i++ {
		if *st.Rules[i-1].Num > *st.Rules[i].Num {
			t.Fatalf("rules not sorted by num: %d before %d", *st.Rules[i-1].Num, *st.Rules[i].Num)
		}
	}
	n1000 := 1000
	removed, err := DeleteRule(st, nil, RuleSelector{Num: &n1000})
	if err != nil || removed.Name != "r1" || len(st.Rules) != 2 {
		t.Fatalf("DeleteRule num=1000: %+v err=%v rules=%d", removed, err, len(st.Rules))
	}

	err = AddDNSServer(st, nil, state.DNSServer{Tag: "cf", Enabled: true, Body: map[string]interface{}{"type": "udp", "server": "9.9.9.9"}})
	if !errors.As(err, &fe) || fe.Field != "tag" {
		t.Fatalf("duplicate dns tag: %v, want FieldError tag", err)
	}
	dres, err := DeleteDNSServer(st, "cf")
	if err != nil {
		t.Fatalf("DeleteDNSServer: %v", err)
	}
	if dres.FinalMovedTo != "g" || len(st.Vars) != 1 || st.Vars[0].Value != "g" {
		t.Errorf("dns_final not moved: res=%+v vars=%+v", dres, st.Vars)
	}
	if len(dres.Dangling) != 1 {
		t.Errorf("dns rule on deleted server not reported: %v", dres.Dangling)
	}
}
