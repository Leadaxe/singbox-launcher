// File regressions_v154_test.go — data-критичные проверки за ревью diff
// v1.5.3..HEAD: поля, которые молча терялись между разбором тела и конфигом,
// и деградации, которые молча не наступали.
//
// Тесты интеграционные намеренно: каждый дефект жил НЕ внутри одной функции,
// а на стыке (парсер кладёт float64 — эмиттер читает int; проход 2 метит хоп
// — проход 3 сверяет его с чужим пространством имён). Юнит на любой из
// половин был бы зелёным и на сломанном коде.
package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// ── Полоса и порты Hysteria из JSON-тела ─────────────────────────────

// materializedBody — тело узла с данным сырым тегом из разбора JSON-тела.
func materializedBody(t *testing.T, body string, tag string) map[string]interface{} {
	t.Helper()
	mat, err := MaterializeSubscriptionBody("SUB1", []byte(body), nil, 0)
	if err != nil {
		t.Fatalf("MaterializeSubscriptionBody: %v", err)
	}
	for _, n := range mat.Nodes {
		if n.Tag != tag {
			continue
		}
		if len(n.Body) == 0 {
			t.Fatalf("узел %q материализован без тела", tag)
		}
		var out map[string]interface{}
		if err := json.Unmarshal(n.Body, &out); err != nil {
			t.Fatalf("тело узла %q нечитаемо: %v", tag, err)
		}
		return out
	}
	t.Fatalf("узел %q не найден среди %d материализованных", tag, len(mat.Nodes))
	return nil
}

// Полоса и server_ports приезжают из JSON числами (float64) и []interface{},
// а эмиттер читал их жёсткими .(int)/.([]string) — и молча выбрасывал. У
// hysteria v1 это фатально: ядро отвергает outbound без полосы («missing
// upload speed») и роняет ВЕСЬ config.json.
func TestHysteriaV1KeepsBandwidthAndPortsFromJSONBody(t *testing.T) {
	body := `{"outbounds":[{"type":"hysteria","tag":"HY1","server":"h1.test","server_port":443,
	  "auth_str":"secret","up_mbps":50,"down_mbps":100,"server_ports":["1000:2000"]}]}`

	ob := materializedBody(t, body, "HY1")

	if got := ob["up_mbps"]; got != float64(50) {
		t.Errorf("up_mbps = %v, want 50 — без полосы ядро отвергает весь конфиг", got)
	}
	if got := ob["down_mbps"]; got != float64(100) {
		t.Errorf("down_mbps = %v, want 100", got)
	}
	ports, _ := ob["server_ports"].([]interface{})
	if len(ports) != 1 || ports[0] != "1000:2000" {
		t.Errorf("server_ports = %v, want [1000:2000]", ob["server_ports"])
	}
}

// Тот же разбор у hysteria2: полоса необязательна для ядра, но заданную
// провайдером терять нельзя — она ограничивает реальную скорость.
func TestHysteria2KeepsBandwidthAndPortsFromJSONBody(t *testing.T) {
	body := `{"outbounds":[{"type":"hysteria2","tag":"HY2","server":"h2.test","server_port":443,
	  "password":"pw","up_mbps":50,"down_mbps":100,"server_ports":["1000:2000"]}]}`

	ob := materializedBody(t, body, "HY2")

	if got := ob["up_mbps"]; got != float64(50) {
		t.Errorf("up_mbps = %v, want 50", got)
	}
	if got := ob["down_mbps"]; got != float64(100) {
		t.Errorf("down_mbps = %v, want 100", got)
	}
	ports, _ := ob["server_ports"].([]interface{})
	if len(ports) != 1 || ports[0] != "1000:2000" {
		t.Errorf("server_ports = %v, want [1000:2000]", ob["server_ports"])
	}
}

