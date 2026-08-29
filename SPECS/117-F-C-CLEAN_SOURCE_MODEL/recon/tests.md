# Recon: тестовый ландшафт против SPEC 117 DRAFT (чистая модель источников)

Аудит существующих тестов, завязанных на структуры источников, против
`SPECS/117-F-N-CLEAN_SOURCE_MODEL/DRAFT.md`. Ветка develop, 2026-08-29.

Терминология этапов:
- **Этап 1** — снос обратного синка: canonical (`state.Connections`) становится
  рабочей моделью, legacy `ParserConfig` — одноразовая проекция перед сборкой,
  `syncConnectionsFromLegacy` умирает. Диск остаётся v6.
- **v7** — новая схема: плоский корень (`sources[]/directions[]/...`), смерть
  `connections`-обёртки, полиморфные Source (Server/Chain/Auto/Folder/Subscription),
  NodeLink вместо detour-тройни, Origin, FolderReplace вместо fold и локальных
  Направлений, материализация узлов подписки (смерть .raw-кэша, disabled_nodes-карты,
  PreviewNodes), mask и excludeFromGlobal убраны, defaults уходят в настройки.

Ключевой факт архитектуры: обратный синк живёт в
`core/state/save.go:36` (`syncConnectionsFromLegacy`) и
`core/state/load_v6.go:81` / `load_v5.go:50` (`syncLegacyFromConnections`).
UI уже частично canonical-first: `ui/configurator/models/wizard_model.go:74`
(`Sources []corestate.Source` — рабочая модель), `wizard_model.go:98`
(`ParserConfig` — derived-кэш). Обратный путь на UI-уровне —
`ui/configurator/presentation/presenter_sync.go:441`
(`ApplyParserConfigFromCurrentJSON`, JSON-editor).

---

## Категория (а) — сломает этап 1 (снос обратного синка)

Покрытие обратного синка ТОЧЕЧНОЕ — всего ~9 тест-функций в 7 файлах:

| Тест | Где | Что умирает |
|---|---|---|
| `TestConfigJSON_LegacyRoundTrip` | core/state/config_json_roundtrip_test.go:17 | прямой вызов обеих sync-функций + инвариант ID-стабильности (матчинг по URI/телу JSON на Save) |
| `TestDetourTag_LegacyRoundTrip` | core/state/detour_mapping_test.go:20 | round-trip detour через оба синка |
| `TestSyncConnectionsFromLegacy_KeepsRefAndID` | core/state/detour_node_ref_test.go:76 | обратный синк сохраняет Ref и ID |
| `TestLoad_V4Minimal` | core/state/state_test.go:52,77 | ассертит заполненный legacy-view `s.ParserConfig` после Load |
| `TestSaveLoadRoundTrip` | core/state/state_test.go:148 | мутирует legacy-view и полагается на Save→canonical |
| `TestRenameDirectionTouchesBothViews` | ui/configurator/business/direction_rename_test.go:105 | «оба вида» перестают существовать |
| `TestParseAndPreview_Discards/AppliesWhenJSONChanges…` (2) | ui/configurator/business/parser_stale_test.go:52,87 | stale-guard по тексту ParserConfigJSON — риск, зависит от судьбы JSON-editor вкладки |
| `TestValidateParserConfigJSON` | ui/configurator/business/validator_test.go:417 | валидация редактируемого legacy-JSON — тот же риск |

**Итог (а): 9 тест-функций (из них 3 — условные, если JSON-editor становится
read-only/умирает).** Прямая проекция (`ToProxySourceV4`,
`syncLegacyFromConnections` как build-проекция, `SerializeParserConfig`)
переживает этап 1 — тесты на неё в (а) не попадают:
core/state/adapter_source.go:20, ui/configurator/business/parser_test.go:13.

Отдельно: ассерты вида «Legacy ParserConfig view заполнен после Load»
(state_test.go:174-178) исчезают вместе с полем State.ParserConfig, если
проекция станет строиться только перед сборкой, а не храниться в State.

---

## Категория (б) — сломает v7 (новая схема)

### core/state — ~42 функции

- `v6_integration_test.go` (7, строки 11-294) — весь файл на `connections`-обёртке
  и `Save_AlwaysWritesV6`; при v7 переписывается под новый корень.
