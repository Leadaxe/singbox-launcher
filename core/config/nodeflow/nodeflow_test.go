package nodeflow

import (
	"encoding/json"
	"testing"
)

// Тесты конвейера — табличные и data-критичные: сверяется тело, которое
// уедет в config.json, и список кодов, который увидит пользователь. Формат
// строк и вёрстка здесь не проверяются (память tests-minimal-policy).

// pipeCase — один прогон Sanitize+Emit.
type pipeCase struct {
	name string
	// scheme и вход ровно в том виде, в каком тело приезжает из JSON:
	// числа float64, массивы []interface{} (ловушка Л8).
	scheme string
	in     string
	// want — ожидаемое тело после эмиттера, байт в байт (порядок ключей =
	// body.order реестра).
	want string
	// codes — коды warnings в порядке обхода; nil = ожидается пусто.
	codes []string
	// drop — код отказа по узлу, пусто = узел выживает.
	drop string
}

const (
	testUUID = "b831381d-6324-4d53-ad4f-8cda48b30811"
	testPBK  = "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0"
)

func TestSanitizeEmit(t *testing.T) {
	cases := []pipeCase{
		{
			// (а) Всё, что приезжает из JSON строками и числами: порт строкой,
			// булево строкой, listable строкой (форма СОХРАНЯЕТСЯ), плюс ключ
			// вне схемы.
			name:   "vless/json-типы-и-неизвестный-ключ",
			scheme: "vless",
			in: `{"server":"a.example.com","server_port":"443","uuid":"` + testUUID + `",
			      "foo":1,"tls":{"enabled":true,"server_name":"a.example.com",
			      "insecure":"true","alpn":"h2,http/1.1"}}`,
			want: `{"server":"a.example.com","server_port":443,"uuid":"` + testUUID + `",` +
				`"tls":{"enabled":true,"server_name":"a.example.com","insecure":true,"alpn":"h2,http/1.1"}}`,
			codes: []string{"unknown_key"},
		},
		{
			// (б) naive читает лишь несколько TLS-полей; остальные ядро либо
			// отвергает фатально, либо молча игнорирует — реестр снимает и те
			// и другие одним кодом. certificate остаётся (LxBox #140).
			name:   "naive/tls-поля-сняты-по-реестру",
			scheme: "naive",
			in: `{"server":"a.example.com","server_port":443,"username":"u","password":"p",
			      "tls":{"enabled":true,"insecure":true,"alpn":["h2"],
			      "utls":{"enabled":true,"fingerprint":"chrome"},"certificate":"PEMDATA"}}`,
			want: `{"server":"a.example.com","server_port":443,"username":"u","password":"p",` +
				`"tls":{"enabled":true,"certificate":"PEMDATA"}}`,
			codes: []string{
				"tls_field_unsupported_naive", // tls.insecure
				"tls_field_unsupported_naive", // tls.alpn
				"tls_field_unsupported_naive", // tls.utls — блок целиком (реестр: forbidden_for на объекте)
			},
		},
		{
			// (в) reality: нечётный short_id ядро валит на старте (hex odd
			// length) — снимается целиком, не обрезается; key_share
			// нормализуется до enum по флагу normalize.
			name:   "vless/reality-short_id-нечётный-и-key_share-с-регистром",
			scheme: "vless",
			in: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",
			      "tls":{"enabled":true,"server_name":"x.com","utls":{"enabled":true,"fingerprint":"chrome"},
			      "reality":{"enabled":true,"public_key":"` + testPBK + `","short_id":"abc","key_share":" Hybrid "}}}`,
			want: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",` +
				`"tls":{"enabled":true,"server_name":"x.com",` +
				`"utls":{"enabled":true,"fingerprint":"chrome"},` +
				`"reality":{"enabled":true,"public_key":"` + testPBK + `","key_share":"hybrid"}}}`,
			codes: []string{"reality_short_id_invalid"},
		},
		{
			// key_share — закрытый enum: мусор = отказ всего конфига.
			name:   "vless/reality-key_share-мусор",
			scheme: "vless",
			in: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",
			      "tls":{"enabled":true,"utls":{"enabled":true},
			      "reality":{"enabled":true,"public_key":"` + testPBK + `","key_share":"garbage"}}}`,
			want: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",` +
				`"tls":{"enabled":true,"utls":{"enabled":true},` +
				`"reality":{"enabled":true,"public_key":"` + testPBK + `"}}}`,
			codes: []string{"reality_key_share_invalid"},
		},
		{
			// (г) vmess security — enum с coerce: значение вне набора ядра
			// заменяется дефолтом, узел живёт.
			name:   "vmess/security-вне-набора-ядра",
			scheme: "vmess",
			in:     `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `","security":"aes-128-ctr"}`,
			want:   `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `","security":"auto"}`,
			codes:  []string{"type_invalid"},
		},
		{
			// (д) xhttp: mode — закрытый enum, снимается; а вот
			// x_padding_placement ядро принимает ТОЛЬКО camelCase, и
			// нормализации у него в реестре нет — значение не трогаем.
			name:   "vless/xhttp-mode-мусор-и-camelCase-placement",
			scheme: "vless",
			in: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",
			      "transport":{"type":"xhttp","mode":"garbage","path":"/x","x_padding_placement":"queryInHeader"}}`,
			want: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",` +
				`"transport":{"type":"xhttp","path":"/x","x_padding_placement":"queryInHeader"}}`,
			codes: []string{"xhttp_param_reset"},
		},
		{
			// (е) tristate: явная пустая строка ОСТАЁТСЯ в теле — у ядра это
			// «без инкапсуляции», а отсутствие ключа = дефолт xudp.
			name:   "vless/packet_encoding-явно-пустой",
			scheme: "vless",
			in:     `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `","packet_encoding":""}`,
			want:   `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `","packet_encoding":""}`,
		},
		{
			name:   "vless/packet_encoding-ключа-нет",
			scheme: "vless",
			in:     `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `"}`,
			want:   `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `"}`,
		},
		{
			// (ж) advisory: ядро шифр принимает, узел живёт, код
			// информационный.
			name:   "shadowsocks/legacy-шифр-остаётся",
			scheme: "shadowsocks",
			in:     `{"server":"a.e.com","server_port":443,"method":"rc4-md5","password":"p"}`,
			want:   `{"server":"a.e.com","server_port":443,"method":"rc4-md5","password":"p"}`,
			codes:  []string{"ss_method_legacy"},
		},
		{
			// method вне набора = drop_node: ядро не стартует ни с каким
			// значением, кроме своего.
			name:   "shadowsocks/шифр-вне-набора-роняет-узел",
			scheme: "shadowsocks",
			in:     `{"server":"a.e.com","server_port":443,"method":"garbage","password":"p"}`,
			want:   `{"server":"a.e.com","server_port":443,"password":"p"}`,
			codes:  []string{"ss_method_invalid"},
			drop:   "ss_method_invalid",
		},
		{
			// (з) Л8: JSON несёт числа как float64 — в теле они обязаны стать
			// целыми, иначе hysteria v1 не стартует.
			name:   "hysteria/float64-в-int",
			scheme: "hysteria",
			in: `{"server":"a.e.com","server_port":443,"up_mbps":100.0,"down_mbps":50.0,
			      "auth_str":"x","tls":{"enabled":true}}`,
			want: `{"server":"a.e.com","server_port":443,"up_mbps":100,"down_mbps":50,` +
				`"auth_str":"x","tls":{"enabled":true}}`,
		},
		{
			// all_or_nothing: частично заданный xmux в ядре обнуляет дефолты
			// соседей — санитайзер дописывает их явно.
			name:   "vless/xmux-частичный-дополнен-дефолтами",
			scheme: "vless",
			in: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",
			      "transport":{"type":"xhttp","xmux":{"h_max_request_times":"100-200"}}}`,
			want: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",` +
				`"transport":{"type":"xhttp","xmux":{"max_concurrency":"1-1",` +
				`"h_max_request_times":"100-200","h_max_reusable_secs":"1800-3000",` +
				`"h_keep_alive_period":0}}}`,
			codes: []string{"partial_object_defaulted"},
		},
		{
			// tag, type и detour пишет сборка конфига, в теле узла их быть не
			// должно — снимаются молча: ⚠ за работу самого лаунчера
			// пользователю показывать не за что.
			name:   "vless/tag-type-detour-сняты-молча",
			scheme: "vless",
			in: `{"type":"vless","tag":"node-1","detour":"chain-1",` +
				`"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `"}`,
			want: `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `"}`,
		},
		{
			// Секрет в warnings не светится (CANON §6).
			name:   "vless/секрет-маскируется-в-warning",
			scheme: "vless",
			in:     `{"server":"a.e.com","server_port":443,"uuid":"не-uuid"}`,
			want:   `{"server":"a.e.com","server_port":443}`,
			codes:  []string{"type_invalid", "field_missing"},
			drop:   "field_missing",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(tc.in), &m); err != nil {
				t.Fatalf("вход не разбирается: %v", err)
			}
			res := Sanitize(tc.scheme, m)
			got, err := Emit(tc.scheme, res.Clean)
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("тело:\n  получено %s\n  ожидалось %s", got, tc.want)
			}
			gotCodes := make([]string, 0, len(res.Warnings))
			for _, w := range res.Warnings {
				gotCodes = append(gotCodes, w.Code)
			}
			if !equalStrings(gotCodes, tc.codes) {
				t.Errorf("коды: получено %v, ожидалось %v", gotCodes, tc.codes)
			}
			switch {
			case tc.drop == "" && res.Drop != nil:
				t.Errorf("узел отброшен без нужды: %+v", *res.Drop)
			case tc.drop != "" && res.Drop == nil:
				t.Errorf("узел обязан быть отброшен с кодом %s", tc.drop)
			case tc.drop != "" && res.Drop.Code != tc.drop:
				t.Errorf("код отказа: %s, ожидался %s", res.Drop.Code, tc.drop)
			}
		})
	}
}

