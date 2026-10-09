package ui

import (
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
)

// Модель окна Service (SPEC 161): одно окно обслуживания демона для
// локальной службы и для каждой удалённой машины. Окно и вкладки не знают,
// чей это демон, — они раскладывают снапшот источника (serviceSource) и
// готовые рецепты core.BuildServiceRecipes. Локальный и удалённый случаи
// различаются только источником.
//
// Файл без build-тега: окно машины открывается на любой платформе лаунчера.
// Символы core с тегом (DaemonServiceCheck, daemonOps, DaemonOpsElevated) сюда
// не попадают — локальный источник (тегированный) переводит их в плоские поля
// localServiceExtras и готовые виджеты localServiceRows.

// serviceTab — вкладка окна Service.
type serviceTab int

const (
	// serviceTabAuto — выбрать по диагнозу (первая проблемная вкладка).
	serviceTabAuto serviceTab = iota
	serviceTabNotRunning
	serviceTabCore
	serviceTabPairing
	serviceTabReference
	// serviceTabUninstall — только локально (localServiceRows.Uninstall).
	serviceTabUninstall
)

// serviceLevel — вердикт для точки на ⚙ и строки предупреждения машины.
type serviceLevel int

const (
	serviceLevelOK serviceLevel = iota
	// serviceLevelCore — ядро старее требуемого (жёлтый).
	serviceLevelCore
	// serviceLevelDown — демон не отвечает (красный).
	serviceLevelDown
)

// serviceSnapshot — всё, что окно показывает, на один момент. Источник
// собирает его из своего кэша без сети, в UI-потоке.
type serviceSnapshot struct {
	// Loaded — у источника есть данные. Локальный источник до первого опроса
	// отдаёт Loaded=false: по пустому снапшоту окно не выбирает вкладку и не
	// рисует диагноз.
	Loaded bool
	// Title — имя машины или «This computer».
	Title string
	// Platform — ОС/архитектура и init-система машины.
	Platform core.ServicePlatform
	// InitChoice — показывать в шапке выбор init-системы (удалённый Linux).
	InitChoice bool
	// Connected — состояние демона проверено: удалённо — машина активна и
	// ответ (или отказ) Connect/heartbeat есть; локально — опрос прошёл.
	// false — окно рисует рецепты по кэшу паспорта, но диагноз не ставит.
	Connected bool
	// Reachable — демон отвечает.
	Reachable bool
	// Err — текст ошибки связи (пусто при Reachable).
	Err string
	// DownSince — с какого момента не отвечает (нуль — неизвестно).
	DownSince time.Time
	// Attempts — сколько проверок подряд не прошло.
	Attempts int
	// Reach — вид ошибки связи (core.ClassifyDaemonReachError(Err)).
	Reach core.DaemonReachKind
	// CoreStatus — idle | started | fatal (если демон ответил).
	CoreStatus string
	// Running — версия ядра демона (свежая или из кэша паспорта).
	Running string
	// Uptime — сколько работает демон (0 — не сообщал).
	Uptime time.Duration
	// Paths — пути службы: паспорт поверх платформенных дефолтов.
	Paths core.ServicePaths
	// PassportAt — когда демон сообщил паспорт (нуль — никогда).
	PassportAt time.Time
	// PassportLive — паспорт получен на текущем ответе, а не из кэша.
	PassportLive bool
	// Paired — сопряжение есть (удалённо — всегда: запись и есть пара).
	Paired bool
	// InterruptedApply — последнее применение конфига откатилось.
	InterruptedApply bool
	// LiveLog — окно живого лога ядра доступно (удалённо — только у
	// подключённой машины).
	LiveLog bool
	// ServiceNote — строка о службе в шапке (локально: плашка SPEC 136 §6);
	// ServiceNoteDanger — красная, иначе жёлтая.
	ServiceNote       string
	ServiceNoteDanger bool
	// Local — локальные факты; nil у удалённой машины.
	Local *localServiceExtras
}

