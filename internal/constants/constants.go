package constants

import "strings"

// File names
const (
	WinTunDLLName   = "wintun.dll"
	TunDLLName      = "tun.dll"
	ConfigFileName  = "config.json"
	SingBoxExecName = "sing-box"
)

// Directory names
const (
	BinDirName  = "bin"
	LogsDirName = "logs"
)

// Log file names
const (
	MainLogFileName   = "singbox-launcher.log"
	ChildLogFileName  = "sing-box.log"
	ParserLogFileName = "parser.log"
	APILogFileName    = "api.log"
)

// Process names for checking
const (
	SingBoxProcessNameWindows = "sing-box.exe"
	SingBoxProcessNameUnix    = "sing-box"
)

// Network constants
const (
	DefaultSTUNServer = "stun.l.google.com:19302"
)

// Application version
// Can be overridden at build time using -ldflags="-X singbox-launcher/internal/constants.AppVersion=..."
var (
	AppVersion = "v0.6.0" // Default version, overridden by build scripts from git tag
)

// GetMyBranch возвращает ветку репозитория для загрузки ассетов (wizard_template.json, get_free.json).
// Если в версии приложения есть суффикс после номера (например 0.7.1-96-gc1343cc или 0.7.1-dev), возвращает "develop", иначе "main".
func GetMyBranch() string {
	v := strings.TrimPrefix(AppVersion, "v")
	if strings.Contains(v, "-") {
		return "develop"
	}
	return "main"
}

// UI Theme settings
const (
	// Theme options: "dark", "light", or "default" (follows system theme)
	AppTheme = "default" // Set to "dark", "light", or "default"
)
