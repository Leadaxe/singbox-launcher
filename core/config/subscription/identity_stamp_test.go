package subscription

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 112 — идентичность узла есть его тег; SPEC 112-B вернул дедуп записей
// по подключению (parse-слой, dedup_test.go). Здесь проверяется стемпинг
// идентичности, поэтому все узлы РАЗНЫЕ по кредам — иначе их схлопнул бы
// дедуп и тест проверял бы не то, что заявляет.
//
// Хуки NodeIdentityFunc / LegacyNodeIdentityHashFunc в тестах пакета
// subscription приложением не устанавливаются: парсер обязан оставаться
// работоспособным в изоляции, и встроенное правило (тег как идентичность) —
// часть контракта, а не заглушка.

// Разные серверы под разными именами — три узла, каждому проставлена
// идентичность.
func TestIdentityStampedForEveryURINode(t *testing.T) {
	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@a.com:443?security=tls&sni=a.com#🇳🇱 NL-1",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@b.com:443?security=tls&sni=b.com#🇳🇱 Amsterdam",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@c.com:443?security=tls&sni=c.com#🇳🇱 Fast",
	}, "\n")

	res := parseInlineBody(t, body, nil)

	if len(res.Entries) != 3 {
		t.Fatalf("получено %d записей, ожидалось 3 (теги: %v)", len(res.Entries), rawTagsOf(res))
	}
	for _, e := range res.Entries {
		if e.RawTag == "" || e.Node.IdentityTag != e.RawTag {
			t.Fatalf("узлу %q не проставлена идентичность (сырой тег %q, identity %q)",
				e.Node.Tag, e.RawTag, e.Node.IdentityTag)
		}
	}
}

// Полные тёзки одного источника (но РАЗНЫЕ подключения) разводятся суффиксом и
// получают РАЗНЫЕ идентичности: иначе одна отметка выключения накрыла бы обе
// строки.
func TestDuplicateTagsGetDistinctIdentities(t *testing.T) {
	body := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@a.com:443?security=tls&sni=a.com#🇳🇱 NL\n" +
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@b.com:443?security=tls&sni=b.com#🇳🇱 NL"

	res := parseInlineBody(t, body, nil)

	if len(res.Entries) != 2 {
		t.Fatalf("получено %d записей, ожидалось 2", len(res.Entries))
	}
	if a, b := res.Entries[0].RawTag, res.Entries[1].RawTag; a == b {
		t.Fatalf("тёзки получили одну идентичность %q", a)
	}
	if got := res.Entries[1].RawTag; got != "🇳🇱 NL-2" {
		t.Errorf("идентичность второго = %q, ожидалась «🇳🇱 NL-2»", got)
	}
}

// Порядок «идентичность ДО tag_prefix», импорт sing-box с префиксом и
// счётчик идентичности на источник проверяются сквозь эмиссию — тег-политику
// применяет она (config/canonical_emit_test.go, TestBodyEmit_*).

// Узлы-группы идентичности не получают.
func TestStampNodeIdentitySkipsGroups(t *testing.T) {
	group := &configtypes.ParsedNode{Tag: "auto", Scheme: configtypes.SchemeGroup}
	if got := StampNodeIdentity(group, map[string]int{}); got != "" {
		t.Fatalf("группа получила идентичность %q", got)
	}
	if group.IdentityTag != "" {
		t.Fatalf("группе проставлен IdentityTag %q", group.IdentityTag)
	}
}
