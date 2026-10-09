package ui

import (
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// Источник окна Service «удалённая машина» (SPEC 161, PLAN §6.4). Сеть не
// трогает: состояние — кэш панели машин (health/liveness, который ведут
// Connect и heartbeat) и запись реестра (кэш паспорта, ssh, init).

// serviceRemoteRefresh — как часто окно перечитывает кэш панели. Как у
// журнала обмена: новое состояние приходит в ритме heartbeat (5 с), секунда
// даёт шапке позеленеть почти сразу после ответа.
const serviceRemoteRefresh = time.Second

const (
	serviceCoreOlderLineText = "⚠ Core %s is older than required (%s)"
	serviceCoreOlderTip      = "Core is older than required — open Service"
	serviceDownTip           = "Daemon does not answer — open Service"
)

type remoteServiceSource struct {
	p  *machineListPanel
	id string
	// d — запись на момент последнего снапшота (ssh/init могли смениться в
	// окне Edit — перечитывается на каждом снапшоте).
	d services.RemoteDaemon

	stopOnce sync.Once
	stop     chan struct{}
}

func newRemoteServiceSource(p *machineListPanel, d services.RemoteDaemon) *remoteServiceSource {
	return &remoteServiceSource{p: p, id: d.ID, d: d, stop: make(chan struct{})}
}

func (s *remoteServiceSource) Key() string { return s.id }

func (s *remoteServiceSource) Snapshot() serviceSnapshot {
	if d, ok, err := s.p.registry.Get(s.id); err == nil && ok {
		s.d = d
	}
	d := s.d
	tgt := d.Target()
	platform := core.ServicePlatform{GOOS: tgt.GOOS, GOARCH: tgt.GOARCH,
		Init: core.NormalizeInitChoice(d.InitSystem, tgt.GOOS, tgt.GOARCH)}

	h, connected := s.p.healthOf(s.id)
	live := s.p.livenessOf(s.id)
	snap := serviceSnapshot{
		Loaded:     true,
		Title:      d.Name,
		Platform:   platform,
		InitChoice: tgt.GOOS == "linux",
		Connected:  connected,
		Paths:      core.MergeServicePaths(core.RemoteServicePassport(d), core.DefaultServicePaths(platform)),
		Paired:     true,
	}
	if d.Passport != nil {
		snap.Running = d.Passport.Version
		if t, err := time.Parse(time.RFC3339, d.Passport.SeenAt); err == nil {
			snap.PassportAt = t
		}
	}
	_, snap.LiveLog = lxdOverrideTransportForID(s.id)
	if !connected {
		return snap
	}
	snap.Reachable = h.Err == "" && live.FailStreak < heartbeatFailThreshold
	if !snap.Reachable {
		snap.Err = h.Err
		if live.LastErr != "" {
			snap.Err = live.LastErr
		}
		snap.Reach = core.ClassifyDaemonReachError(snap.Err)
		snap.DownSince = live.FailSince
		snap.Attempts = live.FailStreak
	}
	snap.CoreStatus = h.CoreStatus
	if h.Version != "" {
		snap.Running = h.Version
	}
	snap.Uptime = time.Duration(h.UptimeSeconds) * time.Second
	snap.PassportLive = snap.Reachable && h.TLS != nil
	snap.InterruptedApply = h.InterruptedApply
	return snap
}

func (s *remoteServiceSource) Poll(onUpdate func()) {
	go func() {
		t := time.NewTicker(serviceRemoteRefresh)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				fyne.Do(onUpdate)
			}
		}
	}()
}

func (s *remoteServiceSource) StopPoll() { s.stopOnce.Do(func() { close(s.stop) }) }

func (s *remoteServiceSource) SSH() (services.SSHTarget, bool) {
	return machineSSHTarget(s.d), true
}

// machineSSHTarget — ssh-цель машины: сохранённая или root@<хост демона>.
func machineSSHTarget(d services.RemoteDaemon) services.SSHTarget {
	if strings.TrimSpace(d.SSH) != "" {
		if t, err := services.ParseSSHTarget(d.SSH); err == nil {
			return t
		}
	}
	return services.DefaultSSHTarget(d.Addr)
}

func (s *remoteServiceSource) SetInit(init core.ServiceInit) error {
	return s.p.registry.SetInitSystem(s.id, string(init))
}

func (s *remoteServiceSource) Pair(invite, addr, secret string, done func(error)) {
	rePairMachine(s.p.registry, s.d, invite, addr, secret, func(_ services.RemoteDaemon, err error) {
		if err == nil {
			// Тот же исход, что у re-pair из окна Edit: строка возвращается к
			// Connect — прежний канал говорил по отозванному мандату.
			if id, _, ok := GetLxdRemoteOverride(); ok && id == s.id {
				s.p.disconnectMachine()
			} else {
				s.p.Reload()
			}
		}
		done(err)
	})
}

func (s *remoteServiceSource) SetSecret(secret string) error {
	return s.p.registry.SetSecret(s.id, secret)
}

func (s *remoteServiceSource) OpenLiveLog(fyne.Window) {
	OpenMachineCoreLogWindow(s.p.ac, s.d)
}

func (s *remoteServiceSource) LocalRows(fyne.Window) localServiceRows { return localServiceRows{} }

// rePairMachine — повторное сопряжение записи (общее для окна Edit и вкладки
// Pairing окна Service). Enroll — сетевой вызов, поэтому в горутине; done —
// в UI-потоке. При успехе закрывает окна, живущие на канале машины: они
// говорят по отозванному пину.
func rePairMachine(registry *services.RemoteRegistry, d services.RemoteDaemon, invite, addr, secret string,
	done func(services.RemoteDaemon, error)) {
	go func() {
		entry, err := registry.RePair(d.ID, invite, strings.TrimSpace(addr), strings.TrimSpace(secret))
		fyne.Do(func() {
			if err != nil {
				debuglog.WarnLog("machine: re-pair %q: %v", d.ID, err)
				done(entry, err)
				return
			}
			if id, _, ok := GetLxdRemoteOverride(); ok && id == d.ID {
				CloseMachineProfiler(d.ID)
				CloseMachineHostWindow(d.ID)
				CloseMachineCoreLogWindow(d.ID)
			}
			debuglog.InfoLog("machine: re-paired %q at %s", entry.Name, entry.Addr)
			done(entry, nil)
		})
	}()
}

// machineServiceVerdict — вердикт строки машины: точка на ⚙, строка
// предупреждения под статусом и подсказка ⚙. Только для подключённой
// машины: до Connect состояние неизвестно, и выдумывать его точкой нельзя.
func machineServiceVerdict(h services.RemoteHealth, live machineLiveness, connected bool) (level serviceLevel, line, tip string) {
	if !connected {
		return serviceLevelOK, "", ""
	}
	down := h.Err != "" || live.FailStreak >= heartbeatFailThreshold || (h.InterruptedApply && live.FailStreak > 0)
	if down {
		err := h.Err
		if live.LastErr != "" {
			err = live.LastErr
		}
		if err == "" {
			err = locale.T("the daemon does not answer")
		}
		return serviceLevelDown, "✖ " + err, locale.T(serviceDownTip)
	}
	if core.CompareCoreVersion(h.Version, constants.RequiredCoreVersion) == core.CoreVersionOlder {
		running, required := core.CoreVersionPairLabels(h.Version, constants.RequiredCoreVersion)
		return serviceLevelCore, locale.Tf(serviceCoreOlderLineText, running, required), locale.T(serviceCoreOlderTip)
	}
	return serviceLevelOK, "", ""
}
