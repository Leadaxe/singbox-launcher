//go:build darwin

package core

import (
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"singbox-launcher/core/services"
	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
)

// isUnimplemented — ядро собрано без with_lx_command: RPC объявлен в proto, но
// реализации нет. Отличать это от «демон лежит» обязательно: первое повтором
// не лечится, второе лечится переподпиской.
func isUnimplemented(err error) bool {
	return status.Code(err) == codes.Unimplemented
}

// DNS-плоскость daemon-режима: подписка на gRPC SubscribeDNSQueries (SPEC 018
// форка). Структурный поток вместо разбора sing-box.log — приходят сервер,
// ответы в wire order (CNAME-цепочка целиком), процесс и признак отказа.
//
// Почему подписка «по требованию», а не как у соединений (постоянный стрим):
// в ядре гейт эмита — HasSubscribers(). Нет подписчика — dnstrack не делает
// ничего; есть — события идут независимо от log.level. Держать стрим открытым
// всё время работы лаунчера значило бы заставлять ядро вечно собирать данные,
// которые никто не смотрит. Поэтому стрим поднимается, когда профайлер просит,
// и закрывается вместе с ним.

// dnsSubscription — активная подписка одного потребителя.
type dnsSubscription struct {
	mu     sync.Mutex
	cancel func()
}

// SubscribeDNSQueries открывает стрим DNS-запросов локального ядра.
//
// includeAnswers=true обязателен: без ответов не восстановить CNAME-цепочку,
// а именно она отвечает на вопрос «куда на самом деле ушёл домен».
//
// Стрим переподнимается с backoff, пока не вызван cancel: ядро могло ещё не
// стартовать в момент подписки, а профайлер живёт дольше отдельного запуска
// VPN. Поэтому недоступность демона прямо сейчас — не ошибка подписки: ждать
// его и есть работа супервизора. Отказ здесь означал бы, что профайлер,
// открытый до старта VPN, навсегда остался бы без DNS-плоскости.
func (b *DaemonBackend) SubscribeDNSQueries(onQuery func(services.DNSQuery)) (cancel func(), err error) {
	return b.subscribeDNSQueries(onQuery, nil)
}

// subscribeDNSQueries — SubscribeDNSQueries с колбэком onUnsupported: он
// срабатывает, если ядро отвечает Unimplemented (собрано без with_lx_command).
// Подписчик обязан на это откатиться к прежнему источнику — иначе DNS-плоскость
// исчезнет совсем: лог он к этому моменту уже подавил, а стрим не придёт.
func (b *DaemonBackend) subscribeDNSQueries(
	onQuery func(services.DNSQuery),
	onUnsupported func(),
) (cancel func(), err error) {
	if onQuery == nil {
		return func() {}, nil
	}
	sub := &dnsSubscription{}
	stop := make(chan struct{})
	sub.cancel = func() { close(stop) }
	go b.superviseDNS(stop, onQuery, onUnsupported)
	return func() {
		sub.mu.Lock()
		defer sub.mu.Unlock()
		if sub.cancel != nil {
			sub.cancel()
			sub.cancel = nil
		}
	}, nil
}

// superviseDNS держит подписку до stop/выключения backend'а; reconnect с
// backoff, как у status/logs/connections (сброс backoff — только после реально
// полученного события: Subscribe* у grpc-go «успешен» даже при лежащем демоне,
// ошибка всплывает в первом Recv).
func (b *DaemonBackend) superviseDNS(
	stop <-chan struct{},
	onQuery func(services.DNSQuery),
	onUnsupported func(),
) {
	backoff := time.Second
	for {
		select {
		case <-stop:
			return
		case <-b.ctx.Done():
			return
		default:
		}
		client, err := b.grpcClient()
		if err == nil {
			var stream grpc.ServerStreamingClient[daemonpb.DnsQueryEvent]
			stream, err = client.SubscribeDNSQueries(b.ctx,
				&daemonpb.SubscribeDNSQueriesRequest{IncludeAnswers: true})
			if err == nil {
				// Subscribe* у grpc-go «успешен» даже при лежащем демоне —
				// и Unimplemented тоже всплывает только в первом Recv.
				var recvErr error
				var got bool
				got, recvErr = b.consumeDNSStream(stream, stop, onQuery)
				if got {
					backoff = time.Second
				}
				if recvErr != nil {
					err = recvErr
				}
			}
		}
		if err != nil {
			// Ядро без with_lx_command отвечает Unimplemented — честный отказ,
			// а не тишина. Повторять подписку смысла нет: пересборкой ядра
			// это не лечится на ходу.
			if isUnimplemented(err) {
				debuglog.WarnLog("daemon.dns: core without SubscribeDNSQueries: %v", err)
				if onUnsupported != nil {
					onUnsupported()
				}
				return
			}
			debuglog.DebugLog("daemon.dns: stream unavailable: %v", err)
		}
		select {
		case <-stop:
			return
		case <-b.ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

// consumeDNSStream читает стрим до обрыва. Возвращает признак «пришло хотя бы
// одно событие» (для сброса backoff) и ошибку обрыва — по ней вызывающий
// отличает отсутствие RPC в ядре от временной недоступности демона.
func (b *DaemonBackend) consumeDNSStream(
	stream grpc.ServerStreamingClient[daemonpb.DnsQueryEvent],
	stop <-chan struct{},
	onQuery func(services.DNSQuery),
) (bool, error) {
	received := false
	for {
		select {
		case <-stop:
			return received, nil
		default:
		}
		ev, err := stream.Recv()
		if err != nil {
			debuglog.DebugLog("daemon.dns: stream closed: %v", err)
			return received, err
		}
		received = true
		if !b.isActive() {
			continue
		}
		onQuery(services.DNSQueryFromProto(ev))
	}
}

// dnsQuerySourceAny реализует dnsQuerySource: подписка как any-функция, чтобы
// core/backend.go не тянул зависимость на конкретный тип колбэка. UI приводит
// обратно.
func (b *DaemonBackend) dnsQuerySourceAny() any {
	return b.subscribeDNSQueries
}
