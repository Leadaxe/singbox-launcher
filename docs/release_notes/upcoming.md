# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Internal / Refactoring

- **Wizard presenter:** `SyncGUIToModelWithoutDirtyFlag` renamed to **`MergeGUIToModel`** (clearer: merge widgets into model without touching **`hasChanges`**). Single contract comment block in **`ui/wizard/presentation/presenter_sync.go`**.

### Highlights

- **Wizard & main window — UI/UX (gutter, Rules, Sources):** **Right scrollbar gutter** where scrollbars used to cover text or controls: **Rules** (custom-rules list), **Sources** (URL field, source list, server preview, tab scroll), **DNS** (server list only; not the rules JSON field), and the main **Servers** tab (**inside each proxy row**, after the buttons — not an outer margin beside the scrollbar). **Rules — custom rules:** **Up/Down** reorder, **scroll position** kept after moves, **delete** asks for **confirmation**; refreshing outbound selects no longer clears **unsaved** edits. **Sources:** compact **label + copy** per row instead of a wide text button.

- **Wizard — unsaved prompt:** Tab switches use a **quiet** GUI→model sync (no spurious **dirty** flag). **Save before close** appears only when data actually changed; opening the wizard, switching tabs, and closing without edits no longer triggers the save/discard dialog. Closing still runs a full sync first so the current tab’s edits are reflected in the check. **Outbounds** list (**↑/↓**, Edit, Add, Delete): the configurator updates the JSON entry under `ParserConfigUpdating`, so **`MarkAsChanged`** is called explicitly after apply — reorder and other list actions now correctly trigger the unsaved dialog.

- **Wizard — DNS tab:** New **DNS** tab (between Outbounds and Rules) edits `dns.servers`, **`dns.rules` as one JSON object** `{"rules":[...]}` (indented; legacy one-object-per-line input still accepted), `dns.final`, `strategy`, `independent_cache`, and `route.default_domain_resolver`. State is saved under root **`dns_options`** in `state.json` only (no duplicate in **`config_params`**; legacy `config_params` entries are read once on load — see **docs/WIZARD_STATE.md**). Each server row has an **enabled** checkbox (wizard-only; stored in `state.json` with optional `description`); disabled servers are omitted from generated sing-box `dns.servers`. **Final**, **default domain resolver**, and rule `server` tags refer to **enabled** servers only; skeleton `config.dns` rows stay visible in the server list but do not appear in those dropdowns until checked (when checked, **`dns_options`** can override the row body per tag). The **enabled** checkbox for skeleton rows is **disabled** (read-only display of inclusion from state/template). Hover the summary label to see `description` from JSON.

- **Wizard — subscription URL fragment:** If a subscription URL contains a `#fragment` (e.g. `#abvpn`), Apply/Append sets `tag_prefix` from that fragment (sanitized, with a trailing `:` like numeric prefixes) when no `tag_prefix` is already stored for that source.

- **Wizard — UTF-8 labels:** Source/outbound labels are truncated by **Unicode code points** (currently up to **60** visible characters before `...`), not raw bytes, so Cyrillic, emoji flags, and punctuation (e.g. `»`, `❯`) no longer break when the UI shortens long strings. VLESS URI **fragments** are decoded with `PathUnescape` so a literal `+` in the name is not turned into a space. **Preview / server list:** subscription lines and `sanitizeForDisplay` no longer iterate broken UTF-8 (which used to insert U+FFFD); strings are cleaned with `ToValidUTF8` before parse and before Fyne; outbound configurator row text uses the same rune-safe truncation. **Abvpn-style `❯` (U+276F) in tags:** when **reading** subscriptions, `internal/textnorm.NormalizeProxyDisplay` maps `❯` / `»` / `›` to ASCII ` > ` on labels and tags (so generated `config.json` matches what the UI shows). **Servers tab (Clash API):** each `ProxyInfo` keeps the raw API `Name` for requests; `DisplayName` is filled at fetch time with the same normalization for list labels, tray submenu, and status text.

