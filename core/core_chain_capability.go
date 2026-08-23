// File core_chain_capability.go — проба поддержки цепочек ядром (SPEC 110).
//
// Тип outbound'а `chain` живёт в форке за тегом сборки `with_lx_chain`
// (`include/lx_chain.go`). Ядро без него отвергает конфиг ЦЕЛИКОМ:
//
//	FATAL decode config: outbounds[2]: unknown outbound type: chain
//
// То есть одна настроенная цепочка оставила бы пользователя вообще без VPN,
// а не без одного маршрута. Отсюда проба: генератор спрашивает ДО эмиссии и
// деградирует запись, а не отдаёт ядру «пусть разберётся».
//
// Ядро у пользователей обновляется отдельно от лаунчера, и версии без
// `with_lx_chain` будут встречаться ещё долго — проба нужна постоянно, а не
// как временная мера до следующего релиза ядра.
//
// Вердикт консервативен ровно как у naive-пробы: нет бинаря, `version` не
// отработал, нет строки Tags — считаем, что поддержка есть. Деградировать на
// догадке нельзя: это отняло бы у пользователя рабочий маршрут; `sing-box
// check` остаётся последним рубежом для экзотических сборок.
package core

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// chainBuildTag — тег сборки ядра, дающий тип `chain`.
const chainBuildTag = "with_lx_chain"

// chainSupportVerdict — кэш вердикта, ключ — (mtime, size) бинаря ядра:
// переустановка ядра внутри сессии обязана перепроверяться.
type chainSupportVerdict struct {
	binMtime  time.Time
	binSize   int64
	supported bool
	reason    string
}

// CoreSupportsChain сообщает, умеет ли установленное ядро outbound'ы типа
// `chain`, и человекочитаемую причину, если нет.
func (ac *AppController) CoreSupportsChain() (bool, string) {
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

	ac.chainSupportCacheMu.Lock()
	defer ac.chainSupportCacheMu.Unlock()
	if c := ac.chainSupportCache; c != nil && c.binMtime.Equal(st.ModTime()) && c.binSize == st.Size() {
		return c.supported, c.reason
	}

	supported, reason := probeChainSupport(singboxPath)
	ac.chainSupportCache = &chainSupportVerdict{
		binMtime:  st.ModTime(),
		binSize:   st.Size(),
		supported: supported,
		reason:    reason,
	}
	if !supported {
		debuglog.WarnLog("CoreSupportsChain: %s", reason)
	}
	return supported, reason
}

// probeChainSupport запускает `sing-box version` и читает вердикт из тегов.
func probeChainSupport(singboxPath string) (bool, string) {
	cmd := exec.Command(singboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("probeChainSupport: sing-box version failed: %v", err)
		return true, ""
	}
	return chainVerdictFromVersionOutput(string(output))
}

// chainVerdictFromVersionOutput — чистая часть пробы, проверяемая тестом.
//
// В причину попадает версия ядра: пользователю, у которого цепочка не
// работает, нужно понять, какое именно ядро у него стоит и что оно должно
// уметь. «Ядро не поддерживает» без версии не даёт сделать следующий шаг.
func chainVerdictFromVersionOutput(versionOutput string) (bool, string) {
	m := versionTagsRegex.FindStringSubmatch(versionOutput)
	if m == nil {
		return true, "" // формат неизвестен — не деградируем по догадке
	}
	for _, t := range splitBuildTags(m[1]) {
		if t == chainBuildTag {
			return true, ""
		}
	}
	version := coreVersionFromVersionOutput(versionOutput)
	if version == "" {
		version = "неизвестной версии"
	}
	return false, "ядро " + version + " собрано без " + chainBuildTag +
		" — цепочки хопов недоступны, обновите ядро"
}

// splitBuildTags — список тегов из строки `Tags:` вывода `sing-box version`.
func splitBuildTags(tagLine string) []string {
	return strings.Split(tagLine, ",")
}

var coreVersionRegex = regexp.MustCompile(`(?m)^sing-box version\s+(\S+)`)

// coreVersionFromVersionOutput — версия из первой строки вывода. Пусто, если
// формат неожиданный: подставлять что-то вместо версии в текст ошибки хуже,
// чем сказать «неизвестной версии».
func coreVersionFromVersionOutput(versionOutput string) string {
	m := coreVersionRegex.FindStringSubmatch(versionOutput)
	if m == nil {
		return ""
	}
	return m[1]
}
