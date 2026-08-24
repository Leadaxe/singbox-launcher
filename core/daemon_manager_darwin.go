//go:build darwin

package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	daemonKickstartBodyText = "The daemon service keeps the old core binary in memory until it restarts. Run this command in Terminal (it asks for your sudo password):"
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
// Модель владения: демон полностью самодостаточен в своём state-dir —
// daemon.json (listen/tls/secret), ключи, доверенные клиенты, last-good.
// Лаунчер НЕ генерирует и НЕ хранит секрет демона; у него остаётся только
// собственная клиентская пара (bin/daemon/).

const (
	// daemonLaunchdLabel зеркалит константу lxd/service_darwin.go форка.
	daemonLaunchdLabel = "com.leadaxe.sing-box-lxd"
	// daemonDefaultListen — отображаемый дефолт адреса управляющего канала
	// (install сканирует свободный порт с 19091; фактический адрес приезжает
	// в приглашении и сохраняется при сопряжении).
	daemonDefaultListen = "127.0.0.1:19091"
	// daemonLegacySecretFileName — файл Bearer-секрета старой модели (до
	// ревизии владения). Больше не создаётся; UnpairDaemon подчищает остатки.
	daemonLegacySecretFileName = "secret"

	// daemonFallbackRuntimeDir — каталог рантайм-файлов демона, используемый
	// ТОЛЬКО когда /admin/info недоступен (демон старой сборки). Обычный путь
	// — state_dir из паспорта демона (см. prepareConfigForDaemon caller).
	daemonFallbackRuntimeDir = "/Library/Application Support/sing-box-lxd"
)

func daemonSystemPlistPath() string {
	return filepath.Join("/Library/LaunchDaemons", daemonLaunchdLabel+".plist")
}

// prepareConfigForDaemon готовит config.json перед отправкой демону:
//  1. cache_file.path → абсолютный в runtimeDir — каталоге, которым владеет
//     демон (state_dir из /admin/info; демон cwd="/", иначе относительный
//     путь уходит в read-only корень и валит старт);
//  2. experimental.clash_api УДАЛЯЕТСЯ — в daemon-режиме управление и ноды
//     идут по gRPC (GetGroups/SelectOutbound/URLTestOutbound), а трафик — по
//     gRPC SubscribeConnections. Clash API демону не нужен вовсе: убираем его,
//     чтобы не занимать порт и не плодить второй управляющий канал. Classic
//     этой функции не проходит — там Clash остаётся (см. развилку
//     ProxyTransport: classic=Clash HTTP, daemon=gRPC).
//
// Конфиг — JSONC (с комментариями): стрипим их (jsonc.ToJSON), правим map,
// сериализуем чистым JSON (демон толерантен к обоим). Возвращает исходный
// конфиг без изменений, если править нечего.
func prepareConfigForDaemon(config []byte, runtimeDir string) ([]byte, error) {
	clean := jsonc.ToJSON(config)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(clean, &root); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	expRaw, ok := root["experimental"]
	if !ok {
		return config, nil // нет experimental — ни cache_file, ни clash_api
	}
	var exp map[string]json.RawMessage
	if err := json.Unmarshal(expRaw, &exp); err != nil {
		return nil, fmt.Errorf("parse experimental: %w", err)
	}
	changed := false

	// (1) cache_file.path → абсолютный.
	if cfRaw, ok := exp["cache_file"]; ok {
		var cf map[string]json.RawMessage
		if err := json.Unmarshal(cfRaw, &cf); err != nil {
			return nil, fmt.Errorf("parse cache_file: %w", err)
		}
		var pathStr string
		if p, ok := cf["path"]; ok {
			_ = json.Unmarshal(p, &pathStr)
		}
		if pathStr == "" {
			pathStr = "cache.db"
		}
		if !filepath.IsAbs(pathStr) {
			abs := filepath.Join(runtimeDir, filepath.Base(pathStr))
			cf["path"], _ = json.Marshal(abs)
			exp["cache_file"], _ = json.Marshal(cf)
			changed = true
			debuglog.InfoLog("daemon: cache_file path %q → %q", pathStr, abs)
		}
	}

	// (2) clash_api — удаляем целиком (daemon работает по gRPC).
	if _, ok := exp["clash_api"]; ok {
		delete(exp, "clash_api")
		changed = true
		debuglog.InfoLog("daemon: removed clash_api (daemon uses gRPC)")
	}

	if !changed {
		return config, nil
	}
	root["experimental"], _ = json.Marshal(exp)
	out, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return out, nil
}

