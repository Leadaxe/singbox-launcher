# PLAN — SPEC 116, этап 3 (папки в UI)

Точки встраивания — из `CODEMAP.md`. **Строки плывут при правках: перед
работой проверять грепом по имени функции, а не по номеру.**

Волны мелкие: **один агент = одна волна**. Каждая волна оставляет дерево
собираемым (`go build ./...`) и не ломает существующие тесты. Волны
W1–W3 независимы друг от друга по файлам; W4+ опираются на W1–W3.

Порядок обязателен там, где сказано «после».

---

## W1 — Модель: merge-заливка в папку (дыра Д1, критерий A5)

**Один файл, никакого UI.** Data-критичная волна, тесты обязательны.

Точка: `core/state/subscription_merge.go:50` `MergeSubscriptionNodes` —
сейчас первой строкой отбрасывает всё, что не `SourceKindSubscription`
(`:51`).

Работа:

1. Выделить общее ядро merge (сопоставление по сырому тегу, сохранение
   `enabled`/`detour`, порядок `nodes[]` = порядок свежего тела, запрет
   удаления при `Truncated` — `:83`) в приватную функцию с параметром
   политики исчезнувшего узла.
2. `MergeSubscriptionNodes` остаётся ровно как есть по поведению
   (политика: **удалить**), меняется только внутренность.
3. Новый экспортируемый вход `MergeFolderNodesFromSubscription(folder
   *Source, subURL string, res *SubFetchMaterial, trusted bool) (bool,
   []string)`:
   - ключ merge — пара (`origin.subUrl == subURL`, сырой тег);
     узлы папки с другим `subUrl` или без него **не участвуют**;
   - совпал → body/origin.raw освежить, `enabled`/`detour`/позицию
     сохранить;
   - новый → добавить включённым, `origin.subUrl = subURL`;
   - исчезнувший → **не удалять**, обнулить `origin.subUrl`, вернуть
     warning «исчез у провайдера»;
   - `Truncated == true` → разыменование не выполняется вовсе;
   - `trusted == false` → `nodes[]` не трогаются (как у подписки).
4. `PendingDisabled` у папки не существует — не трогать.

Тест: `core/state/folder_merge_test.go` — таблица из пяти случаев (A5).

Ловушки: порядок ключей `Node.Body` не пересобирать через `map`
(`core/config/body_keyorder.go`); го1.20-гард.

---

## W2 — Модель/бизнес: перенос узла между контейнерами (Д7, П2-механика)

**Без UI-виджетов**, только операции над моделью + сбор списка переписи.

Точки:
- `ui/configurator/business/` — новый файл `node_move.go` рядом с
  `detour_refs.go` (там же живёт `ResetDetourNodeRefs:36`).
- Переиспользовать: `business.ResetDetourNodeRefs`
  (`detour_refs.go:36`), `business.InvalidateNodePool`
  (`node_pool.go:150`), `WizardModel.BumpRevision`
  (`models/wizard_model.go:280`).

Работа — четыре чистые функции над `*wizardmodels.WizardModel`:

1. `CopyNodeToFolder(m, srcIndex int, rawTag string, dstFolderID string)
   ([]string, error)` — копия узла в `Nodes` целевой папки;
   `origin.subUrl` **сохраняется** (`features/sources.md` §«Наполнение
   папки» п.3); коллизия сырого тега в целевом контейнере
   разрешается суффиксом; возвращает список ссылок, чей финальный тег
   изменился.
2. `MoveNodeToFolder(m, srcIndex, rawTag, dstFolderID)` — то же плюс
   удаление из исходного контейнера. **Отказ, если исходный контейнер —
   подписка** (не-цель §3): состав подписки принадлежит провайдеру.
3. `ExtractFolderNodesToRoot(m, folderIndex int) []string` — каждый узел
   папки становится верхним `Source` (`corestate.Source{Node: …, ID:
   corestate.MakeULID()}`), порядок и `enabled` сохранены (критерий A7).
