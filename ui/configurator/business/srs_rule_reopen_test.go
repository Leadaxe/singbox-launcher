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
	model.RuleOrder = wizardmodels.RuleOrderFromAxis(st.Rules, model.PresetRefs, model.CustomRules)
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
