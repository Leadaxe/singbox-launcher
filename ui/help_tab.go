package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	devBuildFormatText        = "✅ You are using a development build\nCurrent: %s\nLatest release: %s"
	latestVersionFormatText   = "✅ You are using the latest version\nCurrent: %s"
	updateAvailableFormatText = "🆕 Update available: %s\nCurrent: %s"
)

// CreateHelpTab creates and returns the content for the "Help" tab.
//
// v0.9.6: "Open Config Folder" + "Kill Sing-Box" buttons moved to the
// Diagnostics tab (🔍) — they're service/maintenance actions, semantically
// closer to logs/STUN/debug-api там, чем к информации о версии и ссылкам.
func CreateHelpTab(ac *core.AppController) fyne.CanvasObject {
	// Version and links section
	versionLabel := widget.NewLabel(locale.Tf("📦 Version: %s", constants.AppVersion))
	versionLabel.Alignment = fyne.TextAlignCenter

	// Launcher update status
	launcherUpdateLabel := widget.NewLabel(locale.T("Checking for updates..."))
	launcherUpdateLabel.Alignment = fyne.TextAlignCenter
	launcherUpdateLabel.Wrapping = fyne.TextWrapWord

	// Update launcher version info
	updateLauncherVersionInfo := func() {
		latest := ac.GetCachedLauncherVersion()
		current := constants.AppVersion

		if latest == "" {
			launcherUpdateLabel.SetText(locale.T("Unable to check for updates"))
			return
		}

		currentClean := strings.TrimPrefix(current, "v")
		latestClean := strings.TrimPrefix(latest, "v")

		compareResult := core.CompareVersions(currentClean, latestClean)
		if compareResult < 0 {
			launcherUpdateLabel.SetText(locale.Tf(updateAvailableFormatText, latest, current))
		} else if compareResult > 0 {
			launcherUpdateLabel.SetText(locale.Tf(devBuildFormatText, current, latest))
		} else {
			launcherUpdateLabel.SetText(locale.Tf(latestVersionFormatText, current))
		}
	}

	updateLauncherVersionInfo()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for i := 0; i < 10; i++ {
			<-ticker.C
			if platform.IsSleeping() {
				continue
			}
			fyne.Do(func() {
				if ac.GetCachedLauncherVersion() == "" {
					updateLauncherVersionInfo()
				} else {
					updateLauncherVersionInfo()
					return
				}
			})
		}
	}()

	telegramLink := widget.NewHyperlink(locale.T("💬 Telegram Channel"), nil)
	_ = telegramLink.SetURLFromString("https://t.me/singbox_launcher")
	telegramLink.OnTapped = func() {
		if err := platform.OpenURL("https://t.me/singbox_launcher"); err != nil {
			debuglog.ErrorLog("toolsTab: Failed to open Telegram link: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	}

	githubLink := widget.NewHyperlink(locale.T("🐙 GitHub Repository"), nil)
	_ = githubLink.SetURLFromString("https://github.com/Leadaxe/singbox-launcher")
	githubLink.OnTapped = func() {
		if err := platform.OpenURL("https://github.com/Leadaxe/singbox-launcher"); err != nil {
			debuglog.ErrorLog("toolsTab: Failed to open GitHub link: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	}

	// Language selector + download-locales button moved to the Settings tab
	// (ui/settings_tab.go) so all launcher-wide preferences live together.

	return container.NewVBox(
		versionLabel,
		launcherUpdateLabel,
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			telegramLink,
			widget.NewLabel(" | "),
			githubLink,
			layout.NewSpacer(),
		),
	)
}
