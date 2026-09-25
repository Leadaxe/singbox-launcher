package subscription

import (
	"testing"

	"singbox-launcher/core/config/nodeflow"
)

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
// masque) — работа маппера.
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

// tls негодной ФОРМЫ судит реестр (`type: object` секции tls), а не
// рукописная копия на входе sing-box (SPEC 142 A2): не объект — блок снят с
// кодом type_invalid, пустой объект — снят молча, узел жив в обоих случаях.
func TestSanitizeSingboxHandlesMalformedBlocks(t *testing.T) {
	base := func(tls interface{}) map[string]interface{} {
		return map[string]interface{}{
			"type": "vless", "server": "e.com", "server_port": float64(443),
			"uuid": "a0ee37a5-1844-4087-bc5c-1db6f416d38c", "tls": tls,
		}
	}
	res := nodeflow.SanitizeFrom("vless", nodeflow.SourceSingbox, base("yes-please"))
	if res.Drop != nil {
		t.Fatalf("узел с негодным tls отброшен: %+v", res.Drop)
	}
	if _, present := res.Clean["tls"]; present {
		t.Fatal("tls не объект — блок обязан сняться")
	}
	coded := false
	for _, w := range res.Warnings {
		if w.Code == "type_invalid" && w.Path == "tls" {
			coded = true
		}
	}
	if !coded {
		t.Errorf("снятие tls без кода type_invalid: %+v", res.Warnings)
	}

	res = nodeflow.SanitizeFrom("vless", nodeflow.SourceSingbox, base(map[string]interface{}{}))
	if _, present := res.Clean["tls"]; present || res.Drop != nil {
		t.Fatalf("пустой tls обязан сняться, узел жить: clean=%v drop=%+v", res.Clean["tls"], res.Drop)
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
