package constants

import "strings"

// File names
const (
	WinTunDLLName          = "wintun.dll"
	TunDLLName             = "tun.dll"
	ConfigFileName         = "config.json"
	SingBoxExecName        = "sing-box"
	WizardTemplateFileName = "wizard_template.json"
	// LegacyRemoteConfigFileName — bin/remote-config.json, единственный
	// конфиг удалённой машины до SPEC 098.
	//
	// Больше не пишется: у каждой машины свой config.json в её директории
	// (platform.GetRemoteConfigPathFor). Константа осталась только ради
	// миграции, которая читает старый файл и переносит его владельцу.
	LegacyRemoteConfigFileName = "remote-config.json"
	WizardStateFileName        = "state.json"
	// OutboundsCacheFileName — кеш-файл outbounds (SPEC 045 phase 5.1).
	// Лежит в <execDir>/bin/. Scope = последний активный state. Парсер
	// перезаписывает его при каждом успешном Update; на переключении
	// state'а файл не инвалидируется (см. PLAN.md outboundscache).
	OutboundsCacheFileName = "outbounds.cache.json"
)

// Directory names
const (
	BinDirName          = "bin"
	LogsDirName         = "logs"
	RuleSetsDirName     = "rule-sets"
	WizardStatesDirName = "wizard_states"
	// SubscriptionsDirName — каталог raw-body cache подписок (SPEC 052):
	// <execDir>/bin/subscriptions/<source-id>.raw. Один файл per Source(id),
	// атомарная запись через .tmp + Rename, lazy GC orphan-файлов.
	//
	// SPEC 098: для удалённой машины тот же базовый имя каталога, но внутри
	// её директории — bin/wizard_states/remote/<id>/subscriptions/.
	SubscriptionsDirName = "subscriptions"
	// RemoteRuleSetsDirName — каталог .srs УДАЛЁННОЙ машины (SPEC 098 §2.3):
	// bin/wizard_states/remote/<id>/srs/.
	//
	// Имя короче локального rule-sets/ намеренно: путь и так длинный, а
	// каталог лежит внутри директории машины, где двусмысленности нет.
	RemoteRuleSetsDirName = "srs"
)

// Config targets (SPEC 097) — для какой машины лаунчер готовит config.json.
//
// ConfigTargetLocal — эта машина: состояние живёт прямо в
// bin/wizard_states/ (исторический плоский layout, без миграции).
// ConfigTargetRemote — удалённая машина (сервер, роутер, другой mac):
// состояние в bin/wizard_states/remote/. Слаг = имя подпапки.
//
// Значение попадает в state.meta.target и в @runtime.target шаблона.
const (
	ConfigTargetLocal  = "local"
	ConfigTargetRemote = "remote"
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
	SingboxReleasesURL = "https://github.com/Leadaxe/sing-box-lx/releases"
	WintunHomeURL      = "https://www.wintun.net/"
	// RemoteDaemonDocsURL — как поставить `sing-box lxd` на машину, которой
	// хочется управлять из лаунчера (SPEC 098, окно добавления машины).
	//
	// Документ живёт в форке ядра (docs-lx/lxd-daemon.md, ветка lx), а не в
	// репозитории лаунчера: демон — часть ядра, и инструкция обновляется
	// вместе с ним. BUILD_LINUX.md тут был неверной ссылкой — он про сборку
	// самого лаунчера, а не про установку демона.
	// Якорь ведёт сразу в раздел про Linux: удалённая машина почти всегда
	// роутер или VPS, и начало документа (что такое демон, установка на
	// macOS) на этом шаге только отвлекает.
	RemoteDaemonDocsURL = "https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#8-linux--setup-approaches"
)

// sing-box core download source (SPEC 072, Variant A). The launcher ships the
// sing-box-lx fork (upstream + XHTTP `with_xhttp` + AmneziaWG `with_awg`) on
// every platform — including Windows 7 (32-bit), which the fork now builds as a
// `windows-386-legacy-windows-7` asset. See coreReleaseRepo() in core_downloader.go.
const SingboxCoreRepo = "Leadaxe/sing-box-lx" // core for all platforms (XHTTP + AmneziaWG)

// Pinned sing-box core version for this launcher build (SPEC 046 / 072).
// A fork tag `X.Y.Z-lx.N` — the fork binary prints the full tag in
// `sing-box version`, so the strict-equality reinstall check still holds.
// Manually bumped per release; source-of-truth here. See
// docs/RELEASE_PROCESS.md §5.1.
const RequiredCoreVersion = "1.14.0-lx.26"

// AppVersion — git describe output. Set by build scripts via -ldflags.
//
// RequiredTemplateRef — pinned commit ref of wizard_template.json. CI build
// scripts overwrite the source-default via `-ldflags` using
// `git rev-parse HEAD`, so each release ships a binary that fetches the
// exact template snapshot it was tested against. The source-default below
// is bumped by the maintainer in §1.5 of RELEASE_PROCESS.md after every
// merge of main back into develop — local `go run .` builds (which don't
// pass ldflags) thus get a stable pinned ref instead of a moving branch
// HEAD. See docs/RELEASE_PROCESS.md §5.2.
var (
	AppVersion          = "v-local-test"
	RequiredTemplateRef = "cdb46b2046da023ae1b5fe16c5c6c70768727841"
)

// GetMyBranch возвращает ветку репозитория для загрузки ассетов, у которых нет
// pinned-ref модели (например, переводы локалей, wintun zip). wizard_template.json
// больше не использует эту функцию — он pin'ится через RequiredTemplateRef.
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