// Дефолт полосы у v1 обязан доезжать до конфига: импорт ставит его float64,
// и эмиттер, читавший int, выбрасывал ровно ту подстраховку, ради которой
// дефолт и заведён.
func TestHysteriaV1DefaultBandwidthReachesConfig(t *testing.T) {
	body := `{"outbounds":[{"type":"hysteria","tag":"HY0","server":"h0.test","server_port":443,
	  "auth_str":"secret"}]}`

	ob := materializedBody(t, body, "HY0")

	up, upOK := ob["up_mbps"].(float64)
	down, downOK := ob["down_mbps"].(float64)
	if !upOK || up <= 0 || !downOK || down <= 0 {
		t.Errorf("полоса по умолчанию не доехала: up=%v down=%v", ob["up_mbps"], ob["down_mbps"])
	}
}

// ── Релеи: миграция, порядок выживших, уникальность тега ─────────────

// xrayBodyWithRelay — Xray-тело, где узел дозванивается через socks-релей.
const xrayBodyWithRelay = `[{"remarks":"prov","outbounds":[
  {"protocol":"vless","tag":"EXIT","settings":{"vnext":[
    {"address":"exit.test","port":443,"users":[{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}]},
   "streamSettings":{"network":"tcp","sockopt":{"dialerProxy":"relay1"}}},
  {"protocol":"socks","tag":"relay1","settings":{"servers":[{"address":"r1.test","port":1080}]}}
]}]`

// Миграция v6→v7 обязана материализовать релеи так же, как fetch. Пока она
// звала только canonicalNodeFromEntry, BYPASS-подписка после апгрейда шла
// НАПРЯМУЮ — то есть в блокировку, — а первый же fetch добавлял релеи как
// «новые узлы».
func TestMigrationMaterializesProviderRelays(t *testing.T) {
	res, err := materializeSubscriptionForMigration(state.MigrationSubRequest{
		SubID:     "SUB1",
		Body:      []byte(xrayBodyWithRelay),
		TagCounts: map[string]int{},
	})
	if err != nil {
		t.Fatalf("materializeSubscriptionForMigration: %v", err)
	}

	var owner, relay *state.Node
	for i := range res.Nodes {
		n := &res.Nodes[i].Node
		if n.Service {
			relay = n
			continue
		}
		if n.Kind == state.SourceKindServer {
			owner = n
		}
	}
	if owner == nil {
		t.Fatal("владелец не материализован")
	}
	if relay == nil {
		t.Fatal("релей провайдера потерян миграцией — подписка пошла бы напрямую, в блокировку")
	}
	if owner.Detour == nil || owner.Detour.Tag != relay.Tag {
		t.Errorf("detour владельца = %v, want ссылку на релей %q", owner.Detour, relay.Tag)
	}
	if len(relay.Body) == 0 {
		t.Error("релей материализован без тела")
	}
	// Финальный тег релея — его собственный: в старой тег-машине служебных
	// узлов не было, старых ссылок на них не существует.
	for _, mat := range res.Nodes {
		if mat.Node.Service && mat.FinalTag != mat.Node.Tag {
			t.Errorf("FinalTag релея = %q, want собственный тег %q", mat.FinalTag, mat.Node.Tag)
		}
	}
}

// Несобравшийся хоп в середине цепочки релеев: теги и Detour обязаны
// раздаваться по фактическому порядку ВЫЖИВШИХ. Пока они считались от
// индекса в исходном Chain, пропуск оставлял у предыдущего релея detour на
// тег, которого в контейнере нет, и fail-closed ронял владельца — ровно то,
// что этот пропуск обещал предотвратить.
func TestRelayChainSkipsUnemittableHopWithoutDanglingDetour(t *testing.T) {
	entry := relayEntryWithHops(t,
		hopNode("hop-a", "socks", "a.test", 1080),
		nil, // середина не собралась
		hopNode("hop-c", "socks", "c.test", 1080),
	)

	idCounts := map[string]int{"OWNER": 1}
	relays, detour := relayNodesFromEntry("SUB1", entry, "OWNER", idCounts)

	if len(relays) != 2 {
		t.Fatalf("выжило %d релеев, want 2", len(relays))
	}
	if detour == nil || detour.Tag != relays[0].Tag {
		t.Fatalf("detour владельца = %v, want первого выжившего %q", detour, relays[0].Tag)
	}

	byTag := map[string]bool{}
	for _, r := range relays {
		byTag[r.Tag] = true
	}
	// Detour первого обязан указывать на СУЩЕСТВУЮЩИЙ тег, последний — пуст.
	if relays[0].Detour == nil || !byTag[relays[0].Detour.Tag] {
		t.Errorf("detour релея[0] = %v — ссылка в никуда роняет владельца fail-closed", relays[0].Detour)
	}
	if relays[1].Detour != nil {
		t.Errorf("последний релей несёт detour %v, want nil (идёт напрямую)", relays[1].Detour)
	}
}

