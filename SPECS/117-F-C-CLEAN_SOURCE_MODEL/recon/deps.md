# Аудит SPEC 117 DRAFT: внешние зависимости от тегов узлов и источников

Тема: кто ВНЕ сборки outbound'ов держится за теги узлов/источников, и что с этими
потребителями делает новая модель (NodeLink, FolderReplace, материализация,
смерть локальных Направлений, выкинутый mask).

Все пути — от корня `/Users/macbook/projects/singbox-launcher`.

---

## 1. Карта потребителей тегов вне сборки

### 1.1 rules[].outbound — плоский финальный тег

- `core/state/rule_types.go:91-99` — `InlineBody.Outbound` / `SrsBody.Outbound`:
  «outbound tag или литерал reject/drop». Строка, без ссылки-объекта.
- Множество допустимых целей собирает UI:
  `ui/configurator/business/outbound.go:69` (`GetAvailableOutbounds`) —
  Направления (без выключенных), их `addOutbounds`, теги активных пресетов
  (`collectActivePresetOutboundTags`), глобальные outbound'ы шаблона
  (`block-out`, `direct-out`), плюс `direct`/`reject`/`drop`.
  Узлы (ни подписные, ни верхние) целями правил НЕ предлагаются.
- SPEC 108: локальные группы подписок из целей выкинуты
  (`ui/configurator/business/outbound.go:125-133`), осиротевшие цели
  сбрасываются на direct при загрузке —
  `ui/configurator/presentation/rule_target_reset.go:35-83`
  (`resetForeignRuleTargets`; множество known = GetAvailableOutbounds +
  `AllDirectionTags`, т.е. и выключенные Направления).
- Переименование Направления переписывает цели правил:
  `ui/configurator/business/direction_rename.go:132-145`.

### 1.2 route.final

- Два синхронных места хранения: `model.SelectedFinalOutbound` и
  `SettingsVars["route_final"]` —
  `ui/configurator/presentation/presenter_state_helpers.go:141-172`,
  `ui/configurator/presentation/presenter_state.go:194-203`.
- Эмиссия: `core/build/route_merge.go:145-146` (`route["final"]`).
- Сброс осиротевшего final на direct: `rule_target_reset.go:75-82`.
- Rename Направления правит оба места: `direction_rename.go:148-165`.

### 1.3 dns_options: detour DNS-серверов

- kind=user серверы хранят тело сырым (`core/state/dns_options.go:89-96`,
  `Body["detour"]`); эмиссия detour — `core/build/resolve_dns.go:509`,
  strip `direct-out` — `:533-535`, `:571-574`.
- Rename Направления точечно правит detour в сыром JSON:
  `direction_rename.go:206-257` (`renameDNSDetour`) — покрывает ТОЛЬКО тег
  Направления и его `-auto` двойника.
- Граф-санитайзер (`core/build/outbound_graph_sanitize.go`) DNS-секцию не
  трогает — висячий dns.detour на несуществующий тег сборкой не ловится
  (грепом «dns» по санитайзеру — пусто).

### 1.4 Цепочки

- Позиции: `configtypes.SourceChain.Hops []string` —
  `core/config/configtypes/types.go:726` — хранятся ФИНАЛЬНЫМИ конфиговыми
  тегами (узлы после prefix/mask/uniquify, Направления, `direct-out`).
- Резолв: `core/config/chain_nodes.go:55-194` (`ResolveChainSources`) —
  known = все ParsedNode.Tag (финальные) + directionTags + twins +
  `ChainBuiltinHopTags`; несуществующий хоп = деградация цепочки fail-closed.
- Тег самой цепочки: `chain_nodes.go:35-36` — `chainSourceTag` берёт
  `src.TagMask` (!) как тег узла цепочки.
- Рантайм-адресация по тегу к ЖИВОМУ ядру:
  - `core/chain_probe.go:24-28` — `chainProbeTag` → `config.ChainLayerTag`;
  - `core/config/chain_validate.go:100-102` — служебные теги `<chain>#<pos>`;
  - `core/debugapi/chain_endpoints.go:184-187` — `/chains/{tag}/probe`;
  - `core/services/lxd_remote_transport.go:379-389` — то же для машины;
  - `ui/servers_node_info_chain.go:48-218` — `ChainFor(tag)`,
    `SetChainPositionEnabled(chainTag, pos)` — тумблеры позиций по тегу.
