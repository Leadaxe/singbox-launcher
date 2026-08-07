package subscription

import (
	"fmt"
	"strings"
	"testing"
)

// SPEC 094 фаза C — Xray-массив: все протоколы, все узлы элемента.

// Критерий 14: элемент с vmess даёт ноду (до SPEC 094 — ноль).
func TestXrayArrayParsesVMess(t *testing.T) {
	raw := `[{
	  "remarks": "vmess-node",
	  "outbounds": [{
	    "protocol": "vmess",
	    "tag": "proxy",
	    "settings": { "vnext": [{
	      "address": "v.test", "port": 443,
	      "users": [{ "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "alterId": 0, "security": "auto" }]
	    }] },
	    "streamSettings": { "network": "ws", "security": "tls",
	      "wsSettings": { "path": "/ws", "host": "v.test" } }
	  }]
	}]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	n := nodes[0]
	if n.Scheme != "vmess" || n.Server != "v.test" || n.Port != 443 {
		t.Fatalf("node = %s %s:%d, want vmess v.test:443", n.Scheme, n.Server, n.Port)
	}
	if n.Outbound["security"] != "auto" {
		t.Errorf("security = %v, want auto", n.Outbound["security"])
	}
	tr, ok := n.Outbound["transport"].(map[string]interface{})
	if !ok || tr["type"] != "ws" {
		t.Errorf("transport = %v, want ws", n.Outbound["transport"])
	}
}

func TestXrayArrayParsesTrojanAndShadowsocks(t *testing.T) {
	raw := `[
	  {"remarks":"tj","outbounds":[{"protocol":"trojan","tag":"proxy",
	    "settings":{"servers":[{"address":"t.test","port":443,"password":"pw"}]},
	    "streamSettings":{"network":"tcp","security":"tls"}}]},
	  {"remarks":"ss","outbounds":[{"protocol":"shadowsocks","tag":"proxy",
	    "settings":{"servers":[{"address":"s.test","port":8388,
	      "method":"aes-256-gcm","password":"sspw"}]}}]}
	]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}

	if nodes[0].Scheme != "trojan" || nodes[0].UUID != "pw" {
		t.Errorf("trojan node = %+v", nodes[0])
	}
	if nodes[1].Scheme != "ss" || nodes[1].Outbound["method"] != "aes-256-gcm" {
		t.Errorf("shadowsocks node = %+v", nodes[1])
	}
}

