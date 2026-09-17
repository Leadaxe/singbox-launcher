package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// SPEC 044 follow-up (the deferred "sing-box binary feature-probe"): naive
// outbound exists only in cores built with `with_naive_outbound`, and purego
// builds additionally load libcronet.{dll,dylib,so} at runtime from the
// binary's directory / library search path. A single naive node on a core
// without support fails `sing-box check` for the WHOLE config — the probe
// below lets the generator degrade naive nodes instead (same policy as
// broken-URI nodes: drop the node, keep the config).
//
// The verdict is conservative: any uncertainty (no core binary, version call
// failed, no "Tags:" line) → "supported", so we never silently drop nodes on
// guesswork; `sing-box check` remains the backstop for exotic cores.

// naiveSupportVerdict — cached probe result, keyed by the core binary's
// (mtime, size) so a core reinstall in the same session re-probes.
type naiveSupportVerdict struct {
	binMtime  time.Time
	binSize   int64
	supported bool
	reason    string
}

// CoreSupportsNaive reports whether the installed sing-box core can create
// naive outbounds, with a human-readable reason when it can't.
func (ac *AppController) CoreSupportsNaive() (bool, string) {
	if ac == nil || ac.FileService == nil {
		return true, ""
	}
	singboxPath := ac.FileService.SingboxPath
	if resolved, err := exec.LookPath(singboxPath); err == nil {
		singboxPath = resolved
	}
	st, err := os.Stat(singboxPath)
	if err != nil {
		return true, "" // no core installed — nothing to probe, check is skipped anyway
	}

	ac.naiveSupportCacheMu.Lock()
	defer ac.naiveSupportCacheMu.Unlock()
	if c := ac.naiveSupportCache; c != nil && c.binMtime.Equal(st.ModTime()) && c.binSize == st.Size() {
		return c.supported, c.reason
	}

	supported, reason := probeNaiveSupport(singboxPath)
	ac.naiveSupportCache = &naiveSupportVerdict{
		binMtime:  st.ModTime(),
		binSize:   st.Size(),
		supported: supported,
		reason:    reason,
	}
	if !supported {
		debuglog.WarnLog("CoreSupportsNaive: %s", reason)
	}
	return supported, reason
}

// probeNaiveSupport runs `sing-box version` and derives the verdict from the
// build tags plus (for purego builds) libcronet presence.
func probeNaiveSupport(singboxPath string) (bool, string) {
	cmd := exec.Command(singboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("probeNaiveSupport: sing-box version failed: %v", err)
		return true, ""
	}
	return naiveVerdictFromVersionOutput(string(output), cronetLibAvailable(singboxPath))
}

var versionTagsRegex = regexp.MustCompile(`(?m)^Tags:\s*(\S+)`)

// naiveVerdictFromVersionOutput — pure part of the probe, unit-testable.
// libAvailable is the cronetLibAvailable result for the same binary.
func naiveVerdictFromVersionOutput(versionOutput string, libAvailable bool) (bool, string) {
	m := versionTagsRegex.FindStringSubmatch(versionOutput)
	if m == nil {
		return true, "" // unknown output format — don't degrade on guesswork
	}
	tags := splitBuildTags(m[1])
	hasTag := func(want string) bool {
		for _, t := range tags {
			if t == want {
				return true
			}
		}
		return false
	}
	if !hasTag("with_naive_outbound") {
		return false, "sing-box core is built without with_naive_outbound"
	}
	// Static (musl/CGO) builds link cronet in; only purego builds need the
	// companion library at runtime.
	if hasTag("with_purego") && !libAvailable {
		return false, fmt.Sprintf("sing-box core needs %s next to the binary — re-download the core to get it", cronetLibName())
	}
	return true, ""
}

// SPEC 122: то же самое для tailscale. Endpoint типа `tailscale` есть только
// в ядрах, собранных с `with_tailscale` (форк с 1.14.0-lx.31); один такой
// узел на ядре без тега валит `sing-box check` для ВСЕГО конфига, поэтому
// генератор выбрасывает такие узлы с warning — ровно как naive.

// tailscaleSupportVerdict — кэш вердикта по (mtime, size) бинаря ядра.
type tailscaleSupportVerdict struct {
	binMtime  time.Time
	binSize   int64
	supported bool
	reason    string
}

// tailscaleBuildTag — тег сборки ядра, дающий endpoint типа tailscale.
const tailscaleBuildTag = "with_tailscale"

