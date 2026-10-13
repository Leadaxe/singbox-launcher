# TASKS 166

## Этап 1 — контракт
- [ ] T1 `contract/docs/TEMPLATE_LANG.md` §2.1, §4.5.1, §6.5–6.7, таблица разрывов
- [ ] T2 `contract/corpus/template/for_each/` → `for_each_node/` + load-кейсы отказов
- [ ] T3 `contract/corpus/template/for_each_node_do/` — кейсы по SPEC §3
- [ ] T4 `contract/VERSION` → 1.1.119, `contract/TASKS_LXBOX.md` §116, `contract/registry/warnings.json` (причина `internal`)

## Этап 2 — движок
- [ ] T5 `core/template/preset_types.go` — поля `ForEachNode`, `ForEachNodeDo`, отказ `for_each`
- [ ] T6 `core/template/for_each.go` → `node_loop.go` — типы, валидация, scope `node`
- [ ] T7 `core/template/template_validate.go`, `loader.go` — load-отказы, резерв `node`, `unique`, `internal`
- [ ] T8 `core/build/preset_expand.go` — ветка `for_each_node_do`: `#do`/`#final`/`unique`, тело один раз
- [ ] T9 `core/build` — `logical_sub_rule_conditions_lost_dropped` для тела `for_each_node_do`
- [ ] T10 `core/config/varsubst.go` — `internal` игнорирует состояние, `unique` нормализует
- [ ] T11 `core/backup` — экспорт/импорт `internal` (`backup_var_skipped{reason: internal}`)
- [ ] T12 `core/template/contract_template_test.go` — разделы корпуса

## Этап 3 — шаблон, UI, доки
- [ ] T13 `bin/wizard_template.json` — Tailscale на `for_each_node`, пресет `node-domains-fresh-dns`
- [ ] T14 UI: `source_skip_presets.go`, `preset_ref_edit_dialog.go`, `dns_preset_bundled.go`, `business/preset_nodes_view.go`, `presentation/presenter_async.go`, форма пресета (`unique`, `internal`), `bin/locale/ru.json`
- [ ] T15 `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md` (если нужно), `IMPLEMENTATION_REPORT.md`
- [ ] T16 `go build ./...`, раннер корпуса по имени, CI `run_mode=tests`; папка → `166-F-C-…`