// Тег релея выводится из тега владельца и вполне может столкнуться с
// настоящим именем из тела. Разводить столкновение обязана ОБЩАЯ машина
// уникализации контейнера: на дубле тега ядро отвергает весь outbounds.
func TestRelayTagIsUniquifiedAgainstContainerNames(t *testing.T) {
	entry := relayEntryWithHops(t, hopNode("hop-a", "socks", "a.test", 1080))

	// Имя, которое релей хотел бы занять, уже принадлежит соседу по телу.
	taken := "OWNER · relay"
	idCounts := map[string]int{"OWNER": 1, taken: 1}

	relays, detour := relayNodesFromEntry("SUB1", entry, "OWNER", idCounts)
	if len(relays) != 1 {
		t.Fatalf("собрано %d релеев, want 1", len(relays))
	}
	if relays[0].Tag == taken {
		t.Errorf("тег релея = %q — столкнулся с занятым именем контейнера", relays[0].Tag)
	}
	if detour == nil || detour.Tag != relays[0].Tag {
		t.Errorf("detour владельца = %v, want уникализированный тег %q", detour, relays[0].Tag)
	}
}

// relayEntryWithHops собирает запись тела с заданными хопами; nil-хоп
// изображает тот, чьё тело не эмитится.
func relayEntryWithHops(t *testing.T, hops ...*configtypes.ParsedNode) *subscription.ParsedBodyEntry {
	t.Helper()
	chain := make([]*configtypes.ParsedNode, 0, len(hops))
	for _, h := range hops {
		if h == nil {
			// Хоп, тело которого не собирается: группа без outbound-данных —
			// единственная форма, которую эмиттер честно отвергает (остальные
			// схемы он допечатывает по общему пути).
			chain = append(chain, &configtypes.ParsedNode{Tag: "broken", Scheme: configtypes.SchemeGroup})
			continue
		}
		chain = append(chain, h)
	}
	return &subscription.ParsedBodyEntry{
		RawTag: "OWNER",
		Num:    1,
		Node: &configtypes.ParsedNode{
			Tag:      "OWNER",
			Scheme:   "trojan",
			Server:   "owner.test",
			Port:     443,
			UUID:     "pw",
			Outbound: map[string]interface{}{"type": "trojan"},
			Chain:    chain,
		},
	}
}

// ── Пул кандидатов Направлений: служебные узлы ───────────────────────

// Релей не кандидат Направлению, пока источник не разрешил это галкой.
// Правило раньше жило ТОЛЬКО в пикере формы, а сборка считала состав заново
// по фильтрам — и Направление с фильтром по scheme молча забирало релеи.
func TestDirectionPoolHidesRelaysUnlessSourceAllows(t *testing.T) {
	owner := &ParsedNode{Tag: "OWNER", Scheme: "vless", SourceIndex: 0}
	relay := &ParsedNode{Tag: "OWNER · relay", Scheme: "socks", SourceIndex: 0, Service: true}
	all := []*ParsedNode{owner, relay}

	src := func(allow bool) []ProxySource {
		return []ProxySource{{
			ID:        "SUB1",
			Canonical: &configtypes.CanonicalSource{FolderID: "SUB1", IsContainer: true, RelaysInDirections: allow},
		}}
	}

	pool := FilterDirectionCandidatePool(all, src(false))
	if len(pool) != 1 || pool[0] != owner {
		t.Errorf("без галки в пуле %d узлов (%v), want только владельца", len(pool), poolTags(pool))
	}

	pool = FilterDirectionCandidatePool(all, src(true))
	if len(pool) != 2 {
		t.Errorf("с галкой в пуле %v, want владельца и релей", poolTags(pool))
	}
}

