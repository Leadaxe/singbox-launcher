package config

import (
	"strings"
	"testing"
)

// TestAuthoredDetourYield — уступка detour через точку правки (контракт
// 1.1.87, PARSING_PRINCIPLES §10.4): у авторского тела мягкая связь
// (tls.fragment) поле оставляет, жёсткая (listen_port WireGuard, отказ ядра
// NewEndpoint) снимает; у обычного тела снимаются обе.
func TestAuthoredDetourYield(t *testing.T) {
	cases := []struct {
		name, scheme, body, field string
		authored, wantKept        bool
	}{
		{"fragment authored kept", "trojan", `{"type":"trojan","server":"example-1.com","server_port":443,"password":"p","tls":{"enabled":true,"fragment":true},"detour":"hop"}`, `"fragment"`, true, true},
		{"fragment plain removed", "trojan", `{"type":"trojan","server":"example-1.com","server_port":443,"password":"p","tls":{"enabled":true,"fragment":true},"detour":"hop"}`, `"fragment"`, false, false},
		{"listen_port authored removed", "wireguard", `{"type":"wireguard","listen_port":51820,"detour":"hop"}`, `"listen_port"`, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			obj, err := decodeOrderedJSONObject([]byte(c.body))
			if err != nil {
				t.Fatal(err)
			}
			yieldBodyToDetour(obj, c.scheme, c.authored)
			got := string(obj.encode())
			if kept := strings.Contains(got, c.field); kept != c.wantKept {
				t.Errorf("%s в теле = %v, want %v: %s", c.field, kept, c.wantKept, got)
			}
		})
	}
}
