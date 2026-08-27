package subscription

import (
	"strings"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 113 решение 2 / SPEC 113-A §3 — «нет достоверного ответа от подписки,
// состояние не меняем».
//
// Находка аудита C5: миграция legacy-ключей выбрасывала всё, что не совпало,
// даже когда разбор пустой (сеть отпала) или урезан капом. Отметки выключения
// пользователя при этом умирали молча, и узлы включались обратно за его спиной.

// Пустой разбор: legacy-ключи обязаны остаться как есть.
func TestUntrustedEmptyParseKeepsLegacyKeys(t *testing.T) {
	withLegacyHashHook(t)

	oldKey := strings.Repeat("c", 64)
	_, refreshed, migrated := filterDisabledNodes(
		nil, map[string]int64{oldKey: 100}, time.Now(), false)

	if _, ok := refreshed[oldKey]; !ok {
		t.Fatalf("legacy-ключ выброшен на недостоверном разборе, карта: %v", refreshed)
	}
	if migrated {
		t.Error("флаг миграции поднят, хотя карта не менялась")
	}
}

// Разбор, урезанный капом MaxNodesPerSubscription, тоже недостоверен: узел
// отметки мог не попасть в список просто потому, что не влез.
func TestUntrustedCappedParseKeepsLegacyKeys(t *testing.T) {
	withLegacyHashHook(t)

	oldKey := strings.Repeat("d", 64)
	nodes := []*configtypes.ParsedNode{
		{Tag: "a", IdentityTag: "a", Scheme: "vless", Server: "e.com"},
	}
	_, refreshed, _ := filterDisabledNodes(
		nodes, map[string]int64{oldKey: 100}, time.Now(), false)

	if _, ok := refreshed[oldKey]; !ok {
		t.Fatalf("legacy-ключ выброшен при урезанном разборе, карта: %v", refreshed)
	}
}

// Честный разбор выбрасывает неопознанный ключ, как и раньше: узла с таким
// содержимым в источнике действительно нет.
func TestTrustedParseStillDropsUnmatchedLegacyKeys(t *testing.T) {
	withLegacyHashHook(t)

	oldKey := strings.Repeat("e", 64)
	nodes := []*configtypes.ParsedNode{
		{Tag: "a", IdentityTag: "a", Scheme: "vless", Server: "e.com"},
	}
	_, refreshed, migrated := filterDisabledNodes(
		nodes, map[string]int64{oldKey: 100}, time.Now(), true)

	if _, ok := refreshed[oldKey]; ok {
		t.Fatalf("неопознанный ключ пережил достоверный разбор, карта: %v", refreshed)
	}
	if !migrated {
		t.Error("выброс ключа — изменение карты, флаг обязан подняться")
	}
}

// Перепись СОВПАВШЕГО ключа безопасна всегда — даже когда разбору не верим:
// отметка не теряется, просто меняет форму.
func TestUntrustedParseStillRewritesMatchedLegacyKeys(t *testing.T) {
	legacy := withLegacyHashHook(t)

	node := &configtypes.ParsedNode{Tag: "DE", IdentityTag: "DE", Scheme: "vless", Server: "e.com"}
	oldKey := legacy(node)

	_, refreshed, migrated := filterDisabledNodes(
		[]*configtypes.ParsedNode{node}, map[string]int64{oldKey: 100}, time.Now(), false)

	if _, stale := refreshed[oldKey]; stale {
		t.Errorf("совпавший legacy-ключ остался в старой форме: %v", refreshed)
	}
	if _, ok := refreshed["DE"]; !ok {
		t.Fatalf("совпавший ключ не переписан на тег, карта: %v", refreshed)
	}
	if !migrated {
		t.Error("перепись — изменение карты, флаг обязан подняться")
	}
}

// Сквозной путь: тело пришло, но ни одного узла из него не вышло (провайдер
// отдал мусор, вся выдача не распарсилась). Отметки обязаны уцелеть целиком —
// иначе один кривой ответ стирает пользовательские выключения.
func TestEmptyParseKeepsDisabledMarksEndToEnd(t *testing.T) {
	withLegacyHashHook(t)

	oldKey := strings.Repeat("f", 64)
	res := loadFromInlineBody(t, "не-URI\nтоже-не-URI", configtypes.ProxySource{
		DisabledNodes: map[string]int64{oldKey: 100, "DE": 100},
	})

	if len(res.Nodes) != 0 {
		t.Fatalf("нераспарсиваемое тело дало %d узлов", len(res.Nodes))
	}
	for _, want := range []string{oldKey, "DE"} {
		if _, ok := res.DisabledNodes[want]; !ok {
			t.Fatalf("отметка %q потеряна на пустом разборе, карта: %v", want, res.DisabledNodes)
		}
	}
	if res.DisabledMigrated {
		t.Error("флаг миграции поднят на пустом разборе — персистить нечего")
	}
}

// Сквозной путь наоборот: тело прочитано, узлы есть — неопознанный ключ
// выбрасывается, флаг персиста поднят.
func TestTrustedFetchDropsGhostLegacyKey(t *testing.T) {
	withLegacyHashHook(t)

	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@keep.com:443?security=tls&sni=keep.com#Keep"
	ghost := strings.Repeat("a", 64)

	res := loadFromInlineBody(t, uri, configtypes.ProxySource{
		DisabledNodes: map[string]int64{ghost: time.Now().Unix()},
	})

	if _, stale := res.DisabledNodes[ghost]; stale {
		t.Fatalf("призрачный ключ пережил достоверный разбор: %v", res.DisabledNodes)
	}
	if !res.DisabledMigrated {
		t.Error("флаг DisabledMigrated не поднят — вызывающий не сохранит очищенную карту")
	}
}
