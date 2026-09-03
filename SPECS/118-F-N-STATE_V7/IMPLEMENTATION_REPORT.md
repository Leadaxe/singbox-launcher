# SPEC 118 · Этап 2 — отчёт о реализации

Кампания: схема состояния v7, материализация узлов подписки, смерть
raw-кэша и легаси-механик. Волны W1–W8, база сравнения — `39ab397`
(последний коммит до W1).

Волновые отчёты: `reports/W5_REPORT.md`, `reports/W6_REPORT.md`,
`reports/W7_REPORT.md`. Этот файл — сводка кампании и приёмка W8.

**Вердикт байт-эквивалентности — в §7. O3 ЗАКРЫТ вердиктом капитана
(2026-08-29): Р2 принят, Р-DNS-1 принят как раннерный артефакт,
Р-DNS-2 починен здесь. Исполнение вердикта — §7.6.**

---

## 1. Что сделано

Состояние переехало на плоскую схему v7 (`meta` / `sources[]` /
`directions[]` / `rules[]` / `vars[]` / `dns_options` / `warp_accounts`).
Узлы подписки хранятся явно (`nodes[]`) и материализуются на fetch'е;
сборка конфига эмитит outbound'ы из них и больше не разбирает тела.
Кэш сырых ответов, карта выключенных узлов, свёртка (fold),
`exclude_from_global`/`expose_group_tags_to_global`, локальные Направления,
маска тегов, превью-кэши, `Defaults` и detour-тройня удалены; их предмет
живёт в новой модели или назван потерей в отчёте миграции.

---

## 2. Файлы по волнам

Счёт диффа `39ab397..HEAD`: **140 изменённых, 40 добавленных, 33
удалённых** файла (+14 689 / −9 734 строки). Из этого счёта исключить
`ui/traffic/**` и `internal/platform/singtun_fwrules*` — правки
параллельной сессии, кампании не принадлежат.

| Волна | Ядро правки | Ключевые файлы |
|---|---|---|
| **W1** — типы, схема, Load/Save, мост | форма v7 и плоский корень; `State.Sources`/`State.Directions` вместо `Connections` | `core/state/sources_v7.go`, `disk_v7.go`, `save.go`, `load_router.go`, `adapter_source.go` (мост, снят в W5) |
| **W2** — миграция v6→v7, чистый парсер, эталоны | семантический перенос всех легаси-полей + отчёт потерь; эталоны сняты СТАРЫМ движком | `core/state/migration_v6_to_v7.go`, `migration_report.go`, `migration_hooks.go`, `core/config/subscription/parse_body.go`, `core/config/migrate_materialize.go`, `etalon/**`, `core/etalon_v6mig_capture_test.go` |
| **W3** — fetch/merge, материализация | тело → `nodes[]` на живом конвейере; merge по тегу; реальный кап | `core/config/fetch_materialize.go`, `core/state/subscription_merge.go`, `core/config/subscription/fetcher.go`, `source_loader.go` |
| **W4** — сборка из `nodes[]`, единый резолв, гард, пул | эмиссия канона, `NodeLink`-резолв, гард занятости тегов, `dns.detour` в санитайзер | `core/config/canonical_emit.go`, `nodelink_resolve.go`, `folder_replaces.go`, `tag_guard.go`, `core/build/dns_detour_sanitize.go` |
| **W5** — смерть легаси | снос миграции (шаг 8) + удаление всех перечисленных механик; мост снят целиком | удалены `core/state/raw_cache.go`, `core/rebuild_raw_cache.go`, `core/config/source_folds.go`, `configtypes/source_fold.go`; добавлены `core/rebuild_snapshot.go`, `core/state/migration_raw_cache.go`, `core/backup/convert_v7.go`, `configtypes/chain_body.go` |
| **W6** — UI-компенсация | счётчики/превью/пул хопов из `nodes[]`; вкладка Replace; настройки приложения; отчёт миграции в диалоге | `ui/configurator/tabs/source_replace_tab.go`, `source_body_edit.go`, `source_tag_shift_warning.go`, `ui/configurator/business/node_pool.go`, `source_node_counts.go`, `fetch_writeback.go`, `ui/configurator/migration_report_dialog.go`, `ui/settings_tab.go` |
| **W7** — бэкап-конвертеры и remote-гейт | граница v7 ↔ контракт 0.11; гейт схемы у ФАЙЛА; единая сериализация Debug API | `core/backup/convert_v7.go`, `core/state/schema_gate.go`, `core/debugapi/state_endpoints.go`, `remote_state_endpoints.go`, `remote_endpoints.go` |
| **W8** — голдены и приёмка | перезафиксация golden-состояния в v7; починка раннера; §4.B.10 на реальной фикстуре; документы | `core/build/golden_test.go`, `core/build/testdata/golden/real-v088/state.json`, `core/state/testdata/real_v088_v4.json`, `core/state/migration_scenarios_test.go`, `docs/release_notes/upcoming.md`, `.gitignore` |

