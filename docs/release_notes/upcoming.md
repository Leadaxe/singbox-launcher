# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Highlights

- **NaïveProxy (`naive+https://` / `naive+quic://`) support** in subscription parser and share-URI encoder. Parses user/pass, `extra-headers`, and QUIC vs HTTP/2 transport per the de-facto DuckSoft URI spec; emits sing-box `"type": "naive"` outbound with TLS block limited to `server_name` (matching sing-box naive capabilities). `padding=` URI param is logged and ignored (no sing-box equivalent). Right-click → Copy link round-trips back to a valid URI (keys in `extra-headers` sorted lexicographically for determinism). Requires sing-box ≥ 1.13.0 built with `with_naive_proxy`. See [SPEC 044](../../SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md) and `docs/ParserConfig.md` → **NaïveProxy**.

- **macOS wake-from-sleep re-sync** — IOKit `IORegisterForSystemPower` hook in `internal/platform/power_darwin.go`. Closes the macOS half of [SPEC 011](../../SPECS/011-B-C-launcher-freeze-after-sleep/SPEC.md). After resume the launcher resets the Clash API transport, refreshes proxies and re-pings if auto-ping is on — same behavior as Windows / Linux. CFRunLoop runs on a `runtime.LockOSThread`-pinned goroutine; sleep transitions are auto-acknowledged via `IOAllowPowerChange` so the system doesn't stall.
- **Per-source counts in the Update toast.** When at least one subscription source fails or silently produces zero nodes, the success toast now reads `Config updated: 2/3 source(s) succeeded (1 failed)` instead of a blanket «successfully». A source counts as failed if `loadNodesFunc` returns an error or zero nodes — silent-empty is failure from the user's perspective. All-good case still shows the original «Config updated successfully!».
- **Tooltips on Core Dashboard buttons reveal keyboard shortcuts.** `Update` and `Restart` now show «Update subscriptions (⌘+U)» / «Restart sing-box (⌘+R)» on hover (Ctrl on Windows / Linux). Helper `platform.ShortcutModifierLabel()` resolves the platform-specific symbol; both buttons switched to `ttwidget.Button` for tooltip support.
- **Wizard template — auto-degrade `type` to `enum` when `options` use object form `[{title, value}]`.** The free-text combo widget (used for `type: "text"`) cannot safely round-trip object-form options: typed text bypasses the title→value mapping and lands in `config.json` as the literal display label. The fix normalizes `Type` at JSON-unmarshal time — once any option element is `{title, value}`, the var is treated as `enum` (strict dropdown) regardless of the declared type. Legacy plain-string options (`["a","b"]`) are unaffected. Fixes the URLTest preset bug where picking «5m (default)» from the dropdown left the literal string «5m (default)» in the config instead of `5m`.

### Technical / Internal

-

---

## RU

### Основное

- **Поддержка NaïveProxy (`naive+https://` / `naive+quic://`)** в парсере подписок и share-URI энкодере. Парсятся user/pass, `extra-headers` и выбор транспорта (HTTP/2 vs QUIC) по де-факто спеке DuckSoft; собирается sing-box outbound `"type": "naive"` с TLS-блоком только `server_name` (ровно то, что поддерживает sing-box naive). Параметр `padding=` логируется и игнорируется (нет соответствия в sing-box). ПКМ → «Copy link» корректно round-trip'ит обратно в валидный URI (ключи `extra-headers` сортируются лексикографически). Требует sing-box ≥ 1.13.0 со сборкой `with_naive_proxy`. См. [SPEC 044](../../SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md) и `docs/ParserConfig.md` → **NaïveProxy**.

- **macOS wake-from-sleep re-sync** — хук IOKit `IORegisterForSystemPower` в `internal/platform/power_darwin.go`. Закрывает macOS-половину [SPEC 011](../../SPECS/011-B-C-launcher-freeze-after-sleep/SPEC.md). После резюма лаунчер сбрасывает HTTP-транспорт Clash API, перечитывает список прокси и re-пингует ноды (если автопинг включён) — то же поведение, что и на Windows / Linux. CFRunLoop крутится на goroutine с `runtime.LockOSThread`; sleep-транзишены авто-подтверждаются через `IOAllowPowerChange`, чтобы система не залипала.
- **Счётчик источников в тосте Update.** Если хотя бы один источник упал или молча отдал ноль нод, тост успеха теперь пишет `Config updated: 2/3 source(s) succeeded (1 failed)` вместо общего «successfully». Источник считается failed, если `loadNodesFunc` вернул ошибку или ноль нод — «молчаливый ноль» = failure с точки зрения пользователя. Полный успех — оставлен старый текст.
- **Tooltips на кнопках Core Dashboard показывают горячие клавиши.** `Update` и `Restart` при наведении показывают «Обновить подписки (⌘+U)» / «Перезапустить sing-box (⌘+R)» (Ctrl на Windows / Linux). Helper `platform.ShortcutModifierLabel()` отдаёт платформенный символ; обе кнопки переведены на `ttwidget.Button`.
- **Шаблон визарда — автоматическая деградация `type` в `enum` для object-формы `options: [{title, value}]`.** Combo-виджет (`type: "text"` со свободным вводом) не может безопасно round-trip'ить object-форму: вписанный пользователем текст обходит маппинг title→value и попадает в `config.json` буквальной display-подписью. Фикс нормализует `Type` на этапе JSON unmarshal: как только хотя бы один элемент `options` в форме `{title, value}` — переменная трактуется как `enum` (строгий дропдаун), независимо от заявленного типа. Legacy plain-string options (`["a","b"]`) не затронуты. Лечит баг URLTest: выбор «5m (default)» из дропдауна оставлял в конфиге буквальную строку «5m (default)» вместо `5m`.

### Техническое / Внутреннее

-
