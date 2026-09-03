# Recon: конвейер fetch→nodes сегодня vs модель SPEC 117 DRAFT

Аудит существующего кода против SPECS/117-F-N-CLEAN_SOURCE_MODEL/DRAFT.md.
Тема: порядок стадий разбора источника, skip[], max_nodes/Truncated, dedup,
identity/uniquify, disabled_nodes, тег-политика.

## 1. Фактический порядок стадий (код)

Главный конвейер — `LoadNodesFromSourceEx`,
`core/config/subscription/source_loader.go:494-943`. Выполняется **на каждой
сборке/превью** (canonical → ProxySource → parse), не при fetch: сегодня fetch
и парсинг разнесены только кэшом raw-тела.

Порядок на один источник:

1. **Fetch / кэш** (source_loader.go:551-562): хук `LookupCachedBody`
   (source_loader.go:27; ставится Rebuild'ом — core/rebuild_raw_cache.go:83-88 —
   поверх `bin/subscriptions/<id>.raw`) → иначе `FetchSubscription`.
   Флаг доверия `bodyRead` (source_loader.go:524, :578): источник без URL
   достоверен всегда; подписка — когда тело получено.
2. **Нормализация и классификация тела** (source_loader.go:587-592):
   CRLF→LF, `ClassifySubscriptionBody` → ветки vpn:// (Amnezia),
   wg-quick conf → wireguard://-URI, sing-box JSON, Xray JSON array,
   построчный URI-список.
3. **Цикл по записям** — порядок внутри цикла одинаков во всех ветках:
   - **кап max_nodes** (source_loader.go:614, :665, :715, :754, :801, :833,
     :865): жёсткая константа `configtypes.MaxNodesPerSubscription = 3000`
     (core/config/configtypes/types.go:75). Пер-сорсный `state.Source.MaxNodes`
     в парсинг НЕ провозится (в ProxySource поля нет) — см. §4.
   - **skip[]-фильтры** — применяются **внутри парсеров**, до появления узла
     в списке: `ParseNode(line, proxySource.Skip)` (source_loader.go:764, :804,
     :871), `shouldSkipNode` (core/config/subscription/node_parser_core.go:588,
     вызов :469; также node_parser_http.go:145, node_parser_masque.go:179 и
     др.), для JSON-тел — параметром в `ParseSingboxBody` (source_loader.go:655)
     и `ParseNodesFromXrayJSONArrayEx` (source_loader.go:695). Ключи фильтра:
     tag/host/label/scheme/fragment/comment/flow
     (node_parser_core.go:558-578, `getNodeValue`); паттерны — общий
     `configtypes.MatchesPattern` (regex + негация), тот же, что у фильтров
     Направлений.
   - **dedup** (`dedup.accept`, source_loader.go:618, :676, :775, :809, :833,
     :879): пер-сорсный `sourceDedup`
     (core/config/subscription/server_conn_key.go:44-100). Ключ —
     `dedupSignature` = полная эмиссия узла без tag/detour через хук
     `LegacyNodeIdentityHashFunc` (server_conn_key.go:34-42). Живёт один
     разбор, в состояние не пишется. Порядок обязателен: дедуп ДО тегов,
     иначе дубль получил бы уникализованный тег «X-2» и свою идентичность
     (комментарий source_loader.go:513-517). Карта `collapsedInto` (тег дубля →
     тег выжившего) кормит перепривязку групп. Исключение: Xray-массив дедупится
     внутри `ParseNodesFromXrayJSONArray` (ownership по подписи; дедуп внутри
     элемента, не по подписке — source_loader.go:704-711).
   - **identity + теги** (`applyURINodeTags` source_loader.go:951-976;
     `applyTagsToSingboxNode` :1002-1053; `applyTagsToXrayNode` :1180-1211):
     1. `StampNodeIdentity` (source_loader.go:67-84) — идентичность = СЫРОЙ
        провайдерский тег, уникализированный пер-сорсным счётчиком `idCounts`
        (`X`, `X-2`; кандидат проверяется на занятость —
        `uniquifyAgainstCounts` :100-113). Снимается строго ДО тег-политики:
        правка префикса не должна уводить отметки выключения (SPEC 112).
        Группы (SchemeGroup) идентичности не получают (:68).
        Источник-СЕРВЕР: identity = `singleNodeSourceTag` = TagMask без
        переменных (:961-996), т.е. NodeTag/Label из state
        (core/state/adapter_source.go:49-51, 58: `TagMask: s.NodeTagOrLabel()`).
     2. Тег-политика `applyTagPrefixPostfix` (:1217-1238): TagMask заменяет
        весь тег и глушит prefix/postfix; все три поддерживают переменные
        `{$tag} {$scheme} {$protocol} {$server} {$port} {$label} {$comment}
        {$num}` (`replaceTagVariables` :1249-1275).
     3. `textnorm.NormalizeProxyDisplay` → `MakeTagUnique` с **глобальным**
        `tagCounts` конфига (:400-410) — конфиговые теги уникальны глобально,
        идентичность — только внутри источника (:509-511).
