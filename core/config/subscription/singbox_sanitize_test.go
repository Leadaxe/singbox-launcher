package subscription

import "testing"

// SPEC 094 A2 — санитайзы над импортированной sing-box map.
//
// Общий инвариант всех кейсов: битое значение снимает ПОЛЕ (или блок), но
// оставляет узел рабочим. Ядро отвергает конфиг целиком на невалидном
// значении, поэтому «выкинуть ноду» и «пропустить мусор» одинаково плохи.

func TestSanitizeSingboxUTLSFingerprint(t *testing.T) {
	t.Run("xray alias is canonicalized (HelloChrome_120 → chrome)", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "vless",
			"tls": map[string]interface{}{
				"enabled": true,
				"utls":    map[string]interface{}{"enabled": true, "fingerprint": "HelloChrome_120"},
			},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		tls := ob["tls"].(map[string]interface{})
		utls, ok := tls["utls"].(map[string]interface{})
		if !ok {
			t.Fatal("utls block must survive a known alias")
		}
		if utls["fingerprint"] != "chrome" {
			t.Fatalf("fingerprint = %v, want chrome", utls["fingerprint"])
		}
	})

	t.Run("unknown fingerprint drops the utls block, node survives", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "vless",
			"tls": map[string]interface{}{
				"enabled": true,
				"utls":    map[string]interface{}{"enabled": true, "fingerprint": "totally-bogus"},
			},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		tls := ob["tls"].(map[string]interface{})
		if _, present := tls["utls"]; present {
			t.Fatal("unknown fingerprint must drop the utls block")
		}
		if tls["enabled"] != true {
			t.Fatal("tls block itself must survive")
		}
	})
}

func TestSanitizeSingboxReality(t *testing.T) {
	const validPBK = "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0"

	t.Run("invalid public_key degrades node to plain TLS", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "vless",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "example.com",
				"reality":     map[string]interface{}{"enabled": true, "public_key": "enabled"},
			},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		tls := ob["tls"].(map[string]interface{})
		if _, present := tls["reality"]; present {
			t.Fatal("invalid pbk must drop the reality block")
		}
		if tls["server_name"] != "example.com" {
			t.Fatal("plain TLS settings must survive")
		}
	})

	t.Run("valid public_key is kept", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "vless",
			"tls": map[string]interface{}{
				"enabled": true,
				"reality": map[string]interface{}{"enabled": true, "public_key": validPBK},
			},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		tls := ob["tls"].(map[string]interface{})
		if _, present := tls["reality"]; !present {
			t.Fatal("valid reality block must survive")
		}
	})

	t.Run("junk short_id is dropped, hex is normalized", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "vless",
			"tls": map[string]interface{}{
				"enabled": true,
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": validPBK,
					"short_id":   "AB CD",
				},
			},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		reality := ob["tls"].(map[string]interface{})["reality"].(map[string]interface{})
		if reality["short_id"] != "abcd" {
			t.Fatalf("short_id = %v, want abcd", reality["short_id"])
		}
	})

	t.Run("non-hex short_id is removed entirely", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "vless",
			"tls": map[string]interface{}{
				"enabled": true,
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": validPBK,
					"short_id":   "zzz",
				},
			},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		reality := ob["tls"].(map[string]interface{})["reality"].(map[string]interface{})
		if _, present := reality["short_id"]; present {
			t.Fatalf("non-hex short_id must be removed, got %v", reality["short_id"])
		}
	})
}

func TestSanitizeSingboxQUICStripsUTLSAndReality(t *testing.T) {
	for _, quicType := range []string{"hysteria2", "tuic"} {
		t.Run(quicType+": utls and reality are stripped", func(t *testing.T) {
			ob := map[string]interface{}{
				"type": quicType,
				"tls": map[string]interface{}{
					"enabled":     true,
					"server_name": "example.com",
					"utls":        map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
					"reality":     map[string]interface{}{"enabled": true, "public_key": "x"},
				},
			}
			SanitizeSingboxOutboundMap(ob, "n")

			tls := ob["tls"].(map[string]interface{})
			if _, present := tls["utls"]; present {
				t.Error("utls must be stripped on QUIC outbounds")
			}
			if _, present := tls["reality"]; present {
				t.Error("reality must be stripped on QUIC outbounds")
			}
			if tls["server_name"] != "example.com" {
				t.Error("server_name must survive")
			}
		})
	}
}

