package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	tprof "singbox-launcher/internal/traffic"
	"singbox-launcher/ui/traffic"
)

// Профайлер трафика УДАЛЁННОЙ машины (SPEC 059 + SPEC 098).
//
// Отдельный экземпляр на машину, а не переключение общего: профайлер — это
// поток соединений плюс буфер записи, и один на всех означал бы ровно ту
// болезнь, что мы уже лечили в списке узлов — открыл профайлер роутера и
// потерял локальный. Две машины при этом нельзя было бы смотреть рядом, а
// именно ради сравнения такой инструмент и открывают.
//
// Источник — только gRPC SubscribeConnections этой машины. Clash к удалённой
// машине не применяется by design: у её конфига Clash API нет вовсе.

// machineProfiler — профайлер одной машины: собственный экземпляр движка,
// собственное окно и живой стрим соединений.
type machineProfiler struct {
	profiler *tprof.TrafficProfiler
	window   *traffic.Manager
	cancel   func()
}

var (
	machineProfilersMu sync.Mutex
	// machineProfilers — по одному профайлеру на машину, чтобы повторное
	// открытие фокусировало существующее окно, а не плодило стримы.
	machineProfilers = map[string]*machineProfiler{}
)

// OpenMachineProfiler открывает окно профайлера трафика удалённой машины.
//
// Требует живого соединения: источник — её gRPC-стрим, и без транспорта
// смотреть нечего.
func OpenMachineProfiler(ac *core.AppController, d services.RemoteDaemon) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}

	machineProfilersMu.Lock()
	if mp, ok := machineProfilers[d.ID]; ok {
		machineProfilersMu.Unlock()
		mp.window.Show()
		return
	}
	machineProfilersMu.Unlock()

	transport, ok := lxdOverrideTransportForID(d.ID)
	if !ok {
		ShowErrorText(ac.UIService.MainWindow, d.Name,
			locale.T("remote.profiler.needs_connect"))
		return
	}

	snapshot, cancel, err := transport.SubscribeConnections()
	if err != nil {
		debuglog.WarnLog("machine profiler: subscribe %q: %v", d.ID, err)
		ShowError(ac.UIService.MainWindow, err)
		return
	}

	p := tprof.NewTrafficProfiler()
	// Пустой logPath: лог машины лежит на ЕЁ файловой системе. Разбирать его
	// и не нужно — DNS-плоскость приходит структурным стримом (SPEC 018
	// форка), где сервер, ответы и признак отказа уже разложены по полям.
	// Clash-конфиг тоже пуст: к удалённой машине Clash API не применяется
	// by design, весь обмен идёт по gRPC.
	p.Start(func() (string, string, bool) { return "", "", false }, "", profilerHTTPClient)
	p.SetConnSnapshotFunc(tprof.SnapshotFunc(snapshot))

	// DNS-события: отдельный стрим, события подаются профайлеру тем же путём,
	// что у локального дают строки лога.
	dnsCancel, dnsErr := transport.SubscribeDNSQueries(func(q services.DNSQuery) {
		p.PushEvent(dnsQueryToEvent(q))
	})
	if dnsErr != nil {
		// Без DNS-потока профайлер остаётся рабочим — просто без доменной
		// плоскости. Ронять окно из-за этого нельзя.
		debuglog.WarnLog("machine profiler: dns stream %q: %v", d.ID, dnsErr)
		dnsCancel = func() {}
	}

	// Сводка ядра машины: связи, память, скорости. Стрим шапки — отдельный,
	// по рецепту lxd-grpc-api.md.
	statusCancel, statusErr := transport.SubscribeStatus(func(st services.RemoteStatus) {
		mpStatusStore.set(d.ID, st)
	})
	if statusErr != nil {
		debuglog.WarnLog("machine profiler: status stream %q: %v", d.ID, statusErr)
		statusCancel = func() {}
	}

	// Справочники читаем один раз на открытие: правила и теги outbound'ов
	// нужны, чтобы расшифровать Connection.rule и звенья цепочки — сырые
	// значения без них читаются как индексы.
	if rules, rErr := transport.Rules(); rErr == nil {
		debuglog.DebugLog("machine profiler: %q rules=%d", d.ID, len(rules))
	} else {
		debuglog.DebugLog("machine profiler: %q GetRules: %v", d.ID, rErr)
	}
	if obs, oErr := transport.Outbounds(); oErr == nil {
		debuglog.DebugLog("machine profiler: %q outbounds=%d", d.ID, len(obs))
	} else {
		debuglog.DebugLog("machine profiler: %q GetOutbounds: %v", d.ID, oErr)
	}

	mp := &machineProfiler{profiler: p, cancel: func() {
		cancel()
		dnsCancel()
		statusCancel()
		mpStatusStore.drop(d.ID)
	}}
	mp.window = traffic.NewManager(traffic.WindowDeps{
		App:      ac.UIService.Application,
		Profiler: p,
		// Настройки уровня лога и find_process — свойства ЛОКАЛЬНОГО конфига;
		// у машины он свой, и править его отсюда нельзя. Оставляем nil:
		// окно скрывает эти элементы само.
		SingBoxRunning: func() bool { return true },
		RemoteMachine:  true,
		// Справочник устройств машины: аренды DHCP, ARP, порты моста, Wi-Fi
		// и свои метки. Кэш живёт в транспорте — здесь только перевод формы.
		ClientsInfo: func() (map[string]traffic.DeviceInfo, bool) {
			m, ok := transport.ClientsInfo()
			if !ok {
				return nil, false
			}
			out := make(map[string]traffic.DeviceInfo, len(m))
			for ip, c := range m {
				out[ip] = traffic.DeviceInfo{
					Name: c.Name, MAC: c.MAC, SSID: c.SSID,
					Iface: c.Iface, Port: c.Port, Source: c.Source,
				}
			}
			return out, true
		},
		SetClientLabel:    transport.SetClientLabel,
		DeleteClientLabel: transport.DeleteClientLabel,
		CloseConns: func(ids []string) {
			go func() {
				for _, id := range ids {
					if err := transport.CloseConnection(id); err != nil {
						debuglog.WarnLog("machine profiler: close conn %s: %v", id, err)
						return
					}
				}
			}()
		},
	})

	machineProfilersMu.Lock()
	machineProfilers[d.ID] = mp
	machineProfilersMu.Unlock()

	mp.window.Show()
}

