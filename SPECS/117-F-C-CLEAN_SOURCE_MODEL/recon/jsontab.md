# Recon: JSON-представление в UI против модели SPEC 117 DRAFT

Тема: как UI сегодня гоняет legacy JSON-форму (ParserConfigJSON) и derived-кэши,
что станет с «вкладкой JSON» при canonical-модели, какие кэши умирают при
материализации узлов подписки.

Все пути — от корня `/Users/macbook/projects/singbox-launcher`.

---

## 1. Текущая архитектура: canonical + два derived-кэша

### 1.1 Модель

`ui/configurator/models/wizard_model.go`:

- `Sources []corestate.Source` — canonical (строка 74). UI мутирует напрямую.
- `GlobalOutbounds []configtypes.Direction` (строка 79) — canonical для Направлений.
- `ParserConfigJSON string` (строка 93) — **derived №1**: кэш сериализации
  legacy-формы. Заявленные роли: (а) «для JSON-editor виджета» — виджет уже
  удалён (SPEC 104), (б) дешёвый fingerprint для stale-detection в
  ParseAndPreview.
- `ParserConfig *config.ParserConfig` (строка 98) — **derived №2**: структурный
  кэш той же legacy-формы.
- `AsParserConfig()` (строка 265) — проекция canonical → legacy через
  `Source.ToProxySourceV4()` (`core/state/adapter_source.go:20`). Каждый вызов —
  свежий объект, «caller может мутировать».
- `RefreshDerivedParserConfig()` (строка 287) — пересчёт обоих кэшей после любой
  мутации Sources/GlobalOutbounds. Ошибки сериализации тихие.

Плюс параллельная пара конвертаций в core/state («две конвертации на каждый
Load/Save» из DRAFT):

- `core/state/save.go:36` → `syncConnectionsFromLegacy`
  (`core/state/sync_to_connections.go:24`) — Save: legacy → canonical, с
  матчингом по ID/URL/URI/тегу цепочки для сохранения ULID и меты.
- `core/state/load_v6.go:81`, `load_v5.go:50` → `syncLegacyFromConnections`
  (`core/state/sync_to_legacy.go:16`) — Load: canonical → legacy view.

### 1.2 MergeGUIToModel — что реально сливает

`ui/configurator/presentation/presenter_sync.go:305-353`
(`SyncGUIToModel`/`MergeGUIToModel`/`syncGUIToModel`).

Вопреки названию, к JSON-форме он отношения почти не имеет: переносит только
виджеты **SourceURLEntry** (поле Add), **FinalOutboundSelect** и вкладку DNS
(строки 357-436). Sources мутируются напрямую обработчиками
(Add/Edit/Delete/чекбоксы) с последующим `RefreshDerivedParserConfig()`.
Вызывается на каждую смену вкладки (`configurator.go:626`), закрытие
(`configurator.go:960`), перед фоновым parse (`presenter_async.go:43`), перед
сборкой «Итога» (`final_tab.go:330` — синхронно на UI-потоке через fyne.Do).
При canonical-модели механизм остаётся (он про виджеты, не про JSON), но
защитные ветки «keep model» и `WizardWidgetsReady` не затрагиваются рефакторингом.

### 1.3 Обратные синки (то, что DRAFT объявляет вне закона)

Обратный поток legacy → canonical в UI существует в **трёх** местах:

1. **`ApplyParserConfigFromCurrentJSON`**
   (`presenter_sync.go:441-463`): при входе на вкладку Направлений
   (`configurator.go:640-647`) парсит `model.ParserConfigJSON` обратно в
   `model.ParserConfig` (+ валидация + нормализация). В `Sources` НЕ синкает —
   т.е. derived-структура становится рабочей для конфигуратора Направлений, минуя
   canonical.

2. **`onConfiguratorApply`** (`ui/configurator/tabs/source_tab.go:995-1027`):
   outbounds-конфигуратор мутирует `m.ParserConfig` (legacy view), Apply копирует
   назад: `m.GlobalOutbounds = ParserConfig.Outbounds` и — по совпадению
   индексов — `m.Sources[i].Outbounds = proxies[i].Outbounds` (локальные
   Направления источника). Комментарий прямо признаёт хрупкость: «1:1 порядок,
   поэтому обратный sync безопасен по тому же индексу».

