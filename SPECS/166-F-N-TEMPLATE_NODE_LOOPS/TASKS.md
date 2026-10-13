# TASKS 166

## Этап 1 — контракт
- [x] T1 `contract/docs/TEMPLATE_LANG.md` §2.1, §4.5.1, §6.5–6.7, таблица разрывов
- [x] T2 `contract/corpus/template/for_each/` → `for_each_node/` + load-кейсы отказов
- [x] T3 `contract/corpus/template/for_each_node_do/` — кейсы по SPEC §3
- [x] T4 `contract/VERSION` → 1.1.119, `contract/TASKS_LXBOX.md` §116, `contract/registry/warnings.json` (причина `internal`)

## Этап 2 — движок
- [x] T5 `core/template/preset_types.go` — поля `ForEachNode`, `ForEachNodeDo`, отказ `for_each`
- [x] T6 `core/template/for_each.go` → `node_loop.go` — типы, валидация, scope `node`
- [x] T7 `core/template/template_validate.go`, `loader.go` — load-отказы, резерв `node`, `unique`, `internal`
- [x] T8 `core/build/preset_expand.go` — ветка `for_each_node_do`: `#do`/`#final`/`unique`, тело один раз
- [x] T9 `core/build` — `logical_sub_rule_conditions_lost_dropped` для тела `for_each_node_do`
- [x] T10 сборка (`core/build/preset_expand.go` presetVarsMapWithGlobals, `core/template/vars_resolve.go`; varsubst.go — только глобали parser_config, не тронут) — `internal` игнорирует состояние, `unique` нормализует
- [x] T11 `core/backup` — экспорт/импорт `internal` (`backup_var_skipped{reason: internal}`)
- [x] T12 раннер корпуса — `core/build/contract_node_loop_test.go` (бывший `contract_for_each_test.go`), разделы `for_each_node/`, `for_each_node_do/`

## Этап 3 — шаблон, UI, доки
- [ ] T13 `bin/wizard_template.json` — Tailscale на `for_each_node`, пресет `node-domains-fresh-dns` (Tailscale переведён вместе с движком; пресет — волна UI)
- [ ] T14 UI: `source_skip_presets.go`, `preset_ref_edit_dialog.go`, `dns_preset_bundled.go`, `business/preset_nodes_view.go`, `presentation/presenter_async.go`, форма пресета (`unique`, `internal`), `bin/locale/ru.json`
- [ ] T15 `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md` (если нужно), `IMPLEMENTATION_REPORT.md`
- [ ] T16 `go build ./...`, раннер корпуса по имени, CI `run_mode=tests`; папка → `166-F-C-…`
