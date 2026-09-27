package core

import (
	"regexp"
	"strings"
)

// exit_reason.go — SPEC 143: классификация причины завершения ядра.
//
// До этого решение о перезапуске принималось только по трём булевым флагам
// (stoppedByUser / restartRequested / cleanExit), и детерминированная
// ошибка — несовместимое поле конфига, отсутствие прав, занятый порт —
// выглядела как обычное падение: лаунчер трижды поднимал ядро, трижды
// получал ту же ошибку и лишь потом показывал диалог. Пользователь видел
// «ещё пытаюсь подключиться» там, где нужно было конкретное действие.
//
// Здесь — чистая классификация по тексту, который ядро уже печатает в лог.
// Разбор намеренно узкий: узор должен называть именно нашу ошибку запуска,
// а не любое упоминание сети, иначе транзиентный сбой узла будет принят за
// детерминированный и авто-восстановление отключится зря.

// exitReason — почему ядро завершилось.
type exitReason int

const (
	// exitReasonUnknown — причина не распознана. Считается транзиентной:
	// повторять с ограничением безопаснее, чем не повторять вовсе.
	exitReasonUnknown exitReason = iota

	// exitReasonConfigInvalid — конфиг не принят ядром: неизвестное поле,
	// ошибка разбора, несовместимая версия. Повтор не изменит результат.
	exitReasonConfigInvalid

	// exitReasonPermission — не хватает прав: effective root потерян,
	// TUN не создался. Повтор без смены прав бесполезен.
	exitReasonPermission

	// exitReasonPortInUse — локальный порт занят. Повтор займёт тот же
	// порт и получит ту же ошибку, пока занявший его процесс жив.
	exitReasonPortInUse

	// exitReasonMissingResource — нет нужного локального файла/каталога.
	exitReasonMissingResource

	// exitReasonAPIUnavailable — API ядра не поднялся или отверг
	// аутентификацию.
	exitReasonAPIUnavailable

	// exitReasonTransient — временная причина: сеть, загрузка ресурса,
	// недоступный узел. Повтор уместен.
	exitReasonTransient
)

// Deterministic — повтор не может изменить результат.
func (r exitReason) Deterministic() bool {
	switch r {
	case exitReasonConfigInvalid, exitReasonPermission, exitReasonPortInUse,
		exitReasonMissingResource, exitReasonAPIUnavailable:
		return true
	}
	return false
}

// String — для лога и диагностики.
func (r exitReason) String() string {
	switch r {
	case exitReasonConfigInvalid:
		return "config invalid"
	case exitReasonPermission:
		return "permission denied / not effective root"
	case exitReasonPortInUse:
		return "local listener occupied"
	case exitReasonMissingResource:
		return "missing local resource"
	case exitReasonAPIUnavailable:
		return "API unavailable"
	case exitReasonTransient:
		return "transient"
	}
	return "unknown"
}

// Узоры причин. Все — по подстрокам в нижнем регистре.
//
// Порядок важен: сначала узоры, которые ядро печатает при разборе конфига,
// затем права, затем порт. Более специфичные — раньше более общих.
var (
	reUnknownField = regexp.MustCompile(`unknown field "?[a-z_]+"?`)
	reDecodeConfig = regexp.MustCompile(`decode config|json: cannot unmarshal|invalid character`)
	rePermission   = regexp.MustCompile(`operation not permitted|permission denied|not permitted`)
	rePortInUse    = regexp.MustCompile(`address already in use|bind: address already`)
	reMissingFile  = regexp.MustCompile(`no such file or directory|not found`)
	reAPIAuth      = regexp.MustCompile(`authentication failed|401 unauthorized|invalid secret`)
	reTransientNet = regexp.MustCompile(`no route to host|network is unreachable|i/o timeout|connection refused|temporary failure in name resolution|context deadline exceeded`)
)

// classifyExitText определяет причину завершения по тексту лога ядра.
//
// Пустой текст — exitReasonUnknown, а не «всё хорошо»: отсутствие лога не
// доказательство отсутствия ошибки.
func classifyExitText(text string) exitReason {
	if strings.TrimSpace(text) == "" {
		return exitReasonUnknown
	}
	low := strings.ToLower(text)

	switch {
	case reUnknownField.MatchString(low) || reDecodeConfig.MatchString(low):
		return exitReasonConfigInvalid
	case rePortInUse.MatchString(low):
		// Порт проверяем ДО прав: `bind: address already in use` не про права.
		return exitReasonPortInUse
	case rePermission.MatchString(low):
		return exitReasonPermission
	case reAPIAuth.MatchString(low):
		return exitReasonAPIUnavailable
	case reMissingFile.MatchString(low):
		return exitReasonMissingResource
	case reTransientNet.MatchString(low):
		return exitReasonTransient
	}
	return exitReasonUnknown
}

// lastLines — последние n непустых строк текста: причина падения обычно в
// хвосте, а не в начале.
func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// Длинные тексты локализации (ключ = английский текст, SPEC 111). Каждый
// называет конкретное действие пользователя, а не «произошла ошибка»:
// детерминированную причину повтор не исправит, её надо устранить.
const (
	exitReasonConfigInvalidText = "The core rejected its configuration, so restarting cannot help. The launcher has stopped auto-restart. Open the config, fix the reported field, save, then start again."
	exitReasonPermissionText    = "The core did not have the privileges it needs (TUN requires root). Restarting cannot help while the privileges are missing. Check that the root-owned core copy matches the current core, then start again."
	exitReasonPortInUseText     = "A local port the core needs is already in use, so restarting would fail the same way. The launcher has stopped auto-restart. Free the port (another proxy client may hold it) or change the port in the config, then start again."
	exitReasonMissingText       = "A local file or folder the core needs is missing. Restarting cannot help. Check the paths in the config and the launcher's data folder, then start again."
	exitReasonAPIText           = "The core's own API did not come up or rejected the launcher's credentials. Restarting cannot help. Check the Clash API settings (address and secret) and apply them."
)

// deterministicExitText — текст для пользователя по причине.
func deterministicExitText(r exitReason) string {
	switch r {
	case exitReasonConfigInvalid:
		return exitReasonConfigInvalidText
	case exitReasonPermission:
		return exitReasonPermissionText
	case exitReasonPortInUse:
		return exitReasonPortInUseText
	case exitReasonMissingResource:
		return exitReasonMissingText
	case exitReasonAPIUnavailable:
		return exitReasonAPIText
	}
	return ""
}
