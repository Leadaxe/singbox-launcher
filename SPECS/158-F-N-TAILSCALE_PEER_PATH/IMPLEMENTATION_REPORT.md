# IMPLEMENTATION_REPORT 158

## Что сделано

- `core/services/tailscale_status.go`: `TailscalePeerPath` (`""`/`direct`/`peer_relay`/`derp`), поля `Path`, `Endpoint`, `PeerRelay`, `DERPRegionCode`, `LastHandshake` у `TailscalePeer`, `Health` у `TailscaleStatus`; конвертер одного endpoint'а `TailscaleEndpointStatusFromPB` вынесен из цикла потока и общий с унарным вызовом; `TailscaleStatusRPC` (NotFound / InvalidArgument / FailedPrecondition → ok=false, Unimplemented → `ErrTailscalePathUnsupported`); неизвестный enum ядра → путь пуст.
- `TailscaleStatusNow(tag)` у `DaemonBackend` и `LxdRemoteTransport`; в `tailscaleController` и у `AppController` — команда уходит в то же ядро, чей статус на экране.
- Network: `tailscaleDeviceDetails` дописывает путь из снимка потока; подзаголовок узла в списке серверов — тот же путь после выхода (`tailscale ‣ gl-mt2500 · direct ip:port`), просьба владельца по скриншоту 07.10.
- Diagnostics: новая секция «Tailnet» (`ui/servers_node_diagnostics_tailnet.go`): опрос раз в 3 с, пока окно открыто; перерисовка только при смене строк; `health` жёлтым, Exit node с путём и возрастом хендшейка, устройства с путём, счётчик без трафика; старое ядро — одна строка и остановка опроса.
- Переводы, release notes, SPECS/README.

## Проверка

- `go build ./...` — зелёный.
- Живая проверка на tailnet — после тега `1.14.2-lx.12-rc.2` (ядро кладёт владелец). Если у пира с прямым UDP путь покажется как `relay` — баг вердикта ядра (SPEC 115), сообщить ядру.

## Не сделано / за рамками

- `Connection.tailscalePeerID` — следующая спека ядра после ресинка daemonpb.
- Путь в подписи узла списка серверов и в колонке Endpoints — не просили.
