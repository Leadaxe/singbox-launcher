package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// SPEC 097 — выбор машины прямо в шапке вкладки Servers.
//
// До этого сменить машину можно было только через ⚙ → окно подключения →
// область Remote → Connect: четыре шага в модальном окне ради операции,
// которую при админстве роутера делают постоянно. Плюс статичный бейдж
// «🏠 Local» не показывал, с кем идёт работа, если выбран удалённый демон.
//
// Дропдаун решает оба: Local — такой же пункт списка, как удалённые машины,
// переключение в один клик, и текущий выбор всегда виден.
//
// Выбор эфемерный (как и весь lxd-override): после перезапуска лаунчер
// возвращается к Local. Иначе Servers на старте пошёл бы к роутеру вместо
// своего ядра — неожиданно для того, кто просто открыл приложение.

// endpointPickerLocalLabel — подпись пункта «своё ядро».
func endpointPickerLocalLabel() string { return locale.T("servers.endpoint.badge_local") }

// newEndpointPicker строит дропдаун «какой машиной управляем».
//
// onChanged вызывается после смены источника — вкладка должна сбросить
// состояние API и перечитать список прокси, иначе покажет данные прежней
// машины.
func newEndpointPicker(ac *core.AppController, onChanged func()) *widget.Select {
	picker := widget.NewSelect(nil, nil)

	// labelToID — подпись в списке → ID записи реестра; пустой ID = Local.
	labelToID := map[string]string{}

	// programmatic гасит OnChanged на время нашей же перерисовки: SetSelected
	// дёргает колбэк, и без флага перерисовка списка вызывала бы повторное
	// переключение (и лишний реконнект).
	programmatic := false

	rebuild := func() {
		options := []string{endpointPickerLocalLabel()}
		labelToID = map[string]string{}

		if ac != nil && ac.FileService != nil {
			entries, err := services.NewRemoteRegistry(ac.FileService.ExecDir).List()
			if err != nil {
				debuglog.WarnLog("endpoint picker: registry: %v", err)
			}
			for _, e := range entries {
				label := "🌐 " + e.Name
				options = append(options, label)
				labelToID[label] = e.ID
			}
		}
		picker.Options = options

		// Текущий выбор: активный lxd-override либо Local.
		current := endpointPickerLocalLabel()
		if activeID, _, active := GetLxdRemoteOverride(); active {
			for label, id := range labelToID {
				if id == activeID {
					current = label
					break
				}
			}
		}
		programmatic = true
		picker.SetSelected(current)
		programmatic = false
		picker.Refresh()
	}

	picker.OnChanged = func(selected string) {
		if programmatic || selected == "" {
			return
		}
		id, isRemote := labelToID[selected]
		if !isRemote {
			ClearLxdRemoteOverride(ac)
			if onChanged != nil {
				onChanged()
			}
			return
		}
		if err := SetLxdRemoteOverride(ac, id); err != nil {
			// Вернуть список в согласованное состояние: выбор не применился,
			// а показывать выбранным то, к чему не подключились, — врать.
			dialog.ShowError(err, ac.UIService.MainWindow)
			rebuild()
			return
		}
		if onChanged != nil {
			onChanged()
		}
	}

	rebuild()
	// Реестр может пополниться в окне подключения, а override — смениться
	// оттуда же; обе ситуации приходят сюда через общий notify.
	OnOverrideChanged(func() {
		fyne.Do(rebuild)
	})
	return picker
}
