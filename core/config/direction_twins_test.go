package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// --- фикстуры ---------------------------------------------------------

func directionTestConfig(directions ...configtypes.Direction) *ParserConfig {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{{Source: "https://example.com/sub"}}
	pc.ParserConfig.Outbounds = directions
	return pc
}

func directionTestNodes(tags ...string) []*ParsedNode {
	out := make([]*ParsedNode, 0, len(tags))
	for i, tag := range tags {
		out = append(out, &ParsedNode{
			Tag: tag, Scheme: "socks", Server: "10.0.0.1", Port: 1080 + i,
		})
	}
	return out
}

func directionTestOptions() DirectionBuildOptions {
	return DirectionBuildOptions{
		TwinOptions: map[string]interface{}{
			"url":      "http://template.example/generate_204",
			"interval": "5m",
		},
		BlockTag:  "block-out",
		DirectTag: "direct-out",
	}
}

// generateDirections прогоняет направления через весь конвейер и отдаёт
// разобранные группы по тегу. Тест проверяет то же, что увидит ядро.
func generateDirections(t *testing.T, pc *ParserConfig, nodes []*ParsedNode, opts DirectionBuildOptions) map[string]map[string]interface{} {
	t.Helper()
	res, err := GenerateOutboundsFromParserConfig(
		pc, map[string]int{}, nil, naiveDegradeLoadNodes(nodes), opts)
	if err != nil {
		t.Fatalf("генерация: %v", err)
	}
	groups := make(map[string]map[string]interface{}, len(res.OutboundsJSON))
	for _, raw := range res.OutboundsJSON {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(decodeGeneratedEntry(raw)), &m); err != nil {
			continue
		}
		if tag, _ := m["tag"].(string); tag != "" {
			groups[tag] = m
		}
	}
	return groups
}

// decodeGeneratedEntry снимает обёртку генератора: строчный комментарий
// перед объектом и хвостовую запятую (элемент массива config.json).
func decodeGeneratedEntry(raw string) string {
	body := raw
	if i := strings.LastIndex(body, "\n"); i >= 0 {
		body = body[i+1:] // отбрасываем строку комментария
	}
	body = strings.TrimSpace(body)
	return strings.TrimSuffix(body, ",")
}

func members(t *testing.T, group map[string]interface{}) []string {
	t.Helper()
	raw, ok := group["outbounds"].([]interface{})
	if !ok {
		t.Fatalf("у группы нет outbounds: %+v", group)
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

func hasMember(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// --- состав -----------------------------------------------------------

// Направление без фильтра берёт все узлы.
func TestDirectionWithoutFilterTakesAllNodes(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{Tag: "vpn-1", Type: "selector"})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1", "RU-1"), directionTestOptions())

	got := members(t, groups["vpn-1"])
	if len(got) != 3 {
		t.Fatalf("ожидались все узлы, got %v", got)
	}
}

// Фильтр отбирает по итоговому тегу узла, регистр не важен.
func TestDirectionFilterSelectsNodes(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Filters: map[string]interface{}{"tag": configtypes.DirectionFilterPattern("de-", false)},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "DE-2", "NL-1"), directionTestOptions())

	got := members(t, groups["vpn-1"])
	if len(got) != 2 || !hasMember(got, "DE-1") || !hasMember(got, "DE-2") {
		t.Fatalf("фильтр отобрал не то: %v", got)
	}
}

// Инверсия оставляет НЕсовпавшие — так работает исключающий фильтр.
func TestDirectionFilterInvert(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Filters: map[string]interface{}{"tag": configtypes.DirectionFilterPattern("RU", true)},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "RU-1", "NL-1"), directionTestOptions())

	got := members(t, groups["vpn-1"])
	if hasMember(got, "RU-1") {
		t.Fatalf("инверсия не сработала: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

// Фильтр с эмодзи — обычный случай для подписок с флагами стран.
func TestDirectionFilterEmoji(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Filters: map[string]interface{}{"tag": configtypes.DirectionFilterPattern("🇩🇪|🇳🇱", false)},
	})
	groups := generateDirections(t, pc,
		directionTestNodes("🇩🇪 Frankfurt", "🇳🇱 Amsterdam", "🇷🇺 Moscow"), directionTestOptions())

	got := members(t, groups["vpn-1"])
	if len(got) != 2 || hasMember(got, "🇷🇺 Moscow") {
		t.Fatalf("эмодзи-фильтр отобрал не то: %v", got)
	}
}

// Опечатка в регулярке не должна оставлять пользователя без узлов:
// битый ключ отбрасывается, как будто фильтра нет (SPEC 104 §3.5).
func TestDirectionInvalidFilterFallsBackToAllNodes(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Filters: map[string]interface{}{"tag": "/(unclosed/i"},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1"), directionTestOptions())

	got := members(t, groups["vpn-1"])
	if len(got) != 2 {
		t.Fatalf("битая регулярка обеднила направление: %v", got)
	}
}

