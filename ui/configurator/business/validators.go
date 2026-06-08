package business

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/build"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// -----------------------------------------------------------------------------
// Sing-box merge / validation / enabled tags (unchanged behaviour)
// -----------------------------------------------------------------------------

// dnsServerEnabledInWizard: missing or invalid "enabled" → true (same as sing-box: no such field).
func dnsServerEnabledInWizard(m map[string]interface{}) bool {
	v, ok := m["enabled"]
	if !ok || v == nil {
		return true
	}
	b, ok := v.(bool)
	if !ok {
		return true
	}
	return b
}

// DNSServerWizardEnabledRaw unmarshals one server entry; invalid JSON counts as enabled.
func DNSServerWizardEnabledRaw(raw json.RawMessage) bool {
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return true
	}
	return dnsServerEnabledInWizard(m)
}

// ValidateDNSModel checks tags, final, and rules before save / preview.
func ValidateDNSModel(model *wizardmodels.WizardModel) error {
	if model == nil {
		return fmt.Errorf("model is nil")
	}
	if len(model.DNSServers) == 0 {
		return fmt.Errorf("at least one DNS server is required")
	}
	tags := make(map[string]struct{})
	enabledTags := make(map[string]struct{})
	enabledCount := 0
	for i, raw := range model.DNSServers {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("DNS server %d: invalid JSON: %w", i+1, err)
		}
		tag, _ := m["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return fmt.Errorf("DNS server %d: missing tag", i+1)
		}
		if _, dup := tags[tag]; dup {
			return fmt.Errorf("duplicate DNS tag: %s", tag)
		}
		tags[tag] = struct{}{}
		if dnsServerEnabledInWizard(m) {
			enabledTags[tag] = struct{}{}
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return fmt.Errorf("at least one enabled DNS server is required")
	}
	if model.DNSFinal != "" {
		if _, ok := enabledTags[model.DNSFinal]; !ok {
			return fmt.Errorf("dns.final %q must be an enabled server tag", model.DNSFinal)
		}
	}
	if model.DefaultDomainResolver != "" && !model.DefaultDomainResolverUnset {
		if _, ok := enabledTags[model.DefaultDomainResolver]; !ok {
			return fmt.Errorf("default domain resolver %q must be an enabled server tag", model.DefaultDomainResolver)
		}
	}
	rules, err := build.ParseDNSRulesText(model.DNSRulesText)
	if err != nil {
		return err
	}
	for i, r := range rules {
		rm, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		srvVal, ok := rm["server"]
		if !ok || srvVal == nil {
			continue
		}
		srv := dnsRuleServerTagString(srvVal)
		if srv == "" {
			continue
		}
		if _, ok := enabledTags[srv]; !ok {
			return fmt.Errorf("dns rule %d: server %q is missing or disabled", i+1, srv)
		}
	}
	return nil
}

func dnsRuleServerTagString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// DNSEnabledTagOptions returns tags for enabled servers in list order.
// Включает: (1) tagи enabled legacy DNS servers из model.DNSServers,
// (2) tagи bundled DNS-серверов от активных preset-ref'ов (читается через
// PresetBundledDNSTags helper). Это даёт юзеру выбрать `final` /
// `default_domain_resolver` в том числе из bundled DNS-серверов preset'а
// (например `ru-direct:yandex_udp`).
//
// Выпадающие dns.final и route.default_domain_resolver показывают только эти теги: строка из скелета
// без галочки «в конфиг» в список не попадает; при включённой галочке тело может браться из dns_options (см. mergeLockedRow).
func DNSEnabledTagOptions(model *wizardmodels.WizardModel) []string {
	if model == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	addTag := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	for _, raw := range model.DNSServers {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if !dnsServerEnabledInWizard(m) {
			continue
		}
		tag, _ := m["tag"].(string)
		addTag(tag)
	}
	// Bundled DNS servers from active preset-refs.
	for _, tag := range PresetBundledDNSTags(model) {
		addTag(tag)
	}
	return out
}
