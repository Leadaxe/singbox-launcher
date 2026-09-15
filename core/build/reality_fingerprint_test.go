package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
)

// TestRealityFingerprintAllEntryPaths — один интеграционный прогон правила
// D-119 через ВСЕ входы, которые эмитят reality: URI vless, URI anytls,
// Xray-JSON и sing-box-JSON импорт. Каждый вход обязан (а) пометить узел
// кодом reality_fp_not_chrome при явном отпечатке вне chrome-семейства и
// (б) отдать его на сборке КАК ЕСТЬ; chrome ставится только вместо пустого или
// нашего неявного `random`. Россыпь юнитов на каждую точку не пишем: важна
// именно связка «парсер пометил → сборка не подменила».
func TestRealityFingerprintAllEntryPaths(t *testing.T) {
	const pbk = "AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw"

	cases := []struct {
		name     string
		input    string
		wantWarn bool // ждём ли reality_fp_not_chrome на узле
		wantFP   string
	}{
		{
			name:     "uri vless firefox",
			input:    "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&fp=firefox&pbk=" + pbk + "&sid=ab#n",
			wantWarn: true,
			wantFP:   "firefox",
		},
		{
			// Пустой fp у vless — наш дефолт `random` (D-009). Warning'а нет
			// (от явного fp=random неотличим), но на сборке он всё равно
			// становится chrome: против Xray >= v26.9.8 random мёртв в 4 из 5.
			name:     "uri vless empty fp defaults to random then healed",
			input:    "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&pbk=" + pbk + "&sid=ab#n",
			wantWarn: false,
			wantFP:   "chrome",
		},
		{
			name:     "uri vless chrome untouched",
			input:    "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=reality&fp=chrome_pq&pbk=" + pbk + "&sid=ab#n",
			wantWarn: false,
			wantFP:   "chrome_pq",
		},
		{
			name:     "uri anytls safari",
			input:    "anytls://pass@example-1.com:443?security=reality&fp=safari&pbk=" + pbk + "&sid=ab#n",
			wantWarn: true,
			wantFP:   "safari",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := subscription.ParseNode(tc.input, nil)
			if err != nil || node == nil {
				t.Fatalf("ParseNode(%q): node=%v err=%v", tc.input, node, err)
			}
			assertRealityHealed(t, node.Outbound, node.Warnings, tc.wantWarn, tc.wantFP)
		})
	}

	t.Run("xray json import firefox", func(t *testing.T) {
		// Xray-тело приезжает МАССИВОМ конфигов (BodyKindXrayArray).
		raw := `[{"outbounds":[{"tag":"x","protocol":"vless","settings":{"vnext":[{"address":"example-1.com","port":443,` +
			`"users":[{"id":"11111111-1111-1111-1111-111111111111"}]}]},` +
			`"streamSettings":{"network":"tcp","security":"reality",` +
			`"realitySettings":{"serverName":"www.example-3.com","fingerprint":"firefox","publicKey":"` + pbk + `","shortId":"ab"}}}]}]`
		node := parseBodySingleNode(t, raw)
		assertRealityHealed(t, node.Outbound, node.Warnings, true, "firefox")
	})

	t.Run("singbox json import firefox", func(t *testing.T) {
		raw := `{"outbounds":[{"tag":"s","type":"vless","server":"example-1.com","server_port":443,` +
			`"uuid":"11111111-1111-1111-1111-111111111111",` +
			`"tls":{"enabled":true,"server_name":"www.example-3.com",` +
			`"utls":{"enabled":true,"fingerprint":"firefox"},` +
			`"reality":{"enabled":true,"public_key":"` + pbk + `","short_id":"ab"}}}]}`
		node := parseBodySingleNode(t, raw)
		assertRealityHealed(t, node.Outbound, node.Warnings, true, "firefox")
	})
}

// parseBodySingleNode разбирает тело подписки (Xray- или sing-box-JSON) и
// требует ровно один узел — тесту нужен именно он.
func parseBodySingleNode(t *testing.T, body string) *configtypes.ParsedNode {
	t.Helper()
	res, err := subscription.ParseSubscriptionBody([]byte(body), nil, 0)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("ожидалась 1 запись, получено %d (rejected=%v)", len(res.Entries), res.Rejected)
	}
	return res.Entries[0].Node
}

// assertRealityHealed прогоняет outbound через сборочный шаг и сверяет
// отпечаток с ожиданием; warnings проверяются отдельно — они с узла, а не с
// конфига.
func assertRealityHealed(t *testing.T, outbound map[string]interface{}, warnings []string, wantWarn bool, wantFP string) {
	t.Helper()

	gotWarn := false
	for _, w := range warnings {
		if w == subscription.WarnRealityFPNotChrome {
			gotWarn = true
		}
	}
	if gotWarn != wantWarn {
		t.Errorf("warning reality_fp_not_chrome: got %v, want %v (warnings=%v)", gotWarn, wantWarn, warnings)
	}

	raw, err := json.Marshal(outbound)
	if err != nil {
		t.Fatalf("marshal outbound: %v", err)
	}
	healed := HealRealityFingerprints([]json.RawMessage{raw})
	var ob map[string]interface{}
	if err := json.Unmarshal(healed[0], &ob); err != nil {
		t.Fatalf("unmarshal healed: %v", err)
	}
	tls, _ := ob["tls"].(map[string]interface{})
	utls, _ := tls["utls"].(map[string]interface{})
	if fp, _ := utls["fingerprint"].(string); fp != wantFP {
		t.Errorf("fingerprint после сборки: got %q, want %q", fp, wantFP)
	}
}