// --- пустое направление ------------------------------------------------

// Пустое направление получает запасной состав [block, direct] с
// default=block: раньше запись просто выпадала, и трафик правила молча
// уходил в route.final мимо VPN.
func TestEmptyDirectionFallsBackToBlockAndDirect(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Filters: map[string]interface{}{"tag": configtypes.DirectionFilterPattern("НЕТ-ТАКИХ", false)},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1"), directionTestOptions())

	g, ok := groups["vpn-1"]
	if !ok {
		t.Fatalf("пустое направление выпало из конфига, группы: %v", groups)
	}
	got := members(t, g)
	if len(got) != 2 || got[0] != "block-out" || got[1] != "direct-out" {
		t.Fatalf("запасной состав неверен: %v", got)
	}
	if g["default"] != "block-out" {
		t.Fatalf("умолчанием обязана быть блокировка: %v", g["default"])
	}
}

// Без объявленного тега блокировки выдумывать его нельзя — ссылка в никуда
// роняет весь конфиг. Тогда направление выпадает, как раньше.
func TestEmptyDirectionWithoutBlockTagIsSkipped(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Filters: map[string]interface{}{"tag": configtypes.DirectionFilterPattern("НЕТ-ТАКИХ", false)},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1"), DirectionBuildOptions{})

	if _, ok := groups["vpn-1"]; ok {
		t.Fatalf("без тега блокировки запасной состав собирать не из чего")
	}
}

// --- auto-двойник ------------------------------------------------------

// У направления с автовыбором появляется парная urltest-группа, и она же
// становится умолчанием селектора.
func TestDirectionTwinEmittedAndBecomesDefault(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Auto: &configtypes.DirectionAuto{},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1"), directionTestOptions())

	twin, ok := groups["vpn-1-auto"]
	if !ok {
		t.Fatalf("двойник не создан: %v", groups)
	}
	if twin["type"] != "urltest" {
		t.Fatalf("двойник обязан быть urltest: %v", twin["type"])
	}
	if len(members(t, twin)) != 2 {
		t.Fatalf("состав двойника: %v", members(t, twin))
	}

	parent := groups["vpn-1"]
	if parent["default"] != "vpn-1-auto" {
		t.Fatalf("умолчанием обязан стать двойник: %v", parent["default"])
	}
	if got := members(t, parent); got[0] != "vpn-1-auto" {
		t.Fatalf("двойник обязан стоять первым: %v", got)
	}
}

// Явно выбранный узел важнее автовыбора: preferredDefault побеждает.
func TestPreferredDefaultWinsOverTwin(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		PreferredDefault: map[string]interface{}{"tag": configtypes.DirectionFilterPattern("NL", false)},
		Auto:             &configtypes.DirectionAuto{},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1"), directionTestOptions())

	if got := groups["vpn-1"]["default"]; got != "NL-1" {
		t.Fatalf("явный выбор узла должен побеждать: %v", got)
	}
}

// Без Auto двойника нет вовсе — лишняя группа в конфиге не нужна.
func TestNoTwinWithoutAuto(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{Tag: "vpn-1", Type: "selector"})
	groups := generateDirections(t, pc, directionTestNodes("DE-1"), directionTestOptions())

	if _, ok := groups["vpn-1-auto"]; ok {
		t.Fatalf("двойник создан без автовыбора")
	}
	if _, ok := groups["vpn-1"]["default"]; ok {
		t.Fatalf("умолчание выставлено без причины")
	}
}

// Умолчания двойника приходят из шаблона, а заданные поля их перекрывают:
// настройка пользователя важнее, незаданное поле остаётся шаблонным.
func TestTwinOptionsTemplateThenUser(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Auto: &configtypes.DirectionAuto{Interval: "1m", Tolerance: 42},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1"), directionTestOptions())

	twin := groups["vpn-1-auto"]
	if twin["url"] != "http://template.example/generate_204" {
		t.Fatalf("шаблонный url потерян: %v", twin["url"])
	}
	if twin["interval"] != "1m" {
		t.Fatalf("настройка пользователя не перекрыла шаблон: %v", twin["interval"])
	}
	if twin["tolerance"] != float64(42) {
		t.Fatalf("tolerance: %v", twin["tolerance"])
	}
}

