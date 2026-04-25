# Release Notes

Полный черновик следующего релиза: [docs/release_notes/upcoming.md](docs/release_notes/upcoming.md)

**Черновик (следующий релиз), кратко:** пункты накапливайте в [upcoming.md](docs/release_notes/upcoming.md).

**Draft (next release), short:** add items in [upcoming.md](docs/release_notes/upcoming.md).

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
