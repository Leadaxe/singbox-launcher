package subscription

import (
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 D4 + SPEC 112 — сквозной пользовательский сценарий целиком.
//
// Проверяется путь, который пользователь проходит руками: увидел ноды →
// выключил одну → подписка обновилась, провайдер поменял под тем же именем
// сервер и переставил ноды → нода осталась выключенной → пользователь включил
// обратно → она вернулась.
//
// Каждый шаг здесь — отдельный прогон загрузчика, как при реальном обновлении.

func TestUserDisablesNodeThenReenablesIt(t *testing.T) {
	const (
		keepURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@keep.com:443?security=tls&sni=keep.com"
		dropURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com"
	)

	// Шаг 1. Пользователь открыл превью и видит обе ноды.
	body := keepURI + "#Keep me\n" + dropURI + "#Drop me"
	initial := loadFromInlineBody(t, body, configtypes.ProxySource{})
	if len(initial.Nodes) != 2 {
		t.Fatalf("step 1: got %d nodes, want 2", len(initial.Nodes))
	}

	var dropID string
	for _, n := range initial.Nodes {
		if n.Server == "drop.com" {
			dropID = n.IdentityTag
		}
	}
	if dropID == "" {
		t.Fatal("шаг 1: у ноды, которую выключаем, нет идентичности")
	}

	// Шаг 2. Снял галку — в состоянии появилась отметка.
	source := configtypes.ProxySource{
		DisabledNodes: map[string]int64{dropID: time.Now().Unix()},
	}
	afterDisable := loadFromInlineBody(t, body, source)
	if len(afterDisable.Nodes) != 1 || afterDisable.Nodes[0].Server != "keep.com" {
		t.Fatalf("step 2: disabled node still present: %d nodes", len(afterDisable.Nodes))
	}

	// Шаг 3. Провайдер обновил подписку: ноды поменялись местами, а под
	// именем «Drop me» приехал ДРУГОЙ сервер (штатная ротация IP). Отметка
	// привязана к имени, поэтому обязана уцелеть — под этим именем узел и
	// был выключен.
	const rotatedURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@rotated.com:8443?security=tls&sni=rotated.com"
	rotated := rotatedURI + "#Drop me\n" + keepURI + "#Keep me"
	afterRefresh := loadFromInlineBody(t, rotated, configtypes.ProxySource{
		DisabledNodes: afterDisable.DisabledNodes,
	})
	if len(afterRefresh.Nodes) != 1 {
		t.Fatalf("шаг 3: получено %d узлов, ожидался 1 — отметка не пережила обновление", len(afterRefresh.Nodes))
	}
	if afterRefresh.Nodes[0].Server != "keep.com" {
		t.Fatalf("шаг 3: выживший узел = %q, ожидался keep.com", afterRefresh.Nodes[0].Server)
	}

	// Шаг 4. Пользователь вернул галку — нода снова в конфиге.
	afterEnable := loadFromInlineBody(t, rotated, configtypes.ProxySource{})
	if len(afterEnable.Nodes) != 2 {
		t.Fatalf("шаг 4: получено %d узлов, ожидалось 2 — нода не вернулась", len(afterEnable.Nodes))
	}
}

// Отметка НЕ переезжает на соседа при перестановке узлов: ключ — имя, а не
// позиция в списке. С позиционным ключом пользователь выключил бы один сервер,
// а лишился другого.
func TestDisabledMarkDoesNotMigrateOnReorder(t *testing.T) {
	const a = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@a.com:443?security=tls&sni=a.com#Server A"
	const b = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@b.com:443?security=tls&sni=b.com#Server B"

	disabled := map[string]int64{"Server A": time.Now().Unix()}

	// Порядок в подписке — обратный тому, в котором пользователь ставил галку.
	res := loadFromInlineBody(t, b+"\n"+a, configtypes.ProxySource{DisabledNodes: disabled})

	if len(res.Nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(res.Nodes))
	}
	if res.Nodes[0].Server != "b.com" {
		t.Fatalf("выживший узел = %q, ожидался b.com — отметка уехала на соседа", res.Nodes[0].Server)
	}
}

// Обратная сторона решения SPEC 112, зафиксированная честно: переименование
// узла провайдером отметку ТЕРЯЕТ — имя и есть идентичность. Отметка не
// переезжает молча на чужой узел, а доживает свой TTL и исчезает.
func TestDisabledMarkIsLostOnProviderRename(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com"

	res := loadFromInlineBody(t, uri+"#Brand New Name", configtypes.ProxySource{
		DisabledNodes: map[string]int64{"Old Name": time.Now().Unix()},
	})

	if len(res.Nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1 — переименованный узел обязан вернуться", len(res.Nodes))
	}
	// Отметка при этом остаётся в карте: её уберёт штатный TTL-GC, а не парс.
	if _, ok := res.DisabledNodes["Old Name"]; !ok {
		t.Error("отметку убирает GC по TTL, а не парс")
	}
}

// Отметка исчезнувшей ноды доживает до TTL, а потом убирается.
func TestVanishedNodeMarkExpiresOnlyAfterTTL(t *testing.T) {
	now := time.Now()

	// Нода пропала из подписки день назад при суточном интервале обновления:
	// TTL = clamp(3×24h, 24h, 30d) = 72h — отметка ещё нужна.
	recent := map[string]int64{"gone": now.Add(-24 * time.Hour).Unix()}
	if kept := GCDisabledNodes(recent, 24, now); len(kept) != 1 {
		t.Fatalf("mark dropped too early: %v", kept)
	}

	// Прошло больше TTL — отметка бесполезна и удаляется.
	stale := map[string]int64{"gone": now.Add(-96 * time.Hour).Unix()}
	if kept := GCDisabledNodes(stale, 24, now); len(kept) != 0 {
		t.Fatalf("expired mark must be dropped: %v", kept)
	}
}