// CoreSupportsTailscale reports whether the installed sing-box core can create
// tailscale endpoints, with a human-readable reason when it can't.
func (ac *AppController) CoreSupportsTailscale() (bool, string) {
	if ac == nil || ac.FileService == nil {
		return true, ""
	}
	singboxPath := ac.FileService.SingboxPath
	if resolved, err := exec.LookPath(singboxPath); err == nil {
		singboxPath = resolved
	}
	st, err := os.Stat(singboxPath)
	if err != nil {
		return true, "" // no core installed — nothing to probe, check is skipped anyway
	}

	ac.tailscaleSupportCacheMu.Lock()
	defer ac.tailscaleSupportCacheMu.Unlock()
	if c := ac.tailscaleSupportCache; c != nil && c.binMtime.Equal(st.ModTime()) && c.binSize == st.Size() {
		return c.supported, c.reason
	}

	supported, reason := probeTailscaleSupport(singboxPath)
	ac.tailscaleSupportCache = &tailscaleSupportVerdict{
		binMtime:  st.ModTime(),
		binSize:   st.Size(),
		supported: supported,
		reason:    reason,
	}
	if !supported {
		debuglog.WarnLog("CoreSupportsTailscale: %s", reason)
	}
	return supported, reason
}

// probeTailscaleSupport runs `sing-box version` and derives the verdict from
// the build tags.
func probeTailscaleSupport(singboxPath string) (bool, string) {
	cmd := exec.Command(singboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("probeTailscaleSupport: sing-box version failed: %v", err)
		return true, ""
	}
	return tailscaleVerdictFromVersionOutput(string(output))
}

// tailscaleVerdictFromVersionOutput — pure part of the probe, unit-testable.
func tailscaleVerdictFromVersionOutput(versionOutput string) (bool, string) {
	m := versionTagsRegex.FindStringSubmatch(versionOutput)
	if m == nil {
		return true, "" // unknown output format — don't degrade on guesswork
	}
	for _, t := range splitBuildTags(m[1]) {
		if t == tailscaleBuildTag {
			return true, ""
		}
	}
	return false, fmt.Sprintf("sing-box core is built without %s (need 1.14.0-lx.31 or newer)", tailscaleBuildTag)
}

// SPEC 123: то же самое для полей AmneziaWG 3.x на wireguard-узле. Здесь
// одного тега сборки мало: `with_awg` есть и в старых ядрах, а ключи
// header_protection_key / random_trailers / диапазонные тайминги появились
// только в 1.14.0-lx.32. Ядро постарше отвергает конфиг ЦЕЛИКОМ («невалидный
// JSON»), поэтому вердикт складывается из тега и версии.

// awg3SupportVerdict — кэш вердикта по (mtime, size) бинаря ядра.
type awg3SupportVerdict struct {
	binMtime  time.Time
	binSize   int64
	supported bool
	reason    string
}

const (
	// awgBuildTag — тег сборки ядра, дающий AmneziaWG вообще.
	awgBuildTag = "with_awg"
	// awg3MinLxRelease — минимальный номер релиза форка в суффиксе `-lx.N`
	// поверх базовой 1.14.0, начиная с которого ядро знает поля AWG 3.x.
	awg3MinLxRelease = 32
	// awg3MinCoreVersion — та же граница строкой, для текста причины и
	// сравнения базовых версий.
	awg3MinCoreVersion = "1.14.0-lx.32"
)

// CoreSupportsAWG3 reports whether the installed sing-box core understands
// AmneziaWG 3.x endpoint fields, with a human-readable reason when it can't.
func (ac *AppController) CoreSupportsAWG3() (bool, string) {
	if ac == nil || ac.FileService == nil {
		return true, ""
	}
	singboxPath := ac.FileService.SingboxPath
	if resolved, err := exec.LookPath(singboxPath); err == nil {
		singboxPath = resolved
	}
	st, err := os.Stat(singboxPath)
	if err != nil {
		return true, "" // ядра нет — пробовать нечего, check всё равно не запустится
	}

	ac.awg3SupportCacheMu.Lock()
	defer ac.awg3SupportCacheMu.Unlock()
	if c := ac.awg3SupportCache; c != nil && c.binMtime.Equal(st.ModTime()) && c.binSize == st.Size() {
		return c.supported, c.reason
	}

	supported, reason := probeAWG3Support(singboxPath)
	ac.awg3SupportCache = &awg3SupportVerdict{
		binMtime:  st.ModTime(),
		binSize:   st.Size(),
		supported: supported,
		reason:    reason,
	}
	if !supported {
		debuglog.WarnLog("CoreSupportsAWG3: %s", reason)
	}
	return supported, reason
}

