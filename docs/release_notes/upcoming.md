# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **AmneziaWG (AWG 2.0) parameters supported.** WireGuard nodes can now carry the AWG obfuscation params — `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` (numbers) and the CPS packets `i1`–`i5` (strings) — parsed from `wireguard://` / `awg://` subscription URIs, generated into the `endpoints[]` config, and round-tripped back to share URIs without loss. (Runtime requires the sing-box-lx core; until that ships the config is built but the stock core rejects AWG fields — see SPEC 072.)
- **Debug API: "Regenerate token" button.** Settings → Debug API now has a Regenerate button next to Copy token. It rotates the bearer token (confirm dialog — the old token stops working immediately) and, if the API is running, restarts the listener with the new token.

### Technical / Internal
- Sources screen: deleting a subscription or server now asks for confirmation (matches the Rules tab) — no more one-click accidental removal.
- DNS-rule editor dialog: window titles ("Add/Edit DNS Rule") and the two validation errors ("Invalid JSON", "Rule is empty") are now localized (RU added). Field labels, placeholders and type names stay English by design.
- Sources list: the enable-toggle / delete / reorder handlers now share one `applySourceMutation` helper. Side effect of the consolidation: toggling a source on/off now also refreshes the rule outbound selectors (the toggle path previously skipped `RefreshOutboundOptions`, so a just-disabled source's outbounds could linger in the dropdowns until another action).

## RU
### Основное
- **Поддержка параметров AmneziaWG (AWG 2.0).** WireGuard-узлы теперь несут AWG-параметры обфускации — `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` (числа) и CPS-пакеты `i1`–`i5` (строки) — парсятся из подписочных URI `wireguard://` / `awg://`, эмитятся в `endpoints[]` конфига и без потерь сериализуются обратно в share-URI. (В рантайме нужно ядро sing-box-lx; до его подключения конфиг собирается, но stock-ядро отвергает AWG-поля — см. SPEC 072.)
- **Debug API: кнопка «Перегенерировать токен».** В Settings → Debug API рядом с «Копировать токен» появилась кнопка перегенерации. Она ротирует bearer-токен (с подтверждением — старый сразу перестаёт работать) и, если API запущен, перезапускает listener с новым токеном.

### Техническое / Внутреннее
- Экран «Серверы»: удаление подписки или сервера теперь спрашивает подтверждение (как в Rules-табе) — больше нет удаления в один клик по ошибке.
- Диалог редактора DNS-правил: заголовки окна («Добавить/Редактировать DNS-правило») и две ошибки валидации («Некорректный JSON», «Правило пустое») теперь локализованы (добавлен RU). Лейблы полей, плейсхолдеры и названия типов — намеренно английские.
- Список источников: обработчики toggle / delete / reorder сведены в один хелпер `applySourceMutation`. Побочный эффект консолидации: toggle источника теперь тоже обновляет outbound-селекторы правил (раньше toggle-путь пропускал `RefreshOutboundOptions`, и outbound'ы только что выключенного источника могли оставаться в дропдаунах до следующего действия).
