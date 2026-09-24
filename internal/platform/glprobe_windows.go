//go:build windows
// +build windows

package platform

// Issue #105: в RDP-сессии Windows Server (и на ВМ без GPU) система отдаёт
// только «GDI Generic» OpenGL 1.1, а Fyne/GLFW нужен контекст 2.1 — главное
// окно молча не отрисовывается, живым остаётся лишь трей. Тот же класс
// проблемы, что и docs/WIN7_OPENGL.md, где решение — положить Mesa3D
// (llvmpipe) opengl32.dll рядом с exe.
//
// Здесь это автоматизировано:
//
//  1. До инициализации GL-части Fyne лаунчер перезапускает сам себя с флагом
//     -gl-probe: подпроцесс создаёт скрытое окно, WGL-контекст и печатает
//     версию/renderer/vendor. Подпроцесс нужен, чтобы родитель не грузил
//     системный opengl32.dll и чтобы падение кривого драйвера не унесло
//     основной процесс.
//  2. Если версия < 2.1 — нативный MessageBox (Fyne-окно показать нечем)
//     предлагает скачать Mesa3D с релиза лаунчера и распаковать DLL рядом
//     с exe.
//  3. Любая смена рендерера применяется только через перезапуск процесса.
//     OPENGL32.dll стоит в таблице импорта exe (разбор PE, SPEC 125 §6.1 F1):
//     загрузчик Windows отображает его при создании процесса, раньше любой
//     строки Go. Подменить или выгрузить его внутри живого процесса нельзя —
//     прогон RC 13.09.2026 показал, что после переименования DLL старт
//     продолжался с уже отображённой Mesa и падал в glTexImage2D.

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/paths"

	"golang.org/x/sys/windows"
)

const (
	// Собственный релиз-зеркало Mesa3D (llvmpipe) в репозитории лаунчера:
	// mesa-dist-win отдаёт только .7z, который stdlib не распакует, поэтому
	// нужные DLL переупакованы в zip. Релиз помечен prerelease, чтобы не
	// светиться как «latest» для проверки обновлений лаунчера.
	mesaReleaseTag = "mesa3d-26.2.0"
	mesaAssetName  = "mesa3d-26.2.0-win64.zip"

	mesaDownloadCap  = 100 * 1024 * 1024 // байт; реальный ассет ~24 МБ
	rdpOpenGLDocURL  = "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/RDP_OPENGL.md"
	win7OpenGLDocURL = "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/WIN7_OPENGL.md"

	// Диалоги гейта — только английский (SPEC 125 §2.3): locale.SetLang
	// вызывается в main.go ниже гейта, переводов на этот момент ещё нет.
	dlgTitle = "Singbox Launcher — OpenGL"

	// installerMesaTask — подпись задачи Mesa в установщике (SPEC 140 §3.3,
	// build/installer/singbox-launcher.iss, [CustomMessages] TaskMesa).
	installerMesaTask = "Software OpenGL (Mesa3D) for RDP / VM without GPU"

	// mesaPinnedDriver — драйвер Gallium, который лаунчер пинит через
	// GALLIUM_DRIVER. В ассете лежат все драйверы (d3d12, zink, llvmpipe), и
	// без пина Mesa на машине с видеокартой уходит в d3d12 — то есть в тот же
	// GPU-стек, который её и потребовал. Он же — маркер успеха локальной пробы.
	mesaPinnedDriver = "llvmpipe"
)

// Win32-константы (user32/gdi32/wgl).
const (
	wsPopup            = 0x80000000
	pfdDrawToWindow    = 0x00000004
	pfdSupportOpenGL   = 0x00000020
	pfdDoubleBuffer    = 0x00000001
	pfdTypeRGBA        = 0
	glVersionEnum      = 0x1F02
	glRendererEnum     = 0x1F01
	glVendorEnum       = 0x1F00
	mbYesNo            = 0x00000004
	mbOK               = 0x00000000
	mbAbortRetryIgnore = 0x00000002
	mbIconWarning      = 0x00000030
	mbIconError        = 0x00000010
	mbIconInformation  = 0x00000040
	mbTopmost          = 0x00040000
	mbSetForeground    = 0x00010000
	idYes              = 6
	idAbort            = 3
	idRetry            = 4
	idIgnore           = 5
)

// pixelFormatDescriptor — PIXELFORMATDESCRIPTOR из wingdi.h (40 байт).
type pixelFormatDescriptor struct {
	Size                                                          uint16
	Version                                                       uint16
	Flags                                                         uint32
	PixelType                                                     byte
	ColorBits                                                     byte
	RedBits, RedShift, GreenBits, GreenShift, BlueBits, BlueShift byte
	AlphaBits, AlphaShift                                         byte
	AccumBits                                                     byte
	AccumRedBits, AccumGreenBits, AccumBlueBits, AccumAlphaBits   byte
	DepthBits, StencilBits                                        byte
	AuxBuffers                                                    byte
	LayerType                                                     byte
	Reserved                                                      byte
	LayerMask, VisibleMask, DamageMask                            uint32
}

