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
//     версию/renderer. Подпроцесс нужен, чтобы родитель не грузил системный
//     opengl32.dll — иначе его не подменить в текущем процессе.
//  2. Если версия < 2.1 — нативный MessageBox (Fyne-окно показать нечем)
//     предлагает скачать Mesa3D с релиза лаунчера и распаковать DLL рядом
//     с exe.
//  3. Свежераспакованный opengl32.dll грузится через LoadLibraryEx по полному
//     пути: последующий LoadLibrary("opengl32.dll") из GLFW вернёт уже
//     загруженный модуль по совпадению базового имени, так что окно
//     отрисуется без перезапуска. На следующих стартах Mesa подхватывается
//     самим загрузчиком Windows (каталог приложения ищется раньше system32),
//     и пробник не нужен.

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

	"golang.org/x/sys/windows"
)

const (
	// Собственный релиз-зеркало Mesa3D (llvmpipe) в репозитории лаунчера:
	// mesa-dist-win отдаёт только .7z, который stdlib не распакует, поэтому
	// нужные DLL переупакованы в zip. Релиз помечен prerelease, чтобы не
	// светиться как «latest» для проверки обновлений лаунчера.
	mesaReleaseTag = "mesa3d-26.2.0"
	mesaAssetName  = "mesa3d-26.2.0-win64.zip"

	// Минимум для Fyne/GLFW (см. docs/WIN7_OPENGL.md).
	glMinMajor = 2
	glMinMinor = 1

	glProbeTimeout    = 10 * time.Second
	mesaDownloadCap   = 100 * 1024 * 1024 // байт; реальный ассет ~24 МБ
	rdpOpenGLDocURL   = "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/RDP_OPENGL.ru.md"
	win7OpenGLDocURL  = "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/WIN7_OPENGL.ru.md"
	mesaConsentText   = "Не найден аппаратный OpenGL 2.1 — окно программы не сможет отобразиться.\nТак обычно бывает в RDP-сессии или на сервере/ВМ без GPU.\n\nСкачать и установить программный рендерер Mesa3D (~24 МБ)?\nDLL будут распакованы рядом с singbox-launcher.exe. Нужен доступ в интернет.\n\nHardware OpenGL 2.1 not found — the application window cannot be rendered.\nThis is typical for RDP sessions and servers/VMs without a GPU.\n\nDownload and install the Mesa3D software renderer (~24 MB)?\nDLLs will be extracted next to singbox-launcher.exe. Internet access required."
	mesaRestartText   = "Mesa3D установлена, но системный OpenGL уже загружен в процесс.\nПерезапустите Singbox Launcher.\n\nMesa3D is installed, but the system OpenGL is already loaded.\nPlease restart Singbox Launcher."
	mesaInstalledNote = "gl: Mesa3D installed and preloaded — window will render via llvmpipe"
)

