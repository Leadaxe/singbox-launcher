package ui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/debugapi"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// buildSettingsContent builds the Settings UI body. Collects launcher-wide
// toggles that used to be scattered across Core Dashboard (auto-update,
// auto-ping) and Help (language + download-locales), so there's one obvious
// place to look for "change launcher behavior".
//
// Originally a main-window tab (`CreateSettingsTab`); promoted to its own
// OS window when content outgrew a single tab page. The Settings tab in the
// AppTabs strip stays as a clickable entry point but its OnSelected handler
// opens this content in `OpenSettingsWindow` (see ui/settings_window.go)
// and immediately reverts tab selection — visible discoverability without
// stealing tab real-estate.
//
// Settings persist to bin/settings.json via locale.LoadSettings /
// locale.SaveSettings with load-mutate-save — we explicitly avoid the
// `Settings{Lang: code}` "fresh struct" anti-pattern which silently wiped
// every other field.
func buildSettingsContent(ac *core.AppController) fyne.CanvasObject {
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

	// Auto-ping is a connection-behavior toggle (ping proxies after VPN
	// connects), not a subscription setting — it gets its own section.
	connTitle := widget.NewLabelWithStyle(locale.T("settings.section_connection"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
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

	// --- Subscription User-Agent override -----------------------------------
	// Empty entry → fetcher falls back to BuildSubscriptionUserAgent (the
	// default UA shown as the placeholder). Reset button just clears the
	// field, which triggers OnChanged → save empty string → default kicks in
	// on next fetch. We do NOT live-write on every keystroke (would race
	// while user pastes a long UA); save fires on focus-loss via OnSubmitted
	// pattern and explicitly on Reset.
	defaultUA := configtypes.BuildSubscriptionUserAgent()
	uaLabel := widget.NewLabel(locale.T("settings.subscription_ua_label"))
	uaHint := widget.NewLabel(locale.Tf("settings.subscription_ua_hint", defaultUA))
	uaHint.Wrapping = fyne.TextWrapWord
	uaEntry := widget.NewEntry()
	uaEntry.SetPlaceHolder(defaultUA)
	{
		// Initial value from disk. Load fresh — autoUpdateCheck above already
		// loaded once but might be stale if other code wrote to settings since
		// (e.g. HWID lazy-gen on first fetch). Cheap re-load avoids guessing.
		curSt := locale.LoadSettings(binDir)
		uaEntry.SetText(curSt.SubscriptionUserAgent)
	}
	saveUA := func(text string) {
		text = strings.TrimSpace(text)
		cur := locale.LoadSettings(binDir)
		if cur.SubscriptionUserAgent == text {
			return
		}
		cur.SubscriptionUserAgent = text
		if err := locale.SaveSettings(binDir, cur); err != nil {
			debuglog.WarnLog("settings_tab: save subscription_user_agent: %v", err)
		}
	}
	// Debounce 500ms: Fyne fires OnChanged on every keystroke. Without a
	// delay each char triggers a settings.json atomic rename — wasteful and
	// noisy in logs. The timer is reset on every keystroke, so the actual
	// write fires 500ms after the user *stops* typing.
	//
	// Thread-safety: time.AfterFunc fires its callback on a fresh goroutine,
	// so Stop/Reset/store of `uaSaveTimer` must be guarded by a mutex.
	// OnChanged runs on the UI thread, callback runs off-thread, mutex is
	// the cheapest correct synchronization.
	var (
		uaSaveMu    sync.Mutex
		uaSaveTimer *time.Timer
	)
	scheduleSaveUA := func(text string) {
		uaSaveMu.Lock()
		defer uaSaveMu.Unlock()
		if uaSaveTimer != nil {
			uaSaveTimer.Stop()
		}
		uaSaveTimer = time.AfterFunc(500*time.Millisecond, func() {
			saveUA(text)
		})
	}
	flushSaveUA := func(text string) {
		uaSaveMu.Lock()
		if uaSaveTimer != nil {
			uaSaveTimer.Stop()
			uaSaveTimer = nil
		}
		uaSaveMu.Unlock()
		saveUA(text)
	}
	// Save 500ms after user stops typing. Enter / focus-out flushes
	// immediately (Fyne 2.5+ fires OnSubmitted on Tab-out too).
	uaEntry.OnChanged = scheduleSaveUA
	uaEntry.OnSubmitted = flushSaveUA

	// Icon-only reset (text moved to tooltip — same pattern as HWID
	// Regenerate). Tooltip explains the action because the bare refresh
	// icon could read as "refresh field" or "reload from disk".
	// Reset is a deliberate action — flush immediately rather than wait
	// for the debounce window.
	uaResetBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		uaEntry.SetText("") // OnChanged fires → scheduleSaveUA("") starts timer
		flushSaveUA("")     // override the timer with an immediate write
	})
	uaResetBtn.SetToolTip(locale.T("settings.subscription_ua_reset_tooltip"))

	uaRow := container.NewBorder(nil, nil, uaLabel, uaResetBtn, uaEntry)

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

	// ---- Debug API (переехал из Diagnostics tab) ---------------------------
	// Это launcher-wide setting (порт + токен персистятся в bin/settings.json),
	// а не ad-hoc диагностика — поэтому живёт здесь рядом с auto-update,
	// языком и идентификацией подписки.
	debugAPIBlock := buildDebugAPIRow(ac)

	// Language first so the two subscription sections (Subscriptions +
	// Subscription identification) sit together instead of being split by the
	// Language block.
	content := container.NewVBox(
		langTitle,
		langRow,
		widget.NewSeparator(),
		connTitle,
		autoPingCheck,
		widget.NewSeparator(),
		subsTitle,
		autoUpdateCheck,
		uaRow,
		uaHint,
		widget.NewSeparator(),
		subIDTitle,
		subIDBlock,
		widget.NewSeparator(),
		debugAPIBlock,
	)
	return content
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

// buildDebugAPIRow renders the local HTTP Debug API toggle + token copy.
// Off by default. First enable generates a random Bearer token; persists to
// bin/settings.json. UI shows bound address ("127.0.0.1:9263") while running.
//
// Locale keys остаются в `diag.debug_api_*` namespace для backward-compat
// с уже переведёнными строками — функционал тот же, просто переехал из
// Diagnostics → Settings tab (так как это persisted launcher setting,
// а не one-shot диагностическое действие).
func buildDebugAPIRow(ac *core.AppController) fyne.CanvasObject {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)

	title := widget.NewLabelWithStyle(locale.T("diag.debug_api_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	// Hint text wraps to window width instead of forcing the window wider —
	// otherwise a 90-char description pins the whole tab's minimum size.
	hint := widget.NewLabel(locale.T("diag.debug_api_hint"))
	hint.Wrapping = fyne.TextWrapWord
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	refreshStatus := func() {
		addr := ac.DebugAPIAddr()
		if addr == "" {
			status.SetText(locale.T("diag.debug_api_off"))
		} else {
			status.SetText(locale.Tf("diag.debug_api_on", addr))
		}
	}
	refreshStatus()

	copyTokenBtn := widget.NewButtonWithIcon(locale.T("diag.debug_api_copy_token"), theme.ContentCopyIcon(), nil)
	copyTokenBtn.OnTapped = func() {
		// Re-load settings each tap so Copy always reflects the latest token
		// (e.g. after a user regenerates via the checkbox dance).
		cur := locale.LoadSettings(binDir)
		if cur.DebugAPIToken == "" {
			return
		}
		ac.UIService.MainWindow.Clipboard().SetContent(cur.DebugAPIToken)
		// Silent clipboard copies feel like dead buttons. A toast confirms
		// the token actually went to the clipboard.
		dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
			locale.T("diag.debug_api_copied_title"), locale.T("diag.debug_api_copied_msg"))
	}
	if st.DebugAPIToken == "" {
		copyTokenBtn.Disable()
	}

	// Copy API info (connection card, SPEC 078) — one JSON blob with base_url,
	// token, versions and a docs link that the user hands to an agent so it can
	// connect from scratch. Only meaningful while the API is listening (needs a
	// live base_url), so it tracks the enable checkbox.
	copyCardBtn := widget.NewButtonWithIcon(locale.T("diag.debug_api_copy_card"), theme.ContentCopyIcon(), nil)
	copyCardBtn.OnTapped = func() {
		cur := locale.LoadSettings(binDir)
		addr := ac.DebugAPIAddr()
		if addr == "" || cur.DebugAPIToken == "" {
			return
		}
		coreVer, _ := ac.GetInstalledCoreVersion()
		card, err := debugapi.ConnectionCardJSON("http://"+addr, cur.DebugAPIToken, constants.AppVersion, coreVer)
		if err != nil {
			ShowError(ac.UIService.MainWindow, err)
			return
		}
		ac.UIService.MainWindow.Clipboard().SetContent(card)
		dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
			locale.T("diag.debug_api_card_copied_title"), locale.T("diag.debug_api_card_copied_msg"))
	}
	if !st.DebugAPIEnabled {
		copyCardBtn.Disable()
	}

	// Regenerate token — rotates the Bearer token. Confirmed because it
	// invalidates the old token: any script/automation still using it gets
	// 401 on the next call. If the API is currently listening we restart it
	// so the live server picks up the new token; otherwise the new token is
	// just persisted for the next enable.
	regenTokenBtn := widget.NewButtonWithIcon(locale.T("diag.debug_api_regen_token"), theme.ViewRefreshIcon(), nil)
	regenTokenBtn.OnTapped = func() {
		ShowConfirm(
			ac.UIService.MainWindow,
			locale.T("diag.debug_api_regen_confirm_title"),
			locale.T("diag.debug_api_regen_confirm_body"),
			func(ok bool) {
				if !ok {
					return
				}
				tok, err := debugapi.GenerateToken()
				if err != nil {
					debuglog.ErrorLog("settings.debug_api: token regen failed: %v", err)
					ShowError(ac.UIService.MainWindow, err)
					return
				}
				cur := locale.LoadSettings(binDir)
				cur.DebugAPIToken = tok
				if err := locale.SaveSettings(binDir, cur); err != nil {
					debuglog.WarnLog("settings.debug_api: save regenerated token: %v", err)
				}
				// Restart the live listener so it serves the new token.
				if ac.DebugAPIAddr() != "" {
					ac.StopDebugAPI()
					if err := ac.StartDebugAPI(cur.DebugAPIPort, tok); err != nil {
						debuglog.ErrorLog("settings.debug_api: restart after regen failed: %v", err)
						ShowError(ac.UIService.MainWindow, err)
					}
				}
				copyTokenBtn.Enable()
				refreshStatus()
				dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
					locale.T("diag.debug_api_regen_done_title"), locale.T("diag.debug_api_regen_done_msg"))
			},
		)
	}

	// Port entry: пользователь может задать кастомный порт. 0/empty =
	// debugapi.DefaultPort. Меняется только когда API выключен (иначе
	// гонка между Stop старого listener'а и Start нового на занятом порту);
	// поле disable'ится при чекбоксе ON.
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder(fmt.Sprintf("%d", debugapi.DefaultPort))
	if st.DebugAPIPort > 0 {
		portEntry.SetText(fmt.Sprintf("%d", st.DebugAPIPort))
	}
	if st.DebugAPIEnabled {
		portEntry.Disable()
	}

	check := widget.NewCheck(locale.T("diag.debug_api_enable"), nil)
	check.SetChecked(st.DebugAPIEnabled)
	check.OnChanged = func(enabled bool) {
		cur := locale.LoadSettings(binDir)
		// Парсим порт из поля; пустое = default. Невалидное → дёргаем
		// диалог и откатываем чекбокс.
		portText := strings.TrimSpace(portEntry.Text)
		port := 0
		if portText != "" {
			p, err := strconv.Atoi(portText)
			if err != nil || p < 1024 || p > 65535 {
				dialog.ShowInformation(
					locale.T("diag.debug_api_port_invalid_title"),
					locale.T("diag.debug_api_port_invalid_msg"),
					ac.UIService.MainWindow,
				)
				check.SetChecked(false)
				return
			}
			port = p
		}
		cur.DebugAPIPort = port
		cur.DebugAPIEnabled = enabled
		if enabled {
			// Lazy-generate token on first enable so tokens don't exist in
			// settings.json until the user actually opts in.
			if strings.TrimSpace(cur.DebugAPIToken) == "" {
				tok, err := debugapi.GenerateToken()
				if err != nil {
					debuglog.ErrorLog("settings.debug_api: token gen failed: %v", err)
					ShowError(ac.UIService.MainWindow, err)
					check.SetChecked(false)
					return
				}
				cur.DebugAPIToken = tok
			}
			if err := locale.SaveSettings(binDir, cur); err != nil {
				debuglog.WarnLog("settings.debug_api: save settings: %v", err)
			}
			port := cur.DebugAPIPort
			if err := ac.StartDebugAPI(port, cur.DebugAPIToken); err != nil {
				debuglog.ErrorLog("settings.debug_api: start failed: %v", err)
				ShowError(ac.UIService.MainWindow, err)
				check.SetChecked(false)
				cur.DebugAPIEnabled = false
				_ = locale.SaveSettings(binDir, cur)
				refreshStatus()
				return
			}
			copyTokenBtn.Enable()
			copyCardBtn.Enable()
			portEntry.Disable()
		} else {
			ac.StopDebugAPI()
			copyCardBtn.Disable()
			// Keep the token in settings.json so re-enabling doesn't rotate
			// it and break existing scripts. Users who want rotation can
			// delete the key manually.
			if err := locale.SaveSettings(binDir, cur); err != nil {
				debuglog.WarnLog("settings.debug_api: save settings: %v", err)
			}
			portEntry.Enable()
		}
		refreshStatus()
	}

	// Port row: [label] [entry…stretch] [Copy API info]. The connection-card
	// button lives here (Border right) instead of in the button row above so
	// four buttons don't force the window wider; the port entry takes the slack.
	portLabel := widget.NewLabel(locale.T("diag.debug_api_port_label"))
	portRow := container.NewBorder(nil, nil, portLabel, copyCardBtn, portEntry)

	row := container.NewVBox(
		title,
		hint,
		container.NewHBox(check, copyTokenBtn, regenTokenBtn),
		portRow,
		status,
	)
	return row
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
