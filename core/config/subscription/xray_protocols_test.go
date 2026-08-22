package subscription

import (
	"fmt"
	"net/url"
	"reflect"
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

// SPEC 102 — XHTTP `extra` в Xray-JSON ветке.
//
// Фикстуры воспроизводят раскладку живых узлов, на которых дефект найден
// (packet-up с uplinkHTTPMethod=GET, stream-one с xPaddingBytes), но адреса,
// UUID и ключи вымышленные.

// xrayXhttpTransport builds a one-node Xray element around the given
// xhttpSettings JSON and returns the resulting sing-box transport object.
func xrayXhttpTransport(t *testing.T, xhttpSettings string) map[string]interface{} {
	t.Helper()
	raw := fmt.Sprintf(`[{"remarks":"xh","outbounds":[{"protocol":"vless","tag":"proxy",
	  "settings":{"vnext":[{"address":"x.test","port":443,
	    "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
	  "streamSettings":{"network":"xhttp","security":"tls",
	    "xhttpSettings":%s}}]}]`, xhttpSettings)

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
	return tr
}

// Критерий 1: uplinkHTTPMethod=GET доходит до конфига. Сервер packet-up узла
// ждёт uplink методом GET; без этого поля ядро шлёт POST и получает 400.
func TestXrayXhttpExtraCarriesUplinkHTTPMethod(t *testing.T) {
	tr := xrayXhttpTransport(t, `{"path":"/livestreamcontent/","host":"cdn.test",
	  "mode":"packet-up","extra":{"uplinkHTTPMethod":"GET","scMaxBufferedPosts":30,
	  "scMaxEachPostBytes":"1000000","xPaddingBytes":"0-0"}}`)

	if tr["uplink_http_method"] != "GET" {
		t.Errorf("uplink_http_method = %v, want GET", tr["uplink_http_method"])
	}
	if tr["mode"] != "packet-up" || tr["path"] != "/livestreamcontent/" || tr["host"] != "cdn.test" {
		t.Errorf("base trio lost: %v", tr)
	}
}

// Критерий 2: xPaddingBytes доходит дословно, включая "0-0" — это требование
// «padding выключить», а не пустое значение.
func TestXrayXhttpExtraKeepsPaddingVerbatim(t *testing.T) {
	for _, pad := range []string{"50-150", "0-0"} {
		tr := xrayXhttpTransport(t, fmt.Sprintf(
			`{"mode":"stream-one","path":"/p","extra":{"xPaddingBytes":%q}}`, pad))
		if tr["x_padding_bytes"] != pad {
			t.Errorf("x_padding_bytes = %v, want %q", tr["x_padding_bytes"], pad)
		}
	}
}

// Критерий 3: числа из extra становятся строками без экспоненциальной записи —
// эмиттер ждёт строку, а float64 дал бы "1e+06".
func TestXrayXhttpExtraStringifiesNumbers(t *testing.T) {
	tr := xrayXhttpTransport(t, `{"mode":"packet-up","extra":{
	  "scMaxEachPostBytes":1000000,"uplinkChunkSize":0,"scMinPostsIntervalMs":30}}`)

	if got := tr["sc_max_each_post_bytes"]; got != "1000000" {
		t.Errorf("sc_max_each_post_bytes = %v, want \"1000000\"", got)
	}
	if got := tr["sc_min_posts_interval_ms"]; got != "30" {
		t.Errorf("sc_min_posts_interval_ms = %v, want \"30\"", got)
	}
}

// Критерий 4: плоское поле читается, а одноимённое в extra его перекрывает.
func TestXrayXhttpExtraOverridesFlatSettings(t *testing.T) {
	flat := xrayXhttpTransport(t, `{"mode":"stream-one","xPaddingBytes":"10-20"}`)
	if flat["x_padding_bytes"] != "10-20" {
		t.Errorf("flat x_padding_bytes = %v, want 10-20", flat["x_padding_bytes"])
	}

	both := xrayXhttpTransport(t,
		`{"mode":"stream-one","xPaddingBytes":"10-20","extra":{"xPaddingBytes":"50-150"}}`)
	if both["x_padding_bytes"] != "50-150" {
		t.Errorf("extra must win: x_padding_bytes = %v, want 50-150", both["x_padding_bytes"])
	}
}

// Критерий 5: битый extra деградирует узел, а не роняет его и не пишет мусор.
func TestXrayXhttpBrokenExtraDegradesNode(t *testing.T) {
	for _, extra := range []string{`"not-json"`, `[]`, `5`, `null`} {
		tr := xrayXhttpTransport(t, fmt.Sprintf(
			`{"mode":"stream-one","path":"/p","extra":%s}`, extra))
		if tr["mode"] != "stream-one" || tr["path"] != "/p" {
			t.Errorf("extra=%s: base fields lost: %v", extra, tr)
		}
		if _, ok := tr["extra"]; ok {
			t.Errorf("extra=%s: raw extra leaked into transport: %v", extra, tr)
		}
	}
}

// Критерий 6 — гейт против повторного расхождения веток: один и тот же набор
// полей, поданный share-URI и Xray-JSON, даёт идентичный транспорт. Падает,
// если поле добавили только в одну ветку.
func TestXhttpBranchParity(t *testing.T) {
	const extraJSON = `{"uplinkHTTPMethod":"GET","xPaddingBytes":"50-150",` +
		`"scMaxEachPostBytes":"1000000","scMinPostsIntervalMs":"30-60",` +
		`"xPaddingObfsMode":true,"sessionKey":"sk","seqKey":"qk",` +
		`"uplinkDataKey":"uk","xPaddingHeader":"X-Pad","xPaddingMethod":"pm",` +
		`"sessionPlacement":"header","seqPlacement":"query",` +
		`"uplinkDataPlacement":"body","xPaddingPlacement":"header",` +
		`"uplinkChunkSize":"4096","xPaddingKey":"pk"}`

	uri := "vless://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@x.test:443?type=xhttp" +
		"&security=tls&mode=packet-up&path=%2Fp&host=cdn.test&extra=" +
		url.QueryEscape(extraJSON) + "#xh"
	uriNode, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatal(err)
	}
	uriTr, ok := uriNode.Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("uri: no transport: %v", uriNode.Outbound)
	}

	xrayTr := xrayXhttpTransport(t, fmt.Sprintf(
		`{"mode":"packet-up","path":"/p","host":"cdn.test","extra":%s}`, extraJSON))

	if !reflect.DeepEqual(uriTr, xrayTr) {
		t.Errorf("branches diverged:\n  uri  = %v\n  xray = %v", uriTr, xrayTr)
	}
}

