// File preset_lite.go (SPEC 056-R-N) — адаптер template.Preset к state.PresetLite.
//
// state.PresetLite — минимальный интерфейс нужный для SyncDNSOptionsWithActivePresets,
// определён в core/state/state. template импортирует v6 (direction: high-level
// template → low-level state schema) — циклов нет.
package template

import "singbox-launcher/core/state"

// PresetID — implements state.PresetLite.
func (p *Preset) PresetID() string { return p.ID }

// PresetDNSServerTags — возвращает локальные tag'и всех DNS-серверов preset'а
// (в порядке определения в template — этот порядок переносится в state).
//
// Не применяет if/if_or фильтрацию — это пред-resolve декларативного набора
// (sync не знает значений vars; build pipeline фильтрует через ExpandPreset).
// Если preset с if_or='ipv4_enabled' приносит 3 DNS-сервера, а юзер выключил
// ipv4 — entries всё равно в state есть, но при build пропускаются. Это OK
// потому что toggle ipv4 в settings не должен дёргать sync.
func (p *Preset) PresetDNSServerTags() []string {
	out := make([]string, 0, len(p.DNSServers))
	for _, ds := range p.DNSServers {
		if ds.Tag == "" {
			continue
		}
		out = append(out, ds.Tag)
	}
	return out
}

// PresetHasDNSRule — true если preset определяет dns_rule.
func (p *Preset) PresetHasDNSRule() bool {
	return p.DNSRule != nil
}

// PresetLiteMap — собирает map[id]→state.PresetLite из []Preset для передачи в
// state.SyncDNSOptionsWithActivePresets.
//
// Использование:
//
//	m := template.PresetLiteMap(td.Presets)
//	state.SyncDNSOptionsWithActivePresets(state.Rules, &state.DNS, m)
func PresetLiteMap(presets []Preset) map[string]state.PresetLite {
	out := make(map[string]state.PresetLite, len(presets))
	for i := range presets {
		out[presets[i].ID] = &presets[i]
	}
	return out
}