// localServiceExtras — факты локальной службы, сведённые локальным
// источником из core.DaemonServiceCheck (тип с build-тегом) к плоским полям.
type localServiceExtras struct {
	// ServiceInstalled — служба установлена.
	ServiceInstalled bool
	// NeedsInstall — нужен «Install or update service» (Unsafe/Stale/
	// ProcessStale/NotInstalled).
	NeedsInstall bool
	// NeedsBootstrap — служба установлена, но не загружена (launchd
	// bootstrap / sc.exe start).
	NeedsBootstrap bool
	// InstallSupported — ядро лаунчера умеет root-owned копию службы.
	InstallSupported bool
	// CoreTooOld — ядро лаунчера слишком старое для службы.
	CoreTooOld bool
	// CoreHint — подсказка обновить ядро лаунчера (core.DaemonServiceCoreHint);
	// пусто — не нужна.
	CoreHint string
	// LauncherVersion — версия ядра лаунчера.
	LauncherVersion string
}

// localServiceRows — готовые строки локальной службы, которые строит
// локальный источник поверх daemonOps (macOS — Terminal, Windows — «Run as
// administrator»). Окно встраивает их в свои вкладки вместо строк рецептов.
// Создаются один раз на окно (источник держит их состояние); nil — строки
// нет, окно рисует рецепт или ничего.
type localServiceRows struct {
	// Install — «Install or update service» (вкладка Core).
	Install fyne.CanvasObject
	// Bootstrap — загрузка установленной службы (вкладка Not running).
	Bootstrap fyne.CanvasObject
	// Restart — перезапуск службы (вкладка Not running).
	Restart fyne.CanvasObject
	// FreshInvite — новое приглашение (`lxd client add`, вкладка Pairing).
	FreshInvite fyne.CanvasObject
	// Pair — поле приглашения и Pair (вкладка Pairing).
	Pair fyne.CanvasObject
	// Address — адрес демона (вкладка Pairing).
	Address fyne.CanvasObject
	// Secret — Bearer-секрет plain-режима (вкладка Pairing).
	Secret fyne.CanvasObject
	// Uninstall — содержимое вкладки Uninstall; nil — вкладки нет.
	Uninstall fyne.CanvasObject
}

// serviceSource — чей демон показывает окно: локальная служба или машина.
type serviceSource interface {
	// Key — ключ окна: "local" или id машины (одно окно на ключ).
	Key() string
	// Snapshot — текущее состояние из кэша, без сети; UI-поток.
	Snapshot() serviceSnapshot
	// Poll запускает обновление: onUpdate зовётся в UI-потоке, когда
	// снапшот мог измениться. StopPoll — останов (закрытие окна).
	Poll(onUpdate func())
	StopPoll()
	// SSH — ssh-цель машины; false — команды выполняются на этом компьютере.
	SSH() (services.SSHTarget, bool)
	// SetInit сохраняет выбор init-системы (удалённый Linux); локально — nil.
	SetInit(core.ServiceInit) error
	// Pair — сопряжение по приглашению (addr, secret — необязательные
	// переопределения); done — в UI-потоке. Локально вкладка Pairing берёт
	// localServiceRows.Pair, и вызов не нужен.
	Pair(invite, addr, secret string, done func(error))
	// SetSecret сохраняет Bearer-секрет plain-режима (tls:false).
	SetSecret(secret string) error
	// OpenLiveLog открывает живой лог ядра демона.
	OpenLiveLog(win fyne.Window)
	// LocalRows — строки локальной службы (нулевое значение у машины).
	// Зовётся один раз на окно.
	LocalRows(win fyne.Window) localServiceRows
}

// serviceOSLabel — имя системы для шапки: init-система различает OpenWrt и
// прочий Linux.
func serviceOSLabel(p core.ServicePlatform) string {
	switch p.Init {
	case core.ServiceInitProcd:
		return "OpenWrt" // l10n-exempt: product name
	case core.ServiceInitLaunchd:
		return "macOS" // l10n-exempt: product name
	case core.ServiceInitSCM:
		return "Windows" // l10n-exempt: product name
	}
	return "Linux" // l10n-exempt: product name
}

