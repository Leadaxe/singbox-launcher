# Release Notes

Полный черновик следующего релиза: [docs/release_notes/upcoming.md](docs/release_notes/upcoming.md)

**Черновик (следующий релиз), кратко:** пункты накапливайте в [upcoming.md](docs/release_notes/upcoming.md).

**Draft (next release), short:** add items in [upcoming.md](docs/release_notes/upcoming.md).

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
| **v0.8.6** | [docs/release_notes/0-8-6.md](docs/release_notes/0-8-6.md) |
| **v0.8.5** | [docs/release_notes/0-8-5.md](docs/release_notes/0-8-5.md) |
| **v0.8.4** | [docs/release_notes/0-8-4.md](docs/release_notes/0-8-4.md) |
| **v0.8.3** | [docs/release_notes/0-8-3.md](docs/release_notes/0-8-3.md) |
| **v0.8.2** | [docs/release_notes/0-8-2.md](docs/release_notes/0-8-2.md) |
| **v0.8.1** | [docs/release_notes/0-8-1.md](docs/release_notes/0-8-1.md) |
| **v0.8.0** | [docs/release_notes/0-8-0.md](docs/release_notes/0-8-0.md) |

Полное описание каждой версии — по ссылке в таблице (full details in linked files).