// probeAWG3Support runs `sing-box version` and derives the verdict from the
// build tags plus the core version.
func probeAWG3Support(singboxPath string) (bool, string) {
	cmd := exec.Command(singboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("probeAWG3Support: sing-box version failed: %v", err)
		return true, ""
	}
	return awg3VerdictFromVersionOutput(string(output))
}

// awg3LxReleaseRegex — номер релиза форка в суффиксе версии (`-lx.32`,
// `-lx.32-rc.1`).
var awg3LxReleaseRegex = regexp.MustCompile(`-lx\.(\d+)`)

// awg3VerdictFromVersionOutput — чистая часть пробы, проверяемая тестом.
//
// Политика та же, что у naive/tailscale: деградируем только по положительному
// свидетельству. Нет строки `Tags:`, нет разбираемой версии — считаем, что
// ядро умеет; `sing-box check` остаётся последним рубежом.
func awg3VerdictFromVersionOutput(versionOutput string) (bool, string) {
	m := versionTagsRegex.FindStringSubmatch(versionOutput)
	if m == nil {
		return true, "" // формат неизвестен — не деградируем по догадке
	}
	hasAWG := false
	for _, t := range splitBuildTags(m[1]) {
		if t == awgBuildTag {
			hasAWG = true
			break
		}
	}
	if !hasAWG {
		// Ядро без AmneziaWG вообще — версия тут не при чём, и звать
		// обновляться до lx.32 некуда: нужен другой билд.
		return false, fmt.Sprintf("sing-box core is built without %s — AmneziaWG nodes are unavailable", awgBuildTag)
	}

	version := coreVersionFromVersionOutput(versionOutput)
	if version == "" {
		return true, "" // формат неизвестен — не деградируем по догадке
	}
	// Версия старше 1.14.0 — поля появиться не могли; новее — есть в апстриме
	// форка вне зависимости от номера lx-релиза.
	switch CompareVersions(strings.SplitN(version, "-", 2)[0], "1.14.0") {
	case 1:
		return true, ""
	case -1:
		return false, awg3UnsupportedReason(version)
	}
	// Ровно 1.14.0: решает номер релиза форка в суффиксе.
	lx := awg3LxReleaseRegex.FindStringSubmatch(version)
	if lx == nil {
		return true, "" // не форк или неожиданный суффикс — не гадаем
	}
	release, err := strconv.Atoi(lx[1])
	if err != nil || release >= awg3MinLxRelease {
		return true, ""
	}
	return false, awg3UnsupportedReason(version)
}

// awg3UnsupportedReason — текст причины для UI и отчёта сборки. Версия в нём
// обязательна: пользователю нужно понять, какое ядро стоит и до чего его
// обновлять.
func awg3UnsupportedReason(version string) string {
	if version == "" {
		version = "of unknown version"
	}
	return fmt.Sprintf("sing-box core %s does not support AmneziaWG 3.x fields (need %s or newer) — update the core in Core Dashboard",
		version, awg3MinCoreVersion)
}

// cronetLibName — platform-specific companion library filename the cronet
// purego loader looks for.
func cronetLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "libcronet.dll"
	case "darwin":
		return "libcronet.dylib"
	default:
		return "libcronet.so"
	}
}

// cronetLibAvailable mirrors the search order of cronet-go's purego loader:
// the binary's directory, then PATH (Windows) or LD_LIBRARY_PATH /
// DYLD_LIBRARY_PATH + /usr/local/lib + /usr/lib (unix).
func cronetLibAvailable(singboxPath string) bool {
	libName := cronetLibName()
	dirs := []string{filepath.Dir(singboxPath)}
	switch runtime.GOOS {
	case "windows":
		dirs = append(dirs, filepath.SplitList(os.Getenv("PATH"))...)
	default:
		dirs = append(dirs, filepath.SplitList(os.Getenv("LD_LIBRARY_PATH"))...)
		if runtime.GOOS == "darwin" {
			dirs = append(dirs, filepath.SplitList(os.Getenv("DYLD_LIBRARY_PATH"))...)
		}
		dirs = append(dirs, "/usr/local/lib", "/usr/lib")
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, libName)); err == nil {
			return true
		}
	}
	return false
}

