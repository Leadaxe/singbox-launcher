# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Debug API: "Regenerate token" button.** Settings → Debug API now has a Regenerate button next to Copy token. It rotates the bearer token (confirm dialog — the old token stops working immediately) and, if the API is running, restarts the listener with the new token.

### Technical / Internal
- Sources list: the enable-toggle / delete / reorder handlers now share one `applySourceMutation` helper. Side effect of the consolidation: toggling a source on/off now also refreshes the rule outbound selectors (the toggle path previously skipped `RefreshOutboundOptions`, so a just-disabled source's outbounds could linger in the dropdowns until another action).

## RU
### Основное
- **Debug API: кнопка «Перегенерировать токен».** В Settings → Debug API рядом с «Копировать токен» появилась кнопка перегенерации. Она ротирует bearer-токен (с подтверждением — старый сразу перестаёт работать) и, если API запущен, перезапускает listener с новым токеном.

### Техническое / Внутреннее
- Список источников: обработчики toggle / delete / reorder сведены в один хелпер `applySourceMutation`. Побочный эффект консолидации: toggle источника теперь тоже обновляет outbound-селекторы правил (раньше toggle-путь пропускал `RefreshOutboundOptions`, и outbound'ы только что выключенного источника могли оставаться в дропдаунах до следующего действия).