3. **`ParseAndPreview`** (`ui/configurator/business/parser.go:213`):
   `model.ParserConfig = &parserConfig` — записывает свежераспарсенную из
   JSON-строки структуру обратно в модель (после Sync/Migrate preset-патчей на
   ней же, строки 101-122).

Дополнительно `RefreshAfterPresetToggle` (`presenter_sync.go:241-271`) делает
**двойную запись**: `SyncOutboundsWithTemplate` и на `model.GlobalOutbounds`
(canonical), и на `model.ParserConfig.ParserConfig.Outbounds` (derived), затем
Refresh — три представления одного списка синхронизируются вручную.

### 1.4 ParseAndPreview: JSON-строка как транспорт и fingerprint

`ui/configurator/business/parser.go:39-220`:

- Вход — не `AsParserConfig()`, а **строка** `model.ParserConfigJSON`
  (строка 52): trim → size-validate → `json.Unmarshal` → `ValidateParserConfig`.
  То есть на каждый разбор derived-кэш сериализуется (в Refresh) и тут же
  десериализуется обратно.
- Строка 170: `if strings.TrimSpace(model.ParserConfigJSON) != parserConfigJSON`
  — сравнение строк как **stale-fingerprint**: если за время генерации юзер
  изменил конфиг, результат выбрасывается, `BuildReportGen = 0`.
- Строки 199-201: открывает попытку отчёта сборки (`StartBuildReport`),
  номер уезжает в `model.BuildReportGen` — первая половина двухфазного
  конвейера «Итога» (SPEC 115).