4. `DereferenceNodeOrigin(node *corestate.Node) bool` — обнуление
   `origin.subUrl` (общая точка для Д5/W5).

Все четыре: `BumpRevision` + `InvalidateNodePool` делает **вызывающий
UI**, а не эти функции (правило: бизнес-функция чистая, побочки — на
вызывающем, как у `ResetDetourNodeRefs`).

Тест: `node_move_test.go` — критерий A3 и A7 (состав/enabled/detour,
список переписи; текст диалогов не проверяем).

---

## W3 — UI: создание и удаление папки (П1, сценарии С1/С7, §O5/§O6)

Точки:
- `ui/configurator/tabs/source_tab.go:271` — меню ⋮
  (`fyne.NewMenu` с пятью пунктами); добавить «Add folder» согласно §O6.
- `ui/configurator/tabs/source_tab.go:816` `applySourceMutation` —
  общая цепочка мутации списка, звать её.
- Конструктор: `corestate.NewFolderSource(name)`
  (`core/state/sources_v7.go:267`) — **уже есть, вызвать его**, своего
  не писать.
- Имя по умолчанию: по образцу `business.NextChainLabel`
  (`business/sources.go:181`) — свободное `Folder N`.
- Удаление: строка списка уже несёт кнопку удаления — найти её в
  `refreshSourcesList` (`source_tab.go:340`+) рядом с `editBtn`
  (`:552`); для папки показать диалог двух исходов (С7), вызвав
  `business.ExtractFolderNodesToRoot` из W2 во втором.

Не делать: индикатор вида строки — решается §O5.

---

## W4 — UI: настройки папки в окне источника (Д3, критерий A8)

**После W3** (иначе нечего открывать).

Точки:
- `ui/configurator/tabs/source_edit_window.go:776` `rebuildSettingsLayout`
  — бинарная развилка `isServer`/иначе-подписка; добавить третью ветку
  для `SourceKindFolder`: **имя папки, tag prefix, tag postfix,
  подсказка переменных (`sourceTagVarsHintText`), `foldCheck`
  (свёртка), detour** — без URL, интервала, max_nodes, skip.
- `source_edit_window.go:779` `isServerSource` — переменная используется
  ещё в JSON-вкладке (`:1166`) и `syncFoldTabVisible` (`:1453`);
  проверить каждое употребление на предмет «не-server ⇒ подписка».
- `source_edit_window.go:416` — `switch s.Kind` заголовка окна: ветки
  `SourceKindFolder` нет, заголовок падает в `default`. Добавить: имя
  папки.
- Имя папки хранится в `Source.Name` (`sources_v7.go:158`) — не в
  `Label`; проверить `displayName()` (`adapter_source.go`) и
  `business.SourceDisplayName` (`detour_refs.go:111`).
- Вкладка Replace («Группа») у папки обязана появляться той же логикой
  `syncFoldTabVisible` (`:1453`) — условие `!isServerSource &&
  !isChainSource` уже её пропускает, проверить.

Критерий A8.

---

## W5 — UI: список узлов контейнера и операции над узлом (Д4, Д5, П2)

**После W2 и W4.** Самая крупная волна — при необходимости делится
надвое (W5a: меню операций; W5b: авторазыменование и уведомления).

Точки:
- `ui/configurator/tabs/source_edit_window.go:1084` — `NewSecondaryTapWrap`
  вокруг строки узла Preview; `:1092` `wrap.OnSecondary =
  showPreviewNodeContextMenu`.
- `ui/configurator/tabs/preview_node_info.go:44` — само меню (три пункта:
  Node info / Copy JSON / Copy tag). Добавить: **Move to folder…**,
  **Copy to folder…**, **Rename…**, **Delete**. Меню обязано получить
  сырой тег узла и вид контейнера — сейчас оно принимает только
  `*config.ParsedNode`; сырой тег известен вызывающему как
  `identities[id]` (`source_edit_window.go:1120`) — передать его
  параметром, не выводить из `node.Tag` (тот финальный).
