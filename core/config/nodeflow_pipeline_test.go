package config

// Конвейер узла целиком: ссылка / JSON-тело / Xray-объект → одно и то же
// тело и один и тот же набор кодов (SPEC 131 §10, критерий §9.2).
//
// Зачем отдельно от корпуса: корпус сверяет КАЖДЫЙ вход со своим эталоном и
// поймает изменение любого из них, но не поймает, если два входа разъедутся
// ОДИНАКОВО неправильно — эталоны у них разные файлы. Здесь сверяются входы
// между собой, эталона нет вовсе, и правило ровно одно: откуда бы узел ни
// пришёл, сохранится он одинаково.

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
)

// pipelineNode — то, что конвейер сохранит: тело и коды.
type pipelineNode struct {
	Body     string
	Warnings []configtypes.Warning
}

func pipelineFromURI(t *testing.T, uri string) pipelineNode {
	t.Helper()
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode(%q): err=%v node=%v", uri, err, node)
	}
	body, warns, drop := materializeParsedNodeBody(node)
	if drop != nil {
		t.Fatalf("узел отбракован конвейером: %s", dropReason(drop))
	}
	return pipelineNode{Body: string(body), Warnings: warns}
}

func pipelineFromOutbound(t *testing.T, scheme string, outbound map[string]interface{}) pipelineNode {
	t.Helper()
	body, warns, drop := materializeBody(scheme, outbound)
	if drop != nil {
		t.Fatalf("узел отбракован конвейером: %s", dropReason(drop))
	}
	return pipelineNode{Body: string(body), Warnings: warns}
}

// codes — коды с путями, в порядке постановки (порядок нормативен, CANON §6).
func (p pipelineNode) codes() []string {
	out := make([]string, 0, len(p.Warnings))
	for _, w := range p.Warnings {
		out = append(out, w.Code+"@"+w.Path+"="+w.Value)
	}
	return out
}

