package build

// Конформанс-раннер корпуса ВЫПУСКА ПРАВИЛА-ДЕРЕВА (TEMPLATE_LANG §6.13 —
// наборы и теги, §6.11 — зеркало по дереву; контракт 1.1.179–1.1.180, SPEC
// 188). Гоняет contract/corpus/rule_tree/<case>.rules.json тем же путём, что
// сборка: записи → NormalizeRuleRecord → ResolveRouteTagged (теги шаблона и
// пресетов заняты заранее) → RuleDNSMirrors. Матчер DNS нормативен.
//
// Пресеты кейса заданы только тегами наборов (`preset_rule_set_tags`): их
// записи из оси снимаются, а теги идут в занятые вместе с тегами шаблона —
// записи-пресеты базу не берут, поэтому теги правил от этого не меняются.
// Тот же корпус гоняет LxBox.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

type corpusRuleTreeCase struct {
	Doc               string              `json:"_"`
	TemplateTags      []string            `json:"template_rule_set_tags"`
	PresetRuleSetTags map[string][]string `json:"preset_rule_set_tags"`
	Rules             []corestate.Rule    `json:"rules"`
}

type corpusRuleTreeExpected struct {
	RuleSet     []map[string]interface{} `json:"rule_set"`
	RouteRules  []map[string]interface{} `json:"route_rules"`
	DNSRules    []map[string]interface{} `json:"dns_rules"`
	MirrorEmpty []string                 `json:"mirror_empty,omitempty"`
}

func TestContractCorpusRuleTree(t *testing.T) {
	dir := filepath.Join("..", "..", "contract", "corpus", "rule_tree")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("корпус недоступен: %v", err)
	}
	cases := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".rules.json") {
			continue
		}
		caseName := strings.TrimSuffix(name, ".rules.json")
		cases++
		t.Run(caseName, func(t *testing.T) { runRuleTreeCorpusCase(t, dir, caseName) })
	}
	if cases == 0 {
		t.Fatal("корпус rule_tree пуст — раннер молча проходил бы всегда")
	}
}

func runRuleTreeCorpusCase(t *testing.T, dir, caseName string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, caseName+".rules.json"))
	if err != nil {
		t.Fatalf("read case: %v", err)
	}
	var in corpusRuleTreeCase
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("parse case: %v", err)
	}

	reserved := append([]string{}, in.TemplateTags...)
	presetIDs := make([]string, 0, len(in.PresetRuleSetTags))
	for id := range in.PresetRuleSetTags {
		presetIDs = append(presetIDs, id)
	}
	sort.Strings(presetIDs)
	for _, id := range presetIDs {
		reserved = append(reserved, in.PresetRuleSetTags[id]...)
	}

	// Все наборы скачаны; srs_mode — local (README корпуса).
	srsCached := map[string]string{}
	var rules []corestate.Rule
	for _, r := range in.Rules {
		if r.Kind == corestate.RuleKindPreset {
			continue
		}
		corestate.NormalizeRuleRecord(&r)
		for _, ref := range corestate.RuleRefs(r) {
			srsCached[ref] = "cached.srs"
		}
		rules = append(rules, r)
	}
	route := ResolveRouteTagged(&corestate.State{Rules: rules}, &template.TemplateData{},
		"", srsCached, template.TargetSpec{}, nil, nil, reserved)

	got := corpusRuleTreeExpected{
		RuleSet:    []map[string]interface{}{},
		RouteRules: []map[string]interface{}{},
		DNSRules:   []map[string]interface{}{},
	}
	for _, rs := range route.RuleSets {
		if rs.Skipped || !rs.Enabled {
			continue
		}
		entry := map[string]interface{}{}
		for _, k := range []string{"tag", "type", "format", "rules"} {
			if v, ok := rs.Body[k]; ok {
				entry[k] = v
			}
		}
		if entry["type"] == ruleSetTypeInline {
			delete(entry, "format")
		}
		got.RuleSet = append(got.RuleSet, entry)
	}
	for _, r := range route.Rules {
		if !r.Enabled {
			continue
		}
		got.RouteRules = append(got.RouteRules, r.Body)
		if r.Mirror == nil && ruleDNSOptionOn(r.DNS) {
			got.MirrorEmpty = append(got.MirrorEmpty, r.Name)
		}
	}
	for _, m := range RuleDNSMirrors(route) {
		got.DNSRules = append(got.DNSRules, m.Rules...)
	}

	expRaw, err := os.ReadFile(filepath.Join(dir, caseName+".expected.json"))
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}
	var exp, gotNorm corpusRuleTreeExpected
	if err := json.Unmarshal(expRaw, &exp); err != nil {
		t.Fatalf("parse expected: %v", err)
	}
	// Через JSON, чтобы типы (числа, массивы) сравнивались одинаково.
	gotRaw, _ := json.MarshalIndent(got, "", "  ")
	_ = json.Unmarshal(gotRaw, &gotNorm)
	if !reflect.DeepEqual(exp, gotNorm) {
		t.Errorf("%s: %s\nожидалось: %s\nполучено:  %s", caseName, in.Doc, expRaw, gotRaw)
	}
}
