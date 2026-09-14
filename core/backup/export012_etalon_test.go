package backup

// Эталоны 0.12-писателя (SPEC 127 волна 2, W2.7 п. 3).
//
// Инвариант волны: файл 0.12 из Export012 — БАЙТ-В-БАЙТ такой же, как его
// писал сегодняшний Export до того, как экспорт разделили на двух писателей.
// Эталоны сняты ДО правок core/backup и лежат в testdata/export012_*.json;
// этот тест сверяет с ними текущий вывод на четырёх состояниях разного рода:
//
//   - mkstate         — подписка с политикой тегов/skip/detour, корневой
//                       сервер из URI, правила всех трёх видов и переменные;
//   - sections        — корневой узел с секциями (route-правило + DNS-пара);
//   - v8_fixture      — состояние фикстуры core/state/testdata/v8_roundtrip.json
//                       (папка, цепочка, DNS всех видов, warp, Направления);
//   - real_v088       — живое состояние golden-сценария real-v088-v8.
//
// Единственная допустимая разница с прежним выводом — секции узлов: норма
// ONE_NAMESPACE §4 везёт их только в 1.0, поэтому 0.12-писатель их больше не
// пишет и называет потерю кодом backup_local_only_dropped с полем sections.
// Эталон состояния с секциями снят уже с учётом этого решения.
//
// Регенерация (осознанное изменение формата 0.12, не рутина):
//
//	GEN_EXPORT012_ETALON=1 go test -run TestExport012MatchesEtalon ./core/backup/

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"singbox-launcher/core/state"
)

// etalonExportOpts — фиксированный момент и шапка: иначе «то же состояние»
// давало бы разные байты на каждом прогоне.
var etalonExportOpts = ExportOptions{
	AppVersion: "1.4.2",
	Platform:   "darwin",
	Now:        time.Unix(1750000000, 0),
}

// export012Etalon — файл эталона плюс коды предупреждений того же экспорта:
// потеря, о которой писатель молчит, — это потеря, которой никто не заметит.
type export012Etalon struct {
	Warnings []string        `json:"warnings"`
	Backup   json.RawMessage `json:"backup"`
}

// etalonStates — состояния, на которых снят эталон. Имя = имя файла.
func etalonStates(t *testing.T) map[string]*state.State {
	t.Helper()
	return map[string]*state.State{
		"mkstate":   mkState(),
		"sections":  stateWithSections(t, "ts-dns", 945),
		"v8fixture": loadEtalonState(t, "../state/testdata/v8_roundtrip.json"),
		"realv088":  loadEtalonState(t, "../build/testdata/golden/real-v088-v8/state.json"),
	}
}

// loadEtalonState читает состояние с диска через обычный Load, но всегда из
// КОПИИ: Load мигрирует и переписывает файл, а фикстуры трогать нельзя.
func loadEtalonState(t *testing.T, path string) *state.State {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	tmp := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load(tmp)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return s
}

func TestExport012MatchesEtalon(t *testing.T) {
	gen := os.Getenv("GEN_EXPORT012_ETALON") == "1"
	for name, s := range etalonStates(t) {
		t.Run(name, func(t *testing.T) {
			b, warns, err := Export012(s, etalonExportOpts)
			if err != nil {
				t.Fatalf("Export012: %v", err)
			}
			raw, err := json.MarshalIndent(b, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := export012Etalon{Warnings: warnCodesOf(warns), Backup: raw}
			path := filepath.Join("testdata", "export012_"+name+".json")
			encoded, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatalf("marshal etalon: %v", err)
			}
			encoded = append(encoded, '\n')
			if gen {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, encoded, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("эталон %s недоступен: %v", path, err)
			}
			if string(encoded) != string(want) {
				t.Errorf("файл 0.12 разошёлся с эталоном %s\n--- получено ---\n%s\n--- эталон ---\n%s",
					path, encoded, want)
			}
		})
	}
}

// warnCodesOf — коды предупреждений в порядке выдачи (дубли сохраняются:
// два одинаковых кода на разные записи — это две потери).
func warnCodesOf(warns []Warning) []string {
	out := make([]string, 0, len(warns))
	for _, w := range warns {
		out = append(out, w.Code)
	}
	return out
}
