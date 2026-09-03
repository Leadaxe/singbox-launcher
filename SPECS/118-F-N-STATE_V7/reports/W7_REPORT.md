# SPEC 118 · W7 — Бэкап-конвертеры и remote-гейт

Волна закрывает границу состояния с внешним миром: контракт бэкапа 0.11,
частичные правки состояния удалённых машин и отладочные снимки. Контракт
`contract/**` не тронут, корпус `contract/corpus/backup/` зелёный без правки
фикстур.

## 1. Что уже было сделано до W7

Конвертеры границы (`core/backup/convert_v7.go`) написаны в W5 как
компенсация сноса легаси — оба первых пункта TASKS оказались реализованы:

- экспорт v7 → 0.11: `exportFold` (replace → fold), `exportDisabledMap`
  (enabled=false + PendingDisabled → карта по сырым тегам),
  `exportNodeLinkRef` (NodeLink → тройня), `exportHops` (NodeLink → строки),
  `exportChainSpec`; TagPolicy → `tag{prefix,postfix}`, NodeTag = `Node.tag`;
  `nodes[]` не экспортируются;
- импорт 0.11 → v7: `importFold`, `importNodeLinkRef`, `importHops` +
  второй проход `resolveImportedHops`, `importChainBody`, `importMaskTag`,
  `foldDerivedDirectionTags`;
- хвост W3 (перевод импорта на `PendingDisabled` вместо DisabledNodes-карты)
  тоже закрыт в W5 — `importSubscription` кладёт отметки в
  `src.PendingDisabled`, вердикт O2.

W7 достроил недостающее: тесты §4.F.2/§4.F.3, remote-гейт §4.G, актуализацию
формы Debug API — и нашёл при этом одну реальную дыру (см. §3).

## 2. Тесты бэкапа (§4.F)

`core/backup/convert_v7_test.go` — новый файл. Предмет отличается от
соседнего `purity_test.go`: тот проверяет БАЙТ-тождественность файла
(свойство формата), здесь — эквивалентность МОДЕЛИ. Байтовая тождественность
её не доказывает: пара «экспорт теряет X — импорт выдумывает X» даёт
одинаковые файлы и разъехавшуюся модель.

| Тест | Что закрывает |
|---|---|
| `TestRoundTripV7ModelEquivalent` | §4.F.2: все четыре конвертации на круге — enabled+PendingDisabled ⇄ карта, replace ⇄ fold (режим и тег), detour-NodeLink обоих видов (адресный и корневой), хопы (порядок и состав), TagPolicy; плюс явные утверждения о названных ценах: `nodes[]` не едут, настройки маршрута цепочки живут в `Node.Body` и переживают границу, позиции в тело не просачиваются |
| `TestRoundTripV7ResolvesHopIntoContainer` | §4.F.2: «резолв по живому индексу» не ВЫДУМЫВАЕТ адрес — узлов подписки в файле нет, и хоп обязан остаться `NodeLink{"", тег}` (fail-closed на сборке). Заодно фиксирует replace-семантику импорта (папка приёмника замещается) |
| `TestImportLegacy15xBackup` | §4.F.3: файл v1.5.x с fold + локальными Направлениями + disabled-картой + маской. fold(select_auto) → `FolderReplaceBoth` с тем же тегом и параметрами автогруппы; prefix жив, mask — названная потеря; disabled → PendingDisabled; производная свёртки НЕ названа потерей, произвольное `[P] streaming` — названо; правило на тег замены приезжает ВКЛЮЧЁННЫМ |
| `TestImportLegacyServerMaskArrivesAsNodeTag` | §4.F.3, вторая половина: имя одиночного узла (server `node_tag`, chain `tag`) → `Node.Tag` БЕЗ warning'а — потери там нет |
| `TestExportNamesUnrepresentableReplaceTag` | новая находка §3 |

Corpus-тесты (`TestBackupCorpus`, 14 кейсов) зелёные без правки фикстур —
проверено прогоном.

## 3. Находка: тег замены не переживает контракт (исправлено)

`TestRoundTripV7ModelEquivalent` упал на первом же прогоне:

```
replace.tag: "[A]select", было "1:select"
```

**Причина.** В v7 `replace.tag` — обычное поле, которое пользователь задаёт
руками (вкладка Replace, W5). В контракте 0.11 места для него НЕТ: свёртка
несла только режим, а имя группы было позиционным деривативом —
`legacyFoldPrefix(prefix, index) + "select"`, где префикс с пустым значением
даёт `<номер>:`. Импорт другого источника тега не имеет и обязан
воспроизвести ту же формулу (`backupReplaceTag`), иначе правила ТОГО ЖЕ
файла уедут в никуда.

