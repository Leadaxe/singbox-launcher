# SPEC 116 · Этап 3 — отчёт о реализации (папки в UI)

Свод волн W1–W12. Составлен W10 (прогон и сведение).
База этапа — коммит спеки `962fdba`; весь этап лежит рабочей копией
поверх него (в ветку не коммитился, `HEAD == 962fdba`).

Вердикты §O приняты капитаном 2026-08-29 — всюду рекомендованный
вариант **А**; ни один не вводит нового визуала и не трогает
`contract/**`. Пересмотру не подлежат, здесь только зафиксированы.

---

## 1. Что сделано

Папка стала полноценным контейнером узлов в UI: создаётся, наполняется
четырьмя путями, сливается с подпиской merge-правилом, отдаёт себя
целиком JSON'ом, удаляется с выбором судьбы узлов и никогда не теряется
из бэкапа молча. Сверх исходного плана этап забрал две волны:

- **W11** (утверждена Сашей 2026-08-30) — `Unsupported`-узлы:
  отбракованная провайдером запись перестала исчезать из состава и стала
  видимой диагностикой на своей позиции;
- **W12** — шесть точечных UI-фиксов этапа 2, найденных обкаткой
  этапа 3; новых виджетов, глифов и вкладок не заведено.

---

## 2. Файлы по волнам

| Волна | Файлы |
| --- | --- |
| **W1** — merge-заливка в папку | `core/state/subscription_merge.go` (общая половина `refreshMergedNode:43`, новый `MergeFolderNodesFromSubscription:188`); тест `core/state/folder_merge_test.go` |
| **W2** — перенос узла между контейнерами | `ui/configurator/business/node_move.go` (новый); тест `node_move_test.go` |
| **W3** — создание и удаление папки | `ui/configurator/tabs/source_tab.go`, `ui/configurator/business/sources.go` (`NextFolderName`) |
| **W4** — настройки папки в окне источника | `ui/configurator/tabs/source_edit_window.go` (третья ветка `rebuildSettingsLayout`, общие `tagPolicyBlock`/`detourBlock`), `ui/configurator/business/detour_refs.go` (`SourceDisplayName` предпочитает `Name` у контейнеров) |
| **W5** — список узлов и операции над узлом | `ui/configurator/tabs/preview_node_ops.go` (новый), `preview_node_info.go`, `source_edit_window.go`, `internal/fynewidget/drag_reorder.go` (`RegisterRecycled`, `DragHandle.SetIndex`, `Total`/`slots()`); тесты `tabs/node_dereference_test.go`, `tabs/node_reorder_test.go`, `business/node_rename_delete_test.go` |
| **W6** — наполнение папки | `ui/configurator/business/source_input.go` (новый — единственный разбор «текст → узлы»), `business/folder_fill.go` (новый), `business/sources.go`, `ui/configurator/tabs/folder_add_nodes.go` (новый), `ui/configurator/dialogs/add_server_dialog.go` + `warp_dialog.go` (параметр `owner fyne.Window`), `internal/platform/file_dialog*.go` (×5 — `PickOpenFiles` на трёх ОС); тест `business/folder_fill_test.go` |
| **W7** — заливка подписки в папку | `ui/configurator/business/folder_fill_subscription.go` (новый), `ui/configurator/tabs/folder_fill_from_sub.go` (новый), `core/state/subscription_merge.go` (`repointFolderAutoMembers:301`); тест `business/folder_fill_subscription_test.go` |
| **W8** — «взять всю папку → JSON» | `ui/configurator/tabs/folder_copy_json.go` (новый), `source_edit_json.go` (общий сборщик `unpackNodesDoc`), `source_edit_window.go` |
| **W9** — бэкап | `core/backup/export.go`, `import.go` (`Warning.Kind`+`Nodes`), `convert_v7.go`, `file.go`, `ui/configurator/tabs/settings_backup.go`, `settings_backup_report_window.go`; тесты `core/backup/export_folder_loss_test.go`, `tabs/settings_backup_report_test.go` |
| **W10** — прогон и сведение | этот отчёт; `bin/locale/ru.json` (14 недостающих ключей), `SPECS/features/sources.md` (расхождение с реализацией), `docs/release_notes/upcoming.md`, `SPECS/116-F-N-FOLDERS/CODEMAP.md` (два уехавших адреса) |
| **W11** — Unsupported-узлы | `core/state/sources_v7.go` (`IsUnsupported:152`, `NewUnsupportedNode:159`, поле `Reason`), `core/state/adapter_source.go` (отсечение в проекции), `core/state/subscription_merge.go` (переходы в обе стороны), `core/config/subscription/parse_body.go` (`RejectedBodyRecord`), `core/config/fetch_materialize.go` (вставка на позицию, `SubscriptionFetchMaterial.Supported`), `core/config/canonical_emit.go`, `core/config_service_subscriptions.go`, `ui/configurator/tabs/preview_rows.go` + `preview_row_view.go` + `source_edit_raw_body.go` (новые), `source_edit_overview.go`, `internal/fynewidget/secondary_tap_wrap.go` (`ttwidget.ToolTipWidget`); тесты `core/config/unsupported_node_test.go`, `tabs/preview_rows_test.go`; `SPECS/features/sources.md` §«Unsupported» |
| **W12** — UI-фиксы этапа 2 | `core/config/tag_guard.go`, `core/config/emission_warning.go` (новый), `core/config/nodelink_resolve.go`, `core/config/outbound_generator.go`, `core/config/build_report.go`, `core/build_report_feed.go`, `ui/configurator/tabs/final_report_model.go` + `final_tab.go`, `ui/settings_tab.go`, `bin/locale/ru.json`; тест `core/config/tag_guard_twin_test.go` |