---

## 3. Судьба тестов категории (б)

Полные таблицы — `reports/W5_REPORT.md` §5. Сводка: **26 файлов удалены**
(предмет умер), остальные переработаны под новую форму, категория (в)
осталась зелёной.

### Удалены — предмет отменён

| Файл | Причина |
|---|---|
| `core/config/subscription/e2e_disabled_flow_test.go` | **осознанная смена контракта**: сквозной сценарий disabled-карты; отметка живёт на узле (`enabled`), пути «ключ → hash → карта → фильтр» больше нет |
| `core/config/subscription/disabled_nodes_test.go`, `disabled_migration_test.go` | та же карта на уровне парсера |
| `core/config/subscription/detour_test.go` | detour писался в тело узла; тело чисто по построению |
| `core/config/subscription/trusted_parse_test.go` | `trustedParse` жил ради чисток карты; достоверность решает fetch-сервис |
| `core/config/expose_exclude_test.go`, `excluded_sources_test.go` | `ExcludeFromGlobal`/`ExposeGroupTagsToGlobal` и `excludedSources` снесены |
| `core/config/detour_cascade_test.go`, `detour_node_ref_test.go` | резолв тройни снесён; каскад/кольца — `canonical_emit_test.go` |
| `core/config/ira_state_migration_test.go` | миграция `detour_node_hash` умерла с полем |
| `core/state/raw_cache_test.go`, `core/rebuild_raw_cache_test.go` | raw-кэш снесён |
| `core/state/disabled_nodes_roundtrip_test.go` | карта не сериализуется |
| `core/state/detour_mapping_test.go`, `detour_node_ref_test.go` | проекция тройни снесена |
| `core/state/config_json_roundtrip_test.go` | `Source.ConfigJSON` снесён (тело — в `Node.Body`) |
| `core/debugapi/disabled_nodes_api_test.go` | эндпоинт отдавал поле, которого нет |
| `core/preview_nodes_test.go`, `core/refresh_meta_test.go` | `Meta.PreviewNodes` и мостовая Meta-история снесены |
| `core/build_report_generation_test.go` | строился на `state.WriteRawBody` |
| `ui/configurator/tabs/disabled_node_toggle_test.go` | тумблер писал карту |
| `ui/configurator/tabs/source_node_tag_buffer_test.go` | буферизовал `NodeTag`/`Fold`/`DisabledNodes` — все три снесены |
| `ui/configurator/tabs/preview_singbox_body_test.go` | превью разбирало тело; тело лежит готовым |
| `ui/configurator/business/detour_test.go` | пикер на тройне; новый — `detour_refs_test.go` на `NodeLink` |
| `ui/configurator/business/preview_cache_chain_test.go`, `preview_dedup_test.go` | кэш превью разбирал тела; пул строится эмиссией |

### Переработаны

`canonical_emit_test.go`, `chain_emit_test.go`, `contract_direction_test.go`,
`direction_twins_test.go`, `group_node_contract_test.go`,
`naive_degrade_test.go`, `manual_config_emit_test.go`,
`source_parse_failed_test.go`, `varsubst_test.go`,
`uniquify_collision_test.go`, `singbox_import_e2e_test.go`,
`core/state/*`, `core/backup/*`, `ui/configurator/**`.

### Добавлены как обвязка

`core/state/legacy_fixture_copy_test.go` — обязательна: со включённым
сносом Load переписывает легаси-файл на месте, и чтение фикстуры по её
месту в репозитории уничтожало бы её первым же прогоном.

### Одно осознанное отступление в корпусе

`contract/corpus/direction/fold_select_auto` помечен `t.Skip` с явной
причиной (`corpusDivergence`): его ожидания описывают упразднённую пару
тегов. Вердикт **O1 = А** — корпус не трогаем.