Значит явный тег, с деривативом не совпавший, круг не переживает: на
приёмнике группа зовётся иначе, а правила, метившие в прежнее имя, приедут
выключенными. Это молчаливая подмена имени, на которое ссылается
маршрутизация, — ровно то, что формат запрещает (П6).

**Исправление.** Новый код предупреждения `backup_replace_tag_derived`
(`import.go`), проверка `replaceTagSurvivesExport` (`convert_v7.go`) и её
вызов в `Export` (`export.go`). Предупреждение ставится на ЭКСПОРТЕ — там,
где ещё видны оба имени; Detail несёт `было → станет`. Экспортные warning'и
уже показываются пользователю тем же списком, что импортные
(`settings_backup.go`, хвост ревью W1).

**Побочно.** Формула была продублирована трижды (`backupReplaceTag`,
`foldDerivedDirectionTags`, новая проверка) с расхождением в TrimSpace.
Сведена в одну функцию `legacyFoldPrefix` — она воспроизводит старый движок
байт-в-байт, включая TrimSpace префикса: `"[P] "` действительно давал
`[P]select` (сверено с `configtypes.EffectiveTagPrefix` на `e478a92~1`).
Тесты §4.F написаны под этот, фактический, формат.

Контракт не менялся: кода в `contract/registry/warnings.json` нет ни у
одного backup-кода (`backup_source_kind_unsupported`, `backup_tag_mask_dropped`
и прочие из W5 там тоже отсутствуют), а `TestRegistrySyncWarningCodesDeclared`
сверяет только `subscription/parse_warnings.go`.

## 4. Remote-гейт (§4.G)

### Почему нужна отдельная функция, а не `Load(...).Version`

`Load` — не наблюдатель, а преобразователь: мигрирует v2–v6 на диске, а всё,
что `>= 7`, разбирает как v7, игнорируя незнакомые ключи. После него ответ на
вопрос «какой схемой написан файл» всегда «седьмой» — гейтить нечем.

`core/state/schema_gate.go` (новый): `SchemaMajor`, `SchemaVersionOfFile` /
`SchemaVersionOfBytes` (разбирают только шапку — top-level `version` и
`meta.version`), `SchemaMismatchError` (текст называет ОБЕ версии),
`CheckSchemaCompatible`, `ErrSchemaFileMissing`.

### Правила гейта

- **Направление значимо.** Файл СТАРШЕ мажора пропускается — для того и
  написана миграция. Файл из БУДУЩЕГО отвергается: эта сборка выронила бы из
  него всё незнакомое и записала обратно уже без этого.
- **Отсутствие файла — не расхождение.** Возвращается `ErrSchemaFileMissing`,
  обработчик решает сам (PATCH → 404 от load; copy-from → более точный текст
  «configure it first»).
- **GET не гейтуется.** Закрыть диагностику ровно в момент расхождения —
  худшее из возможных поведений.
- **copy-from гейтуется по ИСХОДНИКУ** — переносится именно его файл.
- Отказ = **409** (не 400 и не 404: запрос корректен, цель существует, но
  состояния сторон несовместимы) + поля `schema_found` / `schema_supported`.

### Точки встраивания

| Файл | Что |
|---|---|
| `core/debugapi/state_endpoints.go` | `stateAccess` получил поле `path` (нужно ТОЛЬКО гейту); `localStateAccess` заполняет его `platform.GetWizardStatePath(facade.GetExecDir())` — тот же путь, что читает `LoadState`; новая `guardStateSchema`; вызовы в PATCH-ветках `stateRulesWith` и `stateDNSWith` |
| `core/debugapi/remote_state_endpoints.go` | `machineStateAccess` заполняет `path` — гейт накрывает remote-близнецов теми же вызовами |
| `core/debugapi/remote_endpoints.go` | `handleRemoteProfileCopyFrom` — проверка исходника до `os.Stat` приёмника и до `CopyProfileFrom` |

`core/services/lxd_remote_registry.go` НЕ тронут (чужая сессия) — гейт целиком
в слое endpoint'ов, как и предписано TASKS.

### Тесты (`core/debugapi/schema_gate_test.go`)

Пять тестов: отказ PATCH обеих секций удалённой машины + «файл не тронут»;
работа тех же PATCH'ей при совпадении мажора; GET не гейтуется; copy-from
отказывает по исходнику и не создаёт state приёмника; copy-from работает на
своей схеме; локальный PATCH гейтуется тем же кодом (сценарий — откат на
старую сборку после апгрейда) и `savedState` остаётся nil.

**Проверка остроты.** Каждый гейт временно отключался, тесты падали
осмысленно:
- без `guardStateSchema` — `PATCH state/rules: status 200`, и файл из будущего
  переписывался как v7;
- без проверки в copy-from — `status 200`, `remote profile copy: "src" → "dst"
  (176 bytes, retargeted to linux/amd64)`.

## 5. Debug API — форма v7 (Т10)

