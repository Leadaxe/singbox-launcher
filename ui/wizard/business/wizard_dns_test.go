package business

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

func TestPersistedDNSRulesForState_ValidMultiline(t *testing.T) {
	text := "{\"a\":1}\n#comment\n{\"b\":2}"
	rules := PersistedDNSRulesForState(text)
	if len(rules) != 2 {
		t.Fatalf("rules len: got %d want 2", len(rules))
	}
}

func TestPersistedDNSRulesForState_InvalidReturnsNil(t *testing.T) {
	if rules := PersistedDNSRulesForState("not json"); len(rules) != 0 {
		t.Fatalf("expected nil/empty rules, got %d", len(rules))
	}
}

func TestLoadPersistedWizardDNS_FromRulesArray(t *testing.T) {
	m := wizardmodels.NewWizardModel()
	p := &wizardmodels.PersistedDNSState{
		Rules: []json.RawMessage{
			json.RawMessage(`{"rule_set":"x","server":"a"}`),
			json.RawMessage(`{"server":"b"}`),
		},
	}
	LoadPersistedWizardDNS(m, p)
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(m.DNSRulesText), &root); err != nil {
		t.Fatalf("DNSRulesText must be valid JSON object, got err: %v; text=%q", err, m.DNSRulesText)
	}
	arr, ok := root["rules"].([]interface{})
	if !ok || len(arr) != 2 {
		t.Fatalf("DNSRulesText.rules: got %v, want array of 2 elements", root["rules"])
	}
}

func TestLoadPersistedWizardDNS_EmptyRulesClearsEditor(t *testing.T) {
	m := wizardmodels.NewWizardModel()
	m.DNSRulesText = "should clear"
	p := &wizardmodels.PersistedDNSState{}
	LoadPersistedWizardDNS(m, p)
	if m.DNSRulesText != "" {
		t.Fatalf("got %q want empty", m.DNSRulesText)
	}
}

func tagFromRaw(raw json.RawMessage) string {
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	s, _ := m["tag"].(string)
	return strings.TrimSpace(s)
}

func TestPickDNSStrategy_TemplateOptsOverridesSkeleton(t *testing.T) {
	dnsObj := map[string]interface{}{"strategy": "ipv4_only"}
	optsMap := map[string]json.RawMessage{"strategy": json.RawMessage(`"prefer_ipv6"`)}
	got := pickDNSStrategy(true, optsMap, dnsObj)
	if got != "prefer_ipv6" {
		t.Fatalf("got %q want prefer_ipv6 (dns_options overrides skeleton)", got)
	}
}

func TestPickDNSStrategy_SkeletonWhenOptsMissingOrEmpty(t *testing.T) {
	dnsObj := map[string]interface{}{"strategy": "ipv4_only"}
	if g := pickDNSStrategy(true, map[string]json.RawMessage{}, dnsObj); g != "ipv4_only" {
		t.Fatalf("empty opts map: got %q want ipv4_only", g)
	}
	optsNoKey := map[string]json.RawMessage{"servers": json.RawMessage(`[]`)}
	if g := pickDNSStrategy(true, optsNoKey, dnsObj); g != "ipv4_only" {
		t.Fatalf("no strategy key: got %q want ipv4_only", g)
	}
}

func TestApplyWizardDNSTemplate_StrategyFromSkeleton(t *testing.T) {
	root := findWizardTemplateRoot(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatal(err)
	}
	m := wizardmodels.NewWizardModel()
	m.TemplateData = td
	ApplyWizardDNSTemplate(m)
	ApplyDNSVarsFromSettingsToModel(m)
	if got := strings.TrimSpace(m.DNSStrategy); got != "" {
		t.Fatalf("DNSStrategy: with dns_strategy var and no state.vars override, model stays empty (effective value from template @dns_strategy); got %q", got)
	}
}

func TestApplyWizardDNSTemplate_OrderAndLocks(t *testing.T) {
	root := findWizardTemplateRoot(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatal(err)
	}
	m := wizardmodels.NewWizardModel()
	m.TemplateData = td

	ApplyWizardDNSTemplate(m)
	if len(m.DNSServers) < 2 {
		t.Fatalf("expected at least config.dns servers, got %d", len(m.DNSServers))
	}
	if tagFromRaw(m.DNSServers[0]) != "local_dns_resolver" {
		t.Fatalf("first server tag: got %q want local_dns_resolver", tagFromRaw(m.DNSServers[0]))
	}
	if !DNSTagLocked(m, "local_dns_resolver") || !DNSTagLocked(m, "direct_dns_resolver") {
		t.Fatal("expected config.dns tags to be locked")
	}
	if DNSTagLocked(m, "cloudflare_udp") {
		t.Fatal("dns_options-only tag must not be locked")
	}
}

