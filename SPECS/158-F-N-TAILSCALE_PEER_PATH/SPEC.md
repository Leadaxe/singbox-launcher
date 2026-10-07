# SPEC 158 — Путь пира Tailscale: direct / peer relay / DERP

Статус: **N** (запрос владельца 2026-10-07).
Тип: Feature.
Связь: ядро sing-box-lx SPEC 115 TAILSCALE_PEER_PATH_STATUS (`GetTailscaleStatus`,
поля `TailscalePeer` 18–22, `TailscaleEndpointStatus.health`); SPEC 130 (стрим
статуса), SPEC 148 (вкладка Network), окно узла — вкладка Diagnostics
(d2a8220d); разбор медленного Tailscale 06.10.2026.

## 1. Проблема

Лаунчер не отвечает на вопрос «через что идёт этот пир»: напрямую по UDP,
через peer relay или через DERP. Без этого медленный tailnet разбирается
вслепую (06.10.2026: часы на догадках про роутер и DPI, а причина была в
транзите). Ядро всё это уже держит в magicsock, но стрим `SubscribeTailscaleStatus`
эти поля отбрасывал, а предупреждения `ipnstate.Status.Health` (нет связи с
DERP, exit node offline) не доходили вовсе.

## 2. Контракт ядра (SPEC 115, решение владельца)

- `TailscalePeer`: `path` (enum `NONE / DIRECT / PEER_RELAY / DERP`, вердикт
  считает ядро по правилу magicsock: `CurAddr` → DIRECT даже у idle-пира,
  иначе `PeerRelay` → PEER_RELAY, иначе писали пиру → DERP, иначе NONE;
  `active` в вердикт не входит), `endpoint` (ip:port при DIRECT), `peerRelay`,
  `derpRegionCode` (домашний DERP пира, отдаётся и при DIRECT, может быть
  пустым), `lastHandshake` (unix, 0 — хендшейка не было).
- `TailscaleEndpointStatus.health[]` — предупреждения ipnstate.
- Новый унарный `GetTailscaleStatus(endpointTag)` — свежий `Status()` на
  каждый вызов, полный ответ (self, exitNode, userGroups). Ошибки: NotFound
  (тега нет), InvalidArgument (не tailscale), FailedPrecondition (сервис или
  endpoint не запущен), Unimplemented (ядро ≤ 1.14.2-lx.12-rc.1).
- Поток `SubscribeTailscaleStatus` без изменений: событийный, поля пути в
  нём — снимок на момент последнего события. **Тика в потоке нет** («не надо
  подписчиков засорять»); частоту опроса задаёт вкладка, пока открыта.

## 3. Решение в лаунчере

1. **Домен.** `services.TailscalePeer` получает `Path` (строковый тип
   `TailscalePeerPath`: `""`, `direct`, `peer_relay`, `derp`), `Endpoint`,
   `PeerRelay`, `DERPRegionCode`, `LastHandshake`; `services.TailscaleStatus`
   — `Health []string`. Конвертер pb → домен один на поток и унарный вызов
   (`TailscaleEndpointStatusFromPB`).
2. **Запрос.** `services.TailscaleStatusRPC(ctx, client, tag)` — обёртка
   `GetTailscaleStatus` по образцу `EndpointStatusRPC` (SPEC 114): NotFound /
   InvalidArgument / FailedPrecondition → ok=false без ошибки, Unimplemented →
   `ErrTailscalePathUnsupported`, остальное — ошибка. Реализуют оба gRPC-пути:
   `DaemonBackend.TailscaleStatusNow` и `LxdRemoteTransport.TailscaleStatusNow`;
   диспетчер — `AppController.TailscaleStatusNow(target, tag)` через
   `tailscaleController` (команда уходит в то же ядро, чей статус на экране).
3. **Вкладка Network** (стрим): в хвосте строки устройства — путь из
   последнего снимка: `direct 1.2.3.4:41641` / `peer relay` / `relay fra`;
   при NONE ничего. Точка «в сети» по-прежнему по `online`, без изменений.
4. **Вкладка Diagnostics** узла Tailscale: секция «Tailnet» над проверкой
   через узел, опрос `TailscaleStatusNow` раз в 3 с, пока окно открыто (как
   таймер вкладки Network, `windowOpen`). Содержимое: предупреждения `health`
   (жёлтым), строка Exit node (имя, путь, возраст хендшейка) или «none»,
   устройства с путём ≠ NONE по строке — `● name  ip  direct 1.2.3.4:41641 ·
   handshake 36s ago`; устройства без трафика — одной строкой счётчиком.
   Со старым ядром (`ErrTailscalePathUnsupported`) — одна строка «ядро без
   статуса пути» и опрос останавливается; при ok=false — «узел не запущен»,
   опрос продолжается.
5. **Соединения** (`Connection.tailscalePeerID`) — не в этой спеке: ядро
   выпускает их следующей спекой после ресинка daemonpb.

## 4. Критерии приёмки

- A1. Network: у устройства с прямым UDP в хвосте строки `direct ip:port`;
  у устройства через DERP — `relay <код>`; у устройства без трафика хвост без
  пути.
- A2. Diagnostics узла Tailscale: секция Tailnet обновляется сама раз в 3 с;
  после закрытия окна опрос не идёт (лог демона не растёт).
- A3. Diagnostics: при `health` непустом — предупреждения жёлтым над Exit node.
- A4. Ядро ≤ lx.12-rc.1: Network без пути (как прежде), Diagnostics — одна
  строка о старом ядре, без ошибок в журнале.
- A5. Remote-машина: те же строки по её ядру; переключение главной вкладки
  на Local окно не гасит.
- A6. Classic (Clash API): вкладка Diagnostics говорит про режим демона, как
  прежде; секции Tailnet нет.