// TestPipelineAllInputsAgree — один и тот же мусорный узел тремя дорогами.
//
// Мусор подобран так, чтобы задеть все виды правил реестра сразу: enum
// (flow, packet_encoding), формат с длиной после декода (reality public_key
// — тут валидный, чтобы блок дожил до проверки sid), hex-чистку с потерей
// (short_id), coerce в дефолт (отпечаток) и ключ вне схемы (unknown_key).
func TestPipelineAllInputsAgree(t *testing.T) {
	const (
		uuid = "11111111-1111-1111-1111-111111111111"
		pbk  = "AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw"
		host = "pipeline.example-1.com"
		sni  = "cover.example-1.com"
	)

	uri := "vless://" + uuid + "@" + host + ":443" +
		"?security=reality&encryption=none&sni=" + sni +
		"&fp=totally-bogus&pbk=" + pbk + "&sid=0x1a2" +
		"&packetEncoding=teleport&flow=xtls-rprx-direct#pipeline"

	// Тот же узел телом sing-box: ключи канонические, значения те же.
	singboxBody := map[string]interface{}{
		"type":            "vless",
		"tag":             "pipeline",
		"server":          host,
		"server_port":     443,
		"uuid":            uuid,
		"flow":            "xtls-rprx-direct",
		"packet_encoding": "teleport",
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": "totally-bogus",
			},
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": pbk,
				"short_id":   "0x1a2",
			},
		},
	}

	// Тот же узел из Xray-конфига: маппер обязан перевести streamSettings в
	// те же пути тела. Отдельная дорога, потому что диалект чужой целиком.
	xrayJSON := `[{"outbounds":[{"protocol":"vless","tag":"pipeline",` +
		`"settings":{"vnext":[{"address":"` + host + `","port":443,` +
		`"users":[{"id":"` + uuid + `","encryption":"none","flow":"xtls-rprx-direct"}]}]},` +
		`"streamSettings":{"network":"tcp","security":"reality","realitySettings":` +
		`{"serverName":"` + sni + `","fingerprint":"totally-bogus","publicKey":"` + pbk + `","shortId":"0x1a2"}}}]}]`

	fromURI := pipelineFromURI(t, uri)
	fromBody := pipelineFromOutbound(t, "vless", singboxBody)

	if fromURI.Body != fromBody.Body {
		t.Errorf("тела разошлись\n  ссылка: %s\n  тело:   %s", fromURI.Body, fromBody.Body)
	}
	if got, want := fromURI.codes(), fromBody.codes(); !equalStrings(got, want) {
		t.Errorf("коды разошлись\n  ссылка: %v\n  тело:   %v", got, want)
	}

	// Xray-форма: у неё нет packet_encoding (Xray его не знает), поэтому
	// сверяется всё остальное — иначе пришлось бы выдумывать поле, которого
	// в чужом диалекте нет, и тест проверял бы выдумку.
	xrayNodes, err := subscription.ParseNodesFromXrayJSONArray(xrayJSON, nil)
	if err != nil || len(xrayNodes) != 1 {
		t.Fatalf("ParseNodesFromXrayJSONArray: err=%v nodes=%d", err, len(xrayNodes))
	}
	fromXray := pipelineFromOutbound(t, "vless", xrayNodes[0].Outbound)
	var uriEntry, xrayEntry map[string]interface{}
	if err := json.Unmarshal([]byte(fromURI.Body), &uriEntry); err != nil {
		t.Fatalf("тело ссылки не разбирается: %v", err)
	}
	if err := json.Unmarshal([]byte(fromXray.Body), &xrayEntry); err != nil {
		t.Fatalf("тело Xray не разбирается: %v", err)
	}
	delete(uriEntry, "packet_encoding")
	delete(xrayEntry, "packet_encoding")
	uriBytes, _ := json.Marshal(uriEntry)
	xrayBytes, _ := json.Marshal(xrayEntry)
	if string(uriBytes) != string(xrayBytes) {
		t.Errorf("тело из Xray разошлось со ссылкой\n  ссылка: %s\n  xray:   %s", uriBytes, xrayBytes)
	}
}

// TestPipelineSetsDegradationCodes — каждый вид правила реестра реально
// ставит свой код на выходе конвейера.
//
// До W2d это проверялось на выходе ПАРСЕРА (subscription/parse_warnings_test.go),
// где коды и стояли. Теперь их ставит санитайзер, и смотреть надо туда же,
// куда смотрит state: на тело узла и его warnings.
func TestPipelineSetsDegradationCodes(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "мусорный uTLS-отпечаток заменён каноническим",
			uri:  "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=tls&fp=garbage&sni=example-1.com",
			want: "utls_fp_unknown",
		},
		{
			name: "reality sid нечётной длины снят целиком",
			uri:  "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&pbk=AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw&sid=abc&sni=example-1.com",
			want: "reality_short_id_invalid",
		},
		{
			name: "мусорный pbk снимает весь блок reality",
			uri:  "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&pbk=enabled&sni=example-1.com",
			want: "reality_pbk_invalid",
		},
		{
			name: "hysteria2 obfs без пароля снята",
			uri:  "hysteria2://pass123@example-2.com:443?obfs=salamander&sni=example-2.com",
			want: "obfs_password_missing",
		},
		{
			name: "hysteria2 obfs вне словаря ядра",
			uri:  "hysteria2://pass123@example-2.com:443?obfs=nonsense&obfs-password=p&sni=example-2.com",
			want: "obfs_unknown",
		},
		{
			name: "TUIC congestion_control вне словаря",
			uri:  "tuic://11111111-1111-1111-1111-111111111111:pass@example-3.com:443?congestion_control=nonsense&sni=example-3.com",
			want: "tuic_congestion_invalid",
		},
		{
			name: "TUIC udp_relay_mode вне словаря",
			uri:  "tuic://11111111-1111-1111-1111-111111111111:pass@example-3.com:443?udp_relay_mode=nonsense&sni=example-3.com",
			want: "tuic_udp_relay_mode_invalid",
		},
		{
			name: "vless flow вне набора ядра",
			uri:  "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=tls&flow=xtls-rprx-direct&sni=example-1.com",
			want: "flow_deprecated",
		},
		{
			name: "anytls min_idle_session не число",
			uri:  "anytls://pass@example-4.com:443?min_idle_session=many&sni=example-4.com",
			want: "anytls_min_idle_invalid",
		},
		{
			name: "REALITY с отпечатком без гибридного шара",
			uri:  "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&pbk=AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw&fp=edge&sni=example-1.com",
			want: "reality_fp_not_chrome",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pipelineFromURI(t, tc.uri)
			for _, w := range got.Warnings {
				if w.Code == tc.want {
					if w.Path == "" {
						t.Errorf("код %q проставлен без пути: путь показывает, КАКОЕ поле снято", tc.want)
					}
					return
				}
			}
			t.Fatalf("код %q не проставлен; получено: %v", tc.want, got.codes())
		})
	}
}

