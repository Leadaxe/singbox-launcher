package core

import (
	"testing"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
)

// SPEC 115 §2 — итоги обеих стадий конвейера доезжают до отчёта своими видами
// записей. Раньше половина этих причин жила только в логе: цепочка молча не
// собиралась, naive-узлы молча исчезали, снятые последним рубежом узлы
// назывались «исключением источника», хотя источник работал урезанным.

// Парсерная стадия: исключённые источники, несобравшиеся цепочки, naive.
func TestFeedBuildReportFromParser(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()

	FeedBuildReportFromParser(gen, &config.OutboundGenerationResult{
		ExcludedSources: []config.SourceExclusion{
			{SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"},
		},
		BrokenChains: []config.ChainDegradation{
			{Tag: "hop2", Name: "двойной прыжок", Reason: "позиция не найдена"},
		},
		CoreSkips: []config.CoreSkip{
			{Code: "naive_unavailable", Scheme: "naive", Reason: "ядро собрано без with_naive_outbound", Nodes: 3},
		},
	})

	entries, _, _ := config.BuildReport()
	byKind := make(map[config.BuildReportKind]config.BuildReportEntry, len(entries))
	for _, e := range entries {
		byKind[e.Kind] = e
	}

	if got := byKind[config.BuildReportSourceExcluded].SourceID; got != "01SUB" {
		t.Errorf("исключение источника привязано к %q, ожидался 01SUB", got)
	}
	if got := byKind[config.BuildReportChainFailed].Subject; got != "двойной прыжок" {
		t.Errorf("субъект несобравшейся цепочки = %q, ожидалось её имя", got)
	}
	if got := byKind[config.BuildReportCoreUnsupported].NodeCount; got != 3 {
		t.Errorf("деградация naive насчитала %d узлов, ожидалось 3", got)
	}
	if got := byKind[config.BuildReportCoreUnsupported].Reason; got == "" {
		t.Error("деградация naive приехала без причины — пользователь не узнает, чего не хватает ядру")
	}
}

// Чистая парсерная стадия отчёт не наполняет: запись без причины хуже, чем её
// отсутствие, — она приглашает искать поломку там, где её нет.
func TestFeedBuildReportFromParserCleanRun(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()

	FeedBuildReportFromParser(gen, &config.OutboundGenerationResult{})

	if entries, _, _ := config.BuildReport(); len(entries) != 0 {
		t.Fatalf("чистая сборка положила в отчёт %d записей", len(entries))
	}
}

// Последний рубеж снял хоп на недоступную цель (1.1.171): одна запись
// emit_degraded с кодом detour_unavailable на пару источник × цель, адресат —
// источник, узлы не снимаются.
func TestFeedBuildReportFromDetourStripped(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()

	FeedBuildReportFromDetourStripped(gen, []build.DetourStripped{{
		SourceID:    "01BIG",
		SourceLabel: "Big Sub",
		Target:      "vpn ②",
		Nodes:       []string{"a", "b"},
	}})

	if n, _ := config.DroppedNodesForSource("01BIG"); n != 0 {
		t.Errorf("снятый хоп посчитан снятыми узлами: %d", n)
	}
	entries, _, _ := config.BuildReport()
	if len(entries) != 1 {
		t.Fatalf("записей %d, ожидалась одна: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Kind != config.BuildReportEmitDegraded || e.Code != "detour_unavailable" || e.SourceID != "01BIG" {
		t.Errorf("запись = %+v, ждали emit_degraded / detour_unavailable у 01BIG", e)
	}
	if e.Params["target"] != "vpn ②" || e.Params["count"] != "2" || e.Params["tags"] != "a, b" {
		t.Errorf("параметры записи = %v", e.Params)
	}
}

// Кольцо ссылок внятной цели не имеет: запись о пропавшей цели тогда не
// заводится — называть целью «ничего» значило бы отправить пользователя чинить
// несуществующий тег.
func TestFeedBuildReportFromSanitizerWithoutTarget(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()

	FeedBuildReportFromSanitizer(gen, []build.SourceExclusion{{
		SourceID:     "01RING",
		SourceLabel:  "Ring Sub",
		Reason:       "циклическая ссылка",
		DroppedNodes: 2,
	}})

	entries, _, _ := config.BuildReport()
	if len(entries) != 1 {
		t.Fatalf("записей %d, ожидалась одна (без записи о пропавшей цели)", len(entries))
	}
	if entries[0].Kind != config.BuildReportNodesDropped {
		t.Errorf("вид записи = %q, ожидался nodes_dropped", entries[0].Kind)
	}
}
