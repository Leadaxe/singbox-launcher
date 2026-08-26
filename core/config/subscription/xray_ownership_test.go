package subscription

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 §342 / §322 — владение идентичностью и балансировщик.
//
// Боевой случай, из-за которого это написано: провайдер отдаёт один и тот же
// сервер и как «🇩🇪⚡Германия» (элемент из одного узла), и внутри пула
// «🚀Авто | Лучший сервер» (15 узлов с техническими тегами). Наивный дедуп
// «первый выигрывает» стирал половину стран из списка, потому что пул стоит
// в подписке первым.

// realisticXraySubscription повторяет форму боевой подписки: пул с
// балансировщиком впереди и страны следом.
const realisticXraySubscription = `[
  {
    "remarks": "🇪🇺 Авто | Лучший сервер",
    "outbounds": [
      {"protocol":"vless","tag":"proxy-1-1-1-1-direct","settings":{"vnext":[
        {"address":"1.1.1.1","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}},
      {"protocol":"vless","tag":"proxy-2-2-2-2-direct","settings":{"vnext":[
        {"address":"2.2.2.2","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}
    ],
    "routing": {"balancers": [{"tag":"Auto_Balancer","selector":["proxy"],
      "strategy":{"type":"leastLoad","settings":{"expected":7}}}]},
    "burstObservatory": {"pingConfig":{"destination":"http://www.gstatic.com/generate_204","interval":"2m"}}
  },
  {
    "remarks": "🇩🇪 Германия",
    "outbounds": [{"protocol":"vless","tag":"proxy","settings":{"vnext":[
      {"address":"1.1.1.1","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}]
  },
  {
    "remarks": "🇫🇮 Финляндия",
    "outbounds": [{"protocol":"vless","tag":"proxy","settings":{"vnext":[
      {"address":"2.2.2.2","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]}}]
  }
]`

// Право назвать сервер достаётся элементу с наименьшим числом узлов: имя
// страны осмысленнее технического тега из пула.
func TestXrayOwnershipGivesNamesToSpecificElements(t *testing.T) {
	nodes, err := ParseNodesFromXrayJSONArray(realisticXraySubscription, nil)
	if err != nil {
		t.Fatal(err)
	}

	tags := make([]string, 0, len(nodes))
	for _, n := range nodes {
		tags = append(tags, n.Tag)
	}

	// Страны обязаны быть в списке: именно они пропадали.
	wantCountries := []string{"🇩🇪-Германия", "🇫🇮-Финляндия"}
	for _, want := range wantCountries {
		found := false
		for _, tag := range tags {
			if tag == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("страна %q пропала из списка: %v", want, tags)
		}
	}

	// И ни один узел не должен нести технический тег провайдера.
	for _, tag := range tags {
		if len(tag) > 0 && contains(tag, "proxy-") {
			t.Fatalf("технический тег просочился в список: %q (все: %v)", tag, tags)
		}
	}
}

// Элемент с балансировщиком даёт узел-группу, а не N строк с техтегами.
func TestXrayBalancerBecomesGroupNode(t *testing.T) {
	nodes, err := ParseNodesFromXrayJSONArray(realisticXraySubscription, nil)
	if err != nil {
		t.Fatal(err)
	}

	groups := groupNodesOf(nodes)
	if len(groups) != 1 {
		t.Fatalf("ожидался 1 узел-группа, получено %d (%v)", len(groups), tagsOfNodes(nodes))
	}
	g := groups[0]
	if g.Tag != "🇪🇺 Авто | Лучший сервер" {
		t.Fatalf("группа названа %q, ожидалось чистое имя элемента", g.Tag)
	}
	if g.Outbound["type"] != "urltest" {
		t.Fatalf("тип группы = %v, ожидался urltest", g.Outbound["type"])
	}
	if g.Outbound["url"] != "http://www.gstatic.com/generate_204" {
		t.Errorf("url балансировщика не перенесён: %v", g.Outbound["url"])
	}
	if g.Outbound["interval"] != "2m" {
		t.Errorf("interval балансировщика не перенесён: %v", g.Outbound["interval"])
	}
}

