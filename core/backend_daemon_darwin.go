//go:build darwin

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"singbox-launcher/api"
	"singbox-launcher/core/services"
	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// DaemonBackend — движок daemon-режима: ядро живёт внутри долгоживущего
// `sing-box lxd` (launchd-служба), лаунчер управляет им по admin REST
// (apply/start/stop) и наблюдает по gRPC daemon.StartedService (протокол
// Android-линии). Смена конфига — in-process подмена инстанса в демоне:
// без убийства процесса, без пароля, с валидацией и автооткатом на
// last-good на стороне демона.
type DaemonBackend struct {
	ac    *AppController
	admin *lxdclient.Client

	ctx    context.Context
	cancel context.CancelFunc

	connMu sync.Mutex
	conn   *grpc.ClientConn

	// transport — этот backend's gRPC proxy-транспорт (установлен в
	// APIService.transportOverride). Хранится, чтобы Close снимал ТОЛЬКО
	// свой override, а setBackend мог переустановить его после Close чужого.
	transport *daemonProxyTransport

	// applyMu сериализует Start/Restart/Stop от дребезга кнопок.
	applyMu sync.Mutex

	// Кольцевой буфер логов ядра из gRPC SubscribeLog: в daemon-режиме
	// stdout/stderr ядра принадлежат службе (файл в root-каталоге 0700),
	// поэтому вьюер логов читает отсюда.
	logMu    sync.Mutex
	logLines []string

	// connTracker — накопительный снимок соединений из SubscribeConnections
	// (traffic-график по gRPC вместо Clash /connections).
	connTracker *connTracker
}

// daemonLogMaxLines зеркалит кольцевой буфер демона (lxd logMaxLines).
const daemonLogMaxLines = 3000

// DaemonIdentityDir — каталог клиентской пары сопряжения (bin/daemon).
func DaemonIdentityDir(execDir string) string {
	return filepath.Join(platform.GetBinDir(execDir), "daemon")
}

// DaemonConfigFromSettings строит конфиг клиента демона из settings.json.
// Ошибка — если daemon-режим не сконфигурирован (нет адреса) или не удалось
// поднять клиентскую пару при включённом TLS.
func DaemonConfigFromSettings(ac *AppController) (lxdclient.Config, error) {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	if st.DaemonAddress == "" {
		return lxdclient.Config{}, fmt.Errorf("daemon is not configured: install the service or pair via invite")
	}
	cfg := lxdclient.Config{
		Addr:              st.DaemonAddress,
		ServerFingerprint: st.DaemonServerFingerprint,
		Secret:            st.DaemonSecret,
	}
	if cfg.TLSEnabled() {
		ident, err := lxdclient.LoadOrCreateIdentity(DaemonIdentityDir(ac.FileService.ExecDir))
		if err != nil {
			return lxdclient.Config{}, err
		}
		cfg.Identity = ident
	}
	return cfg, nil
}

// newDaemonBackend конструирует daemon-движок из settings.json и запускает
// supervisor статуса. Вызывается из initBackendFromSettings/SwitchBackendMode.
func newDaemonBackend(ac *AppController) (CoreBackend, error) {
	cfg, err := DaemonConfigFromSettings(ac)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &DaemonBackend{
		ac:     ac,
		admin:  lxdclient.New(cfg),
		ctx:    ctx,
		cancel: cancel,
	}
	b.transport = &daemonProxyTransport{b: b}
	b.connTracker = newConnTracker()
	if ac.APIService != nil {
		ac.APIService.SetTransport(b.transport)
	}
	go b.superviseStatus()
	go b.superviseLogs()
	go b.superviseConnections()
	return b, nil
}

// reinstallTransport переустанавливает свой transport-override в APIService.
// Вызывается setBackend после Close предыдущего backend'а (тот мог снять
// общий override в nil). Идемпотентно.
func (b *DaemonBackend) reinstallTransport() {
	if b.ac.APIService != nil && b.transport != nil {
		b.ac.APIService.SetTransport(b.transport)
	}
}

