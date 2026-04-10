package subscription

import (
	_ "embed"
	"testing"
)

//go:embed testdata/xray_provider_anon.json
var xrayProviderAnonJSON string

func TestParseNodesFromXrayJSONArray_ProviderStyleFixture(t *testing.T) {
	nodes, err := ParseNodesFromXrayJSONArray(xrayProviderAnonJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}

	a := nodes[0]
	if want := "🇱🇻 Sample-A | fixture"; a.Label != want {
		t.Fatalf("node0 label: got %q want %q", a.Label, want)
	}
	if a.Server != "exit-node-a.example.invalid" || a.Port != 443 {
		t.Fatalf("node0 server: %s:%d", a.Server, a.Port)
	}
	if a.UUID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("node0 uuid: %s", a.UUID)
	}
	wantBase := xrayRemarksToTagBase("🇱🇻 Sample-A | fixture", 0)
	if a.Tag != wantBase {
		t.Fatalf("node0 tag: got %q want %q", a.Tag, wantBase)
	}
	if a.Jump == nil || a.Jump.Tag != wantBase+xrayJumpOutboundTagSuffix {
		t.Fatalf("node0 jump tag: got %v want %q", a.Jump, wantBase+xrayJumpOutboundTagSuffix)
	}
	if a.Jump.Scheme != "socks" {
		t.Fatalf("node0 jump scheme: got %q want socks", a.Jump.Scheme)
	}
	if a.Jump.Server != "198.51.100.10" || a.Jump.Port != 61000 {
		t.Fatalf("node0 jump addr: %s:%d", a.Jump.Server, a.Jump.Port)
	}
	if a.Jump.Outbound["username"] != "anonuser" || a.Jump.Outbound["password"] != "anonpass" {
		t.Fatalf("node0 jump auth: %+v", a.Jump.Outbound)
	}
	tls, ok := a.Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("node0 missing tls")
	}
	rel, _ := tls["reality"].(map[string]interface{})
	if rel["public_key"] != "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX" {
		t.Fatalf("node0 reality public_key: %v", rel["public_key"])
	}
	if tls["server_name"] != "sni.example.invalid" {
		t.Fatalf("node0 tls server_name: %v", tls["server_name"])
	}

	b := nodes[1]
	if want := "🇫🇷 Sample-B | anonymous jump"; b.Label != want {
		t.Fatalf("node1 label: got %q want %q", b.Label, want)
	}
	if b.Server != "exit-node-b.example.invalid" {
		t.Fatalf("node1 server: %s", b.Server)
	}
	wantBaseB := xrayRemarksToTagBase("🇫🇷 Sample-B | anonymous jump", 1)
	if b.Tag != wantBaseB {
		t.Fatalf("node1 tag: got %q want %q", b.Tag, wantBaseB)
	}
	if b.Jump == nil || b.Jump.Tag != wantBaseB+xrayJumpOutboundTagSuffix {
		t.Fatalf("node1 jump tag: got %v", b.Jump)
	}
	if b.Jump.Server != "198.51.100.11" || b.Jump.Port != 61001 {
		t.Fatalf("node1 jump addr: %+v", b.Jump)
	}
	if _, ok := b.Jump.Outbound["username"]; ok {
		t.Fatal("node1 anonymous SOCKS must omit username")
	}
}

func TestParseNodesFromXrayJSONArray_BrokenDialerSkipped(t *testing.T) {
	raw := `[
	  {
		"remarks": "bad",
		"outbounds": [
		  {
			"protocol": "vless",
			"tag": "proxy",
			"settings": {
			  "vnext": [{ "address": "x.test", "port": 443, "users": [{ "id": "33333333-3333-3333-3333-333333333333", "encryption": "none", "flow": "xtls-rprx-vision" }] }]
			},
			"streamSettings": {
			  "network": "tcp",
			  "security": "reality",
			  "realitySettings": { "publicKey": "X", "serverName": "sni.test", "shortId": "01" },
			  "sockopt": { "dialerProxy": "missing-tag" }
			}
		  }
		]
	  },
	  {
		"remarks": "good",
		"outbounds": [
		  {
			"protocol": "vless",
			"tag": "proxy",
			"settings": {
			  "vnext": [{ "address": "y.test", "port": 443, "users": [{ "id": "44444444-4444-4444-4444-444444444444", "encryption": "none", "flow": "xtls-rprx-vision" }] }]
			},
			"streamSettings": {
			  "network": "tcp",
			  "security": "reality",
			  "realitySettings": { "publicKey": "Y", "serverName": "sni.test", "shortId": "02" },
			  "sockopt": { "dialerProxy": "j" }
			}
		  },
		  {
			"protocol": "socks",
			"tag": "j",
			"settings": { "servers": [{ "address": "127.0.0.1", "port": 9999 }] }
		  }
		]
	  }
	]`
	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node (broken skipped), got %d", len(nodes))
	}
	if nodes[0].Label != "good" {
		t.Fatalf("label: %q", nodes[0].Label)
	}
}