// DaemonUIStatus — снимок состояния демона для секции настроек.
type DaemonUIStatus struct {
	// CoreSupportsLxd — установленное ядро имеет сабкоманду lxd.
	CoreSupportsLxd bool
	// ServiceInstalled — plist системной службы существует.
	ServiceInstalled bool
	// Paired — есть клиентская пара и пин сервера (mTLS-сопряжение).
	Paired bool
	// Address — настроенный адрес управляющего канала.
	Address string
	// Reachable — админ-плоскость ответила на /admin/status.
	Reachable bool
	// CoreStatus — idle | started | fatal (пусто, если недостижим).
	CoreStatus string
	// LastError — last_error из статуса демона.
	LastError string
	// InterruptedApply — демон обнаружил, что предыдущий apply был прерван
	// смертью процесса (загрузился last-good). Информационный сигнал.
	InterruptedApply bool
	// DaemonVersion/StateDir — паспорт демона (/admin/info); пусто, если
	// демон недостижим или собран до появления info-эндпоинта.
	DaemonVersion string
	StateDir      string
}

// DaemonStatusSnapshot собирает состояние службы/сопряжения/демона.
// Сетевые вызовы — с REST-таймаутом клиента; зовите из горутины, не из UI.
func (ac *AppController) DaemonStatusSnapshot() DaemonUIStatus {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	status := DaemonUIStatus{
		CoreSupportsLxd: ac.CoreSupportsLxd(),
		Address:         st.DaemonAddress,
	}
	if status.Address == "" {
		status.Address = daemonDefaultListen
	}
	if _, err := os.Stat(daemonSystemPlistPath()); err == nil {
		status.ServiceInstalled = true
	}
	status.Paired = st.DaemonServerFingerprint != "" && lxdclient.HasIdentity(DaemonIdentityDir(ac.FileService.ExecDir))
	if !status.Paired && st.DaemonAddress == "" {
		return status
	}
	cfg, err := DaemonConfigFromSettings(ac)
	if err != nil {
		return status
	}
	client := lxdclient.New(cfg)
	info, err := client.Status()
	if err != nil {
		debuglog.DebugLog("DaemonStatusSnapshot: status unavailable: %v", err)
		return status
	}
	status.Reachable = true
	status.CoreStatus = info.Status
	status.LastError = info.LastError
	status.InterruptedApply = info.InterruptedApply
	// Паспорт демона — best-effort: старый демон без /admin/info не делает
	// снапшот ошибочным, поля просто остаются пустыми.
	if passport, infoErr := client.Info(); infoErr == nil {
		status.DaemonVersion = passport.Version
		status.StateDir = passport.StateDir
	}
	return status
}

