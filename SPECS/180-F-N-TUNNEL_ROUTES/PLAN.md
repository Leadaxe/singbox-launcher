# PLAN 180

1. Контракт 1.1.161: `own_routes` в `TEMPLATE_LANG.md` §6.6, новый §6.5.3, строка D-144 в §9, `BACKUP.md`, схема, `registry/presets.json`, журнал версий, `DECISIONS.md`, `TASKS_LXBOX.md` §158, `go generate ./contract/...`.
2. Корпус `corpus/template/for_each_node/tunnel_routes_*`, поле `own_routes` в описании формата кейса.
3. `core/config/configtypes/types.go`: `NodeRecordFlags.OwnRoutes`, `Any`, `MergeTrue`. `core/template/node_loop.go`: `PresetNodeRecordFields`.
4. `bin/wizard_template.json`: пресет `tunnel-routes`. `core/state/rule_order.go`: `LateDefaultPresetIDs`.
5. `ui/configurator/tabs/source_tunnel_access.go`: `tunnelRoutesBlock`; вызов в `source_edit_window.go`. `bin/locale/ru.json`.
6. Тест `core/build/late_preset_dns_order_test.go`: порядок DNS-слота относительно FakeIP; правка ожиданий существующего теста под третий поздний дефолт.
7. `docs/TEMPLATE_REFERENCE.md`, `.ru.md`, `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md`.
