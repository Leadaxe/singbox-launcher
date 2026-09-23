package platform

// Состояние GL-гейта (SPEC 125) и операции над файлами Mesa3D рядом с exe.
//
// Файл без build-тегов намеренно: Win32 здесь не нужен ни на байт, а решение
// гейта (decideGate) — чистая функция, которую можно прогнать тестами на любой
// платформе. Win32-часть (проба, MessageBoxW, preload DLL) живёт в
// glprobe_windows.go.
//
// Предыстория: полевой репорт 09.09.2026 — обычный домашний ПК с рабочей
// видеокартой. Проба OpenGL не ответила за 10 секунд, гейт истолковал таймаут
// как «аппаратного OpenGL нет» и без вопроса скопировал Mesa3D рядом с exe.
// Дальше лаунчер умирал в потоке, созданном DLL: ни crash.txt, ни Fyne error,
// ни строки в логе. Отсюда три инварианта этого файла: (1) отличать таймаут от
// отказа, (2) знать, дошёл ли прошлый старт до кадра, (3) уметь откатиться.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

// Пороги и таймауты пробы. Живут здесь, а не в glprobe_windows.go, потому что
// их читает platform-независимая часть (probeResult.ok / describe).
const (
	// Минимум для Fyne/GLFW (см. docs/WIN7_OPENGL.md).
	glMinMajor = 2
	glMinMinor = 1

	glProbeTimeout = 10 * time.Second
)

// Фазы и режимы записи bin/gl-state.json.
const (
	// GLPhaseStarting — гейт отработал и передал управление Fyne, но кадра
	// ещё не было. Осталось в файле после следующего старта = процесс умер
	// на инициализации GL.
	GLPhaseStarting = "starting"
	// GLPhaseRendered — окно дожило до первого кадра (см. glRenderedGrace в
	// main.go). Такому старту гейт доверяет и пробу не запускает.
	GLPhaseRendered = "rendered"
	// GLPhaseRestart — процесс сам себя перезапускает, чтобы применить смену
	// рендерера (SPEC 125 §6.2 R2). Для гейта такая запись «чистая», как и
	// rendered: старт не умер, он завершился по нашему решению, и переспрашивать
	// про OpenGL в новом процессе не нужно. MarkGLRendered из нового процесса
	// штатно перепишет её в rendered.
	GLPhaseRestart = "restart"

	GLModeHardware = "hardware"
	GLModeMesa     = "mesa"
)

// GLState — содержимое bin/gl-state.json.
type GLState struct {
	Phase           string    `json:"phase"`
	Mode            string    `json:"mode"`
	Driver          string    `json:"driver,omitempty"`
	Renderer        string    `json:"renderer,omitempty"`
	LauncherVersion string    `json:"launcher_version"`
	UpdatedAt       time.Time `json:"updated_at"`
	// OfferedHWRenderer — renderer железа, про который пользователю уже
	// предлагали вернуться с Mesa. Пока строка не изменилась, диалог возврата
	// больше не показывается (SPEC 125 §2.5).
	OfferedHWRenderer string `json:"offered_hw_renderer,omitempty"`
}

// glStateMu сериализует read-modify-write над gl-state.json. Писателей
// немного, но они реально пересекаются: MarkGLRendered взводится таймером на
// 3 с, а диалог возврата на железо пишет OfferedHWRenderer тогда, когда
// пользователь нажал кнопку. Без замка одна запись затирала бы поле другой.
var glStateMu sync.Mutex

// GLStatePath — путь к bin/gl-state.json.
func GLStatePath(execDir string) string {
	return filepath.Join(GetBinDir(execDir), constants.GLStateFileName)
}

// LoadGLState читает состояние гейта. ok=false — файла нет (первый запуск)
// или JSON битый; в обоих случаях гейт обязан сделать пробу, а не гадать.
func LoadGLState(execDir string) (GLState, bool) {
	var s GLState
	data, err := os.ReadFile(GLStatePath(execDir))
	if err != nil {
		return GLState{}, false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		debuglog.WarnLog("gl: %s is unreadable (%v) — treating as first start", constants.GLStateFileName, err)
		return GLState{}, false
	}
	return s, true
}