func TestParseNodesFromXrayJSONArray_VLESSJump(t *testing.T) {
	raw := `[
	  {
		"remarks": "vless-jump-chain",
		"outbounds": [
		  {
			"protocol": "vless",
			"tag": "hop-vless",
			"settings": {
			  "vnext": [{ "address": "198.51.100.20", "port": 443, "users": [{ "id": "11111111-1111-1111-1111-111111111111", "encryption": "none", "flow": "xtls-rprx-vision" }] }]
			},
			"streamSettings": {
			  "network": "tcp",
			  "security": "reality",
			  "realitySettings": { "publicKey": "YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY", "serverName": "jump.sni.test", "shortId": "01", "fingerprint": "chrome" }
			}
		  },
		  {
			"protocol": "vless",
			"tag": "proxy",
			"settings": {
			  "vnext": [{ "address": "198.51.100.21", "port": 443, "users": [{ "id": "22222222-2222-2222-2222-222222222222", "encryption": "none", "flow": "xtls-rprx-vision" }] }]
			},
			"streamSettings": {
			  "network": "tcp",
			  "security": "reality",
			  "realitySettings": { "publicKey": "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ", "serverName": "main.sni.test", "shortId": "02", "fingerprint": "chrome" },
			  "sockopt": { "dialerProxy": "hop-vless" }
			}
		  }
		]
	  }
	]`
	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Jump == nil {
		t.Fatal("expected Jump")
	}
	if n.Jump.Scheme != "vless" {
		t.Fatalf("jump scheme: got %q want vless", n.Jump.Scheme)
	}
	if n.Jump.Server != "198.51.100.20" || n.Jump.Port != 443 {
		t.Fatalf("jump server: %s:%d", n.Jump.Server, n.Jump.Port)
	}
	if n.Jump.UUID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("jump uuid: %s", n.Jump.UUID)
	}
	if n.Server != "198.51.100.21" {
		t.Fatalf("main server: %s", n.Server)
	}
}

func TestParseNodesFromXrayJSONArray_UnsupportedJumpProtocolSkipped(t *testing.T) {
	raw := `[
	  {
		"remarks": "trojan-jump",
		"outbounds": [
		  {
			"protocol": "trojan",
			"tag": "jump-tj",
			"settings": { "servers": [{ "address": "a.test", "port": 443, "password": "secret" }] }
		  },
		  {
			"protocol": "vless",
			"tag": "proxy",
			"settings": {
			  "vnext": [{ "address": "b.test", "port": 443, "users": [{ "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "encryption": "none", "flow": "xtls-rprx-vision" }] }]
			},
			"streamSettings": {
			  "network": "tcp",
			  "security": "reality",
			  "realitySettings": { "publicKey": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", "serverName": "sni.test", "shortId": "01", "fingerprint": "chrome" },
			  "sockopt": { "dialerProxy": "jump-tj" }
			}
		  }
		]
	  }
	]`
	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("want 0 nodes when jump protocol unsupported, got %d", len(nodes))
	}
}

func TestParseNodesFromXrayJSONArray_SingBoxOnlySkipped(t *testing.T) {
	raw := `[{"outbounds":[{"type":"direct","tag":"d"}]}]`
	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("want 0 nodes, got %d", len(nodes))
	}
}

func TestIsXrayJSONArrayBody(t *testing.T) {
	if !IsXrayJSONArrayBody(`  [{"remarks":"x","outbounds":[]}]  `) {
		t.Fatal("expected valid array body")
	}
	if IsXrayJSONArrayBody(`{"a":1}`) {
		t.Fatal("object is not array subscription body")
	}
}

func TestXrayRemarksToTagBase(t *testing.T) {
	if got := xrayRemarksToTagBase("🇱🇻 Sample-A | fixture", 0); got != "🇱🇻-Sample-A-fixture" {
		t.Fatalf("got %q", got)
	}
	if got := xrayRemarksToTagBase("", 3); got != "xray-3" {
		t.Fatalf("empty remarks: got %q", got)
	}
	if got := xrayRemarksToTagBase("!!!", 0); got != "xray-0" {
		t.Fatalf("only punctuation: got %q", got)
	}
}
