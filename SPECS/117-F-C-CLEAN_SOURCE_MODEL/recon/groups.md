# Recon: провайдерские группы (SchemeGroup) против модели DRAFT (SPEC 117)

Аудит существующего кода develop против SPECS/117-F-N-CLEAN_SOURCE_MODEL/DRAFT.md.
Тема: selector/urltest из подписок → узлы SchemeGroup → будущий `Auto {members: NodeLink[], strategy}`.

## 1. Где группы рождаются (два парсера)

### 1.1 sing-box JSON import (SPEC 094 A5)

- `core/config/subscription/singbox_import.go:139-220` (`parseSingboxConfig`): группы
  (`selector`/`urltest`, предикат `IsSingboxGroupType`,
  `core/config/subscription/singbox_sanitize.go:27-42`) откладываются и разбираются
  ПОСЛЕ обычных узлов; кладутся в тот же `result.Nodes`, в хвост.
- `core/config/subscription/singbox_groups.go:48-135` (`singboxGroupToNode`):
  - группа без тега → skip (строка 54);
  - состав резолвится по `nodeByTag` — карте «сырой тег → фактически импортированный
    узел». **Критично: в `nodeByTag` попадают только обычные узлы** (singbox_import.go:206-208),
    поэтому член-ссылка на ДРУГУЮ группу (вложенная группа) молча выпадает
    (singbox_groups.go:75-80, счётчик `lost`). Вложенность провайдера теряется уже на парсе,
    хотя финальный граф-санитайзер сборки вложенные группы поддерживает
    (`core/build/outbound_graph_sanitize_test.go:66-72`);
  - дедуп членов по итоговому тегу (строки 81-86);
  - пустая группа не эмитится вовсе (строки 92-96) — «пустой urltest роняет старт ядра»;
  - опции переносятся по **закрытому allowlist** `singboxGroupOptionKeys`
    (singbox_groups.go:32-38): `url`, `interval`, `tolerance`, `idle_timeout`,
    `interrupt_exist_connections`. Всё прочее отбрасывается (ядро отвергает unknown field);
  - `default` (только selector) валидируется на вхождение в состав (строки 119-125),
    иначе выбрасывается.

### 1.2 Xray balancer (SPEC 094 фаза C §322)

- `core/config/subscription/xray_balancer.go:39-86` (`xrayBalancerFromElement`):
  `routing.balancers[0]` элемента → узел-группа `urltest` с членами = все узлы элемента
  (по фактическим тегам, не по selector-префиксу Xray — тот матчит префиксом и растащил бы
  чужое, комментарий строк 34-38).
  - `url` берётся из `burstObservatory.pingConfig.destination`, дефолт
    `https://www.gstatic.com/generate_204` (строки 23-24, 115-122);
  - `interval` из `pingConfig.interval`, голое число = секунды, дефолт `3m` (строки 124-141);
  - тип всегда `urltest`; `selector`-групп Xray-путь не порождает.
- Владение серверами и резолв состава: `core/config/subscription/xray_json_array.go`
  - `rememberGroupMemberServers` (165-200): члены запоминаются серверными ключами
    ДО фильтра владения; узел без подписи — по запасному тег-ключу;
  - `resolveGroupMembers` (202-255): состав переписывается на итоговые теги выживших;
    группа без выживших членов удаляется (247-249);
  - `filterByServerOwner` (311-333): группы проходят фильтр владения всегда;
  - `computeXrayServerOwners` (264-304): группы серверных ключей не занимают;
  - `simplifySoloElementTags` (119-163): переименование solo-узлов обновляет
    `finalTagByServer`, чтобы группа сослалась на новое имя.
- Других парсеров групп нет: Clash proxy-groups не разбирается (упоминания clash — только
  комментарии), URI-списки и wg-quick групп не дают.

## 2. Какие поля хранятся

Группа — обычный `configtypes.ParsedNode` (`core/config/configtypes/types.go:559+`):

- `Scheme = SchemeGroup` (`"group"`, types.go:528-538);
- `Server`/`Port` пустые — «группа не соединение» (singbox_groups.go:131, balancer:81);
- `Tag`, `Label` (= тег), `SourceIndex`;
- `SourceTag` (types.go:576-582) — сырой тег из импортированного конфига, нужен только
  для перепривязки состава;
- `IdentityTag` НЕ штампуется: `StampNodeIdentity` возвращает "" для групп
  (`core/config/subscription/source_loader.go:67-70`); `NodeIdentity` тоже
  (`core/config/node_hash.go:63-69`). Следствия: у группы нет отметки выключения
  (нельзя disable), нет ключа дедупа (`server_conn_key.go:33-41` — подпись пустая,
  группы не схлопываются никогда), группа не кандидат node-ref/detour-ссылок
  (`core/config/node_ref.go:100-142`);