// RunGLProbeChild — тело подпроцесса `singbox-launcher.exe -gl-probe`
// (local=false, системный OpenGL) или `-gl-probe-local` (local=true,
// opengl32.dll рядом с exe, то есть установленная Mesa3D).
// Печатает в stdout строки version=/renderer=/vendor= и завершает процесс.
// Никогда не возвращается.
func RunGLProbeChild(app paths.AppDir, local bool) {
	version, renderer, vendor, err := probeDesktopOpenGL(app, local)
	if err != nil {
		fmt.Printf("error=%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("version=%s\nrenderer=%s\nvendor=%s\n", version, renderer, vendor)
	os.Exit(0)
}

// glProcSource — источник процедур OpenGL для пробы: либо системный
// opengl32.dll, либо конкретный файл рядом с exe.
type glProcSource interface {
	proc(name string) *windows.Proc
}

// systemGLSource — системный opengl32.dll (NewLazySystemDLL ищет только в
// system32, локальная Mesa рядом с exe им принципиально не подхватывается).
type systemGLSource struct{ dll *windows.LazyDLL }

func (s systemGLSource) proc(name string) *windows.Proc {
	// LazyProc и Proc несовместимы по типу, поэтому системный источник
	// резолвит через загруженный модуль — Load() уже вызван выше.
	h := windows.Handle(s.dll.Handle())
	d := &windows.DLL{Name: s.dll.Name, Handle: h}
	p, err := d.FindProc(name)
	if err != nil {
		return nil
	}
	return p
}

// localGLSource — opengl32.dll, загруженный по полному пути из каталога exe.
type localGLSource struct{ dll *windows.DLL }

func (s localGLSource) proc(name string) *windows.Proc {
	p, err := s.dll.FindProc(name)
	if err != nil {
		return nil
	}
	return p
}

// probeDesktopOpenGL создаёт скрытое окно и legacy-WGL-контекст, читает
// GL_VERSION/GL_RENDERER/GL_VENDOR. Выполняется только в подпроцессе.
//
// local=true — SPEC 125 §1.4: обычная проба через NewLazySystemDLL всегда
// измеряет аппаратный GL, даже когда рядом с exe лежит Mesa (system32 — её
// единственный каталог поиска). Чтобы проверить саму установленную Mesa,
// нужен LoadLibraryEx по полному пути с LOAD_WITH_ALTERED_SEARCH_PATH и
// процедуры от этого хендла.
func probeDesktopOpenGL(app paths.AppDir, local bool) (version, renderer, vendor string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	user32 := windows.NewLazySystemDLL("user32.dll")
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")

	registerClassExW := user32.NewProc("RegisterClassExW")
	createWindowExW := user32.NewProc("CreateWindowExW")
	destroyWindow := user32.NewProc("DestroyWindow")
	defWindowProcW := user32.NewProc("DefWindowProcW")
	getDC := user32.NewProc("GetDC")
	releaseDC := user32.NewProc("ReleaseDC")
	getModuleHandleW := kernel32.NewProc("GetModuleHandleW")
	choosePixelFormat := gdi32.NewProc("ChoosePixelFormat")
	setPixelFormat := gdi32.NewProc("SetPixelFormat")

	var src glProcSource
	if local {
		dllPath := filepath.Join(string(app), "opengl32.dll")
		// LOAD_WITH_ALTERED_SEARCH_PATH: зависимости Mesa (libgallium_wgl.dll)
		// резолвятся из каталога самого opengl32.dll, а не из system32.
		h, loadErr := windows.LoadLibraryEx(dllPath, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
		if loadErr != nil {
			return "", "", "", fmt.Errorf("load %s: %w", dllPath, loadErr)
		}
		src = localGLSource{dll: &windows.DLL{Name: dllPath, Handle: h}}
	} else {
		opengl32 := windows.NewLazySystemDLL("opengl32.dll")
		if loadErr := opengl32.Load(); loadErr != nil {
			return "", "", "", fmt.Errorf("opengl32.dll not loadable: %w", loadErr)
		}
		src = systemGLSource{dll: opengl32}
	}

	wglCreateContext := src.proc("wglCreateContext")
	wglMakeCurrent := src.proc("wglMakeCurrent")
	wglDeleteContext := src.proc("wglDeleteContext")
	glGetString := src.proc("glGetString")
	if wglCreateContext == nil || wglMakeCurrent == nil || wglDeleteContext == nil || glGetString == nil {
		return "", "", "", fmt.Errorf("opengl32.dll is missing WGL entry points")
	}

	mod, _, _ := getModuleHandleW.Call(0)
	className, _ := windows.UTF16PtrFromString("SingboxGLProbe")
	wc := struct {
		Size, Style                        uint32
		WndProc                            uintptr
		ClsExtra, WndExtra                 int32
		Instance, Icon, Cursor, Background uintptr
		MenuName, ClassName                *uint16
		IconSm                             uintptr
	}{
		WndProc:   defWindowProcW.Addr(),
		Instance:  mod,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if cls, _, callErr := registerClassExW.Call(uintptr(unsafe.Pointer(&wc))); cls == 0 {
		return "", "", "", fmt.Errorf("RegisterClassExW: %v", callErr)
	}

	hwnd, _, callErr := createWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		wsPopup,
		0, 0, 1, 1,
		0, 0, mod, 0,
	)
	if hwnd == 0 {
		return "", "", "", fmt.Errorf("CreateWindowExW: %v", callErr)
	}
	defer destroyWindow.Call(hwnd) //nolint:errcheck // cleanup

	hdc, _, callErr := getDC.Call(hwnd)
	if hdc == 0 {
		return "", "", "", fmt.Errorf("GetDC: %v", callErr)
	}
	defer releaseDC.Call(hwnd, hdc) //nolint:errcheck // cleanup

	pfd := pixelFormatDescriptor{
		Version:   1,
		Flags:     pfdDrawToWindow | pfdSupportOpenGL | pfdDoubleBuffer,
		PixelType: pfdTypeRGBA,
		ColorBits: 24,
		DepthBits: 16,
	}
	pfd.Size = uint16(unsafe.Sizeof(pfd))
	format, _, callErr := choosePixelFormat.Call(hdc, uintptr(unsafe.Pointer(&pfd)))
	if format == 0 {
		return "", "", "", fmt.Errorf("ChoosePixelFormat: %v", callErr)
	}
	if ok, _, callErr := setPixelFormat.Call(hdc, format, uintptr(unsafe.Pointer(&pfd))); ok == 0 {
		return "", "", "", fmt.Errorf("SetPixelFormat: %v", callErr)
	}

	ctx, _, callErr := wglCreateContext.Call(hdc)
	if ctx == 0 {
		return "", "", "", fmt.Errorf("wglCreateContext: %v", callErr)
	}
	defer wglDeleteContext.Call(ctx) //nolint:errcheck // cleanup
	if ok, _, callErr := wglMakeCurrent.Call(hdc, ctx); ok == 0 {
		return "", "", "", fmt.Errorf("wglMakeCurrent: %v", callErr)
	}
	defer wglMakeCurrent.Call(0, 0) //nolint:errcheck // cleanup

	// Строку из glGetString читаем без uintptr→unsafe.Pointer конверсий
	// (их не пропускает go vet): длина — lstrlenA, копия — RtlMoveMemory.
	lstrlenA := kernel32.NewProc("lstrlenA")
	rtlMoveMemory := kernel32.NewProc("RtlMoveMemory")
	readGLString := func(name uintptr) string {
		ptr, _, _ := glGetString.Call(name)
		if ptr == 0 {
			return ""
		}
		n, _, _ := lstrlenA.Call(ptr)
		if n == 0 {
			return ""
		}
		if n > 1024 {
			n = 1024
		}
		buf := make([]byte, n)
		rtlMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), ptr, n) //nolint:errcheck // void API
		return string(buf)
	}

	version = readGLString(glVersionEnum)
	if version == "" {
		return "", "", "", fmt.Errorf("glGetString(GL_VERSION) returned empty")
	}
	return version, readGLString(glRendererEnum), readGLString(glVendorEnum), nil
}

// EnsureDesktopOpenGL — гейт перед инициализацией GL-части Fyne (SPEC 125).
//
// Единственное правило: прошлый старт дошёл до первого кадра — пробу не
// делаем; иначе — делаем. «Иначе» = файла bin/gl-state.json нет (первый
// запуск) или в нём осталось phase=starting (процесс умер на инициализации
// GL). Решение — decideGate в glstate.go, здесь только I/O, диалоги и
// действия.
//
// interactive=false (-tray) — ни одного MessageBoxW ни в одной ветке: окна
// пользователь не ждёт, отвечать на диалог некому, и молча переключать
// рендерер тем более нельзя. Все решения уходят в лог.
//
// Никогда не роняет запуск: в худшем случае поведение прежнее (окно не
// откроется), но с внятным логом и подсказкой.
func EnsureDesktopOpenGL(l paths.Layout, interactive bool) {
	if os.Getenv("SINGBOX_LAUNCHER_NO_MESA") == "1" {
		debuglog.InfoLog("gl: gate skipped (SINGBOX_LAUNCHER_NO_MESA=1)")
		return
	}

	state, hasState := LoadGLState(l.Data)
	mesaInstalled := IsMesaInstalled(l.App)
	mode := GLModeHardware
	if mesaInstalled {
		mode = GLModeMesa
		// Пин драйвера — при КАЖДОМ старте с установленной Mesa, а не только
		// на пути установки. Полевой прогон RC 13.09.2026: Mesa, включённая
		// кнопкой Диагностики, на следующем старте загрузилась без пина, ушла
		// в d3d12 и уронила окно; спас только откат через D1. Здесь — до
		// любых подпроцессов и до того, как Fyne загрузит DLL.
		pinMesaDriver()
	}

	// Чужой одиночный opengl32.dll рядом с exe (не наша Mesa) — только WARN:
	// ни отключать, ни считать его Mesa мы не вправе.
	if HasForeignOpenGL(l.App) {
		debuglog.WarnLog("gl: a foreign opengl32.dll sits next to the exe (no libgallium_wgl.dll) — leaving it alone")
	}

	// «Прошлый старт умер именно под Mesa» — не то же самое, что «Mesa стоит».
	// На первом старте после обновления состояния ещё нет, и у RDP-машины с
	// уже установленной Mesa проба железа закономерно провалится: это норма,
	// а не поломка.
	prevDiedUnderMesa := hasState && state.Phase == GLPhaseStarting && state.Mode == GLModeMesa

	in := gateInput{
		State:             state,
		HasState:          hasState,
		MesaInstalled:     mesaInstalled,
		PrevDiedUnderMesa: prevDiedUnderMesa,
		Interactive:       interactive,
	}

	if decideGate(in) == actStart {
		// Прошлый старт дошёл до кадра. Если рисуем через Mesa — сказать об
		// этом громко (иначе факт программного рендеринга невидим) и в фоне
		// проверить, не появилось ли железо.
		if mode == GLModeMesa {
			debuglog.WarnLog("gl: rendering via local Mesa3D (driver=%s, renderer=%s) — hardware OpenGL is not in use; Diagnostics → \"Disable Mesa3D\" to switch",
				mesaActualDriver(), mesaVerifiedRenderer(state))
			startBackgroundHardwareProbe(l.App, state.OfferedHWRenderer)
		}
		MarkGLStarting(l.Data, mode)
		return
	}

	if hasState && state.Phase == GLPhaseStarting {
		debuglog.WarnLog("gl: previous start did not reach the first frame (mode=%s, version=%s) — re-checking OpenGL",
			state.Mode, state.LauncherVersion)
	}

	// Цикл ради Retry в D2: каждая итерация — отдельный клик пользователя,
	// поэтому ограничения на число повторов нет.
	for {
		probe := probeHardware(l.App)
		in.Probed = true
		in.Probe = probe
		if probe.ok() {
			debuglog.InfoLog("gl: hardware OpenGL %d.%d available (renderer=%q)", probe.Major, probe.Minor, probe.Renderer)
		} else {
			debuglog.WarnLog("gl: hardware OpenGL check failed (%s, renderer=%q) — Fyne needs %d.%d",
				probe.describe(), probe.Renderer, glMinMajor, glMinMinor)
		}

		switch decideGate(in) {
		case actStart:
			// Железа нет, но Mesa на месте — штатный режим RDP/ВМ без GPU.
			// Отдельная строка, чтобы в логе не выглядело, будто проба
			// провалилась впустую.
			if mesaInstalled && !in.Probe.ok() {
				debuglog.WarnLog("gl: hardware OpenGL unavailable (%s) — keeping the installed Mesa3D", in.Probe.describe())
				mode = GLModeMesa
			}
			MarkGLStarting(l.Data, mode)
			return

		case actAskDisableMesa:
			// D1: железо отвечает, а рисуем через Mesa. Формулировка зависит
			// от того, действительно ли Mesa сломалась: обвинять её в смерти
			// старта, которого не было, нельзя.
			text := fmt.Sprintf("Mesa3D is installed next to the exe,\n"+
				"but hardware OpenGL works on this machine:\n  %s\n\nDisable Mesa3D and use hardware OpenGL?", in.Probe.Renderer)
			if prevDiedUnderMesa {
				text = fmt.Sprintf("The previous start did not reach the window while rendering via Mesa3D,\n"+
					"but hardware OpenGL works on this machine:\n  %s\n\nDisable Mesa3D and use hardware OpenGL?", in.Probe.Renderer)
			}
			if messageBox(dlgTitle, text, mbYesNo|mbIconWarning|mbTopmost|mbSetForeground) == idYes {
				if err := DisableMesa(l.App); err != nil {
					debuglog.ErrorLog("gl: disable Mesa3D failed: %v", err)
				} else {
					// Продолжать этот старт бессмысленно: Mesa уже отображена
					// загрузчиком (см. шапку файла), и именно так RC умирал
					// второй раз подряд после «Yes».
					restartToApply(l.Data, GLModeHardware,
						"Mesa3D disabled — hardware OpenGL will be used.")
					mode = GLModeHardware
				}
			} else {
				debuglog.WarnLog("gl: user kept Mesa3D despite working hardware OpenGL (renderer=%q)", in.Probe.Renderer)
			}
			MarkGLStarting(l.Data, mode)
			return

		case actAskTimeout:
			// D2: таймаут — обычно временная икота драйвера, а не отсутствие
			// OpenGL. Именно слияние таймаута с отказом поставило Mesa на
			// машину с живой видеокартой (репорт 09.09.2026).
			text := "The OpenGL check did not finish within 10 seconds.\n" +
				"This is usually a temporary driver hiccup, not a missing OpenGL.\n\n" +
				"  Retry   — run the check again\n" +
				"  Ignore  — start with the system OpenGL as is\n" +
				"  Abort   — treat as \"no hardware OpenGL\" (offers Mesa3D / disables it)"
			switch messageBox(dlgTitle, text, mbAbortRetryIgnore|mbIconWarning|mbTopmost|mbSetForeground) {
			case idRetry:
				debuglog.WarnLog("gl: user chose Retry after probe timeout")
				continue
			case idAbort:
				debuglog.WarnLog("gl: user chose Abort after probe timeout — treating as no hardware OpenGL")
				// Абортом пользователь сам объявил железо нерабочим: дальше
				// идём веткой отказа, для чего подменяем исход пробы.
				in.Probe = probeResult{Err: fmt.Errorf("declared unusable by the user after a probe timeout")}
			default:
				debuglog.WarnLog("gl: user chose Ignore after probe timeout — starting with the system OpenGL")
				MarkGLStarting(l.Data, mode)
				return
			}

		case actAskInstallMesa, actNeitherWorks, actProbe:
			// Ниже — общая ветка отказа; actProbe сюда попасть не может
			// (in.Probed уже true), но switch обязан быть исчерпывающим.
		}

		// Ветка отказа железа.
		if decideGate(in) == actNeitherWorks {
			// Mesa уже стоит и всё равно не дошли до кадра — чинить нечем.
			debuglog.ErrorLog("gl: neither hardware OpenGL (%s) nor Mesa3D works — the window may stay blank", in.Probe.describe())
			if interactive {
				// Установка в Program Files (SPEC 140): Mesa туда кладёт задача
				// установщика — снять её можно только переустановкой без неё.
				reinstallHint := ""
				if !paths.AppDirUserWritable(string(l.App), os.Getenv, runtime.GOOS, paths.ProbeWritable) {
					reinstallHint = fmt.Sprintf("If Mesa3D came with the installer, run the installer again\n"+
						"without the task \"%s\".\n\n", installerMesaTask)
				}
				messageBox(dlgTitle, fmt.Sprintf(
					"Neither hardware OpenGL (%s) nor Mesa3D works on this machine.\n"+
						"The launcher will start, but the window may stay blank.\n\n%s"+
						"Details: logs/%s, logs/%s\nManual guide: %s",
					in.Probe.describe(), reinstallHint, constants.NativeStderrLogFileName, constants.MainLogFileName, rdpOpenGLDocURL),
					mbOK|mbIconError|mbTopmost|mbSetForeground)
			}
			MarkGLStarting(l.Data, mode)
			return
		}

		if !interactive {
			debuglog.WarnLog("gl: no hardware OpenGL (%s) and no Mesa3D, but the launcher runs with -tray — not touching any files", in.Probe.describe())
			MarkGLStarting(l.Data, mode)
			return
		}

		// Win7-сборка (386): готового x86-ассета нет, современная Mesa требует
		// Windows 10+. Отправляем к ручной инструкции.
		if runtime.GOARCH == "386" {
			messageBox(dlgTitle,
				"Hardware OpenGL 2.1 was not found — the application window cannot be rendered.\n"+
					"Manual Mesa3D guide:\n"+win7OpenGLDocURL,
				mbOK|mbIconWarning|mbTopmost|mbSetForeground)
			MarkGLStarting(l.Data, mode)
			return
		}

		// Каталог программы не пишется (установка в Program Files, SPEC 140
		// §8): положить DLL рядом с exe отсюда нельзя, копирование упало бы
		// после «Yes». Mesa в такой установке ставит сам установщик — задачей
		// на странице выбора задач. Предикат — общий с выбором раскладки:
		// повышенный экземпляр в Program Files тоже «не пишет».
		if !paths.AppDirUserWritable(string(l.App), os.Getenv, runtime.GOOS, paths.ProbeWritable) {
			debuglog.WarnLog("gl: no hardware OpenGL (%s); the program folder %s is read-only — Mesa3D install is left to the installer task", in.Probe.describe(), l.App)
			messageBox(dlgTitle, fmt.Sprintf("Hardware OpenGL 2.1 was not found (got %d.%d, renderer \"%s\").\n"+
				"The window cannot be rendered without it. This is typical for RDP sessions\n"+
				"and servers/VMs without a GPU.\n\n"+
				"The program folder is read-only, so Mesa3D cannot be installed from here.\n"+
				"Run the launcher installer again and select the task\n"+
				"\"%s\".\n\nManual guide: %s",
				in.Probe.Major, in.Probe.Minor, in.Probe.Renderer, installerMesaTask, rdpOpenGLDocURL),
				mbOK|mbIconWarning|mbTopmost|mbSetForeground)
			MarkGLStarting(l.Data, mode)
			return
		}

		// D3: единственная точка, где Mesa вообще может появиться на машине —
		// и только после «Yes». Прежняя версия ставила её из mesa3d/ молча,
		// что и убивало лаунчер на машинах с работающей видеокартой.
		download := ""
		if !HasMesaBundle(l.App) {
			download = "About 24 MB will be downloaded. Internet access required.\n"
		}
		text := fmt.Sprintf("Hardware OpenGL 2.1 was not found (got %d.%d, renderer \"%s\").\n"+
			"The window cannot be rendered without it. This is typical for RDP sessions\n"+
			"and servers/VMs without a GPU.\n\nInstall the Mesa3D software renderer (llvmpipe)?\n%s"+
			"DLLs are placed next to singbox-launcher.exe.", in.Probe.Major, in.Probe.Minor, in.Probe.Renderer, download)
		if messageBox(dlgTitle, text, mbYesNo|mbIconWarning|mbTopmost|mbSetForeground) != idYes {
			debuglog.WarnLog("gl: user declined Mesa3D install — window will likely not render")
			MarkGLStarting(l.Data, mode)
			return
		}
		if installAndVerifyMesa(l, interactive) {
			// Mesa только что легла рядом с exe, но текущий процесс уже держит
			// системный opengl32.dll с первой своей инструкции — применить её
			// можно только новым процессом.
			restartToApply(l.Data, GLModeMesa,
				fmt.Sprintf("Mesa3D installed and verified (%s).", mesaActualDriver()))
			mode = GLModeMesa
		}
		MarkGLStarting(l.Data, mode)
		return
	}
}

// restartToApply — общий хвост любого переключения рендерера в гейте
// (SPEC 125 §6.2 R2): записать новое состояние, показать D6 и перезапустить
// процесс. При успехе не возвращается (os.Exit(0)).
//
// Продолжать текущий старт после переключения нельзя: OPENGL32.dll отображён
// загрузчиком при создании процесса (см. шапку файла), и прогон RC 13.09.2026
// показал ровно это — после D1 → Yes старт шёл дальше на уже отображённой Mesa
// и падал в glTexImage2D, так и не дойдя до цикла событий.
//
// Если запустить новый процесс не удалось — честно просим перезапустить руками
// и продолжаем старт как раньше: хуже, но не тупик.
func restartToApply(d paths.DataDir, newMode, what string) {
	// phase=restart, а не starting: процесс выйдет сам, по нашему решению, и
	// новый старт не должен принять это за смерть на инициализации GL.
	UpdateGLState(d, func(s *GLState) {
		s.Phase = GLPhaseRestart
		s.Mode = newMode
	})
	debuglog.WarnLog("gl: restarting to apply %s", newMode)

	// Диалог — до запуска потомка: иначе новый процесс успеет открыть своё
	// окно поверх модального сообщения старого, и пользователь увидит два
	// лаунчера сразу.
	head := what + "\n" +
		"OpenGL is loaded when the process starts, so the change needs a restart.\n\n"
	messageBox(dlgTitle, head+"The launcher will restart now.", mbOK|mbIconInformation|mbTopmost|mbSetForeground)

	if err := RestartSelf(); err != nil {
		debuglog.ErrorLog("gl: self-restart failed: %v", err)
		messageBox(dlgTitle, head+"Please restart the launcher manually.", mbOK|mbIconInformation|mbTopmost|mbSetForeground)
		return
	}
	os.Exit(0)
}

// pinMesaDriver выставляет GALLIUM_DRIVER=llvmpipe, если переменная не задана.
// Читается инициализацией opengl32.dll, поэтому вызывать строго до загрузки
// DLL и до запуска подпроцессов пробы (они наследуют окружение). Явно
// заданное пользователем значение не трогаем.
func pinMesaDriver() {
	if os.Getenv("GALLIUM_DRIVER") != "" {
		return
	}
	if err := os.Setenv("GALLIUM_DRIVER", mesaPinnedDriver); err != nil {
		debuglog.WarnLog("gl: cannot pin GALLIUM_DRIVER: %v", err)
	}
}

// mesaActualDriver — какой драйвер Gallium реально получит Mesa на этом старте.
// Только GALLIUM_DRIVER из окружения: pinMesaDriver вызывается выше по коду,
// поэтому к моменту WARN переменная всегда задана.
//
// Прежняя версия подставляла пин «по умолчанию», и лог врал: на прогоне RC он
// написал driver=llvmpipe, пока Mesa на самом деле работала через d3d12 и
// падала в glTexImage2D (SPEC 125 §6.1 F3).
func mesaActualDriver() string {
	if env := os.Getenv("GALLIUM_DRIVER"); env != "" {
		return env
	}
	return "unset"
}

// mesaVerifiedRenderer — renderer, который реально показала verify-проба
// (-gl-probe-local). Пока Mesa не проверялась, честнее сказать об этом прямо,
// чем подставлять ожидаемое значение.
func mesaVerifiedRenderer(state GLState) string {
	if state.Renderer != "" {
		return state.Renderer
	}
	return "not verified"
}

// installAndVerifyMesa ставит Mesa3D (из mesa3d/ или скачиванием), проверяет
// её отдельной пробой и только при успехе подгружает в процесс.
// Возвращает true, если после вызова лаунчер рисует через Mesa.
//
// Проверка обязательна: в ассете лежат все драйверы Gallium, и на машине с
// видеокартой Mesa по умолчанию уходит в d3d12 — тот самый драйвер GPU,
// который только что не ответил (репорт 09.09.2026, dxil.dll рядом с exe).
func installAndVerifyMesa(l paths.Layout, interactive bool) bool {
	var installed []string
	var err error
	if HasMesaBundle(l.App) {
		installed, err = copyMesaFromBundle(l.App)
		if err == nil {
			debuglog.InfoLog("gl: copied %d bundled Mesa3D DLLs from %s next to exe: %s",
				len(installed), constants.MesaBundleDirName, strings.Join(installed, ", "))
		}
	} else {
		installed, err = downloadMesa(l.App)
	}
	if err != nil {
		debuglog.ErrorLog("gl: Mesa3D install failed: %v", err)
		removeMesaFiles(l.App, installed)
		if interactive {
			messageBox(dlgTitle, fmt.Sprintf(
				"Mesa3D could not be installed (%v).\nThe launcher will start with the system OpenGL.\n\nManual guide: %s",
				err, rdpOpenGLDocURL), mbOK|mbIconError|mbTopmost|mbSetForeground)
		}
		return false
	}

	// Пин драйвера ДО запуска подпроцесса пробы: переменную читает
	// инициализация DLL, а подпроцесс наследует окружение родителя — поставь
	// её позже, и проверялся бы не тот драйвер, который потом загрузится.
	// Явно заданное пользователем значение не трогаем.
	pinMesaDriver()

	probe := probeGLViaSubprocess(true)
	if !probe.ok() || !strings.Contains(strings.ToLower(probe.Renderer), mesaPinnedDriver) {
		reason := probe.describe()
		if probe.ok() {
			reason = fmt.Sprintf("renderer %q is not %s", probe.Renderer, mesaPinnedDriver)
		}
		debuglog.ErrorLog("gl: installed Mesa3D did not pass the local probe (%s) — rolling back", reason)
		if disErr := DisableMesa(l.App); disErr != nil {
			// Откатываем хотя бы то, что сами положили.
			removeMesaFiles(l.App, installed)
		}
		if interactive {
			messageBox(dlgTitle, fmt.Sprintf(
				"Mesa3D was installed but did not start (%s).\n"+
					"It has been disabled again. The launcher will start with the system OpenGL.\n\n"+
					"Details: logs/%s, logs/%s\nManual guide: %s",
				reason, constants.NativeStderrLogFileName, constants.MainLogFileName, rdpOpenGLDocURL),
				mbOK|mbIconError|mbTopmost|mbSetForeground)
		}
		return false
	}

	debuglog.WarnLog("gl: Mesa3D installed and verified (renderer=%q)", probe.Renderer)
	// Renderer и driver — в состояние: WARN про Mesa на следующих стартах
	// печатает renderer отсюда, повторной пробы не делая.
	UpdateGLState(l.Data, func(st *GLState) {
		st.Renderer = probe.Renderer
		st.Driver = mesaActualDriver()
	})
	// Preload'а здесь больше нет: подгрузить Mesa в живой процесс невозможно —
	// системный opengl32.dll отображён по таблице импорта exe ещё до main
	// (SPEC 125 §6.1 F1). Применяет установку только перезапуск.
	return true
}

// startBackgroundHardwareProbe — фоновая проба железа в режиме mesa
// (SPEC 125 §2.5). Не блокирует запуск: результат нужен не гейту, а UI,
// который после появления окна предложит вернуться на аппаратный OpenGL.
func startBackgroundHardwareProbe(a paths.AppDir, offeredRenderer string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				debuglog.ErrorLog("gl: background hardware probe panicked: %v", r)
			}
		}()
		// Та же уловка с переименованием, что и в гейте: без неё проба увидит
		// Mesa, через которую прямо сейчас рисует окно, и предложит «вернуться»
		// на неё же.
		probe := probeHardware(a)
		if !probe.ok() {
			debuglog.InfoLog("gl: background hardware probe: still no usable OpenGL (%s)", probe.describe())
			return
		}
		if probe.Renderer == offeredRenderer {
			// Про этот renderer пользователь уже отвечал «Later».
			return
		}
		notifyHardwareGLAvailable(probe.Renderer)
	}()
}

