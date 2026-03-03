# IMPLEMENTATION_REPORT: Config Outbounds (Outbounds Configurator)

## Статус

Реализовано. Кнопка «Config Outbounds» на вкладке Sources & ParserConfig открывает окно конфигуратора outbounds; список (глобальные + по источникам), Edit/Delete/Add и диалог редактирования с scope, type, filters, default, addOutbounds. При закрытии окна ParserConfig в визарде обновляется.

## Изменения

1. **Кнопка в визарде**  
   В `ui/wizard/tabs/source_tab.go` добавлена кнопка «Config Outbounds» в ряд с Parse. По нажатию: синхронизация модели, парсинг ParserConfigJSON; при пустом/невалидном JSON показывается ошибка; при успехе открывается окно конфигуратора с копией ParserConfig и callback для применения (сериализация, обновление модели и виджета).

2. **Пакет outbounds_configurator**  
   - `configurator.go`: окно «Config Outbounds» со списком всех outbounds (сначала глобальные, затем по каждому источнику с подписью). У каждой строки — Edit, Delete. Кнопки Add и Close. При Close вызывается callback с текущим ParserConfig.  
   - `edit_dialog.go`: диалог Edit/Add с полями: Scope (For all / For source: …), Tag, Type (manual (selector) / auto (urltest)), Comment, Filters (ключ + значение, поддержка отрицания в значении, напр. `!/(🇷🇺)/i`), Preferred default (ключ + значение), AddOutbounds (чекбоксы direct-out, reject + чекбоксы по остальным тегам из списка). При Save собирается OutboundConfig и вызывается onSave; при редактировании сохраняются существующие Options и Wizard.

3. **Интеграция**  
   Callback при закрытии конфигуратора: SerializeParserConfig, обновление model.ParserConfigJSON и model.ParserConfig, вызов presenter.UpdateParserConfig(serialized).

## Изменённые файлы

- `todo/OUTBOUNDS_CONFIGURATOR/SPEC.md` — создан
- `todo/OUTBOUNDS_CONFIGURATOR/PLAN.md` — создан
- `todo/OUTBOUNDS_CONFIGURATOR/TASKS.md` — создан
- `ui/wizard/tabs/source_tab.go` — кнопка Config Outbounds и открытие конфигуратора
- `ui/wizard/outbounds_configurator/configurator.go` — новый файл (окно со списком)
- `ui/wizard/outbounds_configurator/edit_dialog.go` — новый файл (диалог Edit/Add)

## Команды для проверки

- `go build ./...` — в среде с CGO/OpenGL (проект использует Fyne)
- `go test ./...` — тесты (GUI-пакеты могут быть исключены)
- `go vet ./...`

## Риски и ограничения

- Фильтры в диалоге: одна пара ключ–значение; несколько пар и полная поддержка OR между объектами не реализованы (при необходимости можно расширить).
- В конфигураторе при Add для «For source» не создаётся новый источник — выбирается существующий; если источников нет, доступен только «For all».

## Assumptions

- «Префикс» из запроса пользователя интерпретирован как опциональный Comment у outbound (в ParserConfig у outbound нет отдельного поля prefix).
- Тип по умолчанию — manual (selector); direct-out и reject — стандартные теги для чекбоксов addOutbounds; остальные outbounds выбираются чекбоксами по тегам из уже существующих в конфиге.