- `direction_storage_test.go` (6, :15-166) — directions уезжают в корень; adoption
  legacy-ключа `outbounds` меняет смысл.
- `disabled_nodes_roundtrip_test.go` (3) — карта `DisabledNodes map[string]int64`
  (connections.go:192) умирает → `node.enabled`.
- `raw_cache_test.go` (6) — `WriteRawBody/ReadRawBody` (.raw-кэш подписок)
  умирает целиком: «состояние само себе кэш».
- `connections_test.go` (4) — diskStateV5-shape c `Defaults{Reload,MaxNodes}`
  и `TagSpec{Mask}` (connections.go:49-51, :205) — Defaults уходят в настройки
  приложения, Mask выкинут; тесты переписываются под legacy-read-only.
- `legacy_migration_test.go` (8) — миграция v4 остаётся (только чтение), но
  целевая форма ассертов (`state.Connections.Defaults.Reload`,
  legacy_migration_test.go:115-119) меняется → переработка, не смерть.
- `detour_node_ref_test.go` (2 остальные), `detour_mapping_test.go` (1),
  `config_json_roundtrip_test.go` (1) — тройня
  `DetourNodeSourceID/DetourNodeTag/DetourTag` (connections.go:147,170-171)
  → NodeLink; `ConfigJSON` → `Server.body`.
- `state_test.go` (~4 из оставшихся 9) — ассерты формы `Connections`.

### core/backup — ~32 функции + 14 корпус-кейсов

Формат — внешний контракт 0.11.0 (types.go:1-52), несёт ВСЁ убиваемое:
detour-тройню (types.go:72-78), `disabled map[string]int64` (:95), `Fold`
(:113), `ExcludeFromGlobal` (:114), локальные `Outbounds` (:111), `Label` (:88).
- `backup_test.go` (22), `purity_test.go` (3), `schema_test.go` (6),
  `corpus_test.go` (1 раннер по `contract/corpus/backup/` — 14 нормативных
  кейсов, включая `disabled_nodes_by_hash`) — маппинг в state.Source
  переписывается; контракт формата требует bump'а, который DRAFT не описывает.
- `directions_test.go` (4), `file_test.go` (14), `rule_order_invariant_test.go`
  (2) — направления/правила/IO — большей частью переживут (см. (в)).

### core (корень) — ~17 функций

- `preview_nodes_test.go` (6) — `ExtractPreviewNodes` умирает вместе с
  `Meta.PreviewNodes` (connections.go:262).
- `rebuild_raw_cache_test.go` (5) — снапшот из .raw-кэша умирает.
- `refresh_meta_test.go` (4) — refresh пишет meta+raw-кэш → fetch теперь
  материализует nodes[]; переработка.
- `debugapi/disabled_nodes_api_test.go` (2) — API disabled-карты → enabled.

### core/config — ~96 функций

Смерть предмета:
- `expose_exclude_test.go` (5, :8-30) — `FilterNodesExcludeFromGlobal` и
  `expose_group_tags_to_global` убраны совсем.
- `node_hash_test.go` (10) + `node_hash_bare_emit_test.go` (7) — контент-хэш
  упразднён (SPEC 112), v7 добивает.
- `ira_state_migration_test.go` (4, :11-25) — миграция legacy-хэшей
  `detour_node_hash` умирает вместе с хэшами и тройней.
- `source_parse_failed_test.go` (6) — сборка больше не парсит подписки;
  сюжет «источник упал при parse на сборке» переезжает в fetch.

Переработка со сменой структур (семантика — «одна механика резолва, один
fail-closed» — сохраняется по DRAFT):
- detour-семейство: `detour_node_ref_test.go` (13 — резолв тройни),
  `detour_cascade` (4), `detour_sanitize` (6), `detour_group_cycle` (3),
  `detour_chain_emit` (4).
- chain-семейство: `chain_cycle` (4), `chain_emit` (10), `chain_validate` (5) —
  `SourceChain.Hops []string` (configtypes/types.go:726) → `[]NodeLink`.
- группы: `group_node_contract_test.go` (7 — схема "group" → Auto),
  `default_in_members_test.go` (4), `excluded_sources_test.go` (4 — реестр
  исключений keyed by source_id; id остаётся только у папки).