---

## 4. Изменённые сигнатуры

| Символ | Было → Стало | Почему |
|---|---|---|
| `config.GenerateOutboundsFromParserConfig` | принимал `loadNodesFunc` → не принимает | «сборка полезла разбирать тела» становится невыразимым по построению |
| `backup.Export` | `(*Backup, error)` → `(*Backup, []Warning, error)` | виды источников, которых контракт 0.11 не знает (folder/auto), обязаны называться пользователю, а не теряться |
| `state.State` | `Connections ConnectionsSection` → `Sources []Source` + `Directions []configtypes.Direction` | плоский корень v7 |
| `(*state.State).MarshalV7` | новый | сериализация у Debug-endpoint'а и у файла теперь ОДНА по построению |
| `configtypes.ChainBody` / `ChainFromBody` | новые | настройки маршрута цепочки переехали в `Node.Body` |
| `config.MaterializeServerNode` | новая | единственная точка «share-URI или JSON → тело узла» |
| `(*TemplateInt).Clone`, `(*DirectionAuto).Clone` | новые | deep-copy вместо разделяемых указателей (хвост ревью W1) |

---

## 5. Таблица grep-инвариантов §4.A (повторный прогон, W8)

Счёт по прод-коду (`--include=*.go`, `*_test.go` исключены), за вычетом
санкционированных исключений: файлы входа миграции v6→v7
(`core/state/connections.go`, `source_fold_migrate.go`, `load_v5.go`,
`load_v6.go`, `disk_v6.go`, `migration_*.go`, `legacy_migration.go`,
`core/config/migrate_materialize.go`) и конвертеры границы бэкапа
(`core/backup/**`).

| Инвариант | Результат W8 |
|---|---|
| raw-кэш снесён | файлов нет; **0** |
| disabled-карта снесена | **0** вне миграции (3 вхождения — `core/state/connections.go`, легаси-форма чтения) |
| fold снесён | **0** вне миграции (`source_fold_migrate.go`, `load_v6.go`) |
| exclude/expose снесены | **0** вне миграции и бэкапа |
| mask снесён | **0** вне миграции (`migrate_materialize.go`, `connections.go`) |
| локальных Направлений нет | поля `Outbounds` у v7 `Source` нет; **0** |
| detour-тройня снесена | **0** вне миграции (`connections.go`) |
| PreviewNodes снесены | **0** |
| defaults ушли | **0** |
| body чист от detour (`ApplySourceDetour`) | **0** |
| сборка не парсит подписки | по построению (сигнатура) |
| обёртки `connections` нет | только миграция |
| греп go1.20 по диффу кампании (`min`/`max`/`clear`/`slices.`/`maps.`/`PathValue`/`errors.Join`) | **0** (три совпадения — комментарии, объясняющие сам гард) |

### Оговорка по инварианту «PreviewNodes снесены»

Греп `SourceNodeCounts` даёт вхождения в `ui/configurator/business/source_node_counts.go`
и `ui/configurator/models/wizard_model.go`. Это **новый** счётчик списка
Sources, читающий `nodes[]` напрямую (W6), а не воскресший превью-кэш:
предмет инварианта — «превью, разбирающее тела» — мёртв
(`PreviewNodes`/`PreviewCacheGeneration`/`RebuildPreviewCache` → 0).
Совпадение имени, не механики.

---

## 6. Результат приёмки (W8)

| Проверка | Результат |
|---|---|
| `go build ./...` | зелёный |
| `go vet ./...` | зелёный |
| `go test -count=1 ./...` | зелёный (весь модуль) |
| `bash build/build_darwin.sh` (GUI) | зелёный, `.app` собран |
| `ETALON_V6MIG=1` (эмиссия v6mig — эталон v6_roundtrip-класса) | **РОВНО одно** задекларированное расхождение Р2 |
| §4.B.10 на синтетической мигрированной модели | зелёный (`tag_guard_model_test.go`) |
| §4.B.10 на **мигрированном real-v088** | зелёный (`TestMigrationScenario10RealV088RuleTargetsNotReset`, добавлен в W8) |
| Golden `real-v088` (config.json) | зелёный БЕЗ skip (`go test ./core/build`), `expected.config.json` перезафиксирован после фикса Р-DNS-2; против эталона W2 — один класс Р-DNS-1, принят. См. §7 |

---

## 7. Байт-эквивалентность конфига (§4.C) — вердикт

