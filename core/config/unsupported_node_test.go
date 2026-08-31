// File unsupported_node_test.go — неразобранная запись тела (SPEC 116 W11).
//
// Проверяется ровно то, что молча сломать легче всего и дороже всего:
// эмиссионный инвариант. Unsupported-узел обязан выпадать ДО тег-машины и не
// потреблять ни переменную номера `{$num}`, ни слот глобальной уникализации —
// иначе появление одной битой строки у провайдера сдвинуло бы финальные теги
// всех соседей, а с ними протухли бы выборы в кэше ядра и все ссылки,
// адресующие финальный тег.
package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// subWithNodes — подписка-контейнер с тег-политикой и готовым составом.
func subWithNodes(prefix string, nodes ...state.Node) state.Source {
	src := state.NewSubscriptionSource("P", "https://example/sub")
	src.ID = "SUB1"
	src.TagPolicy = &state.TagPolicy{Prefix: prefix}
	src.Nodes = nodes
	return src
}

func serverNode(rawTag, server string) state.Node {
	return serverNodeLabelled(rawTag, rawTag, server)
}

// serverNodeLabelled — узел, у которого СЫРОЙ тег и провайдерская подпись
// (фрагмент URI) различаются: подпись — вход тег-политики, сырой тег —
// идентичность, и эмиссия читает именно подпись.
func serverNodeLabelled(rawTag, label, server string) state.Node {
	return state.Node{
		Kind:    state.SourceKindServer,
		Tag:     rawTag,
		Enabled: true,
		Origin:  &state.Origin{Kind: state.OriginKindURI, Raw: "trojan://pw@" + server + ":443#" + label},
		Body:    json.RawMessage(`{"type":"trojan","server":"` + server + `","server_port":443,"password":"pw"}`),
	}
}

// Эмиссия с неразобранной записью посередине обязана дать РОВНО те же
// финальные теги, что и без неё: и номера {$num}, и слот уникализации
// принадлежат только собравшимся узлам.
func TestUnsupportedNodeDoesNotShiftEmittedTags(t *testing.T) {
	clean := subWithNodes("[{$num}] ",
		serverNode("A", "a.example"),
		serverNode("A", "b.example"), // тёзка: слот уникализации даст «-2»
	)
	// Та же подписка, но провайдер вставил битую строку между узлами.
	dirty := subWithNodes("[{$num}] ",
		serverNode("A", "a.example"),
		state.NewUnsupportedNode("junk", "record rejected: unknown scheme",
			state.OriginKindURI, "wtf://not-a-node"),
		serverNode("A", "b.example"),
	)

	want := emitTags(t, clean)
	got := emitTags(t, dirty)

	if len(want) != 2 {
		t.Fatalf("эталонная эмиссия дала %d узлов, ожидали 2: %v", len(want), want)
	}
	if strings.Join(want, "|") != strings.Join(got, "|") {
		t.Fatalf("неразобранная запись сдвинула финальные теги соседей:\n  без неё: %v\n  с ней:   %v\n"+
			"это ломает выборы в кэше ядра и все ссылки на финальный тег", want, got)
	}
	// И заодно поимённо: номер {$num} второго узла — 2, а не 3. Пропусти
	// битую строку через тег-машину — и он стал бы третьим.
	if got[1] != "[2] A" {
		t.Errorf("второй узел получил %q, ожидали \"[2] A\" — {$num} считает собравшихся", got[1])
	}
}

// Слот глобальной уникализации тоже принадлежит только собравшимся: тёзки
// получают «-2» подряд, независимо от битых строк между ними.
func TestUnsupportedNodeDoesNotEatUniquenessSlot(t *testing.T) {
	// Сырые теги уникальны (их уникализировал парсер тела), а вот подпись во
	// фрагменте у провайдера одна и та же — так рождается «A» и «A-2» на
	// эмиссии. Между ними стоит битая строка с тегом «A-2»: займи она слот,
	// второй узел уехал бы в «A-3».
	src := subWithNodes("",
		serverNodeLabelled("A", "A", "a.example"),
		state.NewUnsupportedNode("A-2", "record rejected", state.OriginKindURI, "wtf://x"),
		serverNodeLabelled("A-3", "A", "b.example"),
	)
	tags := emitTags(t, src)
	if len(tags) != 2 || tags[0] != "A" || tags[1] != "A-2" {
		t.Fatalf("финальные теги = %v, ожидали [A A-2]: неразобранная запись слот уникализации не занимает", tags)
	}
}

