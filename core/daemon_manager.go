//go:build darwin || (windows && !386)

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
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	daemonCoreUpdatedBodyText = "The daemon service still runs the previous core. Run this command in Terminal to install the new core into the service and restart it (it asks for your sudo password):"
	// Подсказка вместо команды install/copy, пока ядро лаунчера не умеет
	// root-owned копию (serviceCoreGate).
	// Кнопка ядра на вкладке Local: «Download v…», когда ядра нет, и
	// «Reinstall v…», когда стоит другая версия (core_dashboard_tab_status.go).
	daemonServiceCoreTooOldText  = "The launcher core (%s) is older than %s and cannot install a root-owned service. Update the core first: Local tab → Download/Reinstall v%s, then install or update the service."
	daemonServiceCoreUnknownText = "The launcher core (%s) is not a numbered sing-box-lx release, so the launcher cannot confirm it installs a root-owned service. Update the core first: Local tab → Download/Reinstall v%s, then install or update the service."
)

// Менеджер службы демона `sing-box lxd`, общая часть: снимок состояния для
// UI и Debug API, сопряжение, подготовка конфига к доставке демону, сборка
// команд службы и диалог после обновления ядра. Платформенное — в
// daemon_manager_<os>.go: определение службы у менеджера служб ОС, рендер
// команды для исполнения с правами и открытие терминала.
//
// Модель владения: демон полностью самодостаточен в своём state-dir —
// daemon.json (listen/tls/secret), ключи, доверенные клиенты, last-good.
// Лаунчер НЕ генерирует и НЕ хранит секрет демона; у него остаётся только
// собственная клиентская пара (bin/daemon/).

const (
	// daemonDefaultListen — отображаемый дефолт адреса управляющего канала
	// (install сканирует свободный порт с 19091; фактический адрес приезжает
	// в приглашении и сохраняется при сопряжении).
	daemonDefaultListen = "127.0.0.1:19091"
	// daemonLegacySecretFileName — файл Bearer-секрета старой модели (до
	// ревизии владения). Больше не создаётся; UnpairDaemon подчищает остатки.
	daemonLegacySecretFileName = "secret"
)

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
	// ServiceInstalled — определение системной службы есть (на macOS — plist).
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
	// Service — вердикт классификатора службы (SPEC 136 §4): что запускает
	// launchd, root-owned ли это копия, то ли в ней ядро, что у лаунчера, и
	// из того ли образа работает демон. Заменил сверку путей SPEC 135 §5.1.
	Service DaemonServiceCheck
}

// DaemonStatusSnapshot собирает состояние службы/сопряжения/демона.
// Сетевые вызовы — с REST-таймаутом клиента; зовите из горутины, не из UI.
func (ac *AppController) DaemonStatusSnapshot() DaemonUIStatus {
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	status := DaemonUIStatus{
		CoreSupportsLxd: ac.CoreSupportsLxd(),
		Address:         st.DaemonAddress,
	}
	if status.Address == "" {
		status.Address = daemonDefaultListen
	}
	status.Service = ac.daemonServiceCheck(nil, "")
	status.ServiceInstalled = status.Service.State != DaemonServiceNotInstalled
	status.Paired = st.DaemonServerFingerprint != "" && lxdclient.HasIdentity(DaemonIdentityDir(ac.FileService.Layout.Data))
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
		addDaemonProcessVerdict(&status.Service, passport, cfg.Addr)
	}
	if status.Service.NeedsInstall() || status.Service.NeedsBootstrap() || status.Service.State == DaemonServiceCoreTooOld {
		debuglog.DebugLog("DaemonStatusSnapshot: daemon service %s: %s", status.Service.State, status.Service.Detail)
	}
	return status
}

// daemonServiceCheck — полный вердикт службы: файлы (daemonServiceFileCheck),
// состояние у менеджера служб ОС — на macOS launchd (NotRunning, только
// когда файлы в порядке, compareDaemonServiceRunning) и, если
// передан паспорт работающего демона, процесс. Версия ядра лаунчера (её
// кладёт файловый шаг) — для гейта install, показа и запасного вердикта
// ProcessStale.
func (ac *AppController) daemonServiceCheck(passport *lxdclient.InfoData, addr string) DaemonServiceCheck {
	check := ac.daemonServiceFileCheck()
	if check.State == DaemonServiceOK {
		compareDaemonServiceRunning(&check)
	}
	if passport != nil {
		addDaemonProcessVerdict(&check, *passport, addr)
	}
	return check
}

