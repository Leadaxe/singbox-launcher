// File preset_bundled_dns.go — helpers для извлечения bundled DNS-серверов
// от active preset-ref правил.
//
// Используется UI:
//   - DNS tab: рендеринг read-only "From active presets" секции
//   - Final / Default resolver picker'ы: bundled tag'и попадают в options
//     рядом с legacy DNS-серверами
package business

import (
	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// PresetBundledDNSTags — список tag'ов (уже с preset-prefix) bundled DNS-серверов
// от всех **активных** preset-ref правил. Учитывает filter через @dns_server var
// (ExpandPreset.DNSServers — только used).
//
// Возвращает в порядке: для каждого preset-ref (в порядке PresetRefs) — теги
// его bundled DNS, в порядке dns_servers[] declaration.
func PresetBundledDNSTags(model *wizardmodels.WizardModel) []string {
	if model == nil || model.TemplateData == nil {
		return nil
	}
	presetByID := make(map[string]*wizardtemplate.Preset, len(model.TemplateData.Presets))
	for i := range model.TemplateData.Presets {
		presetByID[model.TemplateData.Presets[i].ID] = &model.TemplateData.Presets[i]
	}
	var out []string
	for _, pr := range model.PresetRefs {
		if pr == nil || !pr.Enabled {
			continue
		}
		tpl := presetByID[pr.Ref]
		if tpl == nil {
			continue
		}
		frags, _, ok := build.ExpandPreset(tpl, pr.Vars)
		if !ok {
			continue
		}
		for _, ds := range frags.DNSServers {
			if tag, ok := ds["tag"].(string); ok && tag != "" {
				out = append(out, tag)
			}
		}
	}
	return out
}
