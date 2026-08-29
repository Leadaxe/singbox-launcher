// SPEC 118 §4.E.8 — `dns.detour` на несуществующий тег ловится санитайзером
// НА СБОРКЕ, а не падением ядра на старте.
package build

import (
	"encoding/json"
	"testing"
)

func dnsSectionWithDetours(detours ...string) json.RawMessage {
	servers := make([]map[string]interface{}, 0, len(detours))
	for i, d := range detours {
		srv := map[string]interface{}{
			"tag":    "dns-" + string(rune('a'+i)),
			"type":   "udp",
			"server": "1.1.1.1",
		}
		if d != "" {
			srv["detour"] = d
		}
		servers = append(servers, srv)
	}
	raw, _ := json.Marshal(map[string]interface{}{"servers": servers})
	return raw
}

func dnsServerDetour(t *testing.T, raw json.RawMessage, tag string) (string, bool) {
	t.Helper()
	var obj struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal dns: %v", err)
	}
	for _, srv := range obj.Servers {
		if got, _ := srv["tag"].(string); got == tag {
			d, has := srv["detour"].(string)
			return d, has
		}
	}
	t.Fatalf("DNS-сервер %q не найден", tag)
	return "", false
}

func TestSanitizeDNSDetours_DanglingKeyIsStripped(t *testing.T) {
	raw := dnsSectionWithDetours("proxy-out", "ghost-out")
	final := map[string]bool{"proxy-out": true, "direct-out": true}

	out := SanitizeDNSDetours(raw, final)

	if d, has := dnsServerDetour(t, out, "dns-a"); !has || d != "proxy-out" {
		t.Errorf("живой detour снят: %q (has=%v)", d, has)
	}
	if _, has := dnsServerDetour(t, out, "dns-b"); has {
		t.Error("висячий detour доехал до ядра — конфиг не стартовал бы")
	}
}

func TestSanitizeDNSDetours_UntouchedWhenAllAlive(t *testing.T) {
	raw := dnsSectionWithDetours("proxy-out", "")
	final := map[string]bool{"proxy-out": true}

	out := SanitizeDNSDetours(raw, final)
	if string(out) != string(raw) {
		t.Errorf("секция переписана без нужды:\n%s\n%s", out, raw)
	}
}

func TestSanitizeDNSDetours_MalformedSectionUntouched(t *testing.T) {
	raw := json.RawMessage(`{"servers": "not-an-array"}`)
	if got := SanitizeDNSDetours(raw, map[string]bool{"x": true}); string(got) != string(raw) {
		t.Errorf("битая секция переписана: %s", got)
	}
	broken := json.RawMessage(`{ not json`)
	if got := SanitizeDNSDetours(broken, map[string]bool{"x": true}); string(got) != string(broken) {
		t.Errorf("нечитаемая секция переписана: %s", got)
	}
}

// Реестр известных целей обязан ВИДЕТЬ ссылку dns.detour: без этого
// переименование Направления оставило бы у DNS-сервера ссылку в никуда.
func TestDNSDetourTags_CollectsReferences(t *testing.T) {
	raw := dnsSectionWithDetours("proxy-out", "", "proxy-out", "vpn-out")
	got := DNSDetourTags(raw)
	if len(got) != 2 {
		t.Fatalf("DNSDetourTags = %v, want 2 уникальных", got)
	}
	seen := map[string]bool{got[0]: true, got[1]: true}
	if !seen["proxy-out"] || !seen["vpn-out"] {
		t.Errorf("DNSDetourTags = %v, want [proxy-out vpn-out]", got)
	}
}