// addDaemonProcessVerdict — сверка с паспортом только для демона на этой
// машине: паспорт чужого адреса о локальной службе ничего не говорит.
// ProcessStale лечится install — гейт по ядру лаунчера и здесь.
func addDaemonProcessVerdict(check *DaemonServiceCheck, passport lxdclient.InfoData, addr string) {
	if !lxdclient.IsLoopbackAddr(addr) {
		return
	}
	compareDaemonServiceProcess(check, passport, daemonServiceCorePath())
	gateServiceInstall(check)
}

// launcherCoreVersion — версия ядра лаунчера для гейта команд службы
// (кэш по идентичности файла); "" — ядра нет или версия не прочиталась.
func (ac *AppController) launcherCoreVersion() string {
	version, err := daemonCoreVersions.version(ac.FileService.SingboxPath)
	if err != nil {
		debuglog.DebugLog("daemon service: launcher core version: %v", err)
		return ""
	}
	return version
}

// DaemonServiceCoreHint — подсказка вместо команды install/copy, пока ядро
// лаунчера версии version не умеет root-owned копию: обновить ядро кнопкой
// на вкладке Local (закреплённая версия), затем установить службу. Без
// команды.
func DaemonServiceCoreHint(version string) string {
	if _, ok := parseCoreBuild(version); ok {
		return locale.Tf(daemonServiceCoreTooOldText, version, minCoreForRootOwnedService, constants.RequiredCoreVersion)
	}
	if version == "" {
		version = "?"
	}
	return locale.Tf(daemonServiceCoreUnknownText, version, constants.RequiredCoreVersion)
}

// DaemonUnsafeServiceNotice — условие модального предупреждения SPEC 136 §6:
// служба Unsafe и на этой версии лаунчера предупреждения ещё не было.
// Дёшево (plist и Lstat цепочки, без хэшей и сети; версия ядра лаунчера —
// из кэша) — зовётся на старте. servicePath — что запускает служба (или
// причина, если plist не разобрался); command — «Install or update
// service». Ядро лаунчера не умеет root-owned копию — command пуста, а
// coreHint — подсказка сначала обновить ядро.
func (ac *AppController) DaemonUnsafeServiceNotice() (servicePath, command, coreHint string, due bool) {
	check := inspectDaemonServiceDefinition(systemDaemonServiceLayout())
	if check.State != DaemonServiceUnsafe {
		return "", "", "", false
	}
	version := ac.launcherCoreVersion()
	command, err := daemonInstallCommandFor(ac.FileService.SingboxPath, version)
	if err != nil {
		coreHint = DaemonServiceCoreHint(version)
		debuglog.WarnLog("daemon service is unsafe: %s — %v", check.Detail, err)
	} else {
		debuglog.WarnLog("daemon service is unsafe: %s — run the Install or update service command", check.Detail)
	}
	if locale.LoadSettings(ac.FileService.Layout.Data.Bin()).DaemonUnsafeNoticeVersion == constants.AppVersion {
		return "", "", "", false
	}
	servicePath = check.ServicePath
	if servicePath == "" {
		servicePath = check.Detail
	}
	return servicePath, command, coreHint, true
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
	identity, err := lxdclient.LoadOrCreateIdentity(DaemonIdentityDir(ac.FileService.Layout.Data))
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
		registry := services.NewRemoteRegistry(ac.FileService.Layout.Data)
		entry, impErr := registry.ImportPairedDaemon(
			invite.Addr, invite.Addr, invite.ServerFingerprint, secret,
			DaemonIdentityDir(ac.FileService.Layout.Data))
		if impErr != nil {
			return fmt.Errorf("pair: register remote daemon: %w", impErr)
		}
		debuglog.InfoLog("PairDaemonWithInvite: %s is a REMOTE daemon — stored in the registry as %q (local daemon settings untouched)",
			invite.Addr, entry.Name)
		return nil
	}

	binDir := ac.FileService.Layout.Data.Bin()
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
	if err := lxdclient.RemoveIdentity(DaemonIdentityDir(ac.FileService.Layout.Data)); err != nil {
		return err
	}
	// Файл секрета старой модели (до ревизии владения): больше не создаётся,
	// но у ранних установок мог остаться — подчищаем.
	legacySecretPath := filepath.Join(DaemonIdentityDir(ac.FileService.Layout.Data), daemonLegacySecretFileName)
	if err := os.Remove(legacySecretPath); err != nil && !os.IsNotExist(err) {
		debuglog.WarnLog("UnpairDaemon: remove legacy secret file: %v", err)
	}
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	st.DaemonAddress = ""
	st.DaemonServerFingerprint = ""
	st.DaemonSecret = ""
	return locale.SaveSettings(binDir, st)
}