- Хопы в форме выбираются из PreviewNodes:
  `ui/configurator/tabs/source_chain_hops.go:151, :181`.

### 1.5 Detour источника (тройня → NodeLink)

- `core/config/configtypes/types.go:170-212`:
  - `DetourTag` — финальный тег ГРУППЫ;
  - `DetourNodeSourceID` (ULID источника) + `DetourNodeTag`
    (identity-тег ДО prefix/mask/uniquify) — ссылка на один узел;
  - переходная форма: пустой source_id ⇒ тег трактуется как ФИНАЛЬНЫЙ,
    ищется глобально (`:192-194`);
  - `DetourNodeHash` — труп SPEC 101, читается только миграцией;
  - `DetourNodeLabel` — display-only снимок.
- Резолв строгий, fail-closed; честность в момент операции —
  `ui/configurator/business/detour_refs.go:36-107` (`ResetDetourNodeRefs`):
  переименование узла сбрасывает ссылки с предупреждением.
- Топологический порядок и каскады: `core/config/detour_topo.go:59`.
- Выбор в UI: `ui/configurator/business/detour.go:47-235`
  (`DetourOptions` / `DetourOptionsWithNodes`) — включая
  `localSubscriptionGroupTags` (`:237-258`) — локальные группы подписок
  как цели detour.

### 1.6 Clash API: группы и переключение

- Список групп читается из СГЕНЕРИРОВАННОГО config.json:
  `ui/clash_api_tab.go:167` (`GetSelectorGroupsFromConfig`),
  `core/config/config_loader.go:58`.
- Remote: группы у демона машины — `ui/clash_api_tab_selector_reload.go:56-78`
  (`collectSelectorGroups`, гейт по scope), `RemoteDaemonGroups`.
- Выбранная группа per-scope: `core/services/api_service.go:36, :199-225`
  (`SelectedClashGroupIn`) — runtime-only.
- `LastSelectedProxyByGroup map[group]proxy` — `api_service.go:40-51` —
  in-memory; ПЕРСИСТЕНТНЫЙ выбор селектора живёт в cache.db ядра под
  (тег группы → тег члена). DRAFT это сохраняет («кэш ядра, как сегодня»).
- Переключение по тегам: `/remote/machines/{id}/proxies/switch`
  (`core/debugapi/remote_endpoints.go:87`).

### 1.7 Remote-машины / daemon (lxd)

- Деплой возит ГОТОВЫЙ config (байты): `core/services/lxd_remote_deploy.go:46`
  — теги уезжают на машину внутри конфига; у машины СВОЙ cache.db, деплой с
  переименованными тегами оставляет там протухший выбор селектора.
- Полный HTTP-фасад завязан на теги и схему state
  (`core/debugapi/remote_endpoints.go:78-…`):
  `/remote/machines/{id}/groups`, `/proxies?group=`, `/proxies/switch`,
  `/pool?group=`, `/outbounds` (теги ядра машины), `/state/full`,
  `GET/PATCH /state/rules`, `/state/dns`, `/state/outbounds/resolved`,
  `/profile/copy-from` (перенос wizard-профиля между машинами).
- Локальные близнецы: `core/debugapi/state_endpoints.go:9-16` —
  `PATCH /state/rules` и `PATCH /state/dns` принимают ТЕКУЩУЮ схему
  (rules[] c body.outbound, dns_options flat).
- Профилировщик машины: `ui/machine_profiler.go` + `ui/traffic/*` —
  outbound-теги приходят В СОБЫТИЯХ от ядра (display + in-memory фильтр
  `ui/traffic/by_client_view.go:92-104`); ничего не персистится — к
  переименованиям устойчив (старые события покажут старый тег, и только).

### 1.8 Бэкап

- `core/backup/types.go`:
  - `:47` — секция `Chains []Chain`, merge по Tag (`:156-165`, контракт 0.7.1);
  - `:71-84` — тройня DetourTag / DetourNodeSourceID+DetourNodeTag + Label;
  - `:91` + `:176-181` — `TagPolicy{Prefix, Postfix, Mask}` — mask ЖИВ;
  - `:98-111` — `Source.Outbounds []Direction` — ЛОКАЛЬНЫЕ Направления
    источника в канонической форме (addOutbounds, preferredDefault переносятся);
  - `:115` — `ExposeGroupTagsToGlobal`;
  - `:195-198` — `Server.NodeTag`: «ТЕГ узла, на него ссылаются rules[].outbound».
