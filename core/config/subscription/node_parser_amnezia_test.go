package subscription

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

// buildVPNLink emulates Amnezia's encoder: vpn:// + base64url-no-padding of
// qCompress(json) (4-byte big-endian uncompressed size + zlib stream).
func buildVPNLink(t *testing.T, profile map[string]interface{}) string {
	t.Helper()
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(data))); err != nil {
		t.Fatalf("write qCompress header: %v", err)
	}
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return "vpn://" + base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

const amneziaAWGIni = `[Interface]
Address = 10.8.1.2/32
DNS = 1.1.1.1, 1.0.0.1
PrivateKey = UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=
MTU = 1420
Jc = 4
Jmin = 40
Jmax = 70
S1 = 116
S2 = 61
H1 = 1239197098
H2 = 1929999940
H3 = 1499605721
H4 = 992706287
I1 = <b 0x000100002112a442><r 12>

[Peer]
PublicKey = QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=
PresharedKey = UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.7:38291
PersistentKeepalive = 25
`

const amneziaPlainWGIni = `[Interface]
Address = 10.8.1.2/32
PrivateKey = UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=
MTU = 1380

[Peer]
PublicKey = QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=
AllowedIPs = 0.0.0.0/0
Endpoint = 198.51.100.4:51820
`

// amneziaContainer wraps an INI text the way Amnezia stores it: the proto
// section holds last_config — a JSON *string* whose "config" field is the INI.
func amneziaContainer(t *testing.T, name, protoKey, ini string) map[string]interface{} {
	t.Helper()
	lastConfig, err := json.Marshal(map[string]interface{}{"config": ini})
	if err != nil {
		t.Fatalf("marshal last_config: %v", err)
	}
	return map[string]interface{}{
		"container": name,
		protoKey:    map[string]interface{}{"last_config": string(lastConfig), "port": "38291"},
	}
}

