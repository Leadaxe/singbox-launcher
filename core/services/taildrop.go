// File taildrop.go — файлы Taildrop узла tailnet (SPEC 175): входящие,
// отправка, скачивание, удаление, отмена приёма. RPC StartedService ядра;
// один код на локальный демон и удалённую машину — оба отдают
// daemonpb.StartedServiceClient (как TailscaleStatusRPC).
//
// Имена файлов и устройств в журнал не пишутся (SPEC 148 §3): ошибки несут
// только тег узла.
package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// TaildropChunkSize — порция отправки; та же, что у ядра
// (daemon.TaildropChunkSize): меньше предела сообщения gRPC с запасом.
const TaildropChunkSize = 16*1024 - 64

// TaildropFile — принятый файл во входящих узла.
type TaildropFile struct {
	Name       string
	Size       int64
	SenderName string
	// ModifiedAt — время приёма по часам этой машины (mtime файла у ядра).
	ModifiedAt time.Time
}

// TaildropReceiving — файл, который принимается сейчас.
type TaildropReceiving struct {
	Name          string
	Size          int64
	ReceivedBytes int64
	SenderID      string
	SenderName    string
}

// TaildropInbox — входящие узла: принятые и принимаемые.
type TaildropInbox struct {
	Files     []TaildropFile
	Receiving []TaildropReceiving
}

// TaildropOutgoing — файл к отправке. Size −1 — размер неизвестен.
type TaildropOutgoing struct {
	Name string
	Size int64
	Body io.Reader
}

// errTaildropUnsupported — ядро без RPC Taildrop.
var errTaildropUnsupported = errors.New("core does not support Taildrop (needs sing-box-lx 1.14.0-lx.27)")

// taildropErr — ошибка RPC человеческим текстом: сообщение статуса без
// обёртки gRPC.
func taildropErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok {
		if st.Code() == codes.Unimplemented {
			return errTaildropUnsupported
		}
		return fmt.Errorf("%s: %s", op, st.Message())
	}
	return fmt.Errorf("%s: %w", op, err)
}

func taildropInboxFromPB(in *daemonpb.TaildropInbox) TaildropInbox {
	var out TaildropInbox
	for _, f := range in.GetFiles() {
		file := TaildropFile{Name: f.GetName(), Size: f.GetSize(), SenderName: f.GetSenderName()}
		if at := f.GetModifiedAt(); at > 0 {
			file.ModifiedAt = time.Unix(at, 0)
		}
		out.Files = append(out.Files, file)
	}
	for _, r := range in.GetReceiving() {
		out.Receiving = append(out.Receiving, TaildropReceiving{
			Name: r.GetName(), Size: r.GetSize(), ReceivedBytes: r.GetReceivedBytes(),
			SenderID: r.GetSenderID(), SenderName: r.GetSenderName(),
		})
	}
	return out
}

// TaildropSubscribeRPC — входящие узла: каждый кадр в onInbox, до отмены ctx
// или обрыва стрима. Первый кадр приходит сразу после подписки; узла нет —
// ядро шлёт пустой кадр и ждёт.
func TaildropSubscribeRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag string, onInbox func(TaildropInbox)) error {
	stream, err := client.SubscribeTaildropInbox(ctx, &daemonpb.SubscribeTaildropInboxRequest{EndpointTag: tag})
	if err != nil {
		return taildropErr("taildrop inbox", err)
	}
	for {
		in, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return taildropErr("taildrop inbox", err)
		}
		onInbox(taildropInboxFromPB(in))
	}
}

