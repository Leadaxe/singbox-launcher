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

// СНЯТО вместе с правилом (контракт 1.1.12):
// TestSanitizeSingboxTLSDisabledBlockRemoved. «`tls:{enabled:false}` = TLS не
// задан» стало атрибутом реестра `absent_when` у секции tls, и тем же
// атрибутом описаны вложенные utls/reality/ech. Рукописная копия здесь
// работала только на ЭТОМ входе: то же тело, приехавшее ручным JSON вкладки
// или чужим бэкапом, доезжало до конфига с выключенным блоком.
//
// Проверяют правило теперь TestAbsentWhenObjects пакета nodeflow (там же
// норма порядка: снятый объект «не задан» для связей соседей) и кейс корпуса
// body/singbox/tls_disabled_block. Держать проверку здесь значило бы оставить
// копию правила в тесте после того, как копию сняли из кода.

// СНЯТО вместе с правилом (SPEC 131, аудит остатков):
// TestSanitizeSingboxHysteria2Obfs. Все четыре его посылки — enum типов,
// пустой тип, пустой пароль, сохранение gecko — выражены в
// hysteria2.json (body.obfs.type enum + on_invalid, body.obfs.password
// required + code) и проверяются на выходе конвейера. Прежний код снимал
// obfs МОЛЧА; теперь узел получает obfs_unknown / obfs_password_missing —
// проверено прогоном nodeflow.Sanitize на обоих кейсах.

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
