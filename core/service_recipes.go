package core

import (
	"net"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/core/services"
)

// Рецепты обслуживания службы демона `sing-box lxd` (SPEC 161): команды
// status / restart / лог / last-good / замена ядра / сопряжение под
// init-систему машины, ssh-обёртка и классификатор ошибки связи с демоном.
//
// Чистые функции без AppController, сети и build-тегов: окно Service
// раскладывает готовые ServiceStep и для локальной службы, и для удалённой
// машины, а удалённые машины обслуживаются с любой платформы лаунчера.
//
// Рецепты не пересобирают командную строку службы (на конкретном роутере
// init-скрипт может нести свои ключи и окружение) — только имя службы и
// restart/stop/start. Таблица команд продублирована в docs/TROUBLESHOOTING
// (раздел Daemon): правишь здесь — правь и там.

// ServiceName — имя службы демона во всех init-системах (procd, systemd,
// SCM); зеркалит форк (lxd/service_*.go).
const ServiceName = "sing-box-lxd"

// ServiceLaunchdLabel — метка launchd-службы демона на macOS; зеркалит
// lxd/service_darwin.go форка.
const ServiceLaunchdLabel = "com.leadaxe.sing-box-lxd"

// ServiceInit — init-система, которой управляется служба демона.
type ServiceInit string

const (
	ServiceInitProcd   ServiceInit = "procd"
	ServiceInitSystemd ServiceInit = "systemd"
	ServiceInitLaunchd ServiceInit = "launchd"
	ServiceInitSCM     ServiceInit = "scm"
)

// Posix — команды для POSIX-шелла (всё, кроме SCM/PowerShell).
func (i ServiceInit) Posix() bool { return i != ServiceInitSCM }

// ServicePlatform — платформа машины и её init-система.
type ServicePlatform struct {
	GOOS, GOARCH string
	Init         ServiceInit
}

// DefaultServiceInit — init-система по платформе: Linux на arm/arm64/mips —
// роутер на OpenWrt (procd), прочий Linux — systemd; macOS — launchd,
// Windows — SCM.
func DefaultServiceInit(goos, goarch string) ServiceInit {
	switch goos {
	case "darwin":
		return ServiceInitLaunchd
	case "windows":
		return ServiceInitSCM
	}
	switch goarch {
	case "arm", "arm64", "mips", "mipsle":
		return ServiceInitProcd
	}
	return ServiceInitSystemd
}

// NormalizeInitChoice — init-система с учётом выбора, сохранённого в записи
// машины (services.RemoteDaemon.InitSystem). Выбор имеет смысл только для
// Linux; пусто или неизвестное значение — DefaultServiceInit.
func NormalizeInitChoice(stored string, goos, goarch string) ServiceInit {
	if goos == "linux" {
		switch ServiceInit(strings.TrimSpace(stored)) {
		case ServiceInitProcd:
			return ServiceInitProcd
		case ServiceInitSystemd:
			return ServiceInitSystemd
		}
	}
	return DefaultServiceInit(goos, goarch)
}

// ServicePath — путь или адрес службы. Default — значение не сообщено
// демоном, а взято из платформенного дефолта (UI подписывает «default»).
type ServicePath struct {
	Value   string
	Default bool
}

// ServicePaths — где что лежит у службы на машине.
type ServicePaths struct {
	Executable  ServicePath // бинарь, из которого работает демон
	StateDir    ServicePath // state-dir: daemon.json, last_good.json, clients.json
	LogPath     ServicePath // лог демона (lxd.log)
	ServiceFile ServicePath // init-скрипт / unit / plist / имя у SCM
	Listen      ServicePath // адрес управляющего канала
	TLS         *bool       // nil — не сообщено
	// Только локально (заполняет локальный источник окна): root-owned копия
	// ядра службы и сайдкар установки.
	ServiceBinary ServicePath
	InstallRecord ServicePath
}

