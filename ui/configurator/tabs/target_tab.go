package tabs

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// CreateTargetTab — шаг 0 визарда (SPEC 097): для какой машины готовим конфиг.
//
// Стоит ПЕРЕД остальными вкладками намеренно: таргет определяет, какие поля
// вообще имеет смысл показывать (платформа целевой машины фильтрует vars и
// params), поэтому его надо знать до того, как визард нарисует Settings.
//
// Вкладка не спрашивает host/port/secret удалённой машины — это настройки
// СОЕДИНЕНИЯ, они живут в gear-диалоге вкладки Servers (SPEC 064). Здесь
// только то, что нужно для ГЕНЕРАЦИИ конфига.
func CreateTargetTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	model := presenter.Model()
	box := container.NewVBox()

	refresh := func() {
		box.RemoveAll()
		tgt := model.Target.Normalized()

		// Вкладка существует только в удалённом режиме (см. createWizardTabs),
		// поэтому ветвления по цели здесь больше нет.
		intro := widget.NewLabel(locale.T("This config is for another machine and differs from a local one: no Clash API (the lxd daemon manages the core there) and no system proxy. Gateway-specific behavior — LAN interfaces, wide proxy listen, process matching off — is controlled by Gateway mode on the Settings tab."))
		intro.Wrapping = fyne.TextWrapWord
		box.Add(intro)

		hint := widget.NewLabel(locale.T("On Save the config is written to bin/remote-config.json for deployment under the lxd daemon — the local bin/config.json is never touched. State lives in bin/wizard_states/remote/state.json."))
		hint.Wrapping = fyne.TextWrapWord
		hint.Importance = widget.LowImportance
		box.Add(hint)

		// --- Платформа целевой машины: только для remote.
		// Для local платформа всегда runtime'а — выбирать нечего.
		if tgt.IsRemote() {
			box.Add(widget.NewSeparator())

			platformSelect := widget.NewSelect([]string{"linux", "darwin", "windows"}, nil)
			platformSelect.SetSelected(tgt.GOOS)
			platformSelect.OnChanged = func(goos string) {
				if goos == "" || goos == model.Target.Normalized().GOOS {
					return
				}
				presenter.SetTargetPlatform(goos, model.Target.Normalized().GOARCH)
			}

			archSelect := widget.NewSelect([]string{"amd64", "arm64", "arm", "386"}, nil)
			arch := tgt.GOARCH
			if arch == "" {
				arch = runtime.GOARCH
			}
			archSelect.SetSelected(arch)
			archSelect.OnChanged = func(goarch string) {
				if goarch == "" || goarch == model.Target.Normalized().GOARCH {
					return
				}
				presenter.SetTargetPlatform(model.Target.Normalized().GOOS, goarch)
			}

			box.Add(widget.NewForm(
				widget.NewFormItem(locale.T("Target OS"), platformSelect),
				widget.NewFormItem(locale.T("Target architecture"), archSelect),
			))

			note := widget.NewLabel(locale.T("Set the TUN interface name explicitly — the core default cannot be relied on, and on a router the name is needed for firewall rules and routes."))
			note.Wrapping = fyne.TextWrapWord
			note.Importance = widget.WarningImportance
			box.Add(note)
		}

		// --- Роль машины: vars с wizard_ui:"target" (gateway_mode и его
		// LAN-интерфейсы). Живут здесь, а не в Settings, потому что от них
		// зависят default_value других полей (proxy_in_listen), а дефолты
		// резолвятся однопроходно в порядке объявления — переключатель
		// обязан быть виден раньше зависимых значений.
		//
		// Раздача в LAN настраивается только для удалённой машины (роутера):
		// на локальной эти поля не нужны — решение владельца. Технически
		// gateway возможен и здесь (Internet Sharing), поэтому сами
		// переменные остаются в шаблоне, скрыт только их показ.
		if td := model.TemplateData; td != nil && tgt.IsRemote() {
			targetVars := wizardtemplate.TargetTabVars(td.Vars)
			if len(targetVars) > 0 {
				box.Add(widget.NewSeparator())
				vi := wizardtemplate.VarIndex(td.Vars)
				resolved := wizardtemplate.ResolveTemplateVarsFor(td.Vars, model.SettingsVars, td.RawTemplate, tgt)
				gs := presenter.GUIState()
				for _, vd := range targetVars {
					rowEnabled := wizardtemplate.VarUISatisfiedFor(vd, vi, resolved, tgt)
					box.Add(buildSettingsVarRow(presenter, model, td, vd,
						wizardtemplate.VarDisplayTitle(vd), wizardtemplate.VarDisplayTooltip(vd), rowEnabled, gs))
				}
			}
		}

		box.Refresh()
	}

	// Состав вкладки зависит от таргета (поля платформы появляются только для
	// remote), поэтому презентер дёргает refresh после каждой смены — иначе
	// выбор "Remote machine" оставлял бы local-подсказку и пустой экран без
	// выбора ОС.
	if gs := presenter.GUIState(); gs != nil {
		gs.RefreshTargetTabFromModel = refresh
	}
	refresh()
	return container.NewVScroll(container.NewPadded(box))
}