// diagnoseReachError превращает сырую ошибку связи с демоном в понятный
// пользователю совет (английский) и реализует «лаунчер следует за демоном»:
// режимом канала владеет демон (tls в его daemon.json), клиент лишь
// обнаруживает смену (lxdclient.DetectChannel) и подстраивается — на
// loopback сброс пина автоматический, по сети только совет (авто-даунгрейд
// в сетевом сценарии открывал бы MITM'у downgrade-атаку). Обратный переход
// (демон стал TLS) автоматике недоступен принципиально: пин нельзя
// выдумать, доверие приезжает только с приглашением.
func (b *DaemonBackend) diagnoseReachError(err error) string {
	s := err.Error()
	addr := b.admin.AddrString()
	switch {
	case strings.Contains(s, "first record does not look like a TLS handshake"):
		// Профиль лаунчера — TLS (пин установлен), а на адресе живёт
		// plain-демон: демон перевели на tls:false.
		if lxdclient.DetectChannel(addr) == lxdclient.ChannelPlain {
			if lxdclient.IsLoopbackAddr(addr) {
				b.ac.followDaemonPlainChannel()
				return "Daemon at " + addr + " switched to plain (secret) mode — the launcher followed automatically. " +
					"If the daemon uses a secret, set it in the connection window (Servers tab, gear button), then try again."
			}
			return "Daemon at " + addr + " now runs in plain (secret) mode. " +
				"Unpair in the connection window to follow it (automatic follow over the network is disabled: it would enable downgrade attacks)."
		}
		return "Cannot reach the daemon: " + s
	case strings.Contains(s, "connection refused"), strings.Contains(s, "no such host"), strings.Contains(s, "dial tcp"):
		return "Daemon is not reachable at " + addr +
			". Install the service and pair (Servers tab, gear button, Local), then try again."
	case strings.Contains(s, "fingerprint"), strings.Contains(s, "certificate"):
		return "Daemon certificate changed (the service was reinstalled). " +
			"Re-pair with a fresh invite (Servers tab, gear button, Local), then try again."
	case strings.Contains(s, "HTTP request to an HTTPS server"):
		// Профиль лаунчера — plain, а демон отвечает 400-сигнатурой
		// net/http-TLS-сервера: его вернули на tls:true.
		return "Daemon at " + addr + " now requires mTLS. " +
			"Pair with a fresh invite (mint one with `sudo sing-box lxd client add` on its host), then try again."
	case strings.Contains(s, "EOF"), strings.Contains(s, "connection reset"):
		// Профиль лаунчера — plain, а демон рвёт HTTP-запросы: вероятно, его
		// вернули на TLS (tls:true) — plain-клиенту он отвечать не будет.
		if lxdclient.DetectChannel(addr) == lxdclient.ChannelTLS {
			return "Daemon at " + addr + " now requires mTLS. " +
				"Pair with a fresh invite (mint one with `sudo sing-box lxd client add` on its host), then try again."
		}
		return "Cannot reach the daemon: " + s
	default:
		return "Cannot reach the daemon: " + s
	}
}

// isActive сообщает, является ли этот backend всё ещё активным в контроллере.
// Мёртвый backend (вытеснен setBackend) не должен трогать общее состояние
// (RunningState, UI): его стримы/apply могли пережить Close.
func (b *DaemonBackend) isActive() bool {
	return b.ac.Backend() == CoreBackend(b)
}

// Mode implements CoreBackend.
func (b *DaemonBackend) Mode() BackendMode { return BackendDaemon }

// Admin возвращает REST-клиент admin-плоскости (для секции настроек).
func (b *DaemonBackend) Admin() *lxdclient.Client { return b.admin }

// StartVPN implements CoreBackend: пересборка config.json (как classic) и
// доставка его демону через POST /admin/apply. Apply валидирует конфиг
// сабпроцессом и поднимает ядро; провал старта откатывается на last-good.
func (b *DaemonBackend) StartVPN(skipRunningCheck ...bool) {
	ac := b.ac
	if ac.RunningState.IsRunning() {
		if ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, locale.T("core.info_title"), locale.T("core.already_running"))
		}
		return
	}
	go b.applyCurrentConfig("StartVPN", false)
}

// RestartVPN implements CoreBackend: тот же apply — демон подменит инстанс
// in-process, канал управления и соединения переживут смену конфига.
//
// forced=true: Restart всегда ПОЛНОСТЬЮ пересобирает config.json, даже если
// dirty-маркеры уже сняты (авто-rebuild мог успеть их сбросить). Иначе
// applyCurrentConfig взял бы старый config.json, и правки пользователя не
// доехали бы до демона — ровно тот баг «новый конфиг не загружается при
// перезапуске». Согласовано с classic-путём кнопки Rebuild (forced=true).
func (b *DaemonBackend) RestartVPN() {
	go b.applyCurrentConfig("RestartVPN", true)
}

