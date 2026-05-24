// File sync_dns.go (SPEC 056-R-N) — единая точка lifecycle entries kind=preset
// в state.dns_options.{servers,rules}[].
//
// **Инвариант: memory == disk.** Никакого runtime materialization preset DNS
// в build pipeline — entries должны быть в state.DNS ровно того набора что
// эмитится в финальный config.
//
// Вызывается:
//   - на load (после parseV6, idempotent)
//   - на каждый toggle preset enable/disable в Rules tab UI
//   - перед Save (defensive — гарантия что entries синхронизированы)
//
// Семантика:
//
//   Для каждого state.Rules[Kind=preset && Enabled=true]:
//     - ensure entries {Kind:preset, Ref:"<id>:<local_tag>"} в DNS.Servers
//       по числу template.presets[id].DNSServers[].
//       Default Enabled=true. Если entry уже была — preserve её Enabled.
//     - ensure entry {Kind:preset, Ref:"<id>"} в DNS.Rules если у preset'а
//       определён dns_rule.
//
//   Drop entries с Ref на disabled/missing preset (auto-cleanup):
//     - DNS.Servers где Ref начинается с "<disabled_preset_id>:"
//     - DNS.Rules где Ref == "<disabled_preset_id>"
//
// Idempotency: повторный вызов с тем же состоянием не меняет ничего.
package v6

import (
	"strings"
)

// PresetLite — минимальный интерфейс template-preset'а нужный sync'у.
// Принимает любой тип удовлетворяющий ему — избавляет от циклической зависимости
// c core/template.
type PresetLite interface {
	PresetID() string
	PresetDNSServerTags() []string
	PresetHasDNSRule() bool
}

// SyncDNSOptionsWithActivePresets — синхронизирует kind=preset entries в DNS
// с текущим набором active preset-ref'ов в state.Rules[].
//
// Параметры:
//   - rules — state.Rules[] (текущий набор правил)
//   - dns   — pointer на state.DNS (мутируется)
//   - presetByID — map[preset_id] → PresetLite (резолв template-данных)
//
// Idempotent. Безопасно вызывать многократно.
func SyncDNSOptionsWithActivePresets(
	rules []Rule,
	dns *DNSOptions,
	presetByID map[string]PresetLite,
) {
	if dns == nil {
		return
	}

	// 1. Build sets: active preset IDs + expected (preset_id, local_tag) pairs.
	activePresetIDs := make(map[string]bool)
	expectedServerRefs := make(map[string]bool) // "<id>:<local_tag>"
	expectedRuleRefs := make(map[string]bool)   // "<id>"
	for _, r := range rules {
		if r.Kind != RuleKindPreset || !r.Enabled || r.Ref == "" {
			continue
		}
		p, ok := presetByID[r.Ref]
		if !ok {
			// Preset отсутствует в template (broken ref) — пропускаем; entries
			// которые могут быть в DNS для этого ref'а будут удалены ниже.
			continue
		}
		activePresetIDs[r.Ref] = true
		for _, localTag := range p.PresetDNSServerTags() {
			expectedServerRefs[r.Ref+":"+localTag] = true
		}
		if p.PresetHasDNSRule() {
			expectedRuleRefs[r.Ref] = true
		}
	}

	// 2. Filter Servers — drop preset-entries с ref на disabled/missing preset.
	//    Preserve enabled state для preset-entries которые остаются.
	keptServers := make([]DNSServer, 0, len(dns.Servers))
	existingPresetEnabled := make(map[string]bool, len(dns.Servers))
	for _, s := range dns.Servers {
		if s.Kind != DNSServerKindPreset {
			keptServers = append(keptServers, s)
			continue
		}
		if !expectedServerRefs[s.Ref] {
			continue // disabled/missing preset → drop
		}
		existingPresetEnabled[s.Ref] = s.Enabled
		keptServers = append(keptServers, s)
	}

	// 3. Add missing preset entries (в порядке active presets / DNS server tags).
	for _, r := range rules {
		if r.Kind != RuleKindPreset || !r.Enabled || r.Ref == "" {
			continue
		}
		p, ok := presetByID[r.Ref]
		if !ok {
			continue
		}
		for _, localTag := range p.PresetDNSServerTags() {
			ref := r.Ref + ":" + localTag
			if _, alreadyPresent := existingPresetEnabled[ref]; alreadyPresent {
				continue
			}
			// Уже после фильтра убедимся что entry не было — добавляем.
			if findServerRefIn(keptServers, ref) >= 0 {
				continue
			}
			keptServers = append(keptServers, DNSServer{
				Kind:    DNSServerKindPreset,
				Ref:     ref,
				Enabled: true, // дефолт — preset включает все свои DNS-серверы
			})
		}
	}
	dns.Servers = keptServers

	// 4. То же для Rules.
	keptRules := make([]DNSRule, 0, len(dns.Rules))
	existingPresetRuleEnabled := make(map[string]bool, len(dns.Rules))
	for _, r := range dns.Rules {
		if r.Kind != DNSRuleKindPreset {
			keptRules = append(keptRules, r)
			continue
		}
		if !expectedRuleRefs[r.Ref] {
			continue
		}
		existingPresetRuleEnabled[r.Ref] = r.Enabled
		keptRules = append(keptRules, r)
	}
	for _, r := range rules {
		if r.Kind != RuleKindPreset || !r.Enabled || r.Ref == "" {
			continue
		}
		if !expectedRuleRefs[r.Ref] {
			continue
		}
		if _, alreadyPresent := existingPresetRuleEnabled[r.Ref]; alreadyPresent {
			continue
		}
		if findRuleRefIn(keptRules, r.Ref) >= 0 {
			continue
		}
		keptRules = append(keptRules, DNSRule{
			Kind:    DNSRuleKindPreset,
			Ref:     r.Ref,
			Enabled: true,
		})
	}
	dns.Rules = keptRules
}

// PresetIDFromServerRef — извлекает <preset_id> из "<preset_id>:<local_tag>".
// Возвращает "" если ref в неверном формате.
func PresetIDFromServerRef(ref string) string {
	if i := strings.Index(ref, ":"); i > 0 {
		return ref[:i]
	}
	return ""
}

// LocalTagFromServerRef — извлекает <local_tag> из "<preset_id>:<local_tag>".
func LocalTagFromServerRef(ref string) string {
	if i := strings.Index(ref, ":"); i > 0 && i+1 < len(ref) {
		return ref[i+1:]
	}
	return ""
}

func findServerRefIn(servers []DNSServer, ref string) int {
	for i, s := range servers {
		if s.Kind == DNSServerKindPreset && s.Ref == ref {
			return i
		}
	}
	return -1
}

func findRuleRefIn(rules []DNSRule, ref string) int {
	for i, r := range rules {
		if r.Kind == DNSRuleKindPreset && r.Ref == ref {
			return i
		}
	}
	return -1
}
