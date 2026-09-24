package constants

import (
	"fmt"
	"strings"
)

// File names
const (
	WinTunDLLName          = "wintun.dll"
	TunDLLName             = "tun.dll"
	ConfigFileName         = "config.json"
	SingBoxExecName        = "sing-box"
	WizardTemplateFileName = "wizard_template.json"
	// WizardTemplateVersionFileName — маркер рядом с шаблоном: версия лаунчера,
	// под которую шаблон положил УСТАНОВЩИК (архив win64-full, install-macos.sh).
	// Совпал с AppVersion — шаблон свежий, и первый запуск его не перекачивает
	// (core.RefreshTemplateIfStale). Сам лаунчер маркер не пишет.
	WizardTemplateVersionFileName = "wizard_template.version"
	// MesaBundleDirName — папка рядом с exe, куда архив win64-full кладёт DLL
	// Mesa3D. Не рядом с exe напрямую: лежащий рядом opengl32.dll загрузчик
	// Windows взял бы всегда, и машина с живым GPU рисовала бы через llvmpipe.
	// Из папки DLL копируются только когда проба показала, что аппаратного
	// OpenGL нет (platform.EnsureDesktopOpenGL).
	MesaBundleDirName = "mesa3d"
	// LegacyRemoteConfigFileName — bin/remote-config.json, единственный
	// конфиг удалённой машины до SPEC 098.
	//
	// Больше не пишется: у каждой машины свой config.json в её директории
	// (platform.GetRemoteConfigPathFor). Константа осталась только ради
	// миграции, которая читает старый файл и переносит его владельцу.
	LegacyRemoteConfigFileName = "remote-config.json"
	WizardStateFileName        = "state.json"
	// OutboundsCacheFileName — кеш-файл outbounds (SPEC 045 phase 5.1).
	// Лежит в <DataDir>/bin/. Scope = последний активный state. Парсер
	// перезаписывает его при каждом успешном Update; на переключении
	// state'а файл не инвалидируется (см. PLAN.md outboundscache).
	OutboundsCacheFileName = "outbounds.cache.json"
	// GLStateFileName — bin/gl-state.json, единственное состояние GL-гейта
	// (SPEC 125). Гейт перед инициализацией GL пишет phase=starting, первый
	// отрисованный кадр переписывает на phase=rendered. Правило гейта: прошлый
	// старт дошёл до кадра — пробу не делаем. Файл можно удалить руками, это
	// полный сброс решений гейта.
	GLStateFileName = "gl-state.json"
	// MesaDisabledSuffix — суффикс, которым гейт и кнопка в Диагностике
	// отключают Mesa3D: DLL рядом с exe переименовываются в <имя>.off.
	// Переименование, а не удаление: откат обратно не требует ни сети, ни
	// папки mesa3d/ (её нет в архиве win64).
	MesaDisabledSuffix = ".off"
)

// Directory names
const (
	BinDirName          = "bin"
	LogsDirName         = "logs"
	RuleSetsDirName     = "rule-sets"
	WizardStatesDirName = "wizard_states"
	// SubscriptionsDirName — каталог raw-body cache подписок (SPEC 052):
	// <DataDir>/bin/subscriptions/<source-id>.raw. Один файл per Source(id),
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
	// TailscaleDirName — корень каталогов состояния tailnet под bin/ (SPEC 122).
	TailscaleDirName = "tailscale"
	// TempDirName — временный каталог скачивания ядра и wintun (под DataDir, не под bin/).
	TempDirName = "temp"
	// DaemonIdentityDirName — клиентская пара сопряжения с локальным демоном под bin/.
	DaemonIdentityDirName = "daemon"
	// RemoteDaemonsDirName — клиентские пары удалённых демонов под bin/, по каталогу на машину.
	RemoteDaemonsDirName = "remote-daemons"
)

// Data layout (SPEC 135): где лежат поставляемое (AppDir), состояние (DataDir)
// и логи (LogDir). Раскладку решает internal/paths.Resolve.
const (
	// PortableMarkerFileName — маркер рядом с бинарём: данные и логи живут в
	// каталоге бинаря. Содержимое не читается, достаточно существования.
	PortableMarkerFileName = "portable.txt"
	// DataDirAppName — имя каталога приложения в платформенных корнях
	// ($XDG_DATA_HOME, ~/Library/Application Support, %LOCALAPPDATA%).
	DataDirAppName = "singbox-launcher"
	// EnvDataDir и EnvLogDir переопределяют DataDir и LogDir независимо
	// друг от друга (Flatpak-обёртки, пакеты, CI, отладка).
	EnvDataDir = "SINGBOX_LAUNCHER_DATA_DIR"
	EnvLogDir  = "SINGBOX_LAUNCHER_LOG_DIR"
	// EnvCorePath — явный путь к бинарю ядра; первый в цепочке поиска
	// (SPEC 135 §3.3: env → Data/bin → App/bin → PATH). Срабатывает, только
	// если файл существует.
	EnvCorePath = "SINGBOX_LAUNCHER_CORE"
	// MigratedFromMarkerFileName — маркер в DataDir после миграции данных из
	// старой раскладки (SPEC 135 §3.4).
	MigratedFromMarkerFileName = ".migrated_from"
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
	CrashLogFileName  = "crash.log" // трасса фатальной паники (debuglog.EnableCrashOutput)
	// NativeStderrLogFileName — stderr чужого нативного кода (Mesa, драйверы,
	// GLFW). У windowsgui-сборки stderr ведёт в INVALID_HANDLE_VALUE, и всё,
	// что пишут DLL, пропадает бесследно; debuglog.RedirectNativeStderr
	// подменяет системный хендл на этот файл (SPEC 125 §2.8).
	NativeStderrLogFileName = "native-stderr.log"
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
	// ContractWarningsDocBaseURL — база ссылки «Подробнее» у предупреждения
	// узла (SPEC 131 §6). Полный адрес = база + код: у каждого кода в
	// сгенерированном документе свой якорь `<a id="<code>">`.
	//
	// Документ живёт в ЭТОМ репозитории (contract/docs/generated/warnings.md):
	// он генерируется из реестра контракта, который лежит здесь же, и
	// обновляется той же волной, что и коды. Ветка main, а не тег: у
	// пользователя релизная сборка, и ссылка обязана вести на актуальный
	// текст, а не на срез времени сборки.
	ContractWarningsDocBaseURL = "https://github.com/Leadaxe/singbox-launcher/blob/main/contract/docs/generated/warnings.md#"
)

