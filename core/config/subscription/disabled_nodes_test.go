package subscription

import (
	"strings"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 D4 + SPEC 112 — выключение отдельной ноды по её идентичности (тегу
// в рамках источника).

// Критерий приёмки 3: правка содержимого узла не рвёт отметку выключения.
// Провайдер вправе крутить сервер, порт и ключ под тем же именем.
func TestDisabledNodeSurvivesContentChange(t *testing.T) {
	const tag = "🇩🇪 Frankfurt"
	before := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#" + tag
	after := "vless://11111111-2222-3333-4444-555555555555@other.com:8443?security=tls&sni=other.com&type=ws&path=%2Fws#" + tag

	first := loadFromInlineBody(t, before, configtypes.ProxySource{})
	if len(first.Nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(first.Nodes))
	}
	id := first.Nodes[0].IdentityTag
	if id != tag {
		t.Fatalf("идентичность = %q, ожидалась %q", id, tag)
	}

	// Провайдер поменял всё, кроме имени — отметка обязана действовать.
	second := loadFromInlineBody(t, after, configtypes.ProxySource{
		DisabledNodes: map[string]int64{id: time.Now().Unix()},
	})
	if len(second.Nodes) != 0 {
		t.Fatalf("выключенный узел просочился в конфиг: %+v", second.Nodes[0])
	}
}

// Смена tag_prefix источника отметку не роняет: идентичность снята до неё.
func TestDisabledNodeSurvivesTagPrefixChange(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#🇩🇪 DE"

	plain := loadFromInlineBody(t, uri, configtypes.ProxySource{})
	id := plain.Nodes[0].IdentityTag

	prefixed := loadFromInlineBody(t, uri, configtypes.ProxySource{
		TagPrefix:     "AL:",
		DisabledNodes: map[string]int64{id: time.Now().Unix()},
	})
	if len(prefixed.Nodes) != 0 {
		t.Fatalf("правка tag_prefix отцепила отметку, узел вернулся: %q", prefixed.Nodes[0].Tag)
	}
}

// Выключение одной ноды не задевает соседей.
func TestDisabledNodeDoesNotAffectOthers(t *testing.T) {
	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@keep.com:443?security=tls&sni=keep.com#Keep",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#Drop",
	}, "\n")

	all := loadFromInlineBody(t, body, configtypes.ProxySource{})
	if len(all.Nodes) != 2 {
		t.Fatalf("базовый прогон: получено %d узлов, ожидалось 2", len(all.Nodes))
	}

	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{"Drop": time.Now().Unix()},
	})

	if len(res.Nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(res.Nodes))
	}
	if res.Nodes[0].Server != "keep.com" {
		t.Fatalf("выживший узел = %q, ожидался keep.com", res.Nodes[0].Server)
	}
}

// Тёзки одного источника выключаются раздельно: у них разные идентичности.
func TestDisabledNodeDistinguishesNamesakes(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com"
	body := uri + "#🇳🇱 NL\n" + uri + "#🇳🇱 NL"

	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{"🇳🇱 NL-2": time.Now().Unix()},
	})

	if len(res.Nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(res.Nodes))
	}
	if res.Nodes[0].IdentityTag != "🇳🇱 NL" {
		t.Fatalf("выжил узел %q, ожидался первый тёзка", res.Nodes[0].IdentityTag)
	}
}

// Отметка ноды, встреченной в подписке, продлевается — иначе GC снёс бы её,
// пока нода всё ещё на месте.
func TestDisabledNodeMarkIsRefreshedWhenSeen(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#Drop"

	stale := time.Now().Add(-20 * 24 * time.Hour).Unix()
	res := loadFromInlineBody(t, uri, configtypes.ProxySource{
		DisabledNodes: map[string]int64{"Drop": stale},
	})

	ts, ok := res.DisabledNodes["Drop"]
	if !ok {
		t.Fatal("отметка узла, который всё ещё на месте, обязана сохраниться")
	}
	if ts <= stale {
		t.Fatalf("время отметки = %d, ожидалось обновление после %d", ts, stale)
	}
	if res.DisabledMigrated {
		t.Error("продление отметки не миграция — флаг поднимать нельзя")
	}
}

// Критерий 22: отметка ноды, отсутствующей дольше TTL, удаляется.
func TestGCDropsExpiredMarks(t *testing.T) {
	now := time.Now()
	ttl := 24 * time.Hour

	disabled := map[string]int64{
		"fresh":   now.Add(-1 * time.Hour).Unix(),
		"expired": now.Add(-48 * time.Hour).Unix(),
	}

	kept := gcDisabledNodes(disabled, ttl, now)

	if _, ok := kept["fresh"]; !ok {
		t.Error("недавно подтверждённая отметка обязана пережить GC")
	}
	if _, ok := kept["expired"]; ok {
		t.Error("отметка старше TTL обязана быть удалена")
	}
}

func TestGCHandlesEmptyMap(t *testing.T) {
	if got := gcDisabledNodes(nil, time.Hour, time.Now()); len(got) != 0 {
		t.Fatalf("получено %d отметок, ожидалось 0", len(got))
	}
}

func TestDisabledNodeTTLClamping(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		want     time.Duration
	}{
		{name: "нулевой интервал падает на пол", interval: 0, want: 24 * time.Hour},
		{name: "короткий интервал клампится к полу", interval: 1, want: 24 * time.Hour},
		{name: "типовой интервал множится на три", interval: 24, want: 72 * time.Hour},
		{name: "длинный интервал упирается в 30 суток", interval: 500, want: 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := disabledNodeTTL(tt.interval); got != tt.want {
				t.Fatalf("disabledNodeTTL(%d) = %v, ожидалось %v", tt.interval, got, tt.want)
			}
		})
	}
}

// Пустая карта отметок — фильтр не трогает ничего.
func TestDisabledFilterIsInertWithoutMarks(t *testing.T) {
	nodes := []*configtypes.ParsedNode{{Tag: "a", Scheme: "vless", IdentityTag: "a"}}
	got, _, migrated := filterDisabledNodes(nodes, nil, time.Now(), true)

	if len(got) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1", len(got))
	}
	if migrated {
		t.Error("без отметок мигрировать нечего")
	}
}

// SPEC 112: узел-группа идентичности не имеет и выключаться не может.
// Отметка с тегом группы её не задевает.
func TestDisabledFilterSkipsGroupNodes(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"urltest","tag":"auto","outbounds":["a"]}
	  ]
	}`

	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{"auto": time.Now().Unix()},
	})

	if got := len(groupNodesOf(res.Nodes)); got != 1 {
		t.Fatalf("узел-группа выключен отметкой, хотя идентичности не имеет (осталось %d)", got)
	}
}
