package build

// Конформанс-раннер раздела corpus/template/for_each/ (LxBox §578,
// TEMPLATE_LANG §4.8, §6.5–§6.7). Формат кейса — contract/corpus/template/README.md:
//
//	<case>.preset.json    {"preset": {...}, "nodes": [{tag, body, enabled?, in_config?, skip_presets?}]}
//	<case>.vars.json      значения переменных пресета строками
//	<case>.expected.json  {"load"?, "rules", "dns_servers", "dns_rules", "warnings"}
//
// Узлы с enabled=false или in_config=false сборка в for_each не передаёт —
// раннер отсеивает их так же.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/template"
)

const forEachCorpusRelPath = "../../contract/corpus/template/for_each"

type forEachCaseNode struct {
	Tag         string                 `json:"tag"`
	Body        map[string]interface{} `json:"body"`
	Enabled     *bool                  `json:"enabled"`
	InConfig    *bool                  `json:"in_config"`
	SkipPresets bool                   `json:"skip_presets"`
}

type forEachExpected struct {
	Load       string        `json:"load"`
	Rules      []interface{} `json:"rules"`
	DNSServers []interface{} `json:"dns_servers"`
	DNSRules   []interface{} `json:"dns_rules"`
	Warnings   []string      `json:"warnings"`
}

func TestContractCorpusForEach(t *testing.T) {
	entries, err := os.ReadDir(forEachCorpusRelPath)
	if err != nil {
		t.Skipf("раздел for_each не найден: %v", err)
	}
	cases := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".preset.json") {
			continue
		}
		cases++
		base := strings.TrimSuffix(e.Name(), ".preset.json")
		t.Run(base, func(t *testing.T) {
			read := func(suffix string, into interface{}) {
				raw, err := os.ReadFile(filepath.Join(forEachCorpusRelPath, base+suffix))
				if err != nil {
					t.Fatalf("read %s: %v", suffix, err)
				}
				if err := json.Unmarshal(raw, into); err != nil {
					t.Fatalf("parse %s: %v", suffix, err)
				}
			}
			var in struct {
				Preset map[string]interface{} `json:"preset"`
				Nodes  []forEachCaseNode      `json:"nodes"`
			}
			var vars map[string]string
			var exp forEachExpected
			read(".preset.json", &in)
			read(".vars.json", &vars)
			read(".expected.json", &exp)

			in.Preset["id"] = "case"
			rawPreset, _ := json.Marshal([]interface{}{in.Preset})
			presets, _ := template.LoadPresets(rawPreset, nil)
			loaded := len(presets) == 1
			switch exp.Load {
			case "", "accept":
				if !loaded {
					t.Fatalf("load: пресет отвергнут, ожидался accept")
				}
			case "reject":
				if loaded {
					t.Fatalf("load: пресет принят, ожидался reject")
				}
			}

			got := forEachExpected{Rules: []interface{}{}, DNSServers: []interface{}{}, DNSRules: []interface{}{}, Warnings: []string{}}
			if loaded {
				var nodes []template.PresetNode
				for _, n := range in.Nodes {
					if (n.Enabled != nil && !*n.Enabled) || (n.InConfig != nil && !*n.InConfig) {
						continue
					}
					nodes = append(nodes, template.PresetNode{Tag: n.Tag, Body: n.Body, SkipPresets: n.SkipPresets})
				}
				frags, warns, _ := ExpandPresetForNodes(&presets[0], vars, nil, nil, template.LocalTarget(), nodes)
				for _, r := range frags.RoutingRules {
					got.Rules = append(got.Rules, r)
				}
				for _, s := range frags.DNSServers {
					got.DNSServers = append(got.DNSServers, s)
				}
				for _, r := range frags.DNSRules {
					got.DNSRules = append(got.DNSRules, r)
				}
				seen := map[string]bool{}
				for _, w := range warns {
					if w.Code != "" && !seen[w.Code] {
						seen[w.Code] = true
						got.Warnings = append(got.Warnings, w.Code)
					}
				}
				sort.Strings(got.Warnings)
			}
			norm := func(v interface{}) interface{} {
				raw, _ := json.Marshal(v)
				var out interface{}
				_ = json.Unmarshal(raw, &out)
				return out
			}
			if exp.Warnings == nil {
				exp.Warnings = []string{}
			}
			sort.Strings(exp.Warnings)
			if !reflect.DeepEqual(norm(got.Rules), norm(exp.Rules)) ||
				!reflect.DeepEqual(norm(got.DNSServers), norm(exp.DNSServers)) ||
				!reflect.DeepEqual(norm(got.DNSRules), norm(exp.DNSRules)) ||
				!reflect.DeepEqual(got.Warnings, exp.Warnings) {
				gj, _ := json.Marshal(got)
				ej, _ := json.Marshal(exp)
				t.Errorf("расхождение с контрактом\n  получено:  %s\n  ожидалось: %s", gj, ej)
			}
		})
	}
	if cases == 0 {
		t.Fatal("раздел for_each пуст")
	}
}
