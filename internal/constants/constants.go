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
	BinDirName     = "bin"
	LogsDirName    = "logs"
	RuleSetsDirName = "rule-sets"
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

// Manual download URLs (shown when automatic download fails)
const (
	SingboxReleasesURL = "https://github.com/SagerNet/sing-box/releases"
	WintunHomeURL      = "https://www.wintun.net/"
)

// Application version + pinned dependency refs (see SPEC 046).
//
// AppVersion — git describe output, set by build scripts via -ldflags.
//
// RequiredCoreVersion — pinned sing-box version for this launcher build.
// Manually bumped per release; the source-of-truth lives here, not in
// auto-discovered GitHub latest. See docs/RELEASE_PROCESS.md.
//
// RequiredTemplateRef — pinned commit ref of wizard_template.json. CI
// build scripts overwrite the source-default via -ldflags using
// `git rev-parse HEAD`, so each release ships a binary that fetches the
// exact template snapshot it was tested against. The source-default below
// is updated by the maintainer in §1.5 of RELEASE_PROCESS.md after every
// merge of main back into develop, so that local `go run .` builds (which
// don't pass ldflags) still get a stable pinned ref instead of a moving
// branch HEAD.
var (
	AppVersion          = "v-local-test"
	RequiredCoreVersion = "1.13.11"
	// Source-default = origin/main HEAD at the time this constant was last
	// bumped. Overwritten by build scripts via -ldflags.
	RequiredTemplateRef = "e6f2fb1b0f38547412623aeb3af7f0aea5223fd7"
)

// GetMyBranch возвращает ветку репозитория для загрузки ассетов, у которых нет
// pinned-ref модели (например, get_free.json). wizard_template.json больше не
// использует эту функцию — он pin'ится через RequiredTemplateRef.
//
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
