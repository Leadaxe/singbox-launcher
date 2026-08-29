package subscription_test

// Каркасные тесты чистого парсера тела подписки (SPEC 118 W2; полный
// поведенческий набор §4.D — волна W3 вместе с merge-слоем).
//
// Внешний пакет: дедуп по подписи содержимого работает через хук эмиттера
// (LegacyNodeIdentityHashFunc / подпись server_conn_key), который подставляет
// init пакета core/config — изнутри пакета subscription его не импортировать
// (цикл), и дедуп в изоляции честно выключен.

import (
	"fmt"
	"strings"
	"testing"

	_ "singbox-launcher/core/config" // хуки подписи/идентичности (init)
	. "singbox-launcher/core/config/subscription"
)

// Регресс v1.5.2: записи, различающиеся только фрагментом-именем, схлопываются
// дедупом по подписи содержимого — 32 ss:// дают один узел.
func TestParseBodyDedupCollapsesFragmentCopies(t *testing.T) {
	var lines []string
	for i := 0; i < 32; i++ {
		lines = append(lines, fmt.Sprintf("ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@srv.example:8388#name-%d", i))
	}
	res, err := ParseSubscriptionBody([]byte(strings.Join(lines, "\n")), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("entries = %d, want 1 (дедуп по подписи, регресс v1.5.2)", len(res.Entries))
	}
}

// Дубли сырых тегов у провайдера — норма: `X, X-2, X` живут как X, X-2, X-3
// (кандидат уникализации проверяется на занятость — SPEC 113-A §5).
func TestParseBodyUniquifiesRawTags(t *testing.T) {
	body := strings.Join([]string{
		"vless://11111111-1111-4111-8111-111111111111@a.example:443?security=tls&sni=a.example#X",
		"vless://22222222-2222-4222-8222-222222222222@b.example:443?security=tls&sni=b.example#X-2",
		"vless://33333333-3333-4333-8333-333333333333@c.example:443?security=tls&sni=c.example#X",
	}, "\n")
	res, err := ParseSubscriptionBody([]byte(body), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, e := range res.Entries {
		tags = append(tags, e.RawTag)
	}
	want := "X,X-2,X-3"
	if got := strings.Join(tags, ","); got != want {
		t.Fatalf("raw tags = %s, want %s", got, want)
	}
}

// Кап — реальный предел разбора: N принятых узлов, остальное не рождается,
// truncated=true.
func TestParseBodyCapTruncates(t *testing.T) {
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf(
			"vless://11111111-1111-4111-8111-11111111111%d@s%d.example:443?security=tls&sni=s%d.example#N-%d", i, i, i, i))
	}
	res, err := ParseSubscriptionBody([]byte(strings.Join(lines, "\n")), nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 3 {
		t.Fatalf("entries = %d, want 3 (кап)", len(res.Entries))
	}
	if !res.Truncated {
		t.Fatal("truncated не выставлен при капе")
	}
}

// skip-фильтр отсекает запись до рождения узла.
func TestParseBodySkipFilters(t *testing.T) {
	body := strings.Join([]string{
		"vless://11111111-1111-4111-8111-111111111111@a.example:443?security=tls&sni=a.example#RU-1",
		"vless://22222222-2222-4222-8222-222222222222@b.example:443?security=tls&sni=b.example#NL-1",
	}, "\n")
	res, err := ParseSubscriptionBody([]byte(body), []map[string]string{{"tag": "/(RU)/i"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.Entries[0].RawTag != "NL-1" {
		t.Fatalf("skip не сработал: %+v", res.Entries)
	}
}

// Тело sing-box-конфига: группы становятся записями с членами по СЫРЫМ
// тегам; selector сохраняет type и default.
func TestParseBodySingboxGroups(t *testing.T) {
	body := `{
	  "outbounds": [
	    {"type": "vless", "tag": "srv-a", "server": "a.example", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "tls": {"enabled": true, "server_name": "a.example"}},
	    {"type": "vless", "tag": "srv-b", "server": "b.example", "server_port": 443, "uuid": "22222222-2222-4222-8222-222222222222", "tls": {"enabled": true, "server_name": "b.example"}},
	    {"type": "selector", "tag": "pick", "outbounds": ["srv-a", "srv-b"], "default": "srv-b"}
	  ]
	}`
	res, err := ParseSubscriptionBody([]byte(body), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var group *ParsedBodyEntry
	for _, e := range res.Entries {
		if e.GroupType != "" {
			group = e
		}
	}
	if group == nil {
		t.Fatalf("группа не материализована: %+v", res.Entries)
	}
	if group.GroupType != "selector" || group.GroupDefaultRaw != "srv-b" {
		t.Fatalf("selector потерял тип/default: %+v", group)
	}
	if strings.Join(group.MemberRawTags, ",") != "srv-a,srv-b" {
		t.Fatalf("члены группы: %v", group.MemberRawTags)
	}
}