// CloseMachineProfiler гасит профайлер машины: рвёт стрим и забывает
// экземпляр. Зовётся при Disconnect и удалении машины — без этого стрим жил
// бы к машине, с которой разговор уже не идёт.
func CloseMachineProfiler(id string) {
	machineProfilersMu.Lock()
	mp, ok := machineProfilers[id]
	delete(machineProfilers, id)
	machineProfilersMu.Unlock()
	if !ok {
		return
	}
	if mp.cancel != nil {
		mp.cancel()
	}
	if mp.profiler != nil {
		mp.profiler.Stop()
	}
}

// lxdOverrideTransportForID — транспорт машины, если она сейчас активна.
//
// Профайлер работает по тому же каналу, что и список узлов: отдельное
// соединение ради него означало бы второй gRPC-стрим к той же машине.
func lxdOverrideTransportForID(id string) (*services.LxdRemoteTransport, bool) {
	activeID, _, connected := GetLxdRemoteOverride()
	if !connected || activeID != id {
		return nil, false
	}
	t, ok := lxdOverrideTransportOrNil().(*services.LxdRemoteTransport)
	return t, ok
}

// dnsQueryToEvent переводит DNS-событие машины в событие профайлера — ту же
// форму, в какой их порождает разбор локального лога.
func dnsQueryToEvent(q services.DNSQuery) tprof.TrafficEvent {
	e := tprof.TrafficEvent{
		TS:          time.Now(),
		Kind:        tprof.EventDNSResolve,
		Domain:      q.Domain,
		ProcessPath: q.ProcessPath,
		ProcessName: processName(q.ProcessPath),
		CnameChain:  q.CNAMEs,
	}
	// Адреса ответа и сервер, который их дал, кладём в диагностическую
	// строку: отдельных полей под них в модели нет, а при разборе «почему
	// домен ушёл не туда» нужны оба.
	var b strings.Builder
	if q.DNSServer != "" {
		fmt.Fprintf(&b, "server=%s ", q.DNSServer)
	}
	if len(q.Answers) > 0 {
		fmt.Fprintf(&b, "answers=%s", strings.Join(q.Answers, ","))
	}
	e.RawLogLine = strings.TrimSpace(b.String())
	if len(q.Answers) > 0 {
		e.IP = q.Answers[0]
	}
	if q.Failed {
		e.Kind = tprof.EventDNSFail
		if q.Error != "" {
			e.RawLogLine = strings.TrimSpace(e.RawLogLine + " error=" + q.Error)
		}
	}
	return e
}

// processName — имя процесса из пути. Путь приходит с удалённой машины, то
// есть всегда в unix-форме, независимо от того, где работает лаунчер.
func processName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// statusStore — последняя сводка ядра по машинам.
//
// Стрим статуса живёт вместе с окном, а читают его из UI-потока при
// перерисовке шапки — отсюда мьютекс.
type statusStore struct {
	mu   sync.Mutex
	byID map[string]services.RemoteStatus
}

var mpStatusStore = &statusStore{byID: map[string]services.RemoteStatus{}}

func (s *statusStore) set(id string, st services.RemoteStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[id] = st
}

func (s *statusStore) drop(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

// MachineStatus — последняя известная сводка ядра машины (ok=false, если
// стрим ещё ничего не принёс).
func MachineStatus(id string) (services.RemoteStatus, bool) {
	mpStatusStore.mu.Lock()
	defer mpStatusStore.mu.Unlock()
	st, ok := mpStatusStore.byID[id]
	return st, ok
}