// TestSecretsAreMasked — значение секретного поля в warnings не появляется.
func TestSecretsAreMasked(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{"server":"a.e.com","server_port":443,"uuid":"не-uuid"}`), &m); err != nil {
		t.Fatal(err)
	}
	res := Sanitize("vless", m)
	for _, w := range res.Warnings {
		if w.Path != "uuid" || w.Value == "" {
			continue
		}
		if w.Value != maskedValue {
			t.Errorf("секрет uuid утёк в warning: %q", w.Value)
		}
		if p, ok := w.Params["value"]; ok {
			t.Errorf("секрет uuid утёк в params: %q", p)
		}
	}
}

// TestGateForCore — полевые гейты снимаются на сборке, а не при разборе:
// тело узла от версии ядра не зависит (SPEC 131 §3.4).
func TestGateForCore(t *testing.T) {
	src := `{"server":"a.e.com","server_port":443,"uuid":"` + testUUID + `",` +
		`"tls":{"enabled":true,"kernel_tx":true,"utls":{"enabled":true},` +
		`"reality":{"enabled":true,"public_key":"` + testPBK + `","key_share":"hybrid"}}}`

	cases := []struct {
		name    string
		core    CoreInfo
		dropped []string
	}{
		{"lx.3 не знает key_share", CoreInfo{Version: "1.14.1-lx.3", GOOS: "linux"}, []string{"tls.reality.key_share"}},
		{"lx.4 знает key_share", CoreInfo{Version: "1.14.1-lx.4", GOOS: "linux"}, nil},
		{"kernel_tx только linux", CoreInfo{Version: "1.14.1-lx.4", GOOS: "darwin"}, []string{"tls.kernel_tx"}},
		{"версия неизвестна — не гадаем", CoreInfo{GOOS: "linux"}, nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, dropped, err := GateForCore("vless", []byte(src), tc.core)
			if err != nil {
				t.Fatalf("GateForCore: %v", err)
			}
			if !equalStrings(dropped, tc.dropped) {
				t.Errorf("снято %v, ожидалось %v", dropped, tc.dropped)
			}
			var check map[string]interface{}
			if err := json.Unmarshal(out, &check); err != nil {
				t.Fatalf("результат гейта не разбирается: %v", err)
			}
		})
	}
}

// TestEmitEveryScheme — эмиттер обязан пережить любую схему реестра: пустое
// тело даёт пустой объект, обязательные поля — валидный JSON (память
// emitter-parser-pairing: схема без эмиссии молча теряет поля).
func TestEmitEveryScheme(t *testing.T) {
	schemes := []string{"anytls", "chain", "http", "hysteria", "hysteria2", "masque",
		"naive", "shadowsocks", "socks", "ssh", "tailscale", "trojan", "tuic",
		"vless", "vmess", "wireguard"}
	base := map[string]interface{}{
		"server": "a.e.com", "server_port": float64(443),
		"uuid": testUUID, "password": "p", "method": "aes-256-gcm",
	}
	for _, scheme := range schemes {
		scheme := scheme
		t.Run(scheme, func(t *testing.T) {
			for _, in := range []map[string]interface{}{{}, base} {
				res := Sanitize(scheme, in)
				body, err := Emit(scheme, res.Clean)
				if err != nil {
					t.Fatalf("Emit: %v", err)
				}
				var check map[string]interface{}
				if err := json.Unmarshal(body, &check); err != nil {
					t.Fatalf("тело не разбирается как JSON: %v (%s)", err, body)
				}
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
