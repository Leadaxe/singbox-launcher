# PLAN 179

1. Контракт 1.1.160: `taildrop_double_copy`, `state_dir_id` в `BACKUP.md`, схеме,
   `TASKS_LXBOX.md`, D-номер. Кейс корпуса на оба поля.
2. `core/state`, `core/backup`: поля записи, круговая передача через бэкап.
3. `core/config`: функция имени каталога (§6), `applyTailscaleStateDirectory`,
   `TailscaleBuildAdditions`, GC, удаление `RenameTailscaleStateDir`.
4. `ui/configurator`: посев `state_dir_id` у нового узла и копии, заморозка при
   переименовании; галки §3 на вкладке Settings.
5. `core/taildrop_service.go`, `core/taildrop_autosave.go`: настройки из записи
   узла, проверка отправителя §2, подписка на узлы с разрешённым Taildrop.
6. `core/taildrop_double_copy.go`: отправка на устройство из записи узла.
   `ui/servers_node_taildrop.go`: убрать пункты разрешений, «Send double copy here»
   и галку автосохранения; выбор устройства на вкладке Settings.
7. `internal/locale/settings.go`: удалить три поля. Locale, release notes,
   `docs/ARCHITECTURE.md`, `SERVICE_FILES.md`: снять «лаунчер пока не применяет».