- Импорт: `core/backup/import.go:147-192` — дедуп Направлений и цепочек по
  тегам, `:242-245` перенос тройни detour, `:260-264` перенос
  ExposeGroupTagsToGlobal и TagSpec с Mask.

### 1.9 Fold (умирающий предок FolderReplace)

- `core/config/source_folds.go:41-67` — материализация: fold → записи в
  `ps.Outbounds` + `ps.ExcludeFromGlobal = true` (`:62`) +
  `ps.ExposeGroupTagsToGlobal = true` (`:64`), маркеры `WIZARD:auto/selector`.
- Теги групп ДЕРИВАТИВНЫЕ: `core/config/configtypes/source_fold.go:93-99` —
  `EffectiveTagPrefix(tagPrefix, sourceIndex)+"auto"/"select"`; при пустом
  префиксе префикс = `"<index+1>:"` — тег зависит от ПОРЯДКА источников.
- `ExcludeFromGlobal` / `ExposeGroupTagsToGlobal` протянуты сквозь весь
  state-слой: `core/config/configtypes/types.go:135-149`,
  `core/state/adapter_source.go:32-33`, `core/state/sync_to_connections.go:119`,
  `core/state/sync_to_legacy.go:28`, `core/state/connections.go:108`,
  фильтр пула — `core/config/outbound_filter.go:16-28, :163-165`.

### 1.10 Переименование Направления — реестр ссылок

`ui/configurator/business/direction_rename.go:81-209` — ЕДИНСТВЕННОЕ место,
знающее все классы ссылок на тег Направления (заявлен принцип «новая ссылка —
сюда»): (1) сам тег + addOutbounds всех Направлений, включая
`model.ParserConfig.ParserConfig.Proxies[i].Outbounds` и
`model.Sources[i].Outbounds` (умирают с локальными Направлениями);
(2) цели правил; (3) route.final в двух местах; (4) outbound-переменные
пресетов; (5) хопы цепочек; (6) dns detour. Двойник `-auto` переименовывается
парой (`:89-90`), суффикс зашит и в `core/config/direction_twins.go:31`.

`DirectionTagTaken` (`direction_rename.go:33-72`) — проверка занятости имени:
Направления + их авто-двойники + GetAvailableOutbounds. Replace-теги папок в
этой проверке не существуют.

### 1.11 Материализация: что сносится и кто это сейчас читает

- `.raw`-кэш: `core/rebuild_raw_cache.go:18-70` — сборка ПАРСИТ
  `bin/subscriptions/<id>.raw` на каждый rebuild; строгий отказ при неполном
  кэше (`ErrRawCacheIncomplete`, `:158-160`); писатель —
  `core/config_service_subscriptions.go:305`.
- `DisabledNodes map[identity]unixtime` с TTL/GC —
  `core/config/configtypes/types.go:213-229`; ключ = raw-тег провайдера,
  uniquified, ДО prefix/mask (совместимо с node.enabled DRAFT); плюс
  debugapi API (`core/debugapi/disabled_nodes_api_test.go`), бэкап
  (`core/backup/export.go`, `import.go`), UI-кэши
  (`ui/configurator/business/preview_cache.go`, `source_node_counts.go`).
- `Meta.PreviewNodes`: писатель `core/config_service_subscriptions.go:269-276`;
  читатели — `ui/configurator/tabs/source_tab.go:401-404, :907`,
  `ui/configurator/tabs/source_chain_hops.go:151, :181` (выбор хопов цепочки!),
  `ui/configurator/tabs/source_edit_window.go:52-67`,
  `core/state/connections.go:262`.

---

## 2. Конфликты кода с моделью DRAFT и пробелы модели

### К1. Chain.Hops — финальные теги, DRAFT требует NodeLink[]
`configtypes/types.go:726` + `chain_nodes.go:55-194`. Хоп «NL» после
tag_prefix «proton-» лежит в state как «proton-NL». Миграция строки в
NodeLink{folderId, tag} требует ОБРАТИТЬ tag policy и uniquify — обратной
функции нет (uniquify «-2»/«-3» зависит от порядка обхода). Единственный
надёжный путь — резолвить на миграции через живой индекс узлов
(nodesByTag уже есть в ResolveChainSources), а нерезолвящийся хоп —
fail-closed с warning. В DRAFT механика миграции hops не описана вообще.

