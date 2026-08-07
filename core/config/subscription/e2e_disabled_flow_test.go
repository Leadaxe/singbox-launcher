package subscription

import (
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 D4 — сквозной пользовательский сценарий целиком.
//
// Проверяется путь, который пользователь проходит руками: увидел ноды →
// выключил одну → подписка обновилась и провайдер её переименовал → нода
// осталась выключенной → пользователь включил обратно → она вернулась.
//
// Каждый шаг здесь — отдельный прогон загрузчика, как при реальном обновлении.

func TestUserDisablesNodeThenReenablesIt(t *testing.T) {
	withIdentityHash(t)

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

	var dropHash string
	for _, n := range initial.Nodes {
		if n.Server == "drop.com" {
			dropHash = NodeIdentityHashFunc(n)
		}
	}
	if dropHash == "" {
		t.Fatal("step 1: could not resolve the hash of the node to disable")
	}

	// Шаг 2. Снял галку — в состоянии появилась отметка.
	source := configtypes.ProxySource{
		DisabledNodes: map[string]int64{dropHash: time.Now().Unix()},
	}
	afterDisable := loadFromInlineBody(t, body, source)
	if len(afterDisable.Nodes) != 1 || afterDisable.Nodes[0].Server != "keep.com" {
		t.Fatalf("step 2: disabled node still present: %d nodes", len(afterDisable.Nodes))
	}

	// Шаг 3. Провайдер обновил подписку, переименовал ноды и поменял их
	// местами. Отметка привязана к хешу, поэтому обязана уцелеть.
	renamed := dropURI + "#🇩🇪 Frankfurt Premium\n" + keepURI + "#🇳🇱 Amsterdam"
	afterRefresh := loadFromInlineBody(t, renamed, configtypes.ProxySource{
		DisabledNodes: afterDisable.DisabledNodes,
	})
	if len(afterRefresh.Nodes) != 1 {
		t.Fatalf("step 3: got %d nodes, want 1 — mark did not survive the rename", len(afterRefresh.Nodes))
	}
	if afterRefresh.Nodes[0].Server != "keep.com" {
		t.Fatalf("step 3: surviving node = %q, want keep.com", afterRefresh.Nodes[0].Server)
	}

	// Шаг 4. Пользователь вернул галку — нода снова в конфиге.
	afterEnable := loadFromInlineBody(t, renamed, configtypes.ProxySource{})
	if len(afterEnable.Nodes) != 2 {
		t.Fatalf("step 4: got %d nodes, want 2 — node did not come back", len(afterEnable.Nodes))
	}
}

// Отметка не переезжает на другую ноду, даже если та заняла её место и имя.
//
// Это главный аргумент в пользу хеша: с ключом по тегу или позиции
// пользователь выключил бы один сервер, а лишился другого.
func TestDisabledMarkDoesNotMigrateToAnotherServer(t *testing.T) {
	withIdentityHash(t)

	const oldURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@old.com:443?security=tls&sni=old.com"
	const newURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@new.com:443?security=tls&sni=new.com"

	first := loadFromInlineBody(t, oldURI+"#Server 1", configtypes.ProxySource{})
	oldHash := NodeIdentityHashFunc(first.Nodes[0])

	// Провайдер заменил сервер, оставив то же имя и ту же позицию.
	res := loadFromInlineBody(t, newURI+"#Server 1", configtypes.ProxySource{
		DisabledNodes: map[string]int64{oldHash: time.Now().Unix()},
	})

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 — the mark must not apply to a different server", len(res.Nodes))
	}
	if res.Nodes[0].Server != "new.com" {
		t.Fatalf("surviving node = %q, want new.com", res.Nodes[0].Server)
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
