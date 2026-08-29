package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 115 — источник, из которого не родилось НИ ОДНОГО узла, обязан доехать
// до отчёта сборки с человекочитаемой причиной.
//
// До этого место кончалось WARN'ом «source returned zero nodes (counted as
// failed)»: причина уходила в DEBUG, а в UI не было ничего — строка Sources
// показывала здоровый источник.
//
// SPEC 118 W5: причина приезжает уже не из разбора (конвейер сборки тела не
// парсит — Т5), а из ЭМИССИИ канона: битое тело узла даёт warning эмиссии,
// подписка без узлов — пустой источник. Провайдерское сообщение по-прежнему
// провозится сборочной формой и лидирует в тексте причины.

// canonSourceWithNodes — источник с готовыми узлами канона.
func canonSourceWithNodes(id, label string, bodies map[string]string) ProxySource {
	nodes := make([]configtypes.CanonicalNode, 0, len(bodies))
	for tag, body := range bodies {
		nodes = append(nodes, configtypes.CanonicalNode{
			Kind: "server", Tag: tag, Enabled: true, Body: json.RawMessage(body),
		})
	}
	return ProxySource{
		ID:     id,
		Label:  label,
		Source: "https://example.com/" + id,
		Canonical: &configtypes.CanonicalSource{
			FolderID: id, IsContainer: true, Nodes: nodes,
		},
	}
}

const okBody = `{"type":"socks","server":"10.0.0.1","server_port":1080}`

// Источник с нулём узлов даёт запись с ULID и причиной; здоровый сосед — нет.
func TestGenerateOutbounds_ZeroNodeSourceReportsReason(t *testing.T) {
	dead := canonSourceWithNodes("01DEAD", "AL: Liberty", map[string]string{
		// Тело без `type` не эмитируется — деградация ЗАПИСИ, и источник
		// остаётся без единого узла.
		"broken": `{"server":"10.0.0.2"}`,
	})
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{
		canonSourceWithNodes("01OK", "Живая подписка", map[string]string{"live-1": okBody}),
		dead,
	}

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}

	if len(res.ParseFailedSources) != 1 {
		t.Fatalf("записей о пустых источниках %d, ожидалась 1: %+v",
			len(res.ParseFailedSources), res.ParseFailedSources)
	}
	got := res.ParseFailedSources[0]
	if got.SourceID != "01DEAD" {
		t.Errorf("запись привязана к %q, ожидался 01DEAD", got.SourceID)
	}
	if got.SourceLabel != "AL: Liberty" {
		t.Errorf("подпись источника = %q — по ней пользователь узнаёт строку", got.SourceLabel)
	}
	if strings.TrimSpace(got.Reason) == "" {
		t.Error("запись без причины — пользователь остаётся без ответа")
	}
	if strings.Contains(got.Reason, "Живая подписка") {
		t.Errorf("причина здорового соседа приписана мёртвому источнику: %q", got.Reason)
	}
}

// Источник без узлов и БЕЗ причин всё равно виден: молчание тут читалось бы
// как «источник в порядке».
func TestGenerateOutbounds_ZeroNodeSourceWithoutReasons(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{
		canonSourceWithNodes("01OK", "Живая подписка", map[string]string{"live-1": okBody}),
		canonSourceWithNodes("01EMPTY", "Всё отфильтровано", nil),
	}

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}
	if len(res.ParseFailedSources) != 1 {
		t.Fatalf("записей %d, ожидалась 1", len(res.ParseFailedSources))
	}
	if strings.TrimSpace(res.ParseFailedSources[0].Reason) == "" {
		t.Error("запись без причины — пользователь снова остаётся без ответа")
	}
}

// Все источники живы — записей нет.
func TestGenerateOutbounds_HealthySourcesGiveNoParseFailures(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{
		canonSourceWithNodes("01OK", "Первая", map[string]string{"a-1": okBody}),
		canonSourceWithNodes("01ALSOOK", "Вторая", map[string]string{"b-1": okBody}),
	}

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}
	if len(res.ParseFailedSources) != 0 {
		t.Fatalf("здоровая сборка дала записи о пустых источниках: %+v", res.ParseFailedSources)
	}
}

// Сообщение провайдера (заголовок announce) идёт ПЕРВОЙ причиной: он
// объясняет, почему состав такой, а наши причины — что мы в нём увидели.
func TestGenerateOutbounds_ProviderAnnounceLeadsTheReason(t *testing.T) {
	dead := canonSourceWithNodes("01DEAD", "AL: Liberty", map[string]string{
		"broken": `{"server":"10.0.0.2"}`,
	})
	dead.ProviderAnnounce = "⚠️ Произошла ошибка при получении подписки."

	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{
		canonSourceWithNodes("01OK", "Живая подписка", map[string]string{"live-1": okBody}),
		dead,
	}

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}
	if len(res.ParseFailedSources) != 1 {
		t.Fatalf("записей %d, ожидалась 1", len(res.ParseFailedSources))
	}
	reason := res.ParseFailedSources[0].Reason
	annPos := strings.Index(reason, "Произошла ошибка")
	ourPos := strings.Index(reason, "broken")
	if annPos < 0 {
		t.Fatalf("сообщение провайдера не попало в причину: %q", reason)
	}
	if ourPos < 0 {
		t.Fatalf("эмиссионная причина потерялась: %q", reason)
	}
	if annPos > ourPos {
		t.Errorf("сообщение провайдера идёт после нашей причины: %q", reason)
	}
}

// ЕДИНСТВЕННЫЙ источник, не давший узлов: сборка проваливается — и всё равно
// обязана вернуть причины.
//
// Регрессия из жизни: у пользователя одна подписка, провайдер отвечает
// «Подписка неактивна». Узлов ноль → генератор выходил голым `return nil, err`,
// и уже собранная причина летела на пол вместе с результатом. В окне источника
// причина была, а строка Sources стояла без пометки — тот самый парадокс
// «здоровый на вид сломанный источник».
func TestGenerateOutbounds_SoleDeadSourceStillReportsReason(t *testing.T) {
	dead := canonSourceWithNodes("01DEAD", "AL: Liberty VPN", map[string]string{
		"broken": `{"server":"10.0.0.2"}`,
	})
	dead.ProviderAnnounce = "Подписка неактивна. Продлите подписку в Боте/WebUI"

	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{dead}

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, DirectionBuildOptions{})

	// Ошибка обязана остаться: конфига действительно нет, и вызывающий не
	// вправе принять эту сборку за удачную.
	if err == nil {
		t.Fatal("сборка без единого узла обязана вернуть ошибку")
	}
	// …но диагностика едет ВМЕСТЕ с ней.
	if res == nil {
		t.Fatal("результат выброшен вместе с ошибкой — строку Sources снова нечем раскрасить")
	}
	if len(res.ParseFailedSources) != 1 {
		t.Fatalf("записей о пустых источниках %d, ожидалась 1: %+v",
			len(res.ParseFailedSources), res.ParseFailedSources)
	}
	got := res.ParseFailedSources[0]
	if got.SourceID != "01DEAD" {
		t.Errorf("запись привязана к %q, ожидался 01DEAD", got.SourceID)
	}
	if !strings.Contains(got.Reason, "Подписка неактивна") {
		t.Errorf("сообщение провайдера потерялось: %q", got.Reason)
	}
}