// TaildropSendRPC отправляет файлы пиру одним стримом. onProgress —
// отправлено байт текущего файла (ядро шлёт не чаще 200 мс). Отмена ctx
// обрывает отправку; ядро удаляет недошедший файл у получателя само.
func TaildropSendRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag, peerStableID string,
	files []TaildropOutgoing, onProgress func(fileIndex int, sent int64)) error {
	if len(files) == 0 {
		return errors.New("taildrop send: no files")
	}
	parent := ctx
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stream, err := client.SendTaildropFiles(ctx)
	if err != nil {
		return taildropErr("taildrop send", err)
	}
	start := &daemonpb.TaildropSendStart{EndpointTag: tag, PeerStableID: peerStableID}
	for _, f := range files {
		start.Files = append(start.Files, &daemonpb.TaildropOutgoingFile{Name: f.Name, Size: f.Size})
	}
	// Ответы читаются параллельно: ядро шлёт прогресс по ходу, а статус
	// ошибки grpc-go отдаёт только через Recv (Send на оборванном стриме
	// возвращает голый io.EOF).
	done := make(chan error, 1)
	go func() {
		for {
			msg, rerr := stream.Recv()
			if rerr != nil {
				if errors.Is(rerr, io.EOF) {
					rerr = nil
				}
				done <- rerr
				return
			}
			if p := msg.GetProgress(); p != nil && onProgress != nil {
				onProgress(int(p.GetFileIndex()), p.GetSentBytes())
			}
		}
	}()
	sendErr := func() error {
		if err := stream.Send(&daemonpb.TaildropSendClientMessage{
			Message: &daemonpb.TaildropSendClientMessage_Start{Start: start},
		}); err != nil {
			return err
		}
		buf := make([]byte, TaildropChunkSize)
		for _, f := range files {
			for {
				n, rerr := f.Body.Read(buf)
				if n > 0 {
					chunk := append([]byte(nil), buf[:n]...)
					if err := stream.Send(&daemonpb.TaildropSendClientMessage{
						Message: &daemonpb.TaildropSendClientMessage_Chunk{Chunk: &daemonpb.TaildropFileChunk{Data: chunk}},
					}); err != nil {
						return err
					}
				}
				if errors.Is(rerr, io.EOF) {
					break
				}
				if rerr != nil {
					cancel()
					return fmt.Errorf("taildrop send: read file: %w", rerr)
				}
			}
			if err := stream.Send(&daemonpb.TaildropSendClientMessage{
				Message: &daemonpb.TaildropSendClientMessage_FileDone{FileDone: &daemonpb.TaildropFileDone{}},
			}); err != nil {
				return err
			}
		}
		return stream.CloseSend()
	}()
	recvErr := <-done
	if err := parent.Err(); err != nil {
		return err
	}
	// Ошибка чтения файла важнее статуса стрима: стрим оборван из-за неё.
	if sendErr != nil && !errors.Is(sendErr, io.EOF) {
		return sendErr
	}
	return taildropErr("taildrop send", recvErr)
}

// TaildropDownloadRPC пишет файл входящих в dst; возвращает число байт.
func TaildropDownloadRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag, name string, dst io.Writer) (int64, error) {
	stream, err := client.DownloadTaildropFile(ctx, &daemonpb.DownloadTaildropFileRequest{EndpointTag: tag, Name: name})
	if err != nil {
		return 0, taildropErr("taildrop download", err)
	}
	var (
		written int64
		size    int64 = -1
	)
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return written, taildropErr("taildrop download", err)
		}
		if data := chunk.GetData(); len(data) > 0 {
			n, werr := dst.Write(data)
			written += int64(n)
			if werr != nil {
				return written, fmt.Errorf("taildrop download: write: %w", werr)
			}
		} else if size < 0 {
			size = chunk.GetSize()
		}
	}
	if size >= 0 && written != size {
		return written, fmt.Errorf("taildrop download: got %d of %d bytes", written, size)
	}
	return written, nil
}

// TaildropDeleteRPC удаляет файл из входящих.
func TaildropDeleteRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag, name string) error {
	_, err := client.DeleteTaildropFile(ctx, &daemonpb.DeleteTaildropFileRequest{EndpointTag: tag, Name: name})
	return taildropErr("taildrop delete", err)
}

// TaildropCancelReceivingRPC обрывает приём файла от отправителя.
func TaildropCancelReceivingRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag, senderID, name string) error {
	_, err := client.CancelTaildropReceiving(ctx, &daemonpb.CancelTaildropReceivingRequest{
		EndpointTag: tag, SenderID: senderID, Name: name,
	})
	return taildropErr("taildrop cancel", err)
}