### 7.1 `v6mig` / эталон класса `v6_roundtrip` — ЧИСТО (с одним объявленным)

`ETALON_V6MIG=1` даёт **ровно одно** расхождение, ровно то, что
задекларировала W2 (риск Р2): авто-группа свёртки `both` называется
`[P]select-auto` вместо `[P]auto`. В снимке это 3 строки — сам тег и две
ссылки на него (`outbounds[]` и `default` селектора). Состав групп,
порядок узлов, все опции и все тела — байт-в байт. Других расхождений
нет.

**Оценка:** задекларированная цена модели v7 (`<tag>` + `<tag>-auto`),
переписанная миграцией со всеми ссылками и с явным предупреждением
пользователю. Не блокер.

### 7.2 `real-v088` — эмиссия ЧИСТО, секция `dns` — ДВА РАСХОЖДЕНИЯ

**Сначала главное: предмет SPEC 118 сошёлся байт-в-байт.** Сборка нового
движка из **мигрированного v7-состояния** real-v088 совпадает с эталоном
W2 по всем секциям, кроме `dns`:

```
outbounds  — идентичны
endpoints  — идентичны
route      — идентичны (включая route.rule_set: ru-domains эмитится)
inbounds / log / experimental — идентичны
dns        — 2 расхождения (ниже)
```

Более того: при **неизменённом раннере** сборка нового движка из
v4-фикстуры совпадает с эталоном W2 **полностью, байт-в-байт**
(`cmp` → идентично). То есть кампания сама по себе конфиг не сдвинула.

**Откуда расхождения.** Задача W8 требует перезафиксировать golden-состояние
в v7-форме. Это вскрыло, что раннер golden читал DNS из **легаси-зеркала
v5** `state.DNSOptions`, минуя тот путь, которым конфиг собирается в бою.
У v7-файла это зеркало `nil` по построению (`load_v6.go`), и старый раннер
на v7-состоянии выдавал `dns.servers: []` — минус 919 байт живой секции.
Раннер починен по прод-функциям `core.dnsConfigForUpdate` /
`buildContextFromState`: v6+-путь читает канонический `state.DNS`, а
servers/rules эмитятся через `MergePresetsIntoDNS`, как в продакшене.

После починки раннера обе версии состояния (v4 и мигрированная v7) дают
**один и тот же** результат — значит, миграция состояние не исказила:

| раннер | состояние | результат против эталона W2 |
|---|---|---|
| старый (путь v5) | v4 | идентично |
| старый (путь v5) | v7 | `dns.servers: []` — сравнение недействительно |
| починенный (путь прод) | v4 | 2 расхождения `dns` |
| починенный (путь прод) | v7 | **те же самые** 2 расхождения `dns` |

### 7.3 Поимённый список на вердикт O3

**Р-DNS-1 — порядок `dns.servers`.**
Тот же набор из 5 серверов, другой порядок:
`[local, direct, cloudflare_udp, google_doh, yandex_doh]` (эталон) против
`[direct, google_doh, yandex_doh, local, cloudflare_udp]` (прод-путь).
Множества совпадают полностью (проверено: 0 лишних, 0 недостающих).
Причина: `MergeDNSSection` на v5-пути **перезаписывает** `dns.servers`
списком из состояния (строка 85, `dnsObj["servers"] = servers`), сохраняя
порядок состояния; на v6+-пути `cfg.Servers` пуст, шаблонные серверы
стираются и заново дописываются `MergePresetsIntoDNS` после серверов
состояния.
**Оценка:** косметика, потери нет. sing-box порядок `dns.servers` не
трактует.

**Р-DNS-2 — снят `rule_set: "ru-domains"` у DNS-правила.**
Эталон: `{"rule_set": "ru-domains", "server": "yandex_doh"}`.
Прод-путь: `{"server": "yandex_doh"}` — правило перестаёт ограничиваться
российскими доменами и начинает матчить всё.
Причина: `MergePresetsIntoDNS` чистит висячие `rule_set`-ссылки через
`cleanDanglingDNSRule`, а множество валидных тегов строит
`collectRuleSetTagsFromPresets` — которая обходит **только правила
`RuleKindPreset`**. В этой фикстуре `ru-domains` объявлен правилом
`RuleKindSrs` (пользовательское srs-правило «Russian domains direct»), и
функция его не видит. При этом `ru-domains` **реально эмитится** в
`route.rule_set` — ссылка не висячая, она просто не найдена.
**Оценка: настоящая потеря поведения. НО — не кампании.**

