# PLAN 177

1. `internal/platform/clipboard_watch_{darwin,windows,other}.go`: счётчик
   изменений буфера и пометка чувствительности.
2. `internal/locale/settings.go`: `TaildropDoubleCopy`.
3. `core/taildrop_double_copy.go`: цикл жеста (`startTaildropDoubleCopy`,
   запуск в `controller.go`), отправка через `TaildropSendText`, уведомления.
4. `ui/servers_node_taildrop.go` + `servers_node_network_tab.go`: галка в меню.
5. Locale, release notes, контракт 1.1.156 (§7 п.3 — жест без показа).
