package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/paths"
)

// Источник выбранного ядра (CoreResolution.Source).
const (
	CoreSourceEnv  = "env"  // SINGBOX_LAUNCHER_CORE
	CoreSourceData = "data" // <Data>/bin/sing-box — скачанное или положенное руками
	CoreSourceApp  = "app"  // <App>/bin/sing-box — поставляемое
	CoreSourcePath = "path" // системный PATH
)

// CoreResolution — итог поиска ядра.
type CoreResolution struct {
	// Path — путь для запуска. Ничего не найдено → <Data>/bin/sing-box
	// (цель скачивания), Source пуст.
	Path string
	// Source — CoreSourceEnv / Data / App / Path; "" — ядро не найдено.
	Source string
	// Shadowed — второе найденное ядро, которое выбранное затеняет: при
	// Source == data — поставляемое из App; при env — первое из Data/App.
	// Иначе пусто. Нужен только для строки лога при старте.
	Shadowed string
}

// ResolveSingboxExecPath ищет ядро по цепочке SPEC 135 §3.3, одинаковой на
// всех платформах:
//
//	SINGBOX_LAUNCHER_CORE → <Data>/bin/sing-box → <App>/bin/sing-box → PATH.
//
// Data побеждает всегда, даже если поставляемое новее: туда кладут
// dev-сборки руками. Системный sing-box из PATH последний — лаунчеру нужен
// форк lx (XHTTP, AWG), дистрибутивный бинарь почти всегда не тот.
//
// env подменяется в тестах (в проде os.Getenv).
func ResolveSingboxExecPath(l paths.Layout, env func(string) string) CoreResolution {
	name := GetExecutableNames()
	dataPath := filepath.Join(l.Data.Bin(), name)
	appPath := ""
	if l.App != "" {
		appPath = filepath.Join(l.App.Bin(), name)
		if filepath.Clean(appPath) == filepath.Clean(dataPath) {
			appPath = "" // portable/legacy: это один и тот же файл
		}
	}
	dataOK := isRegularFile(dataPath)
	appOK := appPath != "" && isRegularFile(appPath)

	if env != nil {
		if p := strings.TrimSpace(env(constants.EnvCorePath)); p != "" {
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
			if isRegularFile(p) {
				r := CoreResolution{Path: p, Source: CoreSourceEnv}
				switch {
				case dataOK:
					r.Shadowed = dataPath
				case appOK:
					r.Shadowed = appPath
				}
				return r
			}
			debuglog.WarnLog("core: %s=%q is not a file — ignored", constants.EnvCorePath, p)
		}
	}
	if dataOK {
		r := CoreResolution{Path: dataPath, Source: CoreSourceData}
		if appOK {
			r.Shadowed = appPath
		}
		return r
	}
	if appOK {
		return CoreResolution{Path: appPath, Source: CoreSourceApp}
	}
	if p, err := exec.LookPath(name); err == nil && isRegularFile(p) {
		return CoreResolution{Path: p, Source: CoreSourcePath}
	}
	return CoreResolution{Path: dataPath}
}

// isRegularFile — путь существует и это не каталог.
func isRegularFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
