//go:build darwin

package core

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"singbox-launcher/core/services"
	daemonpb "singbox-launcher/internal/daemonpb"
)

// grpcStreamStub закрывает обязательную часть grpc.ClientStream, которая в
// этих тестах не используется: нас интересует только Recv.
type grpcStreamStub struct{}

func (grpcStreamStub) Header() (metadata.MD, error) { return nil, nil }
func (grpcStreamStub) Trailer() metadata.MD         { return nil }
func (grpcStreamStub) CloseSend() error             { return nil }
func (grpcStreamStub) Context() context.Context     { return context.Background() }
func (grpcStreamStub) SendMsg(any) error            { return nil }
func (grpcStreamStub) RecvMsg(any) error            { return nil }

// fakeDNSStream отдаёт заготовленные события, затем ошибку обрыва.
type fakeDNSStream struct {
	grpcStreamStub
	events []*daemonpb.DnsQueryEvent
	err    error
	i      int
}

func (f *fakeDNSStream) Recv() (*daemonpb.DnsQueryEvent, error) {
	if f.i < len(f.events) {
		ev := f.events[f.i]
		f.i++
		return ev, nil
	}
	return nil, f.err
}

// Unimplemented обязан отличаться от «демон лежит»: первое приходит от ядра
// без with_lx_command и повтором не лечится, поэтому вызывающий по нему
// откатывает профайлер на разбор лога.
func TestIsUnimplemented(t *testing.T) {
	if !isUnimplemented(status.Error(codes.Unimplemented, "no lx")) {
		t.Fatal("Unimplemented не распознан")
	}
	if isUnimplemented(status.Error(codes.Unavailable, "daemon down")) {
		t.Fatal("Unavailable принят за Unimplemented — переподписки не будет")
	}
	if isUnimplemented(errors.New("boom")) {
		t.Fatal("обычная ошибка принята за Unimplemented")
	}
}

// consumeDNSStream отдаёт ошибку обрыва наверх: без неё Unimplemented,
// пришедший в первом Recv (обычный случай для gRPC), выглядел бы как рядовой
// разрыв и супервизор вечно переподписывался бы к ядру, где RPC нет.
func TestConsumeDNSStreamReportsRecvError(t *testing.T) {
	b := &DaemonBackend{}
	unimpl := status.Error(codes.Unimplemented, "no lx")
	stream := &fakeDNSStream{err: unimpl}

	got, err := b.consumeDNSStream(stream, make(chan struct{}), func(services.DNSQuery) {})
	if got {
		t.Fatal("событий не было, а received=true — backoff сбросился бы зря")
	}
	if !isUnimplemented(err) {
		t.Fatalf("ошибка обрыва потеряна: %v", err)
	}
}

// stop должен прекращать чтение: подписка снимается при смене backend, и
// переживший её стрим держал бы ядро в режиме эмита (гейт — HasSubscribers).
func TestConsumeDNSStreamStops(t *testing.T) {
	b := &DaemonBackend{}
	stop := make(chan struct{})
	close(stop)
	stream := &fakeDNSStream{
		events: []*daemonpb.DnsQueryEvent{{Domain: "a.test"}},
		err:    errors.New("should not be reached"),
	}
	delivered := 0
	got, err := b.consumeDNSStream(stream, stop, func(services.DNSQuery) { delivered++ })
	if delivered != 0 || got || err != nil {
		t.Fatalf("чтение продолжилось после stop: delivered=%d got=%v err=%v", delivered, got, err)
	}
}
