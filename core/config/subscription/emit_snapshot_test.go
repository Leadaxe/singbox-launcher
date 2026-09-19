package subscription

// Снимок РУКОПИСНОГО эмита share-ссылок (SPEC 133, обратный ход).
//
// Зачем снимок. Обратный ход переезжает на движок `core/config/linkmap`, и
// единственный способ доказать, что вид ссылки не поехал, — сравнить вывод
// движка с байтами, которые писал рукописный эмиттер. Снимок снимается
// ПРОГОНОМ (MAPPER_ENGINE.md «Рекомендации» §5), а не пишется руками:
// вписанное руками описывает намерение и расходится с кодом молча.
//
// Тела берутся из ОЖИДАНИЙ общего корпуса `contract/corpus/uri` — это самые
// богатые тела, какие есть (полный xhttp, reality, ws ed, mux, dialer,
// AWG/AWG3, ss plugin, hy2 obfs/mport, tuic, masque, ssh, socks/http с
// паролем, naive). Урок кампании: бедная фикстура даёт зелёную сверку при
// потерянных полях, поэтому снимок снимается по ВСЕМУ корпусу, а не по
// выборке.
//
// Снятие:
//
//	go test ./core/config/subscription -run TestShareURISnapshot -update
//
// Проверка (без -update) сторожит сам рукописный эмиттер до его удаления.

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateSnapshot = flag.Bool("update", false, "переснять снимок эмита")

const (
	emitCorpusRoot   = "../../../contract/corpus/uri"
	emitSnapshotFile = "../linkmap/testdata/emit_snapshot.json"
)

// snapshotEntry — одна строка снимка.
type snapshotEntry struct {
	// Body — тело узла, из которого строилась ссылка (вход эмита).
	Body map[string]interface{} `json:"body"`
	// Label — метка узла: рукописный эмиттер берёт её из `tag` тела, движок
	// получает отдельным полем. Кладётся в снимок, чтобы фрагмент ссылки
	// судился вместе со всем остальным: без метки экранирование фрагмента
	// не проверял бы никто.
	Label string `json:"label,omitempty"`
	// URI — что написал рукописный эмиттер; пусто, если он отказал.
	URI string `json:"uri,omitempty"`
	// Err — текст отказа рукописного эмиттера (без него отказ неотличим от
	// пустой ссылки).
	Err string `json:"err,omitempty"`
}

// corpusCase — тело узла из ожидания корпуса вместе с его меткой.
type corpusCase struct {
	body  map[string]interface{}
	label string
}

// collectCorpusBodies собирает тела узлов из ожиданий корпуса.
// Ключ — "<каталог>/<кейс>".
func collectCorpusBodies(t *testing.T) map[string]corpusCase {
	t.Helper()
	out := map[string]corpusCase{}
	dirs, err := os.ReadDir(emitCorpusRoot)
	if err != nil {
		t.Fatalf("корпус: %v", err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		cases, err := filepath.Glob(filepath.Join(emitCorpusRoot, d.Name(), "*.uri"))
		if err != nil {
			t.Fatalf("glob: %v", err)
		}
		for _, casePath := range cases {
			base := strings.TrimSuffix(casePath, ".uri")
			path := base + ".expected.launcher.json"
			if _, err := os.Stat(path); err != nil {
				path = base + ".expected.json"
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var env struct {
				Nodes []struct {
					Entry map[string]interface{} `json:"entry"`
					Label string                 `json:"label"`
				} `json:"nodes"`
			}
			if err := json.Unmarshal(data, &env); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			if len(env.Nodes) == 0 || env.Nodes[0].Entry == nil {
				continue
			}
			name := d.Name() + "/" + strings.TrimSuffix(filepath.Base(casePath), ".uri")
			out[name] = corpusCase{body: env.Nodes[0].Entry, label: env.Nodes[0].Label}
		}
	}
	return out
}

func TestShareURISnapshot(t *testing.T) {
	bodies := collectCorpusBodies(t)
	if len(bodies) == 0 {
		t.Fatal("корпус пуст — снимать нечего")
	}
	got := map[string]snapshotEntry{}
	names := make([]string, 0, len(bodies))
	for name := range bodies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := bodies[name]
		// Рукописный эмиттер читает метку из `tag` тела (fragmentFromTag),
		// движок получает её отдельным полем — снимок обязан кормить обоих
		// ОДНОЙ меткой, иначе сверка судила бы разные входы.
		body := map[string]interface{}{}
		for k, v := range c.body {
			body[k] = v
		}
		if c.label != "" {
			body["tag"] = c.label
		}
		uri, err := ShareURIFromOutbound(body)
		e := snapshotEntry{Body: c.body, Label: c.label}
		if err != nil {
			e.Err = err.Error()
		} else {
			e.URI = uri
		}
		got[name] = e
	}

	if *updateSnapshot {
		buf, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = append(buf, '\n')
		if err := os.MkdirAll(filepath.Dir(emitSnapshotFile), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(emitSnapshotFile, buf, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		t.Logf("снимок обновлён: %d кейсов", len(got))
		return
	}

	data, err := os.ReadFile(emitSnapshotFile)
	if err != nil {
		t.Skipf("снимка нет (%v) — снять прогоном с -update", err)
	}
	var want map[string]snapshotEntry
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("снимок: %v", err)
	}
	for _, name := range names {
		w, ok := want[name]
		if !ok {
			t.Errorf("%s: в снимке нет кейса — переснять", name)
			continue
		}
		g := got[name]
		if g.URI != w.URI || g.Err != w.Err {
			t.Errorf("%s: рукописный эмит поехал\n got: %q / %q\nwant: %q / %q", name, g.URI, g.Err, w.URI, w.Err)
		}
	}
}
