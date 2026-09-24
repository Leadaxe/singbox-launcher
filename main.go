package main

import (
	_ "embed" // For embedding resource files (icons)
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui"
)

// Embedded resources (icons for the system tray)
//
//go:embed assets/app.ico
var appIconData []byte // Main application icon

//go:embed assets/off.ico
var greyIconData []byte // Icon for "off" state

//go:embed assets/on.ico
var greenIconData []byte // Icon for "on" state

// Constants
const (
	autoStartDelay = 1 * time.Second // Delay before auto-starting VPN with -start parameter
	// glRenderedGrace — сколько процесс должен прожить после старта цикла
	// событий, чтобы старт считался дошедшим до первого кадра (SPEC 125).
	glRenderedGrace = 3 * time.Second
	// handoffWaitTimeout — сколько перезапущенный с повышением экземпляр ждёт
	// выхода родителя (SPEC 139 §5 п. 6): больше бюджета GracefulExit
	// (15 + 3 с), чтобы родитель успел остановить ядро и закрыть логи.
	handoffWaitTimeout = 25 * time.Second
	// macQuitBudget — сколько выход по запросу macOS (Cmd+Q, «Завершить» в
	// Dock, выход из системы, выключение) ждёт GracefulExit. Ядро
	// останавливается штатно, но выход из системы лаунчер дольше не держит:
	// по истечении процесс завершается всё равно (решение владельца 15.09.2026).
	// Сам GracefulExit ждёт остановки ядра до 2 с, остальное — запас.
	macQuitBudget = 5 * time.Second
	// firstRunNoticeDelay — пауза перед уведомлением «данных не найдено»
	// (SPEC 135 §3.4): OnStarted приходит до первого кадра окна.
	firstRunNoticeDelay = 1 * time.Second
)

// rememberOfferedRenderer запоминает renderer железа, про который мы уже
// спросили: пока строка не сменится, диалог возврата больше не всплывает —
// иначе «Later» переспрашивался бы каждый старт.
func rememberOfferedRenderer(d paths.DataDir, renderer string) {
	platform.UpdateGLState(d, func(s *platform.GLState) {
		s.OfferedHWRenderer = renderer
	})
}

// firstRunNoticeDue — условие одноразового уведомления «данных предыдущей
// версии не найдено» (SPEC 135 §3.4): системная раскладка (System/Env), в
// DataDir нет state.json, источника миграции рядом с бинарём нет, и
// уведомление ещё не показывалось. Portable/Legacy не показывают: там данные
// и есть «рядом с бинарём», искать их больше негде.
func firstRunNoticeDue(fsvc *services.FileService, s locale.Settings) bool {
	if fsvc == nil || s.FirstRunNoticeShown {
		return false
	}
	if m := fsvc.Layout.Mode; m != paths.ModeSystem && m != paths.ModeEnv {
		return false
	}
	r := fsvc.Migration
	return !r.Migrated && !r.DataHadState && r.Source == ""
}

// scheduleFirstRunNotice показывает уведомление, когда окно видно (см.
// whenWindowVisible). Флаг ставится сразу после показа.
func scheduleFirstRunNotice(controller *core.AppController, data paths.DataDir, inTray bool) {
	whenWindowVisible(controller, inTray, func(win fyne.Window) {
		dialog.ShowInformation(locale.T("No previous data found"),
			locale.T("No data from a previous launcher version was found in the data folder. If you had settings and subscriptions, restore them from an LX Backup (Settings → Backup). Data folder:")+
				"\n"+string(data), win)
		if err := locale.MarkFirstRunNoticeShown(data.Bin()); err != nil {
			debuglog.WarnLog("first-run notice: persist flag: %v", err)
		}
	})
}

// hiddenDataNoticeDue — данные в системном каталоге скрыты маркером
// portable.txt (paths.HiddenSystemData): Portable/Legacy, в AppDir/bin нет
// state.json, а в системном DataDir он есть, и пользователь ещё не
// отказался. Возвращает найденный каталог.
func hiddenDataNoticeDue(l paths.Layout, exe string, s locale.Settings) (string, bool) {
	if s.HiddenDataNoticeShown {
		return "", false
	}
	return paths.HiddenSystemData(l, exe, os.Getenv, runtime.GOOS, paths.ProbeWritable)
}

// scheduleHiddenDataNotice предлагает переключиться на найденные данные:
// Yes — удалить portable.txt и перезапуститься (копировать нечего: в
// AppDir/bin только поставляемое), Cancel — больше не спрашивать. Показ —
// как у уведомления первого запуска (whenWindowVisible).
func scheduleHiddenDataNotice(controller *core.AppController, l paths.Layout, dir string, inTray bool) {
	whenWindowVisible(controller, inTray, func(win fyne.Window) {
		confirm := dialog.NewConfirm(locale.T("Existing data found"),
			locale.Tf("Your settings and subscriptions were found in:\n%s\n\nThe launcher is running in portable mode because portable.txt lies next to the program. Switch to the found data? The launcher will restart.", dir),
			func(yes bool) {
				if !yes {
					if err := locale.MarkHiddenDataNoticeShown(l.Data.Bin()); err != nil {
						debuglog.WarnLog("hidden data notice: persist flag: %v", err)
					}
					return
				}
				if err := paths.RemovePortableMarker(l.App); err != nil {
					debuglog.ErrorLog("hidden data notice: %v", err)
					ui.ShowError(win, err)
					return
				}
				debuglog.WarnLog("storage: portable.txt removed to use the data found in %s; restarting", dir)
				platform.RequestRestartAfterExit()
				controller.GracefulExit()
			}, win)
		confirm.SetConfirmText(locale.T("Yes"))
		confirm.SetDismissText(locale.T("Cancel"))
		confirm.Show()
	})
}

