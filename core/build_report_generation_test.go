package core

import (
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/platform"
)

// SPEC 115 (фикс-раунд) — попытка сборки как единица: холостой Rebuild её не
// открывает, а поздний Finish чужой попытки её не закрывает.

// Холостой Rebuild (dirty-маркеры чисты, кэш на месте) обязан оставить
// готовый отчёт прошлой ПОЛНОЙ сборки нетронутым.
//
// Раньше разбор raw-кэша открывал попытку прямо у себя, а он идёт ДО
// noop-развилки: холостой вызов стирал записи прошлой сборки («снято N узлов»,
// ⚠ на источниках) и уходил по `return nil`, не положив взамен ни санитайзерных
// записей, ни признака готовности. Теперь разбор отчёта не касается вовсе —
// попытку открывает RebuildConfigIfDirty уже за развилкой.
func TestRawCacheSnapshotDoesNotTouchBuildReport(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	// Готовый отчёт прошлой полной сборки.
	gen := config.StartBuildReport()
	config.AddBuildReportEntries(gen, []config.BuildReportEntry{
		{Kind: config.BuildReportNodesDropped, Subject: "Big Sub", SourceID: "01BIG",
			SourceLabel: "Big Sub", Reason: "цель detour исчезла", NodeCount: 500},
	})
	config.FinishBuildReport(gen)

	execDir := t.TempDir()
	subsDir := platform.GetSubscriptionsDir(execDir)
	if err := os.MkdirAll(subsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("vless://12345678-1234-1234-1234-123456789abc@example.com:443?encryption=none&security=tls&type=tcp#tokyo\n")
	if err := state.WriteRawBody(subsDir, "01NOOPRAW", body); err != nil {
		t.Fatal(err)
	}
	s := &state.State{
		Sources: []state.Source{
			{ID: "01NOOPRAW", Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true}, URL: "https://test/sub"},
		},
	}
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), 0o755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(platform.GetWizardStatesDir(execDir), "state.json")
	if err := s.Save(statePath); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}

	// Ровно то, что делает Rebuild ДО noop-развилки.
	if _, _, err := buildSnapshotFromRawCache(loaded, execDir, nil, nil); err != nil {
		t.Fatalf("buildSnapshotFromRawCache: %v", err)
	}

	if !config.BuildReportReady() {
		t.Error("холостой Rebuild снял признак готовности — отчёт прошлой сборки объявлен несуществующим")
	}
	if n, _ := config.DroppedNodesForSource("01BIG"); n != 500 {
		t.Errorf("холостой Rebuild стёр пометку «снято N» прошлой сборки (осталось %d)", n)
	}
}

// Поздний Finish чужой попытки игнорируется: пока сборка шла, реестр мог
// перехватить другой писатель (фоновое авто-обновление подписок) или сбросить
// правка модели. Объявив готовым уже не свой отчёт, обогнанная сборка открыла
// бы Save на итоге, которого никто не видел.
func TestLateFinishOfSupersededAttemptIgnored(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	// Медленная сборка открыла свою попытку...
	slow := config.StartBuildReport()
	// ...а пока она шла, реестр перехватил другой писатель.
	fresh := config.StartBuildReport()

	if config.FinishBuildReport(slow) {
		t.Fatal("поздний Finish обогнанной попытки принят — Save открылась бы на чужом отчёте")
	}
	if config.BuildReportReady() {
		t.Error("обогнанная попытка объявила отчёт готовым")
	}

	// Записи обогнанной попытки тоже не проходят: они описывают уже не ту
	// конфигурацию, что лежит в реестре.
	FeedBuildReportFromSanitizer(slow, []build.SourceExclusion{{
		SourceID: "01OLD", SourceLabel: "Old Sub", Reason: "устаревшая причина", DroppedNodes: 9,
	}})
	if n, _ := config.DroppedNodesForSource("01OLD"); n != 0 {
		t.Errorf("запись обогнанной попытки попала в чужой отчёт (%d узлов)", n)
	}

	// Живая попытка работает как обычно.
	FeedBuildReportFromSanitizer(fresh, []build.SourceExclusion{{
		SourceID: "01NEW", SourceLabel: "New Sub", Reason: "актуальная причина", DroppedNodes: 4,
	}})
	if n, _ := config.DroppedNodesForSource("01NEW"); n != 4 {
		t.Errorf("живая попытка не смогла дописать свою запись (%d узлов)", n)
	}
	if !config.FinishBuildReport(fresh) {
		t.Error("живая попытка не смогла объявить свой отчёт готовым")
	}
}

// Инвалидация правкой модели убивает идущую попытку: её Finish после правки
// не имеет права объявить готовым отчёт конфигурации, которой уже нет.
func TestResetKillsRunningAttempt(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	gen := config.StartBuildReport()
	config.ResetBuildReport() // правка модели Мастера

	if config.FinishBuildReport(gen) {
		t.Fatal("сборка, начатая до правки модели, объявила свой отчёт готовым")
	}
	if config.BuildReportReady() {
		t.Error("гейт Save открылся отчётом конфигурации, которой уже нет")
	}
}
