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
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	externalIpFormatText = "Your External IP: %s\n%s"
)

// killSingBoxPanic — force-kill всех sing-box процессов на машине.
// На Darwin использует AuthorizationExecuteWithPrivileges (тот же механизм
// что запуск sing-box с TUN, и тот же что в "Sing-Box already running"
// dialog'е через ShowProcessKillConfirmation) — иначе privileged sing-box
// (запущенный с sudo для TUN) не убивается обычным `killall sing-box`.
// Если в текущей session уже был privileged запуск, кэшированный
// AuthorizationRef переиспользуется (без повторного prompt'а). Иначе macOS
// покажет sudo-prompt — это ожидаемо.
//
// Pattern `sing-box run|start-singbox-privileged` — это
// platform.PrivilegedPkillPattern: ловит и сам sing-box, и root-шелл
// обёртки. pkill вызывается по абсолютному пути без шелла (SPEC 137).
//
// На других OS — обычный `killall`/`taskkill`. На Linux права идут через
// capabilities, root не нужен. На Windows лаунчер работает без прав
// (asInvoker, SPEC 139), и ядро, поднятое повышенным экземпляром (TUN), без
// прав не снять: тогда сообщение с «Restart as administrator», и false —
// RunningState не сбрасывается вслепую (SPEC 139 §6 п. 7).
func killSingBoxPanic(ac *core.AppController) bool {
	if runtime.GOOS == "darwin" {
		if err := platform.KillPrivilegedByPattern(); err != nil {
			debuglog.WarnLog("killSingBoxPanic: privileged pkill failed (%v); falling back to non-privileged", err)
			_ = platform.KillProcess(platform.GetProcessNameForCheck())
		}
		return true
	}
	err := platform.KillProcess(platform.GetProcessNameForCheck())
	if ac.KillNeedsElevation(err) {
		ac.ShowKillNeedsElevation()
		return false
	}
	return true
}

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
	stunButton := widget.NewButton(locale.T("STUN [UDP]"), func() {
		waitDialog := dialogs.NewCustom(locale.T("STUN Check"), widget.NewLabel(locale.T("Checking, please wait...")), nil, "", ac.UIService.MainWindow)
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
					resultLabel := widget.NewLabel(locale.Tf(externalIpFormatText, ip, connectionInfo))
					copyButton := widget.NewButton(locale.T("Copy IP"), func() {
						fyne.CurrentApp().Clipboard().SetContent(ip)
						dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, locale.T("Copied"), locale.T("IP address copied to clipboard."))
					})
					ShowCustom(ac.UIService.MainWindow, locale.T("STUN Check Result"), locale.T("Close"), container.NewVBox(resultLabel, copyButton))
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
			widget.NewLabel(locale.T("STUN server (host:port):")),
			container.NewBorder(nil, nil, nil, stunHelpButton, serverEntry),
		)
		var socksCheck *widget.Check
		if runtime.GOOS == "darwin" {
			socksCheck = widget.NewCheck(locale.T("Use system SOCKS5 proxy"), func(bool) {})
			socksCheck.SetChecked(stunUseSOCKS5OnMac)
			content.Add(socksCheck)
		}
		content.Add(widget.NewLabel(" "))

		d := dialog.NewCustomConfirm(locale.T("STUN settings"), locale.T("Save"), locale.T("Cancel"), content, func(ok bool) {
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

	// IP check services — Select + Open кнопка вместо 6 отдельных rows
	// (раньше каждый сервис был отдельной кнопкой, занимали полэкрана).
	ipCheckServices := []struct{ Label, URL string }{
		{"2ip.ru", "https://2ip.ru"},
		{"2ip.io", "https://2ip.io"},
		{"2ip.me", "https://2ip.me"},
		{"Yandex Internet", "https://yandex.ru/internet/"},
		{"SpeedTest", "https://www.speedtest.net/"},
		{"WhatIsMyIPAddress", "https://whatismyipaddress.com"},
	}
	ipServiceLabels := make([]string, len(ipCheckServices))
	for i, s := range ipCheckServices {
		ipServiceLabels[i] = s.Label
	}
	ipServiceSelect := widget.NewSelect(ipServiceLabels, nil)
	ipServiceSelect.SetSelectedIndex(0)
	ipServiceOpenBtn := widget.NewButtonWithIcon(locale.T("Open in browser"), theme.ComputerIcon(), func() {
		idx := -1
		for i, label := range ipServiceLabels {
			if label == ipServiceSelect.Selected {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		if err := platform.OpenURL(ipCheckServices[idx].URL); err != nil {
			debuglog.ErrorLog("diagnosticsTab: Failed to open URL %s: %v", ipCheckServices[idx].URL, err)
			ShowError(ac.UIService.MainWindow, err)
		}
	})
	ipServicesRow := container.NewBorder(nil, nil, nil, ipServiceOpenBtn, ipServiceSelect)

	openLogWindowButton := widget.NewButtonWithIcon(locale.T("Log window"), theme.ViewRestoreIcon(), func() {
		OpenLogViewerWindow(ac)
	})

	// Traffic Profiler button (SPEC 059). The ⚡ badge in the label
	// indicates an active recording session — refreshed by the
	// ParentRefresh callback the window manager invokes on session
	// start/stop. Button itself is a singleton-window opener: repeat
	// click focuses the existing window rather than spawning a second.
	var trafficProfilerBtn *widget.Button
	refreshTrafficBtn := func() {
		if trafficProfilerBtn == nil {
			return
		}
		label := locale.T("Traffic Profiler")
		if trafficManager != nil && trafficManager.IsRecording() {
			label += " ⚡"
		}
		trafficProfilerBtn.SetText(label)
	}
	trafficProfilerBtn = widget.NewButtonWithIcon(locale.T("Traffic Profiler"), theme.SearchIcon(), func() {
		mgr := trafficWindowManager(ac, func() {
			fyne.Do(refreshTrafficBtn)
		})
		mgr.Show()
		refreshTrafficBtn()
	})
	// Hook profiler session change → button label refresh so the ⚡
	// badge appears/disappears even when the Traffic Profiler window is
	// closed (recording continues in the background per SPEC §"Lifecycle").
	wireTrafficBadgeToProfiler(func() {
		fyne.Do(refreshTrafficBtn)
	})
	refreshTrafficBtn()
	openLogsFolderButton := widget.NewButtonWithIcon(locale.T("Logs folder"), theme.FolderOpenIcon(), func() {
		if err := platform.OpenFolder(string(ac.FileService.Layout.Logs)); err != nil {
			debuglog.ErrorLog("diagnosticsTab: Failed to open logs folder: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	})

	// v0.9.6: service actions перенесены из Help tab — это maintenance/
	// troubleshooting действия, по семантике ближе к logs/STUN/debug-api,
	// чем к информации о версии и ссылкам.
	openConfigFolderButton := widget.NewButtonWithIcon(locale.T("Config folder"), theme.FolderOpenIcon(), func() {
		binDir := ac.FileService.Layout.Data.Bin()
		if err := platform.OpenFolder(binDir); err != nil {
			debuglog.ErrorLog("diagnosticsTab: Failed to open config folder: %v", err)
			ShowError(ac.UIService.MainWindow, err)
		}
	})
	// Без иконки — в locale string уже есть 🛑 (по требованию юзера).
	// MediaStopIcon (⏹) дублировал бы visual.
	killSingBoxButton := widget.NewButton(locale.T("🛑 Kill Sing-Box"), func() {
		go func() {
			if !killSingBoxPanic(ac) {
				return
			}
			fyne.Do(func() {
				dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
					locale.T("Kill"), locale.T("Sing-Box killed if running."))
				ac.RunningState.Set(false)
			})
		}()
	})

	// Clean unused rule-set files (.srs not referenced by any saved state).
	// Multi-stage GC is kept (an .srs stays while any saved state uses it);
	// this is the explicit manual trigger to prune true orphans.
	cleanRuleSetsButton := widget.NewButtonWithIcon(locale.T("Clean unused rule-sets"), theme.DeleteIcon(), func() {
		go func() {
			removed, err := ac.CleanOrphanRuleSets()
			msg := locale.Tf("Removed %d unused rule-set file(s).", len(removed))
			if err != nil {
				msg = err.Error()
			}
			fyne.Do(func() {
				dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
					locale.T("Clean unused rule-sets"), msg)
			})
		}()
	})

	// Переключатель Mesa3D (SPEC 125 §2.6) — только Windows и только когда
	// есть что переключать; на остальных платформах nil и в VBox не попадает.
	mesaBtn := buildMesaToggleButton(ac)

	// Layout: 3 строки (per user request).
	//   Row 1: Log window (full width — самое частое действие при дебаге)
	//   Row 2: Logs folder | Config folder (file-system explorer пара)
	//   Row 3: Kill Sing-Box (full width, destructive — отдельная строка)
	//
	// Debug API toggle переехал в Settings tab — это launcher-wide setting,
	// не диагностика (живёт между запусками, не относится к ad-hoc проверкам).
	logWindowRow := openLogWindowButton
	foldersRow := container.NewGridWithColumns(2, openLogsFolderButton, openConfigFolderButton)
	killRow := killSingBoxButton

	rows := []fyne.CanvasObject{
		widget.NewLabel(" "),
		logWindowRow,
		foldersRow,
		cleanRuleSetsButton,
		killRow,
		trafficProfilerBtn,
	}
	if mesaBtn != nil {
		rows = append(rows, mesaBtn)
	}
	rows = append(rows,
		widget.NewLabel(locale.T("IP Check Services:")),
		stunRow,
		ipServicesRow,
	)
	return container.NewVBox(rows...)
}

// buildMesaToggleButton — единственная кнопка управления Mesa3D в UI
// (SPEC 125 §2.6). Её роль — переключение, когда окно уже есть;
// восстановление после мёртвого старта делает гейт, до всякого UI.
//
// nil означает «нечего переключать»: не Windows, либо рядом с exe нет ни
// Mesa, ни отключённых .off, ни папки mesa3d/.
func buildMesaToggleButton(ac *core.AppController) *widget.Button {
	if runtime.GOOS != "windows" {
		return nil
	}
	// Mesa живёт рядом с exe (AppDir) — единственное исключение из «AppDir
	// только чтение» (SPEC 135 §3); gl-state.json — в DataDir.
	appDir := ac.FileService.Layout.App
	// Чужой одиночный opengl32.dll рядом с exe — не наша Mesa (SPEC 125 §2.1).
	// Перезаписывать его копией из mesa3d/ мы не вправе, поэтому кнопки нет.
	if platform.HasForeignOpenGL(appDir) {
		return nil
	}
	if !platform.IsMesaInstalled(appDir) && !platform.IsMesaDisabled(appDir) && !platform.HasMesaBundle(appDir) {
		return nil
	}

	var btn *widget.Button
	// Подпись зависит от состояния файлов и меняется после каждого действия,
	// поэтому считается функцией, а не один раз при сборке вкладки.
	refresh := func() {
		switch {
		case platform.IsMesaInstalled(appDir):
			btn.SetText(locale.T("Disable Mesa3D (use hardware OpenGL)"))
		default:
			btn.SetText(locale.T("Enable Mesa3D (software rendering)"))
		}
	}
	btn = widget.NewButton("", func() {
		disable := platform.IsMesaInstalled(appDir)
		title := locale.T("Enable Mesa3D (software rendering)")
		// Переключение применяется только новым процессом: opengl32.dll
		// отображается загрузчиком Windows при создании процесса, по таблице
		// импорта exe (SPEC 125 §6.1). Предупреждаем об этом в самом вопросе,
		// а не постфактум.
		question := locale.T("Render the window through the Mesa3D software renderer instead of the GPU?") +
			" " + locale.T("The launcher will restart now.")
		if disable {
			title = locale.T("Disable Mesa3D (use hardware OpenGL)")
			question = locale.T("Stop rendering through Mesa3D and use hardware OpenGL?") +
				" " + locale.T("The launcher will restart now.")
		}
		dialog.NewConfirm(title, question, func(yes bool) {
			if !yes {
				return
			}
			var err error
			newMode := platform.GLModeMesa
			if disable {
				err = platform.DisableMesa(appDir)
				newMode = platform.GLModeHardware
			} else {
				err = platform.EnableMesa(appDir)
			}
			if err != nil {
				debuglog.ErrorLog("diagnosticsTab: Mesa3D toggle failed: %v", err)
				ShowError(ac.UIService.MainWindow, err)
				return
			}
			refresh()
			// phase=restart: выходим по своему решению, а не умираем на
			// инициализации GL, и гейт нового процесса не должен принять одно
			// за другое.
			platform.UpdateGLState(ac.FileService.Layout.Data, func(s *platform.GLState) {
				s.Phase = platform.GLPhaseRestart
				s.Mode = newMode
			})
			debuglog.WarnLog("gl: restarting to apply %s (Diagnostics)", newMode)
			// Сам перезапуск — в самом конце main(), после Run() и
			// GracefulExit: новый процесс не должен подниматься, пока старый
			// держит запущенный sing-box, иначе встретит пользователя диалогом
			// «уже запущено».
			platform.RequestRestartAfterExit()
			ac.GracefulExit()
		}, ac.UIService.MainWindow).Show()
	})
	refresh()
	// Переключение переименовывает DLL рядом с exe: в каталоге только для
	// чтения (установка в Program Files) оно заведомо не сработает.
	if !paths.ProbeWritable(string(appDir)) {
		btn.Disable()
	}
	return btn
}