Снесено: `SPECS/116-F-N-SERVER_FOLDERS/SPEC.md` — отменённая редакция
спеки, живёт в git-истории (ссылка стоит в `SPEC.md:5`).

Вне этапа и не мои: `ui/traffic/**` (изменения соседней сессии, в
приёмку этапа не входят и не трогались).

---

## 3. Решения, принятые по ходу

Отклонения и расширения, каждое с причиной. Полностью — в `TASKS.md`
построчно; здесь то, что меняет чужие ожидания.

1. **`nodeOpsAllowed` = «kind == folder», а не «kind != subscription»**
   (W5). У server/chain/auto СОСТАВА нет — их единственный узел это сам
   `Source`: он переименовывается полем формы и удаляется кнопкой строки
   «Источников». Второй путь к тем же действиям разъехался бы с
   первым — у формы есть сброс ссылок на Save, у меню его не было бы.

2. **`resetRefsAfterNodeRename` оставлен жить, rename/delete узла
   контейнера идут реестром W2** (W5). Он путь Save для ВЕРХНЕГО узла и
   зовёт `ResetDetourNodeRefs`, который (а) ГАСИТ ссылку, а при
   переименовании на месте узел никуда не делся и её надо ПЕРЕПИСАТЬ,
   (б) знает только `Source.Detour`, а у узла папки ссылок четыре вида.

3. **Move/Copy применяются к модели сразу, не буферизуются в scratch**
   (W5). Они затрагивают ДВА источника, а scratch знает один;
   буферизация оставила бы после Cancel два экземпляра узла с одним
   сырым тегом. Причина записана в шапке `preview_node_ops.go`.

4. **Авторазыменование делает ОБЩАЯ точка, а не побочка материализации**
   (W5). Признак «связь была» снимается ДО материализации — та
   пересаживает `Origin` целиком. Побочкой правка материализатора тихо
   отменила бы правило.

5. **`DragReorderGroup` расширен** (W5): список узлов — первый
   drag-список на `widget.List`, а тот переиспользует объекты строк.
   Добавлены `RegisterRecycled` (карта индекс→строка держится биекцией),
   `DragHandle.SetIndex` и `Total`/`slots()` (кламп броска по СПИСКУ
   ДАННЫХ). Поведение VBox-списков не изменилось: `Total == 0` →
   `slots() == count()`.

6. **Достоверность ответа считается по `Supported`, а не по длине
   `Nodes`** (W11). Иначе HTML-страница вместо подписки, разобранная в
   ноль принятых и N отбракованных, объявилась бы достоверной и снесла
   бы весь состав.

7. **Unsupported пропускается ДО тег-машины, структурно — в проекции**
   (W11). Не потребляет ни `{$num}`, ни слот уникализации: иначе одна
   битая строка у провайдера сдвинула бы финальные теги всех соседей, а
   с ними протухли бы выборы в кэше ядра и ссылки на финальный тег.

8. **Строки Preview строятся по составу `nodes[]`, а не по эмиссии**
   (W11). Тем же куском починены чекбоксы групп (у групп
   `config.NodeIdentity` пуст по построению, SPEC 112) и выключенный
   узел, который проходил эмиссию не всегда и исчезал из списка.

9. **Дедупликация гарда по владельцу** (W12 фикс 1). Твин-запись
   (`TwinOf != ""`) claim'ится как `TagOwnerTwin`; производная формула
   `d.Tag+twinSuffix` не применяется к тегу, за которым уже стоит своя
   запись списка. Настоящий дубль — по-прежнему конфликт.

10. **§O2 = финальные теги в выгрузке** (W8). Узлы берутся из
    `EmitCanonicalSource` со СВОИМ пустым `tagCounts` — документ
    уникализируется сам в себе, а не в контексте чужих источников; лимит
    показа `previewNodeCap` в выгрузке снят (`limit=0`), иначе «взять всю
    папку» отдало бы не всю папку.