// probeHardware — проба НАСТОЯЩЕГО аппаратного OpenGL, даже когда рядом с exe
// установлена Mesa3D (SPEC 125 §6.2 R3).
//
// Почему просто probeGLViaSubprocess(false) недостаточно: OPENGL32.dll стоит в
// таблице импорта exe, поэтому дочерний процесс -gl-probe грузит opengl32.dll
// из каталога приложения ещё до своего main — и «аппаратная» проба измеряет
// Mesa. В логе RC это выглядело как renderer="D3D12 (NVIDIA GeForce GT 440)"
// вместо настоящего "GeForce GT 440/PCIe/SSE2".
//
// Лечение — на время дочернего процесса увести opengl32.dll из каталога
// приложения переименованием. Windows разрешает переименовать отображённую
// DLL (удалить — нет), так что живая Mesa текущего процесса это переживает.
// libgallium_wgl.dll и dxil.dll не трогаем: Mesa могла бы подгрузить их лениво
// именно в это окно.
func probeHardware(a paths.AppDir) probeResult {
	if IsMesaInstalled(a) {
		src := filepath.Join(string(a), "opengl32.dll")
		dst := src + ".probe"
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			debuglog.WarnLog("gl: cannot move Mesa's opengl32.dll aside for the hardware probe (%v) — the probe may see Mesa", err)
		} else {
			// Вернуть имя обязаны при любом исходе, включая панику: без
			// opengl32.dll рядом с exe Mesa просто перестанет существовать.
			// Между Rename и восстановлением ничего, что могло бы вызвать
			// LoadLibrary, не делаем.
			defer func() {
				if err := os.Rename(dst, src); err != nil {
					debuglog.ErrorLog("gl: cannot restore %s after the hardware probe: %v", src, err)
				}
			}()
		}
	}
	res := probeGLViaSubprocess(false)
	// Страховка: переименование могло не удаться (файл занят чем-то ещё).
	// Тогда мы измерили Mesa, и выдавать её за железо нельзя.
	if strings.Contains(res.Vendor, "Mesa") {
		res.SawMesa = true
	}
	return res
}

