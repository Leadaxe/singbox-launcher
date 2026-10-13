# PLAN 166

## Этап 1 — норма и корпус (контракт 1.1.119)
1. `contract/docs/TEMPLATE_LANG.md`: §2.1 (+`unique`, `wizard_ui: internal`), §4.5.1 (канон `#filter`/`#do`/`#final`/`#append`/`#unique`/`#sort`/`#limit`), §6.5–6.7 переписать по SPEC §2; строка в таблице разрывов.
2. `contract/corpus/template/for_each/` → `for_each_node/` (кейсы на `#filter`, имя `node`), `load`-кейсы отказов; новый `for_each_node_do/` по списку SPEC §3.
3. `contract/VERSION` 1.1.119; `contract/TASKS_LXBOX.md` §116; `contract/registry/warnings.json` — причина `internal` у `backup_var_skipped`, если причины реестровые.

## Этап 2 — движок (`core/template`, `core/build`)
4. `preset_types.go`: `ForEach *PresetForEach` → `ForEachNode *NodeLoop` (`json:"for_each_node"`) и `ForEachNodeDo *NodeLoopDo` (`json:"for_each_node_do"`); `UnmarshalJSON` — сырые `dns_servers` для обоих; `for_each` в JSON → ошибка загрузки.
5. `for_each.go` → `node_loop.go`: `NodeLoop{Filter}`, `NodeLoopDo{Filter, Do map[string]DoOp, Final map[string]FinalOps}`; `Valid()`/`validate` по SPEC §2.1–2.4; `EvalFilter` без изменений; `NewNodeScope("node", n)`.
6. `template_validate.go` / `loader.go`: load-отказы (оба ключа, `node_type`, `as`, `#do` не там, незнакомый глагол, тип переменной, `#final` без `#do`, резерв имени `node`, `unique` не у `text_list`, `wizard_ui: internal`).
7. `preset_expand.go`: `ExpandPresetForNodes` — ветка `ForEachNodeDo`: обход → `#append` в копию `varsMap` (text_list как строка построчно, §2.2 нормы) → `#final` → `unique` → `expandPresetBody(…, nil)` один раз; `selectForEachNodes` без `node_type`; `PresetForEachNodes` для обоих ключей.
8. Dropped: правило без условий после подстановки — `logical_sub_rule_conditions_lost_dropped` для тела `for_each_node_do` (сегодня код даётся в ветке `for_each`; вынести в общую проверку фрагмента).
9. `core/config/varsubst.go` (`loadStateSettingsVars`, `coerceVarValue`) и чтение `vars` пресета: `internal` → значение из состояния игнорируется; `unique` → нормализация при чтении.
10. Бэкап `core/backup`: экспорт пропускает `internal`; импорт снимает с `backup_var_skipped{reason: internal}`.
11. Раннер корпуса `core/template/contract_template_test.go`: поддержка разделов `for_each_node/` и `for_each_node_do/` (если раздел `for_each/` захардкожен — переименовать).

## Этап 3 — шаблон и UI
12. `bin/wizard_template.json`: Tailscale-пресет на `for_each_node` + `#filter`; новый пресет `node-domains-fresh-dns` (SPEC §4); `num` по соседям.
13. UI: `ui/configurator/tabs/source_skip_presets.go` (через `PresetForEachNodes`), `preset_ref_edit_dialog.go`, `dns_preset_bundled.go`, `business/preset_nodes_view.go`, `presentation/presenter_async.go` (`templateHasForEachPreset`) — на новые поля; форма пресета: `unique` при сохранении, `internal` не рендерится; `bin/locale/ru.json` для новых строк.
14. `docs/release_notes/upcoming.md` (EN/RU), `docs/ARCHITECTURE.md` при изменении потоков; `IMPLEMENTATION_REPORT.md`.

## Проверки
- `go build ./...`; раннер корпуса по имени: `go test ./core/template -run 'TestContractCorpus' -count=1`.
- Полный прогон — CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`.
- Живая проверка: пересборка профиля с узлами на доменах → в built config одно DNS-правило с массивом их доменов.