### К2. rules[].outbound / route.final / dns.detour остаются плоскими тегами — а реестр rename неполон для новой модели
DRAFT вводит новые ссылочные имена: FolderReplace.tag (+ дериватив
`<tag>-auto`) и верхние узлы, которые «Направление без фильтра видит все до
единого» — т.е. узловые теги становятся членами групп, а replace-теги —
целями Направлений/NodeLink. Но:
- `direction_rename.go` умеет переписывать только тег НАПРАВЛЕНИЯ;
- переименование FolderReplace.tag сегодня не существует как операция —
  придётся строить второй реестр ссылок (Направления addOutbounds, NodeLink
  detour/hops, cache.db-предупреждение);
- переименование верхнего узла (tag = имя в конфиге, DRAFT: «он же тег»)
  сегодня закрыто ResetDetourNodeRefs (сброс, не перепись) — для NodeLink
  из папок/цепочек этой механики нет.
`rule_target_reset.go:35-83` обязан включить replace-теги в known, иначе
первая загрузка мигрированного state сбросит живые правила на direct.

### К3. Смерть локальных Направлений — sources[].outbounds прошит в 8+ слоёв
`configtypes/types.go` (ProxySource.Outbounds), `core/state/adapter_source.go`,
`sync_to_connections.go`, `sync_to_legacy.go`, `connections.go:108`,
`source_folds.go:58` (fold их ПЕРЕЗАПИСЫВАЕТ на каждой подготовке),
`outbound_generator.go:1069-1337` (local selectors в эмиссии),
`direction_rename.go:123-130`, `detour.go:237-258`
(localSubscriptionGroupTags как цели detour), бэкап `types.go:98-111` +
`import.go`. DRAFT говорит «роль забрал FolderReplace», но у локальных
Направлений есть ФИЛЬТРЫ и addOutbounds — DRAFT теряет их с warning; бэкапы
контракта 0.7.1 с sources[].outbounds останутся на руках у пользователей:
импорт обязан конвертировать (fold-совместимые → replace, остальные →
warning), иначе restore v1.5.x-бэкапа молча потеряет маршрутизацию.

### К4. Fold-теги позиционные — фиксация явного tag при миграции обязательна
`source_fold.go:94-99` + `EffectiveTagPrefix`: без префикса тег группы =
`"<sourceIndex+1>:auto"`. Тег зависит от ПОРЯДКА источников — то, что DRAFT
чинит явным FolderReplace.tag. Миграция fold→replace обязана вычислить
сегодняшний дериватив (с учётом индекса на момент миграции!) и записать его
как явный tag — иначе выбор в cache.db и ссылки detour на `N:select`
протухнут. Пограничный случай: два запуска с разным порядком источников до
миграции дают разные теги.

### К5. TagPolicy.Mask: у цепочек mask = ТЕГ, терять его нельзя
DRAFT выкидывает mask «с warning». Но `chain_nodes.go:35-36`: тег узла
цепочки — это `src.TagMask`; `adapter_source.go:16`: server → `TagMask=Label`.
Т.е. mask сегодня — не шаблон, а ХРАНИЛИЩЕ имени для server/chain источников.
Миграция обязана различать: mask у подписки (шаблон `{$server}` — терять с
warning) и mask у server/chain (это Node.tag — переносить). Плюс mask жив в
бэкапе (`backup/types.go:180`) и в Xray-парсере
(`subscription/source_loader.go:1179`).

### К6. DetourNodeSourceID (ULID) → NodeLink.folderId: у верхних узлов id исчезает
`configtypes/types.go:177-198`. Сегодня ссылка на узел standalone-сервера —
(ULID источника, identity-тег). В DRAFT id есть ТОЛЬКО у папки; верхний узел
адресуется голым тегом в корневом неймспейсе. Миграция тройни: source —
подписка/папка → folderId; source — standalone server → NodeLink{null, tag},
и тег обязан быть уникален в корне (DRAFT это требует, но сегодняшний state
уникальность верхних NodeTag не гарантирует — гарантирует только build-time
MakeTagUnique). Нужна валидация на миграции. Переходная форма «тег без
source_id = финальный, ищется глобально» (`:192-194`) семантически совпадает
с NodeLink{null, tag} — удачно, можно схлопнуть.

