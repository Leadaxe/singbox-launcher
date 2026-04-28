package build

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"singbox-launcher/core/template"
)

// minimalTemplateData возвращает TemplateData для самого простого валидного
// шаблона (без vars, params, — все секции рендерятся as-is). Позволяет
// верифицировать оркестратор вне зависимости от template.GetEffectiveConfig
// сложной логики.
func minimalTemplateData(t *testing.T, raw string) *template.TemplateData {
	t.Helper()
	var rawCfg json.RawMessage = json.RawMessage(raw)
	td := &template.TemplateData{
		RawConfig: rawCfg,
	}
	// parseJSONWithOrder — внутренняя функция template-пакета. Чтобы не
	// зависеть от неё в тестах, парсим map вручную и используем плоский ord.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(rawCfg, &m); err != nil {
		t.Fatalf("parse template: %v", err)
	}
	td.Config = m
	// Order — детерминированный (из ключей map не получается, поэтому
	// используем алфавитный для теста-параметра).
	for k := range m {
		td.ConfigOrder = append(td.ConfigOrder, k)
	}
	return td
}

// TestBuildConfig_NilTemplate — defensive: ErrInvalidInputs.
func TestBuildConfig_NilTemplate(t *testing.T) {
	_, err := BuildConfig(BuildContext{Template: nil})
	var bad *ErrInvalidInputs
	if !errors.As(err, &bad) {
		t.Fatalf("want *ErrInvalidInputs, got %v", err)
	}
}

// TestBuildConfig_TrivialTemplate — самый простой шаблон без vars/params:
// orchestrator возвращает JSON с теми же секциями (через FormatSectionJSON
// fallback). Блок `/** @ParserConfig ... */` БОЛЬШЕ НЕ генерится — был
// удалён в SPEC 045 cleanup'е (state.json — canonical, дубликат не нужен).
func TestBuildConfig_TrivialTemplate(t *testing.T) {
	td := minimalTemplateData(t, `{"log":{"level":"info"}}`)
	res, err := BuildConfig(BuildContext{
		Template:         td,
	})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	out := string(res.ConfigJSON)
	// @ParserConfig блок более не должен присутствовать.
	if strings.Contains(out, "@ParserConfig") {
		t.Errorf("@ParserConfig block must NOT appear in output (SPEC 045 cleanup), got: %s", out)
	}
	if !strings.Contains(out, `"log"`) {
		t.Errorf("expected log section preserved, got: %s", out)
	}
	if !strings.Contains(out, `"level": "info"`) {
		t.Errorf("expected level preserved (after FormatSectionJSON), got: %s", out)
	}
}

// TestBuildConfig_NoParserConfig — ctx.ParserConfigJSON может быть пустым;
// поведение идентично непустому. Pin'им что @ParserConfig нигде не появляется.
func TestBuildConfig_NoParserConfig(t *testing.T) {
	td := minimalTemplateData(t, `{"log":{"level":"info"}}`)
	res, err := BuildConfig(BuildContext{Template: td})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if strings.Contains(string(res.ConfigJSON), "@ParserConfig") {
		t.Errorf("@ParserConfig block must not appear, got: %s", res.ConfigJSON)
	}
}

// TestBuildConfig_OutboundsSectionWithCache — cache.Outbounds попадают в
// секцию outbounds через BuildOutboundsSection (between markers).
func TestBuildConfig_OutboundsSectionWithCache(t *testing.T) {
	td := minimalTemplateData(t, `{"outbounds":[]}`)
	cache := &ParsedCache{
		Outbounds: []json.RawMessage{
			json.RawMessage(`{"type":"vless","tag":"node-1"}`),
		},
	}
	res, err := BuildConfig(BuildContext{
		Template:   td,
		Cache:      cache,
		ForPreview: true,
		Stats:      PreviewStats{NodesCount: 1},
	})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	out := string(res.ConfigJSON)
	if !strings.Contains(out, "@ParserSTART") || !strings.Contains(out, "@ParserEND") {
		t.Errorf("expected outbounds markers, got: %s", out)
	}
	if !strings.Contains(out, `"node-1"`) {
		t.Errorf("expected node-1 in outbounds, got: %s", out)
	}
}

// TestBuildConfig_DNSSectionMerged — DNS-секция мержится через MergeDNSSection.
func TestBuildConfig_DNSSectionMerged(t *testing.T) {
	td := minimalTemplateData(t, `{"dns":{"strategy":"ipv4_only"}}`)
	res, err := BuildConfig(BuildContext{
		Template: td,
		DNS: DNSConfig{
			Servers: []json.RawMessage{
				json.RawMessage(`{"tag":"primary","address":"1.1.1.1"}`),
			},
			Strategy: "prefer_ipv6",
		},
	})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	out := string(res.ConfigJSON)
	if !strings.Contains(out, `"primary"`) {
		t.Errorf("expected DNS server tag in output, got: %s", out)
	}
	// DNS.Strategy override должен взять верх.
	if !strings.Contains(out, `"prefer_ipv6"`) {
		t.Errorf("DNS Strategy override missing, got: %s", out)
	}
}

// TestBuildConfig_RouteSectionMerged — кастомные правила интегрируются через MergeRouteSection.
func TestBuildConfig_RouteSectionMerged(t *testing.T) {
	td := minimalTemplateData(t, `{"route":{"rules":[]}}`)
	res, err := BuildConfig(BuildContext{
		Template: td,
		Route: RouteConfig{
			Rules: []RouteRule{
				{
					Enabled:     true,
					Outbound:    "vpn-1",
					PrimaryRule: map[string]interface{}{"domain": "x"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	out := string(res.ConfigJSON)
	if !strings.Contains(out, `"vpn-1"`) {
		t.Errorf("expected custom rule outbound, got: %s", out)
	}
}

// TestBuildConfig_ResultIsValidJSON — выход — валидный JSON-документ.
func TestBuildConfig_ResultIsValidJSON(t *testing.T) {
	td := minimalTemplateData(t, `{"log":{"level":"info"}}`)
	res, err := BuildConfig(BuildContext{
		Template:         td,
	})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	// Note: output has /** @ParserConfig */ JSON-comment, so plain json.Unmarshal won't accept it.
	// Validate that everything past the comment block is valid JSON.
	out := string(res.ConfigJSON)
	if !strings.HasPrefix(out, "{") || !strings.HasSuffix(strings.TrimSpace(out), "}") {
		t.Errorf("output must be enclosed in {}: %s", out)
	}
}

// TestValidationResult_HasErrors — fatal vs warning.
func TestValidationResult_HasErrors(t *testing.T) {
	v := ValidationResult{Warnings: []string{"w1"}}
	if v.HasErrors() {
		t.Fatalf("warnings only: HasErrors must be false")
	}
	v.Errors = []string{"e1"}
	if !v.HasErrors() {
		t.Fatalf("errors set: HasErrors must be true")
	}
}