func TestParseNode_AmneziaVPN_AWG(t *testing.T) {
	link := buildVPNLink(t, map[string]interface{}{
		"containers":       []interface{}{amneziaContainer(t, "amnezia-awg", "awg", amneziaAWGIni)},
		"defaultContainer": "amnezia-awg",
		"hostName":         "203.0.113.7",
		"description":      "Seliv AWG",
	})
	node, err := ParseNode(link, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	if node.Scheme != "wireguard" {
		t.Errorf("Scheme = %q, want wireguard", node.Scheme)
	}
	if node.Server != "203.0.113.7" || node.Port != 38291 {
		t.Errorf("endpoint = %s:%d, want 203.0.113.7:38291", node.Server, node.Port)
	}
	if node.Tag != "Seliv AWG" {
		t.Errorf("Tag = %q, want description-based label", node.Tag)
	}
	if got, _ := node.Outbound["private_key"].(string); got != "UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=" {
		t.Errorf("private_key mismatch: %q", got)
	}
	wantNum := map[string]int64{"jc": 4, "jmin": 40, "jmax": 70, "s1": 116, "s2": 61,
		"h1": 1239197098, "h2": 1929999940, "h3": 1499605721, "h4": 992706287}
	for k, want := range wantNum {
		if got, _ := node.Outbound[k].(int64); got != want {
			t.Errorf("%s = %v (%T), want %d", k, node.Outbound[k], node.Outbound[k], want)
		}
	}
	if got, _ := node.Outbound["i1"].(string); got != "<b 0x000100002112a442><r 12>" {
		t.Errorf("i1 mismatch: %q", got)
	}
	// MTU из .conf Amnezia доезжает как записан; потолок 1280 у AWG-узла
	// накладывает санитайзер по телу (max_when), а не конвертер .conf.
	if got, _ := node.Outbound["mtu"].(int); got != 1420 {
		t.Errorf("mtu = %v, want 1420 verbatim (потолок — правило реестра)", node.Outbound["mtu"])
	}
	peers, _ := node.Outbound["peers"].([]map[string]interface{})
	if len(peers) != 1 {
		t.Fatalf("peers = %v, want exactly 1", node.Outbound["peers"])
	}
	if got, _ := peers[0]["pre_shared_key"].(string); got != "UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=" {
		t.Errorf("pre_shared_key mismatch: %q", got)
	}
	if got, _ := peers[0]["persistent_keepalive_interval"].(int); got != 25 {
		t.Errorf("keepalive = %v, want 25", peers[0]["persistent_keepalive_interval"])
	}
}

func TestParseNode_AmneziaVPN_PlainWG(t *testing.T) {
	link := buildVPNLink(t, map[string]interface{}{
		"containers":       []interface{}{amneziaContainer(t, "amnezia-wireguard", "wireguard", amneziaPlainWGIni)},
		"defaultContainer": "amnezia-wireguard",
		"hostName":         "198.51.100.4",
	})
	node, err := ParseNode(link, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	for _, k := range append(append([]string{}, awgNumericFields...), awgStringFields...) {
		if _, ok := node.Outbound[k]; ok {
			t.Errorf("plain WG profile gained AWG key %q", k)
		}
	}
	// No AWG fields → no clamp, the conf MTU is honored.
	if got, _ := node.Outbound["mtu"].(int); got != 1380 {
		t.Errorf("mtu = %v, want 1380 from conf", node.Outbound["mtu"])
	}
	if node.Tag != "198.51.100.4" {
		t.Errorf("Tag = %q, want hostName fallback", node.Tag)
	}
}

func TestParseNode_AmneziaVPN_DefaultContainerPreferred(t *testing.T) {
	other := amneziaContainer(t, "amnezia-wireguard", "wireguard", amneziaPlainWGIni)
	link := buildVPNLink(t, map[string]interface{}{
		"containers": []interface{}{
			other, // array order would pick this one (endpoint 198.51.100.4)
			amneziaContainer(t, "amnezia-awg", "awg", amneziaAWGIni),
		},
		"defaultContainer": "amnezia-awg",
	})
	node, err := ParseNode(link, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v", err)
	}
	if node.Server != "203.0.113.7" {
		t.Errorf("Server = %q, want the defaultContainer's 203.0.113.7", node.Server)
	}
}

func TestParseNode_AmneziaVPN_NoWGContainer(t *testing.T) {
	link := buildVPNLink(t, map[string]interface{}{
		"containers": []interface{}{
			map[string]interface{}{
				"container": "amnezia-openvpn",
				"openvpn":   map[string]interface{}{"last_config": `{"config":"client\nremote 1.2.3.4 1194"}`},
			},
		},
		"defaultContainer": "amnezia-openvpn",
	})
	_, err := ParseNode(link, nil)
	if err == nil {
		t.Fatal("expected error for profile without WG/AWG container")
	}
	if !strings.Contains(err.Error(), "amnezia-openvpn") {
		t.Errorf("error should name the containers, got: %v", err)
	}
}

func TestParseNode_AmneziaVPN_Garbage(t *testing.T) {
	for name, link := range map[string]string{
		"bad base64":    "vpn://%%%не-base64%%%",
		"empty":         "vpn://",
		"too short":     "vpn://AAAA",
		"not zlib":      "vpn://" + base64.RawURLEncoding.EncodeToString([]byte{0, 0, 0, 10, 1, 2, 3, 4, 5}),
		"zero size hdr": "vpn://" + base64.RawURLEncoding.EncodeToString([]byte{0, 0, 0, 0, 0x78, 0x9c, 1, 2}),
		"declared bomb": "vpn://" + base64.RawURLEncoding.EncodeToString([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x78, 0x9c, 1, 2}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseNode(link, nil); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestParseNode_AmneziaVPN_NotJSONPayload(t *testing.T) {
	data := []byte("definitely not json")
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(data)))
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(data)
	_ = zw.Close()
	link := "vpn://" + base64.RawURLEncoding.EncodeToString(buf.Bytes())
	if _, err := ParseNode(link, nil); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Errorf("want JSON error, got: %v", err)
	}
}

func TestIsDirectLink_VPN(t *testing.T) {
	if !IsDirectLink("vpn://AAAA") {
		t.Error("vpn:// should be a direct link")
	}
}

// Whitespace tolerance: links copied from chats arrive wrapped across lines.
func TestParseNode_AmneziaVPN_WrappedBase64(t *testing.T) {
	link := buildVPNLink(t, map[string]interface{}{
		"containers":       []interface{}{amneziaContainer(t, "amnezia-awg", "awg", amneziaAWGIni)},
		"defaultContainer": "amnezia-awg",
	})
	payload := strings.TrimPrefix(link, "vpn://")
	var wrapped strings.Builder
	wrapped.WriteString("vpn://")
	for i, r := range payload {
		if i > 0 && i%60 == 0 {
			wrapped.WriteString("\n")
		}
		wrapped.WriteRune(r)
	}
	node, err := ParseNode(wrapped.String(), nil)
	if err != nil || node == nil {
		t.Fatalf("wrapped link must still parse: err=%v", err)
	}
}

// amneziaAWG3Ini — .conf AWG 3.1-контейнера: поверх AWG2-набора защита
// заголовка, паддинг содержимого, тайминги диапазонами, хвосты и cookie.
// MTU здесь НЕТ намеренно — Amnezia кладёт его в last_config рядом с config.
// DNS — плейсхолдеры, реальные адреса лежат в корне профиля. Ключи синтетические.
const amneziaAWG3Ini = `[Interface]
Address = 10.8.1.7/32
DNS = $PRIMARY_DNS, $SECONDARY_DNS
PrivateKey = UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=
Jc = 4
Jmin = 10
Jmax = 50
S1 = 55
S2 = 42
S3 = 40
S4 = 12
H1 = 1
H2 = 2
H3 = 3
H4 = 4
HeaderProtectionKey = Bw4VHCMqMTg/Rk1UW2JpcHd+hYyTmqGor7a9xMvS2eA=
ContentPaddingAddition = 10-100
RekeyAfterTime = 100-120
RekeyTimeout = 3-7
RejectAfterTime = 150-180
KeepaliveTimeout = 5-15
MaxHandshakeAttempts = 15-20
RandomTrailers = on
DisableCookies = on

[Peer]
PublicKey = QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=
PresharedKey = UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.9:30565
PersistentKeepalive = 25-35
`

// amneziaAWG3Container повторяет форму экспорта AWG 3.x: last_config —
// ОБЪЕКТ (а не JSON-строка, как у AWG2), в нём рядом с config лежат mtu и
// hostName. Именно оттуда берётся MTU: в [Interface] его нет.
func amneziaAWG3Container() map[string]interface{} {
	return map[string]interface{}{
		"container": "amnezia-awg2",
		"awg": map[string]interface{}{
			"port":             "30565",
			"protocol_version": "3.1",
			"transport_proto":  "udp",
			"last_config": map[string]interface{}{
				"config":                amneziaAWG3Ini,
				"mtu":                   "1376",
				"hostName":              "203.0.113.9",
				"port":                  float64(30565),
				"persistent_keep_alive": "25-35",
				"allowed_ips":           []interface{}{"0.0.0.0/0", "::/0"},
				"client_ip":             "10.8.1.7",
			},
		},
	}
}

// SPEC 123: импорт AWG 3.1-профиля Amnezia. Проверяет весь путь целиком —
// .conf → URI → endpoint: AWG3-поля на корне с нужными типами, MTU из
// last_config (потолок накладывает реестр, не этот путь), диапазонный keepalive строкой и подстановку
// $PRIMARY_DNS/$SECONDARY_DNS из корня профиля.
func TestParseNode_AmneziaVPN_AWG3(t *testing.T) {
	profile := map[string]interface{}{
		"containers":       []interface{}{amneziaAWG3Container()},
		"defaultContainer": "amnezia-awg2",
		"hostName":         "203.0.113.9",
		"description":      "AWG3 Node",
		"dns1":             "172.29.172.254",
		"dns2":             "1.0.0.1",
	}
	link := buildVPNLink(t, profile)
	node, err := ParseNode(link, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	if node.Scheme != "wireguard" || node.Server != "203.0.113.9" || node.Port != 30565 {
		t.Fatalf("endpoint = %s %s:%d, want wireguard 203.0.113.9:30565", node.Scheme, node.Server, node.Port)
	}
	if !HasAWG3Fields(node.Outbound) {
		t.Errorf("HasAWG3Fields = false for an AWG 3.1 import: %v", node.Outbound)
	}
	if got, _ := node.Outbound["header_protection_key"].(string); got != "Bw4VHCMqMTg/Rk1UW2JpcHd+hYyTmqGor7a9xMvS2eA=" {
		t.Errorf("header_protection_key = %q, want the base64 from the .conf verbatim ('+'/'/' intact)", got)
	}
	// Диапазоны — строками, одиночные значения остались бы числами.
	wantRanges := map[string]string{
		"content_padding_addition": "10-100",
		"rekey_after_time":         "100-120",
		"rekey_timeout":            "3-7",
		"reject_after_time":        "150-180",
		"keepalive_timeout":        "5-15",
		"max_handshake_attempts":   "15-20",
	}
	for k, want := range wantRanges {
		if got, _ := node.Outbound[k].(string); got != want {
			t.Errorf("%s = %v (%T), want string %q", k, node.Outbound[k], node.Outbound[k], want)
		}
	}
	for _, k := range []string{"random_trailers", "disable_cookies"} {
		if got, _ := node.Outbound[k].(bool); !got {
			t.Errorf("%s = %v, want true", k, node.Outbound[k])
		}
	}
	// MTU лежит в last_config, а не в [Interface] — и доезжает оттуда как
	// записан (1376). Потолок 1280 накладывает уже санитайзер по телу.
	if got, _ := node.Outbound["mtu"].(int); got != 1376 {
		t.Errorf("mtu = %v (%T), want last_config 1376 verbatim", node.Outbound["mtu"], node.Outbound["mtu"])
	}
	peers, _ := node.Outbound["peers"].([]map[string]interface{})
	if len(peers) != 1 {
		t.Fatalf("peers = %v, want exactly 1", node.Outbound["peers"])
	}
	if got, _ := peers[0]["persistent_keepalive_interval"].(string); got != "25-35" {
		t.Errorf("persistent_keepalive_interval = %v (%T), want string \"25-35\"",
			peers[0]["persistent_keepalive_interval"], peers[0]["persistent_keepalive_interval"])
	}
	// Плейсхолдеры Amnezia разрешаются из корня профиля: иначе имя сервера
	// «$PRIMARY_DNS» уезжало в конфиг как есть.
	if got := node.Query.Get("dns"); got != "172.29.172.254,1.0.0.1" {
		t.Errorf("dns = %q, want the profile dns1/dns2 with no $ placeholders", got)
	}
	// Тот же профиль через мульти-импорт обязан дать тот же узел.
	all, _, err := ParseAmneziaVPNLinkAll(link, nil)
	if err != nil || len(all) != 1 {
		t.Fatalf("ParseAmneziaVPNLinkAll: err=%v nodes=%d, want 1", err, len(all))
	}
	single, _ := json.Marshal(node.Outbound)
	multi, _ := json.Marshal(all[0].Outbound)
	if string(single) != string(multi) {
		t.Errorf("ParseAmneziaVPNLinkAll gave a different endpoint:\nsingle=%s\nmulti =%s", single, multi)
	}
}