// probeGLViaSubprocess перезапускает лаунчер с -gl-probe (local=false) или
// -gl-probe-local (local=true) и разбирает его stdout. Подпроцесс изолирует
// загрузку opengl32.dll (и возможные падения кривых драйверов) от основного
// процесса.
//
// Таймаут возвращается отдельным полем, а не error: «не ответила за 10 с» и
// «драйвер отдал 1.1» — разные диагнозы с разным лечением.
func probeGLViaSubprocess(local bool) probeResult {
	exe, err := os.Executable()
	if err != nil {
		return probeResult{Err: fmt.Errorf("os.Executable: %w", err)}
	}
	flagName := "-gl-probe"
	if local {
		flagName = "-gl-probe-local"
	}
	cmd := exec.Command(exe, flagName)
	PrepareCommand(cmd)
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		out, runErr = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(glProbeTimeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return probeResult{Timeout: true}
	}

	res := probeResult{}
	version := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "version="):
			version = strings.TrimPrefix(line, "version=")
		case strings.HasPrefix(line, "renderer="):
			res.Renderer = strings.TrimPrefix(line, "renderer=")
		case strings.HasPrefix(line, "vendor="):
			res.Vendor = strings.TrimPrefix(line, "vendor=")
		case strings.HasPrefix(line, "error="):
			res.Err = fmt.Errorf("probe: %s", strings.TrimPrefix(line, "error="))
			return res
		}
	}
	if runErr != nil && version == "" {
		res.Err = fmt.Errorf("gl-probe subprocess: %w (output: %q)", runErr, strings.TrimSpace(string(out)))
		return res
	}
	if n, _ := fmt.Sscanf(version, "%d.%d", &res.Major, &res.Minor); n < 2 {
		res.Err = fmt.Errorf("unparseable GL version %q", version)
		return res
	}
	return res
}

