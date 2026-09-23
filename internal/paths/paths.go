// Package paths решает раскладку данных лаунчера (SPEC 135): где лежит
// поставляемое (AppDir, только чтение), состояние (DataDir) и логи (LogDir).
//
// Раскладка вычисляется один раз в main() функцией Resolve и дальше
// передаётся значением. Пакет — лист: только stdlib и internal/constants,
// чтобы его можно было импортировать отовсюду, включая тесты.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/constants"
)

// AppDir — каталог бинаря: поставляемые шаблон, локали, ядро. Только чтение.
type AppDir string

// DataDir — корень состояния и кэшей (внутри прежняя структура bin/…).
type DataDir string

// LogDir — каталог логов, crash.log и native-stderr.log.
type LogDir string

// Mode — каким правилом Resolve выбрана раскладка.
type Mode string

const (
	ModeEnv      Mode = "env"
	ModePortable Mode = "portable"
	ModeLegacy   Mode = "legacy"
	ModeSystem   Mode = "system"
)

// Layout — итог Resolve.
type Layout struct {
	App  AppDir
	Data DataDir
	Logs LogDir
	Mode Mode
	// EnvSource — имена сработавших переменных при Mode == ModeEnv
	// (в порядке DATA, LOG); пусто иначе.
	EnvSource []string
}

func (a AppDir) String() string  { return string(a) }
func (d DataDir) String() string { return string(d) }
func (l LogDir) String() string  { return string(l) }

// Bin — <AppDir>/bin: поставляемый шаблон, локали, ядро.
func (a AppDir) Bin() string { return filepath.Join(string(a), constants.BinDirName) }

// Bin — <DataDir>/bin: состояние, кэши, скачанное.
func (d DataDir) Bin() string { return filepath.Join(string(d), constants.BinDirName) }

// LogLine — первая строка лога старта.
func (l Layout) LogLine() string {
	s := fmt.Sprintf("layout: mode=%s app=%s data=%s logs=%s", l.Mode, l.App, l.Data, l.Logs)
	if l.Mode == ModeEnv && len(l.EnvSource) > 0 {
		s += " env=" + strings.Join(l.EnvSource, ",")
	}
	return s
}

// Executable — путь к бинарю после разворота симлинков (Homebrew,
// /nix/store): AppDir должен указывать на реальную папку с bin/locale.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// ProbeWritable — честная проба записи: создать и удалить
// <dir>/.write-probe-<pid>. Биты режима и access() на Windows с ACL врут.
func ProbeWritable(dir string) bool {
	p := filepath.Join(dir, fmt.Sprintf(".write-probe-%d", os.Getpid()))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	closeErr := f.Close()
	if err := os.Remove(p); err != nil || closeErr != nil {
		return false
	}
	return true
}

// IsAppBundle — бинарь лежит внутри macOS-бандла (<X>.app/Contents/MacOS/…).
func IsAppBundle(exe string, goos string) bool {
	if goos != "darwin" {
		return false
	}
	parts := strings.Split(filepath.ToSlash(exe), "/")
	for i := 0; i+2 < len(parts); i++ {
		if len(parts[i]) > len(".app") && strings.HasSuffix(parts[i], ".app") &&
			parts[i+1] == "Contents" && parts[i+2] == "MacOS" {
			return true
		}
	}
	return false
}

// Resolve определяет раскладку; первое сработавшее правило выигрывает
// (SPEC 135 §3.2):
//  1. SINGBOX_LAUNCHER_DATA_DIR / SINGBOX_LAUNCHER_LOG_DIR, каждая независимо;
//  2. маркер portable.txt рядом с бинарём;
//  3. унаследованная раскладка (bin/wizard_states/state.json рядом с бинарём
//     и каталог пишется);
//  4. платформенный дефолт.
//
// Правила 2 и 3 не применяются на macOS из .app.
//
// Чистая функция: exe — абсолютный путь уже после EvalSymlinks, env и goos
// подменяются в тестах, probe — проба записи каталога (в проде ProbeWritable).
func Resolve(exe string, env func(string) string, goos string, probe func(dir string) bool) (Layout, error) {
	app := AppDir(filepath.Dir(exe))

	var envSource []string
	envData, err := envAbs(env, constants.EnvDataDir)
	if err != nil {
		return Layout{}, err
	}
	if envData != "" {
		envSource = append(envSource, constants.EnvDataDir)
	}
	envLogs, err := envAbs(env, constants.EnvLogDir)
	if err != nil {
		return Layout{}, err
	}
	if envLogs != "" {
		envSource = append(envSource, constants.EnvLogDir)
	}

	var l Layout
	if envData != "" {
		l = Layout{App: app, Data: DataDir(envData), Logs: LogDir(filepath.Join(envData, constants.LogsDirName))}
	} else {
		l, err = resolveWithoutEnv(app, exe, env, goos, probe)
		if err != nil {
			return Layout{}, err
		}
	}
	if envLogs != "" {
		l.Logs = LogDir(envLogs)
	}
	if len(envSource) > 0 {
		l.Mode = ModeEnv
		l.EnvSource = envSource
	}
	return l, nil
}

// envAbs — значение переменной, приведённое к абсолютному пути; "" если не задана.
func envAbs(env func(string) string, name string) (string, error) {
	v := env(name)
	if v == "" {
		return "", nil
	}
	abs, err := filepath.Abs(v)
	if err != nil {
		return "", fmt.Errorf("%s=%q: %w", name, v, err)
	}
	return abs, nil
}

