package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestRuleCondStackRoundTrip — тело правила → стопка формы → тело не меняется
// на всех записях корпуса contract/corpus/rule_tree (SPEC 188 волна 4): форма
// открывает каждое такое тело и, ничего не меняя, пишет его тем же.
func TestRuleCondStackRoundTrip(t *testing.T) {
	table, err := LauncherRuleCondTable()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "contract", "corpus", "rule_tree", "*.rules.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("corpus rule_tree: %v (%d files)", err, len(files))
	}
	sort.Strings(files)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var doc struct {
			Rules []struct {
				Kind string                 `json:"kind"`
				Name string                 `json:"name"`
				Body map[string]interface{} `json:"body"`
			} `json:"rules"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, r := range doc.Rules {
			if r.Kind != "inline" && r.Kind != "srs" {
				continue
			}
			name := filepath.Base(path) + "/" + r.Name
			stack, ok := table.StackFromBody(r.Body)
			if !ok {
				t.Errorf("%s: form does not open the body", name)
				continue
			}
			want, _ := json.Marshal(ruleMatchForTest(r.Body))
			got, _ := json.Marshal(table.StackBody(stack))
			if string(got) != string(want) {
				t.Errorf("%s: body changed\n got: %s\nwant: %s", name, got, want)
			}
		}
	}
}

func ruleMatchForTest(body map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range body {
		switch k {
		case "outbound", "action", "method":
		default:
			out[k] = v
		}
	}
	return out
}
