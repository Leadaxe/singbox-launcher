# Release Notes

Полный черновик следующего релиза: [docs/release_notes/upcoming.md](docs/release_notes/upcoming.md)

**Черновик (следующий релиз), кратко:** пункты накапливайте в [upcoming.md](docs/release_notes/upcoming.md).

**Draft (next release), short:** add items in [upcoming.md](docs/release_notes/upcoming.md).

---

### Выжимка (RU) — v1.1.1

**Windows 7 переходит на форк-ядро — XHTTP + AmneziaWG 2.0 на всех платформах.** Форк [`sing-box-lx`](https://github.com/Leadaxe/sing-box-lx) начал собирать сборку для **Windows 7 (32-бит, `windows-386-legacy-windows-7`)**, поэтому Win7 больше не откатывается на upstream SagerNet `1.13.12` — лаунчер качает на Win7 то же форк-ядро, что и везде. Win7-пользователи впервые получают **XHTTP** и обфускацию **AmneziaWG 2.0**. Удалена ставшая мёртвой legacy/SourceForge-машинерия (`Win7LegacyVersion`, `SingboxLegacyRepo`, SF-фоллбэк). Ядро запиннено на `v1.13.13-lx.5`. Обновить: Core Dashboard → Download/Reinstall.

**Полный список изменений:** [docs/release_notes/1-1-1.md](docs/release_notes/1-1-1.md).

### Highlights (EN) — v1.1.1

**Windows 7 joins the fork core — XHTTP + AmneziaWG 2.0 on every platform.** The [`sing-box-lx`](https://github.com/Leadaxe/sing-box-lx) fork now publishes a **Windows 7 (32-bit, `windows-386-legacy-windows-7`)** build, so Win7 no longer falls back to upstream SagerNet `1.13.12` — the launcher downloads the same fork core on Win7 as everywhere else. Win7 users get **XHTTP** and **AmneziaWG 2.0** obfuscation for the first time. The now-dead legacy/SourceForge download machinery (`Win7LegacyVersion`, `SingboxLegacyRepo`, the SF fallback) is removed. Core pinned to `v1.13.13-lx.5`. Pull it from Core Dashboard → Download/Reinstall.

**Full changelog:** [docs/release_notes/1-1-1.md](docs/release_notes/1-1-1.md).

---

### Выжимка (RU) — v1.1.0

**Большое обновление: форк-ядро, XHTTP и AmneziaWG.** Лаунчер переключился с upstream-ядра SagerNet на **форк [`sing-box-lx`](https://github.com/Leadaxe/sing-box-lx) (v1.13.13-lx.3)**, который собирает `with_xhttp` + `with_awg` и потому запускает две клиентские фичи, недоступные в upstream. **XHTTP** (чинит тихую регрессию): узлы `type=xhttp` больше не деградируют в `httpupgrade` — честно парсятся, эмитятся в `config.json` и round-trip'ятся в share-URI (VLESS/VMess/Trojan), плюс исправлена путаница `httpupgrade ⇄ xhttp`. **AmneziaWG 2.0**: WireGuard-узлы несут `jc/jmin/jmax`, `s1`–`s4`, `h1`–`h4` и CPS-пакеты `i1`–`i5` из `wireguard://`/`awg://`, эмитятся в `endpoints[]`; **MTU AWG клампится до 1280**, иначе S3/S4-паддинг ломает данные при сохранённом handshake. **Debug API**: кнопка «Перегенерировать токен». **Windows 7 (32-бит)** остаётся на upstream `1.13.12` (без XHTTP/AWG) — не ломается. Обновить ядро: Core Dashboard → Download/Reinstall.

**Полный список изменений:** [docs/release_notes/1-1-0.md](docs/release_notes/1-1-0.md).

### Highlights (EN) — v1.1.0

**Big update: fork core, XHTTP & AmneziaWG.** The launcher switched off the upstream SagerNet core onto the **[`sing-box-lx`](https://github.com/Leadaxe/sing-box-lx) fork (v1.13.13-lx.3)**, which builds `with_xhttp` + `with_awg` and therefore runs two client features upstream can't. **XHTTP** (fixes a silent regression): `type=xhttp` nodes no longer degrade to `httpupgrade` — they're parsed, generated into `config.json`, and round-tripped to share URIs (VLESS/VMess/Trojan); the `httpupgrade ⇄ xhttp` mislabeling is fixed. **AmneziaWG 2.0**: WireGuard nodes carry `jc/jmin/jmax`, `s1`–`s4`, `h1`–`h4` and CPS packets `i1`–`i5` from `wireguard://`/`awg://`, generated into `endpoints[]`; **AWG MTU is clamped to 1280**, or the S3/S4 padding breaks data while the handshake still succeeds. **Debug API**: a "Regenerate token" button. **Windows 7 (32-bit)** stays on upstream `1.13.12` (no XHTTP/AWG) — nothing breaks. Pull the new core via Core Dashboard → Download/Reinstall.

**Full changelog:** [docs/release_notes/1-1-0.md](docs/release_notes/1-1-0.md).

---

### Выжимка (RU) — v1.0.0

**Первый стабильный релиз — выходим из тестирования.** После долгого 0.x-цикла конфигуратор, пайплайн подписок/пресетов/outbound/DNS/правил, Debug API и сборки macOS + Windows (вкл. Win7) считаются production-ready. Содержимое накопилось с v0.9.9. **Template engine получил `#if`** (SPEC 067) — условное включение полей по выражениям с предикатами (`#in`/`#not`/`#notEmpty`/`#matches`…) и namespace `@runtime.*`; это разблокировало multi-rule presets и аутентификацию **proxy-in по логину/паролю**. **Аудит безопасности** (SPEC 068) закрыл 10 верифицированных находок (в т.ч. critical-баг с потерей rules/DNS при headless-Save на legacy state). **Архитектурный рефактор** (SPEC 070): `docs/ARCHITECTURE.md` переписан вокруг 8-слойной модели с ADR + крупная уборка кода (SPEC 069, 304 находки). UX: **подписки и серверы можно переупорядочивать** (↑/↓, сохраняется), **ссылка на поддержку провайдера** в строке источника, **«Fetch now»** для выключенных подписок, **очистка неиспользуемых `.srs`**, **маскированные поля для `type:"secret"`**. Починены DNS-скаляры/template-серверы на Save/reopen; усилена очистка NLA/ghost-TUN на Windows. **Breaking (только кастомные шаблоны):** outer `if`/`if_or` теперь требуют `@`-префикс — bundled-шаблон мигрирован автоматически.

**Полный список изменений:** [docs/release_notes/1-0-0.md](docs/release_notes/1-0-0.md).

### Highlights (EN) — v1.0.0

**First stable release — out of testing.** After an extended 0.x cycle, the configurator, the subscription/preset/outbound/DNS/rules pipeline, the Debug API, and the macOS + Windows (incl. Win7) builds are considered production-ready. Contents accumulated since v0.9.9. **The template engine gained `#if`** (SPEC 067) — conditional field inclusion via an expression language (`#in`/`#not`/`#notEmpty`/`#matches`…) and a `@runtime.*` namespace; this unblocked multi-rule presets and **proxy-in username/password authentication**. A **threat-model audit** (SPEC 068) fixed 10 verified findings (incl. a critical bug that dropped rules/DNS on a headless Save of a legacy state). An **architecture refactor** (SPEC 070): `docs/ARCHITECTURE.md` rewritten around an 8-layer model with ADRs, plus a large code-cleanup pass (SPEC 069, 304 findings). UX: **subscriptions and servers can be reordered** (↑/↓, persisted), a **provider support link** per source row, **"Fetch now"** for disabled subscriptions, **unused `.srs` cleanup**, and **masked fields for `type:"secret"`**. DNS scalars/template-servers now persist correctly on Save/reopen; NLA/ghost-TUN cleanup hardened on Windows. **Breaking (custom templates only):** outer `if`/`if_or` now require an `@`-prefix — the bundled template is migrated automatically.

**Full changelog:** [docs/release_notes/1-0-0.md](docs/release_notes/1-0-0.md).

---

### Выжимка (RU) — v0.9.9

Большой UX-релиз. Settings переехали в отдельное окно, открываемое кликом по табу `⚙️` — таб работает как кнопка; внутри Subscriptions / Language / Идентификация устройства / Debug API в одной полосе скролла. Servers tab подключается к удалённым Clash API (sing-box на роутере, mihomo на VPS, другой инстанс лаунчера) — кнопка ⚙ в шапке таба, badge `🏠 Local` / `🌐 host:port`. Подписки получили кастомное User-Agent поле для провайдеров, режущих наш default. В Outbound Edit диалоге появился picker эмодзи-флагов для построения фильтр-regex'а с live-preview node-list. На Windows 7 phantom TUN адаптеры теперь автоматически чистятся. SPEC 066 убрала фантомный `tun_builtin` toggle. Новый Debug API endpoint `GET/PATCH /settings/user-agent`, документация API доступна в `docs/API.md`.

**Полный список изменений:** [docs/release_notes/0-9-9.md](docs/release_notes/0-9-9.md).

### Highlights (EN) — v0.9.9

Major UX release. Settings moved to a separate window opened from the `⚙️` tab — the tab acts as a button; inside, Subscriptions / Language / Subscription identification / Debug API live on one scrollable column. Servers tab can now connect to remote Clash API endpoints (sing-box on a router, mihomo on a VPS, another launcher instance) — gear button in the tab header, `🏠 Local` / `🌐 host:port` badge. Subscriptions got a custom User-Agent field for providers that reject our default. The Outbound Edit dialog gained an emoji flag picker for building filter regexes with a live preview of which nodes match. Windows 7 phantom TUN adapters are now auto-cleaned. SPEC 066 dropped the phantom `tun_builtin` toggle. New Debug API endpoint `GET/PATCH /settings/user-agent`; full API reference docs available at `docs/API.md`.

**Full changelog:** [docs/release_notes/0-9-9.md](docs/release_notes/0-9-9.md).

---

### Выжимка (RU) — v0.9.8.1

Hotfix поверх v0.9.8 — пользовательские SRS-правила теперь работают end-to-end. До этого релиза user-добавленное SRS-правило либо теряло скачанный `.srs` файл при следующем rebuild (orphan GC мисматчил filename), либо — после file-name фикса в v0.9.8 — при повторном открытии диалога редактирования молча переключалось в "Custom JSON" с пустым body. Оба пути закрыты; других изменений поведения нет.

**Полный список изменений:** [docs/release_notes/0-9-8-1.md](docs/release_notes/0-9-8-1.md).

### Highlights (EN) — v0.9.8.1

Hotfix on top of v0.9.8 — user-defined SRS rules now actually work end-to-end. Before this release a user-added SRS rule would either lose its downloaded `.srs` file at the next rebuild (orphan GC mismatched filename), or — after the file-name fix in v0.9.8 — reopen the rule editor and find the type silently switched to "Custom JSON" with an empty body. Both paths are closed; no other behavior changes.

**Full changelog:** [docs/release_notes/0-9-8-1.md](docs/release_notes/0-9-8-1.md).

---

### Выжимка (RU) — v0.9.8

Plumbing + quality релиз поверх v0.9.7: **sing-box обновлён до 1.13.12** на modern и Win7-legacy сборках, **Win7-релиз теперь корректно показывает версию** (раньше шёл с `v-local-test` из-за бага в CI ldflags), задокументирован **workaround OpenGL через Mesa3D** для старого Win7-железа (Intel HD 1xxx-2xxx), **главное окно теперь работает параллельно с конфигуратором** (был невидимый overlay, который перехватывал клики), **вкладка DNS Rules стала единым ordered списком** с drag ↑↓ (preset 🔗 и user → правила можно перемешивать). За кулисами: **README переписан** вокруг platform-level позиционирования с 6 hero-скриншотами + 3 независимых CI-бага найдены и исправлены (Win7 ldflags инжектил в несуществующий символ, Windows test-фейлы молча проглатывались `set ERRORLEVEL` трапом, Linux/macOS падал на `.gitignore`-проглоченной фикстуре).

**Полный список изменений:** [docs/release_notes/0-9-8.md](docs/release_notes/0-9-8.md).

### Highlights (EN) — v0.9.8

Plumbing + quality release on top of v0.9.7: **sing-box bumped to 1.13.12** on both modern and Win7-legacy builds, **Win7 release now reports the correct version** (was shipping with `v-local-test` due to a CI ldflags bug), **Mesa3D OpenGL workaround documented** for legacy Win7 hardware (Intel HD 1xxx-2xxx), **main window now usable in parallel with the configurator** (an invisible overlay was intercepting clicks), **DNS Rules tab is now a single ordered list** with drag ↑↓ (preset 🔗 and user → rules can be freely interleaved). Behind the scenes: **README rewrite** around platform-level positioning with 6 hero screenshots + 3 independent CI bugs found and fixed (Win7 ldflags injecting into a non-existent symbol, Windows test failures silently swallowed by a `set ERRORLEVEL` trap, Linux/macOS failing on a `.gitignore`-swallowed fixture).

**Full changelog:** [docs/release_notes/0-9-8.md](docs/release_notes/0-9-8.md).

---

### Выжимка (RU) — v0.9.7

Quality-of-life релиз поверх v0.9.6: появился **Профайлер трафика** (новое окно с вкладки Diagnostics — live DNS/TCP/UDP события с привязкой к процессам, per-process recording, verbose toggle). Починили две неприятные регрессии — user-добавленные **DNS-серверы и правила** больше не пропадают после reopen configurator'а, и dropdown «Saved states» на главном экране теперь спрашивает перед перезаписью текущего state.json (раньше тихо стирал). **Правила** стали понятнее: preset-правила из библиотеки префиксованы 🔗, а если у enabled preset'а .srs файл не скачан — появляется ⚠ и launcher тихо скачивает в фоне. Debug API получил ~15 новых endpoint'ов по SPEC 053–059 для скриптинга/автоматизации.

**Полный список изменений:** [docs/release_notes/0-9-7.md](docs/release_notes/0-9-7.md).

### Highlights (EN) — v0.9.7

Quality-of-life release on top of v0.9.6: **Traffic Profiler** ships (new window from the Diagnostics tab — live DNS/TCP/UDP events with process attribution, per-process recording, verbose toggle). Two nasty regressions fixed — user-added **DNS servers and rules** no longer vanish after reopening the configurator, and the **Saved-states** dropdown on the main screen now prompts before overwriting the current state.json (previously silent wipe). **Rules tab** got clearer: preset-library rules are prefixed with 🔗, and if an enabled preset's .srs file isn't downloaded yet a ⚠ badge appears while the launcher silently fetches it in the background. Debug API gains ~15 new endpoints covering SPEC 053–059 for scripting and automation.

**Full changelog:** [docs/release_notes/0-9-7.md](docs/release_notes/0-9-7.md).

---

### Выжимка (RU) — v0.9.6

Пресеты теперь делают всё сразу: один клик — и preset настраивает DNS, добавляет правила маршрутизации, добавляет нужные outbound'ы и патчит существующие. Выключаешь preset — всё чисто исчезает. Правила в твоей конфигурации больше не копии, а ссылки на template — обновления template приходят к тебе автоматически. Outbounds стали гибче: удалять можно любой кроме одного защищённого (`proxy-out` 🔒), а случайно удалённые template-outbound'ы возвращаются одним кликом по кнопке «Restore missing». DNS почищен — убран мёртвый чекбокс `independent_cache` (sing-box 1.14 deprecation) и лишние дефолтные DNS-серверы.

**Полный список изменений:** [docs/release_notes/0-9-6.md](docs/release_notes/0-9-6.md).

### Highlights (EN) — v0.9.6

Presets now do everything at once: one click and a preset sets up DNS, adds routing rules, adds the outbounds it needs, and patches existing ones. Disable the preset and all of it cleanly disappears. Rules in your config are no longer copies but references to the template — template updates flow through automatically. Outbounds got more flexible: you can delete any outbound except one protected one (`proxy-out` 🔒), and accidentally deleted template outbounds come back in one click via the new «Restore missing» button. DNS is cleaner — removed the dead `independent_cache` checkbox (sing-box 1.14 deprecation) and stale default DNS servers.

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
| **v1.0.0** | [docs/release_notes/1-0-0.md](docs/release_notes/1-0-0.md) |
| **v0.9.9** | [docs/release_notes/0-9-9.md](docs/release_notes/0-9-9.md) |
| **v0.9.8.1** | [docs/release_notes/0-9-8-1.md](docs/release_notes/0-9-8-1.md) |
| **v0.9.8** | [docs/release_notes/0-9-8.md](docs/release_notes/0-9-8.md) |
| **v0.9.7** | [docs/release_notes/0-9-7.md](docs/release_notes/0-9-7.md) |
| **v0.9.6** | [docs/release_notes/0-9-6.md](docs/release_notes/0-9-6.md) |
| **v0.9.5** | [docs/release_notes/0-9-5.md](docs/release_notes/0-9-5.md) |
| **v0.9.4** | [docs/release_notes/0-9-4.md](docs/release_notes/0-9-4.md) |
| **v0.9.3** | [docs/release_notes/0-9-3.md](docs/release_notes/0-9-3.md) |
| **v0.9.2** | [docs/release_notes/0-9-2.md](docs/release_notes/0-9-2.md) |
| **v0.9.1** | [docs/release_notes/0-9-1.md](docs/release_notes/0-9-1.md) |
| **v0.9.0** | [docs/release_notes/0-9-0.md](docs/release_notes/0-9-0.md) |
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
