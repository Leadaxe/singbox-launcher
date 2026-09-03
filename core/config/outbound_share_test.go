package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

func TestShareProxyURIForOutboundTag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"outbounds":[{"type":"vless","tag":"n1","uuid":"550e8400-e29b-41d4-a716-446655440000","server":"example.com","server_port":443,"tls":{"enabled":true,"server_name":"example.com"}}]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	u, err := ShareProxyURIForOutboundTag(p, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "vless://") {
		t.Fatalf("expected vless URI, got %q", u)
	}
}

func TestGetOutboundMapByTag_NotFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{"outbounds":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := GetOutboundMapByTag(p, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildShareURILinesForOutboundTags(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"outbounds":[
		{"type":"vless","tag":"n1","uuid":"550e8400-e29b-41d4-a716-446655440000","server":"a.example.com","server_port":443,"tls":{"enabled":true,"server_name":"a.example.com"}},
		{"type":"selector","tag":"sel","outbounds":["n1"]},
		{"type":"vless","tag":"n2","uuid":"650e8400-e29b-41d4-a716-446655440001","server":"b.example.com","server_port":443,"tls":{"enabled":true,"server_name":"b.example.com"}}
	]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	lines, err := BuildShareURILinesForOutboundTags(p, []string{"n1", "missing", "n2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "a.example.com") || !strings.Contains(lines[1], "b.example.com") {
		t.Fatalf("unexpected order or content: %v", lines)
	}
}

func TestShareProxyURIForOutboundTag_WireGuardEndpoint(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"endpoints":[{"type":"wireguard","tag":"wg1","name":"singbox-wg0","system":false,"mtu":1420,"address":["10.10.10.2/32"],"private_key":"RAUTG+IXUH+KW8Ocva7RTpv6y/gdVQIIgh9MeuzeMtU=","peers":[{"address":"198.51.100.7","port":51820,"public_key":"CY6LY0SWr69h/WZokDYecQlTfIsZs8EhdSMd+NuaWJ4=","allowed_ips":["0.0.0.0/0","::/0"]}]}]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	u, err := ShareProxyURIForOutboundTag(p, "wg1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "wireguard://") {
		t.Fatalf("expected wireguard URI, got %q", u)
	}
}

func TestGetDetourTagForOutboundTag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"outbounds":[{"type":"vless","tag":"main","detour":"jump"},{"type":"socks","tag":"jump","server":"127.0.0.1","server_port":1080}]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	d, err := GetDetourTagForOutboundTag(p, "main")
	if err != nil {
		t.Fatal(err)
	}
	if d != "jump" {
		t.Fatalf("want detour jump, got %q", d)
	}
}

func TestShareJumpURIForOutboundTag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"outbounds":[
		{"type":"vless","tag":"main","uuid":"550e8400-e29b-41d4-a716-446655440000","server":"example.com","server_port":443,"detour":"jump"},
		{"type":"socks","tag":"jump","server":"127.0.0.1","server_port":1080,"version":"5"}
	]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	u, err := ShareJumpURIForOutboundTag(p, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "socks5://") && !strings.HasPrefix(u, "socks://") {
		t.Fatalf("expected socks URI, got %q", u)
	}
}

func TestShareMainURIForOutboundTag_ContainsDetourLiteral(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"outbounds":[
		{"type":"vless","tag":"main","uuid":"550e8400-e29b-41d4-a716-446655440000","server":"example.com","server_port":443,"detour":"jump"},
		{"type":"socks","tag":"jump","server":"127.0.0.1","server_port":1080,"version":"5"}
	]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	u, err := ShareMainURIForOutboundTag(p, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "vless://") {
		t.Fatalf("expected vless URI, got %q", u)
	}
	if !strings.Contains(u, "detour=jump") {
		t.Fatalf("expected detour literal in URI, got %q", u)
	}
}

// WireGuard/AmneziaWG узлы лежат в endpoints[], а не в outbounds[]: до фикса
// «Копировать ссылку сервера» падало на них с `outbound with tag ... not found`.
func TestShareMainURIForOutboundTag_WireGuardEndpoint(t *testing.T) {
	uri := "wireguard://RAUTG+IXUH+KW8Ocva7RTpv6y/gdVQIIgh9MeuzeMtU=@198.51.100.7:51820?publickey=CY6LY0SWr69h%2FWZokDYecQlTfIsZs8EhdSMd%2BNuaWJ4%3D&address=10.10.10.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0#server-13"
	n, err := subscription.ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	ep, err := json.Marshal(n.Outbound)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"outbounds":[{"type":"selector","tag":"proxy-out","outbounds":["server-13"]}],"endpoints":[` + string(ep) + `]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	u, err := ShareMainURIForOutboundTag(p, "server-13")
	if err != nil {
		t.Fatalf("ShareMainURIForOutboundTag: %v", err)
	}
	if !strings.HasPrefix(u, "wireguard://") {
		t.Fatalf("expected wireguard URI, got %q", u)
	}
}

// Промах по обоим массивам обязан остаться ошибкой, а не тихо вернуть "".
func TestShareMainURIForOutboundTag_MissingTag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	body := []byte(`{"outbounds":[{"type":"socks","tag":"jump","server":"127.0.0.1","server_port":1080,"version":"5"}]}`)
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ShareMainURIForOutboundTag(p, "nope"); err == nil {
		t.Fatal("expected error for missing tag")
	}
}
