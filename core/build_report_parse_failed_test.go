package core

import (
	"strings"
	"testing"

	"singbox-launcher/core/config"
)

// SPEC 115 — источник, не давший конфигу ни одного узла, обязан быть ВИДЕН.
//
// До этого вида записи такой источник сообщал о себе одним WARN в логе
// («source returned zero nodes (counted as failed)»), а в UI — ничем: строка
// Sources показывала галку и счётчик узлов от прошлой удачной сборки.

func TestFeedBuildReportSourceParseFailed(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()

	FeedBuildReportFromParser(gen, &config.OutboundGenerationResult{
		ParseFailedSources: []config.SourceExclusion{{
			SourceID:    "01SUB",
			SourceLabel: "AL: Liberty",
			Reason:      "vless outbound rejected: empty user id — the server returned a placeholder, subscription may be expired",
		}},
	})

	entries, _, _ := config.BuildReport()
	var found *config.BuildReportEntry
	for i := range entries {
		if entries[i].Kind == config.BuildReportSourceParseFailed {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatal("источник без узлов не попал в отчёт — в UI снова не будет ничего")
	}
	if found.SourceID != "01SUB" {
		t.Errorf("запись привязана к %q, ожидался 01SUB — иначе строку Sources нечем раскрасить", found.SourceID)
	}
	if !strings.Contains(found.Reason, "empty user id") {
		t.Errorf("причина = %q, ожидалась настоящая от разбора", found.Reason)
	}
	if got := config.ParseFailedSourceReason("01SUB"); got == "" {
		t.Error("строка Sources не находит причину по ULID источника")
	}
	if got := config.ParseFailedSourceReason("01OTHER"); got != "" {
		t.Errorf("чужой источник получил причину %q", got)
	}
}

// Чистая сборка записи не даёт: пометка без причины хуже, чем её отсутствие.
func TestFeedBuildReportNoParseFailedOnCleanRun(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()

	FeedBuildReportFromParser(gen, &config.OutboundGenerationResult{})

	if got := config.ParseFailedSourceReason("01SUB"); got != "" {
		t.Fatalf("чистая сборка пометила источник: %q", got)
	}
	entries, _, _ := config.BuildReport()
	for _, e := range entries {
		if e.Kind == config.BuildReportSourceParseFailed {
			t.Fatalf("чистая сборка положила запись source_parse_failed: %+v", e)
		}
	}
}

// Инвалидация общая: правка модели снимает и эту пометку — иначе она пережила
// бы свою причину и врала бы до следующей сборки.
func TestParseFailedReasonClearedOnInvalidate(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)
	gen := config.StartBuildReport()
	FeedBuildReportFromParser(gen, &config.OutboundGenerationResult{
		ParseFailedSources: []config.SourceExclusion{
			{SourceID: "01SUB", SourceLabel: "X", Reason: "пусто"},
		},
	})
	if config.ParseFailedSourceReason("01SUB") == "" {
		t.Fatal("запись не легла в отчёт")
	}

	config.ResetBuildReport()

	if got := config.ParseFailedSourceReason("01SUB"); got != "" {
		t.Fatalf("после инвалидации пометка выжила: %q", got)
	}
}
