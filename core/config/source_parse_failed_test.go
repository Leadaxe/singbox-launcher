package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// SPEC 115 — источник, разобравшийся в НОЛЬ узлов, обязан доехать до отчёта
// сборки с человекочитаемой причиной.
//
// До этого место кончалось WARN'ом «source returned zero nodes (counted as
// failed)»: причина отбраковки уходила в DEBUG, а в UI не было ничего — строка
// Sources показывала здоровый источник.

// loaderWithReasons изображает разбор: узлы отдаёт, а причины отбраковки
// сообщает тем же хуком, что и настоящий LoadNodesFromSourceEx.
func loaderWithReasons(
	byIndex map[int][]*ParsedNode,
	reasonsByIndex map[int][]string,
) func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error) {
	return func(ps ProxySource, _ map[string]int, _ func(float64, string), idx, _ int) ([]*ParsedNode, error) {
		if reasons := reasonsByIndex[idx]; len(reasons) > 0 && subscription.RecordParseFailures != nil {
			subscription.RecordParseFailures(ps, reasons)
		}
		return byIndex[idx], nil
	}
}

func twoSourceParserConfig(second ProxySource) *ParserConfig {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{
		{ID: "01OK", Label: "Живая подписка", Source: "https://example.com/ok"},
		second,
	}
	return pc
}

// Источник с нулём узлов даёт запись с ULID и настоящей причиной; здоровый
// сосед записи не даёт.
func TestGenerateOutbounds_ZeroNodeSourceReportsReason(t *testing.T) {
	pc := twoSourceParserConfig(ProxySource{
		ID: "01DEAD", Label: "AL: Liberty", Source: "https://example.com/dead",
	})

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		loaderWithReasons(
			map[int][]*ParsedNode{0: {testSocksNode("live-1")}},
			map[int][]string{1: {
				"vless outbound rejected: empty user id — the server returned a placeholder, subscription may be expired",
			}},
		),
		DirectionBuildOptions{})
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
	if !strings.Contains(got.Reason, "empty user id") {
		t.Errorf("причина = %q, ожидалась настоящая от разбора", got.Reason)
	}
	if strings.Contains(got.Reason, "Живая подписка") {
		t.Errorf("причина здорового соседа приписана мёртвому источнику: %q", got.Reason)
	}
}

// Источник без узлов и БЕЗ причин всё равно виден: молчание тут читалось бы
// как «источник в порядке».
func TestGenerateOutbounds_ZeroNodeSourceWithoutReasons(t *testing.T) {
	pc := twoSourceParserConfig(ProxySource{
		ID: "01EMPTY", Label: "Всё отфильтровано", Source: "https://example.com/empty",
	})

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		loaderWithReasons(map[int][]*ParsedNode{0: {testSocksNode("live-1")}}, nil),
		DirectionBuildOptions{})
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
	pc := twoSourceParserConfig(ProxySource{
		ID: "01ALSOOK", Label: "Вторая", Source: "https://example.com/ok2",
	})

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		loaderWithReasons(map[int][]*ParsedNode{
			0: {testSocksNode("a-1")},
			1: {testSocksNode("b-1")},
		}, nil),
		DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}
	if len(res.ParseFailedSources) != 0 {
		t.Fatalf("здоровая сборка дала записи о пустых источниках: %+v", res.ParseFailedSources)
	}
}

// Сообщение провайдера (заголовок announce) идёт ПЕРВОЙ причиной: он
// объясняет, почему тело такое, а наши причины — что мы в этом теле увидели.
func TestGenerateOutbounds_ProviderAnnounceLeadsTheReason(t *testing.T) {
	pc := twoSourceParserConfig(ProxySource{
		ID:               "01DEAD",
		Label:            "AL: Liberty",
		Source:           "https://example.com/dead",
		ProviderAnnounce: "⚠️ Произошла ошибка при получении подписки.",
	})

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		loaderWithReasons(
			map[int][]*ParsedNode{0: {testSocksNode("live-1")}},
			map[int][]string{1: {"vless outbound rejected: empty user id"}},
		),
		DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}
	if len(res.ParseFailedSources) != 1 {
		t.Fatalf("записей %d, ожидалась 1", len(res.ParseFailedSources))
	}
	reason := res.ParseFailedSources[0].Reason
	annPos := strings.Index(reason, "Произошла ошибка")
	ourPos := strings.Index(reason, "empty user id")
	if annPos < 0 {
		t.Fatalf("сообщение провайдера не попало в причину: %q", reason)
	}
	if ourPos < 0 {
		t.Fatalf("наша причина потерялась: %q", reason)
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
// причина была (Preview разбирает источник сам), а строка Sources стояла без
// пометки — тот самый парадокс «здоровый на вид сломанный источник», ради
// которого вид записи source_parse_failed и заводили. Все прежние тесты
// проверяли смешанный случай (живой сосед + мёртвый), и дыру не ловили.
func TestGenerateOutbounds_SoleDeadSourceStillReportsReason(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{{
		ID:               "01DEAD",
		Label:            "AL: Liberty VPN",
		Source:           "https://example.com/dead",
		ProviderAnnounce: "Подписка неактивна. Продлите подписку в Боте/WebUI",
	}}

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		loaderWithReasons(nil, map[int][]string{0: {
			"vless outbound rejected: empty user id — the server returned a placeholder, subscription may be expired",
		}}),
		DirectionBuildOptions{})

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
	if !strings.Contains(got.Reason, "empty user id") {
		t.Errorf("наша причина потерялась: %q", got.Reason)
	}
}

// Хук разбора не переживает свою сборку: глобальная переменная, оставшаяся от
// прошлого прогона, приписала бы чужие причины следующему.
func TestGenerateOutbounds_ParseFailureHookRestored(t *testing.T) {
	marker := func(ProxySource, []string) {}
	prev := subscription.RecordParseFailures
	subscription.RecordParseFailures = marker
	t.Cleanup(func() { subscription.RecordParseFailures = prev })

	pc := twoSourceParserConfig(ProxySource{ID: "01B", Source: "https://example.com/b"})
	if _, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil,
		loaderWithReasons(map[int][]*ParsedNode{0: {testSocksNode("a-1")}}, nil),
		DirectionBuildOptions{}); err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}

	if subscription.RecordParseFailures == nil {
		t.Fatal("сборка оставила хук снятым — следующий разбор потеряет причины")
	}
}
