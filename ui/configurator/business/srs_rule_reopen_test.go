package business

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/services"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/srstag"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// TestSrsRuleSurvivesReopen — репорт 1.5.4 (Win10): srs-правило собиралось
// только в той сессии, где его добавили. После переоткрытия конфигуратора
// legacy-вид правила приходил без tag у rule_set-записи: кнопка «✔️ srs»
// пропадала (GetSRSEntries отбрасывает записи без tag), а сборка «Итога»
// падала на «rule-set: remote entry missing tag» и не пускала к Save.
//
// Сквозной сценарий: состояние v7 → Parse (legacy-вид) → модель как при
// открытии конфигуратора → сборка визарда. Заодно фиксирует, что при
// активных v6-правилах пользовательские правила не эмитятся дважды
// (зеркало боевого пути routeConfigForUpdate).
func TestSrsRuleSurvivesReopen(t *testing.T) {
	execDir := findProjectRoot(t)
	templateData, err := wizardtemplate.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}

	const srsURL = "https://example.com/rules/yt.srs"
	stateJSON := `{
  "meta": {"version": 7, "schema": "sources_v7", "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z"},
  "sources": [],
  "directions": [],
  "rules": [
    {"kind": "srs", "enabled": true, "order_num": 1000, "body": {"name": "YT", "srs_url": "` + srsURL + `", "outbound": "direct-out"}},
    {"kind": "inline", "enabled": true, "order_num": 1001, "body": {"name": "Inline", "match": {"domain": ["example.com"]}, "outbound": "direct-out"}}
  ],
  "vars": [],
  "dns_options": {"servers": [], "rules": []},
  "warp_accounts": {}
}`
	st, err := corestate.Parse([]byte(stateJSON))
	if err != nil {
		t.Fatalf("parse v7 state: %v", err)
	}

	// Модель — как restoreCustomRules/restorePresetRefs при открытии.
	model := wizardmodels.NewWizardModel()
	model.TemplateData = templateData
	model.ExecDir = execDir
	model.RulesLibraryMerged = true
	model.Sources = append(model.Sources, wizardmodels.Source{
		ID:   "01TESTSRSREOPEN0000000000",
		Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
		URL:  "https://example.com/sub.txt",
	})
	for i := range st.CustomRules {
		model.CustomRules = append(model.CustomRules, wizardmodels.PersistedCustomRuleToRuleState(&st.CustomRules[i]))
	}
	model.PresetRefs = wizardmodels.SyncStateRulesToPresetRefs(st.Rules)
	model.RuleOrder = wizardmodels.RuleOrderFromAxis(st.Rules, model.PresetRefs, model.CustomRules, model.NodeRuleRefs)
	wizardmodels.ReconcileRuleOrder(model)

	var srsRule *wizardmodels.RuleState
	for _, r := range model.CustomRules {
		if r.Rule.Label == "YT" {
			srsRule = r
		}
	}
	if srsRule == nil {
		t.Fatalf("srs rule not restored from state: %+v", st.CustomRules)
	}
	// Условие видимости кнопки «✔️ srs» и гейта скачивания в rules_tab.
	entries := services.GetSRSEntries(srsRule.Rule.RuleSets)
	wantTag := srstag.TagFromURL(srsURL)
	if len(entries) != 1 || entries[0].Tag != wantTag {
		t.Fatalf("srs entries after reopen = %+v, want one entry with tag %q", entries, wantTag)
	}

	options := EnsureDefaultAvailableOutbounds(GetAvailableOutbounds(model))
	EnsureFinalSelected(model, options)
	ApplyWizardDNSTemplate(model)
	stubLocalSRSForRules(t, execDir, model.CustomRules)

	text, err := BuildPreviewConfig(model)
	if err != nil {
		t.Fatalf("wizard build after reopen failed: %v", err)
	}
	var cfg struct {
		Route struct {
			Rules   []map[string]interface{} `json:"rules"`
			RuleSet []map[string]interface{} `json:"rule_set"`
		} `json:"route"`
	}
	if err := json.Unmarshal(jsonc.ToJSON([]byte(text)), &cfg); err != nil {
		t.Fatalf("config is not valid JSONC: %v", err)
	}

	userSrsRules, inlineRules := 0, 0
	for _, r := range cfg.Route.Rules {
		if s, _ := r["rule_set"].(string); strings.HasPrefix(s, "user:") {
			userSrsRules++
		}
		if d, ok := r["domain"].([]interface{}); ok && len(d) == 1 && d[0] == "example.com" {
			inlineRules++
		}
	}
	if userSrsRules != 1 || inlineRules != 1 {
		t.Errorf("user rules emitted srs=%d inline=%d, want exactly one each (no legacy double-emit)", userSrsRules, inlineRules)
	}
	remoteSrs, localUser := 0, 0
	for _, rs := range cfg.Route.RuleSet {
		if u, _ := rs["url"].(string); u == srsURL {
			remoteSrs++
		}
		if tag, _ := rs["tag"].(string); strings.HasPrefix(tag, "user:") && rs["type"] == "local" {
			localUser++
		}
	}
	if remoteSrs != 0 || localUser != 1 {
		t.Errorf("route.rule_set: remote-by-url=%d local user=%d, want 0 and 1", remoteSrs, localUser)
	}
}