// Win32-константы (user32/gdi32/wgl).
const (
	wsPopup           = 0x80000000
	pfdDrawToWindow   = 0x00000004
	pfdSupportOpenGL  = 0x00000020
	pfdDoubleBuffer   = 0x00000001
	pfdTypeRGBA       = 0
	glVersionEnum     = 0x1F02
	glRendererEnum    = 0x1F01
	glVendorEnum      = 0x1F00
	mbYesNo           = 0x00000004
	mbOK              = 0x00000000
	mbIconWarning     = 0x00000030
	mbIconError       = 0x00000010
	mbIconInformation = 0x00000040
	mbTopmost         = 0x00040000
	mbSetForeground   = 0x00010000
	idYes             = 6
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

// RunGLProbeChild — тело подпроцесса `singbox-launcher.exe -gl-probe`.
// Печатает в stdout строки version=/renderer=/vendor= и завершает процесс.
// Никогда не возвращается.
func RunGLProbeChild() {
	version, renderer, vendor, err := probeDesktopOpenGL()
	if err != nil {
		fmt.Printf("error=%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("version=%s\nrenderer=%s\nvendor=%s\n", version, renderer, vendor)
	os.Exit(0)
}

// probeDesktopOpenGL создаёт скрытое окно и legacy-WGL-контекст, читает
// GL_VERSION/GL_RENDERER/GL_VENDOR. Выполняется только в подпроцессе -gl-probe.
func probeDesktopOpenGL() (version, renderer, vendor string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	user32 := windows.NewLazySystemDLL("user32.dll")
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	opengl32 := windows.NewLazySystemDLL("opengl32.dll")
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
	wglCreateContext := opengl32.NewProc("wglCreateContext")
	wglMakeCurrent := opengl32.NewProc("wglMakeCurrent")
	wglDeleteContext := opengl32.NewProc("wglDeleteContext")
	glGetString := opengl32.NewProc("glGetString")

	if err := opengl32.Load(); err != nil {
		return "", "", "", fmt.Errorf("opengl32.dll not loadable: %w", err)
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

// EnsureDesktopOpenGL — гейт перед инициализацией GL-части Fyne: проверяет
// наличие OpenGL >= 2.1 и при его отсутствии предлагает установить Mesa3D.
// Никогда не роняет запуск: в худшем случае поведение прежнее (окно не
// откроется), но с внятным логом и подсказкой.
func EnsureDesktopOpenGL(execDir string) {
	if os.Getenv("SINGBOX_LAUNCHER_NO_MESA") == "1" {
		debuglog.InfoLog("gl: probe skipped (SINGBOX_LAUNCHER_NO_MESA=1)")
		return
	}

	// Mesa (или иной opengl32.dll) уже лежит рядом с exe — каталог приложения
	// в порядке поиска DLL стоит раньше system32, загрузчик подхватит его сам.
	if _, err := os.Stat(filepath.Join(execDir, "opengl32.dll")); err == nil {
		debuglog.InfoLog("gl: local opengl32.dll found next to exe — Windows loader will use it")
		return
	}

	if os.Getenv("SINGBOX_LAUNCHER_FORCE_MESA") != "1" {
		major, minor, renderer, err := probeGLViaSubprocess()
		if err == nil && (major > glMinMajor || (major == glMinMajor && minor >= glMinMinor)) {
			debuglog.InfoLog("gl: hardware OpenGL %d.%d available (renderer=%q)", major, minor, renderer)
			return
		}
		debuglog.WarnLog("gl: insufficient OpenGL (got %d.%d, renderer=%q, err=%v) — Fyne needs %d.%d; offering Mesa3D",
			major, minor, renderer, err, glMinMajor, glMinMinor)
	} else {
		debuglog.WarnLog("gl: SINGBOX_LAUNCHER_FORCE_MESA=1 — skipping probe, offering Mesa3D")
	}

	// Win7-сборка (386): готового x86-ассета нет, современная Mesa требует
	// Windows 10+. Отправляем к ручной инструкции.
	if runtime.GOARCH == "386" {
		messageBox("Singbox Launcher",
			"Не найден аппаратный OpenGL 2.1 — окно программы не сможет отобразиться.\nРучная установка Mesa3D: "+win7OpenGLDocURL+
				"\n\nHardware OpenGL 2.1 not found. Manual Mesa3D guide:\n"+win7OpenGLDocURL,
			mbOK|mbIconWarning|mbTopmost|mbSetForeground)
		return
	}

	// Архив win64-full кладёт DLL Mesa3D в папку mesa3d/ рядом с exe: без сети
	// и без вопроса — качать нечего, а окно иначе не отрисуется вовсе.
	if found, err := installMesaFromBundle(execDir); found {
		if err != nil {
			debuglog.ErrorLog("gl: bundled Mesa3D install failed: %v", err)
			messageBox("Singbox Launcher",
				fmt.Sprintf("Не удалось установить Mesa3D из папки mesa3d: %v\nРучная установка: %s\n\nBundled Mesa3D install failed. Manual guide:\n%s",
					err, rdpOpenGLDocURL, rdpOpenGLDocURL),
				mbOK|mbIconError|mbTopmost|mbSetForeground)
			return
		}
		debuglog.InfoLog(mesaInstalledNote)
		return
	}

	if messageBox("Singbox Launcher", mesaConsentText, mbYesNo|mbIconWarning|mbTopmost|mbSetForeground) != idYes {
		debuglog.WarnLog("gl: user declined Mesa3D install — window will likely not render")
		return
	}

	if err := installMesa(execDir); err != nil {
		debuglog.ErrorLog("gl: Mesa3D install failed: %v", err)
		messageBox("Singbox Launcher",
			fmt.Sprintf("Не удалось установить Mesa3D: %v\nРучная установка: %s\n\nMesa3D install failed. Manual guide:\n%s",
				err, rdpOpenGLDocURL, rdpOpenGLDocURL),
			mbOK|mbIconError|mbTopmost|mbSetForeground)
		return
	}
	debuglog.InfoLog(mesaInstalledNote)
}

// probeGLViaSubprocess перезапускает лаунчер с -gl-probe и разбирает его stdout.
// Подпроцесс изолирует загрузку системного opengl32.dll (и возможные падения
// кривых драйверов) от основного процесса.
func probeGLViaSubprocess() (major, minor int, renderer string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, 0, "", fmt.Errorf("os.Executable: %w", err)
	}
	cmd := exec.Command(exe, "-gl-probe")
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
		return 0, 0, "", fmt.Errorf("gl-probe timed out after %v", glProbeTimeout)
	}

	version := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "version="):
			version = strings.TrimPrefix(line, "version=")
		case strings.HasPrefix(line, "renderer="):
			renderer = strings.TrimPrefix(line, "renderer=")
		case strings.HasPrefix(line, "error="):
			return 0, 0, "", fmt.Errorf("probe: %s", strings.TrimPrefix(line, "error="))
		}
	}
	if runErr != nil && version == "" {
		return 0, 0, "", fmt.Errorf("gl-probe subprocess: %w (output: %q)", runErr, strings.TrimSpace(string(out)))
	}
	if n, _ := fmt.Sscanf(version, "%d.%d", &major, &minor); n < 2 {
		return 0, 0, renderer, fmt.Errorf("unparseable GL version %q", version)
	}
	return major, minor, renderer, nil
}

