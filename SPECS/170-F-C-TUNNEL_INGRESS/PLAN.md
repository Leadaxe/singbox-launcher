# PLAN 170

Принцип: **поля записи узла идут по тому же пути, что `skip_presets`**, но не четырьмя параллельными map'ами, а одной структурой флагов, которую пайплайн несёт от состояния до обхода пресета.

## Этап 1 — контракт (1.1.126)

1. `contract/docs/TEMPLATE_LANG.md` §6.6 (перечень полей записи: `skip_tailscale_presets` + три `allow_*`, терпимое чтение), §6.5.1 (фильтр `tailscale`), §6.7.5 (поздний дефолт `tunnel-ingress`); решение D-134.
2. `contract/docs/BACKUP.md` (таблица полей узла), `contract/schema/backup.schema.json` (обе записи узла: переименование, deprecated-ключ `skip_presets` «читается, не пишется», три поля), `contract/registry/presets.json` (`tunnel-ingress`), `contract/registry/protocols/tailscale.json` (текст note), `contract/docs/NODE_SECTIONS.md`.
3. Корпус: переименование в `for_each_node/*.preset.json` и `for_each_node_do/do_if_*.preset.json` (поле `nodes[]`, фильтры), README корпуса (формат `nodes[]`), новые `for_each_node_do/tunnel_ingress_mixed.*` и `tunnel_ingress_no_nodes.*` по SPEC §3.
4. `contract/VERSION` → 1.1.126; `contract/README.md` changelog; `contract/TASKS_LXBOX.md` §123.

## Этап 2 — состояние, бэкап, пайплайн

5. **Структура флагов.** `core/config/configtypes/types.go`: `type NodeRecordFlags struct { SkipTailscalePresets, AllowLocal, AllowLan, AllowGateway bool }` с методом `Any() bool`; поле `SkipPresets bool` в `CanonicalNode` (~стр. 250) и в `GeneratedNode` (~стр. 743) → `Record NodeRecordFlags`.
6. `core/state/sources_v7.go` `Node`: `SkipPresets` → `SkipTailscalePresets bool json:"skip_tailscale_presets,omitempty"`, плюс `AllowLocal`, `AllowLan`, `AllowGateway` (`omitempty`); **терпимое чтение** — поле `LegacySkipPresets bool json:"skip_presets,omitempty"` только для чтения: в `core/state/load_normalize.go` (вызов из `Load`, `load_router.go`) новая `normalizeNodeRecordFlags(s)` переносит `true` в новое поле и обнуляет легаси у корневых узлов и членов папок; при записи легаси-поле всегда false → не пишется. `core/state/adapter_source.go` `canonicalNodeProjection`/проекция: флаги в `Record`, у подписки — нули.
7. `core/backup/backup10.go` `Node10`: те же четыре поля + легаси `skip_presets` на чтение; `export10.go` пишет новые; `import10.go` `decode10Source` — легаси → новое; `merge.go` (`rootBodies` ~стр. 778, `addFolderMember` ~стр. 929): «только true» для всех четырёх флагов (вынести в helper `mergeRecordFlags(dst, src *state.Node)`).
8. `core/config/outbound_generator.go`: `SkipPresetsTags map[string]bool` → `RecordFlags map[string]configtypes.NodeRecordFlags` (собирать по `node.Record.Any()`); `canonical_emit.go` (~198); `core/build/parsed_cache.go` (`SkipPresets` → `RecordFlags`), `core/rebuild_snapshot.go` (~114), `core/build/build.go` (~590: `template.PresetNode{…, Record: c.RecordFlags[tag]}`), `core/build/preset_merge.go` (комментарий/поле), `ui/configurator/models/wizard_model.go` (`GeneratedSkipPresets` → `GeneratedRecordFlags`), `ui/configurator/business/preset_nodes_view.go`, `create_config.go`, `parser.go`, `presentation/presenter_target.go` — по grep `SkipPresets`.
9. `core/template/node_loop.go`: `PresetNode.SkipPresets` → `PresetNode.Record NodeRecordFlags` (тип из `configtypes` или зеркальная структура в `template`, чтобы не тянуть зависимость — решить по текущим импортам, `template` не должен импортировать `configtypes`, если сегодня не импортирует); `PresetNodeRecordFields` = `{skip_tailscale_presets, allow_local, allow_lan, allow_gateway}`; `NewNodeScope` кладёт четыре bool.
10. `core/state/rule_order.go`: `LateDefaultPresetIDs = []string{"tailscale", "tunnel-ingress"}`.
11. Раннер `core/build/contract_node_loop_test.go`: поле `nodes[]` — четыре флага (`skip_tailscale_presets`, `allow_*`).
12. Именованный тест (единственный новый): `core/state/record_flags_legacy_test.go` `TestLoadLegacySkipPresetsKey` — `state.json` со `skip_presets: true` у корневого узла и у члена папки → после `Load` флаг в новом поле, при сохранении в файле только новый ключ; аналогично один случай импорта бэкапа со старым ключом (в том же тесте или в `core/backup`).

## Этап 3 — шаблон и UI

13. `bin/wizard_template.json`: пресет `tunnel-ingress` из SPEC §2.3 после `tailscale`; фильтр `tailscale` → `@node.skip_tailscale_presets`.
14. UI, `ui/configurator/tabs/source_skip_presets.go`: переименовать блок в «Skip Tailscale presets», условие показа + `body.type == "tailscale"`; новый файл `ui/configurator/tabs/source_tunnel_access.go` — блок `tunnelAccessBlock(m, link, node)` с тремя `widget.NewCheck` + подсказки (`locale.T`), виден у `kind: server` с `body.type` ∈ четырёх типов, правит рабочую копию узла как `skipPresetsBlock`; у `tailscale` — дополнительная строка подсказки про `advertise_*`. Подключение в `source_edit_window.go` рядом с `skipPresetsBlock` (~стр. 1394). Ключи локали + перевод в `bin/locale/ru.json`.
15. `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md`, `docs/ARCHITECTURE.md` (если поток флагов описан), `docs/TEMPLATE_REFERENCE*.md` (поле), `IMPLEMENTATION_REPORT.md`.

## Проверки

- `go build ./...`.
- По имени: `go test ./core/template -run 'TestContractCorpusTemplate' -count=1`; `go test ./core/build -run 'TestContractCorpusNodeLoops|TestWizardTemplateConfigUnchanged|TestGoldenScenarios' -count=1`; `go test ./core/state -run 'TestLoadLegacySkipPresetsKey' -count=1`.
- CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`; упавшее — `gh run view <ID> --log-failed`.
- Win7: без builtin `min`/`max`, `slices`, `maps`.

## Границы

- Файлы — только перечисленные (по grep `skip_presets|SkipPresets` — полный список в SPEC §3 и этом плане) и новые: `source_tunnel_access.go`, тест легаси-ключа, кейсы корпуса, папка SPEC.
- Не трогать: ядро, реестр протоколов кроме текста note у tailscale, `#enable`, семантику `for_each_node*`.
