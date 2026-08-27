package subscription

import (
	"strings"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 113-A §5 — уникализация не должна занимать чужое имя (находка аудита
// M2).
//
// Подписка `X, X-2, X`: второй `X` получал сгенерированное имя `X-2`, уже
// занятое НАСТОЯЩИМ узлом из выдачи. Для идентичностей это значило две записи
// с одним ключом — отметка выключения гасила оба узла разом. Для конфиговых
// тегов это дубль тега в outbounds, на котором ядро отказывается стартовать.

// Идентичности: `X, X-2, X` → `X, X-2, X-3`.
func TestIdentityUniquifySkipsTakenNames(t *testing.T) {
	idCounts := map[string]int{}
	got := []string{
		makeIdentityUnique("X", idCounts),
		makeIdentityUnique("X-2", idCounts),
		makeIdentityUnique("X", idCounts),
	}
	want := []string{"X", "X-2", "X-3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("идентичность #%d = %q, ожидалась %q (все: %v)", i+1, got[i], want[i], got)
		}
	}
}

// Конфиговые теги: тот же изъян, та же починка. Дубль тега ядро отвергает.
func TestMakeTagUniqueSkipsTakenNames(t *testing.T) {
	tagCounts := map[string]int{}
	got := []string{
		MakeTagUnique("X", tagCounts, "Test"),
		MakeTagUnique("X-2", tagCounts, "Test"),
		MakeTagUnique("X", tagCounts, "Test"),
		MakeTagUnique("X", tagCounts, "Test"),
	}
	want := []string{"X", "X-2", "X-3", "X-4"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("тег #%d = %q, ожидался %q (все: %v)", i+1, got[i], want[i], got)
		}
	}
	// И ни одного повтора.
	seen := map[string]bool{}
	for _, tag := range got {
		if seen[tag] {
			t.Fatalf("тег %q выдан дважды (все: %v)", tag, got)
		}
		seen[tag] = true
	}
}

// Занятое имя может встретиться и ПОСЛЕ генерации: `X, X, X-2` → третий узел
// вынужден уступить, потому что `X-2` уже выдан.
func TestMakeTagUniqueHandlesLateCollision(t *testing.T) {
	tagCounts := map[string]int{}
	got := []string{
		MakeTagUnique("X", tagCounts, "Test"),
		MakeTagUnique("X", tagCounts, "Test"),
		MakeTagUnique("X-2", tagCounts, "Test"),
	}
	if got[1] != "X-2" {
		t.Fatalf("второй тег = %q, ожидался X-2", got[1])
	}
	if got[2] == got[1] {
		t.Fatalf("настоящий тег X-2 столкнулся со сгенерированным: %v", got)
	}
}

// Сквозной путь: подписка `X, X-2, X` даёт три РАЗНЫЕ идентичности, и отметка
// на `X-2` гасит ровно один узел — второй.
func TestSubscriptionXX2XKeepsIdentitiesDistinct(t *testing.T) {
	const uuid = "b831381d-6324-4d53-ad4f-8cda48b30811"
	body := strings.Join([]string{
		"vless://" + uuid + "@one.com:443?security=tls&sni=one.com#X",
		"vless://" + uuid + "@two.com:443?security=tls&sni=two.com#X-2",
		"vless://" + uuid + "@three.com:443?security=tls&sni=three.com#X",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	ids := make([]string, 0, len(res.Nodes))
	for _, n := range res.Nodes {
		ids = append(ids, n.IdentityTag)
	}
	want := []string{"X", "X-2", "X-3"}
	if len(ids) != len(want) {
		t.Fatalf("получено %d узлов, ожидалось 3 (идентичности: %v)", len(ids), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("идентичность #%d = %q, ожидалась %q (все: %v)", i+1, ids[i], want[i], ids)
		}
	}

	// Отметка на настоящем X-2 гасит только его.
	off := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{"X-2": time.Now().Unix()},
	})
	if len(off.Nodes) != 2 {
		t.Fatalf("отметка на X-2 погасила %d узлов из 3, ожидался ровно один",
			3-len(off.Nodes))
	}
	for _, n := range off.Nodes {
		if n.Server == "two.com" {
			t.Fatal("выключенным оказался не тот узел")
		}
	}
}