// serviceCoreVerdict — версия ядра демона против требуемой лаунчером.
func serviceCoreVerdict(s serviceSnapshot) core.CoreVersionVerdict {
	return core.CompareCoreVersion(s.Running, constants.RequiredCoreVersion)
}

// servicePairingBroken — ошибка связи значит «сопряжение», а не «служба
// лежит»: лечится вкладкой Pairing.
func servicePairingBroken(k core.DaemonReachKind) bool {
	switch k {
	case core.ReachCertChanged, core.ReachNotTrusted, core.ReachChannelMismatch:
		return true
	}
	return false
}

// serviceDiagnosis — что чинить: глифы ярлыков вкладок и первая проблемная
// вкладка в порядке Not running → Core → Pairing (иначе Not running). Общий
// вердикт окна; строка машины зовёт machineServiceVerdict поверх тех же
// правил.
func serviceDiagnosis(s serviceSnapshot) (map[serviceTab]string, serviceTab) {
	glyphs := make(map[serviceTab]string)
	if !s.Loaded {
		return glyphs, serviceTabNotRunning
	}
	notRunning := s.Connected && !s.Reachable && s.Paired && !servicePairingBroken(s.Reach)
	coreBad := serviceCoreVerdict(s) == core.CoreVersionOlder
	if l := s.Local; l != nil {
		notRunning = notRunning || l.NeedsBootstrap
		coreBad = coreBad || l.NeedsInstall || l.CoreTooOld || l.CoreHint != ""
	}
	pairingBad := !s.Paired || (s.Connected && !s.Reachable && servicePairingBroken(s.Reach))

	if notRunning {
		glyphs[serviceTabNotRunning] = "✖"
	}
	if coreBad {
		glyphs[serviceTabCore] = "⚠"
	}
	if pairingBad {
		glyphs[serviceTabPairing] = "✖"
	}
	for _, t := range []serviceTab{serviceTabNotRunning, serviceTabCore, serviceTabPairing} {
		if glyphs[t] != "" {
			return glyphs, t
		}
	}
	return glyphs, serviceTabNotRunning
}

// serviceSnapshotEqual — снапшоты равны для отрисовки. Uptime и «давность»
// сравниваются по тому, как они видны в окне (минуты, часы), иначе окно
// перестраивалось бы каждую секунду; указатели — по значению.
func serviceSnapshotEqual(a, b serviceSnapshot) bool {
	if serviceAgeLabel(a.Uptime) != serviceAgeLabel(b.Uptime) ||
		serviceSinceLabel(a.DownSince) != serviceSinceLabel(b.DownSince) ||
		serviceSinceLabel(a.PassportAt) != serviceSinceLabel(b.PassportAt) {
		return false
	}
	if !sameBoolPtr(a.Paths.TLS, b.Paths.TLS) {
		return false
	}
	if (a.Local == nil) != (b.Local == nil) || (a.Local != nil && *a.Local != *b.Local) {
		return false
	}
	a.Uptime, b.Uptime = 0, 0
	a.DownSince, b.DownSince = time.Time{}, time.Time{}
	a.PassportAt, b.PassportAt = time.Time{}, time.Time{}
	a.Paths.TLS, b.Paths.TLS = nil, nil
	a.Local, b.Local = nil, nil
	return a == b
}

// sameBoolPtr — *bool равны по значению (nil равен только nil).
func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// serviceAgeLabel — длительность так, как её видно в окне: не короче
// минуты (секундная точность перестраивала бы окно каждый тик); "" — нуль.
func serviceAgeLabel(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Minute {
		d = time.Minute
	}
	return humanAge(d)
}

// serviceSinceLabel — «сколько прошло с t» для окна; "" — t неизвестно.
func serviceSinceLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return serviceAgeLabel(time.Since(t))
}