---

## 4. Критерии приёмки A1–A10

| # | Критерий | Статус | Чем закрыт |
| --- | --- | --- | --- |
| **A1** | Пустая папка из ⋮; «0 nodes» без пометки «не дал узлов»; «8 of 10» после выключения двух; переживает перезапуск без сети | **закрыт кодом**, ручной прогон не выполнен (см. §5) | W3: `source_tab.go` (создание, строка списка, подавление пометки), формат счётчика существующий; persist — Save/Load v7 (SPEC 118) |
| **A2** | Префикс папки даёт теги `proton-*`; правка префикса не сбрасывает ни `enabled`, ни ссылку NodeLink | **закрыт кодом**, ручной прогон не выполнен | W4: `tagPolicyBlock` общий с подпиской; ссылки на СЫРОМ теге по построению (SPEC 112) — тег-политика их не касается |
| **A3** | Copy сохраняет `origin.subUrl`; move между папками везёт `enabled` и detour; move из подписки в меню нет; всякая ссылка либо переписана, либо названа | **ЗАКРЫТ** | W2 + тест `node_move_test.go` (8 случаев): `repointNodeLinks` переписывает detour/hops/members/default, неперезаписываемые ссылки корня НАЗВАНЫ в возврате `MoveNodeToFolder` |
| **A4** | Правка тега / body / Regen узла с непустым `subUrl` обнуляет его с уведомлением; следующая заливка узел не трогает | **ЗАКРЫТ** | W5 + тест `tabs/node_dereference_test.go` (правка тела / Regen снимают связь; узел без `subUrl` не меняется) |
| **A5** | Merge-заливка (data-критично): совпал / новый / исчез / исчез при `truncated` / недостоверный ответ | **ЗАКРЫТ** | W1 + тест `core/state/folder_merge_test.go` — все пять случаев, проверяются `enabled`, `detour`, `origin.subUrl`, порядок `nodes[]` |
| **A6** | «Copy nodes as JSON» принимается `sing-box check` как фрагмент конфига, теги по §O2 | **ЗАКРЫТ** | W8, проверено на папке с двумя одноимёнными vless (`F-alpha`/`F-alpha-2`) и wireguard (уехал в `endpoints[]`): `bin/sing-box check -c` → exit 0 |
| **A7** | «Вынести узлы в корень» не теряет ни одного: число верхних `Source` растёт ровно на число узлов, порядок и `enabled` целы; «удалить вместе» требует подтверждения | **ЗАКРЫТ** | W2 `ExtractFolderNodesToRoot` + тест `node_move_test.go`; диалог двух исходов — W3 `showFolderDeleteDialog` |
| **A8** | Окно папки без URL / интервала / max_nodes / skip; заголовок — имя папки, не «Source — » | **закрыт кодом**, ручной прогон не выполнен | W4: `switch` по kind в `rebuildSettingsLayout`, ветка заголовка `SourceKindFolder`; проверены ВСЕ употребления `isServerSource` (пять точек, две потребовали ветки папки) |
| **A9** | Экспорт бэкапа с папкой по §O1 и **никогда** не отдаёт успешный файл, молча потерявший папку | **ЗАКРЫТ** | W9 + тест `core/backup/export_folder_loss_test.go`: `backup.Export` зовётся ровно из одного места, отчёт показывается всегда при наличии предупреждения; потери «целиком» встают в начало списка |
| **A10** | Го1.20-гард: ни `min`/`max`/`clear`/`slices.`/`maps.`/`errors.Join`/`PathValue` в новых строках | **ЗАКРЫТ** | Греп по диффу этапа (`+`-строки, 9158 строк дельты): 4 вхождения — все в КОММЕНТАРИЯХ вида «go1.20-гард: без slices.Insert», ни одного употребления |

**A3–A7, A9, A10 закрыты машинно** (тесты + греп). **A1, A2, A8** —
чисто UI-наблюдаемые: код на месте и разобран построчно в `TASKS.md`,
но подтверждение «глазами на собранном приложении» требует запуска, а
он для этой волны запрещён (`-i` запрещён, бинарь не запускать).
Тестами эти три не покрываются намеренно — правило `no-ui-format-tests`.

---

## 5. Результат приёмки (полный прогон)