- вся полезная нагрузка — в `Outbound` map: `tag`, `type` (selector|urltest),
  `outbounds` (= `GroupMembersKey`, types.go:540-542) как `[]interface{}` строк,
  плюс allowlist-опции и `default`.

Персистентность сегодня: группы НЕ материализуются в state — каждая сборка перечитывает
.raw-кэш подписки и заново парсит. В state попадают только как строки превью
(`core/config_service_subscriptions.go:419-432`, `formatGroupPreview` 434+ —
`group://<type>?members=N…#tag`) и как `ParsedNode` в кэше PreviewNodes UI
(`ui/configurator/tabs/source_chain_hops.go:151-159`). Round-trip Outbound через JSON
не искажает эмиссию (`core/config/group_node_contract_test.go:213-247`);
`groupMemberTags` терпит обе формы состава — `[]interface{}` и `[]string`
(`core/config/outbound_generator.go:1470-1490`).

## 3. Теги и перепривязка состава (ключ к вопросу о тег-политике)

- Группа проходит ту же тег-машину, что и узлы: `applyTagsToSingboxNode`
  (source_loader.go:998-1027) применяет prefix/postfix/mask, нормализацию,
  ГЛОБАЛЬНУЮ уникализацию (`MakeTagUnique`, общий `tagCounts` конфига) и переписывает
  `Outbound["tag"]`. `SourceTag` сохраняет исходный тег (1014).
- `rebindImportedGroupNodes` (source_loader.go:1055-1144): после простановки тегов состав
  переписывается «SourceTag → итоговый тег». Член без узла (лимит MaxNodesPerSubscription,
  skip-фильтр, unsupported) выпадает; член, схлопнутый дедупом, перепривязывается на
  выжившую копию через `collapsedInto` (`server_conn_key.go:51-101`, SPEC 113-A §4);
  `default` тоже перепривязывается или удаляется (1134-1140); группа без членов
  удаляется (1128-1131).
- Xray-путь делает то же по серверным ключам (`resolveGroupMembers`).
- Превью-путь: `DedupParsedNodes` + `rebindCollapsedGroupMembers`
  (server_conn_key.go:129-192) — по ТЕКУЩИМ тегам, чтобы превью ≡ боевой разбор.

Итого: сегодня резолв членов при тег-политике — ЧИСТО parse-time механика, повторяемая
на каждой сборке; единственный носитель связи — пара (SourceTag, итоговый тег) в памяти
одного прогона. Смена tag_prefix безболезненна, потому что весь мир пересобирается заново.

## 4. Эмиссия (generateGroupNodeJSON)

- `core/config/outbound_generator.go:233-241`: `GenerateNodeJSONBare` сворачивает на
  отдельный эмиттер для `SchemeGroup` (обходя per-scheme switch — ловушка
  emitter-parser-pairing группе не грозит).
- `generateGroupNodeJSON` (1361-1422): требует `type` и непустой состав (ошибка — последний
  рубеж); порядок: `tag`, `type`, `outbounds`, затем ВСЕ остальные ключи Outbound
  в отсортированном порядке (детерминизм для golden/хеша). **Эмиттер открытый**: любой
  ключ, оказавшийся в Outbound, уедет в конфиг (парсер держит закрытый allowlist, а
  эмиттер — нет; см. конфликт К4 про detour).

## 5. Валидация членов и позиция групп в графе

Три рубежа:

1. parse-time — резолв/перепривязка (см. §3), пустая группа не рождается;
2. НО: `filterDisabledNodes` (source_loader.go:170-216) выкидывает выключенные узлы
   ПОСЛЕ rebind (вызов на 923, rebind на 688) → группа может уехать в сборку с членом,
   которого юзер выключил;
3. финальный граф-санитайзер `core/build/outbound_graph_sanitize.go` (правила 1-5,
   строки 19-42): участники-призраки исключаются, пустая группа удаляется, `default`
   вне состава заменяется первым участником, группа на позиции цепочки ≥1 не должна
   содержать цепочек, кольца по рёбрам member/detour/chain рвутся. Именно он спасает
   случай «disabled-член».

Политика по рёбрам РАЗНАЯ: member = мягкая деградация (prune), detour = fail-closed
выброс носителя (`sanitizeNodeDetours`, outbound_generator.go:1857+). Это осознанно
(память detour-failclosed-uniform — про detour, не про members).

Группы в пулах:

- пул Направления-селектора группы ВКЛЮЧАЕТ (видимы фильтру каналов —
  `group_node_contract_test.go:196-201`);
- auto-двойник Направления группы ВЫБРАСЫВАЕТ (`dropGroupNodes`,
  `core/config/direction_twins.go:273-289`; вызовы `core/config/outbound_validity.go:193,439`;
  SPEC 104: urltest внутри urltest мерил бы чужой выбор);