// daemonUnsafeNoticeText — модальное предупреждение SPEC 136 §6 (ключ
// перевода = английский текст, SPEC 111). daemonUnsafeNoticeCoreText — то
// же, когда ядро лаунчера не умеет root-owned копию: команды нет, вторая
// подстановка — подсказка обновить ядро (core.DaemonServiceCoreHint).
const (
	daemonUnsafeNoticeText     = "The daemon service runs as root from a file your user account can modify:\n%s\n\nAny program running as you could replace it and gain root access. Run this command in Terminal to move the service to a root-owned copy of the core (it asks for your sudo password). The VPN keeps working meanwhile; the LOCAL tab of the connection settings shows this until it is fixed."
	daemonUnsafeNoticeCoreText = "The daemon service runs as root from a file your user account can modify:\n%s\n\nAny program running as you could replace it and gain root access. %s The VPN keeps working meanwhile; the LOCAL tab of the connection settings shows this until it is fixed."
)

// scheduleDaemonUnsafeNotice — служба демона запускает от root файл,
// который может подменить пользователь (SPEC 136): одно модальное
// предупреждение на версию лаунчера, когда окно видно. Daemon-движок не
// блокируется.
func scheduleDaemonUnsafeNotice(controller *core.AppController, data paths.DataDir, servicePath, command, coreHint string, inTray bool) {
	message := locale.Tf(daemonUnsafeNoticeText, servicePath)
	if command == "" {
		message = locale.Tf(daemonUnsafeNoticeCoreText, servicePath, coreHint)
	}
	whenWindowVisible(controller, inTray, func(win fyne.Window) {
		// Windows (SPEC 141 §9): свой текст и кнопка Run as administrator.
		if !controller.ShowDaemonUnsafeNoticeElevated(win, servicePath, command, coreHint) {
			dialogs.ShowLinuxCapabilitiesRequired(win, locale.T("The daemon service is not protected"), message, command)
		}
		if err := locale.MarkDaemonUnsafeNoticeShown(data.Bin(), constants.AppVersion); err != nil {
			debuglog.WarnLog("daemon unsafe notice: persist flag: %v", err)
		}
	})
}

// whenWindowVisible вызывает show, когда окно видно: при обычном старте —
// чуть позже появления окна; при -tray — при первом раскрытии окна из трея
// в этой сессии. Не раскрыли — show не вызывается, и уведомление ждёт
// следующего старта.
func whenWindowVisible(controller *core.AppController, inTray bool, show func(win fyne.Window)) {
	run := func() {
		if win := controller.UIService.MainWindow; win != nil {
			show(win)
		}
	}
	if !inTray {
		time.AfterFunc(firstRunNoticeDelay, func() { fyne.Do(run) })
		return
	}
	// OnWindowShown зовётся из fyne.Do (UIService.ShowMainWindowOrFocusWizard),
	// то есть в UI-потоке, как и этот код из OnStarted — guard без мьютекса.
	shown := false
	prev := controller.UIService.OnWindowShown
	controller.UIService.OnWindowShown = func() {
		if prev != nil {
			prev()
		}
		if !shown {
			shown = true
			run()
		}
	}
}

// resolveLayout — раскладка данных процесса (SPEC 135, SPEC 139 §5): из
// -handoff, если лаунчер перезапущен с повышением (раскладка родителя, а не
// вычисленная заново: иначе повышенный экземпляр мог бы выбрать другой
// DataDir), иначе paths.Resolve. Невалидный -handoff — строка в stderr и
// обычный Resolve, но PID родителя из него (если разобрался) всё равно
// возвращается. parentPID > 0 — родитель, выхода которого надо дождаться;
// handoffErr — почему -handoff не принят (для WARN после открытия логов).
func resolveLayout(exe, handoff string) (layout paths.Layout, parentPID int, handoffErr error) {
	if handoff != "" {
		l, pid, err := paths.ParseHandoff(handoff, exe)
		if err == nil {
			return l, pid, nil
		}
		parentPID, handoffErr = pid, err
		fmt.Fprintf(os.Stderr, "singbox-launcher: %v; resolving the layout as usual\n", err)
	}
	l, err := paths.Resolve(exe, os.Getenv, runtime.GOOS, paths.ProbeWritable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "singbox-launcher: %v\n", err)
		os.Exit(2)
	}
	return l, parentPID, handoffErr
}

// waitForParent ждёт выхода лаунчера, перезапустившего этот экземпляр с
// повышением (SPEC 139 §5 п. 6), — до crash-лога, GL-пробы и контроллера:
// пока родитель жив, он держит логи (.old на Windows не переименовать), порт
// Debug API и иконку трея. Возвращает текст WARN для лога ("" — дождались);
// логов ещё нет, пишет вызывающий после их открытия.
func waitForParent(pid int, exe string) string {
	exited, err := platform.WaitForProcessExit(pid, exe, handoffWaitTimeout)
	switch {
	case err != nil:
		return fmt.Sprintf("restart as administrator: waiting for the previous launcher (pid %d): %v; starting anyway", pid, err)
	case !exited:
		return fmt.Sprintf("restart as administrator: the previous launcher (pid %d) is still running after %v; starting anyway", pid, handoffWaitTimeout)
	}
	return ""
}

