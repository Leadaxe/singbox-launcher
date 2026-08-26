# SPEC 112 — идентичность узла = тег (упразднение identity-хэша)

## Решение

Единственная идентичность узла — его **тег, уникальный в рамках источника**
(подписки или папки). За уникальность отвечает парсер (уникализация дублей).
Содержимое узла (server, port, ключи, SNI, transport, mtu…) в идентичность
НЕ входит. sha256 от эмитированного JSON (SPEC 094 D / SPEC 101,
`core/config/node_hash.go`) упраздняется.

Обоснование (из обсуждения 2026-08-26):
- Тег — имя, которым провайдер управляет узлом. Провайдер вправе поменять
  сервер под тем же именем (ротация IP, группы, обходы белых списков) —
  это ТОТ ЖЕ узел. Привязка идентичности к содержимому ломает логику провайдера.
- Контент-хэш зависит от эмиттера и формы хранения: `uri` и `config_json`
  одного узла давали разные хэши → detour-ссылка Proton NL на
  «🔥🎭 WARP (MASQUE)» молча протухла, узел fail-closed выпал из конфига
  (реальный баг, стейт машины IRA). Класс багов устраняется только сносом
  контент-хэша.

## Ключ

**Идентичность (отметки выключения, принадлежность):**
`сырой провайдерский тег`, уникализированный внутри источника, — снятый ДО
применения наших tag_prefix / tag_mask. Хранится per-source (id источника —
неявно, по месту хранения).
- Уникализация дублей: тем же алгоритмом, что для конфиговых тегов
  (первый `X`, следующий `X-2`), в порядке парсинга.
- Смена tag_prefix / tag_mask источника НЕ меняет идентичность —
  отметки выключения живут.
- Узлы-группы (SchemeGroup) идентичности не имеют (как сейчас исключены
  в resolveNodeHashDetours).

**Ссылки (detour на конкретный узел, SPEC 101):**
финальный глобальный тег узла — та же конвенция, что chains.hops,
DetourTag (SPEC 077) и Направления. Поле `DetourNodeTag`
(json `detour_node_tag`). `DetourNodeLabel` остаётся как подпись.
Fail-closed семантика сохраняется: хоп не найден → зависимый источник
выбрасывается с warning (не direct).

**Контент-дедуп упраздняется** вместе с хэшем: единственный механизм при
дублях — уникализация тегов. Полные копии узла больше не схлопываются
(принято: счётчик узлов на мусорных подписках может вырасти).

## Изменения по файлам

1. `core/config/node_hash.go` — `NodeIdentityHash` → новая функция
   идентичности по тегу; старый sha256 сохранить как
   `legacyNodeIdentityHash` (private) ТОЛЬКО для миграции. Комментарии
   пакета переписать под новую модель.
2. Hook `subscription.NodeIdentityHashFunc`
   (`core/config/subscription/source_loader.go:29-40`,
   `core/controller.go:262`, `core/config/node_hash.go:37`) — сигнатура
   `func(node) string` не меняется, возвращает новую идентичность.
3. `filterDisabledNodes` (`source_loader.go:79-100`) — ключи = новая
   идентичность. Миграция: ключ из 64 hex → прогнать
   `legacyNodeIdentityHash` по распарсенным узлам источника; совпало →
   переписать на тег-ключ (persist через уже существующий возврат
   `refreshedDisabled`); не совпало → выкинуть (отметка и так мёртвая).
4. Контент-дедуп по хэшу (`source_loader.go:155-167`, `:526`,
   `core/config/subscription/xray_json_array.go` — все вызовы
   NodeIdentityHashFunc: 132, 146-155, 240, 274, 315, 483) — снести /
   перевести на тег-ключ. Отдельно xray-ownership хопов
   (`xray_ownership_test.go`) — принадлежность считать по тегам.
5. `resolveNodeHashDetours` (`core/config/outbound_generator.go:1346-1440`)
   → `resolveNodeTagDetours`: lookup узла по финальному тегу; fail-closed
   и warning сохранить дословно по смыслу.