// CoreSupportsLxd проверяет, собрано ли установленное ядро с сабкомандой lxd
// (тег with_lx_command; присутствует в релизах форка как минимум с 1.14.0-lx.19 —
// проверено на darwin-arm64 для lx.19..lx.25-rc.1; релиза lx.23 не существует).
func (ac *AppController) CoreSupportsLxd() bool {
	singbox := ac.FileService.SingboxPath
	if _, err := os.Stat(singbox); err != nil {
		return false
	}
	cmd := exec.Command(singbox, "lxd", "--help")
	platform.PrepareCommand(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	// Маркер — --state-dir: фундаментальный флаг демона (его дом), пережил
	// безфлаговую ревизию, в отличие от --listen (удалён — connection-настройки
	// живут в daemon.json, и детект по нему сломался ровно об этот контракт).
	return strings.Contains(string(out), "--state-dir")
}

// PairDaemonWithInvite выполняет сопряжение по приглашению
// `адрес#отпечаток#код` (поле сопряжения в настройках, либо авто-путь
// установки). secret — Bearer-секрет демона (пусто, если не настроен).
//
// Замечание к адресу: в приглашении стоит listen-адрес демона; для
// удалённого демона с listen 0.0.0.0 пользователь правит адрес в поле
// настроек после сопряжения.
func (ac *AppController) PairDaemonWithInvite(inviteRaw, secret string) error {
	invite, err := lxdclient.ParseInvite(inviteRaw)
	if err != nil {
		return err
	}
	identity, err := lxdclient.LoadOrCreateIdentity(DaemonIdentityDir(ac.FileService.ExecDir))
	if err != nil {
		return err
	}
	enrollClient := lxdclient.New(lxdclient.Config{
		Addr:              invite.Addr,
		ServerFingerprint: invite.ServerFingerprint,
		Identity:          identity,
	})
	if err := enrollClient.Enroll(invite.Code, "singbox-launcher"); err != nil {
		return err
	}

	// SPEC 097: settings.json держит ОДНО подключение — своего демона. Пока
	// сюда же писалось сопряжение с чужой машиной, pair с роутером затирал
	// адрес и пин локального демона, и лаунчер терял с ним связь (движок
	// продолжал стучаться на роутер). Не-loopback сопряжение уходит в реестр
	// удалённых машин, локальные поля не трогаем.
	if !lxdclient.IsLoopbackAddr(invite.Addr) {
		registry := services.NewRemoteRegistry(ac.FileService.ExecDir)
		entry, impErr := registry.ImportPairedDaemon(
			invite.Addr, invite.Addr, invite.ServerFingerprint, secret,
			DaemonIdentityDir(ac.FileService.ExecDir))
		if impErr != nil {
			return fmt.Errorf("pair: register remote daemon: %w", impErr)
		}
		debuglog.InfoLog("PairDaemonWithInvite: %s is a REMOTE daemon — stored in the registry as %q (local daemon settings untouched)",
			invite.Addr, entry.Name)
		return nil
	}

	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	st.DaemonAddress = invite.Addr
	st.DaemonServerFingerprint = invite.ServerFingerprint
	st.DaemonSecret = secret
	if err := locale.SaveSettings(binDir, st); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	debuglog.InfoLog("PairDaemonWithInvite: enrolled at %s (server %s…)", invite.Addr, invite.ServerFingerprint[:12])

	// Если daemon-режим уже активен — пересоздаём backend с новым пином.
	ac.reloadDaemonBackendIfActive()
	return nil
}

// UnpairDaemon стирает локальное сопряжение: клиентскую пару, пин, секрет и
// адрес. Регистрация на стороне демона (если он жив) остаётся — её снимает
// `sing-box lxd client remove` или полное удаление службы.
func (ac *AppController) UnpairDaemon() error {
	if err := lxdclient.RemoveIdentity(DaemonIdentityDir(ac.FileService.ExecDir)); err != nil {
		return err
	}
	// Файл секрета старой модели (до ревизии владения): больше не создаётся,
	// но у ранних установок мог остаться — подчищаем.
	legacySecretPath := filepath.Join(DaemonIdentityDir(ac.FileService.ExecDir), daemonLegacySecretFileName)
	if err := os.Remove(legacySecretPath); err != nil && !os.IsNotExist(err) {
		debuglog.WarnLog("UnpairDaemon: remove legacy secret file: %v", err)
	}
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	st.DaemonAddress = ""
	st.DaemonServerFingerprint = ""
	st.DaemonSecret = ""
	return locale.SaveSettings(binDir, st)
}

// SetDaemonAddress сохраняет откорректированный адрес управляющего канала и
// пересоздаёт активный daemon-backend.
func (ac *AppController) SetDaemonAddress(address string) error {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	st.DaemonAddress = strings.TrimSpace(address)
	if err := locale.SaveSettings(binDir, st); err != nil {
		return err
	}
	ac.reloadDaemonBackendIfActive()
	return nil
}

// followDaemonPlainChannel — «лаунчер следует за демоном»: демон перешёл на
// plain-канал (tls:false в его daemon.json), и закреплённый пин стал ложью.
// Сбрасываем ТОЛЬКО пин (адрес, секрет и клиентская пара остаются) и
// пересоздаём backend — следующая попытка пойдёт по plain-HTTP. Вызывается
// исключительно для loopback-адресов: авто-даунгрейд по сети — это подарок
// MITM'у (downgrade-атака), там решение остаётся за пользователем.
func (ac *AppController) followDaemonPlainChannel() {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	debuglog.InfoLog("followDaemonPlainChannel: daemon at %s dropped TLS; clearing the pinned fingerprint to follow", st.DaemonAddress)
	st.DaemonServerFingerprint = ""
	if err := locale.SaveSettings(binDir, st); err != nil {
		debuglog.WarnLog("followDaemonPlainChannel: save settings: %v", err)
		return
	}
	ac.reloadDaemonBackendIfActive()
}

// DaemonShowSecretCommand — команда просмотра Bearer-секрета в daemon.json
// системной службы (для справки у поля секрета: секретом владеет демон,
// лаунчер его не хранит — подсмотреть можно только под sudo).
func (ac *AppController) DaemonShowSecretCommand() string {
	return "sudo grep '\"secret\"' " + shellQuote(daemonFallbackRuntimeDir+"/state/daemon.json")
}

// SetDaemonSecret сохраняет Bearer-секрет и пересоздаёт активный
// daemon-backend. Нужен только для plain-h2c демона (без TLS): там нет
// сопряжения, и секрет — весь канал аутентификации; для mTLS-демона
// сертификат — полный мандат, а секрет не используется.
func (ac *AppController) SetDaemonSecret(secret string) error {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	st.DaemonSecret = strings.TrimSpace(secret)
	if err := locale.SaveSettings(binDir, st); err != nil {
		return err
	}
	ac.reloadDaemonBackendIfActive()
	return nil
}

// reloadDaemonBackendIfActive пересоздаёт daemon-backend, если он активен —
// подключение должно подхватить новый адрес/пин/секрет.
func (ac *AppController) reloadDaemonBackendIfActive() {
	if ac.BackendMode() != BackendDaemon {
		return
	}
	b, err := newDaemonBackend(ac)
	if err != nil {
		debuglog.WarnLog("reloadDaemonBackendIfActive: %v", err)
		return
	}
	ac.setBackend(b)
}

// restartDaemonServiceAfterCoreUpdate перезапускает установленную службу
// демона после обновления бинаря ядра (launchd держит старый образ в памяти;
// KeepAlive поднимет процесс заново уже с новым бинарём). Привилегированных
// вызовов из лаунчера нет — показываем пользователю готовую sudo-команду
// (терминальная модель, как и все операции со службой). Только активный
// daemon-режим: в classic чужая служба нас не касается.
func (ac *AppController) notifyDaemonServiceAfterCoreUpdate() {
	if ac.BackendMode() != BackendDaemon {
		return
	}
	if _, err := os.Stat(daemonSystemPlistPath()); err != nil {
		return // служба не установлена — нечего перезапускать
	}
	debuglog.InfoLog("notifyDaemonServiceAfterCoreUpdate: core updated; daemon keeps the old binary until kickstart")
	if !ac.hasUI() {
		return
	}
	// Диалог сам оборачивается в fyne.Do — зваться из горутины загрузчика можно.
	dialogs.ShowLinuxCapabilitiesRequired(ac.UIService.MainWindow,
		locale.T("Core updated — restart the daemon service"),
		locale.T(daemonKickstartBodyText),
		ac.DaemonKickstartCommand())
}

// DaemonKickstartCommand — sudo-команда перезапуска установленной службы
// (после обновления бинаря ядра launchd держит старый образ в памяти).
func (ac *AppController) DaemonKickstartCommand() string {
	return "sudo launchctl kickstart -k system/" + daemonLaunchdLabel
}

// DaemonRepairCommand — sudo-команда пере-сопряжения: `lxd client add` сам
// находит state-dir установленной службы, берёт listen/секрет из её
// daemon.json и печатает свежее одноразовое приглашение — его пользователь
// вставляет в поле сопряжения.
func (ac *AppController) DaemonRepairCommand() string {
	return fmt.Sprintf("sudo %s lxd client add --name singbox-launcher",
		shellQuote(ac.FileService.SingboxPath))
}

// --- Терминальная модель (оператор выполняет все привилегированные шаги) ---
//
// Лаунчер открывает Terminal.app с готовой sudo-командой (или даёт её
// скопировать). Оператор видит весь вывод launchctl (включая напечатанное
// приглашение), вводит свой sudo-пароль, полностью контролирует процесс.
// Сопряжение после установки/пере-сопряжения: скопировать приглашение из
// вывода в поле сопряжения.

// DaemonInstallCommand собирает shell-команду установки службы для терминала.
// Никаких параметров: install сам выбирает свободный loopback-порт (19091+,
// либо сохраняет адрес существующей установки), сам генерирует секрет,
// принудительно включает mTLS — и печатает адрес, секрет, путь к daemon.json,
// команду перезапуска и приглашение. Адрес лаунчер узнаёт из приглашения при
// сопряжении (PairDaemonWithInvite сохраняет invite.Addr).
func (ac *AppController) DaemonInstallCommand() (string, error) {
	return fmt.Sprintf("sudo %s lxd --service=install",
		shellQuote(ac.FileService.SingboxPath)), nil
}

// DaemonUninstallCommand собирает shell-команду удаления службы для терминала.
// purge=true — полное удаление данных демона.
func (ac *AppController) DaemonUninstallCommand(purge bool) string {
	cmd := fmt.Sprintf("sudo %s lxd --service=uninstall", shellQuote(ac.FileService.SingboxPath))
	if purge {
		cmd += " --purge"
	}
	return cmd
}

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
	escaped := strings.ReplaceAll(command, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	script := fmt.Sprintf(`tell application "Terminal"
	do script "%s"
	activate
end tell`, escaped)
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not open Terminal: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	debuglog.InfoLog("OpenTerminalWithCommand: opened Terminal for: %s", command)
	return nil
}

// shellQuote заключает строку в одинарные кавычки для безопасной вставки в
// shell-команду (одинарная кавычка внутри → '\”). Пути к бинарю/секрету и
// адрес проходят через это перед показом/вставкой в терминал.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
