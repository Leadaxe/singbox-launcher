package dialogs

import (
	"strings"
	"testing"
)

// Превью и результат строятся из разбора входа — проверяем именно его, а не
// вёрстку: что попадёт в конфиг, а что форма отвергнет.
func TestParseAddServerInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"single share-uri", "socks5://u:p@10.0.0.1:1080#a", 1},
		{"several uris", "socks5://10.0.0.1:1080\nproxy-http://10.0.0.2:8080", 2},
		{"single outbound json", `{"type":"socks","tag":"s","server":"1.1.1.1","server_port":1080}`, 1},
		{"outbound array", `[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080},
		                     {"type":"http","tag":"b","server":"2.2.2.2","server_port":8080}]`, 2},
		{"full config", `{"outbounds":[{"type":"socks","tag":"s","server":"1.1.1.1","server_port":1080}]}`, 1},
		{"garbage", "not a link at all", 0},
		{"empty", "   ", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseAddServerInput(tc.input); len(got) != tc.want {
				t.Errorf("want %d nodes, got %d", tc.want, len(got))
			}
		})
	}
}

// Недонабранный ввод не должен ронять превью: оно пересчитывается на каждый
// символ, и паника здесь means падение всего окна.
func TestParseAddServerInput_NoPanicOnPartialInput(t *testing.T) {
	for _, p := range []string{
		"{", "[", `{"type"`, `{"type":"soc`, "socks5:/", "proxy-http:",
		"[Interface]", "[Interface]\nPrivateKey =", "]}{[",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", p, r)
				}
			}()
			parseAddServerInput(p)
		}()
	}
}

// Схема выбирается по режиму и TLS, а не по локализованной подписи.
func TestBuildProxyURI_Scheme(t *testing.T) {
	base := proxyURIInput{Host: "10.0.0.1", Port: "1080"}
	cases := []struct {
		mode   addServerMode
		tls    bool
		prefix string
	}{
		{modeSocks, false, "socks5://"},
		{modeSocks, true, "socks5://"}, // TLS у SOCKS5 ни на что не влияет
		{modeHTTP, false, "proxy-http://"},
		{modeHTTP, true, "proxy-https://"},
	}
	for _, tc := range cases {
		in := base
		in.Mode, in.TLS = tc.mode, tc.tls
		uri, err := buildProxyURI(in)
		if err != nil {
			t.Fatalf("buildProxyURI: %v", err)
		}
		if !strings.HasPrefix(uri, tc.prefix) {
			t.Errorf("mode=%v tls=%v: want %q, got %q", tc.mode, tc.tls, tc.prefix, uri)
		}
	}
}

// Пустой хост и негодный порт ловятся формой, а не глубиной парсера.
func TestBuildProxyURI_Validation(t *testing.T) {
	if _, err := buildProxyURI(proxyURIInput{Host: "", Port: "1080"}); err == nil {
		t.Error("empty host must fail")
	}
	for _, bad := range []string{"0", "65536", "abc", "", "-1"} {
		if _, err := buildProxyURI(proxyURIInput{Host: "h", Port: bad}); err == nil {
			t.Errorf("port %q must fail", bad)
		}
	}
}

// Креденшелы и тег доезжают до URI, откуда их заберёт общий путь Add.
func TestBuildProxyURI_CredentialsAndTag(t *testing.T) {
	uri, err := buildProxyURI(proxyURIInput{
		Host: "10.0.0.1", Port: "1080", User: "alice", Pass: "s3cret", Tag: "my node",
	})
	if err != nil {
		t.Fatalf("buildProxyURI: %v", err)
	}
	if !strings.Contains(uri, "alice:s3cret@") {
		t.Errorf("credentials lost: %q", uri)
	}
	if !strings.Contains(uri, "#my%20node") {
		t.Errorf("tag lost: %q", uri)
	}

	// Пароль с пробелами легален и не должен обрезаться.
	uri2, err := buildProxyURI(proxyURIInput{Host: "h", Port: "1", Pass: " p p "})
	if err != nil {
		t.Fatalf("buildProxyURI: %v", err)
	}
	if !strings.Contains(uri2, "%20p%20p%20") {
		t.Errorf("password whitespace trimmed: %q", uri2)
	}
}