6. Типы: `core/config/configtypes/types.go:138-166` и
   `core/state/connections.go:144-161` — `DetourNodeHash` →
   `DetourNodeTag` (json `detour_node_tag`); `DisabledNodes` остаётся
   `map[string]int64`, меняется семантика ключа (задокументировать).
   Прокинуть через `core/state/adapter_source.go:35,53`,
   `core/state/sync_to_connections.go:110,144`,
   `core/state/sync_to_legacy.go:30,47`.
7. Миграция стейта при загрузке: если у источника есть legacy
   `detour_node_hash` — (а) при первом парсе прогнать
   legacyNodeIdentityHash по всем узлам, найден → записать его финальный
   тег в DetourNodeTag; (б) не найден → fallback: взять `DetourNodeLabel`
   как тег (это лечит битый стейт IRA — label там и есть тег хопа).
   Поле `detour_node_hash` после миграции не писать.
8. Бэкап: `core/backup/export.go:296-297` — писать `detour_node_tag`;
   `core/backup/import.go:303-322` — читать оба варианта (legacy hash
   импортировать в legacy-поле, мигрирует при первом парсе). Отметки
   выключения в бэкапе — новые ключи, legacy-хэши переживают импорт
   до миграции.
9. UI: `ui/configurator/business/detour.go:132-199`,
   `ui/configurator/tabs/source_edit_window.go:208-240,488-503,886,1243`,
   `ui/configurator/business/source_node_counts.go:64` — пикер «» node»
   пишет DetourNodeTag; счётчики/отметки по новой идентичности.
10. Контракт: `contract/docs/IDENTITY.md` — переписать нормативно
    (идентичность = тег + уникализация + правило миграции);
    `contract/docs/BACKUP.md:121`; `contract/corpus/backup/README.md:55`;
    диаграмма `contract/diagrams/parse_pipeline.mmd:20-24`; bump
    `contract/VERSION` (minor). Пометить в тексте: требует зеркальной
    правки LxBox (процесс как chains/0.7.1).

## Ловушки

- **Порядок стемпинга тегов.** Сейчас хэш игнорирует tag, момент
  вычисления не имел значения. Теперь идентичность = тег → снимать сырой
  тег ДО prefix/mask/uniquify источника, а уникализацию идентичности
  вести отдельно и тем же порядком, что уникализацию конфиговых тегов.
  Проверить `applyTagsToSingboxNode` (`source_loader.go:696+`) и поле
  `SourceTag` (`types.go:513-519`) — возможно, сырой тег уже сохраняется
  для импортов; переиспользовать, не плодить второе поле.
- **Preview ≡ parse.** Идентичность узла из Meta.PreviewNodes
  (`source_node_counts.go`) и из боевого парса обязана совпадать — одна
  функция, один порядок уникализации, иначе галки выключения в превью
  разъедутся с конфигом (класс бага lazy-cache-vs-lost-state).
- **win7-сборка = go1.20**: никаких min/max/slices/PathValue.
- **Тесты**: переписать `node_hash_test.go`, `detour_node_hash_test.go`,
  `identity_contract_test.go`, `disabled_nodes_test.go`,
  `e2e_disabled_flow_test.go`, `disabled_nodes_roundtrip_test.go`,
  `xray_ownership_test.go`, `dedup_test.go`, `corpus_test.go`,
  `ui/.../detour_test.go`, `disabled_node_toggle_test.go`,
  `source_node_counts_test.go` под новую модель; миграцию (hash→тег,
  label-fallback, legacy disabled-ключи) покрыть новыми тестами.
  Тестов на форматирование UI-строк не писать.
- Граф-санитайзер сборки (`build-graph-sanitizer`): detour-ребро по тегу
  уже проверяется финальным проходом — убедиться, что новое разрешение
  не обходит его.

## Критерии приёмки

1. Стейт IRA (источник MASQUE с uri+config_json, протухший
   detour_node_hash `62bff800…`): после загрузки и сборки Proton NL
   присутствует в конфиге с `"detour": "🔥🎭 WARP (MASQUE)"`
   (миграция по label-fallback).
2. Смена формы хранения узла (uri ↔ config_json) не меняет идентичность.
3. Правка mtu/SNI/ключей узла не рвёт ни отметки выключения, ни ссылки.
4. Legacy state.json с disabled_nodes-хэшами: отметки переживают первый
   парс и переписываются на тег-ключи.
5. Полный `go test ./...` зелёный; `go build ./...` зелёный.