**Найдено при проверке.** `/state/full` маршалил Go-структуру `state.State`
напрямую. У неё нет json-тегов, поэтому наружу шло:

```json
{"Version":7,"ID":"","ParserConfig":{...},"Sources":null,"Defaults":{},
 "SelectableRuleStates":null,"CustomRules":[],"RulesLibraryMerged":false,
 "DNSOptions":null,"DNS":{},"Migration":null, ...}
```

То есть PascalCase-ключи, мёртвые поля загрузчика (`Defaults`,
`SelectableRuleStates`, `RulesLibraryMerged`, `DNSOptions: null`), read-only
Load-проекция `ParserConfig` — и всё это расходилось с тем, что лежит в
файле. Для отладки миграции на v7 это худшая из возможных подмен, а SPEC 050
описывает endpoint как «полный state.json».

**Исправление.** `(*State).MarshalV7()` (`core/state/save.go`) — экспортная
обёртка над уже существующим приватным `marshalDisk` (функция чистая, ничего
не мутирует). `stateFullWith` отдаёт её результат как `json.RawMessage`.
Сериализация у файла и у endpoint'а теперь ОДНА по построению; remote-близнец
получил то же изменение бесплатно (общий `stateFullWith`).

`TestStateFull` переписан под форму: ответ разбирается тем же `state.Parse`,
что и файл (расхождение всплывёт ошибкой разбора, а не молчанием),
проверяются `meta.version`, `sources[]` с материализованными `nodes[]`,
`rules`, `dns_options`, `directions`; отдельно — что мёртвые Go-поля наружу не
текут и что ключи v7 присутствуют. Старый тест проходил случайно: `Rules` и
`Directions` совпали по имени поля и ключа, а `dns` уже тогда терялся —
утверждение о нём молча читало нули.

`/debug/snapshot` правки не потребовал: `core/snapshot` кладёт файл как есть,
а Save пишет v7. Секреты не маскируются — by design (`features/state.md`).

## 6. Живые фичи

`SPECS/features/state.md` (нормативный документ) поправлен той же правкой:

- раздел «Бэкап и граница контракта с LxBox» — абзац о теге замены: почему
  дома в контракте нет, какая формула его воспроизводит и почему расхождение
  называет ЭКСПОРТ;
- раздел «Внешние поверхности состояния» — Debug API: сериализация одна с
  файлом, Go-структуру отдавать нельзя; удалённые машины: почему версия
  спрашивается у ФАЙЛА, значимость направления расхождения, гейт copy-from по
  исходнику, GET не гейтуется.

`SPECS/features/sources.md` правки не потребовал — дерево источников волна не
меняет.

## 7. Приёмка

| Проверка | Результат |
|---|---|
| `go build ./...` | зелёный |
| `go vet ./...` | зелёный |
| `go test -count=1 ./...` | зелёный (весь модуль, не только W7-пакеты) |
| `go test ./core/backup/... ./core/debugapi/...` | зелёный |
| Corpus `contract/corpus/backup/` (14 кейсов) | зелёный без правки фикстур |
| `contract/**` | не изменён |
| Греп go1.20 по диффу волны | чисто (единственное вхождение — комментарий `// slices.)`, существовавший до волны) |
| `ETALON_V6MIG=1` | РОВНО одно задекларированное расхождение Р2 (`[P]auto` → `[P]select-auto`), других нет — эмиссионный слой волна не трогает |
| Запретные файлы (`ui/traffic/**`, `ui/machine_*`, `internal/lxdclient/**`, `core/services/lxd_remote_registry.go`) | не тронуты |

## 8. Изменённые файлы

Новые:
- `core/state/schema_gate.go`
- `core/backup/convert_v7_test.go`
- `core/debugapi/schema_gate_test.go`

Изменённые:
- `core/state/save.go` — `MarshalV7`
- `core/backup/convert_v7.go` — `replaceTagSurvivesExport`, `legacyFoldPrefix`,
  `foldDerivedDirectionTags` на общей формуле
- `core/backup/export.go` — предупреждение о теге замены
- `core/backup/import.go` — код `WarnBackupReplaceTagDerived`,
  `backupReplaceTag` на общей формуле
- `core/debugapi/state_endpoints.go` — `stateAccess.path`, `guardStateSchema`,
  v7-форма `/state/full`
- `core/debugapi/remote_state_endpoints.go` — `path` у machine-доступа
- `core/debugapi/remote_endpoints.go` — гейт copy-from
- `core/debugapi/state_endpoints_test.go` — `TestStateFull` под форму v7
- `SPECS/features/state.md` — нормативные абзацы (§6)

## 9. Открытое

Ничего не отложено. Вопрос O2 (pending_disabled) закрыт вердиктом и
реализован; O3 (расхождение Р2) остаётся предметом W8 — волна его не
касалась.
