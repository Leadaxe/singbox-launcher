# Release Notes

Полный черновик следующего релиза: [docs/release_notes/upcoming.md](docs/release_notes/upcoming.md)

**Черновик (следующий релиз), кратко:** пункты накапливайте в [upcoming.md](docs/release_notes/upcoming.md).

**Draft (next release), short:** add items in [upcoming.md](docs/release_notes/upcoming.md).

---

### Выжимка (RU) — v0.9.6

Две архитектурные поставки одновременно. **SPEC 057-R-N** закрывает историю preset-bundles — outbound preset binding теперь живёт в самом state через поля `ref` + `updates[]` на `OutboundConfig`. Bundled outbound'ы вроде `ru VPN 🇷🇺` персистятся в `state.json`, reorder через ↑/↓ сохраняется, disable preset'а чисто откатывает patches без residue в base body. Параллельно убран давно задеприкейтенный DNS-флаг **`independent_cache`** (sing-box 1.14 перевёл DNS-кэш на per-transport keying). Плюс UI-фиксы: «Restore missing» кнопка для template outbound'ов, 4-я точка фикса каскада disabled подписки, library dialog "already added" преsets теперь визуально как required DNS.

**Полный список изменений:** [docs/release_notes/0-9-6.md](docs/release_notes/0-9-6.md).

### Highlights (EN) — v0.9.6

Two architectural deliveries in one release. **SPEC 057-R-N** closes the preset-bundles story by moving outbound preset binding into state itself via `ref` + `updates[]` fields on `OutboundConfig`. Bundled outbounds like `ru VPN 🇷🇺` persist in `state.json`, reorder via ↑/↓ is durable, and disabling a preset cleanly tears down its patches without residue in the base body. Alongside, the long-deprecated DNS **`independent_cache`** option is removed (sing-box 1.14 moved DNS cache to per-transport keying). Plus UI fixes: "Restore missing" button for template outbounds, 4th fix point for disabled-subscription cascade, library dialog "already added" presets now visually mirror required DNS rows.

**Full changelog:** [docs/release_notes/0-9-6.md](docs/release_notes/0-9-6.md).

---

### Выжимка (RU) — v0.9.5

Точечный фикс — продолжение чистки последствий двойного хранилища из SPEC 045/052. Configurator → Outbounds: добавление нового outbound со Scope ≠ "For All" или edit с переключением Scope теперь сохраняются при Save (раньше запись пропадала). Корень тот же что у v0.9.4 `route.final` — Outbounds tab был единственным surface, мутирующим legacy ParserConfig напрямую без обратной sync в canonical `Sources[i].Outbounds`. Аудит остальных tabs показал что больше дыр того же класса нет. Миграция не нужна — баг был строго на write-side, существующие state.json грузятся корректно. Спасибо Michael M за очередной точный репорт.

**Полный список изменений:** [docs/release_notes/0-9-5.md](docs/release_notes/0-9-5.md).

### Highlights (EN) — v0.9.5

Single-fix point release — continuing cleanup of dual-storage residue from SPEC 045/052. Configurator → Outbounds: adding a new outbound with Scope ≠ "For All", or editing existing with a Scope switch, now persists on Save (previously vanished from list). Root cause same family as v0.9.4 `route.final` — Outbounds tab was the only surface mutating legacy ParserConfig directly without back-sync to canonical `Sources[i].Outbounds`. Audit of other tabs confirms no other dual-book patterns remain. No migration needed — the bug was strictly write-side, existing state.json files load fine. Thanks Michael M for another precise report.

**Full changelog:** [docs/release_notes/0-9-5.md](docs/release_notes/0-9-5.md).

---

### Выжимка (RU) — v0.9.4