4. **Перепривязка групп** `rebindImportedGroupNodes` (source_loader.go:688,
   :736, реализация :1073-1144): состав импортированных групп переписывается
   с исходных тегов на итоговые; член, схлопнутый дедупом, перепривязывается
   на выжившего; группа без членов удаляется (пустой urltest роняет ядро).
5. **Detour источника** `ApplySourceDetour` (source_loader.go:909-913,
   :1155-1177): штампует `node.Outbound["detour"] = DetourTag` **в тело
   outbound'а на парсе** (пропуск: Xray Jump, wireguard+listen_port; при
   ссылке на узел — DetourNodeTag — не штампуется вовсе, резолвится
   генератором на проходе 2, fail-closed).
6. **Фильтр disabled_nodes** `filterDisabledNodes` (source_loader.go:923,
   реализация :170-216): ключи — identity-теги; выключенные выпадают из
   выдачи, у переживших продлевается timestamp. Перед фильтрацией — миграция
   legacy-ключей (64-hex контент-хеш SPEC 094/101 → identity,
   `migrateLegacyDisabledKeys` :263-331) под ЗАКОНОМ чисток (:240-247):
   выбрасывать отметку можно только при `trustedParse = bodyRead &&
   len(nodes)>0 && skippedDueToLimit==0` (:922).
7. **GC отметок** — НЕ в парсере: только на пути успешного сетевого
   обновления (`core/config_service_subscriptions.go:94-110`),
   `GCDisabledNodes` с TTL = clamp(3×интервал, 24h, 30d)
   (source_loader.go:139-155); интервал: `src.Update.IntervalHours` →
   `Meta.ProfileUpdateIntervalHours` → пол 24h
   (config_service_subscriptions.go:97-104).

Каналы, минующие тело подписки, идут тем же циклом: direct link в Source
(:792-819), manual `config_json` (:826-841, приоритетнее Connections),
legacy Connections (:844-885) — все через dedup.accept и applyURINodeTags.

## 2. Truncated и max_nodes

- Кап парсинга — **константа 3000** (types.go:73-75), одинаковая для всех.
- Пер-сорсный `Source.MaxNodes` и `Defaults.MaxNodes`
  (core/state/connections.go:51, :120) влияют ТОЛЬКО на бейдж:
  `merged.Truncated = NodesCountFetched > effectiveMax`, резолв
  `src.MaxNodes → defaults.MaxNodes → state.DefaultMaxNodes`
  (core/config_service_subscriptions.go:280-287). Т.е. сегодня настройка
  max_nodes НЕ защищает — защищает константа; поставить подписке max_nodes=100
  = получить бейдж «truncated», но распарсятся всё равно до 3000.
- `NodesCountFetched`/`PreviewNodes` считаются при fetch по сырому телу
  (config_service_subscriptions.go:273-278) — по DRAFT PreviewNodes умирает.

## 3. Кто персистит результаты разбора

- `SourceLoadResult.DisabledNodes/DisabledMigrated` (source_loader.go:433-471)
  сохраняет ТОЛЬКО путь превью Wizard
  (ui/configurator/business/preview_cache.go:90-92,
  `applyMigratedDisabledKeys`). Пути сборки (core/config_service.go:159,
  core/rebuild_raw_cache.go:115) зовут тонкий `LoadNodesFromSource` и
  выбрасывают карту — миграция legacy-ключей на build-пути не персистится
  (доживает до открытия Wizard). С материализацией по DRAFT вся эта асимметрия
  умирает вместе с картой.
- Отметки хранятся в `state.Source.DisabledNodes`
  (core/state/connections.go:181-192), едут в сборочную форму через
  adapter_source.go:41. Бэкап экспортирует карту как `sub.Disabled`
  (core/backup/export.go:222-226, import.go:270-273) — контракт 0.7.1.

## 4. Раскладка стадий на модель DRAFT «fetch → merge → nodes[]»

Что естественно ложится **на fetch** (один раз, при обновлении):
скачивание, классификация тела, парсинг в body, **skip[]**, **dedup по
подписи**, пер-сорсная уникализация сырых тегов, кап max_nodes,
SPEC 113-A-гейт (недостоверный ответ не трогает nodes[]), merge по тегу
с сохранением enabled/detour, обновление meta.