func TestSanitizeSingboxTLSDisabledBlockRemoved(t *testing.T) {
	// SPEC 045: явный tls:{enabled:false} роняет ядро SIGSEGV'ом при dial.
	ob := map[string]interface{}{
		"type": "vless",
		"tls":  map[string]interface{}{"enabled": false},
	}
	SanitizeSingboxOutboundMap(ob, "n")

	if _, present := ob["tls"]; present {
		t.Fatal("tls:{enabled:false} must be removed entirely")
	}
}

func TestSanitizeSingboxFlow(t *testing.T) {
	tests := []struct {
		name      string
		ob        map[string]interface{}
		wantFlow  string
		wantThere bool
	}{
		{
			name:      "vision on bare TLS is kept",
			ob:        map[string]interface{}{"type": "vless", "flow": "xtls-rprx-vision"},
			wantFlow:  "xtls-rprx-vision",
			wantThere: true,
		},
		{
			name: "vision with transport is dropped",
			ob: map[string]interface{}{
				"type":      "vless",
				"flow":      "xtls-rprx-vision",
				"transport": map[string]interface{}{"type": "ws"},
			},
			wantThere: false,
		},
		{
			name:      "deprecated flow is dropped",
			ob:        map[string]interface{}{"type": "vless", "flow": "xtls-rprx-direct"},
			wantThere: false,
		},
		{
			name:      "none is dropped",
			ob:        map[string]interface{}{"type": "vless", "flow": "none"},
			wantThere: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SanitizeSingboxOutboundMap(tt.ob, "n")
			got, present := tt.ob["flow"]
			if present != tt.wantThere {
				t.Fatalf("flow present = %v, want %v (value %v)", present, tt.wantThere, got)
			}
			if tt.wantThere && got != tt.wantFlow {
				t.Fatalf("flow = %v, want %v", got, tt.wantFlow)
			}
		})
	}
}

func TestSanitizeSingboxPacketEncoding(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		want      string
		wantThere bool
	}{
		{name: "xudp kept", value: "xudp", want: "xudp", wantThere: true},
		{name: "packetaddr kept", value: "packetaddr", want: "packetaddr", wantThere: true},
		{name: "none dropped", value: "none", wantThere: false},
		{name: "garbage dropped", value: "teleport", wantThere: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ob := map[string]interface{}{"type": "vless", "packet_encoding": tt.value}
			SanitizeSingboxOutboundMap(ob, "n")

			got, present := ob["packet_encoding"]
			if present != tt.wantThere {
				t.Fatalf("packet_encoding present = %v, want %v (value %v)", present, tt.wantThere, got)
			}
			if tt.wantThere && got != tt.want {
				t.Fatalf("packet_encoding = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeSingboxHysteria2Obfs(t *testing.T) {
	t.Run("salamander with password is kept", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "hysteria2",
			"obfs": map[string]interface{}{"type": "salamander", "password": "secret"},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		if _, present := ob["obfs"]; !present {
			t.Fatal("valid obfs must survive")
		}
	})

	t.Run("unsupported obfs type is dropped", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "hysteria2",
			"obfs": map[string]interface{}{"type": "gecko", "password": "secret"},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		if _, present := ob["obfs"]; present {
			t.Fatal("unsupported obfs type must be dropped (fatal for the whole config)")
		}
	})

	t.Run("obfs without password is dropped", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "hysteria2",
			"obfs": map[string]interface{}{"type": "salamander"},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		if _, present := ob["obfs"]; present {
			t.Fatal("obfs without password must be dropped")
		}
	})
}

func TestSanitizeSingboxHandlesMalformedBlocks(t *testing.T) {
	// tls не объект: ядро отвергло бы конфиг, поле снимается.
	ob := map[string]interface{}{"type": "vless", "tls": "yes-please"}
	SanitizeSingboxOutboundMap(ob, "n")
	if _, present := ob["tls"]; present {
		t.Fatal("non-object tls must be dropped")
	}

	// nil-map не должна паниковать.
	SanitizeSingboxOutboundMap(nil, "n")
}

func TestIsSingboxServiceAndGroupTypes(t *testing.T) {
	for _, s := range []string{"direct", "block", "dns", "DIRECT", " block "} {
		if !IsSingboxServiceType(s) {
			t.Errorf("IsSingboxServiceType(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"vless", "selector", "urltest", ""} {
		if IsSingboxServiceType(s) {
			t.Errorf("IsSingboxServiceType(%q) = true, want false", s)
		}
	}
	for _, s := range []string{"selector", "urltest", "URLTest"} {
		if !IsSingboxGroupType(s) {
			t.Errorf("IsSingboxGroupType(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"vless", "direct", ""} {
		if IsSingboxGroupType(s) {
			t.Errorf("IsSingboxGroupType(%q) = true, want false", s)
		}
	}
}
