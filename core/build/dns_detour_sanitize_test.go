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

// SPEC 121 §8 п. 8 — ребро `dns.servers[].endpoint`: висячая ссылка
// выбрасывает СЕРВЕР ЦЕЛИКОМ (без endpoint'а он невалиден, снять ключ
// нельзя), а правило, ссылавшееся на него, чинится в той же точке.
func TestSanitizeDNSDetours_DanglingEndpointDropsServerAndRepairsRule(t *testing.T) {
	raw, err := json.Marshal(map[string]interface{}{
		"servers": []map[string]interface{}{
			{"tag": "ts-dns", "type": "tailscale", "endpoint": "ghost-node"},
			{"tag": "plain-dns", "type": "udp", "server": "1.1.1.1"},
		},
		"rules": []map[string]interface{}{
			{"domain_suffix": []string{".ts.net"}, "server": "ts-dns"},
			{"domain_suffix": []string{".example"}, "server": "plain-dns"},
		},
		"final": "ts-dns",
	})
	if err != nil {
		t.Fatalf("сборка секции: %v", err)
	}

	out := SanitizeDNSDetours(raw, map[string]bool{"direct-out": true})

	var got struct {
		Servers []map[string]interface{} `json:"servers"`
		Rules   []map[string]interface{} `json:"rules"`
		Final   string                   `json:"final"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("разбор результата: %v", err)
	}

	for _, srv := range got.Servers {
		if tag, _ := srv["tag"].(string); tag == "ts-dns" {
			t.Fatal("сервер с висячим endpoint доехал до ядра — конфиг не стартовал бы")
		}
	}
	for _, r := range got.Rules {
		if srv, _ := r["server"].(string); srv == "ts-dns" {
			t.Error("правило на выброшенный сервер осталось — «dns server not found» роняет конфиг целиком")
		}
	}
	if got.Final == "ts-dns" {
		t.Error("dns.final остался на выброшенном сервере")
	}
	if len(got.Servers) != 1 {
		t.Errorf("живой сервер тоже пропал: осталось %d записей", len(got.Servers))
	}
}
