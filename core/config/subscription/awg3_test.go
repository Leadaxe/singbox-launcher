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

// SPEC 123 §2 «Политика ошибок»: битый ключ защиты и слишком короткий паддинг
// роняют УЗЕЛ (ядро отвергает такой конфиг целиком), а мусор в тайминге или
// булевом снимает ПОЛЕ и оставляет узел жить.
func TestParseWireGuardURI_AWG3Negatives(t *testing.T) {
	// s1–s4 >= 12 обязательны везде, где задан ключ защиты.
	padding := "&s1=55&s2=42&s3=40&s4=12"
	cases := []struct {
		name       string
		query      string
		wantDrop   bool
		wantErrHas string
		wantCode   string
		absentKeys []string
	}{
		{
			name:       "header key decodes to 16 bytes",
			query:      padding + "&headerprotectionkey=AQIDBAUGBwgJCgsMDQ4PEA%3D%3D",
			wantDrop:   true,
			wantErrHas: "32",
		},
		{
			name:       "header key is all zeros",
			query:      padding + "&headerprotectionkey=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA%3D",
			wantDrop:   true,
			wantErrHas: "all zeros",
		},
		{
			name:       "header key is not base64",
			query:      padding + "&headerprotectionkey=%21%21not-base64%21%21",
			wantDrop:   true,
			wantErrHas: "not base64",
		},
		{
			name:       "s4 below the header nonce minimum",
			query:      "&s1=55&s2=42&s3=40&s4=8&headerprotectionkey=Bw4VHCMqMTg%2FRk1UW2JpcHd%2BhYyTmqGor7a9xMvS2eA%3D",
			wantDrop:   true,
			wantErrHas: "s4=8 is below the minimum 12",
		},
		{
			name:       "reversed timing range keeps the node, drops the field",
			query:      "&rekeyaftertime=180-150",
			wantCode:   WarnAWG3FieldInvalid,
			absentKeys: []string{"rekey_after_time"},
		},
		{
			name:       "non-boolean random trailers keeps the node, drops the field",
			query:      "&randomtrailers=maybe",
			wantCode:   WarnAWG3FieldInvalid,
			absentKeys: []string{"random_trailers"},
		},
		{
			// Свойство протокола, а не ошибка: ничего не снимаем, только info.
			// h2–h4 отодвинуты, иначе узел упал бы раньше на overlap заголовков.
			name:     "random trailers with a wide header range is info only",
			query:    "&h1=1000-200000&h2=300000&h3=300001&h4=300002&randomtrailers=on",
			wantCode: WarnAWG3RandomTrailersWideHeaders,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ParseNode(awg3URI(tc.query), nil)
			if tc.wantDrop {
				if err == nil || node != nil {
					t.Fatalf("node must be dropped, got node=%v err=%v", node, err)
				}
				if tc.wantErrHas != "" && !contains(err.Error(), tc.wantErrHas) {
					t.Errorf("error %q must mention %q", err.Error(), tc.wantErrHas)
				}
				return
			}
			if err != nil || node == nil {
				t.Fatalf("node must survive, got err=%v", err)
			}
			if tc.wantCode != "" && !hasWarning(node.Warnings, tc.wantCode) {
				t.Errorf("warnings = %v, want %s", node.Warnings, tc.wantCode)
			}
			for _, k := range tc.absentKeys {
				if _, ok := node.Outbound[k]; ok {
					t.Errorf("%s = %v, want the field dropped", k, node.Outbound[k])
				}
			}
			if tc.wantCode == WarnAWG3RandomTrailersWideHeaders {
				if v, _ := node.Outbound["random_trailers"].(bool); !v {
					t.Error("random_trailers must stay set: the info code removes nothing")
				}
			}
		})
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
	// AWG3 выведен из-под клампа AWG2: MTU задаёт сервер.
	if got, _ := first.Outbound["mtu"].(int); got != 1280 {
		t.Errorf("mtu = %v, want 1376 clamped to 1280 (AWG3 clamps like AWG2)", first.Outbound["mtu"])
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