- detour/цепочка НА группу: import-путь рвёт цепочку на группе
  (`core/config/subscription/detour_chain.go:183-192`, B5 — хоп инлайнится копией, а
  группу не заинлайнить), node-ref-ссылки группы не резолвят (node_ref.go:71-72, 101),
  но строковый DetourTag на группу легален и UI его предлагает
  (`ui/configurator/tabs/source_chain_hops.go:155-158`, hopKindGroup);
- идентичности/дедупа/выключения у групп нет (см. §2).

## 6. Конвертация SchemeGroup → Auto: что теряется, чего не хватает

### Поля: достаточно ли `{members, strategy}`?

Фактический набор полей группы, который обязан пережить конвертацию:

- **тип группы** — selector vs urltest. В DRAFT у Auto только `strategy`; нужен явный
  дискриминатор (selector — это НЕ «urltest без url»: у selector есть `default` и ручной
  выбор, живущий в cache.db);
- `url`, `interval`, `tolerance`, `idle_timeout`, `interrupt_exist_connections` —
  ложатся в AutoStrategy (DRAFT это и обещает: «набор полей возьмём из фактического кода»);
- **`default`** (selector) — в перечислении strategy не назван; он member-зависимый
  (валидируется на вхождение в состав в двух местах: singbox_groups.go:119-125 и
  граф-санитайзер правило 2). Если `default` не переедет — теряется молча;
- `interrupt_exist_connections` есть у ОБОИХ типов — ок в strategy.

Открытость эмиттера (§4) сегодня теоретически пропускает и не-allowlist поля, попавшие в
Outbound иным путём; при переходе на типизированный AutoStrategy это станет жёстким
отсечением — фактических потерь нет, пока allowlist парсера закрыт (он закрыт).

### members: []string → NodeLink[]

- Сегодня члены — итоговые конфиг-теги ВНУТРИ того же источника (rebind не умеет
  cross-source). `NodeLink{folderId, tag}` это выражает (folderId = папка-подписка)
  и заодно легализует cross-folder членов — расширение, не потеря.
- **Тег-политика — главный пробел.** DRAFT: `Node.tag` «он же тег в конфиге» (пост-политика),
  fetch merge «по тегу». Сегодня непрерывность держит `IdentityTag` — СЫРОЙ тег,
  уникализированный в пределах источника ДО политики (source_loader.go:54-84,
  node_hash.go:51-77): смена tag_prefix не рвёт ни отметки, ни ссылки. Если merge-ключ и
  NodeLink.tag = пост-политика, то правка TagPolicy папки без refetch обязана транзакционно
  переписать: теги всех узлов, `Auto.members[]`, `Folder.replace`-ссылки, detour/hops
  чужих узлов на эти теги — иначе воспроизводится ловушка #91 (tag-vs-label) и баг,
  ради которого SPEC 112 сносил контент-хеш. Модель должна явно сказать, какой тег
  хранится в NodeLink и кто переписывает ссылки при смене политики/уникализации.
- Глобальная уникализация (`MakeTagUnique`, общий tagCounts на конфиг) сегодня — per-build
  и зависит от порядка источников. При материализации в state она должна произойти на
  fetch и стать стабильной, а members — следовать за ней (сегодня следуют автоматически,
  потому что rebind в том же прогоне).

### Резолв членов и жизненный цикл

- **Исчезнувший член.** Сегодня: каждый парс группу пересобирает, мёртвый член выпадает,
  пустая группа умирает. В новой модели fetch удаляет исчезнувший узел из `nodes[]`, но
  merge группы по тегу обновит и её members — ок ДЛЯ подписки. А вот выключение члена
  (`node.enabled=false`) больше не удаляет узел из nodes[] — эмиссия Auto обязана
  пропускать выключенных членов и НЕ эмитить опустевший Auto (сегодня это ловит только
  граф-санитайзер, и то потому что узел физически выпадает). DRAFT политику
  «Auto с нерезолвящимся/выключенным членом» не задаёт: prune (текущее поведение member-рёбер)
  или fail-closed (политика detour)? Надо зафиксировать prune + смерть пустого Auto,
  иначе «единая строгость detour» случайно накроет и members.
- **Вложенные группы.** Модель типом не запрещает Auto-член у Auto; парсер их теряет
  (§1.1), санитайзер сборки — поддерживает. Решить: легализовать (убрать фильтр
  singbox_groups.go:75-80) или явно запретить в модели.
- **Origin у Auto.** DRAFT: «Auto тоже помнит subUrl», а `Origin.raw` — «исходник байт в
  байт». Для sing-box-группы фрагмент есть (её JSON-entry), но Regen по нему невозможен:
  состав резолвился по ВСЕМУ телу (nodeByTag, дедуп, rebind). Для Xray-балансера
  фрагмента нет вовсе — группа синтезирована из `routing.balancers[0]` +
  `burstObservatory` + списка узлов элемента. Origin.raw/Regen для Auto либо null,
  либо диагностический без Regen — модель должна оговорить.
