# PLAN 176

1. **daemonpb.** Ресинк стабов с форка `v1.14.4-lx.30` (`scripts/sync_daemonpb.sh`):
   `TaildropFile.senderID`. `core/services/taildrop.go`: `TaildropFile.SenderID`.
2. **Кодек.** `core/services/taildrop_service.go` (новый): константы из
   `contract.Registry` (`registry/service_files.json`), кандидат по имени и
   размеру, разбор тела по §4 контракта, сборка имени и тела отправителя с
   отказами (пусто, больше предела, не UTF-8), одна http(s)-ссылка.
3. **Приём.** `core/taildrop_autosave.go` → общий наблюдатель входящих: ключ
   узла + флаги (автосохранение, служебные файлы); у своего ядра — все теги
   кэша статуса (`TailscaleStatusCache.Tags`). `core/taildrop_service.go`
   (новый): разрешения в settings, разбор кадра, выполнение (буфер через Fyne,
   ссылка через `platform.OpenURL`), удаление, уведомление; отправка.
4. **Настройки.** `internal/locale/settings.go`: `TaildropServicePermissions`.
5. **UI.** `ui/servers_node_network_tab.go`: пункты «Accept clipboard»,
   «Accept links», «Send text…» в меню устройства (только своё ядро);
   `ui/servers_node_taildrop.go`: окно «Send text…».
6. Тест кодека против реестра (один, data-критичный); locale + ru.json;
   release notes; контракт: `current.go` у `service_files.json`.
