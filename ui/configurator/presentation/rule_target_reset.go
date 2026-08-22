// File rule_target_reset.go — сброс правил, целящихся не в Направление
// (SPEC 108, S5).
//
// До SPEC 108 целью правила могла быть локальная группа подписки
// (`AL:select`): она попадала в список целей наравне с Направлениями. Такая
// цель живёт по чужим правилам жизненного цикла — исчезает вместе с
// подпиской, переименовывается вместе с её префиксом, — и правило молча
// указывает в никуда. С SPEC 108 группа подписки это группировка и сахар к
// Направлению, а не самостоятельная цель.
//
// Почему direct, а не выключение правила: выключенное правило пользователь
// не заметит и решит, что маршрутизация сломалась молча. Правило на direct
// работает предсказуемо и видно в списке.
//
// Почему при загрузке, а не на сборке: пользователь должен видеть новое
// состояние в списке правил сразу, а не гадать, почему конфиг ведёт себя
// иначе, чем показывает интерфейс.
//
// Почему ЗДЕСЬ, а не в core/state: множество допустимых целей — это не
// только `connections.direction_outbounds`. В него входят теги шаблона
// (`block-out`, `direct-out`) и теги активных пресетов, а о шаблоне
// состояние ничего не знает. Сброс на неполном множестве убил бы живые
// правила на `proxy-out` — ровно то, чего эта правка избегает.
package presentation

import (
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// resetForeignRuleTargets заменяет на direct цели правил, которых нет среди
// допустимых. Вызывается после восстановления правил и preset-ref'ов, когда
// множество целей уже полное.
func (p *WizardPresenter) resetForeignRuleTargets() {
	if p == nil || p.model == nil {
		return
	}
	known := make(map[string]bool)
	for _, tag := range wizardbusiness.EnsureDefaultAvailableOutbounds(
		wizardbusiness.GetAvailableOutbounds(p.model)) {
		known[tag] = true
	}
	if len(known) == 0 {
		// Шаблон не загрузился — сбрасывать не на чем и не за что.
		return
	}

	for _, rs := range p.model.CustomRules {
		if rs == nil {
			continue
		}
		target := rs.SelectedOutbound
		if target == "" || known[target] {
			continue
		}
		debuglog.WarnLog("SPEC 108: правило %q целилось в %q — такой цели нет, сброшено на %q",
			rs.Rule.Label, target, wizardmodels.DefaultOutboundTag)
		rs.SelectedOutbound = wizardmodels.DefaultOutboundTag
	}

	// route.final: осиротевшая цель здесь увела бы ВЕСЬ трафик по умолчанию
	// в никуда — это хуже, чем одно правило.
	if f := p.model.SelectedFinalOutbound; f != "" && !known[f] {
		debuglog.WarnLog("SPEC 108: route.final целился в %q — такой цели нет, сброшен на %q",
			f, wizardmodels.DefaultOutboundTag)
		p.model.SelectedFinalOutbound = wizardmodels.DefaultOutboundTag
		if p.model.SettingsVars != nil {
			p.model.SettingsVars["route_final"] = wizardmodels.DefaultOutboundTag
		}
	}
}
