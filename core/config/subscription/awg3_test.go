package subscription

import (
	"reflect"
	"testing"
)

// awg3ValidHeaderKey — синтетический валидный ключ защиты: 32 байта base64
// СО ЗНАКАМИ '+' и '/'. Именно они ловят подмену queryParamPreservePlus на
// q.Get: последний декодирует '+' как пробел и ключ разваливается.
const awg3ValidHeaderKey = "Bw4VHCMqMTg/Rk1UW2JpcHd+hYyTmqGor7a9xMvS2eA="

// awg3URI собирает wireguard://-ссылку с базовым обязательным набором и
// дописанным хвостом AWG3-параметров (уже percent-encoded).
func awg3URI(extra string) string {
	return "wireguard://AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=@host.example.com:30565" +
		"?publickey=AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=&address=10.8.1.7/32" +
		"&allowedips=0.0.0.0/0,::/0" + extra
}

// Негативы AWG 3.x проверяются КОРПУСОМ, а не здесь: правила уехали в реестр
// (контракт 1.1.11, находка №8 LEGACY_AUDIT, запросы LxBox (2) и (4)).
//
//   - негодный header_protection_key (16 байт, все нули, не base64) →
//     on_invalid drop_node, код awg3_header_key_invalid: кейсы
//     uri/wireguard/awg3_header_key_short_dropped и парный ему
//     body/singbox/endpoints_awg3_header_key_zeros;
//   - s1..s4 ниже 12 при заданном ключе (и их полное ОТСУТСТВИЕ — ядро читает
//     незаданное поле как 0) → min_when с absent_is_zero, код
//     awg3_padding_too_short: кейсы uri/wireguard/awg3_padding_absent_with_header_key
//     и body/singbox/endpoints_awg3_padding_too_short;
//   - перевёрнутый диапазон тайминга и небулево значение → on_invalid drop,
//     код awg3_field_invalid: кейс uri/wireguard/awg3_timing_range_reversed_dropped.
//
// Прежний TestParseWireGuardURI_AWG3Negatives проверял рукописный
// validateAWG3, которого больше нет; здесь остаётся только то, что реестром не
// выражается — свойство ПАРЫ настроек, которое ничего не снимает.
func TestParseWireGuardURI_AWG3RandomTrailersWideHeaders(t *testing.T) {
	// h2-h4 отодвинуты, иначе узел упал бы раньше на пересечении заголовков.
	node, err := ParseNode(awg3URI("&h1=1000-200000&h2=300000&h3=300001&h4=300002&randomtrailers=on"), nil)
	if err != nil || node == nil {
		t.Fatalf("node must survive, got err=%v", err)
	}
	if !hasWarning(node.Warnings, WarnAWG3RandomTrailersWideHeaders) {
		t.Errorf("warnings = %v, want %s", node.Warnings, WarnAWG3RandomTrailersWideHeaders)
	}
	if v, _ := node.Outbound["random_trailers"].(bool); !v {
		t.Error("random_trailers must stay set: the info code removes nothing")
	}
}

// SPEC 123 §3.3: эмиттер и парсер ходят парой — endpoint → URI → endpoint для
// полного AWG3-набора обязан дать РАВНЫЙ endpoint. Без эмиссии AWG3-ключей
// узел после пересборки ссылки выглядел бы настроенным и не соединялся.
func TestShareURIFromWireGuardEndpoint_AWG3RoundTrip(t *testing.T) {
	uri := awg3URI("&mtu=1376&keepalive=25-35&jc=4&jmin=10&jmax=50" +
		"&s1=55&s2=42&s3=40&s4=12&h1=1&h2=2&h3=3&h4=4" +
		"&headerprotectionkey=Bw4VHCMqMTg%2FRk1UW2JpcHd%2BhYyTmqGor7a9xMvS2eA%3D" +
		"&contentpaddingaddition=10-100&rekeyaftertime=100-120&rekeytimeout=3-7" +
		"&rejectaftertime=150-180&keepalivetimeout=5-15&maxhandshakeattempts=15-20" +
		"&randomtrailers=on&disablecookies=on#awg3-server")
	first, err := ParseNode(uri, nil)
	if err != nil || first == nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got, _ := first.Outbound["header_protection_key"].(string); got != awg3ValidHeaderKey {
		t.Fatalf("header_protection_key = %q, want %q ('+' must survive the query decode)", got, awg3ValidHeaderKey)
	}
	// MTU ссылки доезжает как записан: потолок 1280 накладывает уже санитайзер
	// по телу (wireguard.body.fields.mtu.max_when, контракт 1.1.5), и здесь
	// проверяется именно round-trip значения, а не правило.
	if got, _ := first.Outbound["mtu"].(int); got != 1376 {
		t.Errorf("mtu = %v, want 1376 verbatim (потолок — правило реестра, не парсера)", first.Outbound["mtu"])
	}
	share, err := ShareURIFromWireGuardEndpoint(first.Outbound)
	if err != nil {
		t.Fatalf("ShareURIFromWireGuardEndpoint: %v", err)
	}
	second, err := ParseNode(share, nil)
	if err != nil || second == nil {
		t.Fatalf("re-parse of %q failed: %v", share, err)
	}
	if !reflect.DeepEqual(first.Outbound, second.Outbound) {
		t.Errorf("round-trip changed the endpoint:\nbefore=%#v\nafter =%#v", first.Outbound, second.Outbound)
	}
}