// Группа ссылается на ИТОГОВЫЕ теги членов, даже если те уехали к другим
// элементам. Висячая ссылка роняет старт ядра.
func TestXrayBalancerGroupReferencesSurvivingTags(t *testing.T) {
	nodes, err := ParseNodesFromXrayJSONArray(realisticXraySubscription, nil)
	if err != nil {
		t.Fatal(err)
	}

	alive := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		alive[n.Tag] = true
	}

	groups := groupNodesOf(nodes)
	if len(groups) != 1 {
		t.Fatalf("ожидался 1 узел-группа, получено %d", len(groups))
	}

	members := groupMembersOf(groups[0])
	if len(members) != 2 {
		t.Fatalf("состав группы = %v, ожидалось 2 члена", members)
	}
	for _, m := range members {
		if !alive[m] {
			t.Fatalf("группа ссылается на %q — такого узла в конфиге нет (%v)", m, tagsOfNodes(nodes))
		}
	}
	// Члены — это страны, забравшие серверы себе.
	if members[0] != "🇩🇪-Германия" || members[1] != "🇫🇮-Финляндия" {
		t.Fatalf("состав = %v, ожидались итоговые теги стран", members)
	}
}

// Порядок элементов остаётся авторским: пул стоит в подписке первым и
// обязан остаться первым.
func TestXrayOwnershipKeepsAuthorOrder(t *testing.T) {
	nodes, err := ParseNodesFromXrayJSONArray(realisticXraySubscription, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("нет узлов")
	}
	if nodes[0].Scheme != configtypes.SchemeGroup {
		t.Fatalf("первым идёт %q (%s), ожидалась группа пула",
			nodes[0].Tag, nodes[0].Scheme)
	}
}

// SPEC 112: владение серверами больше не зависит от хука идентичности — оно
// считается своим локальным ключом подключения (xrayServerKey) и работает
// всегда. Раньше без хука пул съедал страны.
func TestXrayOwnershipWorksWithoutIdentityHook(t *testing.T) {
	prev := NodeIdentityFunc
	NodeIdentityFunc = nil
	t.Cleanup(func() { NodeIdentityFunc = prev })

	nodes, err := ParseNodesFromXrayJSONArray(realisticXraySubscription, nil)
	if err != nil {
		t.Fatal(err)
	}
	tags := tagsOfNodes(nodes)
	for _, want := range []string{"🇩🇪-Германия", "🇫🇮-Финляндия"} {
		found := false
		for _, tag := range tags {
			if tag == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("страна %q пропала без хука идентичности (теги: %v)", want, tags)
		}
	}
}

// xrayServerKey — ключ разбора, а не идентичность: один адрес под двумя
// именами внутри массива это одна запись, и владение обязано их свести.
func TestXrayServerKeyIsIndependentOfTag(t *testing.T) {
	a := &configtypes.ParsedNode{Tag: "🇩🇪 Германия", Scheme: "vless", Server: "1.1.1.1", Port: 443, UUID: "u1"}
	b := &configtypes.ParsedNode{Tag: "proxy-1-1-1-1-direct", Scheme: "vless", Server: "1.1.1.1", Port: 443, UUID: "u1"}
	if xrayServerKey(a) != xrayServerKey(b) {
		t.Fatalf("один адрес под двумя именами дал разные ключи: %q и %q",
			xrayServerKey(a), xrayServerKey(b))
	}
	other := &configtypes.ParsedNode{Tag: "🇩🇪 Германия", Scheme: "vless", Server: "2.2.2.2", Port: 443, UUID: "u1"}
	if xrayServerKey(a) == xrayServerKey(other) {
		t.Fatal("разные адреса дали один ключ")
	}
	if got := xrayServerKey(&configtypes.ParsedNode{Tag: "auto", Scheme: configtypes.SchemeGroup}); got != "" {
		t.Fatalf("узел-группа получил серверный ключ %q", got)
	}
}

func tagsOfNodes(nodes []*configtypes.ParsedNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Tag)
	}
	return out
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Регрессия: с tag_prefix состав узла-группы обязан ссылаться на ИТОГОВЫЕ
// теги членов.
//
// Боевой баг: узлы и сама группа получали префикс «AL:», а состав оставался
// на исходных тегах — группа указывала в пустоту. `sing-box check` этого не
// ловит (существование членов он не проверяет), но в рантайме такая группа
// мертва: ядру некуда балансировать.
func TestXrayGroupMembersSurviveTagPrefix(t *testing.T) {
	res := loadFromInlineBody(t, realisticXraySubscription, configtypes.ProxySource{
		TagPrefix: "AL:",
	})

	alive := make(map[string]bool, len(res.Nodes))
	for _, n := range res.Nodes {
		alive[n.Tag] = true
	}

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("ожидался 1 узел-группа, получено %d (%v)", len(groups), tagsOfNodes(res.Nodes))
	}

	members := groupMembersOf(groups[0])
	if len(members) == 0 {
		t.Fatal("группа осталась без членов после применения префикса")
	}
	for _, m := range members {
		if !alive[m] {
			t.Fatalf("группа %q ссылается на %q — такого узла нет (узлы: %v)",
				groups[0].Tag, m, tagsOfNodes(res.Nodes))
		}
	}
}