Что остаётся **на сборке**: эмиссия body (`emitJSON`), резолв
NodeLink-detour/hops (fail-closed, топологический), FolderReplace-группы,
глобальная уникализация тегов конфига (если папки дают пересечения),
Направления/фильтры.

Что умирает: `.raw`-кэш и `LookupCachedBody`/Rebuild-без-сети
(core/rebuild_raw_cache.go целиком), `filterDisabledNodes`+TTL+GC,
`migrateLegacyDisabledKeys` (переезжает в одноразовую миграцию состояния),
PreviewNodes, ленивый кэш превью.

## 5. Конфликты и пробелы модели (главное)

### К1. skip[] в модели отсутствует
DRAFT не упоминает skip вообще. В коде: `state.Source.Skip` — subscription-only
(connections.go:105), применяется внутри парсеров до всего остального.
Гипотеза «поле Subscription, применяется при fetch до merge» —
**согласуется с кодом и моделью**: skip-узел просто не попадает в разобранный
список → при merge его тег «исчез у провайдера» → узел удаляется из nodes[].
НО две цены, которых DRAFT не видит:
- сегодня правка skip[] применяется мгновенно (Rebuild из raw-кэша перечитывает
  тело без сети); с материализацией и смертью raw-кэша правка skip подействует
  только на следующем fetch → UI обязан дёргать fetch после правки фильтров;
- узел, выпиливаемый skip'ом, при merge потеряет enabled/detour навсегда
  (удаление), а снятие фильтра вернёт его «новым» (enabled=true) — сегодня
  отметка выключения переживала игру с фильтрами (TTL 3 цикла). DRAFT
  декларирует эту потерю для «временного пропадания у провайдера», но
  skip-фильтр — то же самое руками пользователя.

### К2. Ключ merge vs тег-политика — центральная неопределённость
DRAFT: «слить в nodes[] ПО ТЕГУ», и одновременно `Node.tag` — «он же тег в
конфиге», а у Folder/Subscription есть `TagPolicy {prefix, postfix}`. Не
сказано, КОГДА применяется политика и какой тег — ключ merge:
- если nodes[] хранит теги С политикой и merge идёт по ним — правка префикса
  осиротит все теги следующего fetch → массовое удаление+добавление с потерей
  enabled/detour. Это ровно ловушка, которую SPEC 112 закрывал правилом
  «идентичность = сырой тег ДО префикса» (source_loader.go:54-66, :945-950);
- если nodes[] хранит сырые теги, а политика применяется на эмиссии —
  merge стабилен, но `Node.tag ≠ тег в конфиге`, что противоречит
  «узлу не нужен label — только tag, он же тег в конфиге» (DRAFT:121).
Модели нужен явный выбор; код голосует за «сырой тег — ключ, политика — на
эмиссии».

### К3. Дубли тегов в одном теле подписки
Провайдерская выдача `X, X-2, X` — норма (комментарии source_loader.go:80-99).
Сегодня пер-сорсный `idCounts` сохраняет оба узла. Merge-по-тегу без
уникализации при fetch молча схлопнет второй `X` в первый (перезапись body).
DRAFT молчит; уникализацию сырых тегов надо выполнять при fetch ДО merge —
но тогда результат зависит от порядка записей в теле, и перестановка у
провайдера меняет, кто из тёзок «X», а кто «X-2» (сегодня то же самое, но
отметка живёт один разбор; с материализацией переезд отметки станет видимым).

