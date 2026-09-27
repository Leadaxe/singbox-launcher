# SPEC 145 — Пресет Tailscale: `for_each`, `@node`, `#tpl`, `skip_presets`

Статус: **C** (решение владельца 2026-09-27, реализовано 2026-09-27).
Коммиты: `2a03c85a` (контракт 1.1.86, пресет и `for_each`), `995a0e3b` (визард: узлы `for_each` тем же отбором, что сборка).
Тип: Feature. Контракт: бамп `contract/VERSION` → **1.1.86**, параграф **§83** в `contract/TASKS_LXBOX.md`.

Источник решения — спека LxBox §578
(`LxBox/docs/spec/tasks/578-tailscale-preset-template-for-each.md`),
образец поведения — реализация LxBox, коммит `b0176c93`. Здесь — только то,
что касается лаунчера. Норма обеих сторон — `contract/docs/TEMPLATE_LANG.md`
§4.8, §6.5–§6.7, поле записи — `contract/docs/BACKUP.md`.

## 1. Проблема

После SPEC 144 узел Tailscale собирается без маршрута в tailnet и без
MagicDNS: связку раньше несли секции узла.

## 2. Решение

- Язык шаблона: `{"#tpl": "…@{имя}…"}`, `for_each {node_type, as, filter?}` у
  пресета, имена узла `@<as>`, `@<as>.skip_presets`, `@<as>.body.<путь>`.
- Пресет `tailscale` в `bin/wizard_template.json` (`num 945`,
  `default_enabled`): на каждый узел Tailscale конфига — DNS-сервер
  `<тег>-dns`, DNS-правило и два правила маршрута через `preferred_by`.
- Поле записи `skip_presets` (свой сервер и член папки), пишется только `true`.
- Переключатель `Skip presets` в форме узла-сервера.

## 3. Реализация

| Что | Где |
|---|---|
| `#tpl`, пространство имён узла в обходчике, `ResolvedVar.Raw` | `core/template/for_each.go`, `substitute_canon.go`, `substitute.go` |
| `for_each` у пресета; `dns_servers` такого пресета читаются сырыми (`ForEachDNSServers`) — typed `PresetDNSServer` с `tag` строкой их не читает | `core/template/preset_types.go`, `preset_loader.go` |
| загрузка: `#tpl` с лишним ключом — отказ; вставки `@{…}` — объявленные имена | `core/template/template_validate.go` |
| раскрытие по узлам, теги без `<preset_id>:` | `core/build/preset_expand.go` (`ExpandPresetForNodes`) |
| узлы для `for_each` — из кэша сборки после граф-санитайзера, порядок секций шаблона | `core/build/build.go` (`collectPresetNodes`) |
| DNS-серверы пресета с `for_each` | `core/build/resolve_dns.go` (шаг 3a') |
| `skip_presets`: запись → канон → узел парсера → `ParsedCache.SkipPresets`; у подписки всегда ложь | `core/state/sources_v7.go`, `adapter_source.go`, `core/config/*`, `core/rebuild_snapshot.go` |
| бэкап: поле, импорт только `true` | `core/backup/backup10.go`, `export10.go`, `import10.go`, `merge.go` |
| переключатель формы | `ui/configurator/tabs/source_skip_presets.go` |

Debug API: `GET /state/full` отдаёт `skip_presets` в записи узла (поле модели).

## 4. Появление пресета у существующих пользователей

`state.SeedLateDefaultRules` + список `state.LateDefaultPresetIDs`
(`{tailscale}`, только растёт) + отметка `meta.late_presets_seeded` в
`state.json`:

- id, которого нет в отметке и который шаблон объявляет, отмечается; если он
  `default_enabled` и правила с таким ref нет, правило добавляется включённым
  на номер шаблона;
- визард (`restorePresetRefs`) сеет при загрузке состояния и сохраняет отметку
  вместе с состоянием: удалённый пользователем пресет не возвращается;
- сборка (`core/config_service_context.go`) сеет в памяти по отметке
  состояния: пользователь, не открывавший визард, получает пресет сразу.

## 5. Проверка

- `go test ./core/build -run 'TestContractCorpusForEach|TestTailscalePresetBuild|TestWizardTemplateConfigUnchanged'`
- `go test ./core/template -run TestContractCorpusTemplate`
- `go test ./core/backup -run 'TestSkipPresetsRoundTrip|TestExport10EntityKeysAreDeclared'`

## 6. Не сделано

- Вкладка JSON редактора пресета (`buildPresetJSONPreview`) раскрывает пресет
  с `for_each` без узлов: фрагменты пусты.

## 7. Интерфейс визарда (доделка, 2026-09-27)

Список узлов для `for_each` в визарде строится ТОЙ ЖЕ функцией, что в сборке:
`build.CollectPresetNodes` (экспортирована из `core/build/build.go`), отбор
пресетом (`node_type`, `filter`) — `build.PresetForEachNodes`, общий с
`ExpandPresetForNodes` (`selectForEachNodes` в `core/build/preset_expand.go`).

| Что | Где |
|---|---|
| превью знает `skip_presets`: `model.GeneratedSkipPresets` из `result.SkipPresetsTags` эмиссии, кэш превью (`inMemoryCacheFromModel`) несёт карту так же, как кэш боевой сборки (`rebuild_snapshot`) | `ui/configurator/models/wizard_model.go`, `business/parser.go`, `business/create_config.go`, `presentation/presenter_target.go` |
| узлы и раскрытие для экранов: `PresetNodesForView`, `ExpandPresetForView`, `PresetServedTags`, `PresetServedNodesLabel` | `ui/configurator/business/preset_nodes_view.go` |
| серверы пресета с `for_each` в списке DNS-серверов (`ResolveDNS` получает `PresetNodes`) и в пикерах резолверов (`PresetBundledDNSTags`) | `tabs/dns_preset_bundled.go`, `business/preset_bundled_dns.go` |
| строка пресета на вкладках Rules и DNS: `<метка> · <теги через запятую>`, узлов нет — `No matching nodes` | `tabs/rules_unified_rows.go`, `tabs/dns_unified_rules.go` |
| DNS-правила пресета на вкладке DNS раскрываются по тем же узлам | `tabs/dns_unified_rules.go`, `tabs/dns_user_rules.go` |
| после разбора визард перерисовывает Rules и DNS, если в шаблоне есть пресет с `for_each`; вход на вкладку DNS тоже запускает разбор | `presentation/presenter_async.go`, `ui/configurator/configurator.go` |

Отбор в визарде: узел попадает в список, если эмиссия его выпустила
(выключенный узел, выключенный источник, отметка подписки и гейт ядра
эмиссией уже сняты), `skip_presets` — из карты эмиссии, у узла подписки
всегда ложь (`adapter_source.go`). Теги — финальные теги эмиссии.

Допущение (как в LxBox): до сборки готового конфига нет. Визард строит список
по последней эмиссии источников без граф-санитайзера и гейта реестра сборки,
поэтому может назвать узел, который сборка потом снимет. До первого разбора
список пуст: строка пресета показывает `No matching nodes`, пока разбор не
закончится.

Сервер пресета с `for_each` в списке DNS-серверов показывается без
выключателя (чекбокс неактивен): состояние такого сервера сборка читает по
тегу без пространства пресета, запись визарда `<preset_id>:<тег>` до неё не
доходит, и выключение в визарде было бы ложным.

Проверка: `go test ./ui/configurator/business -run
'TestPresetNodesForView_MatchesBuildSelection|TestPresetServedNodesLabel'`.
