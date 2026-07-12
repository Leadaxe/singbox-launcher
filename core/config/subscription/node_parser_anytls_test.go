package subscription

import (
	"testing"
)

// All credentials below are fake placeholders.

func TestParseNode_AnyTLS_Canonical(t *testing.T) {
	uri := "anytls://p%40ssw0rd@any.example.test:8443/?sni=cover.example.com&insecure=1&alpn=h2,http/1.1#AnyTLS-smoke"
	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode(%q) error: %v", uri, err)
	}
	assertEq(t, node.Scheme, "anytls")
	assertEq(t, node.Server, "any.example.test")
	assertEq(t, node.Port, 8443)
	assertEq(t, node.UUID, "p@ssw0rd") // userinfo username = password, URL-decoded
	assertEq(t, node.Label, "AnyTLS-smoke")
}

func TestBuildOutbound_AnyTLS(t *testing.T) {
	uri := "anytls://secret@any.example.test:443/?sni=cover.example.com&insecure=1&fp=chrome&idle_session_timeout=30&min_idle_session=2#node"
	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "anytls-out"
	out := buildOutbound(node)

	assertEq(t, out["type"], "anytls")
	assertEq(t, out["tag"], "anytls-out")
	assertEq(t, out["server"], "any.example.test")
	assertEq(t, out["server_port"], 443)
	assertEq(t, out["password"], "secret")
	assertEq(t, out["idle_session_timeout"], "30s") // bare int → seconds
	assertEq(t, out["min_idle_session"], 2)

	tls, ok := out["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("tls block missing: %#v", out["tls"])
	}
	assertEq(t, tls["enabled"], true)
	assertEq(t, tls["server_name"], "cover.example.com")
	assertEq(t, tls["insecure"], true)
	if utls, ok := tls["utls"].(map[string]interface{}); !ok || utls["fingerprint"] != "chrome" {
		t.Errorf("utls fingerprint = %#v, want chrome", tls["utls"])
	}
}

func TestAnyTLS_MissingUserinfoRejected(t *testing.T) {
	if _, err := ParseNode("anytls://any.example.test:443#x", nil); err == nil {
		t.Fatal("expected error for anytls URI without userinfo (password)")
	}
}

func TestAnyTLS_ShareURIRoundTrip(t *testing.T) {
	uri := "anytls://secret@any.example.test:443/?sni=cover.example.com&insecure=1&alpn=h2#node"
	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "rt"
	out := buildOutbound(node)

	share, err := ShareURIFromOutbound(out)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	node2, err := ParseNode(share, nil)
	if err != nil {
		t.Fatalf("reparse %q: %v", share, err)
	}
	out2 := buildOutbound(node2)
	for _, k := range []string{"type", "server", "server_port", "password"} {
		if out[k] != out2[k] {
			t.Errorf("round-trip %s: %v != %v", k, out[k], out2[k])
		}
	}
	tls1 := out["tls"].(map[string]interface{})
	tls2 := out2["tls"].(map[string]interface{})
	if tls1["server_name"] != tls2["server_name"] || tls1["insecure"] != tls2["insecure"] {
		t.Errorf("round-trip tls mismatch: %#v vs %#v", tls1, tls2)
	}
}
