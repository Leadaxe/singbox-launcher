//go:build darwin

package core

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// Управление launchd-службой демона `sing-box lxd` (задача 057 форка,
// ревизия модели владения 2026-08-09): все привилегированные операции —
// ТОЛЬКО через терминал оператора. Лаунчер генерирует готовые sudo-команды
// (установка/удаление/пере-сопряжение/kickstart), открывает Terminal.app и
// принимает результат — приглашение сопряжения пользователь вставляет в поле
// из вывода команды. AuthorizationExecuteWithPrivileges для демона не
// используется вовсе (выпилен вместе с priv-exec-хелпером): свой sudo с
// полным выводом прозрачнее и не тянет euid-фокусы.
//
// Здесь — платформенная часть (общая — daemon_manager.go): метка и plist
// launchd, bootstrap/kickstart, рендер sudo-команды, Terminal.app.

const (
	// daemonLaunchdLabel зеркалит константу lxd/service_darwin.go форка.
	daemonLaunchdLabel = "com.leadaxe.sing-box-lxd"

	// daemonFallbackRuntimeDir — каталог рантайм-файлов демона, используемый
	// ТОЛЬКО когда /admin/info недоступен (демон старой сборки). Обычный путь
	// — state_dir из паспорта демона (см. prepareConfigForDaemon caller).
	daemonFallbackRuntimeDir = "/Library/Application Support/sing-box-lxd"
)

func daemonSystemPlistPath() string {
	return filepath.Join("/Library/LaunchDaemons", daemonLaunchdLabel+".plist")
}

// readPlistProgramPath достаёт ProgramArguments[0] из XML-plist launchd:
// первую <string> массива, идущего за <key>ProgramArguments</key>. Разбор
// минимальный — plist службы пишет сам `lxd --service=install`, бинарные
// plist там не встречаются.
func readPlistProgramPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	var (
		inKey    bool
		afterKey bool
		inArray  bool
		keyText  strings.Builder
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return "", errors.New("ProgramArguments not found")
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case inArray:
				if t.Name.Local != "string" {
					return "", fmt.Errorf("ProgramArguments[0] is <%s>, want <string>", t.Name.Local)
				}
				var value string
				if err := dec.DecodeElement(&value, &t); err != nil {
					return "", err
				}
				return value, nil
			case afterKey:
				if t.Name.Local != "array" {
					return "", fmt.Errorf("ProgramArguments is <%s>, want <array>", t.Name.Local)
				}
				inArray = true
			case t.Name.Local == "key":
				inKey = true
				keyText.Reset()
			}
		case xml.CharData:
			if inKey {
				keyText.Write(t)
			}
		case xml.EndElement:
			switch {
			case inKey && t.Name.Local == "key":
				inKey = false
				afterKey = strings.TrimSpace(keyText.String()) == "ProgramArguments"
			case inArray && t.Name.Local == "array":
				return "", errors.New("ProgramArguments is empty")
			}
		}
	}
}

// DaemonShowSecretCommand — команда просмотра Bearer-секрета в daemon.json
// системной службы (для справки у поля секрета: секретом владеет демон,
// лаунчер его не хранит — подсмотреть можно только под sudo).
func (ac *AppController) DaemonShowSecretCommand() string {
	return "sudo grep '\"secret\"' " + shellQuote(daemonFallbackRuntimeDir+"/state/daemon.json")
}

// daemonServiceCommand — единственное место сборки sudo-команд службы
// (платформенный рендер; аргументы собирает общий код): бинарь в одинарных
// кавычках (пробелы и апострофы в пути), аргументы — константы без
// спецсимволов.
func daemonServiceCommand(binary string, args ...string) string {
	return "sudo " + shellQuote(binary) + " " + strings.Join(args, " ")
}

// DaemonBootstrapCommand — sudo-команда загрузки установленной службы в
// launchd (состояние NotRunning, SPEC 136 §4): plist и копия в порядке,
// переустанавливать нечего.
func (ac *AppController) DaemonBootstrapCommand() (string, error) {
	return daemonBootstrapCommand(), nil
}

func daemonBootstrapCommand() string {
	return "sudo launchctl bootstrap system " + shellQuote(daemonSystemPlistPath())
}

// DaemonKickstartCommand — sudo-команда перезапуска установленной службы
// (после обновления бинаря ядра launchd держит старый образ в памяти).
func (ac *AppController) DaemonKickstartCommand() string {
	return "sudo launchctl kickstart -k system/" + daemonLaunchdLabel
}

// --- Терминальная модель (оператор выполняет все привилегированные шаги) ---
//
// Лаунчер открывает Terminal.app с готовой sudo-командой (или даёт её
// скопировать). Оператор видит весь вывод launchctl (включая напечатанное
// приглашение), вводит свой sudo-пароль, полностью контролирует процесс.
// Сопряжение после установки/пере-сопряжения: скопировать приглашение из
// вывода в поле сопряжения.

// OpenTerminalWithCommand открывает Terminal.app и выполняет команду в новом
// окне — оператор видит весь вывод и вводит свой sudo-пароль. macOS-only.
func (ac *AppController) OpenTerminalWithCommand(command string) error {
	// AppleScript: Terminal.app do script запускает команду. Экранируем
	// двойные кавычки и обратные слэши для строкового литерала AppleScript.
	//
	// Порядок важен: `do script` СНАЧАЛА (создаёт/переиспользует окно и
	// возвращает вкладку), `activate` ПОТОМ (выводит Terminal на передний
	// план). Если делать наоборот — activate поднимает уже открытое окно, а
	// do script без цели создаёт ЕЩЁ одно → два окна (баг, замеченный при
	// первой установке). Здесь do script сам решает, куда писать, и лишнего
	// окна не появляется.
	script := fmt.Sprintf(`tell application "Terminal"
	do script %s
	activate
end tell`, appleScriptString(command))
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not open Terminal: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	debuglog.InfoLog("OpenTerminalWithCommand: opened Terminal for: %s", command)
	return nil
}

// appleScriptString — строковый литерал AppleScript в кавычках: экранируются
// обратный слэш и двойная кавычка (порядок важен — слэш первым).
func appleScriptString(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}

// shellQuote заключает строку в одинарные кавычки для безопасной вставки в
// shell-команду (одинарная кавычка внутри → '\”). Пути к бинарю/секрету и
// адрес проходят через это перед показом/вставкой в терминал.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
