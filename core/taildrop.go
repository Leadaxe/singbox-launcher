// File taildrop.go — файлы Taildrop узла tailnet (SPEC 175) у ядра цели:
// своего (локальный демон) или выбранной машины. Источник — тот же, что у
// команд tailnet (tailscaleControl): файл не должен уйти не в то ядро, чьи
// устройства на экране. Linux classic и Windows 386 ходят в Clash API —
// Taildrop там нет.
package core

import (
	"context"
	"errors"
	"io"
	"time"

	"singbox-launcher/core/services"
	daemonpb "singbox-launcher/internal/daemonpb"
)

// taildropSource — источник, отдающий клиента StartedService для Taildrop.
type taildropSource interface {
	TaildropClient() (daemonpb.StartedServiceClient, error)
}

// errTaildropUnavailable — у ядра цели нет gRPC (classic).
var errTaildropUnavailable = errors.New("Taildrop is not available for this core")

// taildropCallTimeout — дедлайн разовых вызовов (удаление, отмена приёма).
const taildropCallTimeout = 10 * time.Second

func (ac *AppController) taildropClient(t TailscaleTarget) (daemonpb.StartedServiceClient, error) {
	src, ok := ac.tailscaleSource(t)
	if !ok {
		return nil, errTaildropUnavailable
	}
	td, ok := src.(taildropSource)
	if !ok {
		return nil, errTaildropUnavailable
	}
	return td.TaildropClient()
}

// TaildropAvailable — есть ли Taildrop у ядра цели (проверка типа, без RPC).
func (ac *AppController) TaildropAvailable(t TailscaleTarget) bool {
	src, ok := ac.tailscaleSource(t)
	if !ok {
		return false
	}
	_, ok = src.(taildropSource)
	return ok
}

// TaildropSubscribe — входящие узла в onInbox до отмены ctx или обрыва.
func (ac *AppController) TaildropSubscribe(ctx context.Context, t TailscaleTarget, tag string, onInbox func(services.TaildropInbox)) error {
	client, err := ac.taildropClient(t)
	if err != nil {
		return err
	}
	return services.TaildropSubscribeRPC(ctx, client, tag, onInbox)
}

// TaildropSend — отправка файлов пиру; прогресс — в onProgress.
func (ac *AppController) TaildropSend(ctx context.Context, t TailscaleTarget, tag, peerStableID string,
	files []services.TaildropOutgoing, onProgress func(fileIndex int, sent int64)) error {
	client, err := ac.taildropClient(t)
	if err != nil {
		return err
	}
	return services.TaildropSendRPC(ctx, client, tag, peerStableID, files, onProgress)
}

// TaildropDownload — файл входящих в dst.
func (ac *AppController) TaildropDownload(ctx context.Context, t TailscaleTarget, tag, name string, dst io.Writer) (int64, error) {
	client, err := ac.taildropClient(t)
	if err != nil {
		return 0, err
	}
	return services.TaildropDownloadRPC(ctx, client, tag, name, dst)
}

// TaildropDelete — удаление файла из входящих.
func (ac *AppController) TaildropDelete(t TailscaleTarget, tag, name string) error {
	client, err := ac.taildropClient(t)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), taildropCallTimeout)
	defer cancel()
	return services.TaildropDeleteRPC(ctx, client, tag, name)
}

// TaildropCancelReceiving — обрыв приёма файла.
func (ac *AppController) TaildropCancelReceiving(t TailscaleTarget, tag, senderID, name string) error {
	client, err := ac.taildropClient(t)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), taildropCallTimeout)
	defer cancel()
	return services.TaildropCancelReceivingRPC(ctx, client, tag, senderID, name)
}
