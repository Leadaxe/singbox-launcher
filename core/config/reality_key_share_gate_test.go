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
//
// SPEC 131 W2c: гейт перестал быть частной пробой на одно поле и стал
// табличным проходом по реестру (`min_core` в registry/tls.json). Поэтому
// тест подменяет не «умеет ли ядро key_share», а ВЕРСИЮ ядра — то есть ровно
// тот единственный вход, который остался у полевых гейтов. Если бы граница
// поля уехала обратно в код, этот тест продолжил бы проходить только на
// реестре, где она записана.
func withCoreVersion(t *testing.T, version string) {
	t.Helper()
	prev := CoreVersionProbe
	CoreVersionProbe = func() string { return version }
	gateLoggedOnce.Delete(realityKeyShareLogKey(version))
	t.Cleanup(func() { CoreVersionProbe = prev })
}

// realityKeyShareLogKey — ключ дедупа WARN-строки гейта; тест снимает его,
// чтобы прогон не зависел от порядка подтестов.
func realityKeyShareLogKey(version string) string {
	return "vless.tls.reality.key_share@" + version
}

// realityKeyShareBody — тело узла в том виде, в каком его хранит state:
// без tag и detour, с type первым ключом.
const realityKeyShareBody = `{"type":"vless","server":"example-1.com","server_port":443,` +
	`"uuid":"11111111-1111-1111-1111-111111111111",` +
	`"tls":{"enabled":true,"server_name":"w.example.com",` +
	`"utls":{"enabled":true,"fingerprint":"chrome"},` +
	`"reality":{"enabled":true,"public_key":"AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw",` +
	`"short_id":"abcd","key_share":"hybrid"}}}`

func realityKeyShareNode() *ParsedNode {
	return &ParsedNode{
		Tag:         "ks-node",
		Scheme:      "vless",
		Server:      "example-1.com",
		Port:        443,
		UUID:        "11111111-1111-1111-1111-111111111111",
		EmitBody:    []byte(realityKeyShareBody),
		SourceIndex: configtypes.UnsetSourceIndex,
	}
}

func TestRealityKeyShareCoreGate(t *testing.T) {
	t.Run("core lx.4 and newer emits the field", func(t *testing.T) {
		withCoreVersion(t, "1.14.1-lx.4")
		out, err := GenerateNodeJSONBare(realityKeyShareNode())
		if err != nil {
			t.Fatalf("GenerateNodeJSONBare: %v", err)
		}
		if !strings.Contains(out, `"key_share":"hybrid"`) {
			t.Fatalf("key_share must be emitted on a supporting core, got %s", out)
		}
	})

	t.Run("older core omits the field but keeps REALITY", func(t *testing.T) {
		withCoreVersion(t, "1.14.1-lx.3")
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

	t.Run("unknown core version does not degrade", func(t *testing.T) {
		// Пустая версия = «не знаем»: деградировать по догадке нельзя, иначе
		// сломанная проба тихо резала бы поля у всех узлов сразу.
		withCoreVersion(t, "")
		out, err := GenerateNodeJSONBare(realityKeyShareNode())
		if err != nil {
			t.Fatalf("GenerateNodeJSONBare: %v", err)
		}
		if !strings.Contains(out, `"key_share":"hybrid"`) {
			t.Fatalf("unknown version must not drop the field, got %s", out)
		}
	})
}