### К4. Dedup по подписи не покрыт merge-по-тегу
Регресс v1.5.2 (32 одинаковых ss://, различие только `#fragment`) merge-по-тегу
НЕ закрывает: fragment = тег, теги разные → 32 узла. `dedupSignature`
(server_conn_key.go:34) обязан пережить рефакторинг как fetch-стадия ДО merge,
иначе регресс вернётся. В DRAFT дедуп не упомянут.

### К5. Detour запечён в body
`ApplySourceDetour` пишет `Outbound["detour"]` прямо в карту outbound'а на
парсе (source_loader.go:1155-1177), цепочечные хопы импорта — тоже
(:1039-1051). По DRAFT body чист, detour — отдельный `NodeLink`, применяемый
на эмиссии, а fetch-merge «освежает body, сохраняет detour». Значит
fetch-парсер обязан перестать штамповать detour в body; исключения
(wireguard+listen_port, Xray Jump «свой detour побеждает») переезжают в
эмиттер/валидатор.

### К6. max_nodes: настройка сегодня ничего не ограничивает
DRAFT: «потолок max_nodes остаётся защитой» + резолв «своя настройка →
заголовок провайдера → дефолт». Факты кода: (а) парс режет по константе 3000,
настройка красит только бейдж Truncated (§2) — «остаётся защитой» на деле
означает «впервые станет защитой»; (б) заголовка провайдера для max_nodes не
существует — провайдерский заголовок есть только у интервала обновления
(`profile-update-interval`, meta.go:134). Резолв из DRAFT для max_nodes
вырождается в «настройка → дефолт».

### К7. Truncated vs SPEC 113-A «обрезано не трогает nodes[]»
DRAFT: «недостоверный ответ (ошибка, пусто, ОБРЕЗАНО) не трогает nodes[]
вообще». Сегодня trustedParse при `skippedDueToLimit>0` лишь запрещает
чистки — узлы (первые 3000) в конфиг идут. Если «обрезано» в новой модели
включает кап max_nodes, подписка, стабильно превышающая потолок (CIDR-500 при
max_nodes=100), НИКОГДА не обновит nodes[] — вечная заморозка вместо
сегодняшней «первые N». Нужно решение: кап = достоверный-но-частичный merge
(и тогда как отличить «узел исчез» от «узел за капом» — удалять нельзя),
либо кап = недостоверно (заморозка). Оба варианта хуже сегодняшнего поведения
в каком-то углу; DRAFT этого не проговаривает.

### К8. Миграция disabled_nodes → enabled
Сегодня: карта identity-тег→ts (плюс возможные непереписанные 64-hex ключи в
старых state — миграция персистится только через превью Wizard, §3).
Прямолинейная миграция «на первом fetch применить карту и выбросить» ломается,
если первый fetch после апгрейда упал (nodes[] пусты, применять не к чему;
карту нельзя выбрасывать). Рабочий путь: одноразовая миграция состояния
материализует nodes[] ИЗ существующего `bin/subscriptions/<id>.raw` (он ещё
на диске в момент апгрейда), применяет к ним карту (включая legacy-хеши через
имеющийся `migrateLegacyDisabledKeys`-механизм), ставит `enabled=false`
совпавшим, и только после этого сносит raw и карту. Несовпавшие ключи —
потеря по DRAFT (задекларирована). Отдельно: формат бэкапа
(`sub.Disabled`, export.go:222) — контракту нужна секция enabled-отметок или
legacy-чтение, как сделали с chains.

### К9. Переменные тег-политики
`TagPolicy {prefix, postfix}` DRAFT — плоские строки; в коде prefix/postfix/
mask поддерживают 8 переменных (:1249-1275). DRAFT явно хоронит только mask
(«теряет с warning»); судьба `{$num}`/`{$server}` в prefix/postfix не
проговорена — молчаливая потеря или сознательный отказ?

### К10. Standalone exclude_from_global
DRAFT: «excludeFromGlobal УБРАН совсем... служебный транспорт прячется
папкой». В коде флаг жив и отдельно от Fold (types.go:141-149: состояния, где
он стоял БЕЗ локальных групп, свёрткой не выражаются). Миграция таких
источников в модель без флага не описана: заворачивать в персональную папку
без replace? терять с warning? DRAFT покрывает только fold→FolderReplace.

### К11. Группы: members как теги vs NodeLink
Импортированные группы сегодня хранят состав финальными конфиг-тегами после
rebind (:1073-1144), с перепривязкой схлопнутых дублей. В модели
`Auto.members = List<NodeLink>` (folderId+tag) — при fetch члены должны
резолвиться в NodeLink'и на узлы ТОЙ ЖЕ подписки-папки по сырым тегам;
логика collapsedInto (член-дубль → выживший) обязана переехать, иначе группа
из дублей снова потеряет состав (находка аудита M1). Плюс у Auto появляется
enabled (наследник Node) — сегодня у групп отметок нет вовсе (:66).

## 6. Ответы на поставленные вопросы (кратко)

- **Куда skip[]**: поле Subscription (в state он и так subscription-only),
  применяется при fetch до merge. Гипотеза подтверждается, с двумя ценами
  (К1): правка фильтра требует fetch; skip-узел теряет enabled/detour.
- **Что при fetch / что при сборке**: см. §4. Ключевое следствие — dedup,
  skip, кап и уникализация сырых тегов становятся fetch-стадиями; политика
  тегов и detour — эмиссионными (если принять К2/К5 в пользу сырого тега).
- **Миграция disabled_nodes → enabled**: через материализацию из raw-кэша в
  момент апгрейда, до его сноса (К8); legacy-64hex-ключи докручиваются тем же
  механизмом; бэкап-контракт — отдельный пункт.
