package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 112 — миграция отметок выключения с упразднённого контент-хеша на
// тег-идентичность.
//
// Хук LegacyNodeIdentityHashFunc в пакете subscription приложением не
// установлен (его ставит config), поэтому тесты подставляют его сами. Реальный
// хеш здесь не нужен: миграция обязана работать с ЛЮБОЙ функцией, лишь бы она
// давала 64 hex-символа — форму, по которой legacy-ключ и опознаётся.

// withLegacyHashHook подставляет детерминированный «хеш от содержимого»,
// намеренно НЕ зависящий от тега: ровно этим свойством обладал упразднённый
// sha256 от эмиссии, и на нём держится опознание узла.
func withLegacyHashHook(t *testing.T) func(*configtypes.ParsedNode) string {
	t.Helper()
	fn := func(node *configtypes.ParsedNode) string {
		if node == nil || node.Server == "" {
			return ""
		}
		sum := sha256.Sum256([]byte(node.Scheme + "|" + node.Server + "|" + node.UUID))
		return hex.EncodeToString(sum[:])
	}
	prev := LegacyNodeIdentityHashFunc
	LegacyNodeIdentityHashFunc = fn
	t.Cleanup(func() { LegacyNodeIdentityHashFunc = prev })
	return fn
}

// Критерий приёмки 4: legacy state.json с disabled_nodes-хешами — отметки
// переживают первый парс и переписываются на тег-ключи.
func TestLegacyDisabledKeyMigratesToTag(t *testing.T) {
	legacy := withLegacyHashHook(t)

	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#🇩🇪 DE"

	probe, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := legacy(probe)
	if len(oldKey) != 64 {
		t.Fatalf("тестовый legacy-хеш = %q, ожидались 64 hex-символа", oldKey)
	}

	stamped := time.Now().Add(-time.Hour).Unix()
	res := loadFromInlineBody(t, uri, configtypes.ProxySource{
		DisabledNodes: map[string]int64{oldKey: stamped},
	})

	// Отметка действует: узел выключен уже на этом прогоне.
	if len(res.Nodes) != 0 {
		t.Fatalf("выключенный узел просочился в конфиг: %q", res.Nodes[0].Tag)
	}
	// И переписана на тег-ключ.
	if !res.DisabledMigrated {
		t.Error("флаг DisabledMigrated не поднят — вызывающий не сохранит результат")
	}
	if _, stale := res.DisabledNodes[oldKey]; stale {
		t.Errorf("legacy-ключ %q остался в карте", oldKey)
	}
	if _, ok := res.DisabledNodes["🇩🇪 DE"]; !ok {
		t.Fatalf("тег-ключ не появился, карта: %v", res.DisabledNodes)
	}
}

// Legacy-ключ, под который ни один узел источника не подходит, выбрасывается:
// узла с таким содержимым нет, отметка и так мертва.
func TestUnmatchedLegacyDisabledKeyIsDropped(t *testing.T) {
	withLegacyHashHook(t)

	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@keep.com:443?security=tls&sni=keep.com#Keep"
	ghost := strings.Repeat("a", 64)

	res := loadFromInlineBody(t, uri, configtypes.ProxySource{
		DisabledNodes: map[string]int64{ghost: time.Now().Unix()},
	})

	if len(res.Nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(res.Nodes))
	}
	if _, stale := res.DisabledNodes[ghost]; stale {
		t.Error("неопознанный legacy-ключ обязан быть выброшен")
	}
	if !res.DisabledMigrated {
		t.Error("выброс legacy-ключа — тоже изменение карты, флаг обязан подняться")
	}
}

// Legacy-ключ опознаётся по ВЫКЛЮЧЕННОМУ узлу: миграция обязана идти по
// полному списку, до того как фильтр этот узел выбросит.
func TestLegacyMigrationSeesDisabledNodeItself(t *testing.T) {
	legacy := withLegacyHashHook(t)

	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@keep.com:443?security=tls&sni=keep.com#Keep",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#Drop",
	}, "\n")

	probe, err := ParseNode("vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#Drop", nil)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := legacy(probe)

	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{oldKey: time.Now().Unix()},
	})

	if len(res.Nodes) != 1 || res.Nodes[0].Server != "keep.com" {
		t.Fatalf("ожидался один выживший узел keep.com, получено %d", len(res.Nodes))
	}
	if _, ok := res.DisabledNodes["Drop"]; !ok {
		t.Fatalf("отметка выключенного узла не мигрировала, карта: %v", res.DisabledNodes)
	}
}

// Смешанная карта: тег-ключи уже мигрировавших отметок остаются нетронутыми.
func TestMixedDisabledKeysKeepTagEntries(t *testing.T) {
	legacy := withLegacyHashHook(t)

	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@one.com:443?security=tls&sni=one.com#One",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@two.com:443?security=tls&sni=two.com#Two",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@three.com:443?security=tls&sni=three.com#Three",
	}, "\n")

	probe, err := ParseNode("vless://b831381d-6324-4d53-ad4f-8cda48b30811@two.com:443?security=tls&sni=two.com#Two", nil)
	if err != nil {
		t.Fatal(err)
	}

	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{
			"One":         time.Now().Unix(), // уже мигрировавшая отметка
			legacy(probe): time.Now().Unix(), // legacy-хеш
		},
	})

	if len(res.Nodes) != 1 || res.Nodes[0].IdentityTag != "Three" {
		tags := make([]string, 0, len(res.Nodes))
		for _, n := range res.Nodes {
			tags = append(tags, n.IdentityTag)
		}
		t.Fatalf("ожидался один выживший узел Three, получено %v", tags)
	}
	for _, want := range []string{"One", "Two"} {
		if _, ok := res.DisabledNodes[want]; !ok {
			t.Errorf("нет тег-ключа %q, карта: %v", want, res.DisabledNodes)
		}
	}
}

// Без хука legacy-ключи не выбрасываются: доживают до запуска, где хук есть.
// Иначе прогон парсера в изоляции стёр бы пользовательские отметки.
func TestLegacyKeysSurviveWithoutHook(t *testing.T) {
	prev := LegacyNodeIdentityHashFunc
	LegacyNodeIdentityHashFunc = nil
	t.Cleanup(func() { LegacyNodeIdentityHashFunc = prev })

	oldKey := strings.Repeat("b", 64)
	nodes := []*configtypes.ParsedNode{{Tag: "a", Scheme: "vless", IdentityTag: "a", Server: "e.com"}}
	got, refreshed, migrated := filterDisabledNodes(nodes, map[string]int64{oldKey: 1}, time.Now())

	if len(got) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(got))
	}
	if migrated {
		t.Error("без хука мигрировать нечем — флаг подниматься не должен")
	}
	if _, ok := refreshed[oldKey]; !ok {
		t.Error("legacy-ключ обязан дожить до запуска с хуком, а не пропасть молча")
	}
}

// isLegacyIdentityHash различает форму: тег из 64 hex-символов теоретически
// возможен, но всё, что короче/длиннее/не-hex, миграцией не трогается.
func TestIsLegacyIdentityHashShape(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{strings.Repeat("0", 64), true},
		{strings.Repeat("f", 64), true},
		{strings.Repeat("F", 64), false}, // верхний регистр — не наш формат
		{strings.Repeat("a", 63), false},
		{strings.Repeat("a", 65), false},
		{"🇩🇪 DE", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isLegacyIdentityHash(tt.key); got != tt.want {
			t.Errorf("isLegacyIdentityHash(%q) = %v, ожидалось %v", tt.key, got, tt.want)
		}
	}
}