### 7.4 Почему это не регрессия SPEC 118

Весь DNS-конвейер не тронут кампанией. Проверено диффом от базы:

```
git diff --stat 39ab397 -- core/config_service_context.go \
    core/build/preset_merge.go core/build/dns_merge.go \
    core/build/resolve_dns.go core/build/rules_pipeline.go \
    core/build/preset_expand.go
→ пусто
```

Единственная правка кампании в DNS — `SanitizeDNSDetours` (W4,
`core/build/build.go` +8 строк); в этой фикстуре ни у одного DNS-сервера
нет ключа `detour`, функция — no-op.

Входы DNS-секции у v4-парса и v7-парса **байт-идентичны** (sha256 по
`State.DNS`, `State.Rules`, `State.Vars` совпадают; расходится только
`State.CustomRules` — легаси-проекция, на v6+-пути не используемая).

Вывод: **Р-DNS-1 и Р-DNS-2 — предсуществующие свойства нетронутого кода**,
проявившиеся исключительно оттого, что W8 сделала раннер честным.
Р-DNS-2 — настоящий баг, который сегодня бьёт по реальным пользователям
на v6-состоянии (DNS-правило, ссылающееся на пользовательское srs-правило,
молча теряет ограничение). Чинить его внутри SPEC 118 значит менять
поведение продакшена правкой вне объёма кампании.

> **Вердикт капитана:** чинить здесь. Исполнено — §7.6.

### 7.5 Что было отложено до вердикта (исполнено — см. §7.6)

- **`expected.config.json` real-v088 НЕ перезаписан.** Записать в него
  текущий выхлоп значило бы задним числом благословить Р-DNS-2 как
  норму — прямо запрещено O3.
- **SKIP у `real-*` НЕ снят.** Снятие сделало бы `go test ./core/build`
  красным на предсуществующем баге. Текст SKIP заменён на честный: он
  называет причину и ссылается на этот раздел.

---

### 7.6 ВЕРДИКТ КАПИТАНА (2026-08-29) и его исполнение

O3 закрыт. Решение по каждому расхождению поимённо:

| Расхождение | Вердикт | Что сделано |
|---|---|---|
| **Р2** — `[P]auto` → `[P]select-auto` (тег авто-группы свёртки `both`) | **ПРИНЯТ** (принят ещё в W2) | ничего; эталон `v6mig` не перезафиксирован, расхождение остаётся ожидаемым и задекларированным |
| **Р-DNS-1** — порядок `dns.servers` | **ПРИНЯТ** как раннерный артефакт снятия эталона: потери нет, множества совпадают (0 лишних, 0 недостающих), порядок sing-box не трактует | эталон `real-v088.config.json` НЕ тронут; расхождение задекларировано поимённо в `etalon/README.md`; живым рубежом стал golden-сценарий, чей `expected.config.json` перезафиксирован честным выхлопом прод-пути |
| **Р-DNS-2** — снятый `rule_set: "ru-domains"` у DNS-правила | **ЧИНИТЬ ЗДЕСЬ** (предсуществующий прод-баг) | починен, см. ниже |

#### Фикс Р-DNS-2

Диагноз уточнён относительно §7.3: `ru-domains` в этой фикстуре объявлен
не правилом `RuleKindSrs`, а прямо в `route.rule_set` ШАБЛОНА
(`template.json`, секция `config.route.rule_set`); правило состояния,
ссылающееся на него, — `kind=inline`. Существо расхождения от этого не
меняется и делается только шире: множество известных тегов было неполным
**по всем источникам, кроме preset-ref'ов**, а не только по srs.

Суть фикса — множество валидных `rule_set`-тегов строится по ТЕМ ЖЕ
источникам, из которых складывается `route.rule_set` финального конфига,
а не по одному из них.

