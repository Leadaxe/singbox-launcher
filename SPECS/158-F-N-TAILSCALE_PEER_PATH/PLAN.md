# PLAN 158

## Файлы

| Файл | Изменение |
|------|-----------|
| `core/services/tailscale_status.go` | `TailscalePeerPath` + константы; поля пути и `LastHandshake` у `TailscalePeer`, `Health` у `TailscaleStatus`; `TailscaleEndpointStatusFromPB` (общий конвертер потока и унарного вызова); `TailscaleStatusRPC`, `ErrTailscalePathUnsupported` |
| `core/backend_daemon_tailscale.go` | `DaemonBackend.TailscaleStatusNow` |
| `core/services/lxd_remote_tailscale.go` | `LxdRemoteTransport.TailscaleStatusNow` |
| `core/tailscale_status.go` | `TailscaleStatusNow` в `tailscaleController` и у `AppController` |
| `ui/servers_node_info_tailscale.go` | `tailscalePathText` — строка пути по таблице CONSUMERS ядра |
| `ui/servers_node_network_tab.go` | путь в хвосте строки устройства (`tailscaleDeviceDetails`) |
| `ui/servers_node_diagnostics_tailnet.go` | новый: секция Tailnet с опросом раз в 3 с |
| `ui/servers_node_diagnostics_tab.go` | вызов секции для узла Tailscale |
| `bin/locale/ru.json` | переводы |
| `docs/release_notes/upcoming.md`, `SPECS/README.md` | заметки |

## Что не трогаем

- `internal/daemonpb` — ресинк на dc7b00fd уже сделан другой сессией вместе
  с переделкой WG-пиров на `GetWireGuardStatus`; `GetTailscaleStatus` там есть.
- Поток и кеш `TailscaleStatusCache`: правила хранения те же, только новые
  поля в конвертере.
- Тесты: UI-правка и обёртка RPC по готовому образцу; полный прогон в CI.