// TestSrsRuleKeepsAllRuleSetsAcrossReopen — репорт 1.5.5 (Win10): правило с
// тремя srs после переоткрытия помнило только первый. Тело srs-правила
// держало один URL: сохранение из UI выбрасывало остальные наборы, а сборка
// эмитила один rule_set даже в сессии создания.
//
// Сквозной сценарий: состояние v7 с полным списком → legacy-вид (по записи
// на URL) → модель → обратная конверсия в state (все URL на месте — это то,
// что пишет Save) → сборка визарда (local rule_set на каждый набор и ОДНО
// правило маршрута со ссылкой на все).
func TestSrsRuleKeepsAllRuleSetsAcrossReopen(t *testing.T) {
	execDir := findProjectRoot(t)
	templateData, err := wizardtemplate.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}

	urls := []string{
		"https://example.com/rules/a.srs",
		"https://example.com/rules/b.srs",
		"https://example.com/rules/c.srs",
	}
	urlsJSON, _ := json.Marshal(urls)
	stateJSON := `{
  "meta": {"version": 7, "schema": "sources_v7", "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z"},
  "sources": [],
  "directions": [],
  "rules": [
    {"kind": "srs", "enabled": true, "order_num": 1000, "body": {"name": "Multi", "srs_url": "` + urls[0] + `", "srs_urls": ` + string(urlsJSON) + `, "outbound": "direct-out"}}
  ],
  "vars": [],
  "dns_options": {"servers": [], "rules": []},
  "warp_accounts": {}
}`
	st, err := corestate.Parse([]byte(stateJSON))
	if err != nil {
		t.Fatalf("parse v7 state: %v", err)
	}

	model := wizardmodels.NewWizardModel()
	model.TemplateData = templateData
	model.ExecDir = execDir
	model.RulesLibraryMerged = true
	model.Sources = append(model.Sources, wizardmodels.Source{
		ID:   "01TESTSRSMULTI00000000000",
		Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
		URL:  "https://example.com/sub.txt",
	})
	for i := range st.CustomRules {
		model.CustomRules = append(model.CustomRules, wizardmodels.PersistedCustomRuleToRuleState(&st.CustomRules[i]))
	}
	model.PresetRefs = wizardmodels.SyncStateRulesToPresetRefs(st.Rules)
	model.RuleOrder = wizardmodels.RuleOrderFromAxis(st.Rules, model.PresetRefs, model.CustomRules, model.NodeRuleRefs)
	wizardmodels.ReconcileRuleOrder(model)

	var rule *wizardmodels.RuleState
	for _, r := range model.CustomRules {
		if r.Rule.Label == "Multi" {
			rule = r
		}
	}
	if rule == nil {
		t.Fatalf("srs rule not restored from state: %+v", st.CustomRules)
	}

	// 1. Legacy-вид: кнопка «✔️ srs» и гейт скачивания видят ВСЕ наборы.
	entries := services.GetSRSEntries(rule.Rule.RuleSets)
	if len(entries) != len(urls) {
		t.Fatalf("srs entries after reopen = %+v, want %d", entries, len(urls))
	}
	for i, e := range entries {
		if e.URL != urls[i] || e.Tag != srstag.TagFromURL(urls[i]) {
			t.Errorf("entry %d = %+v, want url %q tag %q", i, e, urls[i], srstag.TagFromURL(urls[i]))
		}
	}

	// 2. Обратная конверсия модели в state — то, что пишет Save: все URL на месте.
	rulesV6 := wizardmodels.EmitStateRulesInAxisOrder(model.RuleOrder, model.PresetRefs, model.CustomRules)
	var saved []string
	for _, r := range rulesV6 {
		if r.Kind != corestate.RuleKindSrs {
			continue
		}
		body, err := r.DecodeBody()
		if err != nil {
			t.Fatalf("decode saved srs body: %v", err)
		}
		saved = body.(*corestate.SrsBody).URLs()
	}
	if strings.Join(saved, "\n") != strings.Join(urls, "\n") {
		t.Fatalf("Save would keep %v, want all %v", saved, urls)
	}

	// 3. Сборка: local rule_set на каждый набор, одно правило со списком тегов.
	options := EnsureDefaultAvailableOutbounds(GetAvailableOutbounds(model))
	EnsureFinalSelected(model, options)
	ApplyWizardDNSTemplate(model)
	stubLocalSRSForRules(t, execDir, model.CustomRules)

	text, err := BuildPreviewConfig(model)
	if err != nil {
		t.Fatalf("wizard build failed: %v", err)
	}
	var cfg struct {
		Route struct {
			Rules   []map[string]interface{} `json:"rules"`
			RuleSet []map[string]interface{} `json:"rule_set"`
		} `json:"route"`
	}
	if err := json.Unmarshal(jsonc.ToJSON([]byte(text)), &cfg); err != nil {
		t.Fatalf("config is not valid JSONC: %v", err)
	}

	wantTags := []string{"user:Multi", "user:Multi:2", "user:Multi:3"}
	for i, tag := range wantTags {
		found := 0
		for _, rs := range cfg.Route.RuleSet {
			if rs["tag"] != tag {
				continue
			}
			found++
			path, _ := rs["path"].(string)
			if rs["type"] != "local" || !strings.HasSuffix(path, srstag.TagFromURL(urls[i])+".srs") {
				t.Errorf("rule_set %q = %+v, want local file %s.srs", tag, rs, srstag.TagFromURL(urls[i]))
			}
		}
		if found != 1 {
			t.Errorf("rule_set %q emitted %d times, want 1 (rule_set section: %+v)", tag, found, cfg.Route.RuleSet)
		}
	}
	multiRules := 0
	for _, r := range cfg.Route.Rules {
		arr, ok := r["rule_set"].([]interface{})
		if !ok {
			continue
		}
		got := make([]string, 0, len(arr))
		for _, v := range arr {
			s, _ := v.(string)
			got = append(got, s)
		}
		if strings.Join(got, ",") != strings.Join(wantTags, ",") {
			continue
		}
		multiRules++
		if r["outbound"] != "direct-out" {
			t.Errorf("multi-srs route rule outbound = %v, want direct-out", r["outbound"])
		}
	}
	if multiRules != 1 {
		t.Errorf("route rules referencing %v: %d, want exactly 1 (rules: %+v)", wantTags, multiRules, cfg.Route.Rules)
	}
}
