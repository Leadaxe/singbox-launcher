# PLAN 175

1. **Сборка.** `core/config/endpoint_schemes.go`: `applyTailscaleStateDirectory`
   ставит `taildrop_directory` = `<state_directory>/taildrop`, если ключа нет —
   и для подставленного, и для явного `state_directory`. Удалённый корень —
   разделитель `/`. `core/daemon_manager.go`: шаг (4) переносит и
   `taildrop_directory` из-под localRoot в runtimeDir.
2. **Статус.** `core/services/tailscale_status.go`: `TailscaleStatus.CanShareFiles`,
   `TailscalePeer.CanReceiveFiles` из pb.
3. **RPC.** `core/services/taildrop.go`: доменные типы входящих и функции поверх
   `daemonpb.StartedServiceClient` — подписка, отправка (окно ack не нужно:
   flow control gRPC), скачивание, удаление, отмена приёма. Один код на
   локальный демон и машину.
4. **Источник.** `taildropClient()` у `DaemonBackend`
   (`core/backend_daemon_tailscale.go`) и `LxdRemoteTransport`
   (`core/services/lxd_remote_tailscale.go`); `core/taildrop.go` —
   методы `AppController` по `TailscaleTarget`, как `tailscaleControl`.
5. **UI.** `ui/servers_node_taildrop.go` (новый): секция Files, лист отправки,
   Save в Downloads. `ui/servers_node_network_tab.go`: подписка на время окна,
   ключ перерисовки, пункт «Send file…».
6. Locale + `bin/locale/ru.json`; release notes; контракт 1.1.147.
