package services

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
)

// Расширения LxdRemoteTransport для Debug API (SPEC 100): доступ к
// gRPC-соединению для raw-passthrough, обрыв всех соединений и подписка на
// лог ядра. Отдельным файлом, чтобы не смешивать API-поверхность с
// ProxyTransport-обвязкой вкладки Servers.

// GRPCConn возвращает живое gRPC-соединение транспорта (ленивый dial, как у
// всех RPC транспорта). Нужен raw-gRPC passthrough'у: он говорит с демоном
// напрямую через protoregistry-резолв, минуя типизированных клиентов.
//
// Соединением владеет транспорт — вызывающий НЕ должен его закрывать.
func (t *LxdRemoteTransport) GRPCConn() (*grpc.ClientConn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		conn, err := t.client.DialGRPC()
		if err != nil {
			return nil, fmt.Errorf("lxd remote: dial %s: %w", t.client.AddrString(), err)
		}
		t.conn = conn
	}
	return t.conn, nil
}

// CloseAllConnections обрывает все соединения ядра машины.
func (t *LxdRemoteTransport) CloseAllConnections() error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	if _, err := client.CloseAllConnections(ctx, &emptypb.Empty{}); err != nil {
		return fmt.Errorf("lxd remote CloseAllConnections: %w", err)
	}
	return nil
}

// LogLine — одна строка лога ядра машины.
type LogLine struct {
	Level   string
	Message string
}

// SubscribeLogLines открывает стрим лога ядра машины (gRPC SubscribeLog).
//
// onLine зовётся по одной строке; reset-кадр (ядро перезапустилось, буфер
// демона очищен) прокидывается как onReset — вызывающий решает, чистить ли
// накопленное. Возвращает cancel; вызывающий обязан его позвать.
func (t *LxdRemoteTransport) SubscribeLogLines(onLine func(LogLine), onReset func()) (cancel func(), err error) {
	conn, err := t.GRPCConn()
	if err != nil {
		return nil, err
	}
	ctx, cancelCtx := context.WithCancel(context.Background())
	stream, err := daemonpb.NewStartedServiceClient(conn).SubscribeLog(ctx, &emptypb.Empty{})
	if err != nil {
		cancelCtx()
		return nil, fmt.Errorf("lxd remote SubscribeLog: %w", err)
	}
	go func() {
		for {
			batch, recvErr := stream.Recv()
			if recvErr != nil {
				debuglog.DebugLog("lxd remote log: stream closed: %v", recvErr)
				return
			}
			if batch.GetReset_() && onReset != nil {
				onReset()
			}
			for _, m := range batch.GetMessages() {
				onLine(LogLine{Level: m.GetLevel().String(), Message: m.GetMessage()})
			}
		}
	}()
	return cancelCtx, nil
}
