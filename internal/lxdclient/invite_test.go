//go:build darwin

package lxdclient

import "testing"

func TestParseInvite(t *testing.T) {
	fp := "3f2a9c0011223344556677889900aabbccddeeff00112233445566778899aabb"
	cases := []struct {
		name    string
		raw     string
		wantErr bool
		addr    string
		code    string
	}{
		{"valid", "127.0.0.1:9091#" + fp + "#K7QM-XXNP-2RTD", false, "127.0.0.1:9091", "K7QM-XXNP-2RTD"},
		{"whitespace", "  127.0.0.1:9091#" + fp + "#K7QM-XXNP-2RTD  ", false, "127.0.0.1:9091", "K7QM-XXNP-2RTD"},
		{"uppercase-fp", "127.0.0.1:9091#" + up(fp) + "#CODE", false, "127.0.0.1:9091", "CODE"},
		{"ipv6", "[::1]:9091#" + fp + "#CODE", false, "[::1]:9091", "CODE"},
		{"empty", "", true, "", ""},
		{"two-parts", "127.0.0.1:9091#" + fp, true, "", ""},
		{"four-parts", "a#b#c#d", true, "", ""},
		{"bad-addr", "not-an-addr#" + fp + "#CODE", true, "", ""},
		{"short-fp", "127.0.0.1:9091#abcd#CODE", true, "", ""},
		{"non-hex-fp", "127.0.0.1:9091#" + nonHex() + "#CODE", true, "", ""},
		{"empty-code", "127.0.0.1:9091#" + fp + "#", true, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv, err := ParseInvite(c.raw)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", inv)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inv.Addr != c.addr {
				t.Errorf("addr = %q, want %q", inv.Addr, c.addr)
			}
			if inv.Code != c.code {
				t.Errorf("code = %q, want %q", inv.Code, c.code)
			}
			if len(inv.ServerFingerprint) != 64 {
				t.Errorf("fingerprint length = %d, want 64", len(inv.ServerFingerprint))
			}
			// Отпечаток всегда нормализуется в lowercase.
			for _, r := range inv.ServerFingerprint {
				if r >= 'A' && r <= 'Z' {
					t.Errorf("fingerprint not lowercased: %s", inv.ServerFingerprint)
					break
				}
			}
		})
	}
}

func up(s string) string {
	out := []byte(s)
	for i, b := range out {
		if b >= 'a' && b <= 'f' {
			out[i] = b - 32
		}
	}
	return string(out)
}

func nonHex() string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = 'z'
	}
	return string(b)
}

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:9091":   true,
		"localhost:9091":   true,
		"[::1]:9091":       true,
		"0.0.0.0:9091":     false,
		"192.168.1.5:9091": false,
		"example.com:9091": false,
		"10.0.0.1:9091":    false,
	}
	for addr, want := range cases {
		if got := IsLoopbackAddr(addr); got != want {
			t.Errorf("IsLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}