- **enabled у Auto** — новая способность (сегодня группы нельзя выключить: identity "" →
  отметке не к чему прицепиться). Выигрыш, но каскад «выключил Auto → его члены остаются
  узлами папки» должен быть описан (по семантике replace в DRAFT — остаются, ок).
- **Ручной выбор selector'а** — в модели верно оставлен cache.db; но merge по тегу при
  fetch сохраняет тег → выбор переживает обновление. Совпадает с текущим поведением.

## 7. Прямые конфликты кода с моделью

- **К1 (detour на группе — латентный баг и конфликт модели).** `Auto extends Node` наследует
  `detour: NodeLink?`, но sing-box selector/urltest не принимают dial-поля — `detour` на
  группе валит конфиг целиком (unknown field; сам код это постулирует в
  singbox_groups.go:30-31). При этом УЖЕ СЕЙЧАС `ApplySourceDetour`
  (source_loader.go:1155-1177) и `resolveNodeDetours` (outbound_generator.go:1705-1722)
  штампуют `Outbound["detour"]` на ВСЕ узлы источника БЕЗ исключения SchemeGroup, а
  открытый эмиттер (outbound_generator.go:1412-1419) это поле честно эмитит: подписка с
  импортированной группой + source-level detour у источника = битый config.json. Тестов
  на эту комбинацию нет (detour_test.go групп не содержит). Модель обязана либо
  запретить detour у Auto (и у Folder.detour для Auto-детей), либо определить семантику
  «detour группы = detour каждого члена».
- **К2 (merge-ключ vs тег-политика).** DRAFT сливает по «тегу», но не говорит, по какому
  (сырой провайдерский vs финальный с политикой). Код различает их тремя полями
  (Tag / SourceTag / IdentityTag) именно потому, что финальный тег нестабилен
  (prefix, mask, `-2` от глобальной уникализации). Auto.members на финальных тегах без
  правил перезаписи при смене политики — регресс SPEC 112.
- **К3 (default выпал из strategy).** Перечисление полей strategy в DRAFT (`url, interval,
  tolerance, …`) не покрывает `default` selector'а и сам дискриминатор selector/urltest;
  оба поля живые (парс: singbox_groups.go:105-125; санитайзер: правило 2).
- **К4 (политика битого члена не задана).** Сегодня member-ребро деградирует (prune,
  три рубежа §5), detour — fail-closed. DRAFT формулирует только «резолв NodeLink…
  fail-closed» в контексте detour/replace; без явной оговорки для Auto.members
  реализация может унести fail-closed на группы и начать ронять Auto из-за одного
  выключенного члена.
- **К5 (вложенные группы).** Модель допускает (NodeLink на Auto той же папки), парсер
  теряет молча (singbox_groups.go:75-80). Нужно решение: легализовать или запретить.
- **К6 (Origin/Regen для синтезированных групп).** Xray-балансерная группа не имеет
  raw-фрагмента; sing-box-группа непересобираема из фрагмента (состав контекстный).
  Правило В1 («источник истины — body, Regen из raw») к Auto неприменимо без оговорки.
- **К7 (выключенные члены при материализации).** node.enabled=false перестаёт удалять
  узел из nodes[] → эмиссия Auto обязана фильтровать по enabled сама; сегодня этот случай
  прикрыт лишь случайно (узел физически исчезает из списка, призрака чистит
  граф-санитайзер core/build/outbound_graph_sanitize.go, правило 2). Порядок
  rebind(688) → filterDisabledNodes(923) в source_loader.go уже сейчас оставляет
  призрака до последнего рубежа.

## 8. Что конвертация ПОКРЫВАЕТ без потерь

- Все опции allowlist → strategy (при добавлении default+kind, К3).
- Члены-строки → NodeLink[] (расширение до cross-folder).
- Пул Направлений: dropGroupNodes для auto-двойников переносится 1:1 (Auto не входит в
  urltest Направления — SPEC 104 сохраняется).
- Ручной выбор selector — cache.db, как записано в DRAFT.
- Превью/UI уже читают состав из Outbound (preview_node_info.go:131-170,
  preview_node_subtitle.go:39-60) — при переходе на nodes[] напрямую (смерть PreviewNodes)
  им нужен лишь новый источник данных, не новая логика.
- Дедуп/rebind-машинерия (SourceTag, collapsedInto) целиком остаётся в fetch-парсе и
  наружу не течёт — материализация её инкапсулирует, как DRAFT и хочет («парсинг один
  раз, на fetch»).
