//go:build darwin

package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// Управление launchd-службой демона `sing-box lxd` (задача 057 форка):
// установка/удаление системного LaunchDaemon через существующий механизм
// привилегий (пароль один раз на операцию), генерация Bearer-секрета,
// авто-сопряжение с локальной службой и ручное сопряжение по приглашению.

const (
	// daemonLaunchdLabel зеркалит константу lxd/service_darwin.go форка.
	daemonLaunchdLabel = "com.leadaxe.sing-box-lxd"
	// daemonDefaultListen — адрес управляющего канала локальной службы.
	daemonDefaultListen = "127.0.0.1:9091"
	// daemonSecretFileName — файл Bearer-секрета (0600, в bin/daemon/).
	// Служба читает его через --secret-file; root читает файлы пользователя.
	daemonSecretFileName = "secret"

	daemonPrivilegedTimeout = 60 * time.Second

	// daemonSupportDir — каталог службы демона (root-owned, cwd демона = "/").
	// Зеркалит lxd/service_darwin.go форка (StandardOutPath рядом). Сюда
	// абсолютизируются относительные пути конфига, которые ядро ПИШЕТ
	// (cache_file), иначе они резолвятся от "/" в read-only корень.
	daemonSupportDir = "/Library/Application Support/sing-box-lxd"
)

func daemonSystemPlistPath() string {
	return filepath.Join("/Library/LaunchDaemons", daemonLaunchdLabel+".plist")
}

// daemonRuntimeDir — куда демон пишет рантайм-файлы (cache.db и т.п.).
// Сам support-каталог: он УЖЕ создаётся службой при установке (там лежит
// lxd.log), root в нём пишет — отдельный подкаталог не нужен, mkdir не
// требуется. state демона (last-good/ключи) лежит в подкаталоге state/,
// cache.db — рядом с логом, это разные вещи и не конфликтуют.
func daemonRuntimeDir() string {
	return daemonSupportDir
}

// prepareConfigForDaemon готовит config.json перед отправкой демону:
//  1. cache_file.path → абсолютный в каталоге демона (демон cwd="/", иначе
//     относительный путь уходит в read-only корень и валит старт);
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
func prepareConfigForDaemon(config []byte) ([]byte, error) {
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
			abs := filepath.Join(daemonRuntimeDir(), filepath.Base(pathStr))
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
	info, err := lxdclient.New(cfg).Status()
	if err != nil {
		debuglog.DebugLog("DaemonStatusSnapshot: status unavailable: %v", err)
		return status
	}
	status.Reachable = true
	status.CoreStatus = info.Status
	status.LastError = info.LastError
	status.InterruptedApply = info.InterruptedApply
	return status
}

// CoreSupportsLxd проверяет, собрано ли установленное ядро с сабкомандой lxd
// (тег with_lx_command; появился в релизах форка начиная с 1.14.0-lx.23).
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
	return strings.Contains(string(out), "--listen")
}

// InstallDaemonService устанавливает системный LaunchDaemon и сопрягает с
// ним лаунчер. Один запрос пароля (launchctl bootstrap требует root); дальше
// весь жизненный цикл ядра идёт без пароля через управляющий канал.
//
// Последовательность: секрет → файл секрета (0600) → привилегированный
// `sing-box lxd --service=install --tls` → ожидание listener'а → выпуск
// приглашения через operator-маршрут (loopback + Bearer — сертификат ещё не
// нужен) → enroll собственного сертификата → сохранение настроек.
func (ac *AppController) InstallDaemonService() error {
	if !ac.CoreSupportsLxd() {
		return fmt.Errorf("installed core has no daemon support (need sing-box-lx 1.14.0-lx.23 or newer)")
	}
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	identityDir := DaemonIdentityDir(ac.FileService.ExecDir)
	// 0700: каталог держит файл секрета службы и клиентскую пару.
	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		return fmt.Errorf("create daemon dir: %w", err)
	}

	st := locale.LoadSettings(binDir)
	secret := st.DaemonSecret
	if secret == "" {
		var err error
		secret, err = generateDaemonSecret()
		if err != nil {
			return err
		}
	}
	secretPath := filepath.Join(identityDir, daemonSecretFileName)
	if err := os.WriteFile(secretPath, []byte(secret+"\n"), 0o600); err != nil {
		return fmt.Errorf("write secret file: %w", err)
	}

	address := st.DaemonAddress
	if address == "" {
		address = daemonDefaultListen
	}

	installArgv := []string{
		ac.FileService.SingboxPath,
		"lxd", "--service=install", "--tls",
		"--listen", address,
		"--secret-file", secretPath,
	}
	debuglog.InfoLog("InstallDaemonService: %v", installArgv)
	output, err := platform.RunPrivilegedProgramAndWait(installArgv, daemonPrivilegedTimeout)
	if err != nil {
		return fmt.Errorf("service install: %w", err)
	}
	debuglog.InfoLog("InstallDaemonService: %s", strings.TrimSpace(output))

	// Сохраняем адрес и секрет сразу: сопряжение ниже строит клиента из
	// settings, а при частичном провале пользователь сможет повторить
	// только шаг сопряжения.
	st = locale.LoadSettings(binDir)
	st.DaemonAddress = address
	st.DaemonSecret = secret
	if err := locale.SaveSettings(binDir, st); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}

	return ac.pairWithLocalService(address, secret)
}

