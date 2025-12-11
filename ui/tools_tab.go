package ui

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// CreateToolsTab creates and returns the content for the "Help" tab.
func CreateToolsTab(ac *core.AppController) fyne.CanvasObject {
	logsButton := widget.NewButton("📁 Open Logs Folder", func() {
		logsDir := platform.GetLogsDir(ac.ExecDir)
		if err := platform.OpenFolder(logsDir); err != nil {
			log.Printf("toolsTab: Failed to open logs folder: %v", err)
			ShowError(ac.MainWindow, err)
		}
	})

	configButton := widget.NewButton("⚙️ Open Config Folder", func() {
		binDir := platform.GetBinDir(ac.ExecDir)
		if err := platform.OpenFolder(binDir); err != nil {
			log.Printf("toolsTab: Failed to open config folder: %v", err)
			ShowError(ac.MainWindow, err)
		}
	})
	killButton := widget.NewButton("🛑 Kill Sing-Box", func() {
		go func() {
			processName := platform.GetProcessNameForCheck()
			_ = platform.KillProcess(processName)
			fyne.Do(func() {
				ShowAutoHideInfo(ac.Application, ac.MainWindow, "Kill", "Sing-Box killed if running.")
				ac.RunningState.Set(false)
			})
		}()
	})

	checkUpdatesButton := widget.NewButton("🔄 Check for Updates", func() {
		ac.CheckForUpdates()
	})

	// Version and links section
	versionLabel := widget.NewLabel("📦 Version: " + constants.AppVersion)
	versionLabel.Alignment = fyne.TextAlignCenter

	telegramLink := widget.NewHyperlink("💬 Telegram Channel", nil)
	telegramLink.SetURLFromString("https://t.me/singbox_launcher")
	telegramLink.OnTapped = func() {
		if err := platform.OpenURL("https://t.me/singbox_launcher"); err != nil {
			log.Printf("toolsTab: Failed to open Telegram link: %v", err)
			ShowError(ac.MainWindow, err)
		}
	}

	githubLink := widget.NewHyperlink("🐙 GitHub Repository", nil)
	githubLink.SetURLFromString("https://github.com/Leadaxe/singbox-launcher")
	githubLink.OnTapped = func() {
		if err := platform.OpenURL("https://github.com/Leadaxe/singbox-launcher"); err != nil {
			log.Printf("toolsTab: Failed to open GitHub link: %v", err)
			ShowError(ac.MainWindow, err)
		}
	}

	return container.NewVBox(
		logsButton,
		configButton,
		killButton,
		widget.NewSeparator(),
		checkUpdatesButton,
		widget.NewSeparator(),
		versionLabel,
		container.NewHBox(
			telegramLink,
			widget.NewLabel(" | "),
			githubLink,
		),
	)
}