// round_robin — тот же urltest плюс mode и balancer; least_test не пишет
// ничего, оставляя конфиг апстримным.
func TestTwinRoundRobinEmitsBalancer(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Auto: &configtypes.DirectionAuto{
			Mode: configtypes.AutoModeRoundRobin, Pool: 3, PoolTolerance: 20,
			StickyHash: []string{"process"},
		},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1"), directionTestOptions())

	twin := groups["vpn-1-auto"]
	if twin["mode"] != configtypes.AutoModeRoundRobin {
		t.Fatalf("режим не записан: %v", twin["mode"])
	}
	bal, ok := twin["balancer"].(map[string]interface{})
	if !ok {
		t.Fatalf("balancer не записан: %v", twin["balancer"])
	}
	if bal["pool"] != float64(3) || bal["pool_tolerance"] != float64(20) {
		t.Fatalf("параметры пула: %v", bal)
	}

	// least_test — ни mode, ни balancer.
	pc2 := directionTestConfig(configtypes.Direction{
		Tag: "vpn-2", Type: "selector",
		Auto: &configtypes.DirectionAuto{Mode: configtypes.AutoModeLeastTest},
	})
	twin2 := generateDirections(t, pc2, directionTestNodes("DE-1"), directionTestOptions())["vpn-2-auto"]
	if _, has := twin2["mode"]; has {
		t.Fatalf("least_test не должен писать mode: %v", twin2)
	}
	if _, has := twin2["balancer"]; has {
		t.Fatalf("least_test не должен писать balancer: %v", twin2)
	}
}

// Пустой набор липкости выключает её через sentinel: пустой список ядро
// схлопывает в дефолт, неотличимо от «поле опущено».
func TestTwinEmptyStickyHashUsesSentinel(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector",
		Auto: &configtypes.DirectionAuto{Mode: configtypes.AutoModeRoundRobin, Pool: 2},
	})
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1"), directionTestOptions())

	bal := groups["vpn-1-auto"]["balancer"].(map[string]interface{})
	sticky, _ := bal["sticky_hash"].([]interface{})
	if len(sticky) != 1 || sticky[0] != "none" {
		t.Fatalf("ожидался sentinel [\"none\"], got %v", sticky)
	}
}

// --- выключение --------------------------------------------------------

// Выключенное направление не материализуется вовсе — вместе со своим
// двойником.
func TestDisabledDirectionSkipped(t *testing.T) {
	pc := directionTestConfig(
		configtypes.Direction{Tag: "vpn-1", Type: "selector"},
		configtypes.Direction{Tag: "vpn-2", Type: "selector", Disabled: true,
			Auto: &configtypes.DirectionAuto{}},
	)
	groups := generateDirections(t, pc, directionTestNodes("DE-1"), directionTestOptions())

	if _, ok := groups["vpn-1"]; !ok {
		t.Fatalf("включённое направление пропало")
	}
	if _, ok := groups["vpn-2"]; ok {
		t.Fatalf("выключенное направление попало в конфиг")
	}
	if _, ok := groups["vpn-2-auto"]; ok {
		t.Fatalf("двойник выключенного направления попал в конфиг")
	}
}

// --- взаимные ссылки ---------------------------------------------------

// Направление можно взять опцией в другое (D-8) — ссылка вниз по списку.
func TestDirectionCanIncludeEarlierDirection(t *testing.T) {
	pc := directionTestConfig(
		configtypes.Direction{Tag: "vpn-1", Type: "selector"},
		configtypes.Direction{Tag: "vpn-2", Type: "selector", AddOutbounds: []string{"vpn-1"}},
	)
	groups := generateDirections(t, pc, directionTestNodes("DE-1", "NL-1"), directionTestOptions())

	if got := members(t, groups["vpn-2"]); !hasMember(got, "vpn-1") {
		t.Fatalf("ссылка на направление выше потеряна: %v", got)
	}
}

// --- предупреждения ----------------------------------------------------

// Пустое направление доезжает до UI списком имён: в конфиге оно молча
// блокирует трафик своих правил, и пользователь обязан узнать.
func TestEmptyDirectionReportedToUI(t *testing.T) {
	pc := directionTestConfig(configtypes.Direction{
		Tag: "vpn-1", Type: "selector", Label: "Моя Германия",
		Filters: map[string]interface{}{"tag": configtypes.DirectionFilterPattern("НЕТ-ТАКИХ", false)},
	})
	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		naiveDegradeLoadNodes(directionTestNodes("DE-1")), directionTestOptions())
	if err != nil {
		t.Fatalf("генерация: %v", err)
	}
	if len(res.EmptyDirections) != 1 || res.EmptyDirections[0] != "Моя Германия" {
		t.Fatalf("предупреждение не доехало до UI: %v", res.EmptyDirections)
	}
}

// Пустой фильтр при нуле узлов — вина подписки, а не настройки: валить это
// на направление значит отправить пользователя чинить не то.
func TestNoWarningWhenFilterIsNotToBlame(t *testing.T) {
	// Узлы есть, фильтра нет — направление непустое, предупреждать не о чем.
	pc := directionTestConfig(configtypes.Direction{Tag: "vpn-1", Type: "selector"})
	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		naiveDegradeLoadNodes(directionTestNodes("DE-1")), directionTestOptions())
	if err != nil {
		t.Fatalf("генерация: %v", err)
	}
	if len(res.EmptyDirections) != 0 {
		t.Fatalf("лишнее предупреждение: %v", res.EmptyDirections)
	}
}