// yesNo — значение флага для строки лога.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// main is the application's entry point. It simply creates and runs the AppController.
func main() {
	// Parse command line arguments. Флаги разбираются до раскладки: -handoff
	// (SPEC 139 §5) задаёт её сам.
	autoStart := flag.Bool("start", false, "Automatically start VPN on launch")
	startInTray := flag.Bool("tray", false, "Start minimized to system tray (hide window on launch)")
	glProbe := flag.Bool("gl-probe", false, "Internal: probe desktop OpenGL and exit (used by the launcher itself)")
	glProbeLocal := flag.Bool("gl-probe-local", false, "Internal: probe the opengl32.dll next to the exe (Mesa3D verification)")
	pathsFlag := flag.Bool("paths", false, "Print resolved data/log/core paths and exit")
	purgeData := flag.Bool("purge-data", false, "Remove all launcher data (dry run; add -yes to execute)")
	purgeYes := flag.Bool("yes", false, "Confirm -purge-data")
	autostartFlag := flag.String(core.AutostartFlagName, "", "Windows: start the launcher at sign-in (on|off), print the result and exit")
	handoffFlag := flag.String(core.HandoffFlagName, "", "Internal: layout of the launcher that restarted this one as administrator (<pid>|<mode>|<data>|<logs>)")
	flag.Parse()

	// SPEC 135: раскладка данных (AppDir/DataDir/LogDir) решается один раз,
	// первым действием, и дальше передаётся значением. Локали и логов ещё
	// нет — ошибка уходит в stderr, код выхода 2.
	exe, err := paths.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "singbox-launcher: cannot determine executable path: %v\n", err)
		os.Exit(2)
	}
	layout, parentPID, handoffErr := resolveLayout(exe, *handoffFlag)

	// SPEC 135 §4.1: единственный способ увидеть пути там, где окно не
	// поднимается (NixOS без GL, headless CI). До crash-лога и GL-пробы:
	// ничего не создаём на диске, окно не открываем. Версии ядра нет —
	// бинарь ради неё не запускается.
	if *pathsFlag {
		fmt.Println(core.PathsInfoFor(layout).Text())
		os.Exit(0)
	}

	// SPEC 135 §4.3, решение Е: очистка без окна (uninstall-хук установщика
	// #99, поддержка). Тоже до crash-лога: LogDir не создаётся заново и не
	// держится открытым. Без -yes — только план.
	if *purgeData {
		os.Exit(core.PurgeCLI(layout, exe, *purgeYes, os.Stdout))
	}

	// SPEC 139 §8: автозапуск для установщика (SPEC 140 §3.3) — тоже без
	// окна и до crash-лога. Установщик зовёт его от исходного пользователя:
	// значение пишется в HKCU того, кто входит в систему.
	if *autostartFlag != "" {
		os.Exit(core.AutostartCLI(*autostartFlag, exe, os.Stdout))
	}

	// SPEC 139 §5 п. 6: перезапущенный с повышением экземпляр ждёт выхода
	// родителя, прежде чем трогать логи, порт Debug API и трей.
	var parentWaitWarning string
	if parentPID > 0 {
		parentWaitWarning = waitForParent(parentPID, exe)
	}

	// Windows-бинарь собран с -H windowsgui: stderr у процесса нет, и паника
	// на старте выглядит как «окно мелькнуло и пропало» без единой строки в
	// логе (репорт 09.09.2026). SetCrashOutput дублирует трассу фатальной
	// паники в <LogDir>/crash.log. Обычные логи открываются позже, в
	// NewAppController; каталог логов создаётся здесь, до них.
	logsDir := string(layout.Logs)
	if err := os.MkdirAll(logsDir, platform.DefaultDirMode); err != nil {
		debuglog.WarnLog("logs dir: %v", err)
	} else {
		crashLog := filepath.Join(logsDir, constants.CrashLogFileName)
		if err := debuglog.EnableCrashOutput(crashLog); err != nil {
			debuglog.WarnLog("crash log: %v", err)
		}
		// SPEC 125 §2.8: SetCrashOutput ловит только панику Go. Падение под
		// Mesa 08.09.2026 случилось в потоке, созданном DLL, и не оставило
		// следов нигде: у windowsgui-сборки stderr ведёт в
		// INVALID_HANDLE_VALUE, и всё, что пишут драйверы и GLFW, теряется.
		// Подменяем системный хендл на файл — чужой нативный вывод получает
		// своё место, наш код по-прежнему идёт через debuglog.
		if err := debuglog.RedirectNativeStderr(filepath.Join(logsDir, constants.NativeStderrLogFileName)); err != nil {
			debuglog.WarnLog("native stderr: %v", err)
		}
	}

	// Служебный режим: лаунчер перезапускает сам себя с -gl-probe (системный
	// OpenGL) или -gl-probe-local (opengl32.dll рядом с exe, то есть Mesa3D),
	// чтобы проверить версию GL в отдельном процессе (issue #105, SPEC 125).
	// Печатает результат в stdout и завершается, не доходя до инициализации UI.
	if *glProbe || *glProbeLocal {
		platform.RunGLProbeChild(layout.App, *glProbeLocal)
	}

	// Create the application controller. If an error occurs, print it and exit the program.
	// Use greyIconData for red icon (no separate red icon yet)
	controller, err := core.NewAppController(layout, appIconData, greyIconData, greenIconData, greyIconData)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Первая WARN-строка любого старта. Успешный запуск не писал ни одной
	// строки уровня WARN, и по логу с релиза (GlobalLevel=LevelWarn) нельзя
	// было понять даже, какая версия упала (репорт 09.09.2026, SPEC 125 §2.7).
	// Раскладка данных — в той же строке (SPEC 135): где искать state и логи.
	// elevated= (SPEC 139 §3): релизный лог пишет только WARN и выше, и
	// строка INFO о пропущенных без прав очистках видна только в dev-сборках.
	debuglog.WarnLog("launcher %s %s/%s started, exec=%s, elevated=%s, %s",
		constants.AppVersion, runtime.GOOS, runtime.GOARCH, exe, yesNo(platform.IsElevated()), layout.LogLine())
	switch {
	case handoffErr != nil:
		debuglog.WarnLog("layout: -%s ignored (%v), resolved as usual", core.HandoffFlagName, handoffErr)
	case parentPID > 0:
		debuglog.WarnLog("layout: from -%s (restarted as administrator by pid %d)", core.HandoffFlagName, parentPID)
	}
	if parentWaitWarning != "" {
		debuglog.WarnLog("%s", parentWaitWarning)
	}
	if layout.MarkerIgnored {
		debuglog.WarnLog("layout: %s ignored: the program folder %s is not writable by the user (SPEC 139)", constants.PortableMarkerFileName, layout.App)
	}
	// Итог миграции (SPEC 135 §3.4) выполнен ещё в NewFileService, до
	// открытия логов; пишем здесь, чтобы строка попала в файл.
	if fsvc := controller.FileService; fsvc.MigrationErr != nil {
		debuglog.WarnLog("migration failed: %v — starting with an empty data folder, will retry next launch", fsvc.MigrationErr)
	} else if fsvc.Migration.Migrated {
		debuglog.WarnLog("%s", fsvc.Migration.Summary())
	} else if fsvc.Migration.Busy {
		debuglog.WarnLog("migration: another instance is migrating, skipping")
	}

	// Дополнение 24.09 к SPEC 139: при включённом TUN лаунчер без прав сразу
	// перезапускается с повышением (флаги, включая -tray и -start, как были).
	// До мьютекса экземпляра, GL-гейта, окна и трея: новый экземпляр ждёт
	// выхода этого. Отказ в UAC — обычный старт без прав.
	if controller.ElevateAtStartForTun() {
		api.SetAPILogFile(nil)
		controller.FileService.CloseLogFiles()
		os.Exit(0)
	}

	// SPEC 140 §4: мьютексы экземпляра и событие Quit (Windows). Установщик
	// находит лаунчер по мьютексу и просит закрыться событием: выход — как
	// Quit в трее, на UI-потоке и без перезапуска, чтобы ядро остановилось
	// штатно и сняло системный прокси. Только здесь, в GUI-режиме: служебные
	// запуски выше (-paths, -purge-data, -gl-probe*) живым лаунчером не
	// считаются. До запуска цикла событий fyne.Do из горутины ставит вызов в
	// очередь, и выход произойдёт с первой же итерацией цикла.
	platform.RegisterInstance(func() { fyne.Do(controller.GracefulExit) })

	// Issue #105: в RDP-сессии Windows Server без GPU системный OpenGL — это
	// «GDI Generic» 1.1, и окно Fyne молча не отрисовывается. Гейт проверяет
	// версию GL в подпроцессе и при необходимости предлагает поставить Mesa3D
	// (llvmpipe). Должен отработать до первого обращения Fyne к GLFW (то есть
	// до Application.Run), пока opengl32.dll ещё не загружен в процесс.
	//
	// interactive = не -tray: в трей-режиме окна никто не ждёт, и диалоги
	// гейта показывать некому (SPEC 125 §2.2).
	platform.EnsureDesktopOpenGL(layout, !*startInTray)

	// Replace the wizard template in the background if it was installed by an
	// older launcher version (SPEC 046). The stale file stays until the new
	// one is downloaded, and config builds wait for the refresh — so the
	// first core start is built from the right template. Runs before anything
	// can start the core (autostart, UI, debug API).
	//
	// Failure is non-fatal: the installed template stays in use and the next
	// launch retries.
	controller.StartTemplateRefresh()

	// SPEC 098: до этой версии профиль удалённой машины был один на всех
	// (bin/wizard_states/remote/state.json + bin/remote-config.json). Отдаём
	// его владельцу, пока никто не начал читать новые пути — иначе первая же
	// машина откроется с пустым состоянием, а настроенный конфиг останется
	// лежать файлом, на который больше никто не смотрит.
	//
	// Non-fatal: при неудаче старые файлы остаются на месте нетронутыми.
	if err := services.MigrateLegacyRemoteProfile(layout.Data,
		services.NewRemoteRegistry(layout.Data)); err != nil {
		debuglog.WarnLog("remote migration: %v", err)
	}

	// Load locale settings and external translations. Каталоги двух уровней
	// (SPEC 135 §3.3): поставляемые рядом с бинарём, затем скачанные в
	// DataDir — поздний перекрывает. В portable-раскладке это один каталог.
	binDir := layout.Data.Bin()
	locale.LoadExternalLocales(locale.GetLocaleDir(layout.App.Bin()))
	if binDir != layout.App.Bin() {
		locale.LoadExternalLocales(locale.GetLocaleDir(binDir))
	}
	settings := locale.LoadSettings(binDir)
	locale.SetLang(settings.Lang)
	// Самолечение каталога после апдейта (SPEC 111): апдейт меняет только
	// бинарь, bin/locale/ остаётся старым — для не-английского языка после
	// смены версии докачиваем каталоги в фоне один раз. Non-fatal и не
	// блокирует старт (конституция: первый запуск не зависит от сети);
	// уже построенные окна дорисуются на новом языке после перезапуска.
	if settings.Lang != "en" && settings.LastLocaleLauncherVersion != constants.AppVersion {
		go func() {
			// Каталог уже нового формата (ключ-сентинел на месте) — сеть не
			// нужна, только штамп версии. Заодно закрывает окно, когда код
			// новее содержимого GitHub-ветки: свежий локальный файл не
			// перетирается протухшим удалённым.
			if locale.CatalogHasKey(settings.Lang, "Local") {
				if err := locale.MarkLocalesRefreshed(binDir, constants.AppVersion); err != nil {
					debuglog.WarnLog("locale refresh: persist version: %v", err)
				}
				return
			}
			localeDir := locale.GetLocaleDir(binDir)
			if n, err := locale.DownloadAllRemoteLocales(localeDir); err != nil {
				debuglog.WarnLog("locale refresh after upgrade: %v (downloaded %d)", err, n)
				return
			}
			if err := locale.MarkLocalesRefreshed(binDir, constants.AppVersion); err != nil {
				debuglog.WarnLog("locale refresh: persist version: %v", err)
			}
			debuglog.InfoLog("locale refresh after upgrade: catalogs updated for %s", constants.AppVersion)
		}()
	}
	if settings.PingTestURL != "" {
		api.SetPingTestURL(settings.PingTestURL)
	}
	if settings.PingTestAllConcurrency != 0 {
		api.SetPingTestAllConcurrency(settings.PingTestAllConcurrency)
	}
	if settings.PingTestTimeoutMs != 0 {
		api.SetPingTestTimeoutMs(settings.PingTestTimeoutMs)
	}
	// Honor persisted opt-out of subscription auto-update. The loop is started
	// unconditionally later; this just flips the in-memory gate so it skips work.
	if settings.SubscriptionAutoUpdateDisabled {
		controller.StateService.SetAutoUpdateEnabled(false)
		debuglog.InfoLog("Auto-update: disabled by user setting (subscription_auto_update_disabled=true)")
	}
	// Пункт трея «Скрыть из Dock» (macOS, issue #112). Флаг ставим здесь, до
	// CreateTrayMenu: иначе галка в меню при старте не отражала бы сохранённое
	// состояние. Сама activation policy применяется ниже, перед RunEventLoop.
	if runtime.GOOS == "darwin" && settings.HideAppFromDock {
		controller.UIService.HideAppFromDock = true
		debuglog.InfoLog("Dock: hidden at startup by user setting (hide_app_from_dock=true)")
	}
	if settings.AutoPingAfterConnectDisabled {
		controller.StateService.SetAutoPingAfterConnectEnabled(false)
		debuglog.InfoLog("Auto-ping: disabled by user setting (auto_ping_after_connect_disabled=true)")
	}
	if settings.AutoPingAfterConnectMaxProxies > 0 {
		controller.StateService.SetAutoPingMaxProxies(settings.AutoPingAfterConnectMaxProxies)
		debuglog.InfoLog("Auto-ping: max-proxies cap overridden by user setting (auto_ping_after_connect_max_proxies=%d)", settings.AutoPingAfterConnectMaxProxies)
	}
	// Optional debug-API (localhost:9263 by default). Off unless user toggled
	// it on in the Diagnostics tab; token is generated on first enable.
	if settings.DebugAPIEnabled && settings.DebugAPIToken != "" {
		if err := controller.StartDebugAPI(settings.DebugAPIPort, settings.DebugAPIToken); err != nil {
			debuglog.WarnLog("debug-api: failed to start: %v", err)
		}
	}
	debuglog.WarnLog("Locale: language set to %q, available: %v", locale.GetLang(), locale.Languages())

	// Check launcher version on startup (always checks, popup shown on first window display)
	controller.CheckLauncherVersionOnStartup()

	// SPEC 059: spin up the always-on Traffic Profiler service. It tails
	// sing-box.log + polls Clash /connections in the background so when
	// the user opens the Traffic Profiler window from Diagnostics they
	// immediately see the rolling 60s buffer instead of waiting for
	// events. Recording sessions survive window close.
	ui.EnsureTrafficProfilerStarted(controller)

	// Configure the system tray if the application is running on a Desktop platform.
	//nolint:unused // desktop is used for type assertion, even if linter can't detect it
	if desk, ok := controller.UIService.Application.(desktop.App); ok {
		debuglog.InfoLog("System tray: Desktop platform detected, initializing...")
		// Create the menu update function for the system tray with proxy selection submenu
		// Safe wrapper with debounce to prevent "Invalid menu handle" errors
		// when menu updates happen too quickly
		updateTrayMenu := func() {
			controller.UIService.TrayMenuUpdateMutex.Lock()
			defer controller.UIService.TrayMenuUpdateMutex.Unlock()

			// Cancel previous timer if it exists
			if controller.UIService.TrayMenuUpdateTimer != nil {
				controller.UIService.TrayMenuUpdateTimer.Stop()
			}

			// Calculate dynamic delay based on number of proxies
			// Get proxy count to determine appropriate delay
			var proxyCount int
			if controller.APIService != nil {
				proxyCount = len(controller.APIService.GetProxiesList())
			}

			// Dynamic delay formula:
			// - Base delay: 100ms for small menus (0-10 proxies)
			// - For each proxy above 10, add 20ms
			// - Maximum delay: 500ms to ensure systray has enough time for large menus
			delay := 100 * time.Millisecond
			if proxyCount > 10 {
				extraDelay := time.Duration(proxyCount-10) * 20 * time.Millisecond
				delay += extraDelay
				// Cap at 500ms maximum
				if delay > 500*time.Millisecond {
					delay = 500 * time.Millisecond
				}
			}

			// Create new timer with dynamic debounce delay
			// This prevents rapid successive menu updates that cause systray errors
			controller.UIService.TrayMenuUpdateTimer = time.AfterFunc(delay, func() {
				if platform.IsSleeping() {
					return // Skip tray menu update while system is sleeping
				}
				// Check if update is already in progress
				controller.UIService.TrayMenuUpdateMutex.Lock()
				if controller.UIService.TrayMenuUpdateInProgress {
					controller.UIService.TrayMenuUpdateMutex.Unlock()
					return // Skip update if already in progress
				}
				controller.UIService.TrayMenuUpdateInProgress = true
				controller.UIService.TrayMenuUpdateMutex.Unlock()

				fyne.Do(func() {
					defer func() {
						// Reset flag after update completes
						controller.UIService.TrayMenuUpdateMutex.Lock()
						controller.UIService.TrayMenuUpdateInProgress = false
						controller.UIService.TrayMenuUpdateTimer = nil
						controller.UIService.TrayMenuUpdateMutex.Unlock()
					}()

					menu := controller.CreateTrayMenu()
					// Use recover to handle any panics during menu update
					func() {
						defer func() {
							if r := recover(); r != nil {
								debuglog.ErrorLog("updateTrayMenu: Recovered from panic: %v", r)
							}
						}()
						desk.SetSystemTrayMenu(menu)
					}()
				})
			})
		}
		controller.UIService.UpdateTrayMenuFunc = updateTrayMenu

		// Initialize system tray immediately (required on macOS, works on Windows too)
		// On macOS, system tray must be initialized BEFORE app.Run() to work properly
		debuglog.InfoLog("System tray: Setting icon...")
		desk.SetSystemTrayIcon(controller.UIService.GreyIconData)
		debuglog.InfoLog("System tray: Creating initial menu...")
		initialMenu := controller.CreateTrayMenu()
		desk.SetSystemTrayMenu(initialMenu)
		debuglog.InfoLog("System tray: Icon and menu initialized successfully")

		// Set a handler that fires when the application is fully ready
		controller.UIService.Application.Lifecycle().SetOnStarted(func() {
			// Маркер живого цикла событий: до SPEC 125 успешный старт не писал
			// ни одной WARN-строки, и по релизному логу нельзя было отличить
			// «дошли до UI» от «умерли на инициализации GL».
			debuglog.WarnLog("ui: event loop started")

			// OnStarted срабатывает до первого кадра, поэтому засчитывать
			// «отрисовано» прямо здесь нельзя: падение под Mesa приходило через
			// 1–2 секунды ПОСЛЕ появления окна. 3 секунды прожитой жизни —
			// практический признак, что кадр действительно нарисован; умерли
			// раньше — в bin/gl-state.json останется phase=starting, и
			// следующий старт переспросит про OpenGL (SPEC 125 §2.1).
			time.AfterFunc(glRenderedGrace, func() {
				platform.MarkGLRendered(layout.Data)
			})

			// Сценарий возврата на железо (SPEC 125 §2.5): гейт в режиме mesa
			// пробует железо в фоне; если оно появилось — спрашиваем один раз
			// на каждый новый renderer.
			platform.SetOnHardwareGLAvailable(func(renderer string) {
				fyne.Do(func() {
					win := controller.UIService.MainWindow
					if win == nil {
						return
					}
					confirm := dialog.NewConfirm(
						locale.T("OpenGL"),
						locale.Tf("Hardware OpenGL is now available:\n%s\nDisable Mesa3D and use it? The launcher will restart now.", renderer),
						func(yes bool) {
							rememberOfferedRenderer(layout.Data, renderer)
							if !yes {
								return
							}
							if err := platform.DisableMesa(layout.App); err != nil {
								debuglog.ErrorLog("gl: disable Mesa3D from UI failed: %v", err)
								ui.ShowError(win, err)
								return
							}
							// Тот же путь, что у кнопки Диагностики (SPEC 125 §6.2 R2/R5):
							// opengl32.dll отображён загрузчиком при старте процесса, и
							// переключение применит только новый процесс. Выходим штатно —
							// ядро остановится, логи закроются, RestartSelf в конце main().
							platform.UpdateGLState(layout.Data, func(s *platform.GLState) {
								s.Phase = platform.GLPhaseRestart
								s.Mode = platform.GLModeHardware
							})
							debuglog.WarnLog("gl: restarting to apply hardware (return dialog)")
							platform.RequestRestartAfterExit()
							controller.GracefulExit()
						}, win)
					confirm.SetConfirmText(locale.T("Yes"))
					confirm.SetDismissText(locale.T("Later"))
					confirm.Show()
				})
			})

			// Read state at startup (informational log only). state.json is the
			// canonical source of parser_config since SPEC 045; reading from
			// config.json's @ParserConfig comment-block is gone (block removed
			// during the same cleanup).
			go func() {
				debuglog.InfoLog("Application startup: Reading state...")
				statePath := platform.GetWizardStatePath(layout.Data)
				s, err := state.Load(statePath)
				if err != nil {
					debuglog.WarnLog("Application startup: state.json not loaded: %v", err)
					return
				}
				// Счётчики — из canonical s.Connections (SPEC 117): Load-проекция
				// s.ParserConfig — только для build-путей, читать её здесь незачем.
				debuglog.WarnLog("Application startup: state.json loaded (schema v%d, %d sources, %d outbounds, %d custom rules)",
					s.Version,
					len(s.Sources),
					len(s.Directions),
					len(s.CustomRules))
			}()

			// SPEC 135 §3.4: данных предыдущей версии нет и переносить нечего —
			// один раз подсказать про LX Backup.
			if firstRunNoticeDue(controller.FileService, settings) {
				scheduleFirstRunNotice(controller, layout.Data, *startInTray)
			}
			// Данные в системном каталоге, скрытые portable.txt (новый zip
			// распакован поверх папки после выключения Portable).
			if dir, found := hiddenDataNoticeDue(layout, exe, settings); found {
				debuglog.WarnLog("storage: portable mode, but settings were found in %s", dir)
				scheduleHiddenDataNotice(controller, layout, dir, *startInTray)
			}
			// SPEC 136: служба демона на файле, который может подменить
			// пользователь (plist до lx.11) — предупредить раз на версию.
			if servicePath, command, coreHint, due := controller.DaemonUnsafeServiceNotice(); due {
				scheduleDaemonUnsafeNotice(controller, layout.Data, servicePath, command, coreHint, *startInTray)
			}

			// Auto-start VPN if -start flag is provided
			if *autoStart {
				go func() {
					// Wait a bit for everything to initialize
					<-time.After(autoStartDelay)
					debuglog.InfoLog("Auto-start: Starting VPN due to -start parameter")
					core.StartSingBoxProcess()
				}()
			}

			// Hide window if -tray flag is provided
			if *startInTray {
				go func() {
					// Wait a bit for window to be fully initialized
					<-time.After(500 * time.Millisecond)
					fyne.Do(func() {
						if controller.UIService.MainWindow != nil {
							controller.UIService.HideMainWindow()
							debuglog.InfoLog("Tray mode: Window hidden")
						}
					})
				}()
			}
		})
	}

	// Повышенный экземпляр виден по заголовку (SPEC 139 §3): TUN и очистки
	// в нём работают, а автозапуск и Portable недоступны.
	windowTitle := "Singbox Launcher"
	if runtime.GOOS == "windows" && platform.IsElevated() {
		windowTitle += " (" + locale.T("Administrator") + ")"
	}
	controller.UIService.MainWindow = controller.UIService.Application.NewWindow(windowTitle) // Create the main application window
	controller.UIService.MainWindow.SetIcon(controller.UIService.AppIconData)

	// Create App structure to manage UI
	app := ui.NewApp(controller.UIService.MainWindow, controller)
	controller.UIService.MainWindow.SetContent(fynetooltip.AddWindowToolTipLayer(
		ui.WithMinWindowSize(app.GetContent()), controller.UIService.MainWindow.Canvas()))
	// SPEC 098 §3.2: 1000×700 — и стартовый размер, и минимум. Растянуть
	// можно, сжать ниже нельзя: на вкладках Local и Remote две колонки, и
	// ниже этого они не деградируют, а превращаются в кашу из обрезанных
	// подписей. Прежние 350×450 остались от одноколоночной эпохи.
	//
	// SetFixedSize здесь неприменим (он запрещает и растягивание); нижнюю
	// границу держит MinSize контента — Fyne не даёт окну стать меньше него.
	controller.UIService.MainWindow.Resize(ui.MinWindowSize)
	fynewidget.CenterOnScreen(controller.UIService.MainWindow) // Center the window on the screen

	core.CheckIfLauncherAlreadyRunningUtil()

	// Intercept the window close event (clicking "X") to hide it instead of exiting completely.
	if controller.UIService.MainWindow != nil {
		controller.UIService.MainWindow.SetCloseIntercept(func() {
			controller.UIService.HideMainWindow()
			if controller.UIService.HideAppFromDock {
				platform.HideDockIcon()
			}
		})
	}

	// Handle Dock icon click on macOS - show window when app is activated
	// This makes Dock icon behave like "Open" in tray menu
	// Platform-specific: macOS only (Dock is macOS-specific)
	// Uses native NSApplicationDelegate to handle applicationShouldHandleReopen
	// This is a workaround for Fyne issue #3845 (Dock click not showing hidden window)
	if runtime.GOOS == "darwin" {
		// Тот же делегат получает запрос macOS на завершение: Cmd+Q,
		// «Завершить» в Dock, выход из системы. Выход идёт через GracefulExit,
		// как Quit в трее; без обработчика AppKit завершал процесс сразу, и в
		// classic-режиме ядро оставалось работать без лаунчера.
		platform.SetQuitRequestHandler(func() {
			debuglog.WarnLog("Application shutting down: macOS quit request (Cmd+Q, Dock menu, log out or shut down).")
			controller.GracefulExit()
		}, macQuitBudget)
		platform.SetupDockReopenHandler(func() {
			fyne.Do(func() {
				// Show() is safe to call even if window is already visible
				if controller.UIService != nil {
					// Ensure Dock is restored before showing the window
					platform.RestoreDockIcon()
					controller.UIService.ShowMainWindowOrFocusWizard()
					debuglog.InfoLog("Dock icon clicked (native handler): Dock restored and focused (main or wizard)")
				}
			})
		})
		debuglog.InfoLog("Dock icon click handler registered for macOS (native NSApplicationDelegate)")
	}

	controller.UpdateUI()

	// Reset Clash API HTTP connections after sleep/hibernation and re-sync UI.
	// Platform no-op where not supported (darwin/linux stubs).
	//
	// Problem: after close-lid / open-lid the launcher would often show stale
	// ping numbers and a silent Clash API silence because Windows closes TCP
	// connections during sleep. ResetClashHTTPTransport handles the sockets;
	// we add a small delay-then-refresh dance so the UI visibly reconciles
	// once network is actually back (wake event typically fires before
	// interfaces are fully up).
	platform.RegisterPowerResumeCallback(func() {
		api.ResetClashHTTPTransport()
		debuglog.InfoLog("Power resume: Clash API HTTP transport reset, scheduling re-sync")
		// Give network interfaces a couple seconds to come back, then:
		//   - Re-test the Clash API connection (RefreshAPIFunc → onTestAPIConnection
		//     in clash_api_tab.go — this also reloads the proxies list).
		//   - Trigger the auto-ping-after-connect hook if sing-box is running,
		//     so latency numbers get refreshed instead of staying stuck at
		//     pre-sleep values.
		time.AfterFunc(3*time.Second, func() {
			if controller.UIService != nil && controller.UIService.RefreshAPIFunc != nil {
				// RefreshAPIFunc (onTestAPIConnection) mutates labels directly
				// before dispatching its goroutine, so call from UI thread.
				fyne.Do(controller.UIService.RefreshAPIFunc)
			}
			if controller.RunningState != nil && controller.RunningState.IsRunning() &&
				controller.StateService != nil && controller.StateService.IsAutoPingAfterConnectEnabled() &&
				controller.UIService != nil && controller.UIService.AutoPingAfterConnectFunc != nil {
				// Another 2s beyond the Refresh so /proxies has repopulated.
				time.AfterFunc(2*time.Second, func() {
					if !controller.RunningState.IsRunning() {
						return
					}
					// Soft-cap (mirror of RunningState.Set timer): skip auto-ping
					// on huge subscriptions — same TUN-stampede risk as the
					// connect-time path. SPEC 039 §1.3.
					if max := controller.StateService.GetAutoPingMaxProxies(); max > 0 {
						if count := len(controller.GetProxiesList()); count > max {
							debuglog.InfoLog("auto-ping: skipped post-resume (proxies=%d > cap=%d); use Servers → Test or Cmd/Ctrl+P for manual ping", count, max)
							return
						}
					}
					debuglog.DebugLog("Power resume: triggering post-resume auto-ping")
					// AutoPingAfterConnectFunc already wraps pingAllProxies
					// in fyne.Do internally (clash_api_tab.go registers it
					// that way), but double-wrapping is a harmless no-op if
					// we're already on the UI thread and cheap insurance
					// otherwise.
					fyne.Do(controller.UIService.AutoPingAfterConnectFunc)
				})
			}
		})
	})

	// Check if config.json exists and show a warning if it doesn't
	core.CheckConfigFileExists()

	// Check Linux capabilities and suggest setup if needed
	core.CheckLinuxCapabilities()

	// Check if sing-box is running on startup and show a warning if it is.
	core.CheckIfSingBoxRunningAtStartUtil()

	// Win7: remove accumulated singbox-tun ghosts from prior sessions (SPEC 065).
	core.CleanupStaleTunAtStartUtil()

	// Применяем сохранённое «Скрыть из Dock» (issue #112) — до показа окна,
	// чтобы иконка не успела мигнуть в Dock.
	//
	// Место выбрано под требование AppKit: -[NSApp setActivationPolicy:] зовётся
	// только с главного потока, и launcherHideDockIcon сам на него не уходит.
	// Здесь это выполнено даром: драйвер Fyne в своём init() делает
	// runtime.LockOSThread, так что main.main и есть главный поток (загонять
	// вызов в fyne.Do нельзя — цикл событий стартует строчкой ниже, и очередь
	// разобралась бы уже после того, как окно показано).
	//
	// Окно при старте ПОКАЗЫВАЕМ, скрыта только иконка Dock (решение владельца
	// 18.09.2026): если значок трея по какой-то причине не появился, без окна
	// до запущенного лаунчера было бы не добраться. Старт без окна — только по
	// явному флагу -tray.
	hiddenFromDock := runtime.GOOS == "darwin" && controller.UIService != nil && controller.UIService.HideAppFromDock
	if hiddenFromDock {
		platform.HideDockIcon()
	}

	// Use app.Run() instead of ShowAndRun() for windowless support
	// This allows the app to keep running even when window is closed/hidden
	// On macOS, this enables standard Dock behavior (applicationShouldHandleReopen)
	// See: https://github.com/fyne-io/fyne/issues/3845
	if !*startInTray {
		// Show window on startup if not starting in tray
		if controller.UIService != nil {
			controller.UIService.ShowMainWindowOrFocusWizard()
		}
	}

	// Start the application event loop (windowless mode)
	// This keeps the app running even when window is hidden/closed
	// The menu already has "Open" item that calls MainWindow.Show()
	controller.UIService.RunEventLoop()

	// The code below executes only after the event loop has ended. This is
	// where final cleanup is performed.
	//
	// Цикл гасит либо наш GracefulExit (трей, Exit, перезапуск рендерера) —
	// тогда остановка уже прошла, и вызов ниже ничего не делает, — либо сам
	// драйвер, например по SIGTERM: тогда ядро и логи останавливаются только
	// здесь, а Quit драйверу уже не нужен (UIService.RunEventLoop).
	if controller.IsExiting() {
		debuglog.WarnLog("Application shutting down.")
	} else {
		debuglog.WarnLog("Application shutting down: event loop stopped by the driver (signal or system close request).")
	}

	// Cleanup platform-specific handlers
	if runtime.GOOS == "darwin" {
		platform.CleanupDockReopenHandler()
	}
	platform.StopPowerResumeListener()

	controller.GracefulExit()

	// Close log files through FileService
	if controller.FileService != nil {
		if controller.FileService.MainLogFile != nil {
			debuglog.RunAndLog("main: close main log file", controller.FileService.MainLogFile.Close)
		}
		if controller.FileService.ChildLogFile != nil {
			debuglog.RunAndLog("main: close child log file", controller.FileService.ChildLogFile.Close)
		}
		if controller.FileService.ApiLogFile != nil {
			api.SetAPILogFile(nil)
			debuglog.RunAndLog("main: close API log file", controller.FileService.ApiLogFile.Close)
		}
	}

	// Переключение рендерера из Диагностики (SPEC 125 §6.2 R5): применить его
	// может только новый процесс — opengl32.dll отображается загрузчиком
	// Windows при создании процесса, по таблице импорта exe. Поднимаемся здесь,
	// а не в колбэке кнопки: к этому моменту ядро остановлено и логи закрыты,
	// так что новый лаунчер не наткнётся на живой sing-box старого.
	if platform.RestartRequested() {
		if err := platform.RestartSelf(); err != nil {
			debuglog.ErrorLog("main: self-restart failed: %v", err)
		}
	}
}
