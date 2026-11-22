# TASKS 188

Адреса — `CODEMAP.md`, решения — `PLAN.md`. Каждая волна заканчивается
зелёным `go build ./...`.

## Волна 1 — контракт
- [x] `contract/registry/rule_conditions.json`: таблица условий (PLAN «Таблица условий»), включая `apps` и `network_type` с `forms: ["lxbox"]`
- [x] `core/config/registry/rule_conditions.go`: типы, `RuleConditions()`, `RuleConditionOfKey()`, `RuleSetKeys()`, `RuleDNSKeys()`
- [x] `core/config/registry/registry.go` `Load`: чтение таблицы + проверка согласованности со списками `route_rule_conditions`, `rule_dns_mirror_match`, `rule_dns_block_https_domains`
- [x] `contract/registry/allowlists.json`: `rule_dns_mirror_match` + `network_is_expensive`; `rule_dns_mirror_unseen` + `source_port`, `source_port_range`; `note` трёх списков
- [x] `contract/schema/backup.schema.json`: `lx_backup` `enum [2, 3]`, `$defs/rule_body` (рекурсия, `refs` только у простого), описание `refs` записи как старой формы
- [x] `contract/docs/TEMPLATE_LANG.md` §6.11: зеркало по дереву, обёртка действий над `logical`, `invert`, исключение адресного inline-набора, ⚠
- [x] `contract/docs/TEMPLATE_LANG.md` §6.13 (новый): выпуск наборов правила и норма тегов
- [x] `contract/docs/ONE_NAMESPACE.md` §1: строка «srs-наборы», примеры записей
- [x] `contract/docs/BACKUP.md`: таблица `rules[]`, рукописный `rule_set`, §8 `lx_backup: 3`, §11 «против 1.0»
- [x] `contract/corpus/rule_tree/README.md` + 13 кейсов (PLAN «Корпус `rule_tree/`»)
- [x] `contract/corpus/backup/`: `v10_rule_refs_on_record`, `v11_rule_refs_in_body` (+ `.expected.lxbox.json` при расхождении сторон)
- [x] `contract/VERSION` 1.1.179, строка changelog в `contract/README.md`
- [x] `contract/TASKS_LXBOX.md` §182: дерево, `refs` в теле, таблица условий, нормы наборов и тегов, зеркало, `lx_backup: 3`, кейсы, вопросы 7–8 PLAN
- [x] `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md`: D-156, D-157
- [x] `go generate ./contract/...` — diff в `contract/docs/generated` пуст
- [x] `go build ./...`
- Примечания волны 1 (23.11.2026): `corpus/rule_tree/` до волны 3 не читает ни один раннер (каталог новый, раннер `TestContractCorpusRuleTree` — волна 3); `backup/v11_rule_refs_in_body` (`lx_backup: 3`) действующий раннер пропускает как «формат новее читаемого», `backup/v10_rule_refs_on_record` (`lx_backup: 2`) зелёный уже сейчас — тело в нём не сверяется, перенос `refs` в тело сверяет `v11`. В волне 2 `TestBackupCorpus` обязан пройти оба. Добавлено сверх PLAN: `RuleDomainKeys()`, `RuleCondition.ShownIn(form)`; поле `field` дано и ключам LxBox (`lines` / `enum_multi` / `bool`); §6.11 нормирует, что `invert` снятой обёртки с одним ребёнком переходит на ребёнка; ключ ожиданий `mirror_empty` в `rule_tree/`.