// sing-box core download source (SPEC 072, Variant A). The launcher ships the
// sing-box-lx fork (upstream + XHTTP `with_xhttp` + AmneziaWG `with_awg`) on
// every platform — including Windows 7 (32-bit), which the fork now builds as a
// `windows-386-legacy-windows-7` asset. See coreReleaseRepo() in core_downloader.go.
// GitHubDownloadMirrors are prefix-style GitHub proxies tried when the release
// CDN itself is unreachable. Each entry is prepended to a full
// https://github.com/… URL.
//
// ghproxy.com is deliberately absent: it answers every request with HTTP 200
// and its own ~1.8 KB HTML landing page instead of the file, so the download
// "succeeds" and only fails later as a bogus corrupted-archive error.
var GitHubDownloadMirrors = []string{
	"https://ghfast.top/",
	"https://gh-proxy.com/",
}

const SingboxCoreRepo = "Leadaxe/sing-box-lx" // core for all platforms (XHTTP + AmneziaWG)

// Pinned sing-box core version for this launcher build (SPEC 046 / 072).
// A fork tag `X.Y.Z-lx.N` — the fork binary prints the full tag in
// `sing-box version`, so the strict-equality reinstall check still holds.
// Manually bumped per release; source-of-truth here. See
// docs/RELEASE_PROCESS.md §5.1.
const RequiredCoreVersion = "1.14.2-lx.2"

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
	RequiredTemplateRef = "9736db76558c6b1ba9047a471c31f89b6354d227"
)

// GetMyBranch возвращает ветку репозитория для загрузки ассетов, у которых нет
// pinned-ref модели (например, переводы локалей, wintun zip).
//
// Если в версии приложения есть суффикс после номера (например 0.7.1-96-gc1343cc или 0.7.1-dev), возвращает "develop", иначе "main".
func GetMyBranch() string {
	v := strings.TrimPrefix(AppVersion, "v")
	if strings.Contains(v, "-") {
		return "develop"
	}
	return "main"
}

// IsDevBuild — сборка не из релизного тега: `v-local-test` (значение по
// умолчанию), `unnamed-dev` (значение build-скрипта), `git describe` с
// суффиксом коммитов или `-dirty`.
//
// Тот же признак, по которому GetMyBranch выбирает develop; вынесен
// отдельно, потому что о нём спрашивают и там, где ветка не нужна.
func IsDevBuild() bool {
	v := strings.TrimPrefix(AppVersion, "v")
	if v == "" {
		return true
	}
	if strings.HasPrefix(v, "-local-test") || strings.Contains(v, "unnamed-dev") {
		return true
	}
	return strings.Contains(v, "-")
}

// UI Theme settings
const (
	// Theme options: "dark", "light", or "default" (follows system theme)
	AppTheme = "default" // Set to "dark", "light", or "default"
)

// CompareVersions сравнивает две версии (формат X.Y.Z или X.Y.Z-N-hash или X.Y.Z-dev.branch-hash).
// Возвращает: -1 если v1 < v2, 0 если v1 == v2, 1 если v1 > v2.
//
// Живёт здесь, а не в core: правило выбора шаблона (core/template,
// SPEC 135 §3.3) сравнивает штамп с AppVersion, а core/template не может
// импортировать core. core.CompareVersions — обёртка над этой функцией.
func CompareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	base1, hasSuffix1 := extractBaseVersion(v1)
	base2, hasSuffix2 := extractBaseVersion(v2)

	baseCompare := compareBaseVersions(base1, base2)
	if baseCompare != 0 {
		return baseCompare
	}

	// Если базовые версии равны — версия с суффиксом (коммиты после тега
	// или dev) считается новее. v0.7.1-96-gc1343cc > v0.7.1.
	if hasSuffix1 && !hasSuffix2 {
		return 1
	}
	if !hasSuffix1 && hasSuffix2 {
		return -1
	}

	return 0
}

// extractBaseVersion извлекает базовую версию и проверяет наличие суффикса.
// Форматы: "0.7.1", "0.7.1-96-gc1343cc", "0.7.1-dev.branch-hash".
func extractBaseVersion(version string) (base string, hasSuffix bool) {
	idx := strings.Index(version, "-")
	if idx == -1 {
		return version, false
	}
	return version[:idx], true
}

// compareBaseVersions сравнивает базовые версии (формат X.Y.Z).
func compareBaseVersions(base1, base2 string) int {
	parts1 := strings.Split(base1, ".")
	parts2 := strings.Split(base2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &num1)
		}
		if i < len(parts2) {
			_, _ = fmt.Sscanf(parts2[i], "%d", &num2)
		}

		if num1 < num2 {
			return -1
		}
		if num1 > num2 {
			return 1
		}
	}

	return 0
}