// SetDaemonAddress сохраняет откорректированный адрес управляющего канала и
// пересоздаёт активный daemon-backend.
func (ac *AppController) SetDaemonAddress(address string) error {
	binDir := ac.FileService.Layout.Data.Bin()
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
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	debuglog.InfoLog("followDaemonPlainChannel: daemon at %s dropped TLS; clearing the pinned fingerprint to follow", st.DaemonAddress)
	st.DaemonServerFingerprint = ""
	if err := locale.SaveSettings(binDir, st); err != nil {
		debuglog.WarnLog("followDaemonPlainChannel: save settings: %v", err)
		return
	}
	ac.reloadDaemonBackendIfActive()
}

// SetDaemonSecret сохраняет Bearer-секрет и пересоздаёт активный
// daemon-backend. Нужен только для plain-h2c демона (без TLS): там нет
// сопряжения, и секрет — весь канал аутентификации; для mTLS-демона
// сертификат — полный мандат, а секрет не используется.
func (ac *AppController) SetDaemonSecret(secret string) error {
	binDir := ac.FileService.Layout.Data.Bin()
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

// notifyDaemonServiceAfterCoreUpdate — после обновления ядра служба
// продолжает запускать свою root-owned копию (SPEC 136): до неё новое ядро
// доходит только командой install, которая обновляет копию и перезапускает
// службу. Перезапуск (kickstart) поднял бы ту же старую копию. Условие —
// plist есть, в любом движке: службу запускает launchd, и без лаунчера.
// Привилегированных вызовов нет — диалог с готовой sudo-командой.
func (ac *AppController) notifyDaemonServiceAfterCoreUpdate() {
	check := ac.daemonServiceFileCheck()
	command := daemonCoreUpdatedCommand(check, ac.FileService.SingboxPath)
	switch {
	case check.State == DaemonServiceNotInstalled:
		return
	case check.State == DaemonServiceOK:
		// Скачано то же ядро, что уже в копии: обновлять нечего.
		debuglog.InfoLog("notifyDaemonServiceAfterCoreUpdate: the service already runs this core")
		return
	case command == "":
		// CoreTooOld: скачанное ядро копию не умеет — команды нет.
		debuglog.WarnLog("notifyDaemonServiceAfterCoreUpdate: core updated; the daemon service is %s (%s)", check.State, check.Detail)
		return
	}
	debuglog.WarnLog("notifyDaemonServiceAfterCoreUpdate: core updated; the daemon service is %s (%s) until the install command runs",
		check.State, check.Detail)
	if !ac.hasUI() {
		return
	}
	// Диалог сам оборачивается в fyne.Do — зваться из горутины загрузчика можно.
	dialogs.ShowLinuxCapabilitiesRequired(ac.UIService.MainWindow,
		locale.T("Core updated — update the daemon service"),
		locale.T(daemonCoreUpdatedBodyText),
		command)
}

// daemonCoreUpdatedCommand — команда диалога после скачивания ядра; "" —
// диалога нет: службы нет, копия уже это ядро (OK) или ядро лаунчера не
// умеет root-owned копию (CoreTooOld).
func daemonCoreUpdatedCommand(check DaemonServiceCheck, launcherCore string) string {
	if !check.NeedsInstall() {
		return ""
	}
	command, err := daemonInstallCommandFor(launcherCore, check.LauncherVersion)
	if err != nil {
		return ""
	}
	return command
}

// DaemonBootstrapCommand — команда запуска установленной службы (состояние
// NotRunning, SPEC 136 §4, SPEC 141 §5.1): plist/определение службы и копия
// в порядке, переустанавливать нечего. macOS — `launchctl bootstrap`,
// Windows — `sc.exe start sing-box-lxd` (START у Authenticated Users нет).
func (ac *AppController) DaemonBootstrapCommand() (string, error) {
	return daemonBootstrapCommand(), nil
}

// daemonServiceFileCheck — вердикт по файлам без сети (SPEC 136 §4) для
// системной раскладки и ядра лаунчера.
func (ac *AppController) daemonServiceFileCheck() DaemonServiceCheck {
	return classifyDaemonServiceFiles(systemDaemonServiceLayout(), ac.FileService.SingboxPath,
		ac.launcherCoreVersion(), &daemonServiceHashes)
}

// daemonServiceBinaryFor — бинарь для Uninstall и `lxd client add`: копия
// службы, если plist указывает на неё и цепочка владения цела (она
// переживает удаление данных лаунчера и совпадает с работающей службой);
// иначе — ядро лаунчера, как до SPEC 136.
func daemonServiceBinaryFor(l daemonServiceLayout, launcherCore string) string {
	if inspectDaemonServiceDefinition(l).CopyUsable() {
		return l.CorePath
	}
	return launcherCore
}

// DaemonRepairCommand — sudo-команда пере-сопряжения: `lxd client add` сам
// находит state-dir установленной службы, берёт listen/секрет из её
// daemon.json и печатает свежее одноразовое приглашение — его пользователь
// вставляет в поле сопряжения. Бинарь — копия службы, если она безопасна.
func (ac *AppController) DaemonRepairCommand() string {
	return daemonServiceCommand(daemonServiceBinaryFor(systemDaemonServiceLayout(), ac.FileService.SingboxPath),
		"lxd", "client", "add", "--name", "singbox-launcher")
}

// DaemonInstallCommand — «Install or update service» (SPEC 136 §5): одна
// команда для первой установки, старого небезопасного plist и обновления
// после скачивания ядра. Бинарь — всегда ядро лаунчера: ядро lx.12+ копирует
// СЕБЯ в root-owned копию службы и переписывает plist на копию
// (идемпотентно по sha, daemon.json и клиенты сохраняются, служба
// перезапускается). Никаких параметров: install сам выбирает loopback-порт
// (19091+, либо адрес существующей установки), генерирует секрет, включает
// mTLS и печатает приглашение. Адрес лаунчер узнаёт из приглашения при
// сопряжении (PairDaemonWithInvite сохраняет invite.Addr). Ядро ниже
// minCoreForRootOwnedService (или неизвестной версии) — ошибка
// *serviceCoreTooOldError вместо команды: его install записал бы в plist
// файл пользователя.
func (ac *AppController) DaemonInstallCommand() (string, error) {
	return daemonInstallCommandFor(ac.FileService.SingboxPath, ac.launcherCoreVersion())
}

// daemonInstallCommandFor — команда install для ядра лаунчера launcherCore
// версии launcherVersion, через гейт serviceCoreGate.
func daemonInstallCommandFor(launcherCore, launcherVersion string) (string, error) {
	if err := serviceCoreGate(launcherVersion); err != nil {
		return "", err
	}
	return daemonServiceCommand(launcherCore, "lxd", "--service=install"), nil
}

// DaemonUninstallCommand собирает shell-команду удаления службы для терминала.
// purge=true — полное удаление данных демона. Бинарь — копия службы, если
// она безопасна (SPEC 136 §5), иначе ядро лаунчера. `--keep-copy` (lx.11):
// снимаются plist и launchd, root-owned копия и сайдкар остаются — её
// запускает classic-старт с TUN (SPEC 137).
func (ac *AppController) DaemonUninstallCommand(purge bool) string {
	return daemonUninstallCommandFor(daemonServiceBinaryFor(systemDaemonServiceLayout(), ac.FileService.SingboxPath), purge, true)
}

// daemonUninstallCommandFor — команда удаления службы. keepCopy=false —
// копия уходит вместе со службой (подсказка «Remove all data…»: всё,
// что поставил лаунчер, уходит с данными).
func daemonUninstallCommandFor(binary string, purge, keepCopy bool) string {
	args := []string{"lxd", "--service=uninstall"}
	if keepCopy {
		args = append(args, "--keep-copy")
	}
	if purge {
		args = append(args, "--purge")
	}
	return daemonServiceCommand(binary, args...)
}

// --- Операции службы с правами (SPEC 141 §5) ---------------------------
//
// Панель, диалоги ядра (TUN без прав, гейт classic, «Core updated») и Debug
// API зовут одни и те же операции. На Windows операция исполняется через
// runas (ShellExecuteExW, окно UAC) с ожиданием кода выхода, и итог
// возвращается в DaemonRunResult; на macOS команда открывается в Terminal
// (sudo), итог пользователь видит там (DaemonRunResult.InTerminal).
// Платформенный признак — DaemonOpsElevated. Операции блокируют до ответа
// UAC и выхода процесса (до daemonRunWaitTimeout) — звать из горутины, не
// из UI-потока; на время ожидания — строка DaemonRunWaitingText.

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	daemonRunWaitingText      = "Waiting for the administrator command…"
	daemonRunFailedText       = "The command failed (exit code %d). Run it in an elevated terminal to see its output:"
	daemonRunStillRunningText = "The administrator command is still running. Check the status again in a minute."
	daemonRunNoInviteText     = "The service is installed, but no invite was received. Pair it as a separate step:"
	daemonRunPairFailedText   = "The command succeeded, but pairing failed: %s"
	daemonWarnForeignDataText = "The service data folder was readable by another account before this install. Rotate the admin secret in daemon.json and pair again if this computer is shared."
)