// Неразобранная запись не эмитится вовсе — ни узлом, ни выключенным узлом.
func TestUnsupportedNodeIsNotEmitted(t *testing.T) {
	src := subWithNodes("",
		state.NewUnsupportedNode("junk", "record rejected", state.OriginKindURI, "wtf://x"),
		serverNode("A", "a.example"),
	)
	tags := emitTags(t, src)
	if len(tags) != 1 || tags[0] != "A" {
		t.Fatalf("эмиссия дала %v, ожидали ровно [A]: неразобранная запись в конфиг не едет", tags)
	}
}

// Форма узла: включённой неразобранная запись не бывает, тела/маршрута/состава
// у неё нет — Load обязан это чинить, а не пропускать.
func TestUnsupportedNodeShapeNormalized(t *testing.T) {
	src := state.NewSubscriptionSource("P", "https://example/sub")
	src.Nodes = []state.Node{{
		Kind:    state.SourceKindUnsupported,
		Tag:     "junk",
		Enabled: true, // нелегально: включать нечего
		Origin:  &state.Origin{Kind: state.OriginKindURI, Raw: "wtf://x"},
		Body:    json.RawMessage(`{"type":"trojan"}`),
		Detour:  &state.NodeLink{Tag: "other"},
		Reason:  "record rejected",
	}}
	s := state.New()
	s.Sources = []state.Source{src}
	data, err := s.MarshalV7()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	loaded, err := state.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n := loaded.Sources[0].Nodes[0]
	if n.Enabled {
		t.Error("неразобранная запись загрузилась включённой — включать в ней нечего")
	}
	if len(n.Body) > 0 || n.Detour != nil {
		t.Errorf("нелегальные поля пережили нормализацию: body=%d detour=%v", len(n.Body), n.Detour)
	}
	if n.Reason == "" {
		t.Error("причина потеряна — без неё строка ⚠ не объясняет ничего")
	}
	if n.Origin == nil || n.Origin.Raw == "" {
		t.Error("исходник потерян — запись нечем узнать и нечем починить")
	}
}

// Починенная у провайдера запись оживает ОБЫЧНЫМ узлом на своём месте — и
// включённой: `enabled=false` неразобранной записи это не выбор пользователя.
func TestMergeRevivesFixedRecordEnabled(t *testing.T) {
	sub := state.NewSubscriptionSource("P", "https://example/sub")
	sub.Nodes = []state.Node{
		serverNode("A", "a.example"),
		state.NewUnsupportedNode("B", "record rejected", state.OriginKindURI, "wtf://b"),
	}
	fresh := &state.SubFetchMaterial{Nodes: []state.Node{
		serverNode("A", "a.example"),
		serverNode("B", "b.example"), // провайдер починил запись
	}}

	changed, _ := state.MergeSubscriptionNodes(&sub, fresh, true)
	if !changed {
		t.Fatal("merge не заметил, что запись починилась")
	}
	if len(sub.Nodes) != 2 {
		t.Fatalf("состав = %d узлов, ожидали 2", len(sub.Nodes))
	}
	revived := sub.Nodes[1]
	if revived.Kind != state.SourceKindServer {
		t.Fatalf("узел ожил как %q, ожидали server", revived.Kind)
	}
	if !revived.Enabled {
		t.Error("ожившая запись выключена: `enabled=false` неразобранной записи — не выбор пользователя, переносить его нельзя")
	}
}

// Обратный случай: узел сломался у провайдера. Пользовательская отметка
// «включён» на неразобранную запись не переезжает — включать в ней нечего.
func TestMergeBrokenNodeBecomesUnsupportedDisabled(t *testing.T) {
	sub := state.NewSubscriptionSource("P", "https://example/sub")
	sub.Nodes = []state.Node{serverNode("A", "a.example")}
	fresh := &state.SubFetchMaterial{Nodes: []state.Node{
		state.NewUnsupportedNode("A", "record rejected", state.OriginKindURI, "wtf://a"),
	}}

	if _, _ = state.MergeSubscriptionNodes(&sub, fresh, true); len(sub.Nodes) != 1 {
		t.Fatalf("состав = %d узлов, ожидали 1", len(sub.Nodes))
	}
	if sub.Nodes[0].Enabled {
		t.Error("сломавшаяся запись осталась включённой")
	}
	if !sub.Nodes[0].IsUnsupported() {
		t.Errorf("узел не стал неразобранной записью: kind=%q", sub.Nodes[0].Kind)
	}
}