### core/config/subscription — ~51 функция

- `disabled_nodes_test.go` (10), `disabled_migration_test.go` (6),
  `e2e_disabled_flow_test.go` (4) — вся TTL-механика disabled-карты умирает
  («TTL-механика не переносится — костыль»). ВАЖНО: e2e-тест закрепляет
  «нода осталась выключенной, когда провайдер её переставил» — v7 ОСОЗНАННО
  отменяет это для случая временного пропадания узла (цена из DRAFT).
- `trusted_parse_test.go` (7) — SPEC 113-A guard: семантика переезжает с
  disabled-карты на «не трогать nodes[]» — переработка.
- `dedup_group_rebind_test.go` (4) — rebind состава группы после дедупа —
  при явных members у Auto меняется слой.
- `detour_test.go` (3), `detour_chain_test.go` (11) — прокидка detour в
  parse-слое → NodeLink.
- `meta_test.go` (~6 из 18 — части про PreviewNodes/preview-лимиты).

### ui — ~63 функции

- `tabs/disabled_node_toggle_test.go` (5) — disabled-карта.
- `tabs/preview_parse_reasons_test.go` (3), `tabs/preview_singbox_body_test.go`
  (2), `business/preview_dedup_test.go` (1), `business/preview_cache_chain_test.go`
  (3), `business/preview_target_test.go` (1), `business/source_node_counts_test.go`
  (5) — превью-конвейер (raw-кэш → parse → PreviewNodes) заменяется чтением
  nodes[] напрямую; ловушка «ленивый кэш ≠ данных нет» схлопывается.
- `tabs/source_chain_save_test.go` (6), `source_chain_pending_test.go` (1),
  `source_node_tag_buffer_test.go` (5) — scratch-буферы на `ProxySource.TagMask`
  (source_chain_save_test.go:69: «в ProxySource он живёт в TagMask») — mask
  выкинут, тег становится полем Node.tag.
- `business/detour_refs_test.go` (7), `detour_rename_e2e_test.go` (3) —
  NodeRef-тройня → NodeLink; rename-семантика меняется (идентичность = тег).
- `business/detour_test.go` (11 — переработка; `TestDetourOptions_
  ExcludesAllSubscriptionLocalGroups`:65 теряет предмет — локальных групп нет).
- `business/clone_source_test.go` (6) — клонирование под В2 (копии + subUrl).
- `business/wizard_integration_test.go` (2) — полный флоу на v6-формах.
- `outbounds_configurator/configurator_helpers_test.go` (~2 из 6 — fold-строки
  в CollectRows).

**Итог (б): ~300 тест-функций в ~60 файлах.** Из них грубо ~120 —
переработка с сохранением семантики (chain/detour-резолв, миграции,
SPEC 113-A), остальное — смерть предмета теста (raw-кэш, PreviewNodes,
disabled-TTL, fold, excludeFromGlobal, mask, node_hash, тройня).

---

## Категория (в) — переживёт

- **Корпус контракта**: `contract_test.go` (раннер URI-корпуса, 281 фикстура),
  `contract_body_test.go` (13 тел: base64/singbox/uri_list/vpn/wgconf/xray),
  `contract_emit_test.go` — parse/emit фрагментов; парсинг переезжает на fetch,
  но функции и конверт expected те же. Всего в корпусе 478 expected-файлов.
- **Парсеры** (`core/config/subscription/node_parser_*`, awg, xray_*, wgconf,
  ws_early_data, xhttp_v2, hysteria2, decoder, body_classify_*, dedup_test,
  uniquify_collision, parse_warnings, singbox_import*, singbox_sanitize,
  warp_reserved, wireguard_robustness, userinfo_space, add_server_form_uri,
  identity_stamp) — ~150 функций: URI/тело → body, без привязки к схеме state.
- **Эмиттеры** (`generator_*`, naive_degrade, loadbalance, flow_transport,
  ws_early_data_emit, outbound_share, share_uri_encode, manual_config_emit,
  varsubst, tun_interface_names) — ~80 функций.