// DaemonCommand — команда службы (SPEC 141 §5.1): исполняется argv
// {Binary, Args}; String — только показ и Copy.
type DaemonCommand struct {
	Binary string
	Args   []string
}

// String — платформенный рендер для показа и Copy (daemonServiceCommand):
// macOS — `sudo '<bin>' args`, Windows — PowerShell `& '<bin>' args`
// (' → ''). Пустая команда — "".
func (c DaemonCommand) String() string {
	if c.Binary == "" {
		return ""
	}
	return daemonServiceCommand(c.Binary, c.Args...)
}

// IsZero — команды нет (гейт версии ядра, служба не установлена).
func (c DaemonCommand) IsZero() bool { return c.Binary == "" }

// DaemonServiceOp — операция службы (SPEC 141 §5.1).
type DaemonServiceOp string

const (
	// DaemonOpInstall — «Install or update the service»: install с
	// --invite-out и сопряжение по файлу приглашения (Windows).
	DaemonOpInstall DaemonServiceOp = "install"
	// DaemonOpStart — NotRunning: Windows `sc.exe start sing-box-lxd`,
	// macOS `launchctl bootstrap`.
	DaemonOpStart DaemonServiceOp = "start"
	// DaemonOpFreshInvite — «Need a fresh invite»: `lxd client add` с
	// --invite-out и сопряжение (Windows).
	DaemonOpFreshInvite DaemonServiceOp = "fresh_invite"
	// DaemonOpUninstall — `--service=uninstall [--keep-copy] [--purge]`.
	DaemonOpUninstall DaemonServiceOp = "uninstall"
	// DaemonOpCopy — `--service=copy`: копия для classic с правами, службы
	// нет (SPEC 141 §8).
	DaemonOpCopy DaemonServiceOp = "copy"
)