Два фикса. **`route.final` из Configurator → Rules → Final outbound теперь действительно применяется** — раньше выбор пользователя молча игнорировался и весь не-матченный трафик уходил через template-default `proxy-out` (репорт от Michael M). Корень: presenter писал в `config_params`, build pipeline имел TODO на чтение этого поля, плюс template хардкодил `final` вместо substitution `@route_final`. Унифицировано через template-var канал (как `dns_final`); существующие state.json мигрируются автоматически на первом Save. **Новый dropdown «TLS root certificate store»** в Configurator → Settings (System / Mozilla CA bundle / Chrome Root Store) — однокликовый escape от сломанного Windows cert store ([sing-box `certificate.store`](https://sing-box.sagernet.org/configuration/certificate/)). Default `system` сохраняет текущее поведение.

**Полный список изменений:** [docs/release_notes/0-9-4.md](docs/release_notes/0-9-4.md).

### Highlights (EN) — v0.9.4

Two fixes. **`route.final` from Configurator → Rules → Final outbound is now actually applied** — previously the user's choice was silently ignored and all unmatched traffic went via template default `proxy-out` (reported by Michael M). Root cause: presenter wrote to `config_params`, build pipeline had a TODO for reading it, plus the template hardcoded `final` instead of using `@route_final` substitution. Unified via the template-var channel (matching `dns_final`); existing state.json files migrate silently on first Save. **New «TLS root certificate store» dropdown** in Configurator → Settings (System / Mozilla CA bundle / Chrome Root Store) — one-click escape from broken Windows cert stores ([sing-box `certificate.store`](https://sing-box.sagernet.org/configuration/certificate/)). Default `system` keeps current behavior.

**Full changelog:** [docs/release_notes/0-9-4.md](docs/release_notes/0-9-4.md).

---

### Выжимка (RU) — v0.9.3

Точечный релиз с одним фиксом: **NaïveProxy outbound'ы теперь авторизуются** ([#69](https://github.com/Leadaxe/singbox-launcher/issues/69), [PR #67](https://github.com/Leadaxe/singbox-launcher/pull/67)). На v0.8.8 — v0.9.2 `GenerateNodeJSON` не эмитил `username` / `password` / `quic` / `extra_headers` для naive — sing-box молча получал outbound без credentials и не подключался. Добавлен emit-блок по образцу vless/trojan/hysteria. Спасибо [@hippus](https://github.com/hippus) за отчёт и PR.

**Полный список изменений:** [docs/release_notes/0-9-3.md](docs/release_notes/0-9-3.md).

### Highlights (EN) — v0.9.3

Single-fix point release: **NaïveProxy outbounds now actually authenticate** ([#69](https://github.com/Leadaxe/singbox-launcher/issues/69), [PR #67](https://github.com/Leadaxe/singbox-launcher/pull/67)). On v0.8.8 — v0.9.2, `GenerateNodeJSON` had no naive case-block — sing-box silently received a credential-less outbound and refused to connect. Adds the missing emit branch mirroring vless / trojan / hysteria patterns. Thanks to [@hippus](https://github.com/hippus) for the report and the PR.

**Full changelog:** [docs/release_notes/0-9-3.md](docs/release_notes/0-9-3.md).

---

### Выжимка (RU) — v0.9.2

Hotfix-релиз. SRS rule-sets снова попадают в `config.json` только как `type: local` — восстановлено поведение v0.8.x, потерянное в v0.9.0/v0.9.1 рефакторе. На cold-start v0.9.x sing-box падал с `FATAL: start service: initialize rule-set ... v2ray-http-upgrade: unexpected status: 404` потому что пытался скачивать remote rule-sets через VPN-прокси. Build pipeline теперь эмитит local-only (с проверкой существования файла, error если нет), UI gate блокирует enable rule до успешного download. **Auto-rebuild после Save / Update**: убран toggle `Auto Rebuild on Change` и дублирующая кнопка `Refresh & Rebuild` — обычный Update делает оба прохода. **Content-addressed SRS tag** для user-added rules (`<filename>-<hash8(URL)>`), коллизии невозможны. **Orphan GC** для `bin/rule-sets/` после каждого rebuild с union тегов из всех `wizard_states/*.json`. Существующие сломанные установки лечатся автоматически на следующем Save.

**Полный список изменений:** [docs/release_notes/0-9-2.md](docs/release_notes/0-9-2.md).

### Highlights (EN) — v0.9.2

Hotfix release. SRS rule-sets are once again emitted into `config.json` strictly as `type: local` — restoring v0.8.x behavior that v0.9.0/v0.9.1 architecture refactor accidentally broke. On cold-start v0.9.x, sing-box died with `FATAL: start service: initialize rule-set ... v2ray-http-upgrade: unexpected status: 404` because it tried to fetch remote rule-sets through the VPN proxy. The build pipeline now emits local-only (with file-existence check, error if missing); UI gate blocks rule enable until the SRS file is present. **Auto-rebuild after Save / Update**: the `Auto Rebuild on Change` toggle and the duplicate `Refresh & Rebuild` button are gone — the regular Update does both passes. **Content-addressed SRS tag** for user-added rules (`<filename>-<hash8(URL)>`), collisions impossible. **Orphan GC** for `bin/rule-sets/` after every rebuild, scanning the union of tags across all `wizard_states/*.json`. Existing broken installs heal automatically on the next Save.

**Full changelog:** [docs/release_notes/0-9-2.md](docs/release_notes/0-9-2.md).

---

### Выжимка (RU) — v0.9.1

Hotfix к v0.9.0. Чистая инсталляция (без `state.json`) ловила `FATAL: default outbound not found: proxy-out` от sing-box и неинформативный «Refresh failed: load state: state: file not found» с per-source кнопки Refresh. Корни: (1) `loadConfigFromFile` при загрузке шаблона заполнял только `model.ParserConfigJSON`, но не canonical `model.GlobalOutbounds` → state.json уезжал с пустыми outbounds → config.json без `proxy-out` / `auto-proxy-out` селекторов; (2) `RefreshSingleSubscription` делал `state.Load` до мутации, на cold-start падал. Двойной фикс: канонический `GlobalOutbounds` теперь seed'ится из template, плюс heal-on-empty в `restoreParserConfig` лечит уже сломанный state. Новый `RefreshSourceInPlace(*Source)` работает с in-memory указателем без обращения к state.json (правильное per-source decoupling per SPEC 052). Cold-start `[ERROR]` шум в логе (`LoadClashAPIConfig`, `clash_api_tab`) демонтирован до DEBUG для `os.IsNotExist`. Текст диалога «Configuration Not Found» обновлён во всех 11 локалях под актуальный SPEC 045/052 поток.

**Полный список изменений:** [docs/release_notes/0-9-1.md](docs/release_notes/0-9-1.md).

### Highlights (EN) — v0.9.1

Hotfix for v0.9.0. Clean installs (no prior `state.json`) hit `FATAL: default outbound not found: proxy-out` from sing-box and the unfriendly `Refresh failed: load state: state: file not found` from the per-source Refresh button. Root causes: (1) `loadConfigFromFile` populated only `model.ParserConfigJSON` from the template but not canonical `model.GlobalOutbounds` → state.json saved with empty outbounds → config.json missing `proxy-out` / `auto-proxy-out` selectors; (2) `RefreshSingleSubscription` did `state.Load` before mutating, failed on cold start. Two-part fix: canonical `GlobalOutbounds` is now seeded from the template, plus heal-on-empty in `restoreParserConfig` repairs already-broken state. New `RefreshSourceInPlace(*Source)` works on in-memory pointers without touching `state.json` (the proper per-source decoupling per SPEC 052). Cold-start `[ERROR]` log noise (`LoadClashAPIConfig`, `clash_api_tab`) demoted to DEBUG for `os.IsNotExist`. The "Configuration Not Found" dialog text was refreshed in all 11 locales for the current SPEC 045 / 052 flow.

**Full changelog:** [docs/release_notes/0-9-1.md](docs/release_notes/0-9-1.md).

---

### Выжимка (RU) — v0.9.0

Кратко: **переработан слой подписок** — `state.json` v5 c per-source метаданными (`profile_title`, квота, expire, last_fetched_at, статус, ошибки, превью), сырые тела в `bin/subscriptions/<id>.raw`, `bin/outbounds.cache.json` ушёл, Rebuild парсит `.raw` напрямую без сети, авто-миграция v2-v4. **Авто-обновление переписано на события**: per-source 1ч heartbeat, 15с retry, immediate retry по `VpnStateChanged`/`ProxyActiveChanged`, 5с cooldown на источник, state-level mutex на load→mutate→save. **Pinned-ядро и шаблон** ([SPEC 046](SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md)): `RequiredCoreVersion`, `RequiredTemplateRef` (через `-ldflags`), нет больше «Update to vX.Y.Z» от GitHub-poll'а; на апгрейде шаблон инвалидируется. **Один in-place toast подписок под Exit** вместо попап-диалогов; per-source прогресс «Fetching N/M». **Soft-cap 150 нод** для авто-пинга после connect/wake (порог через `auto_ping_after_connect_max_proxies`). **`GET /debug/snapshot`**, **типизированная event-шина** (`core/events`). **Фикс [issue #68](https://github.com/Leadaxe/singbox-launcher/issues/68)**: VLESS `packetEncoding` теперь allow-list (`xudp`/`packetaddr`), `none` молча дропается — sing-box больше не падает с `panic: unknown value` ([SPEC 049](SPECS/049-Q-O-SINGBOX_PACKET_ENCODING_PANIC/SPEC.md)). **Multi-stage GC** теперь видит все `bin/wizard_states/*.json` — `.raw` соседних стейджей не съедаются. Спеки: [**052** connections redesign](SPECS/052-F-C-CONNECTIONS_REDESIGN/SPEC.md), [**046** pinned core+template](SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md), [**047** typed event bus](SPECS/047-F-N-TYPED_EVENT_BUS/SPEC.md), [**049** packet_encoding panic](SPECS/049-Q-O-SINGBOX_PACKET_ENCODING_PANIC/SPEC.md), [**038 SUB_SPEC_SNAPSHOT**](SPECS/038-F-C-DEBUG_API/SUB_SPEC_SNAPSHOT.md), [**039 §1.3** auto-ping cap](SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/SPEC.md).

**Полный список изменений:** [docs/release_notes/0-9-0.md](docs/release_notes/0-9-0.md).

### Highlights (EN) — v0.9.0

In short: **subscription layer reworked** — `state.json` v5 with per-source metadata (`profile_title`, quota, expire, `last_fetched_at`, status, error count, preview), raw bodies cached at `bin/subscriptions/<id>.raw`, `bin/outbounds.cache.json` retired, Rebuild parses `.raw` directly with no network round-trip, v2-v4 auto-migration. **Auto-update rewritten as event-driven**: per-source 1h heartbeat, 15s retry, immediate retry on `VpnStateChanged`/`ProxyActiveChanged`, 5s per-source cooldown, state-level mutex on load→mutate→save. **Pinned core and template** ([SPEC 046](SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md)): `RequiredCoreVersion`, `RequiredTemplateRef` (via `-ldflags`), no more "Update to vX.Y.Z" GitHub-poll nudges; template invalidated on launcher upgrade. **Single in-place subscription status toast under Exit** replaces popup dialogs; per-source progress "Fetching N/M". **150-node soft cap** for auto-ping after connect/wake (tunable via `auto_ping_after_connect_max_proxies`). **`GET /debug/snapshot`**, **typed event bus** (`core/events`). **[Issue #68](https://github.com/Leadaxe/singbox-launcher/issues/68) fix**: VLESS `packetEncoding` is now allow-listed (`xudp`/`packetaddr`), `none` silently dropped — sing-box no longer panics with `unknown value` ([SPEC 049](SPECS/049-Q-O-SINGBOX_PACKET_ENCODING_PANIC/SPEC.md)). **Multi-stage GC** now scans all `bin/wizard_states/*.json` so neighbouring stages' `.raw` files survive. Specs: [**052** connections redesign](SPECS/052-F-C-CONNECTIONS_REDESIGN/SPEC.md), [**046** pinned core+template](SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md), [**047** typed event bus](SPECS/047-F-N-TYPED_EVENT_BUS/SPEC.md), [**049** packet_encoding panic](SPECS/049-Q-O-SINGBOX_PACKET_ENCODING_PANIC/SPEC.md), [**038 SUB_SPEC_SNAPSHOT**](SPECS/038-F-C-DEBUG_API/SUB_SPEC_SNAPSHOT.md), [**039 §1.3** auto-ping cap](SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/SPEC.md).

**Full changelog:** [docs/release_notes/0-9-0.md](docs/release_notes/0-9-0.md).

---

### Выжимка (RU) — v0.8.8

Кратко: **поддержка NaïveProxy** в парсере подписок и share-URI энкодере (`naive+https://` / `naive+quic://`, требует sing-box ≥ 1.13.0 со сборкой `with_naive_proxy`); **macOS wake-from-sleep re-sync** через IOKit `IORegisterForSystemPower` — закрытие SPEC 011 на всех платформах. **Честный тост Update**: при failures показывает `2/3 source(s) succeeded (1 failed)` вместо общего «successfully»; «молчаливый ноль» считается failure. **Tooltips на Update / Restart** показывают горячие клавиши (Cmd/Ctrl + U/R). **Фикс шаблона визарда**: object-форма `options: [{title, value}]` теперь принудительно даёт `type: "enum"` (строгий дропдаун) — лечит баг URLTest, когда подпись «5m (default)» уезжала в config вместо значения `5m`. URLTest-vars в `bin/wizard_template.json` приведены в порядок: `urltest_url` plain-string options + combo, `urltest_interval/tolerance` явно `enum`. **CI**: per-version release-notes файл обязателен, новый runbook `docs/RELEASE_PROCESS.md`. `FallbackVersion` 1.13.6 → 1.13.11. Заведены спеки на v0.9.x: [**045** state/config decoupling](SPECS/045-F-N-STATE_CONFIG_DECOUPLING/SPEC.md) (по образцу LxBox), [**046** pinned core + template](SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).

**Полный список изменений:** [docs/release_notes/0-8-8.md](docs/release_notes/0-8-8.md).

### Highlights (EN) — v0.8.8

In short: **NaïveProxy support** in the subscription parser and share-URI encoder (`naive+https://` / `naive+quic://`, requires sing-box ≥ 1.13.0 with `with_naive_proxy`); **macOS wake-from-sleep re-sync** via IOKit `IORegisterForSystemPower` — closes SPEC 011 on all platforms. **Honest Update toast**: on failures shows `2/3 source(s) succeeded (1 failed)` instead of a blanket "successfully"; silent-empty is failure. **Tooltips on Update / Restart** reveal keyboard shortcuts (Cmd/Ctrl + U/R). **Wizard template fix**: object-form `options: [{title, value}]` is now force-normalized to `type: "enum"` (strict dropdown) — fixes the URLTest bug where the label "5m (default)" leaked into the config instead of the value `5m`. URLTest vars in `bin/wizard_template.json` cleaned up: `urltest_url` plain-string options + combo; `urltest_interval / tolerance` explicit `enum`. **CI**: per-version release-notes file required, new `docs/RELEASE_PROCESS.md` runbook. `FallbackVersion` 1.13.6 → 1.13.11. SPEC groundwork for v0.9.x: [**045** state/config decoupling](SPECS/045-F-N-STATE_CONFIG_DECOUPLING/SPEC.md) (LxBox-style), [**046** pinned core + template](SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).

**Full changelog:** [docs/release_notes/0-8-8.md](docs/release_notes/0-8-8.md).

---

### Выжимка (RU) — v0.8.7

Кратко: новая вкладка **⚙️ Settings** (язык, автообновление подписок, автопинг после подключения); data-loss баг языка починен (load-mutate-save). Локальный **Debug API** `127.0.0.1:9269` (off by default, Bearer-токен, read/action-эндпоинты) — backing-service для MCP-обёрток, чтобы AI-агенты могли читать и дёргать лаунчер. В визарде — **переключатель ON/OFF для каждой подписки** (не удаляя URL). **URLTest-параметры** (url/interval/tolerance) выведены из `auto-proxy-out` в шаблонные `vars` с preset-дропдаунами; `vars[].options` теперь поддерживает форму `{title, value}`. Горячие клавиши `Cmd/Ctrl+R / U / P`. **Wake-from-sleep re-sync** (Windows + Linux: `systemd-logind PrepareForSleep`, macOS пока stub) — после резюма сбрасываются соединения Clash API, список прокси обновляется, пинг освежается. **Атомарные записи** `config.json` и `settings.json` (stage+rename). **Лимит 100 МБ** на загрузку sing-box core. **Расшифровка HTTP-кодов** в ошибках подписок (401 → «token may have expired» и т.д.). **Подсказка «(подписки: X ч назад)»** и **dirty-marker `*`** на кнопке Update. **Редакция токена Clash API** в debug-логах. Расширены кириллические TLD в `ru-domains` (`.рус / .москва / .дети / .сайт / .орг / .ком` и др.). Спеки: [**037** — toggle подписки](SPECS/037-F-C-SUBSCRIPTION_SOURCE_TOGGLE/SPEC.md), [**038** — Debug API](SPECS/038-F-C-DEBUG_API/SPEC.md), [**039** — Settings-tab](SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/SPEC.md), [**040** — option titles + URLTest](SPECS/040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/SPEC.md), [**041** — атомарные записи](SPECS/041-F-C-ATOMIC_FILE_WRITES/SPEC.md), [**042** — keyboard shortcuts](SPECS/042-F-C-KEYBOARD_SHORTCUTS/SPEC.md), [**043** — dirty-marker](SPECS/043-F-C-DIRTY_CONFIG_MARKER/SPEC.md); расширена [**011** — wake-from-sleep](SPECS/011-B-C-launcher-freeze-after-sleep/SPEC.md).

**Полный список изменений:** [docs/release_notes/0-8-7.md](docs/release_notes/0-8-7.md).

### Highlights (EN) — v0.8.7

New **⚙️ Settings** tab (language, subscription auto-update, auto-ping on connect); latent data-loss bug in language handler fixed (load-mutate-save). Local **Debug API** on `127.0.0.1:9269` (off by default, Bearer auth, read/action endpoints) — designed as a backing service for MCP wrappers so AI agents can introspect and drive the launcher. Wizard Sources get **per-row on/off toggle** (disabled sources stay in file). **URLTest parameters** (url/interval/tolerance) hoisted from `auto-proxy-out` into template `vars` with preset dropdowns; `vars[].options` now accepts `{title, value}` form. Keyboard shortcuts `Cmd/Ctrl+R / U / P`. **Wake-from-sleep re-sync** (Windows + Linux via `systemd-logind PrepareForSleep`; macOS still stub) — resume resets Clash API transport, refreshes proxies list, re-pings nodes. **Atomic writes** for `config.json` and `settings.json` (stage+rename). **100 MB cap** on sing-box core downloads. **HTTP status humanization** in subscription errors (401 → "token may have expired", etc.). **"(subs: Xh ago)"** freshness hint and **dirty-marker `*`** on the Update button. **Clash API token redaction** in debug logs. Expanded Cyrillic TLDs in `ru-domains` (`.рус / .москва / .дети / .сайт / .орг / .ком` et al.). Specs: [**037**](SPECS/037-F-C-SUBSCRIPTION_SOURCE_TOGGLE/SPEC.md), [**038**](SPECS/038-F-C-DEBUG_API/SPEC.md), [**039**](SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/SPEC.md), [**040**](SPECS/040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/SPEC.md), [**041**](SPECS/041-F-C-ATOMIC_FILE_WRITES/SPEC.md), [**042**](SPECS/042-F-C-KEYBOARD_SHORTCUTS/SPEC.md), [**043**](SPECS/043-F-C-DIRTY_CONFIG_MARKER/SPEC.md); extended [**011**](SPECS/011-B-C-launcher-freeze-after-sleep/SPEC.md).

**Full changelog:** [docs/release_notes/0-8-7.md](docs/release_notes/0-8-7.md).

---

### Выжимка (RU) — v0.8.6

Кратко: визард — вкладка **«Настройки»** (`vars`, плейсхолдеры **`@name`**, опциональные разделители); скаляры DNS в **`state.vars`** (**`dns_*`**, плейсхолдеры **`@dns_*`**, в **`dns_options`** — только servers/rules, миграция со старых файлов); **`state.json`** версия **4** при сохранении (чтение 2–4). На **macOS** переключатель **TUN** перенесён с **«Правил»** на **«Настройки»**; запрет снять TUN при работающем ядре/процессе sing-box, честный **Stop**, по запросу удаление кеша в **`bin/`** и логов после остановки. **Win7 x86** — общий с Windows/Linux блок TUN в **`params`**, **`default_value`** по платформам (в т.ч. **`gvisor`** по умолчанию), исправлена загрузка **`wizard_template.json`** без вложенного **`inbounds.stack`**. **Linux** — приоритет **`sing-box`** из **`PATH`**, иначе локальный **`bin/`**. Исходящий HTTP — **`HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`**, единый транспорт, скрытие **`user:password@`** в ошибках, при сбое загрузки **SRS** — предупреждение в лог на вкладке **Rules**. Подписки **Hysteria2** — разбор портов и нормализация **`server_ports`** под sing-box ([#58](https://github.com/Leadaxe/singbox-launcher/issues/58)). Спеки: [**032** — Settings](SPECS/032-F-C-WIZARD_SETTINGS_TAB/), [**034** — HTTP proxy](SPECS/034-F-C-HTTP_ENV_PROXY/), [**019** — Win7](SPECS/019-F-C-WIN7_ADAPTATION/); документация [**035** — VLESS `flow` / sing-box](SPECS/035-Q-C-VLESS_SINGBOX_FLOW_FIELD/SPEC.md).

**Полный список изменений:** [docs/release_notes/0-8-6.md](docs/release_notes/0-8-6.md).

### Highlights (EN) — v0.8.6

Wizard **Settings** tab (`vars`, **`@name`** placeholders, optional row separators); DNS scalars in **`state.vars`** (**`dns_*`**, **`@dns_*`** in template, **`dns_options`** holds servers/rules only, migration on load); wizard **`state.json`** version **4** on save (reads **2–4**). On **macOS**, **TUN** moved from **Rules** to **Settings**; guards (no TUN off while core/sing-box is up), honest **Stop**, optional post-stop cleanup of **`bin/`** cache and **logs**. **Win7 x86** shares the Windows/Linux **`params`** TUN block, platform **`default_value`** (default **`gvisor`** on 32-bit), fixed **`wizard_template.json`** fetch without nested **`inbounds.stack`**. **Linux:** use **`sing-box`** from **`PATH`** if present, else **`bin/`**. Outbound HTTP: **`HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`**, shared transport, credential redaction in errors, **SRS** download failures log a **warning** on **Rules**. **Hysteria2** subscription ports → sing-box **`server_ports`** normalization ([#58](https://github.com/Leadaxe/singbox-launcher/issues/58)). Specs: [**032**](SPECS/032-F-C-WIZARD_SETTINGS_TAB/), [**034**](SPECS/034-F-C-HTTP_ENV_PROXY/), [**019**](SPECS/019-F-C-WIN7_ADAPTATION/); [**035** VLESS `flow` notes](SPECS/035-Q-C-VLESS_SINGBOX_FLOW_FIELD/SPEC.md).

**Full changelog:** [docs/release_notes/0-8-6.md](docs/release_notes/0-8-6.md).

---

### Выжимка (RU) — v0.8.5

Кратко: визард (DNS, Rules v3, Sources, gutter, hover-строки, правка источника, несохранённое), парсер и генерация (лимит узлов 3000, URI/UTF-8/sing-box check), вкладка Servers (ПКМ, share URI, фильтр ошибок пинга, мультивыбор, ScrollToTop), Clash API (кодирование имён), настройки пинга, сборка Linux/macOS, шаблон DNS и sing-box 1.13+.

**Полный список изменений:** [docs/release_notes/0-8-5.md](docs/release_notes/0-8-5.md).

### Draft highlights (EN) — v0.8.5

Wizard (DNS tab, Rules v3, Sources, scroll gutters, row hover, per-source edit, unsaved flow), parser and config generation (3000 nodes cap, URI edge cases, `sing-box check`), Servers tab (context menu, share URI, ping-error filter, multi-select, scroll after switch), Clash API path encoding, launcher ping settings, Linux/macOS build notes, wizard template DNS and sing-box 1.13+ mixed inbound.

**Full changelog:** [docs/release_notes/0-8-5.md](docs/release_notes/0-8-5.md).

*Черновик следующего релиза:* [docs/release_notes/upcoming.md](docs/release_notes/upcoming.md)

---

## Последний релиз / Latest release

| Версия | Описание |
|--------|----------|
| **v0.8.8** | [docs/release_notes/0-8-8.md](docs/release_notes/0-8-8.md) |
| **v0.8.7** | [docs/release_notes/0-8-7.md](docs/release_notes/0-8-7.md) |
| **v0.8.6** | [docs/release_notes/0-8-6.md](docs/release_notes/0-8-6.md) |
| **v0.8.5** | [docs/release_notes/0-8-5.md](docs/release_notes/0-8-5.md) |
| **v0.8.4** | [docs/release_notes/0-8-4.md](docs/release_notes/0-8-4.md) |
| **v0.8.3** | [docs/release_notes/0-8-3.md](docs/release_notes/0-8-3.md) |
| **v0.8.2** | [docs/release_notes/0-8-2.md](docs/release_notes/0-8-2.md) |
| **v0.8.1** | [docs/release_notes/0-8-1.md](docs/release_notes/0-8-1.md) |
| **v0.8.0** | [docs/release_notes/0-8-0.md](docs/release_notes/0-8-0.md) |

Полное описание каждой версии — по ссылке в таблице (full details in linked files).