### К7. Рантайм-адресация по тегу переживает пересборку, но не переезд
`/chains/{tag}/probe`, `<chain>#<pos>` (`chain_validate.go:100`),
`SetChainPositionEnabled(chainTag,...)` (`servers_node_info_chain.go`),
remote-вариант (`lxd_remote_transport.go:379-389`), группы Clash API и
cache.db — всё адресует ЖИВОЕ ядро финальными тегами. Под DRAFT цепочка
может жить в папке → её финальный тег получает prefix/postfix папки;
перенос узла/цепочки в папку или правка TagPolicy = смена финального тега =
UI теряет адресацию до рестарта ядра + cache.db-выбор селекторов протухает.
DRAFT принимает cache.db «как сегодня», но не оговаривает, что ПЕРЕЕЗД в
папку — тоже rename со всеми последствиями (предупреждение обязано быть).

### К8. Материализация ломает публичные API-поверхности state
`GET/PATCH /state/rules`, `/state/dns`, `/state/full`
(`state_endpoints.go:9-16`) и их remote-близнецы + `/profile/copy-from`
(`remote_endpoints.go:78-91`) фиксируют текущую схему. DRAFT меняет корень
(смерть `connections`, плоские sources[]/directions[], nodes[] в подписке,
node.enabled вместо disabled_nodes). Machine-парк — версионный skew: лаунчер
новой версии, PATCH'ащий state машины со старой (или copy-from между
машинами разных версий), обязан либо договориться о версии схемы, либо
migration-on-read на обеих сторонах. В DRAFT remote-план не упомянут.

### К9. Смерть PreviewNodes задевает выбор хопов цепочек
DRAFT: «превью читает nodes[] напрямую». Но
`ui/configurator/tabs/source_chain_hops.go:151, :181` строит СПИСОК ХОПОВ
из PreviewNodes (парсит URI заново) — это не превью, а рабочий ввод. С
материализацией список хопов обязан переехать на nodes[] тех же
подписок — с финальными или identity-тегами? (связан с К1: если hops =
NodeLink, форма должна отдавать (folderId, tag), а не строку).

### К10. Ленивый кэш и .raw: строгий отказ сборки исчезает — семантика меняется
`rebuild_raw_cache.go:47-70`: сегодня отсутствие .raw хоть одной enabled
подписки = отказ сборки (fail-closed, SPEC 052). После материализации
«данные всегда есть» — но есть и НОВЫЙ режим лжи: nodes[] могут быть
пустыми, потому что fetch ещё ни разу не прошёл, и сборка молча соберёт
конфиг без подписки. SPEC 113-A охраняет от затирания, но не от «честно
пусто с рождения» — Направление на пустую папку эмитится заглушкой
(`direction_twins.go:223-260`). Нужен эквивалент строгого отказа или явный
маркер «не фетчилось ни разу».

### К11. dns.detour вне охраны — и rename, и санитайзер
`renameDNSDetour` (`direction_rename.go:220-257`) знает только теги
Направлений; граф-санитайзер DNS не покрывает. Если под DRAFT dns.detour
сможет указывать на replace-тег или верхний узел — висячая ссылка не
ловится ни на rename, ни на сборке (ядро упадёт на check/run). Памятка
«граф-санитайзер сборки: новое ребро — туда» относится и к dns.detour.

### К12. Твины: `-auto` теперь в двух механиках
`direction_twins.go:31` (Направления) и DRAFT FolderReplace mode=both
(`<tag>-auto`). `DirectionTagTaken` (`direction_rename.go:33-72`) проверяет
коллизии только среди Направлений и GetAvailableOutbounds — replace-теги и
их `-auto`-двойники в проверку занятости имён не входят; без расширения
можно завести Направление `X` при папке с replace-тегом `X-auto` наоборот.

---

## 3. Что НЕ конфликтует (проверено)

- Traffic Profiler (`ui/traffic/*`, `ui/machine_profiler.go`) — теги только
  из live-событий ядра, display + in-memory фильтры, ничего не персистится.
- `LastSelectedProxyByGroup` (`api_service.go:40`) — in-memory, умирает с
  процессом; персистентность выбора — целиком cache.db ядра (совпадает с DRAFT).
- Идентичность DisabledNodes (raw-тег до prefix/mask, `types.go:216-218`)
  концептуально совпадает с node.enabled DRAFT — миграция ключей прямая
  (identity-тег = тег узла в папке).
- Строгий fail-closed резолв ссылок (SPEC 112-A, детур-топология
  `detour_topo.go`) — совпадает с «одна механика резолва, один fail-closed»
  для NodeLink/hops.
- resetForeignRuleTargets + правило «правило на direct, а не выключить» —
  переносится на новую модель без изменений семантики (при расширении known).