// DaemonServiceWarning — предупреждение install/copy из сайдкара копии
// (`warnings[{code, text}]`, SPEC 141 §3 п. 3, §5.3).
type DaemonServiceWarning struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// daemonWarnStateDirForeign — каталог данных службы до install был
// читаем чужому SID (SPEC 141 §3 п. 2a).
const daemonWarnStateDirForeign = "state_dir_foreign_before_install"

// DisplayText — текст предупреждения для показа: известный код —
// локализованный текст SPEC 141 §5.3, прочие — text ядра как есть.
func (w DaemonServiceWarning) DisplayText() string {
	if w.Code == daemonWarnStateDirForeign {
		return locale.T(daemonWarnForeignDataText)
	}
	if w.Text != "" {
		return w.Text
	}
	return w.Code
}

// DaemonRunResult — итог операции службы (SPEC 141 §5.2–5.3).
type DaemonRunResult struct {
	Op DaemonServiceOp
	// Command — команда операции в виде для показа и Copy (без
	// --invite-out: её выполняют в консоли администратора и видят вывод).
	// При коде ≠ 0 и при ошибке запуска панель показывает её с Copy.
	Command DaemonCommand
	// InTerminal — macOS: команда открыта в Terminal, итог — там; прочие
	// поля пусты (кроме Err, если Terminal не открылся).
	InTerminal bool
	// CoreHint — команды нет: ядро лаунчера не умеет защищённую копию
	// (install, copy; DaemonServiceCoreHint). Command пуста.
	CoreHint string
	// Cancelled — пользователь отказал в UAC (platform.ErrElevationCancelled):
	// строка статуса, диалог остаётся открытым.
	Cancelled bool
	// Err — ошибка запуска (ShellExecuteExW) или, при коде 0, чтения
	// приглашения и сопряжения.
	Err error
	// Exited / ExitCode — процесс завершился, его код выхода.
	Exited   bool
	ExitCode int
	// TimedOut — daemonRunWaitTimeout прошёл, процесс ещё работает; он не
	// убивается.
	TimedOut bool
	// Paired — install / fresh invite: сопряжено по файлу --invite-out.
	Paired bool
	// Warnings — warnings сайдкара после install/copy (показать и в лог).
	Warnings []DaemonServiceWarning
	// StatusChecked / StatusCode — install с кодом 1: код `lxd
	// --service=status` ядра лаунчера (0 OK, 2 MISMATCH/UNSAFE, 3 NOT
	// INSTALLED, 4 COPY ONLY, 5 NOT RUNNING, 1 ошибка).
	StatusChecked bool
	StatusCode    int
	// NoInvite — install с кодом 1, status 0, служба есть, файла
	// приглашения нет: следующий шаг — DaemonOpFreshInvite (второе окно
	// UAC). FreshInvite — его команда для показа и Copy.
	NoInvite    bool
	FreshInvite DaemonCommand
	// Service — вердикт классификатора, пересчитанный после команды.
	Service DaemonServiceCheck
}