// Неподдерживаемый метод ss отбрасывает ноду, а не роняет весь конфиг.
func TestXrayArrayDropsShadowsocksWithBadMethod(t *testing.T) {
	raw := `[{"remarks":"ss","outbounds":[{"protocol":"shadowsocks","tag":"proxy",
	  "settings":{"servers":[{"address":"s.test","port":8388,
	    "method":"rc4-md5","password":"pw"}]}}]}]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("got %d nodes, want 0 (unsupported method)", len(nodes))
	}
}

// Критерий 15: элемент с тремя vless даёт три ноды (до SPEC 094 — одну).
func TestXrayArrayParsesEveryNodeOfElement(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`[{"remarks":"multi","outbounds":[`)
	for i := 0; i < 3; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"protocol":"vless","tag":"srv%d","settings":{"vnext":[
		  {"address":"n%d.test","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}`, i, i)
	}
	sb.WriteString(`]}]`)

	nodes, err := ParseNodesFromXrayJSONArray(sb.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}

	// Критерий C3: при нескольких узлах тег несёт различитель.
	seen := map[string]bool{}
	for _, n := range nodes {
		if seen[n.Tag] {
			t.Fatalf("duplicate tag %q among element nodes", n.Tag)
		}
		seen[n.Tag] = true
		if !strings.Contains(n.Tag, "multi") {
			t.Errorf("tag %q lost the element remarks", n.Tag)
		}
	}
}

// Критерий 18 (регрессия): одноузловой элемент сохраняет ЧИСТЫЙ тег из remarks.
func TestXraySingleNodeElementKeepsPlainTag(t *testing.T) {
	raw := `[{"remarks":"Solo Node","outbounds":[{"protocol":"vless","tag":"proxy",
	  "settings":{"vnext":[{"address":"n.test","port":443,
	    "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}]}]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	// Тег не должен обзавестись различителем — иначе поедут теги у всех,
	// кто уже пользуется такими подписками.
	if strings.Contains(nodes[0].Tag, "proxy") || strings.Contains(nodes[0].Tag, " 1") {
		t.Fatalf("single-node element tag = %q, want a plain remarks-based tag", nodes[0].Tag)
	}
}

// Критерий 16: xhttp из streamSettings.
func TestXrayArrayParsesXhttpTransport(t *testing.T) {
	raw := `[{"remarks":"xh","outbounds":[{"protocol":"vless","tag":"proxy",
	  "settings":{"vnext":[{"address":"x.test","port":443,
	    "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
	  "streamSettings":{"network":"xhttp","security":"tls",
	    "xhttpSettings":{"path":"/xh","host":"x.test","mode":"auto"}}}]}]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	tr, ok := nodes[0].Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("no transport: %v", nodes[0].Outbound)
	}
	if tr["type"] != "xhttp" || tr["path"] != "/xh" || tr["mode"] != "auto" {
		t.Fatalf("transport = %v, want xhttp /xh auto", tr)
	}
}

func TestXrayArrayParsesHttpUpgradeTransport(t *testing.T) {
	raw := `[{"remarks":"hu","outbounds":[{"protocol":"vless","tag":"proxy",
	  "settings":{"vnext":[{"address":"h.test","port":443,
	    "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
	  "streamSettings":{"network":"httpupgrade",
	    "httpupgradeSettings":{"path":"/hu?ed=2560","host":"h.test"}}}]}]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	tr, _ := nodes[0].Outbound["transport"].(map[string]interface{})
	if tr["type"] != "httpupgrade" {
		t.Fatalf("transport = %v, want httpupgrade", tr)
	}
	// Хвост ?ed=N не часть пути; early data у httpupgrade нет.
	if tr["path"] != "/hu" {
		t.Errorf("path = %v, want /hu (ed tail stripped)", tr["path"])
	}
	if _, present := tr["max_early_data"]; present {
		t.Error("httpupgrade must not carry max_early_data")
	}
}

// Служебные протоколы узлами не становятся (C2).
func TestXrayArraySkipsServiceProtocols(t *testing.T) {
	raw := `[{"remarks":"svc","outbounds":[
	  {"protocol":"freedom","tag":"direct"},
	  {"protocol":"blackhole","tag":"block"},
	  {"protocol":"dns","tag":"dns-out"},
	  {"protocol":"vless","tag":"proxy","settings":{"vnext":[
	    {"address":"n.test","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}
	]}]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (service protocols are not nodes)", len(nodes))
	}
	if nodes[0].Scheme != "vless" {
		t.Fatalf("scheme = %q, want vless", nodes[0].Scheme)
	}
}

// Критерий 17: порядок узлов соответствует порядку в подписке.
func TestXrayArrayPreservesSubscriptionOrder(t *testing.T) {
	raw := `[
	  {"remarks":"Auto Best","outbounds":[{"protocol":"vless","tag":"proxy",
	    "settings":{"vnext":[{"address":"a.test","port":443,
	      "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}]},
	  {"remarks":"Second","outbounds":[{"protocol":"vless","tag":"proxy",
	    "settings":{"vnext":[{"address":"b.test","port":443,
	      "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}]},
	  {"remarks":"Third","outbounds":[{"protocol":"vless","tag":"proxy",
	    "settings":{"vnext":[{"address":"c.test","port":443,
	      "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}]}
	]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}
	// «Авто | Лучший сервер» стоит в подписке первым и обязан остаться первым.
	wantServers := []string{"a.test", "b.test", "c.test"}
	for i, want := range wantServers {
		if nodes[i].Server != want {
			t.Fatalf("node[%d] server = %q, want %q — subscription order changed",
				i, nodes[i].Server, want)
		}
	}
}

// Многозвенная цепочка dialerProxy (C4): A → B → C.
func TestXrayArrayBuildsMultiHopDialerChain(t *testing.T) {
	raw := `[{"remarks":"chain","outbounds":[
	  {"protocol":"vless","tag":"proxy","settings":{"vnext":[
	    {"address":"main.test","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
	   "streamSettings":{"network":"tcp","sockopt":{"dialerProxy":"hop1"}}},
	  {"protocol":"socks","tag":"hop1","settings":{"servers":[{"address":"h1.test","port":1080}]},
	   "streamSettings":{"network":"tcp","sockopt":{"dialerProxy":"hop2"}}},
	  {"protocol":"socks","tag":"hop2","settings":{"servers":[{"address":"h2.test","port":1080}]}}
	]}]`

	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (hops are not standalone nodes)", len(nodes))
	}
	n := nodes[0]
	if len(n.Chain) != 2 {
		t.Fatalf("chain length = %d, want 2", len(n.Chain))
	}
	if n.Chain[0].Server != "h1.test" || n.Chain[1].Server != "h2.test" {
		t.Fatalf("chain = %s → %s, want h1.test → h2.test", n.Chain[0].Server, n.Chain[1].Server)
	}
	// Первый хоп дозванивается через второй.
	if got, _ := n.Chain[0].Outbound["detour"].(string); got != n.Chain[1].Tag {
		t.Errorf("hop[0] detour = %q, want %q", got, n.Chain[1].Tag)
	}
}

func TestIsXrayServiceProtocol(t *testing.T) {
	for _, p := range []string{"freedom", "blackhole", "dns", "loopback", "FREEDOM", " dns "} {
		if !IsXrayServiceProtocol(p) {
			t.Errorf("IsXrayServiceProtocol(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"vless", "vmess", "trojan", ""} {
		if IsXrayServiceProtocol(p) {
			t.Errorf("IsXrayServiceProtocol(%q) = true, want false", p)
		}
	}
}
