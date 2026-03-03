# SPEC: Configurator Outbounds (Config Outbounds)

## Проблема

Пользователь редактирует ParserConfig вручную в JSON. Настройка outbounds (селекторов и urltest) — фильтры, addOutbounds, default, тип — требует знания формата и частых правок в тексте. Нужен визуальный конфигуратор: список всех outbounds, кнопки Edit/Delete/Add и форма редактирования с выбором scope (глобальный / для источника), типа (auto/manual), фильтров и default/addOutbounds.

## Требования

1. **Кнопка в визарде**  
   На вкладке "Sources & ParserConfig" рядом с ParserConfig — кнопка **[Config Outbounds]**. По нажатию открывается окно конфигуратора outbounds.

2. **Окно конфигуратора**  
   - Заголовок: "Parser Outbounds" (или "Config Outbounds").  
   - Список всех outbounds в порядке: сначала глобальные (`ParserConfig.outbounds`), затем по каждому источнику — локальные (`proxies[i].outbounds`) с подписью источника (например, URL или "Source 1", "Source 2").  
   - У каждого элемента списка: отображаемый текст (tag, type, scope), кнопки **Edit**, **Delete**.  
   - Кнопка **Add** (добавить новый outbound).  
   - Кнопка **Close** (закрыть окно; при закрытии применить изменения к ParserConfig в визарде и обновить поле ParserConfig).

3. **Редактирование / добавление outbound**  
   Диалог (модальный или в том же окне) с полями:
   - **Scope**: "For all" (глобальный) или "For specific source" + выбор источника из списка (тогда outbound создаётся в `proxies[sourceIndex].outbounds`).  
   - **Tag**: строка (имя outbound).  
   - **Type**: "manual" (selector) или "auto" (urltest); по умолчанию manual.  
   - **Prefix**: опционально (если имеется в виду префикс для тегов узлов — в текущей модели это на уровне ProxySource; в outbound префикса нет; трактуем как опциональное поле или снимаем, если не нужно — оставляем зарезервированным или как comment/label). Уточнение: в ParserConfig у outbound есть только tag, type, options, filters, addOutbounds, preferredDefault, comment. "Префикс" в запросе пользователя может означать префикс тега или comment; делаем поле **Comment** (опционально).  
   - **Filters**: настройка фильтров (ключ + значение/паттерн). Поддержка отрицания: паттерн вида `!/regex/i` или отдельный чекбокс "negate". Несколько пар ключ–значение (tag, host, scheme и т.д. по ParserConfig.md).  
   - **Default (preferredDefault)**: опционально; фильтр для выбора узла по умолчанию (например, `{"tag": "/🇳🇱/i"}`).  
   - **AddOutbounds**: список тегов, добавляемых в начало списка outbounds селектора. Реализация: чекбоксы для фиксированных "direct-out", "reject" (если такие теги приняты в шаблоне) + выбор из уже созданных outbounds (теги выше по списку в конфигураторе).  
   - Кнопки **Save** / **Cancel**.

4. **Сохранение**  
   После Save в диалоге или Delete в списке — конфигуратор обновляет внутреннюю модель (ParserConfig). При закрытии окна конфигуратора — сериализовать ParserConfig в JSON и записать в `model.ParserConfigJSON` и обновить виджет ParserConfig в визарде (через презентер). Если ParserConfig был пустой или невалидный при открытии конфигуратора — показать ошибку и не открывать или открыть с пустым списком и разрешить добавить outbounds (и при необходимости один источник).

5. **Валидация**  
   - Теги outbounds уникальны в рамках глобальных и в рамках каждого источника.  
   - При сохранении проверять валидность JSON и структуры; при ошибке показывать сообщение и не применять к визарду.

## Критерии приёмки

- [ ] Кнопка "Config Outbounds" видна на вкладке Sources & ParserConfig рядом с Parse/Documentation.  
- [ ] По нажатию открывается окно со списком всех outbounds (глобальные + по источникам).  
- [ ] Edit/Delete/Add работают; при Add и Edit открывается форма с scope, tag, type, filters, default, addOutbounds (direct/reject + другие outbounds), comment.  
- [ ] Type: manual (selector) / auto (urltest), по умолчанию manual.  
- [ ] Фильтры поддерживают отрицание (например, `!/(🇷🇺)/i`).  
- [ ] После закрытия окна конфигуратора ParserConfig в визарде обновлён (текст и модель).  
- [ ] Сообщения в UI на английском; логирование через debuglog.

## Структуры данных

- Используются существующие: `config.ParserConfig`, `config.OutboundConfig`, `config.ProxySource`.  
- Конфигуратор читает из `model.ParserConfigJSON` (или из `model.ParserConfig` после парсинга), при закрытии пишет обратно через сериализацию и `presenter.UpdateParserConfig(...)` / обновление модели.

## Ограничения

- Не менять формат ParserConfig (версия 4).  
- Не выполнять коммиты/пуш без явного указания пользователя.  
- UI и сообщения — только английский.
