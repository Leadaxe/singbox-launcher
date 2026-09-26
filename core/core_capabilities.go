package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// Возможности установленного ядра для узлового гейта сборки (SPEC 142
// волна 5, контракт 1.1.60).
//
// Какому протоколу, полю или форме значения какой тег сборки и какая версия
// ядра нужны, говорит реестр (`build_tag`/`min_core` с
// `on_core_unsupported`), а решает общий nodeflow.NodeCoreRefusal. Отсюда
// ему нужны только ФАКТЫ о бинаре: теги сборки из `sing-box version` и
// теги, которые в сборке есть, но возможности не дают. Прежде здесь жили
// три пробы по имени протокола (naive, tailscale, AmneziaWG 3.x) с тремя
// кэшами и тремя вердиктами.
//
// Политика прежняя: деградируем только по положительному свидетельству.
// Нет бинаря, `version` упал, нет строки `Tags:` — тегов «не знаем», и гейт
// по тегу не применяется; последним рубежом остаётся `sing-box check`.

// coreBuildTagsVerdict — кэш по (mtime, size) бинаря ядра: переустановка
// ядра в той же сессии пробует заново.
type coreBuildTagsVerdict struct {
	binMtime time.Time
	binSize  int64
	tags     []string
	issues   map[string]string
}

// naiveBuildTag — тег, дающий naive outbound. purego-сборке (with_purego)
// для него нужна ещё libcronet рядом с бинарём: это свойство БИНАРЯ, а не
// узла, поэтому живёт в пробе, а не в реестре.
const naiveBuildTag = "with_naive_outbound"

// CoreBuildTags — теги сборки установленного ядра (nil = неизвестны) и
// теги, которые возможности не дают, с причиной (config.CoreBuildTagsProbe).
func (ac *AppController) CoreBuildTags() ([]string, map[string]string) {
	if ac == nil || ac.FileService == nil {
		return nil, nil
	}
	singboxPath := ac.FileService.SingboxPath
	if resolved, err := exec.LookPath(singboxPath); err == nil {
		singboxPath = resolved
	}
	st, err := os.Stat(singboxPath)
	if err != nil {
		return nil, nil // ядра нет — пробовать нечего, check всё равно не запустится
	}

	ac.coreBuildTagsCacheMu.Lock()
	defer ac.coreBuildTagsCacheMu.Unlock()
	if c := ac.coreBuildTagsCache; c != nil && c.binMtime.Equal(st.ModTime()) && c.binSize == st.Size() {
		return c.tags, c.issues
	}

	tags, issues := probeCoreBuildTags(singboxPath)
	ac.coreBuildTagsCache = &coreBuildTagsVerdict{
		binMtime: st.ModTime(),
		binSize:  st.Size(),
		tags:     tags,
		issues:   issues,
	}
	for tag, reason := range issues {
		debuglog.WarnLog("CoreBuildTags: %s: %s", tag, reason)
	}
	return tags, issues
}

// probeCoreBuildTags запускает `sing-box version` и читает теги.
func probeCoreBuildTags(singboxPath string) ([]string, map[string]string) {
	cmd := exec.Command(singboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("probeCoreBuildTags: sing-box version failed: %v", err)
		return nil, nil
	}
	return buildTagsFromVersionOutput(string(output), cronetLibAvailable(singboxPath))
}

var versionTagsRegex = regexp.MustCompile(`(?m)^Tags:\s*(\S+)`)

// buildTagsFromVersionOutput — чистая часть пробы. libAvailable — результат
// cronetLibAvailable для того же бинаря.
func buildTagsFromVersionOutput(versionOutput string, libAvailable bool) ([]string, map[string]string) {
	m := versionTagsRegex.FindStringSubmatch(versionOutput)
	if m == nil {
		return nil, nil // формат неизвестен — не деградируем по догадке
	}
	tags := splitBuildTags(m[1])
	has := func(want string) bool {
		for _, t := range tags {
			if t == want {
				return true
			}
		}
		return false
	}
	var issues map[string]string
	// Статические (musl/CGO) сборки линкуют cronet внутрь; библиотека рядом
	// нужна только purego-сборке.
	if has(naiveBuildTag) && has("with_purego") && !libAvailable {
		issues = map[string]string{
			naiveBuildTag: fmt.Sprintf("sing-box core needs %s next to the binary — re-download the core to get it", cronetLibName()),
		}
	}
	return tags, issues
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

// SPEC 131 W2c: проба на поле `tls.reality.key_share` отсюда СНЯТА.
//
// Она была ПОЛЕВЫМ гейтом — снимала одно поле, узел оставляла живым, — и на
// каждое следующее поле с границей ядра пришлось бы заводить ещё одну такую
// же: кэш вердикта, разбор версии, хук в контроллер и ветку в эмиттере. Всё
// это теперь делает один табличный проход по реестру
// (core/config/node_build_gate.go), которому нужна ЛИШЬ версия ядра —
// граница поля записана в реестре (`min_core` в registry/tls.json), а не в
// коде.
//
// Узловые гейты (снимается УЗЕЛ) теперь тоже табличные: реестр несёт
// требование и код (`on_core_unsupported`), проба выше — только теги.

// coreVersionForBuildGate — версия установленного ядра для полевого гейта
// сборки (config.CoreVersionProbe).
//
// Пустая строка = «версия неизвестна»: гейт по версии тогда не применяется
// вовсе. Это та же политика, что была у снятых проб, — деградировать по
// догадке нельзя, и последним рубежом остаётся `sing-box check`.
func (ac *AppController) coreVersionForBuildGate() string {
	if ac == nil {
		return ""
	}
	version, err := ac.GetInstalledCoreVersion()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(version)
}
