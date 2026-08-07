package business

import (
	"strings"
	"testing"
)

// SPEC 095 — разбор config.json для подзаголовка узла.

func TestDeriveTransport(t *testing.T) {
	tests := []struct {
		name     string
		nodeType string
		raw      map[string]interface{}
		want     string
	}{
		{
			name: "ws transport as is", nodeType: "vless",
			raw:  map[string]interface{}{"transport": map[string]interface{}{"type": "ws"}},
			want: "ws",
		},
		{
			// "http" короче и привычнее читается как h2.
			name: "http shown as h2", nodeType: "vless",
			raw:  map[string]interface{}{"transport": map[string]interface{}{"type": "http"}},
			want: "h2",
		},
		{
			name: "grpc", nodeType: "vless",
			raw:  map[string]interface{}{"transport": map[string]interface{}{"type": "grpc"}},
			want: "grpc",
		},
		{
			name: "xhttp", nodeType: "vless",
			raw:  map[string]interface{}{"transport": map[string]interface{}{"type": "xhttp"}},
			want: "xhttp",
		},
		{
			// Голый TCP — не «нет транспорта», а осмысленное значение.
			name: "vless without transport is tcp", nodeType: "vless",
			raw:  map[string]interface{}{},
			want: "tcp",
		},
		{name: "trojan without transport is tcp", nodeType: "trojan", raw: map[string]interface{}{}, want: "tcp"},
		{name: "anytls without transport is tcp", nodeType: "anytls", raw: map[string]interface{}{}, want: "tcp"},
		{
			name: "masque network", nodeType: "masque",
			raw:  map[string]interface{}{"network": "h2"},
			want: "h2",
		},
		{
			// Пустой network у masque = дефолт ядра.
			name: "masque without network defaults to h3", nodeType: "masque",
			raw:  map[string]interface{}{},
			want: "h3",
		},
		{name: "wireguard has no transport", nodeType: "wireguard", raw: map[string]interface{}{}, want: ""},
		{name: "group has no transport", nodeType: "urltest", raw: map[string]interface{}{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveTransport(tt.nodeType, tt.raw); got != tt.want {
				t.Fatalf("deriveTransport() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeriveSecurityTLS(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]interface{}
		want string
	}{
		{
			name: "plain tls",
			raw:  map[string]interface{}{"tls": map[string]interface{}{"enabled": true}},
			want: "TLS",
		},
		{
			name: "reality",
			raw: map[string]interface{}{"tls": map[string]interface{}{
				"enabled": true,
				"reality": map[string]interface{}{"enabled": true, "public_key": "x"},
			}},
			want: "Reality",
		},
		{
			name: "tls with vision",
			raw: map[string]interface{}{
				"tls":  map[string]interface{}{"enabled": true},
				"flow": "xtls-rprx-vision",
			},
			want: "TLS+Vision",
		},
		{
			name: "reality with vision",
			raw: map[string]interface{}{
				"tls": map[string]interface{}{
					"enabled": true,
					"reality": map[string]interface{}{"enabled": true},
				},
				"flow": "xtls-rprx-vision",
			},
			want: "Reality+Vision",
		},
		{
			name: "tls disabled",
			raw:  map[string]interface{}{"tls": map[string]interface{}{"enabled": false}},
			want: "",
		},
		{name: "no tls block", raw: map[string]interface{}{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveSecurity("vless", tt.raw); got != tt.want {
				t.Fatalf("deriveSecurity() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Уровень AmneziaWG определяется структурно — явной версии в конфиге нет.
func TestDeriveSecurityAmneziaWG(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]interface{}
		want string
	}{
		{
			name: "ranged header means awg2",
			raw:  map[string]interface{}{"h1": "5-10"},
			want: "awg2",
		},
		{
			name: "transport padding means awg2",
			raw:  map[string]interface{}{"s3": float64(20)},
			want: "awg2",
		},
		{
			name: "signature packets mean awg1.5",
			raw:  map[string]interface{}{"i1": "b0ffee"},
			want: "awg1.5",
		},
		{
			name: "basic obfuscation means awg",
			raw:  map[string]interface{}{"jc": float64(4), "s1": float64(15)},
			want: "awg",
		},
		{
			// Одиночный h1 числом — это 1.0, диапазон дал бы 2.0.
			name: "numeric header is plain awg",
			raw:  map[string]interface{}{"h1": float64(1)},
			want: "awg",
		},
		{
			name: "masquerade lifts to 1.5+",
			raw:  map[string]interface{}{"jc": float64(4), "ip": "1.2.3.4"},
			want: "awg1.5+",
		},
		{
			name: "masquerade on awg2 stays awg2+",
			raw:  map[string]interface{}{"h1": "5-10", "ib": "x"},
			want: "awg2+",
		},
		{
			name: "plain wireguard has no label",
			raw:  map[string]interface{}{"mtu": float64(1420)},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveSecurity("wireguard", tt.raw); got != tt.want {
				t.Fatalf("deriveSecurity(wireguard) = %q, want %q", got, tt.want)
			}
		})
	}
}

// Конфиг лаунчера содержит //-комментарии и маркеры парсера — обычный
// json.Unmarshal на нём падает.
func TestParseConfigNodesHandlesCommentsAndMarkers(t *testing.T) {
	raw := []byte(`{
  "outbounds": [
    /** @ParserSTART */
    // reality-node
    {"tag":"a","type":"vless","server":"e.com","server_port":443,
     "tls":{"enabled":true,"reality":{"enabled":true}},
     "transport":{"type":"ws"}},
    // ws-node
    {"tag":"b","type":"trojan","server":"f.com","server_port":8443},
    /** @ParserEND */
    {"tag":"auto","type":"urltest","outbounds":["a","b"],"url":"https://x/y"}
  ],
  "endpoints": [
    {"tag":"wg","type":"wireguard","mtu":1280,"h1":"5-10"}
  ]
}`)

	nodes := ParseConfigNodes(raw)
	if nodes.Len() != 4 {
		t.Fatalf("разобрано %d узлов, ожидалось 4", nodes.Len())
	}

	a := nodes.Lookup("a")
	if a == nil {
		t.Fatal("узел a не найден")
	}
	if a.Transport != "ws" || a.Security != "Reality" {
		t.Errorf("a: transport=%q security=%q, ожидалось ws/Reality", a.Transport, a.Security)
	}
	if a.Server != "e.com" || a.ServerPort != 443 {
		t.Errorf("a: адрес %s:%d", a.Server, a.ServerPort)
	}
	if a.Kind != "outbound" {
		t.Errorf("a: kind=%q, ожидалось outbound", a.Kind)
	}

	b := nodes.Lookup("b")
	if b == nil || b.Transport != "tcp" {
		t.Errorf("b: transport=%q, ожидалось tcp", b.Transport)
	}

	group := nodes.Lookup("auto")
	if group == nil {
		t.Fatal("группа не найдена")
	}
	if !group.IsGroup() {
		t.Error("urltest должен считаться группой")
	}
	if len(group.GroupMembers) != 2 {
		t.Errorf("состав группы = %v, ожидалось 2 члена", group.GroupMembers)
	}

	wg := nodes.Lookup("wg")
	if wg == nil {
		t.Fatal("endpoint не найден")
	}
	if wg.Kind != "endpoint" {
		t.Errorf("wg: kind=%q, ожидалось endpoint", wg.Kind)
	}
	if wg.Security != "awg2" {
		t.Errorf("wg: security=%q, ожидалось awg2", wg.Security)
	}
}

// URL внутри строк не должен приниматься за //-комментарий.
func TestStripJSONCommentsKeepsURLs(t *testing.T) {
	raw := []byte(`{"outbounds":[{"tag":"a","type":"urltest",
	  "url":"https://www.gstatic.com/generate_204","outbounds":["x"]}]}`)

	nodes := ParseConfigNodes(raw)
	a := nodes.Lookup("a")
	if a == nil {
		t.Fatal("узел не разобран — вероятно, URL съеден как комментарий")
	}
	if got, _ := a.Raw["url"].(string); got != "https://www.gstatic.com/generate_204" {
		t.Fatalf("url = %q, ожидался целый URL", got)
	}
}

func TestSubtitleParts(t *testing.T) {
	tests := []struct {
		name string
		node *ConfigNode
		want string
	}{
		{
			name: "full",
			node: &ConfigNode{Type: "vless", Transport: "ws", Security: "Reality"},
			want: "vless·ws·Reality",
		},
		{
			name: "no security",
			node: &ConfigNode{Type: "socks", Transport: "tcp"},
			want: "socks·tcp",
		},
		{
			name: "protocol only",
			node: &ConfigNode{Type: "wireguard"},
			want: "wireguard",
		},
		{name: "nil node", node: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(tt.node.SubtitleParts(), "·")
			if got != tt.want {
				t.Fatalf("подзаголовок = %q, ожидался %q", got, tt.want)
			}
		})
	}
}

// Отсутствующий тег — не ошибка: Clash API может отдать узел, которого нет в
// текущем конфиге (гонка перегенерации).
func TestLookupMissingTagIsSafe(t *testing.T) {
	nodes := ParseConfigNodes([]byte(`{"outbounds":[{"tag":"a","type":"vless"}]}`))
	if nodes.Lookup("нет-такого") != nil {
		t.Fatal("ожидался nil для неизвестного тега")
	}
	var empty *ConfigNodes
	if empty.Lookup("a") != nil {
		t.Fatal("nil-снимок должен возвращать nil без паники")
	}
	if empty.Len() != 0 {
		t.Fatal("nil-снимок должен иметь нулевую длину")
	}
}

func TestParseBrokenConfigIsSafe(t *testing.T) {
	if got := ParseConfigNodes([]byte(`{"outbounds": [`)).Len(); got != 0 {
		t.Fatalf("битый конфиг дал %d узлов, ожидалось 0", got)
	}
	if got := LoadConfigNodes("/no/such/path/config.json").Len(); got != 0 {
		t.Fatalf("несуществующий файл дал %d узлов, ожидалось 0", got)
	}
	if got := LoadConfigNodes("").Len(); got != 0 {
		t.Fatalf("пустой путь дал %d узлов, ожидалось 0", got)
	}
}

func TestServiceNodeDetection(t *testing.T) {
	nodes := ParseConfigNodes([]byte(`{"outbounds":[
	  {"tag":"direct","type":"direct"},
	  {"tag":"block","type":"block"},
	  {"tag":"real","type":"vless"}
	]}`))

	for _, tag := range []string{"direct", "block"} {
		if n := nodes.Lookup(tag); n == nil || !n.IsService() {
			t.Errorf("%q должен считаться служебным", tag)
		}
	}
	if n := nodes.Lookup("real"); n == nil || n.IsService() {
		t.Error("vless не должен считаться служебным")
	}
}