// resolveWithoutEnv — правила 2–4.
func resolveWithoutEnv(app AppDir, exe string, env func(string) string, goos string, probe func(string) bool) (Layout, error) {
	bundle := IsAppBundle(exe, goos)
	if !bundle {
		if _, err := os.Stat(filepath.Join(string(app), constants.PortableMarkerFileName)); err == nil {
			return portable(app, ModePortable), nil
		}
		legacyState := filepath.Join(app.Bin(), constants.WizardStatesDirName, constants.WizardStateFileName)
		if _, err := os.Stat(legacyState); err == nil && probe(string(app)) {
			return portable(app, ModeLegacy), nil
		}
	}

	switch {
	case goos == "windows":
		root := env("LOCALAPPDATA")
		if root == "" {
			if home := env("USERPROFILE"); home != "" {
				root = filepath.Join(home, "AppData", "Local")
			}
		}
		if root == "" {
			if probe(string(app)) {
				return portable(app, ModePortable), nil
			}
			return Layout{}, fmt.Errorf("cannot determine data directory: LOCALAPPDATA and USERPROFILE are empty and %s is not writable; set %s", app, constants.EnvDataDir)
		}
		data := filepath.Join(root, constants.DataDirAppName)
		return Layout{App: app, Data: DataDir(data), Logs: LogDir(filepath.Join(data, constants.LogsDirName)), Mode: ModeSystem}, nil

	case goos == "darwin" && !bundle:
		// Голый бинарь macOS — portable без маркера (SPEC 022).
		return portable(app, ModePortable), nil

	case goos == "darwin":
		home := env("HOME")
		if home == "" {
			return Layout{}, errHomeEmpty
		}
		return Layout{
			App:  app,
			Data: DataDir(filepath.Join(home, "Library", "Application Support", constants.DataDirAppName)),
			Logs: LogDir(filepath.Join(home, "Library", "Logs", constants.DataDirAppName)),
			Mode: ModeSystem,
		}, nil

	default: // linux и прочие unix: XDG
		home := env("HOME")
		dataRoot := xdgDir(env, "XDG_DATA_HOME")
		stateRoot := xdgDir(env, "XDG_STATE_HOME")
		if (dataRoot == "" || stateRoot == "") && home == "" {
			return Layout{}, errHomeEmpty
		}
		if dataRoot == "" {
			dataRoot = filepath.Join(home, ".local", "share")
		}
		if stateRoot == "" {
			stateRoot = filepath.Join(home, ".local", "state")
		}
		return Layout{
			App:  app,
			Data: DataDir(filepath.Join(dataRoot, constants.DataDirAppName)),
			Logs: LogDir(filepath.Join(stateRoot, constants.DataDirAppName, constants.LogsDirName)),
			Mode: ModeSystem,
		}, nil
	}
}

var errHomeEmpty = errors.New("cannot determine data directory: HOME is empty; set " + constants.EnvDataDir)

// xdgDir — значение XDG-переменной; относительный путь по спецификации XDG
// недействителен и игнорируется.
func xdgDir(env func(string) string, name string) string {
	v := env(name)
	if v == "" || !filepath.IsAbs(v) {
		return ""
	}
	return v
}

func portable(app AppDir, mode Mode) Layout {
	return Layout{App: app, Data: DataDir(app), Logs: LogDir(filepath.Join(string(app), constants.LogsDirName)), Mode: mode}
}

// PathsInfo — всё, что показывают пользователю о раскладке (SPEC 135 §4.1):
// один блок для раздела Storage, кнопки Copy paths, флага -paths и
// GET /debug/paths. Заполняет core (AppController.PathsInfo, PathsInfoFor):
// пакет-лист не знает ни про ядро, ни про шаблон.
type PathsInfo struct {
	Layout         Layout
	CorePath       string // FileService.SingboxPath
	CoreSource     string // env|data|app|path|"" (не найдено)
	CoreVersion    string // "" — ещё не известна
	ShadowedCore   string // второе найденное ядро, затенённое выбранным
	TemplatePath   string
	TemplateSource string // data|app|"" (шаблона нет)
	WintunPath     string // только Windows, иначе ""
	WintunFound    bool
}

// pathsInfoNone — подстановка пустого значения в Lines.
const pathsInfoNone = "(none)"

func orNone(s string) string {
	if s == "" {
		return pathsInfoNone
	}
	return s
}

// Lines — строки "Key: value" в фиксированном порядке: Mode (с
// переменными окружения), Program, Data, Logs, Core (путь, версия,
// источник), Shadowed core (только если есть), Template (путь, источник),
// wintun (только когда путь известен, то есть на Windows). Пустые значения —
// "(none)". Текст не локализуется: его прикладывают к issue.
func (p PathsInfo) Lines() []string {
	mode := orNone(string(p.Layout.Mode))
	if len(p.Layout.EnvSource) > 0 {
		mode += " (" + strings.Join(p.Layout.EnvSource, ", ") + ")"
	}
	lines := []string{
		"Mode: " + mode,
		"Program: " + orNone(string(p.Layout.App)),
		"Data: " + orNone(string(p.Layout.Data)),
		"Logs: " + orNone(string(p.Layout.Logs)),
		fmt.Sprintf("Core: %s (version: %s, source: %s)", orNone(p.CorePath), orNone(p.CoreVersion), orNone(p.CoreSource)),
	}
	if p.ShadowedCore != "" {
		lines = append(lines, "Shadowed core: "+p.ShadowedCore)
	}
	lines = append(lines, fmt.Sprintf("Template: %s (source: %s)", orNone(p.TemplatePath), orNone(p.TemplateSource)))
	if p.WintunPath != "" {
		found := "not found"
		if p.WintunFound {
			found = "found"
		}
		lines = append(lines, fmt.Sprintf("wintun: %s (%s)", p.WintunPath, found))
	}
	return lines
}

// Text — Lines через "\n": для Copy paths и -paths.
func (p PathsInfo) Text() string {
	return strings.Join(p.Lines(), "\n")
}
