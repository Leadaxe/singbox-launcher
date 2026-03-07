package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// CreateHelpTab creates and returns the content for the "Help" tab.
func CreateHelpTab(ac *core.AppController) fyne.CanvasObject {
	configButton := widget.NewButton("⚙️ Open Config Folder", func() {
		binDir := platform.GetBinDir(ac.FileService.ExecDir)
		if err := platform.OpenFolder(binDir); err != nil {
			debuglog.ErrorLog("toolsTab: Failed to open config folder: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	})
	killButton := widget.NewButton("🛑 Kill Sing-Box", func() {
		go func() {
			processName := platform.GetProcessNameForCheck()
			_ = platform.KillProcess(processName)
			fyne.Do(func() {
				ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Kill", "Sing-Box killed if running.")
				ac.RunningState.Set(false)
			})
		}()
	})

	// Version and links section
	versionLabel := widget.NewLabel("📦 Version: " + constants.AppVersion)
	versionLabel.Alignment = fyne.TextAlignCenter

	// Launcher update status
	launcherUpdateLabel := widget.NewLabel("Checking for updates...")
	launcherUpdateLabel.Alignment = fyne.TextAlignCenter
	launcherUpdateLabel.Wrapping = fyne.TextWrapWord

	// Update launcher version info
	updateLauncherVersionInfo := func() {
		latest := ac.GetCachedLauncherVersion()
		current := constants.AppVersion

		if latest == "" {
			launcherUpdateLabel.SetText("Unable to check for updates")
			return
		}

		// Сравниваем версии (убираем префикс v для сравнения)
		currentClean := strings.TrimPrefix(current, "v")
		latestClean := strings.TrimPrefix(latest, "v")

		compareResult := core.CompareVersions(currentClean, latestClean)
		if compareResult < 0 {
			// Новая версия доступна
			launcherUpdateLabel.SetText(fmt.Sprintf("🆕 Update available: %s\nCurrent: %s", latest, current))
		} else if compareResult > 0 {
			// Текущая версия новее (dev build)
			launcherUpdateLabel.SetText(fmt.Sprintf("✅ You are using a development build\nCurrent: %s\nLatest release: %s", current, latest))
		} else {
			// Версии совпадают
			launcherUpdateLabel.SetText(fmt.Sprintf("✅ You are using the latest version\nCurrent: %s", current))
		}
	}

	// Обновляем информацию при создании вкладки
	updateLauncherVersionInfo()

	// Периодически обновляем информацию (если версия еще не получена)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for i := 0; i < 10; i++ {
			select {
			case <-ticker.C:
				fyne.Do(func() {
					if ac.GetCachedLauncherVersion() == "" {
						updateLauncherVersionInfo()
					} else {
						updateLauncherVersionInfo()
						return
					}
				})
			}
		}
	}()

	telegramLink := widget.NewHyperlink("💬 Telegram Channel", nil)
	_ = telegramLink.SetURLFromString("https://t.me/singbox_launcher")
	telegramLink.OnTapped = func() {
		if err := platform.OpenURL("https://t.me/singbox_launcher"); err != nil {
			debuglog.ErrorLog("toolsTab: Failed to open Telegram link: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	}

	githubLink := widget.NewHyperlink("🐙 GitHub Repository", nil)
	_ = githubLink.SetURLFromString("https://github.com/Leadaxe/singbox-launcher")
	githubLink.OnTapped = func() {
		if err := platform.OpenURL("https://github.com/Leadaxe/singbox-launcher"); err != nil {
			debuglog.ErrorLog("toolsTab: Failed to open GitHub link: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	}

	return container.NewVBox(
		configButton,
		killButton,
		widget.NewSeparator(),
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