// TestPipelineCleanNodeStaysClean — здоровый узел кодов не получает.
//
// Ложное срабатывание обесценивает весь механизм: пользователь перестаёт
// читать предупреждения. Отдельно проверяется, что приведение РЕГИСТРА hex
// деградацией не считается — ядро декодирует ABCD и abcd одинаково.
func TestPipelineCleanNodeStaysClean(t *testing.T) {
	clean := []string{
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=tls&fp=chrome&sni=example-1.com",
		"trojan://testpass123@example-2.com:443?sni=example-2.com",
		"hysteria2://pass123@example-2.com:443?obfs=salamander&obfs-password=secret&sni=example-2.com",
		// sid=ABCD — только регистр, кода быть не должно.
		"vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&pbk=AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw&sid=ABCD&fp=chrome&sni=example-1.com",
	}
	for _, uri := range clean {
		got := pipelineFromURI(t, uri)
		if len(got.Warnings) != 0 {
			t.Errorf("здоровый узел помечен: %v\n  %s", got.codes(), uri)
		}
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

// TestPipelineHysteriaV1BandwidthDefault — полоса hysteria v1 появляется в
// теле на КАЖДОМ входе.
//
// Ядро отказывается инициализировать outbound v1 без up_mbps/down_mbps
// («missing upload speed»), и это фатал для ВСЕГО config.json, а не для узла.
// Ссылки v1 скорость сплошь и рядом не несут, поэтому реестр велит
// материализовать дефолт явно (default_when у hysteria.body.up_mbps).
//
// Прежде тот же дефолт лежал тремя копиями в коде — в URI-парсере, в
// санитайзере sing-box-импорта и в Xray-конвертере, — и проверять приходилось
// каждую. Теперь копия одна, в контракте, и проверяется её результат.
func TestPipelineHysteriaV1BandwidthDefault(t *testing.T) {
	inputs := map[string]pipelineNode{
		"ссылка": pipelineFromURI(t, "hysteria://host.example.com:36712?auth=a&sni=host.example.com"),
		"тело sing-box": pipelineFromOutbound(t, "hysteria", map[string]interface{}{
			"type": "hysteria", "server": "1.2.3.4", "server_port": 443,
			"auth_str": "pw",
			"tls":      map[string]interface{}{"enabled": true, "server_name": "a.b"},
		}),
	}
	for name, got := range inputs {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(got.Body), &entry); err != nil {
			t.Fatalf("%s: тело не разбирается: %v", name, err)
		}
		for _, key := range []string{"up_mbps", "down_mbps"} {
			v, ok := entry[key].(float64)
			if !ok || v <= 0 {
				t.Errorf("%s: %s = %#v — ядро отвергнет ВЕСЬ конфиг", name, key, entry[key])
			}
		}
	}
}