- Пункты Move/Rename/Delete **отсутствуют, когда контейнер — подписка**
  (узлы подписки несвободны, `features/sources.md` §«Свобода и
  несвобода»); Copy to остаётся.
- Вызовы: `business.MoveNodeToFolder` / `CopyNodeToFolder` из W2,
  затем на вызывающей стороне `m.BumpRevision()` +
  `business.InvalidateNodePool(m)` + `presenter.MarkAsChanged()` — по
  образцу `applySourceEditToModel` (`source_edit_window.go:353`).
- Предупреждение о протухшем выборе — существующее
  `showStaleSelectionDialog` (`source_edit_window.go:1638`);
  перепись ссылок — `resetRefsAfterNodeRename:1571` +
  `showDetourRefsResetDialog:1614`. **Своих диалогов не заводить.**
- Авторазыменование (Д5): точка — там же, где правка узла меняет
  тег/body/raw. Для правки тега — новый пункт Rename этой волны; для
  body/Regen — `source_body_edit.go:40` `applyServerBodyJSON` и `:71`
  `regenServerBodyFromRaw`. Звать `business.DereferenceNodeOrigin`
  (W2) и показывать уведомление существующим
  `dialogs.ShowAutoHideInfo` (образец — `source_tab.go:544`).

Ловушки: value-snapshot окна (`cloneSource:190`) — операции над узлами
идут **по модели напрямую или по scratch?** Решение волны: Move/Copy
между контейнерами затрагивают ДВА источника, а scratch знает один —
значит эти операции применяются к модели немедленно (как fetch), а не
буферизуются до Save, и окно после них обязано перечитать `scratch`
из живой записи. Это обязано быть явно закомментировано в коде.

---

## W6 — UI: наполнение папки (Д6, сценарий С1, цель 2)

**После W4.**

Точки:
- `ui/configurator/business/sources.go:33` `AppendURLsToSources` —
  разбор входа (`carveSingboxJSON`, `classifyInputLines`) и
  материализация (`config.MaterializeServerNode:261` в
  `core/config/migrate_materialize.go`). Вынести общее ядро
  «текст → []corestate.Node» и дать второй вход
  `AppendNodesToFolder(ctx, folderID string, input string) error`,
  кладущий узлы в `Source.Nodes` папки вместо корня. **Второго разбора
  не заводить** — ловушка «эмиттер и парсер ходят парой».
- Уникализация сырого тега в пределах папки — там же (у подписки её
  делает `MaterializeSubscriptionBody`, у папки делать на входе).
- UI-кнопка «Add nodes…» в шапке списка узлов
  (`source_edit_window.go`, рядом с `previewStatus:832`), меню тех же
  путей, что у ⋮ Sources (`source_tab.go:271`): вставка текста, из
  файла (`applySourceMutation`-путь `:142`+), Add WARP
  (`wizarddialogs.ShowAddWarpDialog`, `source_tab.go:182`), Add server
  (`ShowAddServerDialog`, `:191`).
- Имя узла из имени файла при отсутствии `#fragment` — правило уже
  описано в `features/sources.md` §«Наполнение папки» п.1; путь файла
  известен в обработчике выбора файла (`source_tab.go:142`+).

---

## W7 — UI: заливка подписки в папку (П4, сценарий С5)

**После W1 и W6.**

Точки:
- Кнопка/пункт «Fill from subscription…» в окне папки (там же, где
  «Add nodes…» из W6).
- Список подписок для выбора: `m.Sources` с `Kind ==
  SourceKindSubscription` (образец фильтрации — `state.go:190`
  `GetSubscriptionSources`, в UI — по модели).
- Материал заливки: **уже материализованные `sub.Nodes`**, а не
  повторный fetch и не разбор тела (ловушка «сборка не парсит тела
  подписок»). Собрать `state.SubFetchMaterial{Nodes: копия sub.Nodes,
  Truncated: из sub.UpdateStatus}` и отдать в
  `MergeFolderNodesFromSubscription` (W1).
  Если у подписки `len(Nodes) == 0` — сказать «подписку ещё ни разу не
  обновляли», предложить обновить (существующий путь
  `refreshOneSourceFromUI`, `source_tab.go:1033`), заливку не делать.