// Разбор тела: битая строка не исчезает, а встаёт узлом kind=unsupported на
// СВОЁЙ позиции — со своим исходником и без права на номер {$num}.
func TestMaterializeKeepsRejectedRecordInPlace(t *testing.T) {
	body := []byte(strings.Join([]string{
		"trojan://pw@a.example:443#A",
		"wtf://definitely-not-a-node",
		"trojan://pw@b.example:443#B",
	}, "\n"))

	mat, err := MaterializeSubscriptionBody("SUB1", body, nil, 0, false)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(mat.Nodes) != 3 {
		t.Fatalf("узлов = %d, ожидали 3 (два собравшихся + одна неразобранная): %+v", len(mat.Nodes), mat.Nodes)
	}
	if mat.Supported != 2 {
		t.Errorf("Supported = %d, ожидали 2 — достоверность ответа считается по собравшимся", mat.Supported)
	}
	mid := mat.Nodes[1]
	if !mid.IsUnsupported() {
		t.Fatalf("на позиции битой строки стоит %q, ожидали unsupported", mid.Kind)
	}
	if mid.Origin == nil || mid.Origin.Raw != "wtf://definitely-not-a-node" {
		t.Errorf("исходник записи потерян: %+v", mid.Origin)
	}
	if mid.Reason == "" {
		t.Error("причина отбраковки потеряна")
	}
	if mid.Enabled {
		t.Error("неразобранная запись приехала включённой")
	}
	if mat.Nodes[0].Tag != "A" || mat.Nodes[2].Tag != "B" {
		t.Errorf("сырые теги соседей поехали: %q / %q", mat.Nodes[0].Tag, mat.Nodes[2].Tag)
	}
}

// Xray-тело: неподдержанный протокол ПОСЕРЕДИНЕ обязан встать узлом
// kind=unsupported на своей позиции — с исходником и причиной про протокол.
//
// Дефект W13: Xray-ветка выбрасывала такую запись внутри себя одним WARN'ом
// («unsupported protocol "hysteria" skipped») и наверх не отдавала ничего —
// материализация строит unsupported-узлы только из отбраковок общего потока,
// поэтому на Xray-телах они не появлялись вовсе. URI-ветка это делала с W11
// (см. TestMaterializeKeepsRejectedRecordInPlace) — тела разных форматов
// обязаны вести себя одинаково.
func TestMaterializeXrayBodyKeepsUnsupportedProtocolInPlace(t *testing.T) {
	body := []byte(`[
	  {"remarks":"A","outbounds":[{"protocol":"vless","tag":"proxy",
	    "settings":{"vnext":[{"address":"a.example","port":443,
	      "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","encryption":"none"}]}]},
	    "streamSettings":{"network":"tcp","security":"tls"}}]},
	  {"remarks":"BROKEN","outbounds":[{"protocol":"wireguard","tag":"legacy",
	    "settings":{"peers":[{"endpoint":"h.example:443","publicKey":"pw"}]}}]},
	  {"remarks":"B","outbounds":[{"protocol":"vless","tag":"proxy",
	    "settings":{"vnext":[{"address":"b.example","port":443,
	      "users":[{"id":"11111111-2222-3333-4444-555555555555","encryption":"none"}]}]},
	    "streamSettings":{"network":"tcp","security":"tls"}}]}
	]`)

	mat, err := MaterializeSubscriptionBody("SUB1", body, nil, 0, false)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(mat.Nodes) != 3 {
		t.Fatalf("узлов = %d, ожидали 3 (два собравшихся + одна неразобранная): %+v",
			len(mat.Nodes), mat.Nodes)
	}
	if mat.Supported != 2 {
		t.Errorf("Supported = %d, ожидали 2 — достоверность ответа считается по собравшимся", mat.Supported)
	}

	// Позиция: неразобранная запись стоит ВТОРОЙ — ровно там, где её прислал
	// провайдер, между двумя собравшимися узлами.
	if mat.Nodes[0].Kind != state.SourceKindServer || mat.Nodes[2].Kind != state.SourceKindServer {
		t.Fatalf("соседи неразобранной записи не собрались: %q / %q",
			mat.Nodes[0].Kind, mat.Nodes[2].Kind)
	}
	mid := mat.Nodes[1]
	if !mid.IsUnsupported() {
		t.Fatalf("на позиции неподдержанного протокола стоит %q, ожидали unsupported", mid.Kind)
	}
	if !strings.Contains(mid.Reason, "wireguard") {
		t.Errorf("причина = %q, ожидали упоминание протокола: пользователь обязан понять,\n"+
			"что запись отбракована именно из-за протокола, а не «сломалась вообще»", mid.Reason)
	}
	if mid.Origin == nil || !strings.Contains(mid.Origin.Raw, "h.example") {
		t.Errorf("исходник записи потерян: %+v — без него запись нечем узнать", mid.Origin)
	}
	if mid.Enabled {
		t.Error("неразобранная запись приехала включённой — собирать из неё нечего")
	}
	if mid.Tag == "" {
		t.Error("у неразобранной записи нет тега — он ей нужен как идентичность (merge-ключ)")
	}
}

