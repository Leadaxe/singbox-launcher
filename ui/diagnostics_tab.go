package ui

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/pion/stun"
	"github.com/txthinking/socks5"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// STUN settings (process-wide, overridable from Diagnostics tab).
var (
	stunServerAddr = constants.DefaultSTUNServer
	// stunUseSOCKS5OnMac: on darwin, when true use system SOCKS5 if available; when false always use direct connection.
	stunUseSOCKS5OnMac = true
)

// checkSTUN performs a STUN request to determine the external IP address.
// useProxy: on darwin, when true use system SOCKS5 if available; when false use direct connection. Ignored on other platforms.
// Returns IP address, whether proxy was used, and error.
func checkSTUN(serverAddr string, useProxy bool) (ip string, usedProxy bool, err error) {
	var conn net.Conn

	if runtime.GOOS == "darwin" && useProxy {
		proxyHost, proxyPort, proxyEnabled, proxyErr := platform.GetSystemSOCKSProxy()
		if proxyErr == nil && proxyEnabled && proxyHost != "" && proxyPort > 0 {
			debuglog.DebugLog("diagnosticsTab: Using system SOCKS5 proxy %s:%d for STUN test", proxyHost, proxyPort)
			socksClient, err := socks5.NewClient(fmt.Sprintf("%s:%d", proxyHost, proxyPort), "", "", 0, 60)
			if err != nil {
				return "", false, fmt.Errorf("failed to create SOCKS5 client: %w", err)
			}
			conn, err = socksClient.Dial("udp", serverAddr)
			if err != nil {
				return "", false, fmt.Errorf("failed to dial STUN server via SOCKS5 proxy: %w", err)
			}
			usedProxy = true
		} else {
			if proxyErr != nil {
				debuglog.DebugLog("diagnosticsTab: Failed to get system proxy settings: %v, using direct connection", proxyErr)
			}
			conn, err = net.Dial("udp", serverAddr)
			if err != nil {
				return "", false, fmt.Errorf("failed to dial STUN server: %w", err)
			}
		}
	} else {
		if runtime.GOOS == "darwin" && !useProxy {
			debuglog.DebugLog("diagnosticsTab: STUN test via direct connection (user setting)")
		}
		conn, err = net.Dial("udp", serverAddr)
		if err != nil {
			return "", false, fmt.Errorf("failed to dial STUN server: %w", err)
		}
	}
	defer debuglog.RunAndLog("checkSTUN: close connection", conn.Close)

	// Create STUN client
	c, err := stun.NewClient(conn)
	if err != nil {
		return "", usedProxy, fmt.Errorf("failed to create STUN client: %w", err)
	}
	// Гарантируем корректное освобождение внутренних горутин и ресурсов клиента
	defer debuglog.RunAndLog("checkSTUN: close STUN client", c.Close)

	// Создаем сообщение для запроса
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	var xorAddr stun.XORMappedAddress
	var errResult error

	// Канал для получения результата из горутины
	done := make(chan bool)

	// Выполняем запрос в горутине
	go func() {
		err = c.Do(message, func(res stun.Event) {
			if res.Error != nil {
				errResult = res.Error
				return
			}
			// Ищем XORMappedAddress в ответе
			if err := xorAddr.GetFrom(res.Message); err != nil {
				errResult = err
				return
			}
		})
		if err != nil {
			errResult = err
		}
		close(done)
	}()

	// Ждем результата или таймаута
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-done:
		if errResult != nil {
			return "", usedProxy, fmt.Errorf("STUN request failed: %w", errResult)
		}
		return xorAddr.IP.String(), usedProxy, nil
	case <-ctx.Done():
		return "", usedProxy, fmt.Errorf("STUN request timed out")
	}
}

func effectiveSTUNServer() string {
	s := strings.TrimSpace(stunServerAddr)
	if s == "" {
		return constants.DefaultSTUNServer
	}
	return s
}