| Файл | Правка |
|---|---|
| `core/build/preset_merge.go` | `collectRuleSetTagsFromPresets` (только `RuleKindPreset`) заменена на **`CollectEmittedRouteRuleSetTags(routeRaw, routeCfg, ctx)`**: (1) теги шаблонной route-секции, (2) rule_set'ы включённых `RouteConfig.Rules` (легаси-путь `MergeRouteSection`), (3) всё, что эмитит `ResolveRouteWithGlobals` — ЛЮБОГО kind'а, с теми же фильтрами `Skipped`/`Enabled`, что и `MergePresetsIntoRoute`. Источник (3) — вызов резолвера, а не перечисление kind'ов: новый вид правила, эмитящий rule_set, попадает в множество сам |
| `core/build/preset_merge.go` | у `PresetMergeContext` — поле `EmittedRuleSetTags`; `MergePresetsIntoDNS` берёт готовое множество оттуда, `nil` = вырожденный режим для вызывающих без доступа к route (превью секции, юнит-тесты merge'а) |
| `core/build/build.go` | `buildOrderedSections` считает множество ОДИН раз до обхода секций (рядом с `collectAllFinalOutboundTags`, по тому же образцу) — секция `dns` собирается раньше `route` и своими силами этого знать не может |
| `core/build/dns_ruleset_dangling_test.go` | новый комплексный тест |

**Чистка не отключена — это принципиально.** `cleanDanglingDNSRule`
работает как прежде: ссылка на тег, которого в конфиге нет, снимается,
иначе ядро падает на `initialize DNS rule[N]: rule-set not found: <X>` и
конфиг мёртв целиком. Изменилось только то, по какому множеству она судит.

Тест `TestDNSRuleSetRefsSurviveWhenTagIsEmitted` держит обе стороны в
одном прогоне BuildConfig (порядок секций зафиксирован `dns` → `route` —
то самое условие, в котором баг возникал): живая ссылка на шаблонный
`ru-domains` уцелела, живая ссылка на тег srs-правила (`user:Ads`, файл в
кэше) уцелела, действительно висячая `ghost-set` снята, и снята ровно
одна. Разделять их нельзя: по отдельности каждая половина зеленеет на
сборке, где чистка вообще выключена. Проверен «на кусачесть» — с
откаченной правкой падает на `ru-domains`.

#### Состояние после исполнения вердикта

| Проверка | Результат |
|---|---|
| `real-v088` против эталона W2 | ровно один класс расхождений — **Р-DNS-1** (порядок `dns.servers`, множества идентичны). Р-DNS-2 исчез: `rule_set: "ru-domains"` на месте |
| `expected.config.json` сценария | перезафиксирован честным выхлопом починенного раннера |
| SKIP у `real-*` в `golden_test.go` | **снят**; переменная `GOLDEN_RUN_REAL` удалена, сценарий идёт в обычном `go test ./core/build`; сравнение осталось строгим байт-в-байт (нормализуются только timestamp `parser.last_updated` и косметические `//`-комментарии — как и до правки) |
| `ETALON_V6MIG=1` | по-прежнему **ровно одно** расхождение Р2, те же три строки |
| `go build ./...`, `go vet ./...`, `go test -count=1 ./...` | зелёные |
| греп go1.20 по правкам фикса | 0 |

---

## 8. Статус открытых вопросов

| Вопрос | Статус |
|---|---|
| **O1** — fold-фикстуры общего корпуса | **ЗАКРЫТ, вариант А.** Корпус не тронут; `fold_select_auto` пропущен с явной причиной; правка корпуса — трек `folders[]` с LxBox |
| **O2** — disabled-отметки при импорте бэкапа | **ЗАКРЫТ, вариант А.** `pending_disabled` у Subscription: применяется по сырым тегам на первом достоверном fetch и стирается; покрыт тестом (W7); `features/state.md` дополнен |
| **O3** — порог расхождений golden | **ЗАКРЫТ вердиктом капитана 2026-08-29.** Поимённо: **Р2** (`[P]auto` → `[P]select-auto`) — ПРИНЯТ, задекларированная цена модели v7, эталон `v6mig` не тронут; **Р-DNS-1** (порядок `dns.servers`) — ПРИНЯТ как раннерный артефакт снятия эталона, потери нет, множества совпадают, эталон `real-v088.config.json` не тронут, расхождение задекларировано в `etalon/README.md`; **Р-DNS-2** (снятый `rule_set` у DNS-правила) — ПОЧИНЕН здесь: `CollectEmittedRouteRuleSetTags` в `core/build/preset_merge.go` + точка вызова в `core/build/build.go`, тест `core/build/dns_ruleset_dangling_test.go`. Следом: `expected.config.json` real-v088 перезафиксирован честным выхлопом, SKIP у `real-*` снят. Разбор и исполнение — §7.6 |
