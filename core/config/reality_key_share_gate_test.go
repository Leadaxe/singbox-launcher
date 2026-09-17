package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// D-121: поле tls.reality.key_share знает только ядро >= 1.14.1-lx.4. На
// более старом ядре ключ НЕИЗВЕСТЕН, а неизвестный ключ ядро отвергает
// отказом ВСЕГО конфига — то есть без гейта один такой узел оставил бы
// пользователя вообще без VPN.
//
// Гейт ПОЛЕВОЙ, в отличие от tailscale/AWG3: узел не выбрасывается, снимается
// одно поле — REALITY работает и без него (обмен ключами берётся из
// uTLS-отпечатка). Тест проверяет ровно это: поле уходит, REALITY остаётся.
func withRealityKeyShareProbe(t *testing.T, probe func() (bool, string)) {
	t.Helper()
	prev := RealityKeyShareSupportProbe
	RealityKeyShareSupportProbe = probe
	t.Cleanup(func() { RealityKeyShareSupportProbe = prev })
}

func realityKeyShareNode() *ParsedNode {
	return &ParsedNode{
		Tag:    "ks-node",
		Scheme: "vless",
		Server: "example-1.com",
		Port:   443,
		UUID:   "11111111-1111-1111-1111-111111111111",
		Outbound: map[string]interface{}{
			"type":        "vless",
			"tag":         "ks-node",
			"server":      "example-1.com",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "w.example.com",
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": "AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw",
					"short_id":   "abcd",
					"key_share":  "hybrid",
				},
			},
		},
		SourceIndex: configtypes.UnsetSourceIndex,
	}
}

func TestRealityKeyShareCoreGate(t *testing.T) {
	t.Run("core lx.4 and newer emits the field", func(t *testing.T) {
		withRealityKeyShareProbe(t, func() (bool, string) { return true, "" })
		out, err := GenerateNodeJSONBare(realityKeyShareNode())
		if err != nil {
			t.Fatalf("GenerateNodeJSONBare: %v", err)
		}
		if !strings.Contains(out, `"key_share":"hybrid"`) {
			t.Fatalf("key_share must be emitted on a supporting core, got %s", out)
		}
	})

	t.Run("older core omits the field but keeps REALITY", func(t *testing.T) {
		withRealityKeyShareProbe(t, func() (bool, string) {
			return false, "sing-box core 1.14.1-lx.3 does not support tls.reality.key_share"
		})
		out, err := GenerateNodeJSONBare(realityKeyShareNode())
		if err != nil {
			t.Fatalf("GenerateNodeJSONBare: %v", err)
		}
		if strings.Contains(out, "key_share") {
			t.Fatalf("key_share must NOT be emitted on an older core, got %s", out)
		}
		// Узел обязан остаться REALITY: гейт снимает поле, а не блок и не узел.
		if !strings.Contains(out, `"public_key":"AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw"`) {
			t.Fatalf("REALITY block must survive the gate, got %s", out)
		}
	})
}