func poolTags(pool []*ParsedNode) []string {
	out := make([]string, 0, len(pool))
	for _, n := range pool {
		out = append(out, n.Tag)
	}
	return out
}

// ── Цепочка: нерезолвнутый хоп не уезжает на однофамильца ────────────

// Хоп {FolderID:"F1", Tag:"US-1"} при ВЫКЛЮЧЕННОЙ папке F1 и корневом узле с
// финальным тегом `US-1` проходил проверку позиций по совпадению имён — и
// цепочка молча собиралась через ЧУЖОЙ сервер. Деградация обязана быть
// fail-closed независимо от совпадений.
func TestChainHopUnresolvedDoesNotFallOnRootNamesake(t *testing.T) {
	// Однофамилец в корне: узел с финальным тегом US-1.
	namesake := canonRoot("S1", "US-1", canonServerNode("US-1", "US-1", "root.example", 443))
	// Законная вторая позиция цепочки.
	exit := canonRoot("S2", "EXIT", canonServerNode("EXIT", "EXIT", "exit.example", 443))

	// Выключенная папка, на узел которой смотрит цепочка.
	folder := canonFolder("F1", "", "", canonServerNode("US-1", "US-1", "folder.example", 443))
	folder.Disabled = true

	chainSrc := ProxySource{
		ID:    "C1",
		Label: "my-chain",
		Canonical: &configtypes.CanonicalSource{
			Nodes: []configtypes.CanonicalNode{{
				Kind:    canonicalKindChain,
				Tag:     "my-chain",
				Enabled: true,
				Body:    json.RawMessage(`{}`),
				Hops: []configtypes.NodeLink{
					// Первая позиция смотрит в ВЫКЛЮЧЕННУЮ папку F1 — она не
					// резолвится. Её сырой тег совпадает с финальным тегом
					// корневого однофамильца, и именно на этом совпадении
					// цепочка раньше собиралась через чужой сервер.
					{FolderID: "F1", Tag: "US-1"},
					// Вторая — законный корневой узел, чтобы цепочка не
					// падала раньше по другой причине (двух позиций ядру
					// хватает, повторов между ними нет).
					{Tag: "EXIT"},
				},
			}},
		},
	}

	res := runCanonicalBuild(t, []ProxySource{namesake, exit, folder, chainSrc}, nil)
	tags := emittedTags(res)

	if hasTag(tags, "my-chain") {
		t.Error("цепочка собралась при нерезолвнутом хопе — маршрут молча уехал через чужой сервер")
	}
	if w := joinWarnings(res); !strings.Contains(w, "US-1") {
		t.Errorf("предупреждение не называет позицию: %q", w)
	}
	// Служебный маркер нерезолвимости пользователю показывать нельзя.
	if w := joinWarnings(res); strings.Contains(w, chainHopUnresolvedMark) {
		t.Errorf("маркер сборки утёк в текст для человека: %q", w)
	}
}

// ── Отчёт сборки: несколько причин у одного источника ────────────────

// У emit_degraded Subject — имя ИСТОЧНИКА, одно на все его деградации. Без
// сравнения причин у источника с тремя выпавшими узлами в отчёте оставалась
// первая, и человек чинил их по одной за пересборку.
func TestBuildReportKeepsSeveralReasonsPerSource(t *testing.T) {
	t.Cleanup(ResetBuildReport)
	gen := StartBuildReport()

	AddBuildReportEntries(gen, []BuildReportEntry{
		{Kind: BuildReportEmitDegraded, Subject: "Sub", SourceID: "01S", SourceLabel: "Sub", Reason: "позиция цепочки не резолвится"},
		{Kind: BuildReportEmitDegraded, Subject: "Sub", SourceID: "01S", SourceLabel: "Sub", Reason: "член группы выпал"},
		// Полный дубль по-прежнему проглатывается.
		{Kind: BuildReportEmitDegraded, Subject: "Sub", SourceID: "01S", SourceLabel: "Sub", Reason: "член группы выпал"},
	})

	entries, _, _ := BuildReport()
	if len(entries) != 2 {
		t.Fatalf("в отчёте %d записей, want 2 (обе причины, дубль проглочен)", len(entries))
	}
}
