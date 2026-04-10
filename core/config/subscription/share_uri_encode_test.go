package subscription

import (
	"errors"
	"strings"
	"testing"
)

func TestShareURIFromOutbound_RoundTripVLESS(t *testing.T) {
	uri := "vless://550e8400-e29b-41d4-a716-446655440000@example.com:443?encryption=none&security=tls&sni=example.com#tag1"
	n, err := ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	got, err := ShareURIFromOutbound(n.Outbound)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	n2, err := ParseNode(got, nil)
	if err != nil || n2 == nil {
		t.Fatalf("ParseNode second: %v uri=%q", err, got)
	}
	if n.Server != n2.Server || n.Port != n2.Port || n.UUID != n2.UUID || n.Tag != n2.Tag {
		t.Fatalf("mismatch server=%q/%q port=%d/%d uuid=%q/%q tag=%q/%q",
			n.Server, n2.Server, n.Port, n2.Port, n.UUID, n2.UUID, n.Tag, n2.Tag)
	}
}

func TestShareURIFromOutbound_RoundTripTrojan(t *testing.T) {
	uri := "trojan://secretpass@example.com:443?sni=example.com#tr1"
	n, err := ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	got, err := ShareURIFromOutbound(n.Outbound)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	n2, err := ParseNode(got, nil)
	if err != nil || n2 == nil {
		t.Fatalf("ParseNode second: %v uri=%q", err, got)
	}
	if n.Server != n2.Server || n.Port != n2.Port || n.UUID != n2.UUID {
		t.Fatalf("mismatch %+v vs %+v", n, n2)
	}
}

func TestShareURIFromOutbound_RoundTripShadowsocks(t *testing.T) {
	uri := "ss://YWVzLTEyOC1nY206c2VjcmV0@192.0.2.1:8388#ss1"
	n, err := ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	got, err := ShareURIFromOutbound(n.Outbound)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	n2, err := ParseNode(got, nil)
	if err != nil || n2 == nil {
		t.Fatalf("ParseNode second: %v uri=%q", err, got)
	}
	if n.Server != n2.Server || n.Port != n2.Port || n.Tag != n2.Tag {
		t.Fatalf("mismatch %+v vs %+v", n, n2)
	}
}

func TestShareURIFromOutbound_Selector(t *testing.T) {
	_, err := ShareURIFromOutbound(map[string]interface{}{
		"type": "selector",
		"tag":  "x",
	})
	if !errors.Is(err, ErrShareURINotSupported) {
		t.Fatalf("want ErrShareURINotSupported, got %v", err)
	}
}

func TestShareURIFromOutbound_DetourChain(t *testing.T) {
	_, err := ShareURIFromOutbound(map[string]interface{}{
		"type":        "vless",
		"tag":         "main",
		"detour":      "jump",
		"server":      "example.com",
		"server_port": 443,
		"uuid":        "00000000-0000-0000-0000-000000000001",
	})
	if !errors.Is(err, ErrShareURINotSupported) {
		t.Fatalf("want ErrShareURINotSupported, got %v", err)
	}
}

func TestShareURIFromOutbound_Socks(t *testing.T) {
	out := map[string]interface{}{
		"type":        "socks",
		"tag":         "s1",
		"server":      "127.0.0.1",
		"server_port": 1080,
		"version":     "5",
		"username":    "u",
		"password":    "p",
	}
	u, err := ShareURIFromOutbound(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "socks5://") {
		t.Fatal(u)
	}
	n, err := ParseNode(u, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if n.Server != "127.0.0.1" || n.Port != 1080 {
		t.Fatalf("got server=%q port=%d", n.Server, n.Port)
	}
}

func TestShareURIFromWireGuardEndpoint_RoundTrip(t *testing.T) {
	uri := "wireguard://aDHCHnkcdMjnq0bF+V4fARkbJBW8cWjuYoVjKfUwsXo=@212.232.78.237:51820?publickey=fiK9ZG990zunr5cpRnx%2BSOVW2rVKKqFoVxmHMHAvAFk%3D&address=10.10.10.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0#wgtest"
	n, err := ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	got, err := ShareURIFromWireGuardEndpoint(n.Outbound)
	if err != nil {
		t.Fatalf("ShareURIFromWireGuardEndpoint: %v", err)
	}
	got2, err := ShareURIFromOutbound(n.Outbound)
	if err != nil || got2 != got {
		t.Fatalf("ShareURIFromOutbound wireguard: err=%v got=%q want=%q", err, got2, got)
	}
	n2, err := ParseNode(got, nil)
	if err != nil || n2 == nil {
		t.Fatalf("ParseNode second: %v uri=%q", err, got)
	}
	if n.Server != n2.Server || n.Port != n2.Port || n.Tag != n2.Tag {
		t.Fatalf("mismatch server=%q/%q port=%d/%d tag=%q/%q", n.Server, n2.Server, n.Port, n2.Port, n.Tag, n2.Tag)
	}
	pk1, _ := n.Outbound["private_key"].(string)
	pk2, _ := n2.Outbound["private_key"].(string)
	if pk1 != pk2 {
		t.Fatalf("private_key mismatch")
	}
}

func TestShareURIFromWireGuardEndpoint_MultiPeer(t *testing.T) {
	ep := map[string]interface{}{
		"type":        "wireguard",
		"tag":         "x",
		"private_key": "aGVsbG8=",
		"address":     []interface{}{"10.0.0.1/32"},
		"peers": []interface{}{
			map[string]interface{}{
				"address": "1.1.1.1", "port": float64(51820),
				"public_key": "YmFy", "allowed_ips": []interface{}{"0.0.0.0/0"},
			},
			map[string]interface{}{
				"address": "2.2.2.2", "port": float64(51820),
				"public_key": "YmF6", "allowed_ips": []interface{}{"0.0.0.0/0"},
			},
		},
	}
	_, err := ShareURIFromWireGuardEndpoint(ep)
	if !errors.Is(err, ErrShareURINotSupported) {
		t.Fatalf("want ErrShareURINotSupported, got %v", err)
	}
}