- **Направления/твины**: `direction_twins_test.go` (17) — паттерн твинов
  переиспользуется FolderReplace-both; `contract_direction_test.go` (раннер,
  27 фикстур) — глобальные Направления остаются, НО fold/групповые фикстуры
  корпуса (`auto_twin_excludes_group_nodes` и пр.) потребуют контрактной правки.
- **`core/build`** — работает на configtypes-проекции (rules/DNS/directions):
  переживает этап 1 полностью; на v7 — точечные правки. Исключение —
  `golden_test.go`: единственный сценарий `real-v088` содержит state.json
  v6-формы и по умолчанию SKIPPED (golden_test.go:47-50) — при v7 протухнет
  молча.
- **core/state rules/dns/target**: rule_order (15), rule_types (17),
  rule_order_invariant (10), rule_identity (3), sync_dns (7), target_meta (2),
  target_roundtrip (1), explicit_patch_load (3), empty_user_patch (1),
  provider_announce_message (3 — SubMeta остаётся в модели).
- **core/backup**: directions_test (4), file_test (14), rule_order_invariant (2).
- **UI**: sources_json_test (6 — carve singbox JSON), config_nodes_test (9 —
  derive из body), add_server_dialog_test (13 — URI-формы), wizard_dns (11),
  dialogs/models rules-семейство, settings_* — правила/DNS/настройки, к
  источникам не привязаны. Компиляционно заденет смена полей WizardModel
  (все 21 файл с `WizardModel{...}`), но это механическая правка.

**Итог (в): ~250 тест-функций source-ландшафта + 281 URI-фикстура корпуса**
(плюс весь rules/dns/template-массив вне темы источников).

---

## Сводные числа

| Категория | Тест-функций | Файлов | Характер |
|---|---|---|---|
| (а) этап 1 | ~9 (3 условные) | 7 | смерть предмета (сам синк) |
| (б) v7 | ~300 | ~60 | ~180 смерть предмета, ~120 переработка |
| (в) переживёт | ~250 + корпус | ~70 | парсеры, эмиттеры, корпус uri/body, rules/dns |

Всего в репо 55k строк тестов; source-ландшафт — примерно половина.

## Конфликты кода с DRAFT и пробелы модели

1. **Backup — внешний контракт 0.11.0** несёт убиваемое: detour-тройню
   (core/backup/types.go:72-78), disabled-карту (:95), Fold (:113),
   ExcludeFromGlobal (:114), локальные Outbounds (:111), Label (:88).
   DRAFT не описывает bump формата и судьбу общего с LxBox корпуса
   `contract/corpus/backup/` (14 нормативных кейсов, вкл.
   `disabled_nodes_by_hash`).
2. **TagMask перегружен**: в ProxySource он несёт ТЕГ узла цепочки/сервера, а
   не маску (ui/configurator/tabs/source_chain_save_test.go:69); снос mask по
   DRAFT заденет канал передачи тега, не только шаблон.
3. **ID-стабильность источников** сегодня гарантируется матчингом обратного
   синка (core/state/config_json_roundtrip_test.go:44-50); после сноса синка
   инвариант «json-only источник не получает новый ULID на каждый Save» никто
   не охраняет — в DRAFT замены нет (у Node вообще нет id — только тег).
4. **e2e_disabled_flow_test.go закрепляет отменяемое поведение**: «нода
   осталась выключенной после перестановки у провайдера» — v7 осознанно
   платит потерей enable-отметки при временном пропадании узла; тест придётся
   не чинить, а удалять с фиксацией смены контракта.
5. **Корпус direction** содержит fold/групповые фикстуры
   (auto_twin_excludes_group_nodes и др.) — нормативные для LxBox; переход на
   FolderReplace = правка общего корпуса, DRAFT молчит.
6. **debugapi** отдаёт формы state наружу (disabled_nodes_api, state_endpoints,
   snapshot) — v7 меняет их без упоминания в DRAFT.
7. **golden real-v088 skipped by default** (core/build/golden_test.go:47-50) —
   единственный полный state-фикстур-сценарий протухнет молча при v7.
8. **Chain.Hops []string** (configtypes/types.go:726) — все chain-тесты и
   backup-контракт (chains по tag, 0.7.1) на строковых тегах; NodeLink с
   folderId ломает и внешний контракт цепочек.