- Копии Auto-узлов: `features/sources.md` §Auto — «Auto при копии/заливке
  в папку переуказывает members на узлы папки (совпадение по сырому
  тегу); член, чья копия не попала, — prune с warning». Реализуется
  здесь же, в W1-ядре или сразу после заливки.
- После: `BumpRevision` + `InvalidateNodePool` + `MarkAsChanged`.

---

## W8 — UI: «взять всю папку → JSON» (П3, §O2)

**После W4.** Мелкая волна.

Точки:
- Точка эмиссии — та же, что у сборки: `config.EmitNodeJSONs`
  (`core/config/outbound_generator.go:1040`); образец использования в
  UI — `source_edit_json.go:53` `renderUnpackedNodes` / `:38`
  `emittedToEditableJSON`.
- Действие: пункт «Copy nodes as JSON» в окне папки; буфер —
  `fynewidget.SetClipboard` (`preview_node_info.go:49`), уведомление —
  `dialogs.ShowAutoHideInfo` (`source_tab.go:544`).
- Теги в выгрузке — по решению §O2 (рекомендация: финальные).

---

## W9 — Бэкап: папка не теряется молча (Д2, критерий A9)

**После решения §O1.** Объём зависит от выбранного варианта.

Точки:
- `core/backup/export.go:98` — ветка `SourceKindFolder,
  SourceKindAuto` и `WarnBackupSourceKindUnsupported`.
- UI отчёта: `ui/configurator/tabs/settings_backup_report_window.go`
  (и `settings_backup.go`).
- **`contract/**` не трогать** — вариант В §O1 выносится в отдельную
  задачу с согласованием LxBox-стороны.

---

## W10 — Прогон и сведение

- `go build ./...`, `go vet ./...`, `go test ./...`.
- Го1.20-гард: греп по `slices.`, `maps.`, `errors.Join`, `min(`,
  `max(`, `clear(`, `PathValue` в изменённых файлах (Win7-джоба CI).
- Локали: новые строки — через `locale.T`/`Tf`; ключи в `bin/locale/ru.json`.
- Сценарии С1–С8 прогоняются руками на собранном приложении
  (`build_darwin.sh -i`).
- `CODEMAP.md` обновляется новыми адресами (merge папки, node_move,
  ветка настроек папки, меню узла).

---

## W12 — UI-фиксы этапа 2 по обкатке (после W11)

Шесть точечных правок существующих виджетов и текстов, найденных обкаткой
этапа 3. Новых виджетов, глифов и вкладок не заводится.

Точки:
- Гард: `core/config/tag_guard.go` `BuildTagGuard` — пара «Направление +
  его развёрнутый твин» больше не считается двумя претендентами на
  `<tag>-auto` (форма приходит уже после `ExpandDirectionTwins`).
- Локаль: `core/config/emission_warning.go` — английские ключи константами,
  перевод в `bin/locale/ru.json`; туда же переехали значения
  `TagOwnerKind` и причины `NodeLinkTargets.Resolve`.
- Адресат: `EmissionWarning{Text,SourceID,SourceLabel,DirectionTag}` вместо
  строки; `core/build_report_feed.go` кладёт ULID в запись отчёта,
  `config.EmitWarningsForSource` отдаёт их строке Sources.
- Статус сборки: `ui/configurator/tabs/final_report_model.go`
  `finalBuildStatusText` + `statusLabel` в `final_tab.go`.
- Кнопка: `Copy config` иконкой `theme.ContentCopyIcon()` — как «Copy
  token» в `ui/settings_tab.go`.
- Поля: `ui/settings_tab.go` — короткие «Default update interval» /
  «Default max nodes», широкий «Device ID (HWID)».

Тесты — только на гард (`core/config/tag_guard_twin_test.go`): на вёрстку
и формулировки тестов в проекте нет.