// Succeeded — команда отработала с кодом 0 и, для install и fresh invite,
// сопряжение по приглашению прошло.
func (r DaemonRunResult) Succeeded() bool {
	if r.InTerminal || r.CoreHint != "" || r.Cancelled || r.TimedOut || !r.Exited || r.ExitCode != 0 || r.Err != nil {
		return false
	}
	switch r.Op {
	case DaemonOpInstall, DaemonOpFreshInvite:
		return r.Paired
	}
	return true
}

// StatusText — строка статуса для панели и диалогов (SPEC 141 §5.3), с
// предупреждениями сайдкара отдельными строками. "" — показывать нечего
// (macOS: итог в Terminal). Команду для Copy (Command или FreshInvite)
// вызывающий показывает сам.
func (r DaemonRunResult) StatusText() string {
	var head string
	switch {
	case r.InTerminal:
		if r.Err != nil {
			return r.Err.Error()
		}
		return ""
	case r.CoreHint != "":
		return r.CoreHint
	case r.Cancelled:
		head = locale.T("The administrator prompt was cancelled.")
	case r.TimedOut:
		head = locale.T(daemonRunStillRunningText)
	case !r.Exited && r.Err != nil:
		head = locale.Tf("Could not run the command as administrator: %s", r.Err.Error())
	case r.NoInvite:
		head = locale.T(daemonRunNoInviteText)
	case r.Exited && r.ExitCode != 0:
		head = locale.Tf(daemonRunFailedText, r.ExitCode)
	case r.Err != nil:
		head = locale.Tf(daemonRunPairFailedText, r.Err.Error())
	case r.Paired:
		head = locale.T("Paired with the daemon.")
	case r.Op == DaemonOpStart:
		head = locale.T("The service was started.")
	case r.Op == DaemonOpUninstall:
		head = locale.T("The service was removed.")
	case r.Op == DaemonOpCopy:
		head = locale.T("The core copy was created or updated.")
	}
	lines := []string{head}
	for _, w := range r.Warnings {
		lines = append(lines, "⚠ "+w.DisplayText())
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// DaemonRunWaitingText — строка статуса на время операции (окно UAC и
// ожидание выхода процесса).
func DaemonRunWaitingText() string { return locale.T(daemonRunWaitingText) }