// SaveGLState пишет состояние атомарно (tmp + rename, как locale.SaveSettings).
// Обрыв записи на середине оставил бы битый JSON, а его гейт трактует как
// «первый запуск» — то есть лишняя проба при каждом старте.
func SaveGLState(execDir string, s GLState) error {
	binDir := GetBinDir(execDir)
	if err := os.MkdirAll(binDir, DefaultDirMode); err != nil {
		return fmt.Errorf("gl: create bin dir: %w", err)
	}
	path := filepath.Join(binDir, constants.GLStateFileName)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("gl: marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, DefaultFileMode); err != nil {
		return fmt.Errorf("gl: write temp state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("gl: rename state: %w", err)
	}
	return nil
}

// MarkGLStarting штампует «идём на инициализацию GL в режиме mode».
//
// Renderer, Driver и OfferedHWRenderer берутся из прежней записи: потеря
// OfferedHWRenderer означала бы, что диалог возврата на железо всплывает при
// каждом старте, хотя пользователь уже ответил «Later».
func MarkGLStarting(execDir, mode string) {
	UpdateGLState(execDir, func(s *GLState) {
		s.Phase = GLPhaseStarting
		s.Mode = mode
	})
}

// UpdateGLState читает запись, даёт её изменить и пишет обратно под замком.
// Единственный способ менять gl-state.json: LauncherVersion и UpdatedAt
// проставляются здесь, а не в каждом вызывающем.
func UpdateGLState(execDir string, mutate func(*GLState)) {
	glStateMu.Lock()
	defer glStateMu.Unlock()

	s, _ := LoadGLState(execDir)
	mutate(&s)
	s.LauncherVersion = constants.AppVersion
	s.UpdatedAt = time.Now().UTC()
	if err := SaveGLState(execDir, s); err != nil {
		debuglog.WarnLog("gl: cannot persist gate state: %v", err)
	}
}

// MarkGLRendered фиксирует, что окно дожило до кадра. Остальные поля —
// из прежней записи.
func MarkGLRendered(execDir string) {
	UpdateGLState(execDir, func(s *GLState) {
		if s.Mode == "" {
			// Гейт до нас не дошёл (не-Windows или ошибка записи). Ставим
			// честный минимум, чтобы следующий старт не считал это смертью.
			s.Mode = GLModeHardware
		}
		s.Phase = GLPhaseRendered
	})
}

// restartPending — кнопка Диагностики попросила перезапуск после переключения
// рендерера. Сам RestartSelf вызывается не там, а в самом конце main(), после
// Application.Run() и GracefulExit: новый процесс не должен стартовать, пока
// старый держит запущенный sing-box (иначе он встретит пользователя диалогом
// «уже запущено»), а из колбэка Fyne-диалога дождаться этого нельзя.
var restartPending atomic.Bool

// RequestRestartAfterExit помечает, что после штатного завершения лаунчер
// должен подняться снова.
func RequestRestartAfterExit() { restartPending.Store(true) }

// RestartRequested — был ли запрошен перезапуск (читается в конце main()).
func RestartRequested() bool { return restartPending.Load() }

// errForeignOpenGL — рядом с exe лежит opengl32.dll, но libgallium_wgl.dll
// нет: это не наша Mesa, а чужая подмена (или ручная установка пользователя).
// Трогать её мы не имеем права.
var errForeignOpenGL = errors.New("local opengl32.dll is not our Mesa3D (libgallium_wgl.dll is missing)")

// mesaDLLs — файлы, которыми оперируют DisableMesa/EnableMesa. Порядок важен
// только для читаемости лога.
var mesaDLLs = []string{"opengl32.dll", "libgallium_wgl.dll", "dxil.dll"}

// IsMesaInstalled — рядом с exe лежит именно наша Mesa3D: opengl32.dll вместе
// с libgallium_wgl.dll. Одиночный opengl32.dll — чужой, см. errForeignOpenGL.
func IsMesaInstalled(execDir string) bool {
	return fileExists(filepath.Join(execDir, "opengl32.dll")) &&
		fileExists(filepath.Join(execDir, "libgallium_wgl.dll"))
}

// IsMesaDisabled — Mesa отключена переименованием (opengl32.dll.off).
func IsMesaDisabled(execDir string) bool {
	return fileExists(filepath.Join(execDir, "opengl32.dll"+constants.MesaDisabledSuffix))
}

// HasMesaBundle — рядом с exe есть папка mesa3d/ с DLL (архив win64-full),
// то есть установку можно сделать без сети.
func HasMesaBundle(execDir string) bool {
	return fileExists(filepath.Join(execDir, constants.MesaBundleDirName, "opengl32.dll"))
}

// HasForeignOpenGL — рядом с exe одиночный opengl32.dll без Mesa-спутника.
func HasForeignOpenGL(execDir string) bool {
	return fileExists(filepath.Join(execDir, "opengl32.dll")) &&
		!fileExists(filepath.Join(execDir, "libgallium_wgl.dll"))
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// DisableMesa переименовывает DLL Mesa3D рядом с exe в <имя>.off.
// Переименование обратимо без сети и без папки mesa3d/ — в отличие от удаления.
func DisableMesa(execDir string) error {
	if HasForeignOpenGL(execDir) {
		return errForeignOpenGL
	}
	var moved []string
	for _, name := range mesaDLLs {
		src := filepath.Join(execDir, name)
		if !fileExists(src) {
			continue
		}
		dst := src + constants.MesaDisabledSuffix
		// Прошлый .off с той же машины мешает rename на Windows — сносим.
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("gl: disable %s: %w", name, err)
		}
		moved = append(moved, name)
	}
	if len(moved) == 0 {
		return fmt.Errorf("gl: no Mesa3D DLLs next to exe")
	}
	debuglog.WarnLog("gl: Mesa3D disabled — renamed to %s: %s", constants.MesaDisabledSuffix, strings.Join(moved, ", "))
	return nil
}

// EnableMesa возвращает отключённые DLL обратно. Если .off нет, но есть папка
// mesa3d/ — копирует оттуда (кнопка «Enable» в Диагностике на чистой машине).
func EnableMesa(execDir string) error {
	if !IsMesaDisabled(execDir) {
		names, err := copyMesaFromBundle(execDir)
		if err != nil {
			return err
		}
		debuglog.WarnLog("gl: Mesa3D enabled from %s: %s", constants.MesaBundleDirName, strings.Join(names, ", "))
		return nil
	}
	var restored []string
	for _, name := range mesaDLLs {
		src := filepath.Join(execDir, name+constants.MesaDisabledSuffix)
		if !fileExists(src) {
			continue
		}
		dst := filepath.Join(execDir, name)
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("gl: enable %s: %w", name, err)
		}
		restored = append(restored, name)
	}
	if len(restored) == 0 {
		return fmt.Errorf("gl: no disabled Mesa3D DLLs next to exe")
	}
	debuglog.WarnLog("gl: Mesa3D enabled — restored: %s", strings.Join(restored, ", "))
	return nil
}