| Проверка | Результат |
| --- | --- |
| `go build ./...` | **зелено** (только `ld: warning: ignoring duplicate libraries: '-lobjc'` — постоянный шум линковщика) |
| `go vet ./...` | **зелено**, ноль замечаний |
| `go test -count=1 ./...` | **зелено**, ни одного FAIL по всему модулю |
| `go test -count=1 ./core/build` | **зелено** (`ok … 1.327s`) |
| `ETALON_V6MIG=1` (эмиссия v6mig) | **РОВНО одно** задекларированное расхождение **Р2**, те же три строки: `[P]auto` → `[P]select-auto` (тег, `outbounds[]` и `default` авто-группы свёртки `both`). Других расхождений нет. `capture` НЕ запускался, эталон не тронут |
| Го1.20-гард по диффу этапа (база `962fdba`) | **чисто** — 0 употреблений запрещённых конструкций |
| `go run ./tools/l10n/l10n_check` | **hard fails 0** (было 0 и до правки). Недостающих ключей стало 18 → 4, и все четыре — в `ui/traffic/toolbar.go`, вне границ этапа |
| Сценарии С1–С8 на собранном приложении | **НЕ ВЫПОЛНЕНО** — запуск сборки этой волне запрещён (`-i` запрещён, бинарь не запускать). Остаётся за капитаном |

### Что W10 поправил сам

- **`bin/locale/ru.json`** — 14 недостающих ключей (`Regen from raw`,
  `Group tag`, `MTU`, подсказки вкладки JSON, тексты отчёта миграции,
  строка переменных тег-политики и др.). Все — накопленный долг
  этапов 2–3 в файлах этапа; ключи только добавлены, ни один не удалён и
  не переписан.
- **`SPECS/features/sources.md` §«Наполнение папки»** — документ обещал
  ПЯТЬ путей наполнения, включая «создание цепочки (Chain) в папке».
  Такого пути в реализации нет: `folder_add_nodes.go` даёт вставку,
  импорт файлов, WARP, Add server и заливку подписки; отдельного «создать
  цепочку здесь» не заведено ни в W6, ни где-либо ещё. Список приведён к
  четырём фактическим, и вместо обещания записано, как цепочка в папке
  оказывается на самом деле: заводится в корне и переносится move to —
  узел с `Hops` папка держит наравне с любым другим
  (`cloneCanonicalNodeForMove` копирует `Hops`, `repointNodeLinks` их
  переписывает).
- **`CODEMAP.md`** — два адреса уехали от своих функций дальше шапки
  док-комментария: `MergeSubscriptionNodes` (`:80` → `:94`),
  `MergeFolderNodesFromSubscription` (`:175` → `:188`),
  `repointFolderAutoMembers` (`:288` → `:301`). Сверены ВСЕ `*.go:NN`
  карты — остальные попадают в шапку док-комментария своей декларации
  (конвенция карты), максимальный отрыв 7 строк.
- **`docs/release_notes/upcoming.md`** — папки и Unsupported-узлы,
  обе половины (EN/RU): наполнение, merge-заливка с сохранением правок,
  авторазыменование, видимая отбракованная запись, выгрузка JSON,
  удаление папки, потеря папки в экспорте — и техническая половина
  (единый разбор, реестр переписи, гард тегов, адресат предупреждений).

---

## 6. Судьба волн W11 и W12

Обе **выполнены и приняты в состав этапа 3**, отдельными пунктами
`TASKS.md` (все чекбоксы закрыты исполнителями).

- **W11 — Unsupported-узлы и диагностика источника.** Утверждена Сашей
  2026-08-30 уже по ходу этапа: меняет логику, поэтому тем же куском
  правился `SPECS/features/sources.md` (раздел «Unsupported: запись,
  которую не разобрали» + обещание пользователю). Инвариант эмиссии
  проверен тестом (`{$num}` и слот уникализации не потребляются) и
  подтверждён эталоном: `ETALON_V6MIG=1` даёт по-прежнему ровно одно
  расхождение Р2, новых W11 не добавила.
- **W12 — шесть UI-фиксов этапа 2 по обкатке.** Правки существующих
  виджетов и текстов; ничего нового не заведено. Тест написан только на
  гард (`tag_guard_twin_test.go`) — на вёрстку и формулировки тестов в
  проекте нет.

Отложено за границу этапа (вердикт §O1, вариант В): **трек `folders[]`
в контракте бэкапа** — отдельная задача с согласованием LxBox-стороны,
`contract/**` этапом 3 не тронут ни строкой.
Вердикт §O3 держится: **обратной заливки JSON → папка в этапе 3 нет**.

---

## 7. Открытое

1. **Ручной прогон С1–С8** на собранном приложении — единственная
   незакрытая строка приёмки. Критерии A1, A2, A8 ждут именно его.
2. **Четыре недостающих ключа локали в `ui/traffic/toolbar.go`**
   («Visible rows only», «Whole buffer», «Export live buffer», текст про
   активный фильтр) — работа соседней сессии, границы этапа 3, не
   трогались.
3. **`folders[]` в контракте бэкапа** — см. §6.
