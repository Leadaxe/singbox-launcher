# План: редактор источника (Edit) и `exclude_from_global` в ParserConfig

## 1. Архитектура UI

- **Файл:** `ui/wizard/tabs/source_tab.go` (и при необходимости вынести диалог в `ui/wizard/dialogs/` если разрастётся).
- Заменить `showSourceServersWindow` на что-то вроде `showSourceEditWindow`: сверху — как сейчас заголовок/закрытие; контент — **`container.AppTabs`** или **`widget.DocTabs`** Fyne с двумя табами:
  - **Настройки** — форма;
  - **Просмотр** — текущий список нод + статус (логика кэша `PreviewNodesBySource` / `RebuildPreviewCache` / fallback `fetchAndParseSource` сохраняется).
- **Префикс:** перенести редактирование из строки списка источников в окно **Настройки** (один источник правки), либо оставить в строке только отображение и дублировать в окне — предпочтительно **один** контрол (в окне), в строке краткий текст префикса для сканирования списка без открытия Edit.
- Синхронизация с моделью: те же пути, что у `prefixEntry.OnChanged` сегодня (`SerializeParserConfig`, `InvalidatePreviewCache`, `ScheduleRefreshOutboundOptionsDebounced`).

## 2. Маппинг чекбоксов → `proxies[].outbounds`

- Ввести в бизнес-слое визарда (`ui/wizard/business/`) функции вида `ApplyLocalOutboundsFromFlags(proxy *ProxySource, auto, select, exclude bool)` или держать логику в презентере — главное: **идемпотентно** включать/выключать заранее описанные элементы `OutboundConfig`, не затирая **прочие** ручные записи в `proxies[i].outbounds`.
- **Стратегия идентификации «наших» записей:** комментарий `wizard_managed` в `Comment`, или поле в `Options`, или фиксированные теги по шаблону `{tag_prefix}auto` / `{tag_prefix}select` — выбрать один способ в реализации и зафиксировать в IMPLEMENTATION_REPORT.
- **Локальный auto (`urltest`):** `type: urltest`, `filters` охватывают только ноды данного источника (как сейчас для локальных селекторов — через фильтр по префиксу тега или эквивалент, согласованный с существующими примерами в доке).
- **Локальный select (`selector`):** при включённом auto — `addOutbounds` или порядок `outbounds` в JSON должен включать тег auto **и** ноды; **`default`** = тег auto (в проекте часть опций в `options` — свериться с `GenerateSelectorWithFilteredAddOutbounds` и полями sing-box).

## 3. Поле `exclude_from_global` (рабочее имя)

- **`ProxySource`:** `ExcludeFromGlobal bool` `json:"exclude_from_global,omitempty"` (или другое имя после ревью — едино в коде и ParserConfig.md).
- **`GenerateOutboundsFromParserConfig` / `buildOutboundsInfo`:**
  - при сборке `allNodes` помечать каждую ноду индексом источника (`ParsedNode.SourceIndex int`, `-1` = неизвестно);
  - функция-обёртка `filterNodesForGlobalSelectors(allNodes, parserConfig) []*ParsedNode`, исключающая ноды, чей `SourceIndex` указывает на `proxies[j]` с `ExcludeFromGlobal == true`;
  - в `buildOutboundsInfo` для **глобальных** селекторов вызывать `filterNodesForSelector` на **отфильтрованном** списке; для локальных — без изменений (`nodesBySource[i]`).
- **WireGuard / endpoints:** уточнить в реализации: исключение касается только outbounds-нод или также endpoint-тегов в глобальных селекторах (если endpoint участвует в тех же фильтрах — скорее да, единая семантика «источник исключён из глобальных списков»).

## 4. Миграция и версия

- Если только новые optional поля без смены семантики старых — **версия 4** достаточна.
- Если меняется контракт версии — обновить `ParserConfigVersion`, `migrator.go`, фрагменты в **ParserConfig.md**.

## 5. Локализация

- `internal/locale/en.json`, `ru.json`: кнопка Edit, названия подвкладок, подписи чекбоксов, предупреждение про отсутствие локальных групп при exclude.

## 6. Документация и приёмка

| Артефакт | Действие |
|----------|----------|
| `docs/ParserConfig.md` | Поле `exclude_from_global`, сценарий с локальными auto/select, пример JSON. |
| `docs/release_notes/upcoming.md` | После реализации — кратко EN/RU. |
| `SPECS/026-.../IMPLEMENTATION_REPORT.md` | Заполнить по факту. |
| Папка задачи | По workflow README переименовать в `026-F-C-…` после завершения. |

## 7. Ориентир по файлам

| Зона | Файлы |
|------|--------|
| Модель | `core/config/configtypes/types.go`, при необходимости `ParsedNode` |
| Генератор | `core/config/outbound_generator.go`, возможно `outbound_filter.go` |
| Загрузка нод | место, где формируется `[]*ParsedNode` из `ProxySource` — проставить `SourceIndex` |
| Мигратор | `core/config/parser/migrator.go` |
| UI | `ui/wizard/tabs/source_tab.go`, `ui/wizard/business/*`, `ui/wizard/presentation/*` при необходимости |
| Тесты | `outbound_generator` / интеграционные сценарии exclude + локальные селекторы |
| Документация | `docs/ParserConfig.md` |

## 8. Риски

- Дублирование тегов при смене `tag_prefix` после создания локальных outbounds — при смене префикса обновлять теги управляемых записей или документировать ручную правку.
- Пустые глобальные селекторы, если все источники исключены и нет addOutbounds на локальные теги — ожидаемо; UI-предупреждение снижает сюрприз.
