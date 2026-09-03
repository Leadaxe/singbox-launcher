// File row_refresh_writeback_test.go — комплексный тест пути кнопки ↻ строки
// подписки (SPEC 116 W13).
//
// Путь целиком: клик ↻ → снимок источника → fetch на in-memory URL →
// материализация тела (включая НЕРАЗОБРАННЫЕ записи узлами kind=unsupported)
// → merge в nodes[] → updateStatus → запись результата в живую модель
// (ApplyFetchSnapshot) → закрепление в state.json.
//
// Что ловит тест. До W13 путь ↻ был единственным входом fetch'а, который на
// диск не писал вовсе: результат жил только в модели визарда, а state.json в
// это же время переписывали ДРУГИЕ писатели (heartbeat авто-обновления,
// VPN-event retry, Update) — каждый со своей копии, загруженной с диска, и
// целым файлом. На живой машине это выглядело так: ↻ отработал, лог показал
// разбор, state.json перезаписан свежим mtime — а у ЭТОЙ подписки
// last_success_at и состав узлов остались часовой давности.
package business

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	corepkg "singbox-launcher/core"
	"singbox-launcher/core/services"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/platform"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// rowRefreshBody — тело подписки с одной разбираемой записью и одной, которую
// парсер понять не может: unsupported обязан доехать до диска узлом, иначе
// пользователь не увидит, что провайдер прислал строку, которую мы не поняли.
const rowRefreshBody = "vless://11111111-1111-4111-8111-111111111111@s1.example:443?security=tls&sni=s1.example#RU-1\n" +
	"totally-unknown-scheme://x@s2.example:443#Weird-1\n"

// rowRefreshFixture — контроллер с временным execDir и подписка-стаб.
func rowRefreshFixture(t *testing.T, body string) (*corepkg.ConfigService, string, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	execDir := t.TempDir()
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), platform.DefaultDirMode); err != nil {
		t.Fatal(err)
	}
	ac := &corepkg.AppController{FileService: &services.FileService{ExecDir: execDir}}
	return corepkg.NewConfigService(ac), execDir, srv.URL
}

func rowRefreshModel(url string) *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{Node: wizardmodels.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
			ID: "SUBLIB", URL: url, Name: "Liberty"},
	}}
}