// CreateDiagnosticsTab creates and returns the content for the "Diagnostics" tab.
func CreateDiagnosticsTab(ac *core.AppController) fyne.CanvasObject {
	stunButton := widget.NewButton(locale.T("diag.stun_button"), func() {
		waitDialog := dialogs.NewCustom(locale.T("diag.stun_check_title"), widget.NewLabel(locale.T("diag.stun_checking")), nil, "", ac.UIService.MainWindow)
		waitDialog.Show()

		server := effectiveSTUNServer()
		useProxy := stunUseSOCKS5OnMac

		go func() {
			ip, usedProxy, err := checkSTUN(server, useProxy)

			fyne.Do(func() {
				waitDialog.Hide()
				if err != nil {
					debuglog.ErrorLog("diagnosticsTab: STUN check failed: %v", err)
					ShowError(ac.UIService.MainWindow, err)
				} else {
					var connectionInfo string
					if usedProxy {
						debuglog.InfoLog("diagnosticsTab: STUN check successful via SOCKS5 proxy, IP: %s", ip)
						connectionInfo = fmt.Sprintf("(determined via [UDP]%s)\nvia system proxy SOCKS5", server)
					} else {
						debuglog.InfoLog("diagnosticsTab: STUN check successful, IP: %s", ip)
						connectionInfo = fmt.Sprintf("(determined via [UDP]%s, direct connection)", server)
					}
					resultLabel := widget.NewLabel(locale.Tf("diag.external_ip_format", ip, connectionInfo))
					copyButton := widget.NewButton(locale.T("diag.copy_ip"), func() {
						fyne.CurrentApp().Clipboard().SetContent(ip)
						dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, locale.T("diag.copied_title"), locale.T("diag.ip_copied"))
					})
					ShowCustom(ac.UIService.MainWindow, locale.T("diag.stun_result_title"), locale.T("diag.close"), container.NewVBox(resultLabel, copyButton))
				}
			})
		}()
	})

	const alwaysOnlineSTUNURL = "https://github.com/pradt2/always-online-stun?tab=readme-ov-file#always-online-stun-servers"

	stunSettingsButton := widget.NewButton("⚙", func() {
		serverEntry := widget.NewEntry()
		serverEntry.SetPlaceHolder(constants.DefaultSTUNServer)
		serverEntry.SetText(stunServerAddr)

		stunHelpButton := widget.NewButton("?", func() {
			if err := platform.OpenURL(alwaysOnlineSTUNURL); err != nil {
				debuglog.ErrorLog("diagnosticsTab: Failed to open STUN list URL: %v", err)
				ShowError(ac.UIService.MainWindow, err)
			}
		})

		content := container.NewVBox(
			widget.NewLabel(locale.T("diag.stun_server_label")),
			container.NewBorder(nil, nil, nil, stunHelpButton, serverEntry),
		)
		var socksCheck *widget.Check
		if runtime.GOOS == "darwin" {
			socksCheck = widget.NewCheck(locale.T("diag.use_system_socks5"), func(bool) {})
			socksCheck.SetChecked(stunUseSOCKS5OnMac)
			content.Add(socksCheck)
		}
		content.Add(widget.NewLabel(" "))

		d := dialog.NewCustomConfirm(locale.T("diag.stun_settings"), locale.T("diag.save"), locale.T("diag.cancel"), content, func(ok bool) {
			if !ok {
				return
			}
			stunServerAddr = strings.TrimSpace(serverEntry.Text)
			if stunServerAddr == "" {
				stunServerAddr = constants.DefaultSTUNServer
			}
			if socksCheck != nil {
				stunUseSOCKS5OnMac = socksCheck.Checked
			}
		}, ac.UIService.MainWindow)
		// Fyne auto-sizes to content, which clips the URL entry on Windows
		// (issue #54). Force a readable width so a 40-char STUN URL fits.
		d.Resize(fyne.NewSize(520, 0))
		d.Show()
	})

	// STUN button fills width, gear on the right
	stunRow := container.NewBorder(nil, nil, nil, stunSettingsButton, stunButton)

	// Helper function to create "Open in Browser" buttons
	openBrowserButton := func(label, url string) fyne.CanvasObject {
		return widget.NewButton(label, func() {
			if err := platform.OpenURL(url); err != nil {
				debuglog.ErrorLog("diagnosticsTab: Failed to open URL %s: %v", url, err)
				ShowError(ac.UIService.MainWindow, err)
			}
		})
	}

	openLogWindowButton := widget.NewButtonWithIcon(locale.T("diag.open_log_window"), theme.ViewRestoreIcon(), func() {
		OpenLogViewerWindow(ac)
	})
	openLogsFolderButton := widget.NewButtonWithIcon(locale.T("diag.open_logs_folder"), theme.FolderOpenIcon(), func() {
		logsDir := platform.GetLogsDir(ac.FileService.ExecDir)
		if err := platform.OpenFolder(logsDir); err != nil {
			debuglog.ErrorLog("diagnosticsTab: Failed to open logs folder: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	})

	return container.NewVBox(
		widget.NewLabel(" "),
		container.NewHBox(openLogWindowButton, openLogsFolderButton),
		widget.NewLabel(locale.T("diag.ip_check_services")),
		stunRow,
		openBrowserButton("2ip.ru", "https://2ip.ru"),
		openBrowserButton("2ip.io", "https://2ip.io"),
		openBrowserButton("2ip.me", "https://2ip.me"),
		openBrowserButton("Yandex Internet", "https://yandex.ru/internet/"),
		openBrowserButton("SpeedTest", "https://www.speedtest.net/"),
		openBrowserButton("WhatIsMyIPAddress", "https://whatismyipaddress.com"),
	)
}
