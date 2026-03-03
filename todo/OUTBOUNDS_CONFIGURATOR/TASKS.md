# TASKS: Configurator Outbounds

## Этап 1: Кнопка и открытие окна

- [ ] **1.1** В `ui/wizard/tabs/source_tab.go` добавить кнопку "Config Outbounds" в headerRow (рядом с Parse / Documentation).
- [ ] **1.2** Обработчик кнопки: SyncGUIToModel; парсить model.ParserConfigJSON в config.ParserConfig; при пустом/невалидном JSON показать dialog с ошибкой и не открывать окно.
- [ ] **1.3** При валидном парсинге открыть окно конфигуратора (новый пакет ui/wizard/outbounds_configurator), передать копию ParserConfig и callback: сериализовать, обновить model.ParserConfigJSON и model.ParserConfig, вызвать presenter.UpdateParserConfig(serialized).

## Этап 2: Окно конфигуратора (список)

- [ ] **2.1** Создать пакет `ui/wizard/outbounds_configurator`, файл `configurator.go`: NewWindow(parserConfig *config.ParserConfig, parent fyne.Window, onApply func(*config.ParserConfig)) — создаёт окно с заголовком "Config Outbounds".
- [ ] **2.2** В окне: список всех outbounds — сначала ParserConfig.outbounds (подпись "Global"), затем для каждого proxies[i] — блок с подписью источника (source URL или "Source 1", "Source 2") и proxies[i].Outbounds. Каждая строка: label (tag, type), кнопки Edit, Delete.
- [ ] **2.3** Кнопка Add: открыть диалог добавления (scope, tag, type, …); при Save вставить outbound в нужное место (global или proxies[i].Outbounds) и обновить список в окне.
- [ ] **2.4** Edit: открыть диалог с заполненными полями; при Save заменить выбранный outbound и обновить список.
- [ ] **2.5** Delete: удалить outbound из слайса и обновить список.
- [ ] **2.6** Кнопка Close: вызвать onApply(parserConfig), закрыть окно. Конфигуратор работает с переданной копией и модифицирует её; callback получает указатель на тот же объект.

## Этап 3: Диалог Edit/Add

- [ ] **3.1** Файл `edit_dialog.go`: ShowOutboundEditDialog(parent, parserConfig, existing *OutboundConfig, scopeKind string, sourceIndex int, existingTags []string, onSave func(cfg *OutboundConfig, scopeKind string, sourceIndex int)).
- [ ] **3.2** Поля: Scope (For all / For source: …), Tag, Type (manual/auto), Comment, Filters (минимум: один ключ tag со значением, поддержка отрицания в значении или чекбокс), Preferred default (одна пара), AddOutbounds (чекбоксы direct-out, reject + выбор из existingTags).
- [ ] **3.3** При Save: собрать OutboundConfig, вызвать onSave; закрыть диалог.

## Этап 4: Интеграция и проверка

- [ ] **4.1** Убедиться, что при закрытии конфигуратора визард получает обновлённый ParserConfig JSON и виджет обновляется (SyncModelToGUI или UpdateParserConfig).
- [ ] **4.2** Логирование: debuglog при открытии/закрытии конфигуратора, при Apply, при ошибках парсинга.
- [ ] **4.3** Сообщения пользователю на английском; ошибки через dialogs.ShowError.
- [ ] **4.4** Проверка: go build ./..., go test ./..., go vet ./...
