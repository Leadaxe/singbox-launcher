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
// не выражается: СТРУКТУРНЫЕ преобразования диалекта (форма obfs, плоский
// masque, снятие tls-блока негодной ФОРМЫ) — работа маппера.
//
// Контракт 1.1.4: TestSanitizeSingboxQUICStripsUTLSAndReality снят вместе со
// своим правилом — срез utls/reality на QUIC переехал в реестр (forbidden_for
// + forbidden_codes → tls_not_applicable_quic), и проверяют его парные кейсы
// корпуса uri/hysteria2/reality_fp_stripped_quic_pair ↔
// body/singbox/hysteria2_quic_tls_pair (и та же пара у tuic). Проверять здесь
// значило бы держать копию правила в тесте после того, как копию сняли
// из кода.

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
