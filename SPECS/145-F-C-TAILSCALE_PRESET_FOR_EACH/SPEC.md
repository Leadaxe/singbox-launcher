# SPEC 145 — Пресет Tailscale: `for_each`, `@node`, `#tpl`, `skip_presets`

Статус: **C** (решение владельца 2026-09-27, реализовано 2026-09-27).
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

- Строка пресета на вкладках Rules/DNS не показывает обслуживаемые узлы;
  серверы пресета с `for_each` не видны в списке DNS-серверов визарда
  (раскрытие в UI идёт без узлов).
- Превью визарда не знает `skip_presets` (кэш превью строится из
  `GeneratedOutbounds` без записи): узел со `skip_presets` в превью
  обслуживается, в боевой сборке — нет.
