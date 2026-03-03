# Release Notes

## Последний релиз / Latest release

**v0.8.1** — полное описание (full details): [docs/release_notes/0-8-1.md](docs/release_notes/0-8-1.md)

**v0.8.0** — полное описание (full details): [docs/release_notes/0-8-0.md](docs/release_notes/0-8-0.md)

<details>
<summary><b>Что не вошло в релиз / Not yet released</b></summary>

Изменения после v0.8.1 (changes since v0.8.1): [upcoming.md](docs/release_notes/upcoming.md)

<details>
<summary><b>🇬🇧 English (upcoming)</b></summary>

### Highlights
- **Unified “download failed” dialog** — When download of sing-box, wintun.dll, config template, or SRS fails, one dialog is shown: short message, “Open download page” link with a copy-URL button, “Open folder”, and “Close”. Same behavior for all resources.
- **SRS local download** — Rule-set files (SRS) from `raw.githubusercontent.com` are now downloaded locally to `bin/rule-sets/`. SRS button (⬇/🔄/✔️) in Rules tab; tooltip on hover shows original URL from template. Removed `go-any-way-githubusercontent` outbound and `download_detour`/`update_interval` from rule_set.
- **Windows:** Start/stop is more stable. Graceful stop via console (AttachConsole + CTRL_BREAK) when the core has a console; fallback to taskkill. taskkill tries without `/F` first, then with `/F` on error (same for Kill by name in Help tab). Fewer crashes and WinTun issues on restart.
- **Logging:** Default log level is now build-based (release = Warn, dev = Verbose). Removed `SINGBOX_DEBUG` env var. API log level follows global level; `logFile` param removed from API methods. Less noise: core version and stability timer are cached.
- **Diagnostics log viewer:** New Logs window from the Diagnostics tab with three tabs: Internal (live app logs via sink), Core (tail of `logs/sing-box.log` with auto-refresh every 5 seconds), and API (live Clash API requests). Supports level filters on Internal/API and shows newest entries at the top.
- **Config:** `getConfigJSON` outputs trailing commas for all config readers. Windows TUN: removed netsh cleanup on stop (interfaces close normally).

</details>

<details>
<summary><b>🇷🇺 Русская версия (upcoming)</b></summary>

### Основное
- **Единый диалог «загрузка не удалась»** — при ошибке загрузки sing-box, wintun.dll, шаблона конфига или SRS показывается один диалог: короткое сообщение, ссылка «Open download page» с кнопкой копирования URL, «Open folder» и «Close». Одинаковое поведение для всех ресурсов.
- **Локальное скачивание SRS** — rule-set файлы с `raw.githubusercontent.com` скачиваются локально в `bin/rule-sets/`. Кнопка SRS (⬇/🔄/✔️) во вкладке Rules; при наведении — tooltip с оригинальным URL из шаблона. Удалены outbound `go-any-way-githubusercontent` и `download_detour`/`update_interval` из rule_set.
- **Windows:** Запуск и остановка работают стабильнее. Мягкая остановка по консоли (AttachConsole + CTRL_BREAK), при необходимости — fallback на taskkill. taskkill сначала без `/F`, при ошибке — с `/F` (так же для Kill по имени во вкладке Help). Меньше крашей и проблем с WinTun при перезапуске.
- **Логирование:** Уровень логов по умолчанию зависит от сборки (release = Warn, dev = Verbose). Убрана переменная `SINGBOX_DEBUG`. Уровень api.log следует глобальному; параметр `logFile` убран из методов API. Меньше шума: кэшируются версия ядра и таймер стабильности.
- **Окно логов Diagnostics:** Новое окно Logs с вкладки Diagnostics: три вкладки — Internal (живые логи лаунчера через sink), Core (хвост `logs/sing-box.log` с автообновлением раз в 5 секунд) и API (живые запросы Clash API). Поддерживаются фильтры по уровню на Internal/API, новые записи отображаются сверху.
- **Конфиг:** `getConfigJSON` выводит trailing commas для всех читателей конфига. Windows TUN: убрана очистка через netsh при остановке (интерфейсы закрываются сами).

</details>

</details>

---

<details>
<summary><b>🇬🇧 English (v0.8.1)</b></summary>

### Highlights
- **Fix** — Config no longer corrupted after "Update" or timer refresh ([#31](https://github.com/Leadaxe/singbox-launcher/issues/31)).
- **macOS TUN** — privileged launch path for TUN on macOS; optional system helper for reliable TUN when running without full admin rights.
- **Refactoring** — `core/config/generator.go` → `outbound_generator.go`, `ui/wizard/business/generator.go` → `create_config.go`; simplified outbounds assembly (dynamic between @ParserSTART/@ParserEND + static, comma only when both present).

</details>

<details>
<summary><b>🇷🇺 Русская версия (v0.8.1)</b></summary>

### Основное
- **Исправление** — конфиг больше не портится после «Обновить» или обновления по таймеру ([#31](https://github.com/Leadaxe/singbox-launcher/issues/31)).
- **TUN на macOS** — привилегированный запуск для TUN на macOS; опциональный системный хелпер для стабильной работы TUN без полных прав администратора.
- **Рефакторинг** — переименование генераторов: `core/config/generator.go` → `outbound_generator.go`, `ui/wizard/business/generator.go` → `create_config.go`; упрощённая сборка outbounds (динамические между @ParserSTART/@ParserEND и статические, запятая только при наличии обоих блоков).

</details>