// applyCurrentConfig — общий путь Start/Restart: rebuild → read → apply.
// forced прокидывается в RebuildConfigIfDirty: Restart форсирует полную
// пересборку, Start — обычный dirty-путь.
func (b *DaemonBackend) applyCurrentConfig(caller string, forced bool) {
	b.applyMu.Lock()
	defer b.applyMu.Unlock()
	ac := b.ac

	// Pre-start rebuild — тот же хук, что в classic ProcessService.Start:
	// dirty-маркеры Wizard'а материализуются в config.json перед доставкой.
	// Restart форсирует полную пересборку (forced=true), иначе взяли бы
	// устаревший config.json.
	if err := ac.RebuildConfigIfDirty(forced); err != nil {
		debuglog.WarnLog("daemon.%s: config rebuild failed (%v); proceeding with existing config.json", caller, err)
	}

	// Синхронизируем APIService с пересобранным config.json. В daemon-режиме
	// Clash API из доставляемой демону копии ВЫРЕЗАН (prepareConfigForDaemon),
	// а proxy-операции идут через gRPC transport-override — но состояние
	// APIService (endpoint, Enabled) должно отражать config.json на диске,
	// чтобы возврат на classic-режим не унёс устаревший endpoint.
	if ac.APIService != nil {
		if err := ac.APIService.ReloadClashAPIConfig(); err != nil {
			debuglog.WarnLog("daemon.%s: reload Clash API config: %v", caller, err)
		}
	}
	if ac.UIService != nil && ac.UIService.ResetAPIStateFunc != nil {
		ac.UIService.ResetAPIStateFunc()
	}

	config, err := os.ReadFile(ac.FileService.ConfigPath)
	if err != nil {
		ac.ShowStartupError(fmt.Errorf("daemon apply: cannot read config.json: %w", err))
		return
	}

	// Pre-flight: убеждаемся, что демон жив и его сертификат совпадает с
	// закреплённым, ДО отправки конфига — иначе пользователь видит сырой
	// "connection refused" / "fingerprint mismatch" вместо понятного совета.
	if _, err := b.admin.Status(); err != nil {
		msg := b.diagnoseReachError(err)
		ac.ShowStartupError(fmt.Errorf("%s", msg))
		return
	}

	// Подготовка конфига для демона: (1) абсолютизация cache_file в каталог,
	// которым ВЛАДЕЕТ демон — state_dir из его паспорта (/admin/info); демон
	// работает с cwd="/", относительный путь ушёл бы в read-only корень.
	// Fallback на историческую константу — только если info недоступен
	// (демон старой сборки). (2) Полное удаление clash_api (всё по gRPC).
	// Classic-режим этой подготовки не проходит (cwd=bin/, единственное ядро).
	runtimeDir := daemonFallbackRuntimeDir
	if passport, infoErr := b.admin.Info(); infoErr == nil && passport.StateDir != "" {
		runtimeDir = passport.StateDir
	} else if infoErr != nil {
		debuglog.WarnLog("daemon.%s: /admin/info unavailable (%v); using fallback runtime dir", caller, infoErr)
	}
	config, err = prepareConfigForDaemon(config, runtimeDir)
	if err != nil {
		ac.ShowStartupError(fmt.Errorf("daemon apply: prepare config: %w", err))
		return
	}

	debuglog.InfoLog("daemon.%s: applying config.json (%d bytes) to %s", caller, len(config), b.admin.AddrString())
	if err := b.admin.Apply(config); err != nil {
		debuglog.ErrorLog("daemon.%s: apply failed: %v", caller, err)
		ac.ShowStartupError(fmt.Errorf("daemon apply: %w", err))
		// Статус мог смениться (откат/фатал) — supervisor подтянет.
		b.refreshUI()
		return
	}

	if !b.isActive() {
		// Backend вытеснен (смена адреса/пересопряжение) пока летел apply —
		// не трогаем общее состояние: им владеет новый backend.
		debuglog.InfoLog("daemon.%s: applied but backend is no longer active; skipping state update", caller)
		return
	}
	ac.RunningState.Set(true) // стрим статусов подтвердит
	ac.StateService.ResetAutoUpdateFailedAttempts()
	debuglog.InfoLog("daemon.%s: config applied, core is up", caller)
	b.refreshUI()

	go func() {
		select {
		case <-time.After(2 * time.Second):
		case <-b.ctx.Done():
			return
		}
		ac.AutoLoadProxies()
	}()
}

