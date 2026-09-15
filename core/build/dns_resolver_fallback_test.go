package build

import (
	"encoding/json"
	"testing"
)

// TestDNSServerResolverSelfReplacementUsesNextSuitable — сервер-замена сам
// ссылался на выпавший сервер. Сослаться на себя нельзя, поэтому берётся
// следующий пригодный сервер (не fakeip/hosts), а не снимается ключ: у сервера
// с доменным адресом без резолвера ядро не поднимет транспорт. Норма SPEC 129,
// сверено с LxBox.
func TestDNSServerResolverSelfReplacementUsesNextSuitable(t *testing.T) {
	in := `{"servers":[
		{"type":"https","tag":"doh","server":"dns.google","domain_resolver":"vpn_udp"},
		{"type":"udp","tag":"vpn_udp","server":"8.8.8.8","detour":"gone"},
		{"type":"fakeip","tag":"fake"},
		{"type":"udp","tag":"local","server":"1.1.1.1"}
	]}`
	out, fc := sanitizeDNSSection(json.RawMessage(in), map[string]bool{"proxy": true}, "doh")
	if !fc.active() || !fc.dropped["vpn_udp"] {
		t.Fatalf("vpn_udp with a dangling detour must be dropped: %+v", fc)
	}
	var got struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	for _, s := range got.Servers {
		if s["tag"] == "doh" {
			if s["domain_resolver"] != "local" {
				t.Errorf("doh must fall back to the next suitable server (local, not itself or fakeip): %v", s["domain_resolver"])
			}
			return
		}
	}
	t.Fatal("doh must stay in the config")
}
