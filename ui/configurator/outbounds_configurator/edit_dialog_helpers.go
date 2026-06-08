// edit_dialog_helpers.go holds standalone helpers for the Add/Edit outbound
// dialog: template-var dropdown construction, value/label lookup, and USER-patch
// / referenced-entry body manipulation. These are pure top-level helpers split
// out of edit_dialog.go; the dialog builder itself stays in edit_dialog.go.
package outbounds_configurator

import (
	"singbox-launcher/core/config"
)

// templateVarChoices строит (labels, labelToValue) для dropdown'а на основе
// template var. Семантика:
//   - labels[0] = "@varname" placeholder — позволяет выбрать «inherit from
//     Settings» (substituter резолвит в текущее значение state.vars[var]).
//   - labels[1..] = OptionTitles если есть, иначе raw values (mirror того что
//     Settings tab показывает).
//   - currentValue добавляется в конец списка если не matchится ни с placeholder
//     ни с template options — preserve юзерское custom value (например, юзер
//     раньше ввёл нестандартное "7m" — не теряем).
//   - labelToValue: label → raw value (для save mapping).
func templateVarChoices(editPresenter OutboundEditPresenter, varName, currentValue string) ([]string, map[string]string) {
	placeholder := "@" + varName
	labels := []string{placeholder}
	labelToValue := map[string]string{placeholder: placeholder}

	if editPresenter != nil {
		if m := editPresenter.Model(); m != nil && m.TemplateData != nil {
			for _, v := range m.TemplateData.Vars {
				if v.Name != varName {
					continue
				}
				for i, opt := range v.Options {
					label := opt
					if i < len(v.OptionTitles) && v.OptionTitles[i] != "" {
						label = v.OptionTitles[i]
					}
					labels = append(labels, label)
					labelToValue[label] = opt
				}
				break
			}
		}
	}

	// Preserve custom currentValue если не среди известных options/placeholder.
	if currentValue != "" {
		found := false
		for _, val := range labelToValue {
			if val == currentValue {
				found = true
				break
			}
		}
		if !found {
			labels = append(labels, currentValue)
			labelToValue[currentValue] = currentValue
		}
	}
	return labels, labelToValue
}

// labelForValue ищет label соответствующий значению value в map. Возвращает
// первый matching label или пустую строку.
func labelForValue(labelToValue map[string]string, value string) string {
	for label, val := range labelToValue {
		if val == value {
			return label
		}
	}
	return ""
}

// filterOutUserPatch returns Updates with USER patch entry removed (preset
// patches kept). Используется в diff computation: merged_base = template body +
// active preset patches (БЕЗ USER patch — он и есть результат текущего edit).
func filterOutUserPatch(updates []config.OutboundUpdate) []config.OutboundUpdate {
	if len(updates) == 0 {
		return nil
	}
	out := make([]config.OutboundUpdate, 0, len(updates))
	for _, u := range updates {
		if u.Ref == config.RefUser {
			continue
		}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stripDirectBodyForReferenced — referenced entries (ref != "") хранят thin
// shape: только tag + ref + updates. Body fields обнуляются (live из template/preset).
func stripDirectBodyForReferenced(cfg *config.OutboundConfig) {
	if cfg == nil || cfg.Ref == "" {
		return
	}
	cfg.Type = ""
	cfg.Options = nil
	cfg.Filters = nil
	cfg.AddOutbounds = nil
	cfg.PreferredDefault = nil
	cfg.Comment = ""
}