// Пункт 3.7 — фикстуры трёх живых узлов из SPEC 102 §1.2, на которых нашёлся
// дефект (`URLTestOutbound` возвращал 400 Bad Request). Адреса заменены на
// TEST-NET-1 (RFC 5737) / example-N.com, UUID и ключи — на синтетические той
// же формы; форма и значения extra-блока сохранены дословно.

// SPEC 102 §1.2, узел `188.72.103.4:443` (mode packet-up). Сервер ждёт uplink
// методом GET; без extra ядро слало POST → "unexpected upload status: 400".
func TestXrayXhttpFixturePacketUpNode(t *testing.T) {
	// Full element shape (address 192.0.2.4, TEST-NET-1): vless/reality over
	// xhttp, mode packet-up, extra as on the live node.
	tr := xrayXhttpTransport(t, `{"path":"/livestream/chunk","host":"example-3.com","mode":"packet-up",
	  "extra":{"uplinkHTTPMethod":"GET","scMaxBufferedPosts":30,
	    "scMaxEachPostBytes":"1000000","scStreamUpServerSecs":"20-80",
	    "xPaddingBytes":"0-0"}}`)

	if tr["mode"] != "packet-up" {
		t.Errorf("mode = %v, want packet-up", tr["mode"])
	}
	if tr["uplink_http_method"] != "GET" {
		t.Errorf("uplink_http_method = %v, want GET", tr["uplink_http_method"])
	}
	if tr["sc_max_buffered_posts"] != 30 {
		t.Errorf("sc_max_buffered_posts = %v (%T), want int 30", tr["sc_max_buffered_posts"], tr["sc_max_buffered_posts"])
	}
	if tr["sc_max_each_post_bytes"] != "1000000" {
		t.Errorf("sc_max_each_post_bytes = %v, want \"1000000\"", tr["sc_max_each_post_bytes"])
	}
	if tr["sc_stream_up_server_secs"] != "20-80" {
		t.Errorf("sc_stream_up_server_secs = %v, want \"20-80\"", tr["sc_stream_up_server_secs"])
	}
	if tr["x_padding_bytes"] != "0-0" {
		t.Errorf("x_padding_bytes = %v, want \"0-0\" (padding disabled, not empty)", tr["x_padding_bytes"])
	}
}

