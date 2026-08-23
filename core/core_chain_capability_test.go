// File core_chain_capability_test.go — проба поддержки цепочек (SPEC 110).
package core

import (
	"strings"
	"testing"
)

// Вывод настоящих ядер: rc.5 собрано с with_lx_chain, rc.3-dev — без.
const (
	chainVersionWithTag = `sing-box version 1.14.0-lx.27-rc.5

Environment: go1.25.12 darwin/arm64
Tags: with_gvisor,with_quic,with_utls,with_xhttp,with_awg,with_lx_command,with_lxd,with_lx_chain
Revision: deadbeef
CGO: disabled`

	chainVersionWithoutTag = `sing-box version 1.14.0-lx.27-rc.3-dev

Environment: go1.25.12 darwin/arm64
Tags: with_gvisor,with_quic,with_utls,with_xhttp,with_awg,with_lx_command,with_lxd
Revision: deadbeef
CGO: disabled`
)

func TestChainVerdict_TagPresent(t *testing.T) {
	ok, reason := chainVerdictFromVersionOutput(chainVersionWithTag)
	if !ok {
		t.Fatalf("ядро с with_lx_chain признано неподдерживающим: %s", reason)
	}
}

func TestChainVerdict_TagMissing(t *testing.T) {
	ok, reason := chainVerdictFromVersionOutput(chainVersionWithoutTag)
	if ok {
		t.Fatal("ядро без with_lx_chain признано поддерживающим — конфиг не стартует целиком")
	}
	// Пользователю нужна версия: без неё непонятно, какое ядро обновлять.
	if !strings.Contains(reason, "1.14.0-lx.27-rc.3-dev") {
		t.Errorf("причина не называет версию ядра: %q", reason)
	}
	if !strings.Contains(reason, chainBuildTag) {
		t.Errorf("причина не называет тег сборки: %q", reason)
	}
}

// Неизвестный формат вывода — не деградируем: отнять рабочий маршрут по
// догадке хуже, чем положиться на `sing-box check`.
func TestChainVerdict_UnknownOutputAssumesSupported(t *testing.T) {
	for _, out := range []string{"", "какой-то другой вывод", "sing-box version 1.2.3"} {
		if ok, _ := chainVerdictFromVersionOutput(out); !ok {
			t.Errorf("деградация по догадке на выводе %q", out)
		}
	}
}

// Тег-подстрока не считается: `with_lx_chainsomething` не даёт `chain`.
func TestChainVerdict_TagIsNotSubstring(t *testing.T) {
	out := "sing-box version 1.0\nTags: with_quic,with_lx_chain_experimental\n"
	if ok, _ := chainVerdictFromVersionOutput(out); ok {
		t.Error("тег-подстрока принята за with_lx_chain")
	}
}