// Платформенные пути службы по умолчанию (гайд форка lxd-daemon §7, §7a, §8).
const (
	procdInitScript   = "/etc/init.d/" + ServiceName
	systemdUnitPath   = "/etc/systemd/system/" + ServiceName + ".service"
	launchdPlistPath  = "/Library/LaunchDaemons/" + ServiceLaunchdLabel + ".plist"
	launchdSupportDir = "/Library/Application Support/" + ServiceName
	scmDataDir        = `C:\ProgramData\` + ServiceName
	// serviceDefaultPort — порт управляющего канала, который install
	// выбирает первым; адрес без паспорта неизвестен, порт — почти всегда он.
	serviceDefaultPort = "19091"
	// coreUploadPath — куда шаг загрузки кладёт новое ядро на машине.
	coreUploadPath = "/tmp/sing-box.new"
	// scmRestartCommand — перезапуск службы Windows (PowerShell). Не пара
	// sc.exe stop + start: stop асинхронен, и немедленный start падает с
	// 1056 «already running». Её же исполняет DaemonRestartService.
	scmRestartCommand = "Restart-Service -Name " + ServiceName + " -Force"
)

// DefaultServicePaths — платформенные дефолты (все с Default=true).
// Локальный источник окна подменяет launchd/scm-значения точными из
// core/daemon_*_<os>.go.
func DefaultServicePaths(p ServicePlatform) ServicePaths {
	def := func(v string) ServicePath { return ServicePath{Value: v, Default: true} }
	switch p.Init {
	case ServiceInitProcd:
		return ServicePaths{
			Executable: def("/usr/bin/sing-box"), StateDir: def("/etc/sing-box-lxd/state"),
			LogPath: def("/tmp/lxd.log"), ServiceFile: def(procdInitScript), Listen: def(":" + serviceDefaultPort),
		}
	case ServiceInitLaunchd:
		return ServicePaths{
			Executable: def("/Library/PrivilegedHelperTools/" + ServiceName), StateDir: def(launchdSupportDir + "/state"),
			LogPath: def(launchdSupportDir + "/lxd.log"), ServiceFile: def(launchdPlistPath), Listen: def("127.0.0.1:" + serviceDefaultPort),
		}
	case ServiceInitSCM:
		return ServicePaths{
			Executable: def(`C:\Program Files\` + ServiceName + `\` + ServiceName + ".exe"), StateDir: def(scmDataDir + `\state`),
			LogPath: def(scmDataDir + `\logs\lxd.log`), ServiceFile: def(ServiceName), Listen: def("127.0.0.1:" + serviceDefaultPort),
		}
	}
	return ServicePaths{
		Executable: def("/usr/local/bin/sing-box"), StateDir: def("/var/lib/sing-box-lxd/state"),
		LogPath: def("/var/lib/sing-box-lxd/lxd.log"), ServiceFile: def(systemdUnitPath), Listen: def(":" + serviceDefaultPort),
	}
}

// ServicePassport — что демон сообщил о себе (/admin/info) или его кэш.
type ServicePassport struct {
	Version, StateDir, Executable, LogPath, Listen string
	TLS                                            *bool
}

// RemoteServicePassport — паспорт машины из её записи в реестре: кэш
// паспорта и state_dir (он хранится отдельно, в RemoteDaemon.StateDir).
func RemoteServicePassport(d services.RemoteDaemon) ServicePassport {
	out := ServicePassport{StateDir: d.StateDir}
	if p := d.Passport; p != nil {
		out.Version, out.Executable, out.LogPath, out.Listen, out.TLS = p.Version, p.Executable, p.LogPath, p.Listen, p.TLS
	}
	return out
}

// MergeServicePaths — пути службы: непустое из паспорта (Default=false),
// иначе значение из def как есть (с его флагом Default).
func MergeServicePaths(reported ServicePassport, def ServicePaths) ServicePaths {
	out := def
	pick := func(dst *ServicePath, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = ServicePath{Value: v}
		}
	}
	pick(&out.Executable, reported.Executable)
	pick(&out.StateDir, reported.StateDir)
	pick(&out.LogPath, reported.LogPath)
	pick(&out.Listen, reported.Listen)
	if reported.TLS != nil {
		out.TLS = reported.TLS
	}
	return out
}

// Идентификаторы шагов (ServiceStep.ID, ключи ServiceRecipes.Steps).
const (
	ServiceStepStatus        = "status"
	ServiceStepRestart       = "restart"
	ServiceStepStart         = "start"
	ServiceStepStop          = "stop"
	ServiceStepBootstrap     = "bootstrap"
	ServiceStepLogTail       = "log_tail"
	ServiceStepLogFollow     = "log_follow"
	ServiceStepLastGood      = "last_good"
	ServiceStepShowConfig    = "show_daemon_json"
	ServiceStepPortOwner     = "port_owner"
	ServiceStepClientAdd     = "client_add"
	ServiceStepClientList    = "client_list"
	ServiceStepClientRemove  = "client_remove"
	ServiceStepCoreUpload    = "core_upload"
	ServiceStepCoreCheck     = "core_check"
	ServiceStepCoreSwap      = "core_swap"
	ServiceStepCoreRollback  = "core_rollback"
	ServiceStepScratchEnable = "scratch_enable"
	ServiceStepRemoveService = "remove_service"
	ServiceStepRemoveState   = "remove_state"
)

// ServiceStep — одна команда рецепта.
type ServiceStep struct {
	ID string
	// Command — ровно то, что копирует ⧉ (без ssh-обёртки).
	Command string
	// RunsLocally — команда выполняется на ЭТОМ компьютере (загрузка ядра
	// на машину): ▶ без ssh-обёртки.
	RunsLocally bool
	// NeedsRoot — команде нужен root на машине. Для ssh-пользователя не root
	// на Linux sudo уже добавлен в Command.
	NeedsRoot bool
	// UsesDefault — в команде есть путь, не сообщённый демоном (Default).
	UsesDefault bool
	// Interactive — команда работает на переднем плане (last-good): ▶ можно,
	// но подпись — про Ctrl-C и последующий start.
	Interactive bool
	// Placeholder — в команде плейсхолдер <…> (имя клиента для revoke, путь
	// ещё не скачанного ядра): только ⧉ и правка руками, ▶ не предлагать.
	Placeholder bool
}

// ServiceRecipeInput — всё, из чего строятся команды.
type ServiceRecipeInput struct {
	Platform ServicePlatform
	Paths    ServicePaths
	// SSH — ssh-цель удалённой машины: пользователь (не root на Linux —
	// sudo в командах с NeedsRoot) и цель шага загрузки ядра. Нулевое
	// значение — локальная служба.
	SSH services.SSHTarget
	// Running — версия ядра на машине (имя бэкапа при замене ядра).
	Running string
	// Uploaded — локальный путь скачанного ядра для шага загрузки; "" —
	// плейсхолдер.
	Uploaded string
	// Today — дата в имени бэкапа, когда версия неизвестна; нуль — сейчас.
	Today time.Time
}

// ServiceRecipes — шаги по ID и тексты «установки с нуля» (Linux).
type ServiceRecipes struct {
	Steps map[string]ServiceStep
	// ScratchScript — init-скрипт procd / unit systemd с путями машины;
	// ScratchPath — куда его положить. Пусто вне Linux.
	ScratchScript string
	ScratchPath   string
	// BackupPath — куда core_swap копирует текущее ядро (его же берёт
	// core_rollback). Пусто вне Linux.
	BackupPath string
}

// BuildServiceRecipes строит команды по init-системе машины (таблица — PLAN
// SPEC 161 §3). Шаги замены ядра, установки с нуля и удаления службы — только
// для Linux: на macOS и Windows служба ставится `lxd --service=install` на
// самой машине.
func BuildServiceRecipes(in ServiceRecipeInput) ServiceRecipes {
	b := recipeBuilder{in: in, out: ServiceRecipes{Steps: make(map[string]ServiceStep)}}
	switch in.Platform.Init {
	case ServiceInitLaunchd:
		b.launchd()
	case ServiceInitSCM:
		b.scm()
	case ServiceInitProcd, ServiceInitSystemd:
		b.linux()
	}
	return b.out
}

type recipeBuilder struct {
	in  ServiceRecipeInput
	out ServiceRecipes
}

// add кладёт шаг; uses — пути, попавшие в команду (для UsesDefault).
func (b *recipeBuilder) add(st ServiceStep, uses ...ServicePath) {
	for _, p := range uses {
		if p.Default {
			st.UsesDefault = true
		}
	}
	b.out.Steps[st.ID] = st
}

// port — порт управляющего канала из Listen; без паспорта — дефолтный.
func (b *recipeBuilder) port() string {
	if _, port, err := net.SplitHostPort(b.in.Paths.Listen.Value); err == nil && port != "" {
		return port
	}
	return serviceDefaultPort
}

func (b *recipeBuilder) launchd() {
	p := b.in.Paths
	exe, state, log := PosixQuote(p.Executable.Value), p.StateDir.Value, PosixQuote(p.LogPath.Value)
	target := "system/" + ServiceLaunchdLabel
	lastGood := "sudo " + exe + " lxd --state-dir " + PosixQuote(state) + " --config-force " + PosixQuote(state+"/last_good.json")
	b.add(ServiceStep{ID: ServiceStepStatus, Command: "launchctl print " + target})
	b.add(ServiceStep{ID: ServiceStepRestart, Command: "sudo launchctl kickstart -k " + target, NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepStart, Command: "sudo launchctl kickstart " + target, NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepStop, Command: "sudo launchctl bootout " + target, NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepBootstrap, Command: "sudo launchctl bootstrap system " + PosixQuote(p.ServiceFile.Value), NeedsRoot: true}, p.ServiceFile)
	b.add(ServiceStep{ID: ServiceStepLogTail, Command: "sudo tail -n 100 " + log, NeedsRoot: true}, p.LogPath)
	b.add(ServiceStep{ID: ServiceStepLogFollow, Command: "sudo tail -f " + log, NeedsRoot: true}, p.LogPath)
	// `;`, а не `&&`: bootout незагруженной службы — ошибка, а запуск
	// last-good нужен именно тогда.
	b.add(ServiceStep{ID: ServiceStepLastGood, Command: "sudo launchctl bootout " + target + " ; " + lastGood, NeedsRoot: true, Interactive: true}, p.Executable, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepShowConfig, Command: "sudo cat " + PosixQuote(state+"/daemon.json"), NeedsRoot: true}, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepPortOwner, Command: "sudo lsof -nP -i :" + b.port(), NeedsRoot: true}, p.Listen)
	b.clientSteps("sudo "+exe, PosixQuote(state))
}

func (b *recipeBuilder) scm() {
	p := b.in.Paths
	exe, state, log := psQuote(p.Executable.Value), p.StateDir.Value, psQuote(p.LogPath.Value)
	lastGood := "& " + exe + " lxd --state-dir " + psQuote(state) + " --config-force " + psQuote(state+`\last_good.json`)
	b.add(ServiceStep{ID: ServiceStepStatus, Command: "sc.exe query " + ServiceName})
	b.add(ServiceStep{ID: ServiceStepRestart, Command: scmRestartCommand, NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepStart, Command: "sc.exe start " + ServiceName, NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepBootstrap, Command: "sc.exe start " + ServiceName, NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepStop, Command: "sc.exe stop " + ServiceName, NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepLogTail, Command: "Get-Content -Tail 100 " + log, NeedsRoot: true}, p.LogPath)
	b.add(ServiceStep{ID: ServiceStepLogFollow, Command: "Get-Content -Wait -Tail 50 " + log, NeedsRoot: true}, p.LogPath)
	b.add(ServiceStep{ID: ServiceStepLastGood, Command: "sc.exe stop " + ServiceName + "; " + lastGood, NeedsRoot: true, Interactive: true}, p.Executable, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepShowConfig, Command: "Get-Content " + psQuote(state+`\daemon.json`), NeedsRoot: true}, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepPortOwner, Command: "netstat -ano | findstr :" + b.port()}, p.Listen)
	b.clientSteps("& "+exe, psQuote(state))
}

// clientSteps — client add / list / remove: run — «как запустить бинарь»
// (с sudo, через & PowerShell), state — state-dir в кавычках шелла.
func (b *recipeBuilder) clientSteps(run, state string) {
	p := b.in.Paths
	b.add(ServiceStep{ID: ServiceStepClientAdd, Command: run + " lxd client add --name singbox-launcher --state-dir " + state, NeedsRoot: true}, p.Executable, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepClientList, Command: run + " lxd client list --state-dir " + state, NeedsRoot: true}, p.Executable, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepClientRemove, Command: run + " lxd client remove <name> --state-dir " + state, NeedsRoot: true, Placeholder: true}, p.Executable, p.StateDir)
}

func (b *recipeBuilder) linux() {
	p := b.in.Paths
	procd := b.in.Platform.Init == ServiceInitProcd
	initCmd := func(action string) string {
		if procd {
			return procdInitScript + " " + action
		}
		return "systemctl " + action + " " + ServiceName
	}
	exeRaw, stateRaw := p.Executable.Value, p.StateDir.Value
	exe, state, log := PosixQuote(exeRaw), PosixQuote(stateRaw), PosixQuote(p.LogPath.Value)
	lastGoodFile := PosixQuote(stateRaw + "/last_good.json")

	status := initCmd("status")
	portOwner := "netstat -lnp | grep :" + b.port()
	if !procd {
		status += " --no-pager"
		portOwner = "ss -ltnp | grep :" + b.port()
	}
	b.add(ServiceStep{ID: ServiceStepStatus, Command: status})
	b.add(ServiceStep{ID: ServiceStepRestart, Command: initCmd("restart"), NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepStart, Command: initCmd("start"), NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepStop, Command: initCmd("stop"), NeedsRoot: true})
	b.add(ServiceStep{ID: ServiceStepLogTail, Command: "tail -n 100 " + log, NeedsRoot: true}, p.LogPath)
	b.add(ServiceStep{ID: ServiceStepLogFollow, Command: "tail -f " + log, NeedsRoot: true}, p.LogPath)
	b.add(ServiceStep{ID: ServiceStepLastGood, Command: initCmd("stop") + " && " + exe + " lxd --state-dir " + state + " --config-force " + lastGoodFile,
		NeedsRoot: true, Interactive: true}, p.Executable, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepShowConfig, Command: "cat " + PosixQuote(stateRaw+"/daemon.json"), NeedsRoot: true}, p.StateDir)
	b.add(ServiceStep{ID: ServiceStepPortOwner, Command: portOwner, NeedsRoot: true}, p.Listen)
	b.clientSteps(exe, state)

	// Замена ядра: загрузка потоком через ssh (на OpenWrt нет sftp, scp не
	// работает) → проверка новым бинарём → бэкап, stop, mv, start.
	upload := ServiceStep{ID: ServiceStepCoreUpload, RunsLocally: true}
	dest := sshDestination(b.in.SSH)
	if dest == "" {
		dest, upload.Placeholder = "<user@host>", true
	}
	src := "<downloaded sing-box>"
	if b.in.Uploaded != "" {
		src = PosixQuote(b.in.Uploaded)
	} else {
		upload.Placeholder = true
	}
	upload.Command = sshCommand(b.in.SSH, dest, false) + " " + singleQuote("cat > "+coreUploadPath) + " < " + src
	b.add(upload)
	b.add(ServiceStep{ID: ServiceStepCoreCheck, Command: "chmod +x " + coreUploadPath + " && sha256sum " + coreUploadPath + " && " +
		coreUploadPath + " version && " + coreUploadPath + " check -c " + lastGoodFile, NeedsRoot: true}, p.StateDir)
	backup := b.backupPath(procd, exeRaw)
	b.out.BackupPath = backup
	b.add(ServiceStep{ID: ServiceStepCoreSwap, Command: "cp " + exe + " " + PosixQuote(backup) + " && " + initCmd("stop") + " && mv " + coreUploadPath + " " + exe +
		" && chmod 755 " + exe + " && " + initCmd("start"), NeedsRoot: true}, p.Executable)
	b.add(ServiceStep{ID: ServiceStepCoreRollback, Command: "cp " + PosixQuote(backup) + " " + exe + " && " + initCmd("restart"), NeedsRoot: true}, p.Executable)

	// Установка с нуля и удаление службы.
	if procd {
		b.out.ScratchPath = procdInitScript
		b.out.ScratchScript = "#!/bin/sh /etc/rc.common\nSTART=95\nUSE_PROCD=1\n\nstart_service() {\n" +
			"    procd_open_instance\n" +
			"    procd_set_param command " + exe + " lxd --state-dir " + state + "\n" +
			"    procd_set_param respawn\n    procd_set_param stdout 1\n    procd_set_param stderr 1\n" +
			"    procd_close_instance\n}\n"
		b.add(ServiceStep{ID: ServiceStepScratchEnable, Command: "chmod +x " + procdInitScript + " && " + initCmd("enable") + " && " + initCmd("start"), NeedsRoot: true})
		b.add(ServiceStep{ID: ServiceStepRemoveService, Command: initCmd("disable") + " && " + initCmd("stop") + " && rm " + procdInitScript, NeedsRoot: true})
	} else {
		b.out.ScratchPath = systemdUnitPath
		b.out.ScratchScript = "[Unit]\nDescription=sing-box-lx daemon\nAfter=network-online.target\nWants=network-online.target\n\n" +
			"[Service]\nExecStart=" + exe + " lxd --state-dir " + state + "\nRestart=always\nRestartSec=2\n\n" +
			"[Install]\nWantedBy=multi-user.target\n"
		b.add(ServiceStep{ID: ServiceStepScratchEnable, Command: "systemctl daemon-reload && systemctl enable --now " + ServiceName, NeedsRoot: true})
		b.add(ServiceStep{ID: ServiceStepRemoveService, Command: "systemctl disable --now " + ServiceName + " && rm " + systemdUnitPath + " && systemctl daemon-reload", NeedsRoot: true})
	}
	b.add(ServiceStep{ID: ServiceStepRemoveState, Command: "rm -r " + state, NeedsRoot: true}, p.StateDir)

	if !b.in.SSH.IsRoot() {
		b.sudoAll()
	}
}

// backupPath — бэкап текущего ядра перед заменой: procd — в /root (overlay
// переживает перезагрузку, а /tmp нет), systemd — рядом с бинарём. Версия
// из имени — полная; неизвестна — дата.
func (b *recipeBuilder) backupPath(procd bool, exe string) string {
	tag := strings.TrimPrefix(strings.TrimSpace(b.in.Running), "v")
	if _, ok := parseCoreBuild(tag); !ok {
		today := b.in.Today
		if today.IsZero() {
			today = time.Now()
		}
		tag = today.Format("20060102")
	}
	if procd {
		return "/root/sing-box." + tag + ".bak"
	}
	return exe + "." + tag + ".bak"
}

// sudoAll — ssh-пользователь не root: командам с NeedsRoot нужен sudo.
// Составные команды — целиком под `sudo sh -c`, иначе sudo досталось бы
// только первой из них.
func (b *recipeBuilder) sudoAll() {
	for id, st := range b.out.Steps {
		if !st.NeedsRoot || st.RunsLocally {
			continue
		}
		if strings.ContainsAny(st.Command, ";|&<>") {
			st.Command = "sudo sh -c " + singleQuote(st.Command)
		} else {
			st.Command = "sudo " + st.Command
		}
		b.out.Steps[id] = st
	}
}

// WrapSSH — команда для выполнения remoteCmd на машине из Terminal этого
// компьютера: `ssh [-t] [-p N] user@host '<cmd>'`. tty — выделить терминал
// (нужно sudo для ввода пароля).
func WrapSSH(t services.SSHTarget, remoteCmd string, tty bool) string {
	return sshCommand(t, sshDestination(t), tty) + " " + singleQuote(remoteCmd)
}

func sshCommand(t services.SSHTarget, dest string, tty bool) string {
	var sb strings.Builder
	sb.WriteString("ssh")
	if tty {
		sb.WriteString(" -t")
	}
	if t.Port != 0 {
		sb.WriteString(" -p " + strconv.Itoa(t.Port))
	}
	sb.WriteString(" " + dest)
	return sb.String()
}

// sshDestination — user@host для командной строки ssh (порт — ключом -p;
// IPv6 ssh принимает без скобок).
func sshDestination(t services.SSHTarget) string {
	if t.Host == "" {
		return ""
	}
	if t.User != "" {
		return t.User + "@" + t.Host
	}
	return t.Host
}

// PosixQuote — s для POSIX-шелла: как есть, если в нём только
// [A-Za-z0-9_./=:@,+-] (так /tmp/lxd.log и --service=install остаются
// читаемыми), иначе в одинарных кавычках.
func PosixQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_./=:@,+-", r):
		default:
			return singleQuote(s)
		}
	}
	return s
}

// singleQuote — s в одинарных кавычках POSIX-шелла; кавычка внутри
// закрывает литерал, экранируется и открывает его снова.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// psQuote — строковый литерал PowerShell в одинарных кавычках; кавычка
// внутри удваивается.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// DaemonReachKind — что значит ошибка связи с демоном для пользователя.
type DaemonReachKind int

const (
	// ReachOK — ошибки нет.
	ReachOK DaemonReachKind = iota
	// ReachDown — демон не отвечает: служба остановлена, падает, не тот
	// адрес или машина недоступна.
	ReachDown
	// ReachCertChanged — сертификат демона не совпал с пином (служба
	// переустановлена, state-dir стёрт).
	ReachCertChanged
	// ReachNotTrusted — демон не доверяет клиенту (клиент отозван или не
	// сопряжён).
	ReachNotTrusted
	// ReachChannelMismatch — режим канала разошёлся: клиент TLS, а демон
	// plain, или наоборот.
	ReachChannelMismatch
	// ReachConnReset — демон рвёт соединение (EOF): часто тоже смена режима
	// канала, но по одной строке не доказать.
	ReachConnReset
	// ReachUnknown — прочее.
	ReachUnknown
)

// ClassifyDaemonReachError — вид ошибки связи по её тексту. Порядок проверок
// значим: строка dial-ошибки с «connection refused» — это Down, даже если
// дальше в ней встречается что-то ещё.
func ClassifyDaemonReachError(err string) DaemonReachKind {
	s := strings.TrimSpace(err)
	has := func(subs ...string) bool {
		for _, sub := range subs {
			if strings.Contains(s, sub) {
				return true
			}
		}
		return false
	}
	switch {
	case s == "":
		return ReachOK
	case has("first record does not look like a TLS handshake", "HTTP request to an HTTPS server"):
		return ReachChannelMismatch
	case has("connection refused", "no route to host", "i/o timeout", "no such host", "dial tcp",
		"deadline exceeded", "network is unreachable", "host is down"):
		return ReachDown
	case has("bad certificate", "unknown certificate authority", "403", "not paired"):
		return ReachNotTrusted
	case has("fingerprint", "certificate"):
		return ReachCertChanged
	case has("EOF", "connection reset"):
		return ReachConnReset
	}
	return ReachUnknown
}
