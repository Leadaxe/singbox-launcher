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
		SkippedNaiveNodes:  3,
		SkippedNaiveReason: "ядро собрано без with_naive_outbound",
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
	if got := byKind[config.BuildReportNaiveDegraded].NodeCount; got != 3 {
		t.Errorf("деградация naive насчитала %d узлов, ожидалось 3", got)
	}
	if got := byKind[config.BuildReportNaiveDegraded].Reason; got == "" {
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

// Последний рубеж: одна потеря даёт ДВЕ записи — что потерял пользователь
// (nodes_dropped, по ней раскрашивается строка Sources) и чего не хватило в
// сборке (target_missing, субъект = сама цель, её и чинить).
func TestFeedBuildReportFromSanitizer(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()

	FeedBuildReportFromSanitizer(gen, []build.SourceExclusion{{
		SourceID:      "01BIG",
		SourceLabel:   "Big Sub",
		Reason:        "цель detour «vpn ②» не существует в конфиге",
		DroppedNodes:  500,
		MissingTarget: "vpn ②",
	}})

	if n, reason := config.DroppedNodesForSource("01BIG"); n != 500 || reason == "" {
		t.Errorf("снято %d узлов (причина %q), ожидалось 500 с причиной", n, reason)
	}

	entries, _, _ := config.BuildReport()
	var missing *config.BuildReportEntry
	for i := range entries {
		if entries[i].Kind == config.BuildReportTargetMissing {
			missing = &entries[i]
		}
	}
	if missing == nil {
		t.Fatal("пропавшая цель detour не попала в отчёт отдельной записью")
	}
	if missing.Subject != "vpn ②" {
		t.Errorf("субъект записи о пропавшей цели = %q, ожидался сам тег цели", missing.Subject)
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
