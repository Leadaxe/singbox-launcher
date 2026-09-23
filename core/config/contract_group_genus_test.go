package config

// Страж рода группы (контракт 1.1.47, TASKS_LXBOX §43.2, CANON §5).
//
// Род группы — это тип тела sing-box, и таблица `genus` в
// `registry/protocols/group.json` называет его у каждого вида источника.
// Сборка документа (xray_balancer.go, singbox_groups.go) в маппер не входит
// (PRIMITIVES §10) и таблицу не читает, поэтому единственная связь «данные
// ↔ поведение» — вот эта сверка по корпусу: ожидание, где род расходится с
// таблицей, означает, что одна из двух правд устарела.
//
// Запуск:
//
//	go test ./core/config -run TestContractGroupGenus -count=1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type groupGenus struct {
	Values   []string          `json:"values"`
	BySource map[string]string `json:"by_source"`
}

func TestContractGroupGenus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractCorpusRelPath, "..", "registry", "protocols", "group.json"))
	if err != nil {
		t.Fatalf("чтение group.json: %v", err)
	}
	var file struct {
		Genus *groupGenus `json:"genus"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("разбор group.json: %v", err)
	}
	g := file.Genus
	if g == nil || len(g.Values) == 0 {
		t.Fatal("group.json не объявил genus.values — род группы не назван данными")
	}
	allowed := map[string]bool{}
	for _, v := range g.Values {
		allowed[v] = true
	}
	// Литерал рода у вида источника обязан быть одним из допустимых типов:
	// иначе таблица называла бы род, которого ядро не знает.
	for src, v := range g.BySource {
		if v != "$as_is" && !allowed[v] {
			t.Errorf("genus.by_source.%s = %q вне genus.values %v", src, v, g.Values)
		}
	}

	// Каталог корпуса тел назван по виду источника (`body/xray`,
	// `body/singbox`) — это имя и есть ключ by_source.
	root := filepath.Join(contractCorpusRelPath, "body")
	checked := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".expected.json") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var env struct {
			Nodes []struct {
				Kind  string         `json:"kind"`
				Entry map[string]any `json:"entry"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			t.Errorf("%s: %v", path, err)
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		source := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		want, declared := g.BySource[source]
		for _, n := range env.Nodes {
			if n.Kind != "group" {
				continue
			}
			checked++
			typ, _ := n.Entry["type"].(string)
			if !allowed[typ] {
				t.Errorf("%s: entry.type группы %q вне genus.values %v", rel, typ, g.Values)
				continue
			}
			if !declared {
				t.Errorf("%s: вид источника %q не назван в genus.by_source", rel, source)
				continue
			}
			if want != "$as_is" && typ != want {
				t.Errorf("%s: род группы %q, а genus.by_source.%s = %q", rel, typ, source, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход корпуса тел: %v", err)
	}
	// Ноль групп в корпусе — отказ, а не зелёный прогон: сверять было нечего.
	if checked == 0 {
		t.Fatal("в корпусе тел нет ни одного узла-группы — род сверять не на чем")
	}
}
