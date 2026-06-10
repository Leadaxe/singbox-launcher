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
PrivateKey = UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMA=
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
PublicKey = QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=
PresharedKey = UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.7:38291
PersistentKeepalive = 25
`

const amneziaPlainWGIni = `[Interface]
Address = 10.8.1.2/32
PrivateKey = UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMA=
MTU = 1380

[Peer]
PublicKey = QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=
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
	if got, _ := node.Outbound["private_key"].(string); got != "UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMA=" {
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
	// AWG endpoint: MTU=1420 from the Amnezia conf must be clamped (SPEC 073).
	if got, _ := node.Outbound["mtu"].(int); got != 1280 {
		t.Errorf("mtu = %v, want clamped 1280", node.Outbound["mtu"])
	}
	peers, _ := node.Outbound["peers"].([]map[string]interface{})
	if len(peers) != 1 {
		t.Fatalf("peers = %v, want exactly 1", node.Outbound["peers"])
	}
	if got, _ := peers[0]["pre_shared_key"].(string); got != "UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=" {
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