- **VLESS / Trojan subscription links:** Parser and `GenerateNodeJSON` build sing-box [V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) from URI query: `ws` (path, headers `Host` — if `host=` is missing, **`sni` is used** for `Host`, e.g. abvpn-style `type=ws&sni=…` only), `http` (`host` as JSON list, path), `grpc` (`service_name`), `xhttp` → `httpupgrade` (only `host` / `path` / `headers` per docs; Xray `mode` is not in the schema). VLESS `security=none` omits TLS; plain TLS and Reality (`pbk`) follow [outbound TLS](https://sing-box.sagernet.org/configuration/shared/tls/#outbound). **REALITY over plain TCP** with no `flow` in the URI gets **`flow: xtls-rprx-vision`** (not applied when transport is `ws`/`grpc`/`http`/`xhttp`). Trojan + WS gets `transport` + `tls`. VMess WS uses the same `host` / `sni` fallback for `Host`. VMess gRPC uses `service_name` from JSON `path`. Wizard preview deduplicates tags like the main parser (`MakeTagUnique`). Query keys are matched case-insensitively where providers use `allowinsecure=0`; multiply-encoded `alpn` is normalized; `fp=QQ` maps to utls `qq`; `tcp`/`raw` with `headerType=http` maps to HTTP transport; `packetEncoding` is copied to outbound `packet_encoding`.

- **VLESS `xtls-rprx-vision-udp443`:** Subscriptions often use Xray’s vision-udp443 flow; sing-box only accepts `xtls-rprx-vision`. The parser already mapped this internally, but generated `config.json` still wrote the original flow and omitted `packet_encoding`. Generation now matches sing-box (vision + `packet_encoding: xudp` when applicable).

- **SOCKS5 in connections:** Parser now supports `socks5://` and `socks://` direct links in Source and Connections (e.g. `socks5://user:pass@proxy.example.com:1080#Office SOCKS5`). Resulting nodes become sing-box outbounds of type `socks` and participate in selectors like other protocols.

- **Linux build:** `build_linux.sh` now checks for required system packages (OpenGL/X11) and prints install commands for Debian/Ubuntu and Fedora. README and new `docs/BUILD_LINUX.md` document dependencies; optional `build/Dockerfile.linux` allows building without installing dev packages locally (see [Issue #40](https://github.com/Leadaxe/singbox-launcher/issues/40)).

- **macOS build script:** `build_darwin.sh` supports `-i` (if the app already exists in `/Applications`, only the executable is updated so `Contents/MacOS/bin/` and logs are kept; otherwise full `.app` copy; then removes the built `.app` from the project directory), `arm64` for a fast Apple Silicon–only build, and `-h` / `--help` (parsed before `go mod tidy`). README documents the options.

- **Wizard template — DNS:** The default `bin/wizard_template.json` DNS section was reworked: `local` resolver, separate UDP servers (e.g. Cloudflare 1.1.1.1 and a Google UDP bootstrap for DoH), Google DoH endpoints use host `dns.google` with `domain_resolver`, and `dns.final` targets the system local resolver. Legacy `bin/config_template.json` and `bin/config_template_macos.json` were removed from the repo. **Recommendation:** delete or reset your saved wizard/parser template in the app data directory so the next run picks up the bundled template and new DNS defaults (otherwise an old copy keeps the previous DNS block).

### Technical / Internal

- **Clash API:** `GET /proxies/{name}/delay` and `PUT /proxies/{group}` now **percent-encode** proxy/group names (spaces, `>`, Unicode, etc.); delay `url` query uses `QueryEscape`. Switch payload uses `json.Marshal` for `name`. Fixes 404 «Resource not found» when pinging tags like `abvpn:… > …`.

- **UI:** `ShowDownloadFailedManual` and `ShowAutoHideInfo` are no longer re-exported from `ui/dialogs.go`; call sites in package `ui` use `internal/dialogs` directly (same behavior).

- **Docs:** `docs/ParserConfig.md` — VLESS/Trojan URI: expanded query parameters and link to `SPECS/023-…/SUBSCRIPTION_PARAMS_REPORT.md` (sing-box field reference); wizard auto `tag_prefix` from subscription URL `#fragment`.

- **Wizard template — `dns_options`:** The loader keeps raw `dns_options` on `TemplateData`. On first DNS init, the wizard reads `servers` (strips wizard-only `description` / `enabled`), `rules`, `dns.final` or `final`, `strategy`, `independent_cache`, and prepends a `local` server from `config.dns` when missing. `DefaultDomainResolver` is taken from `default_domain_resolver` or `route.default_domain_resolver` inside `dns_options`.

- **Wizard — DNS merge (single entrypoint):** `ApplyWizardDNSTemplate` rebuilds `DNSServers` from **`config.dns.servers`** (locked rows), then **`dns_options.servers`**, then orphan saved tags; fills empty rules (or UI placeholder), final, strategy, cache, and default resolver from `dns_options` / `config.dns`; prepends missing `type: local` from `config.dns` when needed. **`LoadPersistedWizardDNS`** + `ApplyWizardDNSTemplate` replace the old Init/Enrich split; opening without `state.json` triggers one `Apply` from `initializeWizardContent` when the server list is still empty. **`bin/locale/ru.json`:** `wizard.dns.*` and `wizard.tab_dns`.

- **Wizard — DNS / Save race:** `SyncModelToGUI` always queues model→widgets via **`fyne.Do`**. `SyncGUIToModel` skips overwriting **DNS rules**, **Final**, and **default domain resolver** when widgets are still out of sync with the model (empty selection, or **Select.Options** missing the model tag before the next refresh). Tab close / tab switch / preview parse use **`MergeGUIToModel`** (same merge, no **`hasChanges`**). Contract for **`hasChanges`** and widget order (`WizardWidgetsReady` / **`MarkAsSaved`** after **Final** outbound) is documented in **`presenter_sync.go`**. `ApplyWizardDNSTemplate` fills **`default_domain_resolver`** from raw **`dns_options`** when the model field is still empty. **Performance:** toggling a server’s **enabled** checkbox calls **`RefreshDNSDependentSelectsOnly`** (updates DNS selects only), not full **`SyncModelToGUI`** — the latter rebuilt the whole server list every click and made the tab unusable. **Layout:** **Final** + **default resolver** row uses nested **HBox** groups so Fyne gives each **Select** a non-zero width. **Tooltips:** server summary uses **`ttwidget.Label`** (standard `Label` ignores `SetToolTip`). If `dns_options.rules` JSON fails to parse, rules text falls back to **`config.dns.rules`**.

- **Wizard — DNS server row:** Non-empty **`detour`** in server JSON is shown at the end of the **list line** as **`[detour]`** (after `tag · type · server`). Row **tooltip** shows **`description`** only (no duplicate **`detour`**).

- **internal/fynewidget:** **`NewCheckWithContent`** — empty **`Check`** plus arbitrary **`CanvasObject`**: primary tap on the content toggles the check; hover on the content mirrors the check hover ( **`ttwidget.Label`** uses its **`OnMouseIn`/`Out`** hooks). Optional **`ContentToolTip`** when the child implements **`SetToolTip`**. **DNS** server rows and **Rules** tab (selectable + custom rule rows, macOS **TUN**) use it; rule **`description`** / TUN help moved from **`?`** dialogs to **tooltips** on the label.

- **Wizard state — DNS resolver:** **`route.default_domain_resolver`** is no longer duplicated in **`config_params`**; **`dns_options`** is the only persistence path. Legacy **`config_params`** entries are applied once in **`restoreDNS`** if the model resolver is still empty after **`dns_options`**. See **docs/WIZARD_STATE.md**.

- **Wizard state — DNS rules:** In **`dns_options`**, rules are persisted as a JSON **`rules`** array (same shape as sing-box `dns.rules`). The **`rules_text`** key is not used; invalid editor lines omit **`rules`** on save. Comments `#` and blank lines are not preserved when round-tripping through **`rules`**.

(пункты для следующего релиза)

---

## RU

### Внутреннее / Рефакторинг

- **Презентер визарда:** `SyncGUIToModelWithoutDirtyFlag` переименован в **`MergeGUIToModel`** (яснее: слить виджеты в модель без **`hasChanges`**). Единый блок комментариев-контракта в **`ui/wizard/presentation/presenter_sync.go`**.

- **`internal/fynewidget`:** **`NewCheckWithContent`** — пустой **`Check`** + произвольный контент: тап по контенту переключает галку, hover дублируется на галку; опциональный тултип на контенте. Строки **DNS**, вкладка **Rules** (шаблонные и пользовательские правила, **TUN** на macOS) на этом helper; тексты из диалогов по **`?`** перенесены в **тултипы** подписи.

### Основное

- **Визард и главное окно — UI/UX (gutter, Rules, Sources):** **Отступ справа под скролл**, где полоса прокрутки наезжала на текст или кнопки: **Rules** (список пользовательских правил), **Sources** (поле URL, список источников, превью серверов, общий скролл вкладки), **DNS** (только список серверов; не поле JSON правил), вкладка **Servers** — **внутри каждой строки** списка прокси после кнопок (не внешний зазор между скроллом и краем окна). **Rules — пользовательские правила:** **↑/↓**, позиция прокрутки сохраняется, удаление с **подтверждением**; обновление outbound-селектов больше **не сбрасывает несохранённые правки**. **Sources:** компактная строка **подпись + копирование** вместо широкой кнопки.

- **Визард — Outbounds, несохранённое:** список outbounds (**↑/↓**, правка, добавление, удаление) обновляет JSON под флагом **`ParserConfigUpdating`**, поэтому **`MarkAsChanged`** вызывается явно после применения — диалог «сохранить при закрытии» снова появляется после смены порядка и прочих действий в списке.

- **Визард — вкладка DNS:** `dns.servers`, **`dns.rules` одним JSON-объектом** `{"rules":[...]}` (старый формат «объект в строку» по-прежнему читается), `dns.final`, `strategy`, `independent_cache`, `route.default_domain_resolver`. В `state.json` — **`dns_options`** (см. **docs/WIZARD_STATE.md**). Включение серверов (**enabled**), скелетные строки из шаблона, привязка Final / резолвера / тегов в правилах к **включённым** серверам — по логике вкладки (подробности в **docs/WIZARD_STATE.md**).

- **Визард — фрагмент URL подписки:** если в ссылке на подписку есть `#фрагмент` (например `#abvpn`), при Apply/Append в `tag_prefix` подставляется этот фрагмент (очищенный, с завершающим `:` как у числовых префиксов), если для этого источника ещё не сохранён свой `tag_prefix`.

- **Визард — UTF-8 в подписях:** обрезка длинных подписей источников/строк — по **рунам** (сейчас до **60** символов до `...`), а не по байтам, чтобы не ломать UTF-8 (кириллица, флаги, символы вроде `»` и `❯`). Фрагмент `vless://…#…` декодируется через `PathUnescape`, чтобы `+` в имени не превращался в пробел. **Превью / список серверов:** строки подписки и `sanitizeForDisplay` больше не гоняют по рунам битый UTF-8 (из‑за этого в тег попадал U+FFFD); перед разбором и перед выводом в Fyne применяется `ToValidUTF8`; строки в списке конфигуратора outbounds — та же обрезка по рунам. **Теги с `❯` (U+276F), как у abvpn:** при **чтении** подписки `internal/textnorm.NormalizeProxyDisplay` заменяет `❯`/`»`/`›` на ASCII ` > ` в подписях и тегах (итоговый `config.json` совпадает с тем, что видно в UI). **Вкладка «Серверы» (Clash API):** в `ProxyInfo` сохраняется исходное `Name` для запросов к API; при загрузке списка заполняется `DisplayName` той же нормализацией — список, меню трея и статусные строки показывают его.

- **Ссылки VLESS / Trojan из подписок:** парсер и `GenerateNodeJSON` собирают [V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) sing-box из query: для **WS** в заголовок `Host` подставляется **`host` из query**, а если его нет — **`sni`** (как у abvpn: только `type=ws&sni=…`). `http` (поле `host` — список строк), `grpc` (`service_name`), `xhttp` → `httpupgrade`. VLESS: `security=none` без TLS; обычный TLS и Reality (`pbk`) — по [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound). **REALITY по TCP** без `flow` в URI получает **`flow: xtls-rprx-vision`** (не для `ws`/`grpc`/`http`/`xhttp`). Trojan + WS: `transport` и `tls`. VMess WS: тот же fallback `host`/`sni` для `Host`. VMess gRPC: `service_name` из `path` в JSON. Превью в визарде: `MakeTagUnique` как в основном парсере. Ключи query без учёта регистра; `alpn` с многослойным кодированием нормализуется; `fp=QQ` → utls `qq`; `tcp`/`raw` + `headerType=http` → транспорт `http`; `packetEncoding` → `packet_encoding` в outbound.

- **VLESS `xtls-rprx-vision-udp443`:** В подписках часто приходит flow из Xray; sing-box понимает только `xtls-rprx-vision`. Парсер уже переводил значение во внутренней структуре, но в итоговом `config.json` попадал исходный flow без `packet_encoding`. Генерация конфига исправлена (vision + при необходимости `packet_encoding: xudp`).

- **SOCKS5 в connections:** В Source и Connections можно добавлять прямые ссылки `socks5://` и `socks://` (например `socks5://user:pass@proxy.example.com:1080#Office SOCKS5`). Узлы превращаются в outbound типа `socks` и участвуют в селекторах наравне с остальными протоколами.

- **Сборка на Linux:** скрипт `build_linux.sh` проверяет наличие системных пакетов (OpenGL/X11) и выводит команды установки для Debian/Ubuntu и Fedora. В README и в новом `docs/BUILD_LINUX.md` описаны зависимости; добавлен опциональный `build/Dockerfile.linux` для сборки без установки dev-пакетов (см. [Issue #40](https://github.com/Leadaxe/singbox-launcher/issues/40)).

- **Сборка macOS:** в `build_darwin.sh` флаг `-i` при уже установленном приложении обновляет только исполняемый файл (сохраняются `Contents/MacOS/bin/` и логи), при первой установке копируется весь `.app`, после успеха удаляется собранный `.app` из каталога проекта; режим `arm64`; `-h` / `--help` до `go mod tidy`. В README описаны опции.

- **Шаблон визарда — DNS:** в дефолтном `bin/wizard_template.json` сильно переработана секция DNS: локальный резолвер, отдельные UDP-серверы (в т.ч. Cloudflare 1.1.1.1 и UDP-bootstrap под Google DoH), для Google DoH указан хост `dns.google` с `domain_resolver`, `dns.final` ведёт на системный локальный DNS. Из репозитория убраны устаревшие `bin/config_template.json` и `bin/config_template_macos.json`. **Рекомендация:** удалить или сбросить сохранённый шаблон визарда/парсера в каталоге данных приложения, чтобы при следующем запуске подтянулся встроенный шаблон и новые настройки DNS (иначе останется старая копия с прежней DNS-секцией).

### Техническое / Внутреннее

- **Clash API:** для `GET /proxies/{name}/delay` и `PUT /proxies/{group}` имена прокси/группы **кодируются** (`PathEscape`), параметр `url` в delay — `QueryEscape`; тело переключения — `json.Marshal` для поля `name`. Устраняет 404 при пинге тегов с пробелами и `>` (например abvpn после нормализации).

- **UI:** `ShowDownloadFailedManual` и `ShowAutoHideInfo` больше не реэкспортируются из `ui/dialogs.go`; вызовы в пакете `ui` идут в `internal/dialogs` напрямую (поведение то же).

- **Документация:** `docs/ParserConfig.md` — VLESS/Trojan URI: расширен список query-параметров и ссылка на `SPECS/023-…/SUBSCRIPTION_PARAMS_REPORT.md` (справочник полей sing-box); описан автоматический `tag_prefix` из `#` во вводе визарда.

- **Шаблон визарда — `dns_options`:** загрузчик сохраняет сырой `dns_options` в `TemplateData`. При первичной инициализации DNS визард читает `servers` (убирает только визардные `description` / `enabled`), `rules`, `dns.final` или `final`, `strategy`, `independent_cache` и при необходимости добавляет `local` из `config.dns`. `DefaultDomainResolver` — из `default_domain_resolver` или `route.default_domain_resolver` внутри `dns_options`.

- **Визард — слияние DNS (одна точка входа):** `ApplyWizardDNSTemplate` пересобирает список серверов из **`config.dns.servers`**, затем **`dns_options.servers`**, затем осиротевшие теги; подставляет пустые правила (или плейсхолдер), final, strategy, кэш и резолвер; при необходимости добавляет **`local`** из `config.dns`. Восстановление из state: **`LoadPersistedWizardDNS`** + `ApplyWizardDNSTemplate` (вместо пары Init/Enrich). Без `state.json` один вызов `Apply` из `initializeWizardContent`, если список серверов пуст. **`bin/locale/ru.json`:** ключи `wizard.dns.*`, `wizard.tab_dns`.

- **Визард — гонка DNS и Save:** `SyncModelToGUI` через **`fyne.Do`**; защиты в **`SyncGUIToModel`** для правил / Final / резолвера; закрытие / смена вкладки / parse превью — **`MergeGUIToModel`** (то же слияние без **`hasChanges`**). Контракт **`hasChanges`** и порядок инициализации виджетов — в комментариях **`presenter_sync.go`**. **`default_domain_resolver`** из **`dns_options`**. **Производительность:** галочка **включён** вызывает только **`RefreshDNSDependentSelectsOnly`**, а не полный **`SyncModelToGUI`** (иначе список серверов пересобирался на каждый клик). **Вёрстка:** строка Final + резолвер — вложенные **HBox**, чтобы селекты не схлопывались в ноль. **Тултипы:** подпись сервера — **`ttwidget.Label`**. Ошибка разбора `dns_options.rules` → **`config.dns.rules`**.

- **Визард DNS — строка сервера:** непустой **`detour`** в JSON — в **тексте строки** в конце как **`[detour]`** (после `tag · type · server`). **Тултип** — только **`description`** (без дубля **`detour`**).

- **State / DNS:** **`route.default_domain_resolver`** больше **не** пишется в **`config_params`**; единственный источник — **`dns_options`**. Старый дубль в **`config_params`** подхватывается один раз в **`restoreDNS`**, если после **`dns_options`** резолвер в модели пуст. См. **docs/WIZARD_STATE.md**.

- **State / DNS — правила:** в **`dns_options`** только массив **`rules`** (как в sing-box `dns.rules`); ключ **`rules_text`** не читается и не пишется. Невалидный текст редактора → при сохранении без **`rules`**. Комментарии `#` и пустые строки через **`rules`** не восстанавливаются.

(пункты для следующего релиза)
