// Риск Р8 (двухфазный отчёт «Итога») + SPEC 118 Т3: деградации разбора тела
// живут в состоянии (`update_status.warnings`), и отчёт сборки читает их
// ОТТУДА — синхронно, без пересборки.
//
// До W4 такие строки рождались parse-стадией сборки: источник с битой записью
// становился виден в «Итоге» только после следующей пересборки. Разбор теперь
// живёт только в fetch, поэтому единственный честный источник — состояние.
package core

import (
	"strings"
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
)

func reportReasonsFromReport(kind config.BuildReportKind) []string {
	entries, _, _ := config.BuildReport()
	return reportReasons(entries, kind)
}

func reportReasons(entries []config.BuildReportEntry, kind config.BuildReportKind) []string {
	var out []string
	for _, e := range entries {
		if e.Kind == kind {
			out = append(out, e.Subject+": "+e.Reason)
		}
	}
	return out
}

func TestFeedBuildReportFromFetchStatus_ReadsWarningsFromState(t *testing.T) {
	sources := []state.Source{
		{
			Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			ID:   "SUB1",
			Name: "Proton NL",
			Nodes: []state.Node{
				{Kind: state.SourceKindServer, Tag: "NL-1", Enabled: true},
			},
			UpdateStatus: &state.SubUpdateStatus{
				LastSuccessAt: "2026-08-01T00:00:00Z",
				Warnings: []state.FetchWarning{
					{Kind: "bad_record", Tag: "DE-9", Message: "record rejected: unsupported scheme"},
					{Kind: "skip", Message: "3 record(s) skipped", Count: 3},
				},
			},
		},
	}

	gen := config.StartBuildReport()
	FeedBuildReportFromFetchStatus(gen, sources)
	entries, _, _ := config.BuildReport()

	reasons := reportReasons(entries, config.BuildReportFetchDegraded)
	if len(reasons) != 2 {
		t.Fatalf("строки деградаций fetch = %v, want 2", reasons)
	}
	joined := strings.Join(reasons, " | ")
	// Адресация (folderId, tag): субъект называет узел, пометка строки
	// Sources остаётся за SourceID.
	if !strings.Contains(joined, "Proton NL → DE-9") {
		t.Errorf("адресация по узлу потеряна: %v", reasons)
	}
	if !strings.Contains(joined, "unsupported scheme") {
		t.Errorf("причина разбора не доехала: %v", reasons)
	}
	for _, e := range entries {
		if e.Kind == config.BuildReportFetchDegraded && e.SourceID != "SUB1" {
			t.Errorf("запись не привязана к источнику: %+v", e)
		}
	}
}

// Подписка, ни разу не сфетченная, — предупреждение отчёта, а не отказ
// сборки (SPEC Т3: `ErrRawCacheIncomplete`-строгость умерла).
func TestFeedBuildReportFromFetchStatus_NeverFetchedIsWarningNotFailure(t *testing.T) {
	sources := []state.Source{
		{
			Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			ID:   "SUB2",
			Name: "Fresh",
			URL:  "https://example.invalid/sub",
		},
	}
	gen := config.StartBuildReport()
	FeedBuildReportFromFetchStatus(gen, sources)

	reasons := reportReasonsFromReport(config.BuildReportFetchDegraded)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "ни разу не обновлялась") {
		t.Fatalf("нефетченная подписка не отражена: %v", reasons)
	}
}

// Выключенная подписка и здоровая — молчат: пометка, пережившая свою
// причину, читается как баг.
func TestFeedBuildReportFromFetchStatus_QuietWhenHealthy(t *testing.T) {
	sources := []state.Source{
		{
			Node:         state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			ID:           "OK",
			Nodes:        []state.Node{{Kind: state.SourceKindServer, Tag: "n", Enabled: true}},
			UpdateStatus: &state.SubUpdateStatus{LastSuccessAt: "2026-08-01T00:00:00Z"},
		},
		{
			Node: state.Node{Kind: state.SourceKindSubscription, Enabled: false},
			ID:   "OFF",
		},
	}
	gen := config.StartBuildReport()
	FeedBuildReportFromFetchStatus(gen, sources)

	if reasons := reportReasonsFromReport(config.BuildReportFetchDegraded); len(reasons) != 0 {
		t.Errorf("здоровая конфигурация оставила пометки: %v", reasons)
	}
}