// StopVPN implements CoreBackend: ядро гаснет, демон и канал остаются жить.
func (b *DaemonBackend) StopVPN() {
	go func() {
		b.applyMu.Lock()
		defer b.applyMu.Unlock()
		ac := b.ac
		if err := b.admin.Stop(); err != nil {
			debuglog.ErrorLog("daemon.StopVPN: %v", err)
			if ac.hasUI() {
				dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf("daemon stop: %w", err))
			}
			return
		}
		ac.RunningState.Set(false)
	}()
}

// OnAppExit implements CoreBackend: по умолчанию выход из лаунчера оставляет
// VPN работать (это и есть смысл daemon-режима); опция DaemonStopVPNOnExit
// возвращает классическое поведение.
func (b *DaemonBackend) OnAppExit() bool {
	binDir := platform.GetBinDir(b.ac.FileService.ExecDir)
	if !locale.LoadSettings(binDir).DaemonStopVPNOnExit {
		return false
	}
	if err := b.admin.Stop(); err != nil {
		debuglog.WarnLog("daemon.OnAppExit: stop failed: %v", err)
	}
	b.ac.RunningState.Set(false)
	return true
}

// Close implements CoreBackend: гасит supervisor и gRPC-соединение, снимает
// транспорт-override. Ядро в демоне не трогается.
func (b *DaemonBackend) Close() {
	b.cancel()
	// Снимаем override ТОЛЬКО если он всё ещё наш: при daemon→daemon свопе
	// новый backend уже установил свой транспорт до нашего Close, и затирать
	// его в nil нельзя (иначе proxy-операции теряют gRPC-транспорт).
	if b.ac.APIService != nil && b.ac.APIService.TransportOverride() == services.ProxyTransport(b.transport) {
		b.ac.APIService.SetTransport(nil)
	}
	b.connMu.Lock()
	if b.conn != nil {
		_ = b.conn.Close()
		b.conn = nil
	}
	b.connMu.Unlock()
}

// refreshUI дёргает статус-виджеты после операций, которые могли не дать
// перехода RunningState (Set дедуплицирует no-op вызовы).
func (b *DaemonBackend) refreshUI() {
	if b.ac.UIService != nil && b.ac.UIService.UpdateCoreStatusFunc != nil {
		b.ac.UIService.UpdateCoreStatusFunc()
	}
}

// grpcConn лениво создаёт единственное gRPC-соединение (grpc.NewClient сам
// переустанавливает транспорт при обрывах).
func (b *DaemonBackend) grpcConn() (*grpc.ClientConn, error) {
	b.connMu.Lock()
	defer b.connMu.Unlock()
	if b.conn != nil {
		return b.conn, nil
	}
	conn, err := b.admin.DialGRPC()
	if err != nil {
		return nil, err
	}
	b.conn = conn
	return conn, nil
}

// grpcClient возвращает клиента протокола StartedService.
func (b *DaemonBackend) grpcClient() (daemonpb.StartedServiceClient, error) {
	conn, err := b.grpcConn()
	if err != nil {
		return nil, err
	}
	return daemonpb.NewStartedServiceClient(conn), nil
}

