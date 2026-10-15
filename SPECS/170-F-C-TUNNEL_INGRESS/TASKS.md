# TASKS 170

## Этап 1 — контракт (1.1.126)
- [x] T1 `contract/docs/TEMPLATE_LANG.md` §6.6, §6.5.1, §6.7.5; решение D-134
- [x] T2 `contract/docs/BACKUP.md`, `contract/schema/backup.schema.json`, `contract/registry/presets.json`, `contract/registry/protocols/tailscale.json` (note), `contract/docs/NODE_SECTIONS.md`
- [x] T3 Корпус: переименование поля в `for_each_node/*`, `for_each_node_do/do_if_*`; README; новые `tunnel_ingress_mixed`, `tunnel_ingress_no_nodes`
- [x] T4 `contract/VERSION` → 1.1.126; `contract/README.md` changelog; `contract/TASKS_LXBOX.md` §123

## Этап 2 — состояние, бэкап, пайплайн
- [x] T5 `configtypes.NodeRecordFlags`; `CanonicalNode`/`GeneratedNode` → `Record`
- [x] T6 `core/state/sources_v7.go` (четыре поля + легаси на чтение), `load_normalize.go` (`normalizeNodeRecordFlags`), `adapter_source.go`
- [x] T7 `core/backup`: `backup10.go`, `export10.go`, `import10.go`, `merge.go` (`mergeRecordFlags`)
- [x] T8 Пайплайн: `outbound_generator.go` (`RecordFlags`), `canonical_emit.go`, `parsed_cache.go`, `rebuild_snapshot.go`, `build.go`, `preset_merge.go`, UI-модель и business/presentation по grep `SkipPresets`
- [x] T9 `core/template/node_loop.go`: `PresetNode.Record`, `PresetNodeRecordFields`, `NewNodeScope`
- [x] T10 `core/state/rule_order.go`: `LateDefaultPresetIDs` += `tunnel-ingress`
- [x] T11 Раннер `contract_node_loop_test.go`: четыре флага в `nodes[]`; корпус зелёный
- [x] T12 Тест `TestLoadLegacySkipPresetsKey` (состояние + импорт бэкапа со старым ключом)

## Этап 3 — шаблон, UI, доки
- [x] T13 `bin/wizard_template.json`: пресет `tunnel-ingress`; фильтр `tailscale` на новое имя; golden проверены
- [x] T14 UI: `source_skip_presets.go` (только tailscale, «Skip Tailscale presets»), новый `source_tunnel_access.go`, подключение в `source_edit_window.go`, локаль `bin/locale/ru.json`
- [x] T15 `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md`, `docs/TEMPLATE_REFERENCE*.md`, `docs/ARCHITECTURE.md` при необходимости, `IMPLEMENTATION_REPORT.md`
- [ ] T16 `go build ./...`; тесты по имени; CI `run_mode=tests` зелёный; папка → `170-F-C-…`
