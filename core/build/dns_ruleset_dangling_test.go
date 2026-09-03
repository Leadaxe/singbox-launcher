// File dns_ruleset_dangling_test.go — SPEC 118, расхождение Р-DNS-2.
//
// Предмет: множество валидных `rule_set`-тегов, по которому чистятся
// DNS-правила. Ошибка была в его НЕПОЛНОТЕ — оно строилось только по
// правилам kind=preset, и живая ссылка на тег, объявленный шаблоном или
// пользовательским srs-правилом, снималась как висячая. Молча: правило
// теряло ограничение и начинало матчить весь трафик.
//
// Тест держит обе стороны сразу — иначе «фикс» вырождается в отключение
// чистки, а она защищает от падения ядра
// (`initialize DNS rule[N]: rule-set not found`).
package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	state "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// TestDNSRuleSetRefsSurviveWhenTagIsEmitted — комплексный прогон BuildConfig
// на одном конфиге с ТРЕМЯ DNS-правилами разом:
//
//	ru-domains  — тег объявлен прямо в route.rule_set ШАБЛОНА → живой;
//	user:Ads    — тег эмитит правило состояния kind=srs (файл в кэше) → живой;
//	ghost-set   — тега нет нигде → висячий, обязан быть снят.
//
// Три в одном прогоне намеренно: разделив их, легко получить зелёный тест на
// сборке, где чистка вообще отключена.
func TestDNSRuleSetRefsSurviveWhenTagIsEmitted(t *testing.T) {
	// .srs-файл обязан существовать на диске: без него резолвер помечает
	// rule_set как Skipped, тег в конфиг не уезжает, и ссылка на него
	// становится по-настоящему висячей.
	execDir := t.TempDir()
	srsPath := filepath.Join(execDir, "resources", "srs")
	if err := os.MkdirAll(srsPath, 0o755); err != nil {
		t.Fatalf("mkdir srs: %v", err)
	}

	srsRule := state.Rule{
		Kind:    state.RuleKindSrs,
		Enabled: true,
		Body: json.RawMessage(`{"name":"Ads",` +
			`"srs_url":"https://example.invalid/geosite-ads.srs","outbound":"direct-out"}`),
	}
	srsID := state.StableRuleID(srsRule)
	srsTag := "user:" + srsID
	srsFile := filepath.Join(srsPath, srsID+".srs")
	if err := os.WriteFile(srsFile, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write srs: %v", err)
	}

	// Шаблон: route.rule_set уже несёт `ru-domains`; порядок секций
	// зафиксирован так, что dns собирается ДО route — это и есть условие,
	// в котором баг возникал.
	td := &template.TemplateData{
		RawConfig: json.RawMessage(`{}`),
		Config: map[string]json.RawMessage{
			"dns": json.RawMessage(`{"servers":[{"tag":"direct_dns","type":"local"}]}`),
			"route": json.RawMessage(`{"rules":[],"rule_set":[` +
				`{"tag":"ru-domains","type":"inline","format":"domain_suffix",` +
				`"rules":[{"domain_suffix":["ru"]}]}]}`),
		},
		ConfigOrder: []string{"dns", "route"},
	}

	ctx := BuildContext{
		Template: td,
		Preset: PresetMergeContext{
			ExecDir:        execDir,
			SrsCachedPaths: map[string]string{srsID: srsFile},
			Rules:          []state.Rule{srsRule},
			DNS: state.DNSOptions{
				Servers: []state.DNSServer{
					{Kind: state.DNSServerKindUser, Tag: "direct_dns", Enabled: true, Body: map[string]interface{}{
						"tag": "direct_dns", "type": "local",
					}},
				},
				Rules: []state.DNSRule{
					{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{
						"rule_set": "ru-domains", "server": "direct_dns",
					}},
					{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{
						"rule_set": srsTag, "server": "direct_dns",
					}},
					{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{
						"rule_set": "ghost-set", "server": "direct_dns",
					}},
				},
			},
		},
	}

	res, err := BuildConfig(ctx)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}

	var cfg struct {
		DNS struct {
			Rules []map[string]interface{} `json:"rules"`
		} `json:"dns"`
		Route struct {
			RuleSet []map[string]interface{} `json:"rule_set"`
		} `json:"route"`
	}
	if err := json.Unmarshal(res.ConfigJSON, &cfg); err != nil {
		t.Fatalf("parse built config: %v\n%s", err, res.ConfigJSON)
	}

	// Предпосылка теста: оба «живых» тега действительно уехали в route.
	// Если она сломается, тест ниже проверял бы не то, что заявляет.
	emitted := make(map[string]bool, len(cfg.Route.RuleSet))
	for _, rs := range cfg.Route.RuleSet {
		if tag, _ := rs["tag"].(string); tag != "" {
			emitted[tag] = true
		}
	}
	for _, want := range []string{"ru-domains", srsTag} {
		if !emitted[want] {
			t.Fatalf("предпосылка сломана: %q нет в route.rule_set: %v", want, emitted)
		}
	}
	if emitted["ghost-set"] {
		t.Fatalf("предпосылка сломана: ghost-set эмитнут, ссылка на него не висячая")
	}

	// Что именно уцелело у DNS-правил.
	kept := make(map[string]bool, len(cfg.DNS.Rules))
	rulesWithoutRuleSet := 0
	for _, r := range cfg.DNS.Rules {
		if tag, ok := r["rule_set"].(string); ok && tag != "" {
			kept[tag] = true
			continue
		}
		rulesWithoutRuleSet++
	}

	if !kept["ru-domains"] {
		t.Errorf("Р-DNS-2: снята живая ссылка на шаблонный rule_set ru-domains; dns.rules=%v", cfg.DNS.Rules)
	}
	if !kept[srsTag] {
		t.Errorf("Р-DNS-2: снята живая ссылка на srs-rule_set %q; dns.rules=%v", srsTag, cfg.DNS.Rules)
	}
	if kept["ghost-set"] {
		t.Errorf("чистка отключена: висячая ссылка ghost-set дошла до конфига; dns.rules=%v", cfg.DNS.Rules)
	}
	// Висячее правило теряет `rule_set`, но остаётся по `server` — ровно
	// одно правило без rule_set. Больше — значит сняли лишнее.
	if rulesWithoutRuleSet != 1 {
		t.Errorf("ожидалось ровно одно DNS-правило без rule_set (вычищенный ghost-set), получено %d; dns.rules=%v",
			rulesWithoutRuleSet, cfg.DNS.Rules)
	}
}