## Волна 2 — хранение
- [x] `core/state/rule_tree.go`: `WalkRuleTree`, `IsLogicalRule`, `RuleRefs`, `NormalizeRuleRecord(s)` (подъём по сырому JSON, `refs` первым ключом, корень `logical` — обёртка `and`); плюс `BodyRefs`, `BodyRefsNested`, `StripBodyRefs`, `RuleKeyRefs`
- [x] `core/state/rule_types.go`: `NewSrsRule` пишет `refs` в тело; `NewTreeRule`; `DecodeBody` через `RuleRefs`; шапка файла
- [x] `core/state/rule_set_options.go`: очистка настроек по выведенному `kind` (нет ссылок — очистка)
- [x] `core/state/disk_v8.go`, `save.go`, `state.go`, `schema_gate.go`, `load_router.go`: v9, нормализация в `parseV8` до `MigrateRuleSetTagRefs`; та переведена на `RuleRefs`
- [x] `core/state/load_v6.go`: UI-вид — `RuleSet` из `RuleRefs`
- [x] `core/build/resolve_route.go`: ссылки из `RuleRefs`, снятие `refs` с выпускаемого тела, вложенные `refs` → пропуск с логом
- [x] `core/build/preset_merge.go` `CollectSrsCachedPaths` → `map[ref]path`; `PresetMergeContext.SrsCachedPaths` новой формы
- [x] `core/build/rule_set_policy.go`: `RulePolicy`, `RuleSetFetches`, `DropUncachedSrsPaths`
- [x] `core/config_service.go` GC keep-лист; `core/config_service_context.go`; `ui/configurator/business/create_config.go`; `add_rule_dialog.go:1656` (последние три — без правок: тип карты выводится)
- [x] `core/stateedit/rules.go` `AddRule` (нормализация), `servers.go` `nameRefs` (правки не нужны: читает цель корня, не ссылки)
- [x] `core/debugapi/state_endpoints.go`: нормализация записей PATCH
- [x] `core/backup`: `FormatVersion11 = 3`, `Parse` 2|3, экспорт 3, нормализация в `decode10Rule` (через неё же идёт `kind: json` 0.x; `importRule` 0.x пишет `NewSrsRule` — новая форма) и в `export10Rules`; Debug API `/backup/formats` `reads` + 3
- [x] `core/backup/corpus_test.go`: ожидание `refs` через `RuleRefs`; `core/build/contract_rule_dns_test.go`: кэш по ссылке
- [x] Тест `TestRuleRefsLiftedIntoBody` (`core/state`, внешний пакет `state_test`: загрузка v8 → сохранение v9 → повторная загрузка, дерево главнее, дерево с `refs` в под-правиле, сборка плоского правила, экспорт/импорт бэкапа)
- [x] `go build ./...`; `go test ./core/state -run TestRuleRefsLiftedIntoBody -count=1`; `go test ./core/backup -run TestBackupCorpus -count=1` (`v10_rule_refs_on_record`, `v11_rule_refs_in_body` — PASS, не пропуск); `go test ./core/build -run TestContractCorpusRuleDNS -count=1`
- Примечания волны 2 (23.11.2026):
  - **Граница волны.** Сборка выпускает только ссылки корневого простого правила. `refs` в под-правилах `logical` — правило не выпускается (лог `rule sets inside a rule group are not built yet`, запись `Skipped`), остальная сборка идёт. Выпуск дерева — волна 3. Теги прежние (`user:<id>`, `user:<id>:N`, N — позиция в `RuleRefs`): плоское правило собирается в прежний `config.json`.
  - **Сверх PLAN (UI, защита от потери данных).** srs-правило, которое не «наборы + выход» (условия рядом со ссылками или дерево), в UI-виде (`load_v6.go`, `ruleTreeFlatRefs`) идёт телом целиком как Custom JSON со ссылками внутри — иначе любое сохранение визарда переписывало его `NewSrsRule` с одними ссылками. Следом: `DetermineRuleType` — тело с `refs` → raw; `customRuleStateToV6Rule` — inline-ветка через `NewTreeRule` (+ настройки наборов); `modelHasRuleSetFiles` видит `refs` в теле.
  - **Вывод `kind`.** Запись srs без ссылок и без единого условия (тело — только цель) остаётся srs (сборка пропускает её, как прежде): как inline она ловила бы весь трафик.
  - Тесты, чьи ожидания зависели от старой формы, поправлены без прогона (`rule_types_test`, `migration_v7_to_v8_test`, `canonical_roundtrip_test` версия, `v6_integration_test`, `purity_test`, `schema_test`, `preset_ref_sync_test`, тесты `core/build` с картой кэша). Голдены `core/state/testdata/v8_roundtrip.json` (байт-в-байт против Save) и сценарий `real-v088-v8` перегенерированы в волне 3.