// Фикс-раунд W4: последний fetch провалился, а конфиг всё-таки собран — из
// узлов прошлого успеха. Раньше такая подписка была не видна в отчёте вовсе:
// warnings прошлого разбора не про это, а «ни разу не обновлялась» не про неё.
// Пользователь смотрел на полный список узлов и считал подписку свежей.
func TestFeedBuildReportFromFetchStatus_LastFetchFailedIsVisible(t *testing.T) {
	sources := []state.Source{
		{
			Node:  state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			ID:    "SUB3",
			Name:  "Stale",
			Nodes: []state.Node{{Kind: state.SourceKindServer, Tag: "NL-1", Enabled: true}},
			UpdateStatus: &state.SubUpdateStatus{
				LastStatus:    "err",
				LastAttemptAt: "2026-08-27T10:00:00Z",
				LastSuccessAt: "2026-08-01T00:00:00Z",
				LastErrorMsg:  "dial tcp: i/o timeout",
			},
		},
	}
	gen := config.StartBuildReport()
	FeedBuildReportFromFetchStatus(gen, sources)
	entries, _, _ := config.BuildReport()

	reasons := reportReasons(entries, config.BuildReportFetchDegraded)
	if len(reasons) != 1 {
		t.Fatalf("провал последнего обновления не отражён ровно одной строкой: %v", reasons)
	}
	// Обе половины смысла: что сломалось и ЧЕМ при этом собран конфиг.
	if !strings.Contains(reasons[0], "i/o timeout") {
		t.Errorf("причина провала не доехала: %v", reasons)
	}
	if !strings.Contains(reasons[0], "2026-08-01T00:00:00Z") {
		t.Errorf("дата узлов, из которых собран конфиг, не названа: %v", reasons)
	}
	for _, e := range entries {
		if e.Kind != config.BuildReportFetchDegraded {
			continue
		}
		if e.SourceID != "SUB3" {
			t.Errorf("запись не привязана к источнику: %+v", e)
		}
		// Пользователю важно, СКОЛЬКО узлов при этом уехало в конфиг.
		if e.NodeCount != 1 {
			t.Errorf("число узлов прошлого успеха потеряно: %+v", e)
		}
	}
}

// Провал БЕЗ единого успеха в истории: строка «ни разу не обновлялась»
// остаётся единственной — дублировать её провалом значило бы сказать одно и то
// же дважды, а дату успеха взять неоткуда.
func TestFeedBuildReportFromFetchStatus_FailedAndNeverSucceededSaysItOnce(t *testing.T) {
	sources := []state.Source{
		{
			Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			ID:   "SUB4",
			Name: "Never",
			UpdateStatus: &state.SubUpdateStatus{
				LastStatus:   "err",
				LastErrorMsg: "404 Not Found",
			},
		},
	}
	gen := config.StartBuildReport()
	FeedBuildReportFromFetchStatus(gen, sources)

	reasons := reportReasonsFromReport(config.BuildReportFetchDegraded)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "ни разу не обновлялась") {
		t.Fatalf("нефетченная подписка описана не одной строкой: %v", reasons)
	}
}

// Успешное последнее обновление молчит и при заполненной истории ошибок:
// error_count — это прошлое, а не деградация текущей сборки.
func TestFeedBuildReportFromFetchStatus_RecoveredSourceIsQuiet(t *testing.T) {
	sources := []state.Source{
		{
			Node:  state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			ID:    "SUB5",
			Nodes: []state.Node{{Kind: state.SourceKindServer, Tag: "n", Enabled: true}},
			UpdateStatus: &state.SubUpdateStatus{
				LastStatus:    "ok",
				LastSuccessAt: "2026-08-27T10:00:00Z",
				ErrorCount:    4,
				LastErrorMsg:  "старая ошибка",
			},
		},
	}
	gen := config.StartBuildReport()
	FeedBuildReportFromFetchStatus(gen, sources)

	if reasons := reportReasonsFromReport(config.BuildReportFetchDegraded); len(reasons) != 0 {
		t.Errorf("восстановившаяся подписка помечена деградацией: %v", reasons)
	}
}
