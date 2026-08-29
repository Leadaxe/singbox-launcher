package subscription

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 A5 — сквозной проход через LoadNodesFromSourceEx.
//
// Отличие от singbox_import_test.go: там проверяется ядро разбора, здесь —
// весь путь источника целиком, включая применение тегов (префикс/маска/
// уникализация), перепривязку состава групп и простановку detour у хопов.
// Именно на этом стыке ломается больше всего: узлы переименованы, а группы
// продолжают ссылаться на исходные теги.

// loadFromInlineBody прогоняет тело через LoadNodesFromSourceEx, подменив
// сетевой фетч кэш-хуком.
func loadFromInlineBody(t *testing.T, body string, ps configtypes.ProxySource) *SourceLoadResult {
	t.Helper()
	return loadFromInlineBodyWithCounts(t, body, ps, map[string]int{})
}

// loadFromInlineBodyWithCounts — то же, но с ЯВНЫМ tagCounts: он общий на весь
// конфиг, и тесты про «идентичность уникальна в пределах источника» обязаны
// прогонять два источника через один счётчик тегов (SPEC 112).
func loadFromInlineBodyWithCounts(
	t *testing.T,
	body string,
	ps configtypes.ProxySource,
	tagCounts map[string]int,
) *SourceLoadResult {
	t.Helper()

	// SPEC 118 W5: подсовывать тело хуком больше нечем — кэш тел умер вместе
	// с raw-файлами. Тело отдаёт локальный стаб: разбор при этом идёт тем же
	// путём, что в бою (скачать → декодировать → классифицировать →
	// распарсить). base64 — потому что провайдеры так и отдают целые
	// sing-box конфиги: голый JSON-объект декодер отвергает как «это не
	// список ссылок», и такой ответ в бою до разбора не доходит.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(body))))
	}))
	t.Cleanup(srv.Close)
	ps.Source = srv.URL

	res, err := LoadNodesFromSourceEx(ps, tagCounts, nil, 0, 1)
	if err != nil {
		t.Fatalf("LoadNodesFromSourceEx() error: %v", err)
	}
	if res == nil {
		t.Fatal("LoadNodesFromSourceEx() returned nil result")
	}
	return res
}

// Группы переживают переименование узлов префиксом: состав переписывается
// на итоговые теги, а не остаётся висеть на исходных.
func TestLoadSourceRebindsGroupsAfterTagPrefix(t *testing.T) {
	res := loadFromInlineBody(t, singboxFullConfigFixture, configtypes.ProxySource{
		TagPrefix: "NL-",
	})

	if len(res.Nodes) == 0 {
		t.Fatal("no nodes loaded")
	}
	for _, node := range res.Nodes {
		if len(node.Tag) < 3 || node.Tag[:3] != "NL-" {
			t.Fatalf("node %q did not receive the tag prefix", node.Tag)
		}
		// Тег в эмитируемой map обязан совпадать с итоговым.
		if got, _ := node.Outbound["tag"].(string); got != node.Tag {
			t.Fatalf("node %q: outbound tag = %q, want %q", node.Tag, got, node.Tag)
		}
	}

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 2 {
		t.Fatalf("got %d group nodes, want 2", len(groups))
	}

	finalTags := map[string]bool{}
	for _, node := range res.Nodes {
		finalTags[node.Tag] = true
	}
	for _, group := range groups {
		members := groupMembersOf(group)
		if len(members) == 0 {
			t.Fatalf("group %q lost all members", group.Tag)
		}
		for _, member := range members {
			if !finalTags[member] {
				t.Fatalf("group %q references %q, which is not an emitted node tag (%v)",
					group.Tag, member, members)
			}
		}
		if def, ok := group.Outbound["default"].(string); ok && !finalTags[def] {
			t.Fatalf("group %q default %q is not an emitted node tag", group.Tag, def)
		}
	}
}

// groupNodesOf отбирает узлы-группы (SchemeGroup) из общего списка.
func groupNodesOf(nodes []*configtypes.ParsedNode) []*configtypes.ParsedNode {
	out := make([]*configtypes.ParsedNode, 0)
	for _, n := range nodes {
		if n != nil && n.Scheme == configtypes.SchemeGroup {
			out = append(out, n)
		}
	}
	return out
}