func TestApplyWizardDNSTemplate_LockedTagUsesOptsWhenEnabled(t *testing.T) {
	root := findWizardTemplateRoot(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatal(err)
	}
	m := wizardmodels.NewWizardModel()
	m.TemplateData = td
	ApplyWizardDNSTemplate(m)

	idxDirect := -1
	for i, raw := range m.DNSServers {
		if tagFromRaw(raw) == "direct_dns_resolver" {
			idxDirect = i
			break
		}
	}
	if idxDirect < 0 {
		t.Fatal("direct_dns_resolver not found")
	}
	obj := make(map[string]interface{})
	if err := json.Unmarshal(m.DNSServers[idxDirect], &obj); err != nil {
		t.Fatal(err)
	}
	obj["enabled"] = false
	b, _ := json.Marshal(obj)
	m.DNSServers[idxDirect] = b

	ApplyWizardDNSTemplate(m)
	obj = make(map[string]interface{})
	if err := json.Unmarshal(m.DNSServers[idxDirect], &obj); err != nil {
		t.Fatal(err)
	}
	if _, has := obj["description"]; has {
		t.Fatal("disabled locked row should follow config shape without wizard description")
	}

	obj["enabled"] = true
	b, _ = json.Marshal(obj)
	m.DNSServers[idxDirect] = b
	ApplyWizardDNSTemplate(m)
	obj = make(map[string]interface{})
	if err := json.Unmarshal(m.DNSServers[idxDirect], &obj); err != nil {
		t.Fatal(err)
	}
	desc := strings.TrimSpace(fmt.Sprint(obj["description"]))
	if desc == "" {
		t.Fatal("enabled locked row with dns_options should carry description from template")
	}
}

func TestDNSEnabledTagOptions_ExcludesDisabledLockedAndCustom(t *testing.T) {
	root := findWizardTemplateRoot(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatal(err)
	}
	m := wizardmodels.NewWizardModel()
	m.TemplateData = td
	ApplyWizardDNSTemplate(m)

	customTag := "custom_disabled_udp"
	custom := map[string]interface{}{
		"tag": customTag, "type": "udp", "server": "9.9.9.9", "server_port": 53, "enabled": false,
	}
	cb, _ := json.Marshal(custom)
	m.DNSServers = append(m.DNSServers, cb)

	idx := -1
	for i, raw := range m.DNSServers {
		if tagFromRaw(raw) == "direct_dns_resolver" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("direct_dns_resolver not found")
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(m.DNSServers[idx], &obj); err != nil {
		t.Fatal(err)
	}
	obj["enabled"] = false
	b, _ := json.Marshal(obj)
	m.DNSServers[idx] = b

	opts := DNSEnabledTagOptions(m)
	for _, tag := range opts {
		if tag == "direct_dns_resolver" || tag == customTag {
			t.Fatalf("disabled tags must not appear in options, got %v", opts)
		}
	}
}

func TestValidateDNSModel_FinalRejectsDisabledLockedSkeleton(t *testing.T) {
	root := findWizardTemplateRoot(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatal(err)
	}
	m := wizardmodels.NewWizardModel()
	m.TemplateData = td
	ApplyWizardDNSTemplate(m)

	idx := -1
	for i, raw := range m.DNSServers {
		if tagFromRaw(raw) == "direct_dns_resolver" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("direct_dns_resolver not found")
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(m.DNSServers[idx], &obj); err != nil {
		t.Fatal(err)
	}
	obj["enabled"] = false
	b, _ := json.Marshal(obj)
	m.DNSServers[idx] = b

	m.DNSFinal = "direct_dns_resolver"
	if err := ValidateDNSModel(m); err == nil {
		t.Fatal("expected validation error for dns.final on disabled server (including locked skeleton)")
	}
}

func TestValidateDNSModel_FinalRejectsDisabledCustom(t *testing.T) {
	root := findWizardTemplateRoot(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatal(err)
	}
	m := wizardmodels.NewWizardModel()
	m.TemplateData = td
	ApplyWizardDNSTemplate(m)

	customTag := "custom_off"
	custom := map[string]interface{}{
		"tag": customTag, "type": "udp", "server": "9.9.9.9", "server_port": 53, "enabled": false,
	}
	cb, _ := json.Marshal(custom)
	m.DNSServers = append(m.DNSServers, cb)
	m.DNSFinal = customTag
	if err := ValidateDNSModel(m); err == nil {
		t.Fatal("expected validation error for dns.final on disabled non-locked server")
	}
}

func findWizardTemplateRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "bin", "wizard_template.json")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("bin/wizard_template.json not found")
	return ""
}
