// File preset_ref_srs.go — извлечение SRS-entries из preset-ref'а для UI tile
// (download cloud button) и build pipeline (remote → local conversion).
//
// Логика: проходим preset.rule_set[] фильтруя по if/if_or против текущих
// varsValues; для каждого `type: remote` берём URL + генерируем content-addressed
// tag через dialogs.SRSTagFromURL (как для user srs правил).
//
// Возвращаемые SRSEntry структуры совместимы с services.AllSRSDownloadedForEntries
// и services.DownloadSRSGroup — переиспользуем существующий download stack.
package tabs

import (
	"strings"

	"singbox-launcher/core/services"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/ui/configurator/dialogs"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// presetRefSRSEntries — список remote rule_set entries активных под текущими
// vars (с учётом if/if_or filter). Используется для:
//   - проверки "все ли SRS скачаны" → определить нужно ли облачко (☁) в tile
//   - триггера скачивания при клике на облачко
//   - build pipeline'а (resolve remote → local path)
//
// Если preset не содержит ни одного remote rule_set (например private-ips-direct),
// возвращает nil — облачко не показывается.
func presetRefSRSEntries(pr *wizardmodels.PresetRefState, tpl *wizardtemplate.Preset) []services.SRSEntry {
	if pr == nil || tpl == nil {
		return nil
	}
	// Build effective varsMap (working = state vars + template defaults).
	vars := make(map[string]string, len(tpl.Vars))
	for _, v := range tpl.Vars {
		if val, ok := pr.Vars[v.Name]; ok && val != "" {
			vars[v.Name] = val
		} else {
			vars[v.Name] = v.Default
		}
	}
	var out []services.SRSEntry
	for _, rs := range tpl.RuleSet {
		if rs.Type != "remote" || rs.URL == "" {
			continue
		}
		if !ifActiveForRuleSet(rs, vars) {
			continue
		}
		tag := dialogs.SRSTagFromURL(rs.URL)
		if tag == "" {
			continue
		}
		out = append(out, services.SRSEntry{Tag: tag, URL: rs.URL})
	}
	return out
}

// ifActiveForRuleSet — копия логики evalIf из core/build/preset_expand.go,
// дублирована тут чтобы избежать import cycle (UI → core/build → UI...).
//
// Когда core/build/preset_expand.go вынесет evalIf в публичный API — заменить.
func ifActiveForRuleSet(rs wizardtemplate.PresetRuleSet, vars map[string]string) bool {
	for _, name := range rs.If {
		if !strings.EqualFold(vars[name], "true") {
			return false
		}
	}
	if len(rs.IfOr) > 0 {
		any := false
		for _, name := range rs.IfOr {
			if strings.EqualFold(vars[name], "true") {
				any = true
				break
			}
		}
		if !any {
			return false
		}
	}
	return true
}
