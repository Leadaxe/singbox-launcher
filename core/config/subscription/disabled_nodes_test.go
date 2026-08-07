package subscription

import (
	"strings"
	"testing"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 D4 — выключение отдельной ноды по хешу идентичности.

// Критерий 21: выключенная нода не попадает в конфиг и остаётся выключенной
// после обновления подписки — отметка живёт по хешу, а не по тегу.
func TestDisabledNodeSurvivesProviderRename(t *testing.T) {
	withIdentityHash(t)

	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com"

	// Первый прогон: узнаём хеш ноды, которую пользователь хочет выключить.
	first := loadFromInlineBody(t, uri+"#Old Name", configtypes.ProxySource{})
	if len(first.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(first.Nodes))
	}
	hash := NodeIdentityHashFunc(first.Nodes[0])
	if hash == "" {
		t.Fatal("identity hash must not be empty")
	}

	// Провайдер переименовал ноду — отметка обязана продолжать действовать.
	second := loadFromInlineBody(t, uri+"#Brand New Name", configtypes.ProxySource{
		DisabledNodes: map[string]int64{hash: time.Now().Unix()},
	})

	if len(second.Nodes) != 0 {
		t.Fatalf("disabled node leaked into the config: %+v", second.Nodes[0])
	}
}

// Выключение одной ноды не задевает соседей.
func TestDisabledNodeDoesNotAffectOthers(t *testing.T) {
	withIdentityHash(t)

	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@keep.com:443?security=tls&sni=keep.com#Keep",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#Drop",
	}, "\n")

	all := loadFromInlineBody(t, body, configtypes.ProxySource{})
	if len(all.Nodes) != 2 {
		t.Fatalf("baseline: got %d nodes, want 2", len(all.Nodes))
	}

	var dropHash string
	for _, n := range all.Nodes {
		if n.Server == "drop.com" {
			dropHash = NodeIdentityHashFunc(n)
		}
	}
	if dropHash == "" {
		t.Fatal("could not resolve the hash of the node to disable")
	}

	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{dropHash: time.Now().Unix()},
	})

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(res.Nodes))
	}
	if res.Nodes[0].Server != "keep.com" {
		t.Fatalf("surviving node = %q, want keep.com", res.Nodes[0].Server)
	}
}

// Отметка ноды, встреченной в подписке, продлевается — иначе GC снёс бы её,
// пока нода всё ещё на месте.
func TestDisabledNodeMarkIsRefreshedWhenSeen(t *testing.T) {
	withIdentityHash(t)

	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@drop.com:443?security=tls&sni=drop.com#Drop"

	first := loadFromInlineBody(t, uri, configtypes.ProxySource{})
	hash := NodeIdentityHashFunc(first.Nodes[0])

	stale := time.Now().Add(-20 * 24 * time.Hour).Unix()
	res := loadFromInlineBody(t, uri, configtypes.ProxySource{
		DisabledNodes: map[string]int64{hash: stale},
	})

	ts, ok := res.DisabledNodes[hash]
	if !ok {
		t.Fatal("mark for a node still present must be kept")
	}
	if ts <= stale {
		t.Fatalf("mark timestamp = %d, want it refreshed past %d", ts, stale)
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
		t.Error("a recently confirmed mark must survive GC")
	}
	if _, ok := kept["expired"]; ok {
		t.Error("a mark older than the TTL must be dropped")
	}
}

func TestGCHandlesEmptyMap(t *testing.T) {
	if got := gcDisabledNodes(nil, time.Hour, time.Now()); len(got) != 0 {
		t.Fatalf("got %d marks, want 0", len(got))
	}
}

func TestDisabledNodeTTLClamping(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		want     time.Duration
	}{
		{name: "zero interval falls back to the floor", interval: 0, want: 24 * time.Hour},
		{name: "short interval is clamped to the floor", interval: 1, want: 24 * time.Hour},
		{name: "typical interval scales by three", interval: 24, want: 72 * time.Hour},
		{name: "long interval is capped at 30 days", interval: 500, want: 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := disabledNodeTTL(tt.interval); got != tt.want {
				t.Fatalf("disabledNodeTTL(%d) = %v, want %v", tt.interval, got, tt.want)
			}
		})
	}
}

// Без хука идентичности фильтрация не выполняется: парсер обязан работать
// в изоляции, а не выкидывать ноды наугад.
func TestDisabledFilterIsInertWithoutHook(t *testing.T) {
	prev := NodeIdentityHashFunc
	NodeIdentityHashFunc = nil
	t.Cleanup(func() { NodeIdentityHashFunc = prev })

	nodes := []*configtypes.ParsedNode{{Tag: "a", Scheme: "vless"}}
	got, _ := filterDisabledNodes(nodes, map[string]int64{"some-hash": 1}, time.Now())

	if len(got) != 1 {
		t.Fatalf("got %d nodes, want 1 — filtering must be inert without the hook", len(got))
	}
}

// Узел-группа тоже можно выключить: она рядовая нода списка.
func TestDisabledFilterAppliesToGroupNodes(t *testing.T) {
	withIdentityHash(t)

	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"urltest","tag":"auto","outbounds":["a"]}
	  ]
	}`

	all := loadFromInlineBody(t, body, configtypes.ProxySource{})
	groups := groupNodesOf(all.Nodes)
	if len(groups) != 1 {
		t.Fatalf("baseline: got %d group nodes, want 1", len(groups))
	}
	groupHash := NodeIdentityHashFunc(groups[0])
	if groupHash == "" {
		t.Skip("test identity hook yields no hash for group nodes")
	}

	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		DisabledNodes: map[string]int64{groupHash: time.Now().Unix()},
	})

	if got := len(groupNodesOf(res.Nodes)); got != 0 {
		t.Fatalf("disabled group node leaked into the config (%d left)", got)
	}
}
