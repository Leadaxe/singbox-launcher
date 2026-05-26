package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// CreateSettingsTab builds the Settings tab. Collects launcher-wide toggles
// that used to be scattered across Core Dashboard (auto-update, auto-ping)
// and Help (language + download-locales), so there's one obvious place to
// look for "change launcher behavior".
//
// Settings persist to bin/settings.json via locale.LoadSettings /
// locale.SaveSettings with load-mutate-save — we explicitly avoid the
// `Settings{Lang: code}` "fresh struct" anti-pattern which silently wiped
// every other field.
func CreateSettingsTab(ac *core.AppController) fyne.CanvasObject {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)

	// ---- Subscriptions section ---------------------------------------------
	subsTitle := widget.NewLabelWithStyle(locale.T("settings.section_subscriptions"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	autoUpdateCheck := widget.NewCheck(locale.T("core.auto_update_subs_label"), nil)
	autoUpdateCheck.SetChecked(ac.StateService.IsAutoUpdateEnabled())
	autoUpdateCheck.OnChanged = func(enabled bool) {
		ac.StateService.SetAutoUpdateEnabled(enabled)
		if enabled {
			ac.StateService.ResetAutoUpdateFailedAttempts()
		}
		st := locale.LoadSettings(binDir)
		st.SubscriptionAutoUpdateDisabled = !enabled
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("settings_tab: save subscription_auto_update_disabled: %v", err)
		}
	}

	autoPingCheck := widget.NewCheck(locale.T("core.auto_ping_label"), nil)
	autoPingCheck.SetChecked(ac.StateService.IsAutoPingAfterConnectEnabled())
	autoPingCheck.OnChanged = func(enabled bool) {
		ac.StateService.SetAutoPingAfterConnectEnabled(enabled)
		st := locale.LoadSettings(binDir)
		st.AutoPingAfterConnectDisabled = !enabled
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("settings_tab: save auto_ping_after_connect_disabled: %v", err)
		}
	}

	// ---- Language section --------------------------------------------------
	langTitle := widget.NewLabelWithStyle(locale.T("settings.section_language"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	langLabel := widget.NewLabel(locale.T("help.language_label"))
	langSelect := widget.NewSelect(locale.LangDisplayNames(), nil)
	langSelect.Selected = locale.LangDisplayName(locale.GetLang())

	downloadLocalesBtn := ttwidget.NewButton(locale.T("help.download_locales_btn"), nil)
	downloadLocalesBtn.SetToolTip(locale.T("help.download_locales"))

	langSelect.OnChanged = func(selected string) {
		code := locale.LangCodeByDisplayName(selected)
		if code == "" || code == locale.GetLang() {
			return
		}
		locale.SetLang(code)
		// load-mutate-save so we don't clobber other settings fields
		st := locale.LoadSettings(binDir)
		st.Lang = code
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.ErrorLog("settings_tab: save lang: %v", err)
		}
		ShowInfo(ac.UIService.MainWindow, locale.T("help.language_label"),
			fmt.Sprintf("%s\n\n%s", locale.LangDisplayName(code), locale.T("help.language_changed")))
	}

	downloadLocalesBtn.OnTapped = func() {
		downloadLocalesBtn.Disable()
		downloadLocalesBtn.SetText(locale.T("help.downloading_locales_btn"))
		go func() {
			localeDir := locale.GetLocaleDir(binDir)
			count, err := locale.DownloadAllRemoteLocales(localeDir)
			fyne.Do(func() {
				downloadLocalesBtn.Enable()
				downloadLocalesBtn.SetText(locale.T("help.download_locales_btn"))
				if err != nil && count == 0 {
					downloadURL := ""
					if len(locale.RemoteLanguages) > 0 {
						downloadURL = locale.GetLocaleURL(locale.RemoteLanguages[0])
					}
					dialogs.ShowDownloadFailedManual(
						ac.UIService.MainWindow,
						locale.T("help.download_locales_failed"),
						downloadURL,
						localeDir,
					)
					return
				}
				langSelect.Options = locale.LangDisplayNames()
				langSelect.Selected = locale.LangDisplayName(locale.GetLang())
				langSelect.Refresh()
				ShowInfo(ac.UIService.MainWindow, locale.T("help.language_label"),
					locale.Tf("help.download_locales_success", count))
			})
		}()
	}

	// langSelect stretches; button stays compact on the right.
	langRow := container.NewBorder(nil, nil, langLabel, downloadLocalesBtn, langSelect)

	// ---- Subscription identification (SPEC 061 Phase 4) -------------------
	subIDTitle := widget.NewLabelWithStyle(locale.T("settings.section_subscription_identification"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subIDBlock := buildSubscriptionIdentificationBlock(ac, binDir)

	content := container.NewVBox(
		subsTitle,
		autoUpdateCheck,
		autoPingCheck,
		widget.NewSeparator(),
		langTitle,
		langRow,
		widget.NewSeparator(),
		subIDTitle,
		subIDBlock,
	)
	return container.NewPadded(content)
}

// buildSubscriptionIdentificationBlock — SPEC 061 Phase 4 controls:
//
//   - Checkbox "Send device identification to providers" — toggles all
//     four X-Hwid-* request headers. Writes to Settings.SubscriptionSendHWID
//     (pointer to distinguish "explicitly false" from "default nil = true").
//
//   - Checkbox "Hash device model" — if checked, X-Device-Model is sent
//     as sha256(model)[:16] instead of raw "MacBookPro18,1". Disabled
//     (greyed) when send_hwid is off — no headers go out anyway.
//
//   - Entry "Device ID (HWID)" + Regenerate button — exposes the
//     random-UUIDv4 identifier. Editing accepts any 8-4-4-4-12 hex form
//     (loose validation — providers don't validate version/variant bits;
//     advanced users may want to paste their old install's UUID to keep
//     the same device slot at the provider). Regenerate prompts before
//     overwriting since it can burn a device slot.
//
// Layout: stack of rows in a VBox, each row a Border / HBox so labels
// stay left, controls fill right.
func buildSubscriptionIdentificationBlock(ac *core.AppController, binDir string) fyne.CanvasObject {
	st := locale.LoadSettings(binDir)
	// Lazy-generate HWID on first open so the entry isn't blank for a
	// first-time visit; persist immediately so the row's current display
	// matches what the launcher will send on the next subscription fetch.
	if st.HWID == "" {
		_ = st.EnsureHWID()
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("settings_tab: persist lazy-generated HWID: %v", err)
		}
	}

	// helpDialog — common pattern for the long-form explanations that
	// used to sit on the checkbox label. Short label + tiny "?" button
	// next to it; click opens a modal with the full text. Same shape
	// we use elsewhere in the app (singboxHelpBtn et al.).
	helpDialog := func(title, body string) func() {
		return func() {
			ShowInfo(ac.UIService.MainWindow, title, body)
		}
	}

	// --- send_hwid checkbox + "?" help
	sendHWIDCheck := widget.NewCheck(locale.T("settings.send_hwid_label"), nil)
	sendHWIDCheck.SetChecked(st.ShouldSendHWID())
	sendHWIDHelp := widget.NewButton("?", helpDialog(
		locale.T("settings.send_hwid_label"),
		locale.T("settings.send_hwid_tooltip"),
	))
	sendHWIDHelp.Importance = widget.LowImportance
	sendHWIDRow := container.NewHBox(sendHWIDCheck, sendHWIDHelp)

	// --- hash_model checkbox + "?" help
	hashModelCheck := widget.NewCheck(locale.T("settings.hash_device_model_label"), nil)
	hashModelCheck.SetChecked(st.SubscriptionDeviceModelHashed)
	hashModelHelp := widget.NewButton("?", helpDialog(
		locale.T("settings.hash_device_model_label"),
		locale.T("settings.hash_device_model_tooltip"),
	))
	hashModelHelp.Importance = widget.LowImportance
	hashModelRow := container.NewHBox(hashModelCheck, hashModelHelp)
	if !st.ShouldSendHWID() {
		hashModelCheck.Disable() // greyed when whole HWID send is off
	}

	// --- HWID entry + Regenerate (icon-only — text moved to tooltip)
	hwidEntry := widget.NewEntry()
	hwidEntry.SetText(st.HWID)

	regenBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), nil)
	regenBtn.SetToolTip(locale.T("settings.hwid_regenerate"))

	// Wire send_hwid first so hashModelCheck.Enable/Disable can react.
	sendHWIDCheck.OnChanged = func(checked bool) {
		cur := locale.LoadSettings(binDir)
		b := checked
		cur.SubscriptionSendHWID = &b
		if err := locale.SaveSettings(binDir, cur); err != nil {
			debuglog.WarnLog("settings_tab: save subscription_send_hwid: %v", err)
		}
		if checked {
			hashModelCheck.Enable()
		} else {
			hashModelCheck.Disable()
		}
	}

	hashModelCheck.OnChanged = func(checked bool) {
		cur := locale.LoadSettings(binDir)
		cur.SubscriptionDeviceModelHashed = checked
		if err := locale.SaveSettings(binDir, cur); err != nil {
			debuglog.WarnLog("settings_tab: save subscription_device_model_hashed: %v", err)
		}
	}

	hwidEntry.OnChanged = func(text string) {
		// Loose UUID validation: 8-4-4-4-12 hex, case-insensitive. Empty
		// is invalid (would leave us without an identifier on next fetch).
		if !looksLikeUUID(text) {
			return // wait for more characters; don't toast on every keystroke
		}
		cur := locale.LoadSettings(binDir)
		cur.HWID = text
		if err := locale.SaveSettings(binDir, cur); err != nil {
			debuglog.WarnLog("settings_tab: save hwid: %v", err)
		}
	}

	regenBtn.OnTapped = func() {
		// Confirm — burning a fresh UUID means the next fetch registers
		// as a new device at HWID-binding providers, consuming one of N
		// allowed slots. Once accepted, the old UUID is dead until the
		// user removes it via the provider's management bot.
		ShowConfirm(
			ac.UIService.MainWindow,
			locale.T("settings.hwid_regenerate_confirm_title"),
			locale.T("settings.hwid_regenerate_confirm_body"),
			func(ok bool) {
				if !ok {
					return
				}
				newID := locale.GenerateUUIDv4()
				hwidEntry.SetText(newID)
				cur := locale.LoadSettings(binDir)
				cur.HWID = newID
				if err := locale.SaveSettings(binDir, cur); err != nil {
					debuglog.WarnLog("settings_tab: save regenerated hwid: %v", err)
				}
			},
		)
	}

	hwidLabel := widget.NewLabel(locale.T("settings.hwid_label"))
	// 120px ≈ 12 visible UUID chars; full 36-char UUID still fits via
	// horizontal scroll inside the entry. Compact-by-default — users
	// either copy-paste the whole string or use Regenerate, both work
	// without seeing the full ID at once.
	hwidEntryFixed := container.New(layout.NewGridWrapLayout(fyne.NewSize(120, hwidEntry.MinSize().Height)), hwidEntry)
	hwidRow := container.NewHBox(hwidLabel, hwidEntryFixed, regenBtn, layout.NewSpacer())

	return container.NewVBox(
		sendHWIDRow,
		hashModelRow,
		hwidRow,
	)
}

// looksLikeUUID — 8-4-4-4-12 hex check, case-insensitive. We don't
// require RFC 4122 version/variant bits because the provider won't
// either; advanced users may paste any UUID-shaped string from an
// older install to keep their device slot.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
