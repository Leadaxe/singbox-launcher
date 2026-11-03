package build

// Конформанс-раннер корпуса DNS-ЗЕРКАЛА ПРАВИЛА (TEMPLATE_LANG §6.11,
// контракт 1.1.139).
//
// Гоняет contract/corpus/rule_dns/*.rule.json через тот же путь, что окно
// правила и сборка: запись правила → ResolveRouteWithGlobals → RuleDNSMirrors.
// Матчер снимается до сравнения: его форма — выбор стороны (§6.11), а
// нормативны число, порядок и действия DNS-правил.
//
// Тот же корпус читает LxBox. Регенерация:
// go test ./core/build -run TestContractCorpusRuleDNS -update-rule-dns

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"singbox-launcher/core/config/registry"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

var updateRuleDNSCorpus = flag.Bool("update-rule-dns", false,
	"перезаписать expected-файлы корпуса DNS-зеркала правила")

type corpusRuleDNSCase struct {
	Doc  string          `json:"_"`
	Rule corestate.Rule `json:"rule"`
}

type corpusRuleDNSExpected struct {
	DNSRules []map[string]interface{} `json:"dns_rules"`
}

func TestContractCorpusRuleDNS(t *testing.T) {
	dir := filepath.Join("..", "..", "contract", "corpus", "rule_dns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("корпус недоступен: %v", err)
	}
	cases := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".rule.json") {
			continue
		}
		caseName := strings.TrimSuffix(name, ".rule.json")
		cases++
		t.Run(caseName, func(t *testing.T) { runRuleDNSCorpusCase(t, dir, caseName) })
	}
	if cases == 0 {
		t.Fatal("корпус DNS-зеркала правила пуст — раннер молча проходил бы всегда")
	}
}

func runRuleDNSCorpusCase(t *testing.T, dir, caseName string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, caseName+".rule.json"))
	if err != nil {
		t.Fatalf("read case: %v", err)
	}
	var in corpusRuleDNSCase
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("parse case: %v", err)
	}
	rule := in.Rule

	// Наборы srs считаются скачанными: путь на каждый URL записи.
	srsCached := map[string][]string{}
	if rule.Kind == corestate.RuleKindSrs {
		paths := make([]string, len(rule.Refs))
		for i := range paths {
			paths[i] = "cached.srs"
		}
		srsCached[corestate.StableRuleID(rule)] = paths
	}
	route := ResolveRouteWithGlobals(&corestate.State{Rules: []corestate.Rule{rule}},
		&template.TemplateData{}, "", srsCached, template.TargetSpec{}, nil)

	reg, err := registry.Get()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	match := map[string]bool{}
	for _, k := range reg.Allowlist(ruleDNSMirrorMatchList) {
		match[k] = true
	}
	got := corpusRuleDNSExpected{DNSRules: []map[string]interface{}{}}
	for _, m := range RuleDNSMirrors(route) {
		for _, r := range m.Rules {
			action := map[string]interface{}{}
			for k, v := range r {
				if !match[k] {
					action[k] = v
				}
			}
			got.DNSRules = append(got.DNSRules, action)
		}
	}

	expPath := filepath.Join(dir, caseName+".expected.json")
	if *updateRuleDNSCorpus {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal expected: %v", err)
		}
		if err := os.WriteFile(expPath, append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write expected: %v", err)
		}
		return
	}
	expRaw, err := os.ReadFile(expPath)
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}
	var exp, gotNorm corpusRuleDNSExpected
	if err := json.Unmarshal(expRaw, &exp); err != nil {
		t.Fatalf("parse expected: %v", err)
	}
	// Через JSON, чтобы типы (числа, массивы) сравнивались одинаково.
	gotRaw, _ := json.Marshal(got)
	_ = json.Unmarshal(gotRaw, &gotNorm)
	if !reflect.DeepEqual(exp, gotNorm) {
		t.Errorf("%s: %s\nожидалось: %s\nполучено:  %s", caseName, in.Doc, expRaw, gotRaw)
	}
}