// Анонс провайдера («Лучшие сервера») — запись состава, а не сервер и не
// сломанный узел (SPEC 116 W13).
//
// До W13 такая строка получала общий диагноз «unsupported scheme»: пользователь
// читал у собственного оглавления причину про схему, которой там нет и не
// подразумевалось. Проверяется всё, на чём держится модель: своя причина,
// тег = подпись баннера (он же merge-ключ), исходник байт в байт и позиция
// между узлами.
func TestMaterializeProviderBannerBecomesUnsupported(t *testing.T) {
	body := []byte(strings.Join([]string{
		"Лучшие сервера",
		"trojan://pw@a.example:443#A",
		"ПОДХОДЯТ ДЛЯ ИГР",
		"trojan://pw@b.example:443#B",
	}, "\n"))

	mat, err := MaterializeSubscriptionBody("SUB1", body, nil, 0, false)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(mat.Nodes) != 4 {
		t.Fatalf("узлов = %d, ожидали 4 (два баннера + два сервера): %+v", len(mat.Nodes), mat.Nodes)
	}
	if mat.Supported != 2 {
		t.Errorf("Supported = %d, ожидали 2 — баннеры серверами не считаются", mat.Supported)
	}

	// Позиция: баннер стоит там, где его прислал провайдер, — он и есть
	// заголовок следующей за ним группы узлов.
	if mat.Nodes[1].Tag != "A" || mat.Nodes[3].Tag != "B" {
		t.Fatalf("баннер сдвинул соседей: %q / %q", mat.Nodes[1].Tag, mat.Nodes[3].Tag)
	}

	for i, want := range map[int]string{0: "Лучшие сервера", 2: "ПОДХОДЯТ ДЛЯ ИГР"} {
		n := mat.Nodes[i]
		if !n.IsUnsupported() {
			t.Errorf("на позиции %d стоит %q, ожидали unsupported", i, n.Kind)
			continue
		}
		if n.Tag != want {
			t.Errorf("тег баннера = %q, ожидали %q — тег баннера это его подпись,\n"+
				"иначе при повторном fetch тот же баннер не сматчится и удвоится", n.Tag, want)
		}
		if n.Reason != subscription.RejectReasonProviderBanner {
			t.Errorf("причина = %q, ожидали %q — «unsupported scheme» у строки,\n"+
				"которая узлом не притворялась, объясняет не то", n.Reason, subscription.RejectReasonProviderBanner)
		}
		if n.Origin == nil || n.Origin.Raw != want {
			t.Errorf("исходник баннера = %+v, ожидали %q как есть", n.Origin, want)
		}
	}
}