// pairWithLocalService — авто-сопряжение с локально установленной службой:
// лаунчер знает Bearer-секрет, поэтому может сам выпустить приглашение
// (operator-маршрут /admin/client-code доступен с loopback без сертификата).
func (ac *AppController) pairWithLocalService(address, secret string) error {
	// Bootstrap без пина отправляет Bearer-секрет по неаутентифицированному
	// TLS — это безопасно ТОЛЬКО на loopback (MITM невозможен без локального
	// root). Для не-loopback адреса отказываемся: пользователь должен сопрячь
	// удалённый демон вручную по приглашению (там пин приезжает с отпечатком).
	if !lxdclient.IsLoopbackAddr(address) {
		return fmt.Errorf("auto-pairing is only for a local daemon (%s is not loopback); for a remote one paste the invite manually", address)
	}
	// AllowUnpinnedTLS: отпечатка сервера ещё нет — он приедет в приглашении.
	operator := lxdclient.New(lxdclient.Config{Addr: address, Secret: secret, AllowUnpinnedTLS: true})

	// Служба только что стартовала — ждём listener до ~15 секунд.
	var invite string
	var err error
	deadline := time.Now().Add(15 * time.Second)
	for {
		invite, err = operator.MintInvite()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service did not respond to invite request: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return ac.PairDaemonWithInvite(invite, secret)
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

// UninstallDaemonService снимает системную службу. purge=true — полное
// удаление: демон стирает свой state-каталог (ключи, last-good, доверенных
// клиентов), лаунчер — локальное сопряжение; режим возвращается на classic.
func (ac *AppController) UninstallDaemonService(purge bool) error {
	if ac.BackendMode() == BackendDaemon && ac.RunningState.IsRunning() {
		return fmt.Errorf("stop the VPN before uninstalling the service")
	}
	uninstallArgv := []string{ac.FileService.SingboxPath, "lxd", "--service=uninstall"}
	if purge {
		uninstallArgv = append(uninstallArgv, "--purge")
	}
	output, err := platform.RunPrivilegedProgramAndWait(uninstallArgv, daemonPrivilegedTimeout)
	if err != nil {
		return fmt.Errorf("service uninstall: %w", err)
	}
	debuglog.InfoLog("UninstallDaemonService(purge=%v): %s", purge, strings.TrimSpace(output))

	if purge {
		if err := ac.UnpairDaemon(); err != nil {
			debuglog.WarnLog("UninstallDaemonService: unpair: %v", err)
		}
	}
	// Служба снята — daemon-режим больше не работоспособен.
	if ac.BackendMode() == BackendDaemon {
		if err := ac.SwitchBackendMode(BackendClassic); err != nil {
			debuglog.WarnLog("UninstallDaemonService: switch to classic: %v", err)
		}
	}
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	st.CoreBackendMode = string(BackendClassic)
	if err := locale.SaveSettings(binDir, st); err != nil {
		debuglog.WarnLog("UninstallDaemonService: save settings: %v", err)
	}
	return nil
}

// UnpairDaemon стирает локальное сопряжение: клиентскую пару, пин, секрет и
// адрес. Регистрация на стороне демона (если он жив) остаётся — её снимает
// `sing-box lxd client remove` или полное удаление службы.
func (ac *AppController) UnpairDaemon() error {
	if err := lxdclient.RemoveIdentity(DaemonIdentityDir(ac.FileService.ExecDir)); err != nil {
		return err
	}
	secretPath := filepath.Join(DaemonIdentityDir(ac.FileService.ExecDir), daemonSecretFileName)
	if err := os.Remove(secretPath); err != nil && !os.IsNotExist(err) {
		debuglog.WarnLog("UnpairDaemon: remove secret file: %v", err)
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
// KeepAlive поднимет процесс заново уже с новым бинарём). Требует одного
// ввода пароля; вызывается только когда служба реально установлена.
func (ac *AppController) restartDaemonServiceAfterCoreUpdate() {
	// Только в активном daemon-режиме: в classic-режиме служба (если её plist
	// остался от прошлой установки) не управляет нашим ядром, и внезапный
	// запрос пароля + kickstart чужого демона был бы неожиданным для юзера.
	if ac.BackendMode() != BackendDaemon {
		return
	}
	if _, err := os.Stat(daemonSystemPlistPath()); err != nil {
		return // служба не установлена — нечего перезапускать
	}
	debuglog.InfoLog("restartDaemonServiceAfterCoreUpdate: kickstarting %s", daemonLaunchdLabel)
	kickstartArgv := []string{"launchctl", "kickstart", "-k", "system/" + daemonLaunchdLabel}
	if out, err := platform.RunPrivilegedProgramAndWait(kickstartArgv, daemonPrivilegedTimeout); err != nil {
		debuglog.WarnLog("restartDaemonServiceAfterCoreUpdate: %v (%s) — демон продолжит работать на старой версии ядра до перезапуска службы", err, strings.TrimSpace(out))
	}
}

// --- Терминальный путь (оператор управляет установкой сам) ---------------
//
// Альтернатива автоматической установке: лаунчер готовит секрет-файл и
// открывает Terminal.app с готовой sudo-командой. Оператор видит весь вывод
// launchctl, вводит свой sudo-пароль, полностью контролирует процесс. Это
// надёжнее AuthorizationExecuteWithPrivileges (настоящий root, никаких
// euid-фокусов) и прозрачнее. Сопряжение после этого лаунчер делает сам
// кнопкой «Сопрячь с локальной службой» (пароль не нужен — loopback+секрет).

// DaemonInstallCommand собирает shell-команду установки службы для терминала
// и гарантирует наличие секрет-файла. Возвращает (команда, ошибка подготовки).
func (ac *AppController) DaemonInstallCommand() (string, error) {
	if !ac.CoreSupportsLxd() {
		return "", fmt.Errorf("installed core has no daemon support (need sing-box-lx 1.14.0-lx.23 or newer)")
	}
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	identityDir := DaemonIdentityDir(ac.FileService.ExecDir)
	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		return "", fmt.Errorf("create daemon dir: %w", err)
	}
	st := locale.LoadSettings(binDir)
	secret := st.DaemonSecret
	if secret == "" {
		var err error
		secret, err = generateDaemonSecret()
		if err != nil {
			return "", err
		}
	}
	secretPath := filepath.Join(identityDir, daemonSecretFileName)
	if err := os.WriteFile(secretPath, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write secret file: %w", err)
	}
	address := st.DaemonAddress
	if address == "" {
		address = daemonDefaultListen
	}
	// Сохраняем адрес/секрет заранее: после терминальной установки лаунчер
	// сопрягается по ним, зная секрет.
	st.DaemonAddress = address
	st.DaemonSecret = secret
	if err := locale.SaveSettings(binDir, st); err != nil {
		return "", fmt.Errorf("save settings: %w", err)
	}
	return fmt.Sprintf("sudo %s lxd --service=install --tls --listen %s --secret-file %s",
		shellQuote(ac.FileService.SingboxPath), shellQuote(address), shellQuote(secretPath)), nil
}

// PairWithLocalService — публичная обёртка сопряжения с локально
// установленной службой (для терминального пути: служба поставлена руками в
// терминале, лаунчер досопрягается сам по сохранённым адресу/секрету).
func (ac *AppController) PairWithLocalService() error {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	address := st.DaemonAddress
	if address == "" {
		address = daemonDefaultListen
	}
	if st.DaemonSecret == "" {
		return fmt.Errorf("no saved secret: prepare installation with the Terminal button first")
	}
	if err := ac.pairWithLocalService(address, st.DaemonSecret); err != nil {
		return err
	}
	ac.reloadDaemonBackendIfActive()
	return nil
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

func generateDaemonSecret() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
