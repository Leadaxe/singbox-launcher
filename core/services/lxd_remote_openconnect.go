// File lxd_remote_openconnect.go — стрим статуса openconnect удалённой
// машины и ответы на челленджи второго фактора / SSO (SPEC 167 §2.5).
// Образец — lxd_remote_tailscale.go: ленивый старт при первом чтении,
// переподписка runResilientStream, гашение в Close.
//
// Отличие: ядро без with_openconnect (или старый демон) отвечает
// Unimplemented — стрим гасится насовсем для этого транспорта, иначе
// runResilientStream молотил бы переподписку каждые 2 с.
package services

import (
	"context"
	"sync"

	"google.golang.org/protobuf/types/known/emptypb"

	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
)

// openConnectStream — состояние ленивого стрима транспорта.
type openConnectStream struct {
	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	cache   OpenConnectStatusCache
}

// ensureOpenConnectStream поднимает стрим один раз на транспорт. Ошибка
// streamConn не фатальна: следующий вызов повторит попытку.
func (t *LxdRemoteTransport) ensureOpenConnectStream() {
	t.oc.mu.Lock()
	defer t.oc.mu.Unlock()
	if t.oc.started || t.oc.cache.Unsupported() {
		return
	}
	conn, err := t.streamConn()
	if err != nil {
		debuglog.DebugLog("lxd remote openconnect: stream not opened: %v", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.oc.cancel = cancel
	t.oc.started = true
	debuglog.DebugLog("lxd remote openconnect: subscribing")
	runResilientStream(ctx, "openconnect", func() error {
		stream, serr := daemonpb.NewStartedServiceClient(conn).SubscribeOpenConnectStatus(ctx, &emptypb.Empty{})
		if serr == nil {
			first := true
			for {
				upd, recvErr := stream.Recv()
				if recvErr != nil {
					serr = recvErr
					break
				}
				if first {
					first = false
					debuglog.DebugLog("lxd remote openconnect: first status frame, %d endpoints", len(upd.GetEndpoints()))
				}
				t.oc.cache.Apply(upd)
			}
		}
		// Unimplemented всплывает в первом Recv: ядро без openconnect.
		// Отмена ctx — единственный штатный выход runResilientStream.
		if IsUnimplementedRPC(serr) {
			debuglog.InfoLog("lxd remote openconnect: core has no openconnect status RPC, stream stopped")
			t.oc.cache.MarkUnsupported()
			cancel()
		}
		return serr
	}, t.oc.cache.MarkDead)
}

// stopOpenConnectStream гасит стрим; зовётся из Close.
func (t *LxdRemoteTransport) stopOpenConnectStream() {
	t.oc.mu.Lock()
	defer t.oc.mu.Unlock()
	if t.oc.cancel != nil {
		t.oc.cancel()
	}
}

// OpenConnectStatus — статус endpoint'а openconnect удалённого ядра из кеша
// (реализует core.openConnectSource).
func (t *LxdRemoteTransport) OpenConnectStatus(tag string) (OpenConnectStatus, bool) {
	t.ensureOpenConnectStream()
	return t.oc.cache.Get(tag)
}

// OpenConnectLive — жив ли стрим статуса удалённого ядра.
func (t *LxdRemoteTransport) OpenConnectLive() bool {
	t.ensureOpenConnectStream()
	return t.oc.cache.Live()
}

// OpenConnectPending — статусы с висящим челленджем.
func (t *LxdRemoteTransport) OpenConnectPending() []OpenConnectStatus {
	t.ensureOpenConnectStream()
	return t.oc.cache.Pending()
}

// OpenConnectSetNotify — колбэк на новые снимки. Ставится ДО первого чтения
// (OpenConnectStatus поднимает стрим), чтобы первый кадр с висящим
// челленджем не прошёл мимо.
func (t *LxdRemoteTransport) OpenConnectSetNotify(fn OpenConnectNotify) {
	t.oc.cache.SetNotify(fn)
}

// OpenConnectSubmitForm — ответ формы второго фактора удалённому ядру.
func (t *LxdRemoteTransport) OpenConnectSubmitForm(tag, challengeID string, values map[string]string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	debuglog.DebugLog("lxd remote openconnect: submit form for %s (%d fields)", tag, len(values))
	return OpenConnectSubmitFormRPC(ctx, client, tag, challengeID, values)
}

// OpenConnectSubmitBrowser — URL редиректа SSO удалённому ядру: браузер и
// слушатель на этом компьютере, ядро получает только строку.
func (t *LxdRemoteTransport) OpenConnectSubmitBrowser(tag, challengeID, finalURL string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	debuglog.DebugLog("lxd remote openconnect: submit browser result for %s", tag)
	return OpenConnectSubmitBrowserRPC(ctx, client, tag, challengeID, finalURL)
}

// OpenConnectCancel — отмена челленджа удалённого ядра (новый логин).
func (t *LxdRemoteTransport) OpenConnectCancel(tag, challengeID string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	debuglog.DebugLog("lxd remote openconnect: cancel challenge for %s", tag)
	return OpenConnectCancelRPC(ctx, client, tag, challengeID)
}
