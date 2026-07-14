// edit_dialog_helpers.go holds standalone helpers for the Add/Edit outbound
// dialog: template-var dropdown construction, value/label lookup, and USER-patch
// / referenced-entry body manipulation. These are pure top-level helpers split
// out of edit_dialog.go; the dialog builder itself stays in edit_dialog.go.
package outbounds_configurator

import (
	"fmt"
	"strconv"
	"strings"

	"singbox-launcher/core/config"
)

// stickyHashKeys — компоненты sticky_hash в порядке отображения в UI.
var stickyHashKeys = []string{"process", "domain", "source_ip", "dest_ip", "dest_port"}

// balancerFormState — плоское представление UI-полей балансировки (SPEC 088),
// отвязанное от Fyne-виджетов ради тестируемости чтения/записи Options.
type balancerFormState struct {
	RoundRobin    bool            // mode == "round_robin"
	Pool          string          // raw text (может быть пустым)
	PoolTolerance string          // raw text
	Sticky        map[string]bool // выбранные компоненты sticky_hash
}

// parseBalancerFromOptions читает mode/balancer из Options существующего
// urltest-outbound'а в balancerFormState для заполнения виджетов. "none"-sentinel
// в sticky_hash трактуется как «выкл» (ни один чекбокс не отмечен).
func parseBalancerFromOptions(opts map[string]interface{}) balancerFormState {
	st := balancerFormState{Sticky: map[string]bool{}}
	if opts == nil {
		return st
	}
	if mode, _ := opts["mode"].(string); mode == "round_robin" {
		st.RoundRobin = true
	}
	bal, ok := opts["balancer"].(map[string]interface{})
	if !ok {
		return st
	}
	if v, ok := bal["pool"]; ok {
		st.Pool = fmt.Sprintf("%v", v)
	}
	if v, ok := bal["pool_tolerance"]; ok {
		st.PoolTolerance = fmt.Sprintf("%v", v)
	}
	if sh, ok := bal["sticky_hash"].([]interface{}); ok {
		for _, item := range sh {
			if s, _ := item.(string); s != "" && s != "none" {
				st.Sticky[s] = true
			}
		}
	}
	return st
}

// buildBalancerOptions собирает mode+balancer из balancerFormState в целевой
// Options-map. round_robin → пишет mode + balancer{pool,pool_tolerance,
// sticky_hash}; иначе удаляет оба ключа (plain urltest остаётся чистым).
//
// Контракт ядра: пустой sticky_hash дропается генератором (= дефолтная
// липкость), поэтому «выкл» кодируется явным ["none"]. pool_tolerance ограничен
// uint16 (0..65535). pool/pool_tolerance с невалидным/пустым текстом опускаются.
func buildBalancerOptions(opts map[string]interface{}, st balancerFormState) {
	if opts == nil {
		return
	}
	if !st.RoundRobin {
		delete(opts, "mode")
		delete(opts, "balancer")
		return
	}
	opts["mode"] = "round_robin"
	balancer := map[string]interface{}{}
	if n, err := strconv.Atoi(strings.TrimSpace(st.Pool)); err == nil && n > 0 {
		balancer["pool"] = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(st.PoolTolerance)); err == nil && n >= 0 {
		if n > 65535 {
			n = 65535
		}
		balancer["pool_tolerance"] = n
	}
	sticky := make([]interface{}, 0, len(stickyHashKeys))
	for _, k := range stickyHashKeys {
		if st.Sticky[k] {
			sticky = append(sticky, k)
		}
	}
	if len(sticky) == 0 {
		sticky = append(sticky, "none")
	}
	balancer["sticky_hash"] = sticky
	opts["balancer"] = balancer
}

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
