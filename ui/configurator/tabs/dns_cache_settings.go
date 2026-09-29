package tabs

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// buildDNSCacheSettings — настройки кэша DNS на вкладке DNS (SPEC 147,
// LxBox §580): размер кэша, устаревшие ответы, хранение между запусками.
// Значения живут прямо в model.SettingsVars (зеркальных полей модели нет),
// любое изменение помечает конфиг как требующий пересборки. Размер вне
// границ не сохраняется: поле показывает ошибку с границами.
//
// Подписи — литералы каталога переводов (те же, что title/tooltip переменных
// в шаблоне), чтобы их видел l10n_check.
func buildDNSCacheSettings(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	programmatic := false

	setVar := func(name, value string) {
		mod := presenter.Model()
		if mod == nil {
			return
		}
		if mod.SettingsVars == nil {
			mod.SettingsVars = make(map[string]string)
		}
		if cur, ok := mod.SettingsVars[name]; ok && cur == value {
			return
		}
		mod.SettingsVars[name] = value
		presenter.MarkAsChanged()
	}
	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	capLabel := newTooltipLabel(locale.T("DNS cache size"), "")
	capEntry := widget.NewEntry()
	capEntry.Validator = func(s string) error {
		if build.ValidDNSCacheCapacity(s) {
			return nil
		}
		return fmt.Errorf(locale.T("From %d to %d"), build.DNSCacheCapacityMin, build.DNSCacheCapacityMax)
	}
	capEntry.OnChanged = func(s string) {
		if programmatic || !build.ValidDNSCacheCapacity(s) {
			return
		}
		setVar(build.VarDNSCacheCapacity, strings.TrimSpace(s))
	}
	capHint := widget.NewLabel(locale.T("Number of cached answers."))
	capHint.Importance = widget.LowImportance
	capWidth := canvas.NewRectangle(nil)
	capWidth.SetMinSize(fyne.NewSize(110, 0))
	capRow := container.NewHBox(capLabel, container.NewStack(capWidth, capEntry), capHint)

	optimisticCheck := widget.NewCheck(locale.T("Serve stale answers"), func(b bool) {
		if programmatic {
			return
		}
		setVar(build.VarDNSOptimistic, boolStr(b))
	})
	optimisticHint := widget.NewLabel(locale.T("Answer from cache at once and refresh in the background."))
	optimisticHint.Importance = widget.LowImportance
	optimisticRow := container.NewHBox(optimisticCheck, optimisticHint)
	storeCheck := widget.NewCheck(locale.T("Keep DNS cache after restart"), func(b bool) {
		if programmatic {
			return
		}
		setVar(build.VarDNSStoreCache, boolStr(b))
	})

	refresh := func() {
		mod := presenter.Model()
		if mod == nil || mod.TemplateData == nil {
			return
		}
		td := mod.TemplateData
		value := func(name string) string {
			return strings.TrimSpace(wizardtemplate.DisplaySettingValueFor(
				td.Vars, mod.SettingsVars, td.RawTemplate, name, mod.Target.Normalized()))
		}
		programmatic = true
		defer func() { programmatic = false }()
		// Свой шаблон без этих переменных: строки нет, поля нет в конфиге.
		showIf := func(o fyne.CanvasObject, name string) {
			if _, ok := wizardtemplate.VarByName(td.Vars, name); ok {
				o.Show()
			} else {
				o.Hide()
			}
		}
		showIf(capRow, build.VarDNSCacheCapacity)
		showIf(optimisticRow, build.VarDNSOptimistic)
		showIf(storeCheck, build.VarDNSStoreCache)
		capEntry.SetText(value(build.VarDNSCacheCapacity))
		optimisticCheck.SetChecked(value(build.VarDNSOptimistic) == "true")
		storeCheck.SetChecked(value(build.VarDNSStoreCache) == "true")
	}
	if gs := presenter.GUIState(); gs != nil {
		gs.RefreshDNSCacheSettings = refresh
	}
	refresh()

	clearBtn := widget.NewButton(locale.T("Clear DNS cache"), func() {
		showClearDNSCache(presenter)
	})
	clearHint := widget.NewLabel(locale.T("Drops cached answers of the running core, including those kept after restart."))
	clearHint.Importance = widget.LowImportance

	return container.NewVBox(
		capRow,
		optimisticRow,
		storeCheck,
		container.NewHBox(clearBtn, clearHint),
	)
}

// showClearDNSCache — кнопка «Clear DNS cache» (SPEC 147): при работающем
// ядре после подтверждения чистит кэш вызовом Clash API POST /cache/dns/flush;
// при остановленном ядре только сообщает, что очистка возможна при
// работающем ядре. Файл cache.db не трогается.
func showClearDNSCache(presenter *wizardpresentation.WizardPresenter) {
	parent := presenter.DialogParent()
	ac := presenter.Controller()
	running := ac != nil && ac.RunningState != nil && ac.RunningState.IsRunning()
	var baseURL, token string
	var apiEnabled bool
	if ac != nil {
		if b, t, ok := ac.DaemonClashEndpoint(); ok {
			baseURL, token, apiEnabled = b, t, true
		} else if ac.APIService != nil {
			baseURL, token, apiEnabled = ac.APIService.GetClashAPIConfig()
		}
	}
	title := locale.T("Clear DNS cache")
	if !running {
		_, msg := clearDNSCacheOutcome(false, apiEnabled, baseURL, token, nil)
		dialogs.ShowInfo(parent, title, msg)
		return
	}
	dialogs.ShowConfirm(parent, locale.T("Clear DNS cache?"),
		locale.T("Cached DNS answers are dropped, including those kept in cache.db. The connection stays up."),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				done, msg := clearDNSCacheOutcome(true, apiEnabled, baseURL, token, api.FlushDNSCache)
				fyne.Do(func() {
					if done {
						dialogs.ShowInfo(parent, title, msg)
					} else {
						dialogs.ShowErrorText(parent, title, msg)
					}
				})
			}()
		})
}

// clearDNSCacheOutcome — логика кнопки очистки кэша DNS без интерфейса:
// решает, можно ли звать ядро, зовёт flush и возвращает итог и текст для
// пользователя.
func clearDNSCacheOutcome(running, apiEnabled bool, baseURL, token string, flush func(baseURL, token string) error) (bool, string) {
	if !running {
		return false, locale.T("DNS cache can be cleared only while the core is running.")
	}
	if !apiEnabled || strings.TrimSpace(baseURL) == "" || flush == nil {
		return false, locale.T("Clearing the DNS cache needs the Clash API, which is off in this config.")
	}
	if err := flush(baseURL, token); err != nil {
		return false, locale.Tf("Could not clear DNS cache: %s", err.Error())
	}
	return true, locale.T("DNS cache cleared.")
}