// installMesa скачивает zip с DLL Mesa3D с релиза лаунчера, распаковывает их
// рядом с exe и подгружает opengl32.dll в текущий процесс, чтобы окно
// отрисовалось без перезапуска.
func installMesa(execDir string) error {
	tmp, err := os.CreateTemp("", "mesa3d-*.zip")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
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
		return fmt.Errorf("download: %w", lastErr)
	}

	names, err := extractDLLs(tmpPath, execDir)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if len(names) == 0 {
		return fmt.Errorf("archive %s contains no DLLs", mesaAssetName)
	}
	debuglog.InfoLog("gl: extracted %d Mesa3D DLLs next to exe: %s", len(names), strings.Join(names, ", "))

	// Если системный opengl32.dll уже успел загрузиться в процесс (не должен —
	// гейт стоит до инициализации GL-части Fyne), подменить его в этой сессии
	// нельзя: просим перезапуститься, дальше DLL подхватит загрузчик Windows.
	return preloadMesa(execDir)
}

// installMesaFromBundle ставит Mesa3D из папки mesa3d/ рядом с exe (архив
// win64-full). found=false — папки или opengl32.dll в ней нет, и вызывающий
// идёт обычным путём с согласием и скачиванием. DLL копируются рядом с exe
// через .tmp и rename, как при распаковке архива.
func installMesaFromBundle(execDir string) (found bool, err error) {
	srcDir := filepath.Join(execDir, constants.MesaBundleDirName)
	if _, statErr := os.Stat(filepath.Join(srcDir, "opengl32.dll")); statErr != nil {
		return false, nil
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return true, fmt.Errorf("read %s: %w", srcDir, err)
	}
	var names []string
	for _, ent := range entries {
		base := ent.Name()
		if ent.IsDir() || !strings.EqualFold(filepath.Ext(base), ".dll") {
			continue
		}
		tmpPath := filepath.Join(execDir, base+".tmp")
		if copyErr := copyFile(filepath.Join(srcDir, base), tmpPath); copyErr != nil {
			_ = os.Remove(tmpPath)
			return true, fmt.Errorf("%s: %w", base, copyErr)
		}
		finalPath := filepath.Join(execDir, base)
		_ = os.Remove(finalPath)
		if renameErr := os.Rename(tmpPath, finalPath); renameErr != nil {
			_ = os.Remove(tmpPath)
			return true, fmt.Errorf("%s: %w", base, renameErr)
		}
		names = append(names, base)
	}
	debuglog.InfoLog("gl: copied %d bundled Mesa3D DLLs from %s next to exe: %s", len(names), constants.MesaBundleDirName, strings.Join(names, ", "))
	return true, preloadMesa(execDir)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only source
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// preloadMesa грузит распакованный рядом с exe opengl32.dll в текущий процесс
// (общий хвост скачанной и вложенной установки).
func preloadMesa(execDir string) error {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getModuleHandleW := kernel32.NewProc("GetModuleHandleW")
	namePtr, _ := windows.UTF16PtrFromString("opengl32.dll")
	if h, _, _ := getModuleHandleW.Call(uintptr(unsafe.Pointer(namePtr))); h != 0 {
		debuglog.WarnLog("gl: system opengl32.dll already loaded in this process — restart required to pick up Mesa")
		messageBox("Singbox Launcher", mesaRestartText, mbOK|mbIconInformation|mbTopmost|mbSetForeground)
		return nil
	}

	// LOAD_WITH_ALTERED_SEARCH_PATH: зависимости Mesa (libgallium_wgl.dll и др.)
	// резолвятся из каталога самого opengl32.dll. Последующий
	// LoadLibrary("opengl32.dll") из GLFW вернёт этот модуль по базовому имени.
	if _, err := windows.LoadLibraryEx(filepath.Join(execDir, "opengl32.dll"), 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH); err != nil {
		return fmt.Errorf("preload mesa opengl32.dll: %w", err)
	}
	return nil
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