// superviseStatus держит подписку SubscribeServiceStatus: демон шлёт текущий
// статус сразу при подписке и каждый переход дальше — один стрим видит всё
// без переподключения. Обрыв → reconnect с экспоненциальным backoff.
//
// Backoff сбрасывается ТОЛЬКО после реально полученного кадра: создание
// стрима у grpc-go ленивое (NewClient не коннектится, Subscribe* возвращает
// stream без ошибки даже при лежащем демоне — ошибка всплывает в первом
// Recv), поэтому сброс «по успешному Subscribe» держал бы reconnect-цикл на
// вечной 1s без роста.
func (b *DaemonBackend) superviseStatus() {
	backoff := time.Second
	for {
		if b.ctx.Err() != nil {
			return
		}
		client, err := b.grpcClient()
		if err == nil {
			var stream grpc.ServerStreamingClient[daemonpb.ServiceStatus]
			stream, err = client.SubscribeServiceStatus(b.ctx, &emptypb.Empty{})
			if err == nil && b.consumeStatusStream(stream) {
				backoff = time.Second
			}
		}
		if err != nil {
			debuglog.DebugLog("daemon.supervisor: status stream unavailable: %v", err)
		}
		select {
		case <-b.ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

// consumeStatusStream читает стрим до обрыва; возвращает true, если был
// получен хотя бы один кадр (стрим реально работал, а не умер на первом Recv).
func (b *DaemonBackend) consumeStatusStream(stream grpc.ServerStreamingClient[daemonpb.ServiceStatus]) bool {
	ac := b.ac
	received := false
	for {
		status, err := stream.Recv()
		if err != nil {
			debuglog.DebugLog("daemon.supervisor: status stream closed: %v", err)
			return received
		}
		received = true
		if !b.isActive() {
			// Стрим вытесненного backend'а не должен дёргать общее состояние.
			continue
		}
		wasRunning := ac.RunningState.IsRunning()
		var running bool
		switch status.GetStatus() {
		case daemonpb.ServiceStatus_STARTED:
			running = true
		case daemonpb.ServiceStatus_STARTING, daemonpb.ServiceStatus_STOPPING:
			// In-process reload (apply конфига) проходит STOPPING→STARTING→
			// STARTED без разрыва туннеля. Не мигаем RunningState в false на
			// промежуточных состояниях — держим текущее, чтобы UI не дёргался.
			running = wasRunning
		case daemonpb.ServiceStatus_IDLE:
			running = false
		case daemonpb.ServiceStatus_FATAL:
			running = false
			debuglog.ErrorLog("daemon.supervisor: core FATAL: %s", status.GetErrorMessage())
		}
		ac.RunningState.Set(running)
		if running && !wasRunning {
			// Ядро поднялось (в т.ч. кем-то извне) — подтянуть список нод.
			go func() {
				select {
				case <-time.After(2 * time.Second):
				case <-b.ctx.Done():
					return
				}
				ac.AutoLoadProxies()
			}()
		}
	}
}

// superviseLogs держит подписку SubscribeLog и наполняет кольцевой буфер.
// Тот же reconnect-паттерн, что у superviseStatus (сброс backoff — только
// после реально полученного кадра, см. комментарий там).
func (b *DaemonBackend) superviseLogs() {
	backoff := time.Second
	for {
		if b.ctx.Err() != nil {
			return
		}
		client, err := b.grpcClient()
		if err == nil {
			var stream grpc.ServerStreamingClient[daemonpb.Log]
			stream, err = client.SubscribeLog(b.ctx, &emptypb.Empty{})
			if err == nil && b.consumeLogStream(stream) {
				backoff = time.Second
			}
		}
		if err != nil {
			debuglog.DebugLog("daemon.logs: stream unavailable: %v", err)
		}
		select {
		case <-b.ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

// consumeLogStream читает стрим до обрыва; true — получен хотя бы один кадр.
func (b *DaemonBackend) consumeLogStream(stream grpc.ServerStreamingClient[daemonpb.Log]) bool {
	received := false
	for {
		batch, err := stream.Recv()
		if err != nil {
			debuglog.DebugLog("daemon.logs: stream closed: %v", err)
			return received
		}
		received = true
		b.logMu.Lock()
		if batch.GetReset_() {
			b.logLines = b.logLines[:0]
		}
		for _, message := range batch.GetMessages() {
			b.logLines = append(b.logLines, message.GetMessage())
		}
		if overflow := len(b.logLines) - daemonLogMaxLines; overflow > 0 {
			b.logLines = append(b.logLines[:0], b.logLines[overflow:]...)
		}
		b.logMu.Unlock()
	}
}

// CoreLogLines implements coreLogSource: хвост буфера логов ядра.
func (b *DaemonBackend) CoreLogLines(max int) []string {
	b.logMu.Lock()
	defer b.logMu.Unlock()
	start := 0
	if max > 0 && len(b.logLines) > max {
		start = len(b.logLines) - max
	}
	out := make([]string, len(b.logLines)-start)
	copy(out, b.logLines[start:])
	return out
}

// ClashEndpoint implements clashEndpointSource. В daemon-режиме Clash API
// вырезан из конфига (prepareConfigForDaemon), потому что управление, ноды и
// трафик идут по gRPC. Поэтому Clash-эндпоинта нет: возвращаем ok=false, и
// UI-слой Clash (Test API, traffic-поллер по /connections) в daemon-режиме
// не активируется. Метод оставлен как явный сигнал «Clash недоступен», а не
// удалён, чтобы профайлер мог отличить daemon-режим от «Clash просто выключен
// в classic-конфиге».
func (b *DaemonBackend) ClashEndpoint() (baseURL, token string, ok bool) {
	return "", "", false
}

// PoolSlots implements poolSource через lx-RPC GetPool.
func (b *DaemonBackend) PoolSlots(group string) ([]PoolSlotInfo, error) {
	client, err := b.grpcClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(b.ctx, daemonRPCTimeout)
	defer cancel()
	pool, err := client.GetPool(ctx, &daemonpb.GetPoolRequest{GroupTag: group})
	if err != nil {
		return nil, fmt.Errorf("daemon GetPool: %w", err)
	}
	slots := make([]PoolSlotInfo, 0, len(pool.GetSlots()))
	for _, slot := range pool.GetSlots() {
		slots = append(slots, PoolSlotInfo{Slot: slot.GetSlot(), Tag: slot.GetTag(), Delay: slot.GetDelay()})
	}
	return slots, nil
}

// --- gRPC-транспорт proxy-операций (Servers tab, tray, auto-load) --------

// daemonProxyTransport реализует services.ProxyTransport поверх gRPC:
// группы/выбор/URL-тест — те же RPC, что использует Android CommandClient.
type daemonProxyTransport struct {
	b *DaemonBackend
}

const daemonRPCTimeout = 15 * time.Second

func (t *daemonProxyTransport) rpc() (daemonpb.StartedServiceClient, context.Context, context.CancelFunc, error) {
	client, err := t.b.grpcClient()
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(t.b.ctx, daemonRPCTimeout)
	return client, ctx, cancel, nil
}

// GroupProxies implements services.ProxyTransport через GetGroups.
func (t *daemonProxyTransport) GroupProxies(group string) ([]api.ProxyInfo, string, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, "", err
	}
	defer cancel()
	groups, err := client.GetGroups(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, "", fmt.Errorf("daemon GetGroups: %w", err)
	}
	for _, g := range groups.GetGroup() {
		if g.GetTag() != group {
			continue
		}
		selected := g.GetSelected()
		proxies := make([]api.ProxyInfo, 0, len(g.GetItems()))
		for _, item := range g.GetItems() {
			info := api.ProxyInfo{
				Name:      item.GetTag(),
				ClashType: item.GetType(),
				Delay:     int64(item.GetUrlTestDelay()),
			}
			// Проставляем Now для активного узла группы, чтобы Servers-tab
			// рисовал маркер выбранного (Clash-путь заполняет Now из ответа
			// /proxies; GetGroups отдаёт выбор только на уровне группы —
			// разворачиваем его в per-node Now у совпадающего узла).
			if item.GetTag() == selected {
				info.Now = selected
			}
			proxies = append(proxies, info)
		}
		return proxies, selected, nil
	}
	return nil, "", fmt.Errorf("daemon: group %q not found", group)
}

// SwitchProxy implements services.ProxyTransport через SelectOutbound.
func (t *daemonProxyTransport) SwitchProxy(group, name string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	if _, err := client.SelectOutbound(ctx, &daemonpb.SelectOutboundRequest{GroupTag: group, OutboundTag: name}); err != nil {
		return fmt.Errorf("daemon SelectOutbound: %w", err)
	}
	return nil
}

// Delay implements services.ProxyTransport через lx-RPC URLTestOutbound —
// точечный URL-тест одного узла с granular cancel (SPEC 015 форка).
func (t *daemonProxyTransport) Delay(proxyName string) (int64, error) {
	client, err := t.b.grpcClient()
	if err != nil {
		return 0, err
	}
	// Собственный дедлайн чуть больше серверного таймаута теста.
	ctx, cancel := context.WithTimeout(t.b.ctx, daemonRPCTimeout)
	defer cancel()
	resp, err := client.URLTestOutbound(ctx, &daemonpb.URLTestOutboundRequest{
		OutboundTag: proxyName,
		Link:        api.GetPingTestURL(),
		// Timeout — миллисекунды (uint32), в отличие от Interval у Subscribe*,
		// который в наносекундах (time.Duration). Всегда через time.Duration.
		// 5s — единый бюджет одиночного теста во всех трёх транспортах
		// (classic GetDelay шлёт timeout=5000).
		Timeout: uint32((5 * time.Second).Milliseconds()),
	})
	if err != nil {
		return 0, fmt.Errorf("daemon URLTestOutbound: %w", err)
	}
	if resp.GetError() != "" {
		return 0, fmt.Errorf("%s", resp.GetError())
	}
	return int64(resp.GetDelay()), nil
}

// Убедимся на компиляции, что транспорт реализует интерфейс.
var _ services.ProxyTransport = (*daemonProxyTransport)(nil)
