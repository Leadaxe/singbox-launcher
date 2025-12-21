package ui

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/pion/stun"
	"github.com/txthinking/socks5"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// checkSTUN performs a STUN request to determine the external IP address.
// Returns IP address, whether proxy was used, and error.
func checkSTUN(serverAddr string) (ip string, usedProxy bool, err error) {
	var conn net.Conn

	// On macOS, try to use system SOCKS5 proxy if enabled
	if runtime.GOOS == "darwin" {
		proxyHost, proxyPort, proxyEnabled, proxyErr := platform.GetSystemSOCKSProxy()
		if proxyErr == nil && proxyEnabled && proxyHost != "" && proxyPort > 0 {
			log.Printf("diagnosticsTab: Using system SOCKS5 proxy %s:%d for STUN test", proxyHost, proxyPort)
			// Create SOCKS5 client
			socksClient, err := socks5.NewClient(fmt.Sprintf("%s:%d", proxyHost, proxyPort), "", "", 0, 60)
			if err != nil {
				return "", false, fmt.Errorf("failed to create SOCKS5 client: %w", err)
			}
			// Dial UDP connection through SOCKS5 proxy
			conn, err = socksClient.Dial("udp", serverAddr)
			if err != nil {
				return "", false, fmt.Errorf("failed to dial STUN server via SOCKS5 proxy: %w", err)
			}
			usedProxy = true
		} else {
			// Proxy not enabled or error getting settings, use direct connection
			if proxyErr != nil {
				log.Printf("diagnosticsTab: Failed to get system proxy settings: %v, using direct connection", proxyErr)
			}
			conn, err = net.Dial("udp", serverAddr)
			if err != nil {
				return "", false, fmt.Errorf("failed to dial STUN server: %w", err)
			}
		}
	} else {
		// On other platforms, use direct connection
		conn, err = net.Dial("udp", serverAddr)
		if err != nil {
			return "", false, fmt.Errorf("failed to dial STUN server: %w", err)
		}
	}
	defer conn.Close()

	// Create STUN client
	c, err := stun.NewClient(conn)
	if err != nil {
		return "", usedProxy, fmt.Errorf("failed to create STUN client: %w", err)
	}
	// Гарантируем корректное освобождение внутренних горутин и ресурсов клиента
	defer c.Close()

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

// CreateDiagnosticsTab creates and returns the content for the "Diagnostics" tab.
func CreateDiagnosticsTab(ac *core.AppController) fyne.CanvasObject {
	// Кнопка для проверки STUN (Google STUN [UDP])
	stunButton := widget.NewButton("Google STUN [UDP]", func() {
		// Показываем диалог ожидания
		waitDialog := dialog.NewCustomWithoutButtons("STUN Check", widget.NewLabel("Checking, please wait..."), ac.UIService.MainWindow)
		waitDialog.Show()

		go func() {
			stunServer := constants.DefaultSTUNServer
			ip, usedProxy, err := checkSTUN(stunServer)

			// Закрываем диалог ожидания и показываем результат
			fyne.Do(func() {
				waitDialog.Hide()
				if err != nil {
					log.Printf("diagnosticsTab: STUN check failed: %v", err)
					ShowError(ac.UIService.MainWindow, err)
				} else {
					var connectionInfo string
					if usedProxy {
						log.Printf("diagnosticsTab: STUN check successful via SOCKS5 proxy, IP: %s", ip)
						connectionInfo = fmt.Sprintf("(determined via [UDP]%s)\nvia system proxy SOCKS5", stunServer)
					} else {
						log.Printf("diagnosticsTab: STUN check successful, IP: %s", ip)
						connectionInfo = fmt.Sprintf("(determined via [UDP]%s, direct connection)", stunServer)
					}
					// Создаем кастомный диалог с кнопкой "Copy"
					resultLabel := widget.NewLabel(fmt.Sprintf("Your External IP: %s\n%s", ip, connectionInfo))
					copyButton := widget.NewButton("Copy IP", func() {
						ac.UIService.MainWindow.Clipboard().SetContent(ip)
						ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Copied", "IP address copied to clipboard.")
					})

					ShowCustom(ac.UIService.MainWindow, "STUN Check Result", "Close", container.NewVBox(resultLabel, copyButton))
				}
			})
		}()
	})

	// Helper function to create "Open in Browser" buttons
	openBrowserButton := func(label, url string) fyne.CanvasObject {
		return widget.NewButton(label, func() {
			if err := platform.OpenURL(url); err != nil {
				log.Printf("diagnosticsTab: Failed to open URL %s: %v", url, err)
				ShowError(ac.UIService.MainWindow, err)
			}
		})
	}

	return container.NewVBox(
		widget.NewLabel("IP Check Services:"),
		stunButton, // Google STUN [UDP] перенесен в секцию IP Check Services
		openBrowserButton("2ip.ru", "https://2ip.ru"),
		openBrowserButton("2ip.io", "https://2ip.io"),
		openBrowserButton("2ip.me", "https://2ip.me"),
		openBrowserButton("Yandex Internet", "https://yandex.ru/internet/"),
		openBrowserButton("SpeedTest", "https://www.speedtest.net/"),
		openBrowserButton("WhatIsMyIPAddress", "https://whatismyipaddress.com"),
	)
}