## Волна 3 — сборка
- [x] `core/state/rule_tags.go`: `RuleTags`, `AllocateRuleTags`, `RefHash`; плюс `RuleTagBase` (края обрезаны), `RuleNodePath`, `NodeRefs`, `NodeHasSetKeys`, `StateRuleTags` (ось состояния, занятые — префиксы пресетов, PLAN вопрос 9), `RuleSetKeysFromRegistry`
- [x] `core/state/rule_identity.go`: `legacySrsRuleSetTag`, `MigrateRuleSetTagRefsV9` (тела inline/srs + DNS `user`; старый тег, совпавший с новым, не трогается — идемпотентность); вызов в `disk_v8.go`; UI-заглушка `load_v6.go` — новые теги
- [x] `ReservedRuleSetTags` — в `core/build/rule_tree_emit.go` (отдельный файл не заводился)
- [x] `core/build/preset_merge.go`: `PresetMergeContext.TemplateRouteRuleSetTags`; заполняет `BuildConfig` (`build.go`, из `cfg["route"]`) — один раз для всех разрешений маршрута сборки, поэтому `config_service_context.go` и `create_config.go` не правились; фолбэк из `routeRaw` в `MergePresetsIntoRoute` и `CollectEmittedRouteRuleSetTags`
- [x] `core/build/rule_tree_emit.go`: `emitUserRule` — ссылки, inline-наборы, `rule_set` (рукописные + ссылки + inline), всё или ничего, `refs` на узле `logical` → невыпуск, дерево зеркала, `MirrorDomains`; `RuleDNSMirrorForBody`, `RuleDNSMirrorEmpty` (признак ⚠ для волны 4), `TemplateRouteRuleSetTags`
- [x] `core/build/resolve_route.go`: `ResolveRouteTagged` (теги шаблона), раздача тегов до цикла, `resolveUserRouteRule`, поля `Mirror`/`MirrorDomains`; висячие рукописные `rule_set` — `dropDanglingHandwrittenRuleSets` (фикспойнт, правило и его наборы — Skipped, лог)
- [x] `core/build/rule_dns_mirror.go`: зеркало из `r.Mirror`, обёртка действий над `logical` и над простым `M` с `invert` (контракт 1.1.180); `RuleDNSMirrorMatch`/`BlockHTTPSAvailable`/`Narrowed` — по дереву тела
- [x] Все вызовы `ResolveRouteChecked` сборки → `ResolveRouteTagged` с `ctx.TemplateRouteRuleSetTags` (`preset_merge.go` ×2, `rule_dns_mirror.go`); превью окна — то же с тегами шаблона модели
- [x] `ui/configurator/dialogs/add_rule_dialog.go` `rulePreviewText`: ось модели целиком (правило окна на месте редактируемого) + пресеты и теги шаблона; в показ — только правило окна
- [x] `ui/configurator/tabs/dns_unified_rules.go`, DNS-вкладка окна: правок не потребовалось — экспортированные функции зеркала стали древесными под теми же именами
- [x] Раннер `TestContractCorpusRuleTree` (`core/build/contract_rule_tree_test.go`): 15 кейсов (13 + 2 из 1.1.180)
- [x] Кейс миграции ссылок в `TestRuleRefsLiftedIntoBody` (inline `user:Lists:2`, DNS `user:Lists`); ожидания сборки — новые теги, дерево выпускается
- [x] Контракт 1.1.180 (пять решений по прогону LxBox + обёртка `invert`): TEMPLATE_LANG §6.11/§6.13, `corpus/rule_tree/README.md`, кейсы `disabled_derived_stable`, `invert_block_https`, changelog, TASKS_LXBOX §183; SPEC §4 «края обрезаны»
- [x] `go build ./...`; по имени: `TestContractCorpusRuleTree`, `TestContractCorpusRuleDNS`, `TestRuleSetRemoteEmission`, `TestGoldenScenarios`, `TestWizardTemplateConfigUnchanged` (`core/build`); `TestRuleRefsLiftedIntoBody`, `TestStableRuleID_LossyNamesStayDistinct` (`core/state`); `TestBackupCorpus` (`core/backup`)
- [ ] CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`
- [ ] Владельцу: свой `config.json`, `sing-box check`
- Примечания волны 3 (23.11.2026):
  - Теги шаблона неизвестны (`ResolveRouteChecked`, `templateTags == nil`) — рукописные `rule_set` на висячесть не проверяются (как `finalTags == nil` у download_detour). Боевой путь всегда передаёт теги.
  - Рукописный `rule_set` в узле без `refs` оставлен байт-в-байт (список из одного тега не сворачивается в строку): `config.json` inline-правил прежний.
  - config.json плоских правил без наборов не меняется; у правил с наборами меняются только теги (`user:<id>` → имя). Изменения сверх тегов: правило с `invert` и DNS-опцией получает `invert` в зеркале (SPEC §5), рукописный висячий `rule_set` снимает правило.
  - Ревью волны 2 (отдельный коммит `fix(state)`): (1) `meta.schema` файла v9 — объект `{"name":"sources_v9","min_reader":9}` — разбор сборок до v9 (2.4.x) на нём падает, файл отвергается целиком, а не читается с потерей srs-правил; (2) окно правила в режиме JSON с `refs` в теле сохраняет настройки наборов правила; (3) пустой `refs` снимается нормализацией, `refs` на узле `logical` — запись как есть, правило не выпускается (Skipped + лог), ключ `refs` в config.json не уходит ни в одной ветке.
  - Голдены перегенерированы: `core/state/testdata/v8_roundtrip.json`, `core/build/testdata/golden/real-v088-v8/state.json` — версия 9, `meta.schema` объектом, `refs` в теле; в `real-v088-v8` ещё дрейф прошлых волн — ключ папки `strategy` → `auto` (контракт свёртки, Save пишет новый ключ). Конфиги сборки не менялись (srs-правила сценария выключены).

## Волна 4 — форма
- [ ] `ui/configurator/models/rule_conditions.go`: модель стопки, `StackFromBody`, `Body`; сохранение исходных байт при неизменной стопке
- [ ] `ui/configurator/dialogs/rule_cond_stack.go`: стопка, «AND», «+ Condition», «Rule group» за чертой
- [ ] `ui/configurator/dialogs/rule_cond_frame.go`: поля по виду из таблицы, «any of», `one_of`, пометка Wi-Fi по платформе цели
- [ ] `ui/configurator/dialogs/rule_cond_group.go`: all of / any of, Not, «+ Rule», рекурсия
- [ ] `ui/configurator/dialogs/rule_cond_refs.go`: поле наборов (перенос SPEC 187), подсказка, `ip_is_private` недоступна при `refs`
- [ ] `ui/configurator/dialogs/rule_dialog_dns.go`: перенос вкладки DNS, опции доступны всегда, ⚠
- [ ] `ui/configurator/dialogs/rule_dialog_preview.go`, `rule_process_picker.go`: перенос
- [ ] `ui/configurator/dialogs/add_rule_dialog.go`: окно, имя, выход, вкладки, save, JSON ↔ форма через модель; цель только у корня; убрать `Params`, радио типа, `srsRuleSetsForRawBody`
- [ ] Удалить `ui/configurator/dialogs/rule_type_selection.go`
- [ ] `ui/configurator/dialogs/rule_set_options.go`: виден при `refs` в дереве
- [ ] `core/state/load_v6.go`: `cr.Rule` — тело с `refs`, без заглушки `rule_set`
- [ ] `ui/configurator/models/preset_ref_sync.go`: `NewTreeRule`; `RuleSets` модели — производное от `refs`
- [ ] `ui/configurator/models/wizard_state_file.go`: `DetermineRuleType` только для legacy `Type`
- [ ] `ui/configurator/tabs/rules_tab.go`: srs-признак по `refs`, ⚠ в строке правила
- [ ] `ui/configurator/tabs/dns_unified_rules.go`: строка зеркала с ⚠, `ruleStateIsSRS` по `refs`
- [ ] `bin/locale/ru.json`: все новые ключи
- [ ] `go build ./ui/...`; показ владельцу

## Волна 5 — закрытие
- [ ] `docs/release_notes/upcoming.md` (EN Highlights, RU Основное)
- [ ] `RELEASE_NOTES.md`
- [ ] `docs/ARCHITECTURE.md`, `docs/WIZARD_STATE.md`
- [ ] `IMPLEMENTATION_REPORT.md`
- [ ] `SPECS/README.md`: строка 188
- [ ] CI зелёный на трёх платформах
- [ ] Ответ LxBox по §182 получен; релиз — после LxBox
- [ ] Папка → `188-F-C-RULE_CONDITIONS_TREE`
