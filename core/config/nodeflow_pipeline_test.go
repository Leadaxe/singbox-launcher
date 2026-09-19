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
	"singbox-launcher/core/state"
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

// pipelineFromOutbound — тело картой sing-box. `source` — вход, которым тело
// приехало: его читают правила значений, различающие, кто сочинил значение
// (см. materializeBody).
func pipelineFromOutbound(t *testing.T, scheme, source string, outbound map[string]interface{}) pipelineNode {
	t.Helper()
	body, warns, drop := materializeBody(scheme, source, outbound)
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
	fromBody := pipelineFromOutbound(t, "vless", configtypes.NodeSourceSingbox, singboxBody)

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
	fromXray := pipelineFromOutbound(t, "vless", configtypes.NodeSourceXray, xrayNodes[0].Outbound)
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
		"тело sing-box": pipelineFromOutbound(t, "hysteria", configtypes.NodeSourceSingbox, map[string]interface{}{
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

// TestPipelineAWGMTUCeiling — потолок MTU у AmneziaWG как правило РЕЕСТРА
// (SPEC 131, находка №5 LEGACY_AUDIT; контракт 1.1.5).
//
// Пока правило жило кодом в парсере ссылки (awgMaxMTU), оно не применялось к
// телу из sing-box-импорта: один и тот же узел ссылкой и объектом получал
// разный MTU, и объектная половина тихо уносила в ядро значение, на котором
// туннель поднимается и не несёт данных. Теперь правило одно, в контракте, —
// и проверяется здесь его результат на всех входах сразу.
//
// Парность входов нарушена НАМЕРЕННО (решение владельца 18.09.2026,
// DRIFT §7.23): тело в форме ядра человек или подписка написали сами, и
// молча переписывать его лаунчер не вправе — он предупреждает.
func TestPipelineAWGMTUCeiling(t *testing.T) {
	const (
		priv = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
		pub  = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
		host = "awg.example-1.com"
		base = "publickey=" + pub + "&address=10.0.0.2/32"
	)
	uri := func(extra string) string {
		return "wireguard://" + priv + "@" + host + ":51820?" + base + extra + "#awg"
	}
	// body — то же тело картой sing-box; `awg` решает, быть ли узлу AWG.
	body := func(awg bool, mtu interface{}) map[string]interface{} {
		ob := map[string]interface{}{
			"type":        "wireguard",
			"address":     []interface{}{"10.0.0.2/32"},
			"private_key": priv,
			"peers": []interface{}{map[string]interface{}{
				"address": host, "port": 51820, "public_key": pub,
				"allowed_ips": []interface{}{"0.0.0.0/0"},
			}},
		}
		if awg {
			ob["jc"] = 10
		}
		if mtu != nil {
			ob["mtu"] = mtu
		}
		return ob
	}

	cases := []struct {
		name    string
		got     pipelineNode
		wantMTU interface{} // nil = ключа mtu быть не должно
		wantCod string      // "" = кодов по mtu быть не должно
	}{
		// Ссылка: значение сочинил генератор провайдера — правило заменяет.
		{"ссылка awg без mtu", pipelineFromURI(t, uri("&jc=10")), float64(1280), ""},
		{"ссылка awg mtu 1420", pipelineFromURI(t, uri("&jc=10&mtu=1420")), float64(1280), "awg_mtu_clamped"},
		{"ссылка awg mtu 1200", pipelineFromURI(t, uri("&jc=10&mtu=1200")), float64(1200), ""},
		// AWG3-маркер без единого поля AWG2 — тот же узел AmneziaWG.
		{"ссылка awg3 mtu 1376", pipelineFromURI(t, uri("&randomtrailers=on&mtu=1376")), float64(1280), "awg_mtu_clamped"},
		// jc=0 — законное «мусор выключен», а не «поля нет»: условие правила
		// смотрит на НАЛИЧИЕ ключа. Прочитай оно значение — с такого узла
		// потолок снялся бы, и вернулась бы ровно та тихая поломка.
		{"ссылка awg jc=0 mtu 1420", pipelineFromURI(t, uri("&jc=0&mtu=1420")), float64(1280), "awg_mtu_clamped"},
		// Обычный WireGuard правило не трогает вовсе.
		{"ссылка plain wg mtu 1420", pipelineFromURI(t, uri("&mtu=1420")), float64(1420), ""},
		{"ссылка plain wg без mtu", pipelineFromURI(t, uri("")), nil, ""},

		// Тело sing-box: написано в форме ядра — значение СОХРАНЯЕТСЯ с info.
		{"тело awg mtu 1420", pipelineFromOutbound(t, "wireguard", configtypes.NodeSourceSingbox, body(true, 1420)), float64(1420), "awg_mtu_high"},
		// Дефолт при отсутствии mtu действует и здесь: это не замена
		// написанного человеком, а дефолт.
		{"тело awg без mtu", pipelineFromOutbound(t, "wireguard", configtypes.NodeSourceSingbox, body(true, nil)), float64(1280), ""},
		{"тело plain wg mtu 1420", pipelineFromOutbound(t, "wireguard", configtypes.NodeSourceSingbox, body(false, 1420)), float64(1420), ""},
		// Вход не назван — исключение не действует: молчать про завышенный
		// MTU только потому, что вход забыли протянуть, нельзя.
		{"тело awg mtu 1420, вход не назван", pipelineFromOutbound(t, "wireguard", "", body(true, 1420)), float64(1280), "awg_mtu_clamped"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(c.got.Body), &entry); err != nil {
				t.Fatalf("тело не разбирается: %v", err)
			}
			raw, present := entry["mtu"]
			if c.wantMTU == nil {
				if present {
					t.Errorf("mtu = %v, want ключа нет", raw)
				}
			} else if raw != c.wantMTU {
				t.Errorf("mtu = %#v, want %#v", raw, c.wantMTU)
			}

			var mtuCodes []string
			for _, w := range c.got.Warnings {
				if w.Path == "mtu" {
					mtuCodes = append(mtuCodes, w.Code)
				}
			}
			if c.wantCod == "" {
				if len(mtuCodes) > 0 {
					t.Errorf("коды по mtu = %v, want ни одного", mtuCodes)
				}
				return
			}
			if len(mtuCodes) != 1 || mtuCodes[0] != c.wantCod {
				t.Errorf("коды по mtu = %v, want [%s]", mtuCodes, c.wantCod)
			}
		})
	}
}

// TestPipelineAWGMTUExceptionSurvivesStateReload — исключение по входу обязано
// пережить перезапуск.
//
// Это не «ещё один случай», а самостоятельный риск конструкции. Вход узла
// живёт в двух местах: на разборе — в ParsedNode.Source, в сохранённом
// состоянии — в Origin.Kind. Пересчёт кодов при загрузке state гоняет тело
// через тот же конвейер и ВПРАВЕ переписать тело. Если бы он не знал входа,
// JSON-узел с mtu 1420 был бы заклампен задним числом — то есть настройка
// пользователя исчезала бы от перезапуска, молча и необратимо.
func TestPipelineAWGMTUExceptionSurvivesStateReload(t *testing.T) {
	stored := []byte(`{"type":"wireguard","mtu":1420,"address":["10.0.0.2/32"],` +
		`"private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","jc":10,` +
		`"peers":[{"address":"awg.example-1.com","port":51820,` +
		`"public_key":"AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=",` +
		`"allowed_ips":["0.0.0.0/0"]}]}`)

	cases := []struct {
		name       string
		originKind string
		wantRewrit bool
		wantCode   string
	}{
		// Узел из sing-box JSON: тело остаётся байт в байт, код — info.
		{"origin=json", state.OriginKindJSON, false, "awg_mtu_high"},
		// Узел из ссылки: значение заменяется, тело переписывается.
		{"origin=uri", state.OriginKindURI, true, "awg_mtu_clamped"},
		// .conf — ссылка того же рода.
		{"origin=wg_ini", state.OriginKindWGIni, true, "awg_mtu_clamped"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := sanitizeStoredNodeBody(state.SanitizeBodyRequest{
				Body: stored, OriginKind: c.originKind,
			})
			if err != nil {
				t.Fatalf("пересчёт кодов: %v", err)
			}
			if res.Drop {
				t.Fatal("узел объявлен непригодным — тело валидно")
			}
			if got := len(res.Body) > 0; got != c.wantRewrit {
				t.Errorf("тело переписано = %v, want %v (body=%s)", got, c.wantRewrit, res.Body)
			}
			var codes []string
			for _, w := range res.Warnings {
				if w.Path == "mtu" {
					codes = append(codes, w.Code)
				}
			}
			if len(codes) != 1 || codes[0] != c.wantCode {
				t.Errorf("коды по mtu = %v, want [%s]", codes, c.wantCode)
			}
		})
	}
}
