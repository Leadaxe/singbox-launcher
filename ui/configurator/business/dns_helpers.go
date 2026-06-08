package business

// dns_helpers.go — единый дом для парсеров template DNS-секции (dns_options).
//
// SPEC 070 Stage B: раньше эти helper'ы были продублированы между business
// (create_config.go) и presentation (preset_ref_helpers.go) — копия в
// presentation существовала только чтобы не тянуть presentation в business
// (import cycle). presentation уже импортирует business, поэтому достаточно
// одного экспортированного дома здесь; обе стороны зовут отсюда.

import (
	"encoding/json"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
)

// ParseTemplateDNSDefaults — то же что core_service.parseTemplateDNSDefaultsFromTD,
// продублировано в business чтобы не импортировать core (избегаем import cycle).
// Используется BuildPreviewConfig: preview tab визарда должен показывать тот же
// config, что Save/Rebuild — материализованную template DNS library из dns_options.
//
// Возвращает nil если td nil / нет dns_options / парс не удался — caller
// (MergePresetsIntoDNS) тогда просто не материализует, остальные DNS-сервера
// (template config.dns.servers + bundled + extras) работают как раньше.
func ParseTemplateDNSDefaults(td *wizardtemplate.TemplateData) []build.TemplateDNSServer {
	if td == nil || len(td.DNSOptionsRaw) == 0 {
		return nil
	}
	var dnsOpt struct {
		Servers []json.RawMessage `json:"servers"`
	}
	if err := json.Unmarshal(td.DNSOptionsRaw, &dnsOpt); err != nil {
		return nil
	}
	return build.ParseTemplateDNSDefaults(dnsOpt.Servers)
}

// ExtractTemplateDNSTags — выдаёт set tag'ов template-defined DNS-серверов из
// template.DNSOptionsRaw (используется для split'а на overrides vs extras при
// save v6, и для order-aware DNS sync в preview).
func ExtractTemplateDNSTags(td *wizardtemplate.TemplateData) map[string]bool {
	if td == nil || len(td.DNSOptionsRaw) == 0 {
		return nil
	}
	var dnsOpt struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(td.DNSOptionsRaw, &dnsOpt); err != nil {
		return nil
	}
	out := make(map[string]bool, len(dnsOpt.Servers))
	for _, s := range dnsOpt.Servers {
		if tag, ok := s["tag"].(string); ok && tag != "" {
			out[tag] = true
		}
	}
	return out
}
