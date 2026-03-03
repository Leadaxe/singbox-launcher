# PLAN: Configurator Outbounds

## Компоненты

1. **Кнопка в визарде**  
   Файл: `ui/wizard/tabs/source_tab.go`. В строку с Parse/Documentation добавить кнопку "Config Outbounds". По нажатию: синхронизация GUI → модель, парсинг ParserConfigJSON в структуру; при успехе — открыть окно конфигуратора, передав текущий ParserConfig и callback для применения результата (обновить model.ParserConfigJSON и виджет).

2. **Окно конфигуратора**  
   Новый файл (или пакет): `ui/wizard/outbounds_configurator/` — окно Fyne с:
   - Список элементов: глобальные outbounds, затем по каждому `proxies[i]` — блок "Source N" / short URL с локальными outbounds. Каждая строка: текст (tag, type, scope), Edit, Delete.
   - Кнопки Add, Close.
   - При Close: сериализовать ParserConfig (wizardbusiness.SerializeParserConfig), вызвать callback с новым JSON; презентер обновит model и UpdateParserConfig.

3. **Диалог Edit/Add**  
   В том же пакете или `ui/wizard/dialogs/`: модальный диалог (fyne dialog или второе окно) с полями:
   - Scope: Select "For all" | "For source: ..." (список источников по индексу/URL).
   - Tag (Entry).
   - Type: Select "manual (selector)" | "auto (urltest)".
   - Comment (Entry, optional).
   - Filters: список пар ключ–значение (tag, host, scheme, label и т.д.); для каждого значения — чекбокс "negate" (добавить `!` к паттерну) или ввод уже с `!/.../i`.
   - Preferred default: одна пара ключ–значение (объект фильтра по умолчанию).
   - AddOutbounds: чекбоксы "direct-out", "reject" + MultiSelect или список выбора из тегов уже существующих outbounds (выше по списку).
   - Save / Cancel.

4. **Интеграция с моделью**  
   - Конфигуратор не зависит от presenter напрямую; получает при открытии: `*config.ParserConfig` (копию или указатель) и `onApply func(newParserConfig *config.ParserConfig)`.  
   - В source_tab при нажатии "Config Outbounds": парсить ParserConfigJSON; если ошибка — показать диалог ошибки; иначе открыть конфигуратор с копией ParserConfig и callback'ом, который вызывает presenter: обновить model.ParserConfig и ParserConfigJSON, SerializeParserConfig, UpdateParserConfig(serialized).

## Изменения по файлам

| Файл | Изменения |
|------|-----------|
| `ui/wizard/tabs/source_tab.go` | Добавить кнопку "Config Outbounds", обработчик: SyncGUIToModel, парсинг JSON, открытие окна конфигуратора с callback. |
| `ui/wizard/outbounds_configurator/configurator.go` (новый) | Окно со списком outbounds (глобальные + по источникам), кнопки Edit/Delete/Add, Close; вызов onApply при Close. |
| `ui/wizard/outbounds_configurator/list.go` или в configurator | Построение списка записей (tag, type, scope, sourceIndex для локальных). |
| `ui/wizard/outbounds_configurator/edit_dialog.go` (новый) | Диалог редактирования: scope, tag, type, comment, filters, preferredDefault, addOutbounds; возврат *OutboundConfig + scope (global vs sourceIndex). |
| `ui/wizard/presentation/gui_state.go` | При необходимости — ссылка на окно конфигуратора (опционально, можно создавать каждый раз). |

## Зависимости

- `config.ParserConfig`, `config.OutboundConfig`, `config.ProxySource` (core/config).
- `wizardbusiness.SerializeParserConfig` (ui/wizard/business).
- Презентер передаёт callback — без циклического импорта (конфигуратор в ui/wizard, вызывает переданный callback).

## Риски

- Сложная форма фильтров (ключ + значение + negate). Первая версия: один фильтр tag с одной строкой значения (поддержка `!/regex/i` в одной строке) и опционально preferredDefault в том же формате; расширение на несколько ключей — позже при необходимости.