- Результат: `model.GeneratedOutbounds/GeneratedEndpoints` (эмитированные JSON
  outbound'ов, []string), `OutboundStats`, `PreviewNeedsParse = false`.

### 1.5 Фоллбэки «ParserConfig == nil → распарси JSON-строку»

Паттерн размножен по коду — все читатели умеют жить в трёх состояниях
(структура есть / есть только строка / нет ничего):

- `ui/configurator/business/outbound.go:80-102` — `GetAvailableOutbounds` с
  **мемо по строке JSON** (`AvailableOutboundsMemoKey/Tags`,
  `wizard_model.go:210-211`, сброс в InvalidatePreviewCache); ошибки парса
  молча глотаются («expected while user is typing» — комментарий из эпохи
  живого редактора).
- `ui/configurator/business/detour.go:245` — `localSubscriptionGroupTags`.
- `ui/configurator/tabs/source_chain_tab.go:714-731` —
  `getParserConfigForChain` (кандидаты позиций цепочки, Направления).
- `ui/configurator/business/create_config.go:77` — гейт «ParserConfig is empty»
  по строке.
- `ui/configurator/tabs/source_tab.go:983-992` — CreateDirectionsTab лениво
  восстанавливает `m.ParserConfig` из строки.

## 2. Вкладка JSON: что есть сегодня

### 2.1 Глобальный редактор ParserConfig JSON — уже удалён

SPEC 104 снёс редактор с вкладки Направлений:

- `configurator.go:636-638` — «валидации JSON при уходе с вкладки больше нет —
  редактор убран, и править руками там нечего».
- `source_tab.go:972-976` — «Прежний редактор ParserConfig JSON отсюда убран…
  сырой JSON остался там, где он и нужен, — внутри окна одного направления».
- `presenter_async.go:38-40` — условие запуска превью подчищено после удаления.
- `presenter_ui_updater.go:31-38` — `UpdateParserConfig(text)` **игнорирует
  аргумент**: от прежнего «обновить entry» остался только
  `RefreshOutboundsConfiguratorList()`. Но сигнатура и ~15 вызовов
  `presenter.UpdateParserConfig(m.ParserConfigJSON)` (source_tab.go:101,212,240,
  825,1017; source_edit_window.go:329,663,1577; sources.go:153;
  sources_json.go:148,164) живы — мёртвый транспорт строки.

### 2.2 Per-source JSON tab (окно Edit Source)

`ui/configurator/tabs/source_edit_window.go:1099-1418` — вкладка «JSON» в окне
источника, три режима:

- **server** (строки 1103-1105, 1267-1321): показывает распакованный sing-box
  outbound — либо ручной `scratch.ConfigJSON` как есть (порядок полей автора),
  либо генерацию из URI **тем же путём, что сборка**
  (`LoadNodesFromSourceEx` → `config.EmitNodeJSONs`, строки 1286-1319).
  Редактируемая: «Apply JSON» кладёт compact-JSON в `Source.ConfigJSON`
  (строка 1205), «Reset to URI» стирает его (строка 1221) — URI снова источник
  истины.
- **subscription** (строки 1333-1382): read-only распаковка — горутина читает
  `state.ReadRawBody(subsDir, src.ID)` (строка 1352, кэш
  `bin/subscriptions/<id>.raw`), декодит, парсит
  `parsePreviewNodesFromBody`, фильтрует по `DisabledNodes`, применяет detour и
  рендерит. Состояние «Subscription has not been fetched yet» (строка 1353).
- **chain** (строки 1240-1265): `config.ChainOutboundObject` + `ChainEmitError`;
  редактируемая ради `rewrite` (обратный разбор только известных ключей,
  строки 1162-1196).

### 2.3 Canonical-сериализация уже показывается — в Overview

`ui/configurator/tabs/source_edit_overview.go:300-330` —
`appendStorageRecordSection`: блок «Storage record (state.json)» — это
`json.MarshalIndent(corestate.Source)`, т.е. ровно canonical-запись. Комментарий:
«Раньше этот снапшот был вкладкой JSON; переехал сюда, когда вкладка JSON стала
показывать распакованный sing-box outbound». Там же Overview показывает **raw
body подписки** (`readRawBodySmart`, строка 213).

### 2.4 Прочие JSON-окна

- `final_tab.go:386-418` — «Show config»: собранный config.json в отдельном
  окне, read-only с откатом ввода.
- `preview_node_info.go:105` — JSON одного узла превью.

### 2.5 Ответ на вопрос «что станет с вкладкой JSON при canonical-модели»

Разделение UI уже совпадает с DRAFT по духу: «как узел уедет в конфиг» (вкладка
JSON = emitJSON()) и «как записан в состоянии» (Overview = canonical). При
canonical-модели:

- **server**: вкладка JSON перестаёт быть «распаковкой» и становится прямым
  редактором `Server.body` — body и есть canonical (В1). Но семантика кнопок
  **инвертируется**: сегодня истина — URI, а ConfigJSON — переопределение
  («Reset to URI» возвращает генерацию из URI); в DRAFT истина — ВСЕГДА body, а
  raw — сырьё для явного Regen. «Reset to URI» → «Regen from raw» с откатом при
  неразбираемом raw. Путь генерации из URI на каждый рендер
  (source_edit_window.go:1286) умирает — body уже лежит.
- **subscription**: горутина с ReadRawBody/декодом/parsePreviewNodesFromBody
  умирает целиком — узлы материализованы, вкладка показывает body из nodes[]
  синхронно. Состояние «not fetched yet» исчезает (данные всегда есть — это и
  есть смерть ловушки «ленивый кэш ≠ данных нет»). Правка узла подписки
  запрещена моделью (несвободные узлы) — read-only остаётся, но уже по
  типу узла, а не по типу источника.
- **Overview Storage record** остаётся canonical-сериализацией (теперь
  Node/Folder), а вот **Overview raw body** теряет источник данных: .raw-кэш
  умирает, «цельного тела подписки больше нет» — остаётся только per-node
  `origin.raw`. В DRAFT судьба этого UI-блока не оговорена (пробел).

## 3. Derived-кэши: что умирает при материализации

| Кэш | Где | Судьба по DRAFT |
|---|---|---|
| `ParserConfigJSON` (строка) | wizard_model.go:93 | Умирает как рабочее поле: JSON-editor нет, fingerprint нужен другой (см. К3). Legacy-строка нужна максимум одноразовой проекции перед сборкой. |
| `ParserConfig` (структура) | wizard_model.go:98 | Умирает как хранимый кэш; остаётся одноразовая проекция без обратного синка. |
| `AvailableOutboundsMemoKey/Tags` | wizard_model.go:210 | Умирает вместе со строкой-ключом; фоллбэк-парс в outbound.go:95 тоже. |
| `PreviewNodes`, `PreviewNodesBySource`, `PreviewIgnoredSectionsBySource` | wizard_model.go:183-207, preview_cache.go | Умирают: превью читает nodes[] напрямую (DRAFT, блок про материализацию). `RebuildPreviewCache` — повторный разбор всех подписок «тем же парсером, что сборка» — не нужен: парсинг один раз, на fetch. IgnoredSections переезжают в fetch-диагностику. |
| `SourceNodeCounts` + `EnsureSourceNodeCounts` + `PreviewCacheGeneration` | wizard_model.go:193-200, source_node_counts.go | Ленивая машинерия (фоновый счёт секунды, generation-гонка, ключи-индексы) умирает: счёт = O(n) по материализованным nodes[] с node.enabled. |
| `GeneratedOutbounds/GeneratedEndpoints` + `PreviewNeedsParse` + `AutoParseInProgress` | wizard_model.go:104-105,168-169 | Радикально дешевеют: «разбор подписок» стадии больше нет, остаётся эмиссия из body. Wait-loop `EnsureOutboundsParsed` (presenter_save.go:514-537, поллинг 100мс до parseWaitTimeout) и вся защита от «двух горутин в одни поля» съёживаются. |
| `.raw`-кэш `bin/subscriptions/*.raw` | core/state/raw_cache.go | Умирает (DRAFT прямо): состояние само себе кэш, per-node raw в origin. Читатели: source_edit_window.go:1352 (JSON tab подписки), source_edit_overview.go:213 (raw body), core/config/subscription/source_loader.go (сборка). |
| `DisabledNodes map[string]int64` + TTL/GC | core/state/connections.go:181-192, source_loader.go:132-166 | Умирает → `node.enabled`; TTL-механика не переносится. Миграционный костыль SPEC 112 `applyMigratedDisabledKeys` (preview_cache.go:191) уходит вместе с ней. |
| `meta.PreviewNodes` | core/state/connections.go:262 | Умирает (DRAFT прямо; превью читает nodes[]). Пишется в core/config_service_subscriptions.go:274-276; UI-читателей не нашёл — уже полумёртв. |
| syncConnectionsFromLegacy / syncLegacyFromConnections | core/state/save.go:36, load_v6.go:81 | Умирают — «две конвертации на каждый Load/Save» и есть мишень DRAFT. С ними умирает матчинг по URL/URI/chain-tag при Save. |

## 4. Конфликты и пробелы (нумерация для сводки)

**К1. Три обратных синка legacy→canonical в UI.**
`ApplyParserConfigFromCurrentJSON` (presenter_sync.go:441; вызов
configurator.go:643), `onConfiguratorApply` (source_tab.go:995-1017, обратное
копирование по индексу в `m.Sources[i].Outbounds`), `ParseAndPreview`
(parser.go:213). DRAFT: canonical — рабочая модель, legacy — одноразовая
проекция, «обратного синка нет». Первый из трёх ещё и оставляет
Sources рассинхронизированными со структурой, по которой работает конфигуратор
Направлений.

**К2. `m.Sources[i].Outbounds` — локальные Направления источника.**
DRAFT: «Локальных Направлений источника БОЛЬШЕ НЕТ — их роль забрал
FolderReplace; в replace не ложатся (у них фильтры) — теряются с warning».
Живой код: onConfiguratorApply (source_tab.go:1005-1012), ProxySource.Outbounds,
`localSubscriptionGroupTags` (detour.go:237-267), Scope ≠ "For All" в
конфигураторе. Миграция и warning не написаны.

**К3. Fingerprint для stale-detection не определён.**
Сегодня выброс результата гонки держится на сравнении строк ParserConfigJSON
(parser.go:170) и на `PreviewCacheGeneration` (только для превью-кэша). Со
смертью строки нужен явный generation/ревизия canonical-модели; DRAFT механизм
не называет — пробел (естественный кандидат — расширить PreviewCacheGeneration
до ревизии модели).

**К4. «Сборка не парсит подписки» ломает двухфазный отчёт «Итога».**
Весь контракт SPEC 115 построен на том, что разбор подписок — первая половина
сборки: `PrepareFinalBuild` (presenter_save.go:553-578) перегоняет
ParseAndPreview, чтобы причины парсерной стадии (source_excluded, chain_failed,
naive_degraded) легли в живую попытку `BuildReportGen`; final_tab.go:355-378
прямо это документирует. При материализации парсинг происходит на **fetch**, в
другой момент времени и другим актором — где живут его причины и как попадают в
отчёт «Итога» (который собирается позже), DRAFT не говорит. Пробел: судьба
BuildReportGen-спарки и самих parse-диагностик при переносе разбора на fetch.

**К5. Все derived-карты ключуются индексом источника.**
`PreviewNodesBySource`/`SourceNodeCounts`/`PreviewIgnoredSectionsBySource` —
`map[int]`, отсюда generation-гонки (wizard_model.go:195-200) и кривые проверки
вроде `sourceIndex < len(m.PreviewNodesBySource)` для map (source_tab.go:401-403).
DRAFT-идентичность — тег в папке/корне. Карты умирают вместе с кэшем, но всякий
переживший индекс-ключ — конфликт с новой идентификацией.

**К6. ULID у каждого источника vs «id только у папки».**
DRAFT: id есть ТОЛЬКО у Folder, узел идентифицируется тегом. Сегодня ULID у
каждого Source (state.Source.ID), на нём: переход из отчёта «Итога»
(final_tab.go:196-251, modelSourceIDs/RevealSource), адресация ссылок SPEC 112-A
«source_id + identity-тег» (configtypes/types.go:95-103), имя .raw-файла,
матчинг при Save. Все эти механизмы переезжают на tag/folderId — в DRAFT
миграция ссылок описана для NodeLink, но судьба source_id в отчётах сборки не
оговорена.

**К7. defaults в проекции.**
`AsParserConfig` кладёт `m.Defaults.Reload` в `pc.Parser.Reload`
(wizard_model.go:280), `model.Defaults` зеркалит state.connections.defaults
(wizard_model.go:83). DRAFT уводит defaults из состояния в настройки приложения
— меняются модель, Load/Save и проекция.

**К8. Мёртвый транспорт `UpdateParserConfig(text)`.**
Аргумент игнорируется с SPEC 104 (presenter_ui_updater.go:31), но ~15 вызовов
продолжают таскать `m.ParserConfigJSON`. При рефакторинге — переименовать/убрать,
иначе сигнатура врёт, что строка кому-то нужна.

**К9. Двойная запись Направлений при preset-toggle.**
`RefreshAfterPresetToggle` (presenter_sync.go:249-260) синкает и canonical
GlobalOutbounds, и derived ParserConfig.Outbounds. При canonical-модели остаётся
одна запись; сейчас — образец рассинхронизации, которую DRAFT называет причиной
удара.

**К10. Overview raw body теряет данные.**
Блок raw body подписки (source_edit_overview.go:213) читает умирающий .raw-кэш.
DRAFT признаёт потерю «цельного тела» ценой решения, но UI-последствие
(чем заменить блок: конкатенация origin.raw? убрать?) не решено — пробел.

**К11. Судьба chain-резолва в превью.**
`RebuildPreviewCache` резолвит цепочки против списка Направлений из derived
ParserConfig (`previewDirectionTags`, preview_cache.go:147-163). При
материализации сам кэш умирает, но требование остаётся: пул для UI (flag picker,
кандидаты позиций source_chain_tab.go:743 по PreviewNodes) обязан включать
резолвнутые цепочки и — по DRAFT — replace-теги наравне с узлами. Куда
переезжает этот резолв (в модель? в ленивый view поверх nodes[]?) — не описано.

## 5. Резюме

Код уже наполовину прошёл путь DRAFT: canonical Sources — источник истины
(SPEC 052 phase 8), глобальный JSON-редактор снесён (SPEC 104), canonical-запись
показывается в Overview. Оставшийся хвост — ровно то, что DRAFT называет
проблемой: legacy ParserConfig живёт второй рабочей моделью (конфигуратор
Направлений мутирует его напрямую), JSON-строка служит транспортом в парсер и
fingerprint'ом, и три обратных синка держат всё это в согласии вручную.
Материализация nodes[] сносит целый этаж derived-кэшей (превью, счётчики,
generation-гонки, .raw, DisabledNodes+TTL, meta.PreviewNodes) — но рвёт
двухфазный отчёт «Итога» (К4) и оставляет четыре пробела: fingerprint (К3),
fetch-диагностика в отчёте (К4), raw body в Overview (К10), место chain/replace
резолва для UI-пула (К11).