// copyMesaFromBundle копирует все *.dll из mesa3d/ рядом с exe через .tmp и
// rename (обрыв на середине не должен оставить обрезанный opengl32.dll).
// Preload сюда не входит: сначала установленную Mesa надо проверить пробой.
func copyMesaFromBundle(execDir string) ([]string, error) {
	// Чужой одиночный opengl32.dll рядом с exe затирать нельзя: это не наша
	// установка, а чья-то ещё (SPEC 125 §2.1 — такой файл только WARN'ится).
	if HasForeignOpenGL(execDir) {
		return nil, errForeignOpenGL
	}
	srcDir := filepath.Join(execDir, constants.MesaBundleDirName)
	if !fileExists(filepath.Join(srcDir, "opengl32.dll")) {
		return nil, fmt.Errorf("gl: %s has no opengl32.dll", constants.MesaBundleDirName)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("gl: read %s: %w", srcDir, err)
	}
	var names []string
	for _, ent := range entries {
		base := ent.Name()
		if ent.IsDir() || !strings.EqualFold(filepath.Ext(base), ".dll") {
			continue
		}
		tmpPath := filepath.Join(execDir, base+".tmp")
		if copyErr := copyFileGL(filepath.Join(srcDir, base), tmpPath); copyErr != nil {
			_ = os.Remove(tmpPath)
			return names, fmt.Errorf("gl: copy %s: %w", base, copyErr)
		}
		finalPath := filepath.Join(execDir, base)
		_ = os.Remove(finalPath)
		if renameErr := os.Rename(tmpPath, finalPath); renameErr != nil {
			_ = os.Remove(tmpPath)
			return names, fmt.Errorf("gl: install %s: %w", base, renameErr)
		}
		// Отключённая копия того же файла осталась бы висеть мусором и
		// сбивала бы IsMesaDisabled на следующем старте.
		_ = os.Remove(finalPath + constants.MesaDisabledSuffix)
		names = append(names, base)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("gl: %s contains no DLLs", constants.MesaBundleDirName)
	}
	return names, nil
}

// removeMesaFiles сносит перечисленные файлы рядом с exe. Используется для
// отката только что скопированной/скачанной Mesa, когда проба её не приняла.
func removeMesaFiles(execDir string, names []string) { //nolint:unused // вызывается только из glprobe_windows.go (//go:build windows)
	for _, name := range names {
		if err := os.Remove(filepath.Join(execDir, name)); err != nil && !os.IsNotExist(err) {
			debuglog.WarnLog("gl: rollback: cannot remove %s: %v", name, err)
		}
	}
}