// IPv6 берётся в скобки один раз, а не дважды.
func TestJoinHostPort_IPv6(t *testing.T) {
	if got := joinHostPort("::1", 1080); got != "[::1]:1080" {
		t.Errorf("bare IPv6: got %q", got)
	}
	if got := joinHostPort("[::1]", 1080); got != "[::1]:1080" {
		t.Errorf("already bracketed: got %q", got)
	}
	if got := joinHostPort("example.com", 443); got != "example.com:443" {
		t.Errorf("hostname: got %q", got)
	}
}

// Ручная правка одиночного объекта сохраняется побайтово — в этом её смысл.
func TestManualJSONResult_SingleObjectKeepsUnknownFields(t *testing.T) {
	edited := `{"type":"socks","tag":"hand","server":"9.9.9.9","server_port":9999,"custom_field":"kept"}`
	res, err := manualJSONResult(edited, "label")
	if err != nil {
		t.Fatalf("manualJSONResult: %v", err)
	}
	if len(res.ConfigJSON) == 0 {
		t.Fatal("single object must come back as ConfigJSON")
	}
	body := string(res.ConfigJSON)
	if !strings.Contains(body, "9.9.9.9") {
		t.Errorf("edited server lost: %s", body)
	}
	// Поле, которого наш парсер не знает, обязано выжить.
	if !strings.Contains(body, "custom_field") {
		t.Errorf("unknown field dropped: %s", body)
	}
	if res.Text != "" {
		t.Errorf("Text must stay empty for ConfigJSON path, got %q", res.Text)
	}
	if res.Label != "label" {
		t.Errorf("label lost: %q", res.Label)
	}
}

// Массив и целый конфиг уходят текстом — их разберёт общий путь Add.
func TestManualJSONResult_MultiGoesAsText(t *testing.T) {
	for _, body := range []string{
		`[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080}]`,
		`{"outbounds":[{"type":"socks","tag":"s","server":"1.1.1.1","server_port":1080}]}`,
	} {
		res, err := manualJSONResult(body, "")
		if err != nil {
			t.Fatalf("manualJSONResult(%s): %v", body, err)
		}
		if len(res.ConfigJSON) != 0 {
			t.Errorf("multi-node body must not become ConfigJSON: %s", body)
		}
		if res.Text == "" {
			t.Errorf("multi-node body must come back as Text: %s", body)
		}
	}
}

// Пустой и битый JSON доходят до пользователя ошибкой.
func TestManualJSONResult_Rejects(t *testing.T) {
	if _, err := manualJSONResult("   ", ""); err == nil {
		t.Error("empty JSON must fail")
	}
	if _, err := manualJSONResult(`{"tag":"no-type"}`, ""); err == nil {
		t.Error("object without type must fail")
	}
}

// WireGuard: обязательные поля проверяются формой, а не парсером.
func TestBuildWireGuardURI_RequiredFields(t *testing.T) {
	full := wgURIInput{
		Host: "vpn.example", Port: "51820",
		Private: testWGPriv, Public: testWGPub,
		Address: "10.0.0.2/32", Allowed: "0.0.0.0/0",
	}
	if _, err := buildWireGuardURI(full); err != nil {
		t.Fatalf("full input must pass: %v", err)
	}

	for name, mutate := range map[string]func(*wgURIInput){
		"no host":    func(in *wgURIInput) { in.Host = "" },
		"no private": func(in *wgURIInput) { in.Private = "" },
		"no public":  func(in *wgURIInput) { in.Public = "" },
		"no address": func(in *wgURIInput) { in.Address = "" },
		"bad port":   func(in *wgURIInput) { in.Port = "0" },
	} {
		in := full
		mutate(&in)
		if _, err := buildWireGuardURI(in); err == nil {
			t.Errorf("%s must fail", name)
		}
	}
}

// URI, собранный формой, обязан разобраться нашим же парсером — иначе форма
// и парсер разошлись, и узел молча не доедет до конфига.
func TestBuildWireGuardURI_RoundTrip(t *testing.T) {
	uri, err := buildWireGuardURI(wgURIInput{
		Host: "vpn.example", Port: "51820",
		Private: testWGPriv, Public: testWGPub,
		Address: "10.0.0.2/32", Allowed: "0.0.0.0/0",
		MTU: "1280", Keepalive: "25", Tag: "wg node",
	})
	if err != nil {
		t.Fatalf("buildWireGuardURI: %v", err)
	}
	nodes := parseAddServerInput(uri)
	if len(nodes) != 1 {
		t.Fatalf("want 1 node from %q, got %d", uri, len(nodes))
	}
	if nodes[0].Scheme != "wireguard" {
		t.Errorf("scheme: got %q", nodes[0].Scheme)
	}
	if mtu, ok := nodes[0].Outbound["mtu"].(int); !ok || mtu != 1280 {
		t.Errorf("mtu lost: %v", nodes[0].Outbound["mtu"])
	}
}

