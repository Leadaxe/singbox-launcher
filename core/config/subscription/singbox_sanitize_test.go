package subscription

import "testing"

// SPEC 094 A2 — санитайзы над импортированной sing-box map.
//
// Общий инвариант всех кейсов: битое значение снимает ПОЛЕ (или блок), но
// оставляет узел рабочим. Ядро отвергает конфиг целиком на невалидном
// значении, поэтому «выкинуть ноду» и «пропустить мусор» одинаково плохи.

// SPEC 131 W2c: тесты на ЗНАЧЕНИЯ (utls-отпечаток, REALITY, flow,
// packet_encoding) отсюда сняты вместе с самими проверками — эти правила
// живут в реестре, их исполняет nodeflow.Sanitize, и сверяют их табличный
// тест пакета nodeflow и корпус контракта. Здесь остаётся то, что реестром
// не выражается: СТРУКТУРНЫЕ преобразования диалекта (снятие tls-блока на
// QUIC, форма obfs, плоский masque) — работа маппера.
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
			"obfs": map[string]interface{}{"type": "quicksand", "password": "secret"},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		if _, present := ob["obfs"]; present {
			t.Fatal("unsupported obfs type must be dropped (fatal for the whole config)")
		}
	})

	// gecko is implemented by sing-box-lx (protocol/hysteria2/outbound.go) and
	// accepted by LxBox, so it must survive the import (SPEC 103, D-016(а)).
	t.Run("gecko obfs is kept", func(t *testing.T) {
		ob := map[string]interface{}{
			"type": "hysteria2",
			"obfs": map[string]interface{}{"type": "gecko", "password": "secret"},
		}
		SanitizeSingboxOutboundMap(ob, "n")

		if _, present := ob["obfs"]; !present {
			t.Fatal("gecko obfs must be kept — the core supports it")
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