// Тот же баннер при повторном fetch обязан сматчиться по сырому тегу и НЕ
// удвоиться: на этом и держится решение «тег = подпись», а не позиционный.
func TestMergeKeepsProviderBannerStable(t *testing.T) {
	body := []byte(strings.Join([]string{
		"Лучшие сервера",
		"trojan://pw@a.example:443#A",
	}, "\n"))

	first, err := MaterializeSubscriptionBody("SUB1", body, nil, 0, false)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	sub := state.NewSubscriptionSource("P", "https://example/sub")
	sub.Nodes = append([]state.Node(nil), first.Nodes...)

	// Второй fetch того же тела — состав обязан остаться прежним.
	second, err := MaterializeSubscriptionBody("SUB1", body, nil, 0, false)
	if err != nil {
		t.Fatalf("materialize #2: %v", err)
	}
	state.MergeSubscriptionNodes(&sub, &state.SubFetchMaterial{Nodes: second.Nodes}, true)

	if len(sub.Nodes) != 2 {
		tags := make([]string, 0, len(sub.Nodes))
		for _, n := range sub.Nodes {
			tags = append(tags, n.Tag)
		}
		t.Fatalf("после повторного fetch состав = %v, ожидали 2 узла: баннер размножился —\n"+
			"значит его тег не сматчился сам с собой", tags)
	}
	if sub.Nodes[0].Tag != "Лучшие сервера" {
		t.Errorf("тег баннера поехал на %q", sub.Nodes[0].Tag)
	}
}

// Два одинаковых баннера в одном теле разводятся ТОЙ ЖЕ машиной уникализации
// (`X`, `X-2`), что и два одноимённых сервера: своё второе правило дало бы им
// разные схемы именования и сломало бы матч второму.
func TestMaterializeUniquifiesDuplicateBannerTags(t *testing.T) {
	body := []byte(strings.Join([]string{
		"Лучшие сервера",
		"trojan://pw@a.example:443#A",
		"Лучшие сервера",
	}, "\n"))

	mat, err := MaterializeSubscriptionBody("SUB1", body, nil, 0, false)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(mat.Nodes) != 3 {
		t.Fatalf("узлов = %d, ожидали 3", len(mat.Nodes))
	}
	if mat.Nodes[0].Tag != "Лучшие сервера" || mat.Nodes[2].Tag != "Лучшие сервера-2" {
		t.Errorf("теги баннеров = %q / %q, ожидали «Лучшие сервера» и «Лучшие сервера-2»:\n"+
			"уникализация обязана быть общей машиной контейнера, а не своей",
			mat.Nodes[0].Tag, mat.Nodes[2].Tag)
	}
}

// Элемент Xray-тела со своим полем `tag` обязан отдать это имя неразобранной
// записи (SPEC 116 W13): позиционный `unsupported-N` — только для записи, у
// которой имени нет вовсе.
func TestMaterializeXrayUnsupportedTakesTagFromRecord(t *testing.T) {
	body := []byte(`[
	  {"remarks":"A","outbounds":[{"protocol":"vless","tag":"proxy",
	    "settings":{"vnext":[{"address":"a.example","port":443,
	      "users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","encryption":"none"}]}]},
	    "streamSettings":{"network":"tcp","security":"tls"}}]},
	  {"remarks":"BROKEN","outbounds":[{"protocol":"wireguard","tag":"Токио · игры",
	    "settings":{"peers":[{"endpoint":"h.example:443","publicKey":"pw"}]}}]}
	]`)

	mat, err := MaterializeSubscriptionBody("SUB1", body, nil, 0, false)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(mat.Nodes) != 2 {
		t.Fatalf("узлов = %d, ожидали 2: %+v", len(mat.Nodes), mat.Nodes)
	}
	bad := mat.Nodes[1]
	if !bad.IsUnsupported() {
		t.Fatalf("второй узел = %q, ожидали unsupported", bad.Kind)
	}
	if bad.Tag != "Токио · игры" {
		t.Errorf("тег = %q, ожидали «Токио · игры» — имя берётся ИЗ ЗАПИСИ (поле tag),\n"+
			"позиционный unsupported-N остаётся только для безымянных", bad.Tag)
	}
}

// emitTags — финальные теги узлов источника, как их видит сборка.
func emitTags(t *testing.T, src state.Source) []string {
	t.Helper()
	res := EmitCanonicalSource(src.ToProxySourceV4(), 0, map[string]int{})
	out := make([]string, 0, len(res.Nodes))
	for _, n := range res.Nodes {
		out = append(out, n.Tag)
	}
	return out
}
