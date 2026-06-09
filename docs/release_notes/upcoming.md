# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Core switched to the sing-box-lx fork (v1.13.13-lx.3).** The launcher now downloads and pins the [sing-box-lx](https://github.com/Leadaxe/sing-box-lx) core, which builds in XHTTP (`with_xhttp`) and AmneziaWG 2.0 (`with_awg`) — so the two features below actually run. Downloaded archives are now **SHA256-verified** against the release `SHA256SUMS` (soft-degrades if unavailable). Use Core Dashboard → Download/Reinstall to pull the new core. **Windows 7 (32-bit)** has no fork build and stays on upstream SagerNet `1.13.12` (no XHTTP/AWG there) — nothing breaks for Win7 users.
- **XHTTP transport supported (fixes a silent regression).** Subscription nodes with `type=xhttp` were silently degraded to sing-box `httpupgrade` — a different wire protocol, so XHTTP+Reality nodes failed to connect and the `mode`/`x_padding_bytes`/`no_grpc_header` fields were dropped. XHTTP is now parsed, carried, generated into `config.json`, and round-tripped to share URIs as a real `xhttp` transport (VLESS/VMess/Trojan). The old `httpupgrade ⇄ xhttp` URL mislabeling is fixed (httpupgrade now exports as `type=httpupgrade`). Runs on the bundled sing-box-lx core; not available on the Win7 32-bit build.
- **AmneziaWG (AWG 2.0) parameters supported.** WireGuard nodes can now carry the AWG obfuscation params — `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` (numbers) and the CPS packets `i1`–`i5` (strings) — parsed from `wireguard://` / `awg://` subscription URIs, generated into the `endpoints[]` config, and round-tripped back to share URIs without loss. Runs on the bundled sing-box-lx core; not available on the Win7 32-bit build.
- **Debug API: "Regenerate token" button.** Settings → Debug API now has a Regenerate button next to Copy token. It rotates the bearer token (confirm dialog — the old token stops working immediately) and, if the API is running, restarts the listener with the new token.

### Technical / Internal
- Sources screen: deleting a subscription or server now asks for confirmation (matches the Rules tab) — no more one-click accidental removal.
- DNS-rule editor dialog: window titles ("Add/Edit DNS Rule") and the two validation errors ("Invalid JSON", "Rule is empty") are now localized (RU added). Field labels, placeholders and type names stay English by design.
- Sources list: the enable-toggle / delete / reorder handlers now share one `applySourceMutation` helper. Side effect of the consolidation: toggling a source on/off now also refreshes the rule outbound selectors (the toggle path previously skipped `RefreshOutboundOptions`, so a just-disabled source's outbounds could linger in the dropdowns until another action).

## RU
### Основное
- **Ядро переключено на форк sing-box-lx (v1.13.13-lx.3).** Лаунчер теперь качает и пиннит ядро [sing-box-lx](https://github.com/Leadaxe/sing-box-lx), в котором собраны XHTTP (`with_xhttp`) и AmneziaWG 2.0 (`with_awg`) — поэтому обе фичи ниже реально работают. Скачанные архивы теперь **проверяются по SHA256** против `SHA256SUMS` релиза (мягкая деградация, если недоступен). Обновить ядро: Core Dashboard → Download/Reinstall. **Windows 7 (32-бит)** не имеет форк-сборки и остаётся на upstream SagerNet `1.13.12` (без XHTTP/AWG) — для Win7-пользователей ничего не ломается.
- **Поддержка транспорта XHTTP (чинит тихую регрессию).** Узлы подписок с `type=xhttp` молча деградировали в sing-box `httpupgrade` — это другой wire-протокол, поэтому XHTTP+Reality узлы не подключались, а поля `mode`/`x_padding_bytes`/`no_grpc_header` терялись. Теперь XHTTP честно парсится, переносится, эмитится в `config.json` и сериализуется обратно в share-URI как настоящий `xhttp` (VLESS/VMess/Trojan). Исправлена путаница в URL `httpupgrade ⇄ xhttp` (httpupgrade теперь экспортируется как `type=httpupgrade`). Работает на bundled-ядре sing-box-lx; недоступно на сборке Win7 32-бит.
- **Поддержка параметров AmneziaWG (AWG 2.0).** WireGuard-узлы теперь несут AWG-параметры обфускации — `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` (числа) и CPS-пакеты `i1`–`i5` (строки) — парсятся из подписочных URI `wireguard://` / `awg://`, эмитятся в `endpoints[]` конфига и без потерь сериализуются обратно в share-URI. Работает на bundled-ядре sing-box-lx; недоступно на сборке Win7 32-бит.
- **Debug API: кнопка «Перегенерировать токен».** В Settings → Debug API рядом с «Копировать токен» появилась кнопка перегенерации. Она ротирует bearer-токен (с подтверждением — старый сразу перестаёт работать) и, если API запущен, перезапускает listener с новым токеном.

### Техническое / Внутреннее
- Экран «Серверы»: удаление подписки или сервера теперь спрашивает подтверждение (как в Rules-табе) — больше нет удаления в один клик по ошибке.
- Диалог редактора DNS-правил: заголовки окна («Добавить/Редактировать DNS-правило») и две ошибки валидации («Некорректный JSON», «Правило пустое») теперь локализованы (добавлен RU). Лейблы полей, плейсхолдеры и названия типов — намеренно английские.
- Список источников: обработчики toggle / delete / reorder сведены в один хелпер `applySourceMutation`. Побочный эффект консолидации: toggle источника теперь тоже обновляет outbound-селекторы правил (раньше toggle-путь пропускал `RefreshOutboundOptions`, и outbound'ы только что выключенного источника могли оставаться в дропдаунах до следующего действия).