// SPEC 102 §1.2, узлы `46.243.142.42:9443` и `95.163.232.194:8444` (mode
// stream-one, идентичный extra-блок). Терялся x_padding_bytes и весь xmux —
// вероятная причина отказа 400 (ядро подставляло свой padding).
func TestXrayXhttpFixtureStreamOneNodesWithXmux(t *testing.T) {
	fixtures := []struct {
		name string
		addr string
		host string
	}{
		{"node-a", "192.0.2.42", "example-1.com"},
		{"node-b", "192.0.2.94", "example-2.com"},
	}

	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			raw := fmt.Sprintf(`[{"remarks":%q,"outbounds":[{"protocol":"vless","tag":"proxy",
			  "settings":{"vnext":[{"address":%q,"port":9443,
			    "users":[{"id":"2b3c4d5e-6f7a-4b2c-9d3e-4f5a6b7c8d9e","encryption":"none"}]}]},
			  "streamSettings":{"network":"xhttp","security":"reality",
			    "xhttpSettings":{"mode":"stream-one","host":%q,
			      "extra":{"xPaddingBytes":"50-150","xmux":{
			        "maxConcurrency":"16-32","cMaxReuseTimes":"256-512",
			        "maxConnections":0,"hKeepAlivePeriod":0,
			        "hMaxReusableSecs":"900-1500"}}}}}]}]`, fx.name, fx.addr, fx.host)

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

			if tr["mode"] != "stream-one" {
				t.Errorf("mode = %v, want stream-one", tr["mode"])
			}
			if tr["x_padding_bytes"] != "50-150" {
				t.Errorf("x_padding_bytes = %v, want \"50-150\"", tr["x_padding_bytes"])
			}

			xmux, ok := tr["xmux"].(map[string]interface{})
			if !ok {
				t.Fatalf("xmux missing or wrong type: %v", tr["xmux"])
			}
			wantStr := map[string]interface{}{
				"max_concurrency":     "16-32",
				"c_max_reuse_times":   "256-512",
				"h_max_reusable_secs": "900-1500",
			}
			for k, want := range wantStr {
				if xmux[k] != want {
					t.Errorf("xmux[%q] = %v, want %v", k, xmux[k], want)
				}
			}
			// maxConnections/hKeepAlivePeriod are 0 — the int-lookup path must not
			// drop a legitimate zero the way an empty-string check would.
			if xmux["max_connections"] != "0" {
				t.Errorf("xmux[max_connections] = %v, want \"0\"", xmux["max_connections"])
			}
			if xmux["h_keep_alive_period"] != 0 {
				t.Errorf("xmux[h_keep_alive_period] = %v (%T), want int 0", xmux["h_keep_alive_period"], xmux["h_keep_alive_period"])
			}
		})
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