// clickRowRefresh — ровно то, что делает refreshOneSourceFromUI: глубокий
// снимок по ID, fetch в «горутине», запись результата в живую модель под
// проверку ревизии. Fyne тут не нужен: у пути кнопки нет виджетной логики,
// только эти три шага.
func clickRowRefresh(t *testing.T, svc *corepkg.ConfigService, m *wizardmodels.WizardModel, sourceID string) {
	t.Helper()
	idx := -1
	for i := range m.Sources {
		if m.Sources[i].ID == sourceID {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("источник %s не найден в модели", sourceID)
	}
	snapshot := m.Sources[idx]
	// Глубокая копия — как deepCopySourceForFetch в UI: горутина не имеет
	// права уехать с backing-массивами живой модели.
	if snapshot.Meta != nil {
		mc := *snapshot.Meta
		snapshot.Meta = &mc
	}
	if snapshot.Nodes != nil {
		snapshot.Nodes = append([]wizardmodels.Node(nil), snapshot.Nodes...)
	}
	if snapshot.PendingDisabled != nil {
		snapshot.PendingDisabled = append([]string(nil), snapshot.PendingDisabled...)
	}
	revAtStart := m.Revision
	if _, err := svc.RefreshSourceInPlace(&snapshot); err != nil {
		t.Fatalf("RefreshSourceInPlace: %v", err)
	}
	if !ApplyFetchSnapshot(m, &snapshot, revAtStart) {
		t.Fatal("ApplyFetchSnapshot: результат fetch'а не записан в модель")
	}
	m.BumpRevision()
}

func countUnsupported(nodes []corestate.Node) int {
	n := 0
	for i := range nodes {
		if nodes[i].Kind == corestate.SourceKindUnsupported {
			n++
		}
	}
	return n
}

// TestRowRefreshDeliversFetchResultToStateFile — весь путь кнопки ↻ строки за
// один прогон: скачано → материализовано (включая unsupported) → merge →
// updateStatus → модель → state.json.
//
// Диск проверяется БЕЗ Save визарда: именно этого закрепления пути не хватало.
// Пользователь, нажавший ↻ и не нажавший Save, обязан найти результат в
// состоянии — иначе его затрёт первый же чужой писатель state.json.
func TestRowRefreshDeliversFetchResultToStateFile(t *testing.T) {
	svc, execDir, url := rowRefreshFixture(t, rowRefreshBody)
	statePath := platform.GetWizardStatePath(execDir)
	m := rowRefreshModel(url)

	// Состояние, каким его сохранил визард ДО обновления: подписка есть,
	// узлов нет, попыток fetch'а не было.
	seed := corestate.New()
	seed.Sources = append([]corestate.Source(nil), m.Sources...)
	if err := seed.Save(statePath); err != nil {
		t.Fatal(err)
	}

	clickRowRefresh(t, svc, m, "SUBLIB")

	// 1. Модель: состав и диагностика.
	live := &m.Sources[0]
	if len(live.Nodes) != 2 {
		t.Fatalf("модель: ожидались 2 узла (собравшийся + unsupported), получено %d", len(live.Nodes))
	}
	if countUnsupported(live.Nodes) != 1 {
		t.Errorf("модель: неразобранная запись не материализована узлом: %+v", live.Nodes)
	}
	if live.UpdateStatus == nil || live.UpdateStatus.LastStatus != "ok" || live.UpdateStatus.LastSuccessAt == "" {
		t.Fatalf("модель: updateStatus не заполнен успешной попыткой: %+v", live.UpdateStatus)
	}
	if live.UpdateStatus.NodesCountFetched != 1 {
		t.Errorf("модель: счёт узлов считает unsupported за узел подписки: %d", live.UpdateStatus.NodesCountFetched)
	}

	// 2. Диск — БЕЗ Save визарда. Это и есть предмет бага.
	disk, err := corestate.Load(statePath)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	got := disk.FindSource("SUBLIB")
	if got == nil {
		t.Fatal("state.json: источник исчез")
	}
	if got.UpdateStatus == nil || got.UpdateStatus.LastSuccessAt == "" {
		t.Fatalf("state.json: last_success_at не доехал — результат ↻ остался только в памяти: %+v", got.UpdateStatus)
	}
	if got.UpdateStatus.LastSuccessAt != live.UpdateStatus.LastSuccessAt {
		t.Errorf("state.json: last_success_at %q ≠ модельного %q",
			got.UpdateStatus.LastSuccessAt, live.UpdateStatus.LastSuccessAt)
	}
	if len(got.Nodes) != 2 || countUnsupported(got.Nodes) != 1 {
		t.Errorf("state.json: состав узлов не доехал (nodes=%d, unsupported=%d)",
			len(got.Nodes), countUnsupported(got.Nodes))
	}

	// 3. Правки пользователя на диске путь fetch'а не переписывает: у него
	// свои поля, остальным владеет Save визарда.
	if got.URL != url || got.Name != "Liberty" {
		t.Errorf("state.json: fetch переписал поля пользователя (url=%q name=%q)", got.URL, got.Name)
	}
}

// Источник, которого визард ещё не сохранял (cold start, сценарий 1
// RefreshSourceInPlace): ↻ обязан отработать и НЕ родить на диске половинчатую
// запись — state.json пишет визард.
func TestRowRefreshColdStartDoesNotInventSourceOnDisk(t *testing.T) {
	svc, execDir, url := rowRefreshFixture(t, rowRefreshBody)
	statePath := platform.GetWizardStatePath(execDir)
	m := rowRefreshModel(url)

	clickRowRefresh(t, svc, m, "SUBLIB")

	if len(m.Sources[0].Nodes) != 2 {
		t.Fatalf("cold start: обновление в памяти не отработало: %d узл(ов)", len(m.Sources[0].Nodes))
	}
	if _, err := os.Stat(statePath); err == nil {
		disk, loadErr := corestate.Load(statePath)
		if loadErr != nil {
			t.Fatalf("state.Load: %v", loadErr)
		}
		if disk.FindSource("SUBLIB") != nil {
			t.Error("cold start: путь fetch'а создал источник в state.json — это работа Save визарда")
		}
	}
}

// Неудачный fetch: nodes[] на диске не трогаются (SPEC 113-A), а диагностика
// отказа доезжает — иначе строка источника осталась бы с виду здоровой.
func TestRowRefreshFailureKeepsNodesAndPersistsError(t *testing.T) {
	execDir := t.TempDir()
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), platform.DefaultDirMode); err != nil {
		t.Fatal(err)
	}
	ok := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(rowRefreshBody))
	}))
	defer srv.Close()
	ac := &corepkg.AppController{FileService: &services.FileService{ExecDir: execDir}}
	svc := corepkg.NewConfigService(ac)
	statePath := platform.GetWizardStatePath(execDir)

	m := rowRefreshModel(srv.URL)
	seed := corestate.New()
	seed.Sources = append([]corestate.Source(nil), m.Sources...)
	if err := seed.Save(statePath); err != nil {
		t.Fatal(err)
	}

	clickRowRefresh(t, svc, m, "SUBLIB")
	goodSuccess := m.Sources[0].UpdateStatus.LastSuccessAt

	ok = false
	clickRowRefresh(t, svc, m, "SUBLIB")

	disk, err := corestate.Load(statePath)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	got := disk.FindSource("SUBLIB")
	if got == nil {
		t.Fatal("state.json: источник исчез")
	}
	if len(got.Nodes) != 2 {
		t.Errorf("state.json: неудачный fetch тронул nodes[] (%d узл(ов))", len(got.Nodes))
	}
	if got.UpdateStatus == nil || got.UpdateStatus.LastStatus != "err" {
		t.Fatalf("state.json: отказ не записан: %+v", got.UpdateStatus)
	}
	if got.UpdateStatus.LastSuccessAt != goodSuccess {
		t.Errorf("state.json: память о последнем успехе потеряна: %q ≠ %q",
			got.UpdateStatus.LastSuccessAt, goodSuccess)
	}
}
