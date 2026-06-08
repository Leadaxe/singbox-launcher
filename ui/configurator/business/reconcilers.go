package business

import (
	"encoding/json"
	"strings"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// -----------------------------------------------------------------------------
// Reconcile servers: единый template-list (dns_options) → in-memory model.DNSServers
// + orphan saved tags
// -----------------------------------------------------------------------------
//
// SPEC unify: больше нет config.dns.servers (он пуст) — все DNS-серверы
// живут в template.dns_options.servers[]. Раньше reconcileDNSServers
// учитывал tag-collision между config.dns.servers и dns_options.servers
// (mergeLockedRow); теперь этот сценарий невозможен — функция упростилась.

func reconcileDNSServers(model *wizardmodels.WizardModel, dnsObj map[string]interface{}, optsMap map[string]json.RawMessage) {
	_ = dnsObj // dnsObj.servers пуст в новой схеме; параметр оставлен для backward-compat сигнатуры.

	saved := append([]json.RawMessage(nil), model.DNSServers...)
	byTag, order := indexDNSStateByTag(saved)

	optsByTag, optsOrder := dnsOptionsServersByTag(optsMap)

	var out []json.RawMessage
	seen := make(map[string]struct{})

	// Pass 1: template dns_options порядок (required entries приходят первыми
	// в template'е и сохраняют этот порядок здесь).
	for _, tag := range optsOrder {
		if _, done := seen[tag]; done {
			continue
		}
		if raw, ok := byTag[tag]; ok && len(raw) > 0 {
			out = append(out, raw)
		} else if opt := optsByTag[tag]; opt != nil {
			if b, err := json.Marshal(opt); err == nil {
				out = append(out, json.RawMessage(b))
			}
		}
		seen[tag] = struct{}{}
	}

	// Pass 2: orphan saved tags (user-added servers с tag'ом которого нет в template).
	for _, tag := range order {
		if _, ok := seen[tag]; ok {
			continue
		}
		if raw, ok := byTag[tag]; ok && len(raw) > 0 {
			out = append(out, raw)
			seen[tag] = struct{}{}
		}
	}

	model.DNSServers = out
}

func indexDNSStateByTag(servers []json.RawMessage) (byTag map[string]json.RawMessage, order []string) {
	byTag = make(map[string]json.RawMessage)
	for _, raw := range servers {
		tag := tagFromServerJSON(raw)
		if tag == "" {
			continue
		}
		if _, ok := byTag[tag]; !ok {
			order = append(order, tag)
		}
		byTag[tag] = raw
	}
	return byTag, order
}

func dnsOptionsServersByTag(optsMap map[string]json.RawMessage) (byTag map[string]map[string]interface{}, order []string) {
	byTag = make(map[string]map[string]interface{})
	if optsMap == nil {
		return byTag, order
	}
	raw, ok := optsMap["servers"]
	if !ok || len(raw) == 0 {
		return byTag, order
	}
	var arr []interface{}
	if json.Unmarshal(raw, &arr) != nil {
		return byTag, order
	}
	for _, s := range arr {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		tag := strings.TrimSpace(jsonString(m["tag"]))
		if tag == "" {
			continue
		}
		if _, has := byTag[tag]; !has {
			byTag[tag] = m
			order = append(order, tag)
		}
	}
	return byTag, order
}

// mergeLockedRow — УДАЛЕНА в SPEC unify. Tag-collision между config.dns.servers
// и dns_options.servers больше невозможна (config.dns.servers пуст).

// -----------------------------------------------------------------------------
// Local resolver from config.dns (if missing after reconcile)
// -----------------------------------------------------------------------------

func prependMissingLocalServers(model *wizardmodels.WizardModel, dnsObj map[string]interface{}) {
	if model == nil || dnsObj == nil {
		return
	}
	have := make(map[string]struct{})
	for _, raw := range model.DNSServers {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if t, ok := m["tag"].(string); ok && t != "" {
			have[t] = struct{}{}
		}
	}
	arr, ok := dnsObj["servers"].([]interface{})
	if !ok {
		return
	}
	var prepend []json.RawMessage
	for _, s := range arr {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		tag, _ := m["tag"].(string)
		if typ != "local" || tag == "" {
			continue
		}
		if _, exists := have[tag]; exists {
			continue
		}
		b, err := json.Marshal(m)
		if err != nil {
			continue
		}
		prepend = append(prepend, json.RawMessage(b))
		have[tag] = struct{}{}
	}
	if len(prepend) == 0 {
		return
	}
	model.DNSServers = append(prepend, model.DNSServers...)
}