// groupMembersOf возвращает состав узла-группы.
func groupMembersOf(node *configtypes.ParsedNode) []string {
	if node == nil || node.Outbound == nil {
		return nil
	}
	raw, _ := node.Outbound[configtypes.GroupMembersKey].([]interface{})
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Цепочка после применения тегов остаётся связной: хопы получают уникальные
// теги, узел ссылается на первый хоп, хопы — друг на друга.
func TestLoadSourceKeepsChainConsistentAfterTagging(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"trojan","tag":"main","server":"m.com","server_port":443,"password":"p","detour":"hopA"},
	    {"type":"socks","tag":"hopA","server":"a.com","server_port":1080,"detour":"hopB"},
	    {"type":"socks","tag":"hopB","server":"b.com","server_port":1080}
	  ]
	}`
	res := loadFromInlineBody(t, body, configtypes.ProxySource{TagPrefix: "X-"})

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(res.Nodes))
	}
	node := res.Nodes[0]
	if len(node.Chain) != 2 {
		t.Fatalf("chain length = %d, want 2", len(node.Chain))
	}

	// Узел дозванивается через первый хоп.
	if got, _ := node.Outbound["detour"].(string); got != node.Chain[0].Tag {
		t.Fatalf("node detour = %q, want %q", got, node.Chain[0].Tag)
	}
	// Первый хоп — через второй.
	if got, _ := node.Chain[0].Outbound["detour"].(string); got != node.Chain[1].Tag {
		t.Fatalf("hop[0] detour = %q, want %q", got, node.Chain[1].Tag)
	}
	// Последний хоп идёт напрямую.
	if _, present := node.Chain[1].Outbound["detour"]; present {
		t.Fatal("last hop must not carry a detour")
	}
	// Теги хопов уникальны и записаны в их map.
	if node.Chain[0].Tag == node.Chain[1].Tag {
		t.Fatal("chain hops must have distinct tags")
	}
	for i, hop := range node.Chain {
		if got, _ := hop.Outbound["tag"].(string); got != hop.Tag {
			t.Fatalf("hop[%d]: outbound tag = %q, want %q", i, got, hop.Tag)
		}
	}
	// Deprecated Jump синхронизирован.
	if node.Jump == nil || node.Jump.Tag != node.Chain[0].Tag {
		t.Fatalf("Jump must mirror Chain[0], got %+v", node.Jump)
	}
}

// Обычная URI-подписка не затронута: групп нет, узлы разбираются как раньше.
func TestLoadSourceURIListUnaffected(t *testing.T) {
	body := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#node-one"
	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(res.Nodes))
	}
	if got := len(groupNodesOf(res.Nodes)); got != 0 {
		t.Fatalf("URI list must not produce group nodes, got %d", got)
	}
	if len(res.IgnoredSections) != 0 {
		t.Fatalf("URI list must not report ignored sections, got %v", res.IgnoredSections)
	}
}

// skip-фильтр применяется и к импортированным узлам, и группа теряет
// отфильтрованного члена, оставаясь валидной.
func TestLoadSourceSkipFilterShrinksImportedGroup(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"keep","server":"good.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"drop","server":"bad.com","server_port":443,"uuid":"u2"},
	    {"type":"urltest","tag":"auto","outbounds":["keep","drop"]}
	  ]
	}`
	res := loadFromInlineBody(t, body, configtypes.ProxySource{
		Skip: []map[string]string{{"tag": "drop"}},
	})

	// Всего два узла: обычный "keep" и узел-группа "auto".
	if len(res.Nodes) != 2 {
		t.Fatalf("nodes = %v, want [keep auto]", tagsOf(&SingboxImportResult{Nodes: res.Nodes}))
	}
	if res.Nodes[0].Tag != "keep" {
		t.Fatalf("first node = %q, want keep", res.Nodes[0].Tag)
	}

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("got %d group nodes, want 1", len(groups))
	}
	members := groupMembersOf(groups[0])
	if len(members) != 1 || members[0] != "keep" {
		t.Fatalf("group members = %v, want [keep]", members)
	}
}