// MTU и keepalive вне разумных границ отвергаются формой.
func TestBuildWireGuardURI_NumericBounds(t *testing.T) {
	base := wgURIInput{
		Host: "h", Port: "51820", Private: testWGPriv, Public: testWGPub, Address: "10.0.0.2/32",
	}
	for _, bad := range []string{"1", "100000", "abc"} {
		in := base
		in.MTU = bad
		if _, err := buildWireGuardURI(in); err == nil {
			t.Errorf("mtu %q must fail", bad)
		}
	}
	for _, bad := range []string{"-1", "70000", "x"} {
		in := base
		in.Keepalive = bad
		if _, err := buildWireGuardURI(in); err == nil {
			t.Errorf("keepalive %q must fail", bad)
		}
	}
}

// Direct: плоский outbound без полей назначения — ядро 1.13+ их не принимает.
func TestDirectResult_PlainOutbound(t *testing.T) {
	res, err := directResult("", "", "wa")
	if err != nil {
		t.Fatalf("directResult: %v", err)
	}
	body := string(res.ConfigJSON)
	if !strings.Contains(body, `"type":"direct"`) {
		t.Errorf("not a direct outbound: %s", body)
	}
	// Именно эти поля ядро отвергает — их не должно быть в outbound'е.
	for _, banned := range []string{"override_address", "override_port", "server", "server_port"} {
		if strings.Contains(body, banned) {
			t.Errorf("banned field %q emitted into direct outbound: %s", banned, body)
		}
	}
	if res.Label != "wa" {
		t.Errorf("label: got %q", res.Label)
	}
}

// Заполненные IP/порт возвращаются отдельно — они для правила маршрута.
func TestDirectResult_OverrideReturnedSeparately(t *testing.T) {
	res, err := directResult("1.2.3.4", "443", "")
	if err != nil {
		t.Fatalf("directResult: %v", err)
	}
	if res.OverrideIP != "1.2.3.4" || res.OverridePort != "443" {
		t.Errorf("override lost: ip=%q port=%q", res.OverrideIP, res.OverridePort)
	}
	if strings.Contains(string(res.ConfigJSON), "1.2.3.4") {
		t.Errorf("override leaked into outbound: %s", res.ConfigJSON)
	}
	// Пустой тег получает осмысленный fallback, а не пустую строку.
	if res.Label == "" {
		t.Error("empty tag must fall back to a default")
	}

	if _, err := directResult("", "70000", ""); err == nil {
		t.Error("bad port must fail")
	}
}

// Ключи WireGuard — ровно 32 байта base64. Значения детерминированные и не
// секретные: важна только длина, которую требует и парсер, и ядро.
const (
	testWGPriv = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	testWGPub  = "ICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj8="
	testWGPSK  = "QEFCQ0RFRkdISUpLTE1OT1BRUlNUVVZXWFlaW1xdXl8="
)

// Негодный ключ обязан отвергаться формой, а не молча ронять узел в парсере:
// там он уходит warning'ом в лог, которого человек в форме не увидит.
func TestBuildWireGuardURI_RejectsBadKeys(t *testing.T) {
	base := wgURIInput{
		Host: "h", Port: "51820", Private: testWGPriv, Public: testWGPub,
		Address: "10.0.0.2/32",
	}
	for name, mutate := range map[string]func(*wgURIInput){
		"short private": func(in *wgURIInput) { in.Private = "dG9vLXNob3J0" },
		"short public":  func(in *wgURIInput) { in.Public = "dG9vLXNob3J0" },
		"not base64":    func(in *wgURIInput) { in.Private = "!!! not base64 !!!" },
		"short psk":     func(in *wgURIInput) { in.Preshared = "dG9vLXNob3J0" },
	} {
		in := base
		mutate(&in)
		if _, err := buildWireGuardURI(in); err == nil {
			t.Errorf("%s must be rejected by the form", name)
		}
	}

	// Валидный pre-shared ключ проходит.
	in := base
	in.Preshared = testWGPSK
	if _, err := buildWireGuardURI(in); err != nil {
		t.Errorf("valid psk rejected: %v", err)
	}
}