// SPEC 089 ядра: поле `tls.reality.key_share` знает только ядро
// 1.14.1-lx.4 и новее. На ядре постарше это НЕИЗВЕСТНЫЙ ключ, а неизвестный
// ключ ядро отвергает отказом ВСЕГО конфига — то есть без гейта один узел с
// key_share оставил бы пользователя вообще без VPN. Здесь, в отличие от
// tailscale/AWG3, узел выбрасывать не надо: REALITY прекрасно работает и без
// поля (обмен ключами берётся из uTLS-отпечатка), поэтому гейт ПОЛЕВОЙ —
// снимается одно поле.

// keyShareSupportVerdict — кэш вердикта по (mtime, size) бинаря ядра.
type keyShareSupportVerdict struct {
	binMtime  time.Time
	binSize   int64
	supported bool
	reason    string
}

// keyShareMinCoreVersion — граница строкой, для текста причины и сравнения
// базовых версий.
const keyShareMinCoreVersion = "1.14.1-lx.4"

// keyShareMinLxRelease — минимальный номер релиза форка в суффиксе `-lx.N`
// поверх базовой 1.14.1.
const keyShareMinLxRelease = 4

// keyShareMinBaseVersion — базовая версия, начиная с которой смотрим суффикс.
const keyShareMinBaseVersion = "1.14.1"

// CoreSupportsRealityKeyShare reports whether the installed sing-box core
// understands tls.reality.key_share, with a human-readable reason when it
// can't.
func (ac *AppController) CoreSupportsRealityKeyShare() (bool, string) {
	if ac == nil || ac.FileService == nil {
		return true, ""
	}
	singboxPath := ac.FileService.SingboxPath
	if resolved, err := exec.LookPath(singboxPath); err == nil {
		singboxPath = resolved
	}
	st, err := os.Stat(singboxPath)
	if err != nil {
		return true, "" // ядра нет — пробовать нечего
	}

	ac.keyShareSupportCacheMu.Lock()
	defer ac.keyShareSupportCacheMu.Unlock()
	if c := ac.keyShareSupportCache; c != nil && c.binMtime.Equal(st.ModTime()) && c.binSize == st.Size() {
		return c.supported, c.reason
	}

	supported, reason := probeKeyShareSupport(singboxPath)
	ac.keyShareSupportCache = &keyShareSupportVerdict{
		binMtime:  st.ModTime(),
		binSize:   st.Size(),
		supported: supported,
		reason:    reason,
	}
	if !supported {
		debuglog.WarnLog("CoreSupportsRealityKeyShare: %s", reason)
	}
	return supported, reason
}

// probeKeyShareSupport runs `sing-box version` and derives the verdict.
func probeKeyShareSupport(singboxPath string) (bool, string) {
	cmd := exec.Command(singboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("probeKeyShareSupport: sing-box version failed: %v", err)
		return true, ""
	}
	return keyShareVerdictFromVersionOutput(string(output))
}

// keyShareVerdictFromVersionOutput — чистая часть пробы, проверяемая тестом.
//
// Тега сборки у поля нет (оно в option/tls.go, а не за build-тегом), поэтому
// вердикт строится только по версии. Политика та же, что у соседей:
// деградируем только по положительному свидетельству — неразобранная версия
// значит «умеет», а последним рубежом остаётся `sing-box check`.
func keyShareVerdictFromVersionOutput(versionOutput string) (bool, string) {
	version := coreVersionFromVersionOutput(versionOutput)
	if version == "" {
		return true, "" // формат неизвестен — не деградируем по догадке
	}
	base := strings.SplitN(version, "-", 2)[0]
	switch CompareVersions(base, keyShareMinBaseVersion) {
	case 1:
		return true, "" // 1.14.2+ — поле уже в апстриме форка
	case -1:
		return false, keyShareUnsupportedReason(version)
	}
	// Ровно 1.14.1: решает номер релиза форка в суффиксе.
	lx := awg3LxReleaseRegex.FindStringSubmatch(version)
	if lx == nil {
		return true, "" // не форк или неожиданный суффикс — не гадаем
	}
	release, err := strconv.Atoi(lx[1])
	if err != nil || release >= keyShareMinLxRelease {
		return true, ""
	}
	return false, keyShareUnsupportedReason(version)
}

// keyShareUnsupportedReason — текст причины для лога и отчёта сборки.
func keyShareUnsupportedReason(version string) string {
	if version == "" {
		version = "of unknown version"
	}
	return fmt.Sprintf("sing-box core %s does not support tls.reality.key_share (need %s or newer) — the field is omitted, REALITY nodes keep working",
		version, keyShareMinCoreVersion)
}
