// File sync_dns.go (SPEC 056-R-N) — единая точка lifecycle entries kind=preset
// в state.dns_options.{servers,rules}[].
//
// **Инвариант: memory == disk.** Никакого runtime materialization preset DNS
// в build pipeline — entries должны быть в state.DNS ровно того набора что
// эмитится в финальный config.
//
// Вызывается:
//   - на load (после parseCurrent, idempotent)
//   - на каждый toggle preset enable/disable в Rules tab UI
//   - перед Save (defensive — гарантия что entries синхронизированы)
//
// Семантика:
//
//	Для каждого state.Rules[Kind=preset && Enabled=true]:
//	  - ensure entries {Kind:preset, Ref:"<id>:<local_tag>"} в DNS.Servers
//	    по числу template.presets[id].DNSServers[].
//	    Default Enabled=true. Если entry уже была — preserve её Enabled.
//	  - ensure entry {Kind:preset, Ref:"<id>"} в DNS.Rules если у preset'а
//	    определён dns_rule. Новая запись встаёт ПО ОСИ ПРАВИЛ (SPEC 171,
//	    D-135): перед первым слотом пресета с бо́льшим номером маршрутного
//	    правила, иначе в конец; пользовательские правила не двигаются.
//
//	Drop entries с Ref на disabled/missing preset (auto-cleanup):
//	  - DNS.Servers где Ref начинается с "<disabled_preset_id>:"
//	  - DNS.Rules где Ref == "<disabled_preset_id>"
//
// Idempotency: повторный вызов с тем же состоянием не меняет ничего.
package state

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
	nums := presetRuleNums(rules)
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
		// SPEC 171 (D-135): место нового слота — по оси маршрутных правил,
		// а не хвост списка. Иначе поздний дефолт `tunnel-ingress` (940)
		// вставал после `fakeip` (1130), и DNS-reject закрытых пиров не
		// срабатывал за «A/AAAA → fakeip».
		at := PresetDNSSlotIndex(len(keptRules), nums[r.Ref], func(i int) (int, bool) {
			if keptRules[i].Kind != DNSRuleKindPreset {
				return 0, false
			}
			n, ok := nums[keptRules[i].Ref]
			return n, ok
		})
		keptRules = insertDNSRule(keptRules, at, DNSRule{
			Kind:    DNSRuleKindPreset,
			Ref:     r.Ref,
			Enabled: true,
		})
	}
	dns.Rules = keptRules
}

// PresetDNSSlotIndex — место нового DNS-слота пресета с номером num среди n
// уже стоящих слотов (SPEC 171, D-135, TEMPLATE_LANG §6.10): индекс первого
// слота пресета, чей номер БОЛЬШЕ num; такого нет — n (в конец). slotNum(i)
// возвращает номер слота i и ok=false у слота без номера (пользовательское
// DNS-правило) — такие слоты не учитываются и не двигаются.
//
// Номер — `num` маршрутного правила пресета (`rules[]`), DNS-оси нет: ось
// даёт только место появления, дальше слот живёт порядком пользователя
// (SPEC 062). Равные номера — новый слот после (стабильно). Общая функция
// для состояния (SyncDNSOptionsWithActivePresets) и модели визарда
// (ReconcileDNSRuleOrder), чтобы место считалось одинаково.
func PresetDNSSlotIndex(n int, num int, slotNum func(i int) (int, bool)) int {
	for i := 0; i < n; i++ {
		if got, ok := slotNum(i); ok && got > num {
			return i
		}
	}
	return n
}

// presetRuleNums — номер оси маршрутного правила каждого preset-ref'а
// (неразмеченное — DefaultRuleNum, как при сортировке).
func presetRuleNums(rules []Rule) map[string]int {
	nums := make(map[string]int)
	for _, r := range rules {
		if r.Kind != RuleKindPreset || r.Ref == "" {
			continue
		}
		if _, seen := nums[r.Ref]; seen {
			continue
		}
		nums[r.Ref] = ruleNum(r)
	}
	return nums
}

func insertDNSRule(rules []DNSRule, at int, r DNSRule) []DNSRule {
	if at < 0 || at >= len(rules) {
		return append(rules, r)
	}
	rules = append(rules, DNSRule{})
	copy(rules[at+1:], rules[at:])
	rules[at] = r
	return rules
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
