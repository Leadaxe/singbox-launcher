package subscription

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 112 — контент-дедуп упразднён; при дублях работает уникализация тегов
// и идентичностей.
//
// Хуки NodeIdentityFunc / LegacyNodeIdentityHashFunc в тестах пакета
// subscription приложением не устанавливаются: парсер обязан оставаться
// работоспособным в изоляции, и встроенное правило (тег как идентичность) —
// часть контракта, а не заглушка.

// Один сервер, повторённый трижды с разными ремарками, теперь даёт ТРИ узла:
// имена разные, значит и узлы разные. Раньше их схлопывал контент-хеш —
// вместе с ним уезжали и пользовательские отметки, привязанные к содержимому.
func TestNoContentDedupInURIList(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com"
	body := strings.Join([]string{
		uri + "#🇳🇱 NL-1",
		uri + "#🇳🇱 Amsterdam",
		uri + "#🇳🇱 Fast",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 3 {
		got := make([]string, 0, len(res.Nodes))
		for _, n := range res.Nodes {
			got = append(got, n.Tag)
		}
		t.Fatalf("получено %d узлов, ожидалось 3 (теги: %v)", len(res.Nodes), got)
	}
	for _, n := range res.Nodes {
		if n.IdentityTag == "" {
			t.Fatalf("узлу %q не проставлена идентичность", n.Tag)
		}
	}
}

// Полные тёзки одного источника разводятся суффиксом и получают РАЗНЫЕ
// идентичности: иначе одна отметка выключения накрыла бы обе строки.
func TestDuplicateTagsGetDistinctIdentities(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com"
	body := uri + "#🇳🇱 NL\n" + uri + "#🇳🇱 NL"

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2", len(res.Nodes))
	}
	if a, b := res.Nodes[0].IdentityTag, res.Nodes[1].IdentityTag; a == b {
		t.Fatalf("тёзки получили одну идентичность %q", a)
	}
	if got := res.Nodes[1].IdentityTag; got != "🇳🇱 NL-2" {
		t.Errorf("идентичность второго = %q, ожидалась «🇳🇱 NL-2»", got)
	}
}

// Ловушка «порядок стемпинга тегов»: идентичность снимается ДО tag_prefix.
// Правка политики тегов источника не должна уводить отметки выключения.
func TestIdentityStampedBeforeTagPolicy(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#🇩🇪 DE"

	plain := loadFromInlineBody(t, uri, configtypes.ProxySource{})
	prefixed := loadFromInlineBody(t, uri, configtypes.ProxySource{TagPrefix: "AL:"})

	if len(plain.Nodes) != 1 || len(prefixed.Nodes) != 1 {
		t.Fatalf("ожидалось по 1 узлу, получено %d и %d", len(plain.Nodes), len(prefixed.Nodes))
	}
	if prefixed.Nodes[0].Tag == plain.Nodes[0].Tag {
		t.Fatal("tag_prefix не применился — тест не проверяет заявленное")
	}
	if plain.Nodes[0].IdentityTag != prefixed.Nodes[0].IdentityTag {
		t.Fatalf("tag_prefix увёл идентичность: %q → %q",
			plain.Nodes[0].IdentityTag, prefixed.Nodes[0].IdentityTag)
	}
}

// Импорт sing-box конфига тоже штампует идентичность, и тоже до префикса.
func TestIdentityStampedForSingboxImport(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"first","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"second","server":"e.com","server_port":443,"uuid":"u1"}
	  ]
	}`
	res := loadFromInlineBody(t, body, configtypes.ProxySource{TagPrefix: "AL:"})

	if len(res.Nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2 (дедупа больше нет)", len(res.Nodes))
	}
	want := []string{"first", "second"}
	for i, n := range res.Nodes {
		if n.IdentityTag != want[i] {
			t.Errorf("идентичность #%d = %q, ожидалась %q", i+1, n.IdentityTag, want[i])
		}
		if !strings.HasPrefix(n.Tag, "AL:") {
			t.Errorf("тег #%d = %q — префикс не применился", i+1, n.Tag)
		}
	}
}

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

// Уникализация идентичностей ведётся своим счётчиком на источник, а не общим
// tagCounts конфига: идентичность уникальна В ПРЕДЕЛАХ источника.
func TestIdentityCounterIsPerSource(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#🇳🇱 NL"

	// Общий tagCounts на два источника — как в боевом пайплайне.
	tagCounts := map[string]int{}
	first := loadFromInlineBodyWithCounts(t, uri, configtypes.ProxySource{}, tagCounts)
	second := loadFromInlineBodyWithCounts(t, uri, configtypes.ProxySource{}, tagCounts)

	if first.Nodes[0].IdentityTag != "🇳🇱 NL" || second.Nodes[0].IdentityTag != "🇳🇱 NL" {
		t.Fatalf("идентичности разъехались между источниками: %q и %q",
			first.Nodes[0].IdentityTag, second.Nodes[0].IdentityTag)
	}
	// А конфиговые теги обязаны разойтись — они глобальны.
	if first.Nodes[0].Tag == second.Nodes[0].Tag {
		t.Fatalf("конфиговые теги совпали (%q) — глобальная уникализация сломана", first.Nodes[0].Tag)
	}
}