// downloadMesa скачивает zip с DLL Mesa3D с релиза лаунчера и распаковывает
// их рядом с exe. Preload сюда не входит: установленную Mesa сначала надо
// проверить пробой -gl-probe-local (SPEC 125 §2.4).
func downloadMesa(a paths.AppDir) ([]string, error) {
	tmp, err := os.CreateTemp("", "mesa3d-*.zip")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup

	directURL := fmt.Sprintf("https://github.com/Leadaxe/singbox-launcher/releases/download/%s/%s", mesaReleaseTag, mesaAssetName)
	// Как и в core_downloader: зеркала на случай, если GitHub недоступен без
	// VPN (а VPN ещё не запущен — курица и яйцо). ghproxy.com здесь больше нет:
	// он отдаёт HTTP 200 со своей HTML-страницей вместо файла, то есть
	// «успешно» скачивается мусор вместо архива Mesa.
	urls := []string{directURL}
	for _, prefix := range constants.GitHubDownloadMirrors {
		urls = append(urls, prefix+directURL)
	}
	var lastErr error
	for _, u := range urls {
		if lastErr = downloadToFile(u, tmpPath); lastErr == nil {
			break
		}
		debuglog.WarnLog("gl: mesa download from %s failed: %v", u, lastErr)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("download: %w", lastErr)
	}

	names, err := extractDLLs(tmpPath, string(a))
	if err != nil {
		return names, fmt.Errorf("extract: %w", err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("archive %s contains no DLLs", mesaAssetName)
	}
	debuglog.InfoLog("gl: extracted %d Mesa3D DLLs next to exe: %s", len(names), strings.Join(names, ", "))
	return names, nil
}

// downloadToFile качает url в destPath с жёстким потолком размера.
func downloadToFile(url, destPath string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > mesaDownloadCap {
		return fmt.Errorf("advertised size %d exceeds cap %d", resp.ContentLength, int64(mesaDownloadCap))
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	written, err := io.Copy(f, io.LimitReader(resp.Body, mesaDownloadCap+1))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if written > mesaDownloadCap {
		return fmt.Errorf("download exceeds cap %d", int64(mesaDownloadCap))
	}
	return nil
}

// extractDLLs распаковывает все *.dll из zip плоско в destDir (через .tmp и
// rename, чтобы не оставить обрезанный opengl32.dll при сбое на середине).
func extractDLLs(zipPath, destDir string) ([]string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close() //nolint:errcheck // read-only archive

	var names []string
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if f.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(base), ".dll") {
			continue
		}
		if err := func() error {
			src, err := f.Open()
			if err != nil {
				return err
			}
			defer src.Close() //nolint:errcheck // read-only entry
			tmpPath := filepath.Join(destDir, base+".tmp")
			dst, err := os.Create(tmpPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, src); err != nil {
				_ = dst.Close()
				_ = os.Remove(tmpPath)
				return err
			}
			if err := dst.Close(); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}
			finalPath := filepath.Join(destDir, base)
			_ = os.Remove(finalPath)
			return os.Rename(tmpPath, finalPath)
		}(); err != nil {
			return names, fmt.Errorf("%s: %w", base, err)
		}
		names = append(names, base)
	}
	return names, nil
}

// messageBox показывает нативный Win32 MessageBox (работает без OpenGL,
// в отличие от любых Fyne-диалогов). Возвращает код нажатой кнопки.
func messageBox(title, text string, flags uint32) int {
	user32 := windows.NewLazySystemDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")
	textPtr, _ := windows.UTF16PtrFromString(text)
	titlePtr, _ := windows.UTF16PtrFromString(title)
	ret, _, _ := messageBoxW.Call(0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(flags))
	return int(ret)
}