func copyFileGL(src, dst string) error {
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

// probeResult — исход пробы OpenGL. Таймаут вынесен в отдельное поле
// намеренно: слияние «не ответила за 10 с» и «драйвер отдал 1.1» в один
// error было причиной репорта 09.09.2026 (Mesa поставилась на живом GPU).
type probeResult struct {
	Major, Minor int
	Renderer     string
	Vendor       string
	Err          error // nil — проба ответила
	Timeout      bool  // true — не уложилась в glProbeTimeout
	// SawMesa — «аппаратная» проба на самом деле измерила Mesa3D. Страховка на
	// случай, если временное переименование opengl32.dll не сработало
	// (SPEC 125 §6.2 R3): OPENGL32.dll стоит в таблице импорта exe, поэтому
	// дочерний процесс с Mesa рядом видит Mesa, а не железо. Такой результат
	// железом считать нельзя — иначе гейт предложит «вернуться» на то же самое.
	SawMesa bool
}

// ok — проба ответила, версия достаточна для Fyne/GLFW и это действительно
// железо, а не Mesa под видом железа.
func (r probeResult) ok() bool {
	if r.Err != nil || r.Timeout || r.SawMesa {
		return false
	}
	return r.Major > glMinMajor || (r.Major == glMinMajor && r.Minor >= glMinMinor)
}

// describe — короткая причина отказа для текста диалога.
func (r probeResult) describe() string {
	switch {
	case r.Timeout:
		return fmt.Sprintf("timed out after %v", glProbeTimeout)
	case r.Err != nil:
		return r.Err.Error()
	case r.SawMesa:
		return "probe hit Mesa3D instead of hardware OpenGL"
	default:
		return fmt.Sprintf("OpenGL %d.%d", r.Major, r.Minor)
	}
}

// gateInput — всё, что нужно решению гейта. Никакого Win32 и никакого I/O:
// вызывающий собирает снимок и получает действие.
type gateInput struct {
	State    GLState
	HasState bool
	// MesaInstalled — наша Mesa лежит рядом с exe (IsMesaInstalled).
	MesaInstalled bool
	// PrevDiedUnderMesa — прошлый старт умер, НЕ дойдя до кадра, и рисовал
	// при этом через Mesa. Отличает «Mesa сломана» от «Mesa просто стоит».
	//
	// Без этого различия первый же старт после обновления у RDP-пользователя,
	// которому Mesa положила старая версия, выглядел бы как «не работает
	// ничего»: файла состояния ещё нет, значит делается проба, а железа в RDP
	// нет по определению — и исправная Mesa получала бы D5 вместо тихого
	// старта.
	PrevDiedUnderMesa bool
	// Interactive — можно показывать MessageBoxW (не -tray).
	Interactive bool
	// Probed — Probe заполнен; без этого decideGate вернёт actProbe.
	Probed bool
	Probe  probeResult
}

type gateAction int

const (
	actStart          gateAction = iota // MarkGLStarting и обычный запуск
	actProbe                            // нужна проба железа, потом decideGate ещё раз
	actAskDisableMesa                   // D1: Mesa не дожила до кадра, железо работает
	actAskTimeout                       // D2: проба не уложилась в таймаут
	actAskInstallMesa                   // D3: железа нет, Mesa рядом нет
	actNeitherWorks                     // D5: не работает ни железо, ни Mesa
)

// decideGate — единственное правило гейта (SPEC 125 §2.2):
// прошлый старт дошёл до кадра — пробу не делаем; иначе — делаем.
//
// В non-interactive (-tray) диалогов нет вообще, поэтому любое «спросить»
// вырождается в actStart: вызывающий пишет WARN и стартует как есть, файлы
// не трогая. Это гарантирует критерий приёмки №9.
func decideGate(in gateInput) gateAction {
	if !in.Probed {
		// Прошлый старт дошёл до кадра — доверяем ему и не тратим 10 секунд.
		// restart — тоже чистый исход: процесс вышел сам, чтобы применить
		// новый рендерер, и всё уже решено предыдущим стартом.
		if in.HasState && (in.State.Phase == GLPhaseRendered || in.State.Phase == GLPhaseRestart) {
			return actStart
		}
		return actProbe
	}

	switch {
	case in.Probe.ok():
		// Железо работает. Если при этом мы рисуем через Mesa и предыдущий
		// старт не дожил до кадра — предлагаем вернуться на железо.
		if in.MesaInstalled {
			if !in.Interactive {
				return actStart
			}
			return actAskDisableMesa
		}
		return actStart
	case in.Probe.Timeout:
		// Таймаут — не приговор железу: обычно это временная икота драйвера.
		// Без пользователя ничего не меняем.
		if !in.Interactive {
			return actStart
		}
		return actAskTimeout
	default:
		// Отказ: 1.1, ошибка WGL или нераспознанная версия.
		if in.MesaInstalled {
			// Железа нет, но Mesa стоит — это штатный режим RDP/ВМ без GPU,
			// ради которого Mesa и ставится. Жаловаться тут не на что.
			// Другое дело, если прошлый старт под этой самой Mesa умер: тогда
			// не работает ни то, ни другое, и об этом надо сказать.
			if in.PrevDiedUnderMesa {
				return actNeitherWorks
			}
			return actStart
		}
		if !in.Interactive {
			return actStart
		}
		return actAskInstallMesa
	}
}
