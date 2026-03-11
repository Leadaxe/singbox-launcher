# Release Notes

## Последний релиз / Latest release

**v0.8.3** — полное описание (full details): [docs/release_notes/0-8-3.md](docs/release_notes/0-8-3.md)

<details>
<summary><b>🇷🇺 Кратко (v0.8.3)</b></summary>

- Парсер wireguard://; секция endpoints в конфиге (sing-box 1.11+)
- Шаблон визарда почищен, TUN на macOS возвращён
- Исправлено падение Clash API на названиях с `#` и `//` (jsonc.ToJSON)
- Визард: кеш превью, View по источнику из кеша, Get free VPN — подтверждение, диалог Outbound (Raw→Settings), конфигуратор синхронно, смена Scope при редактировании
- Визард: модель как источник истины (Model(), единый префикс 1:/2:/3:, унифицированная сборка прокси)
- Диагностика: настройки STUN (свой сервер, на Mac — SOCKS5/напрямую), ссылка на список STUN-серверов
- Diagnostics: две кнопки (окно логов, папка логов) с иконками; повторное нажатие — фокус на открытое окно; в Help убрана кнопка «Open Logs Folder»
- Исправлено исчезновение источников после reopen визарда, смены префиксов и сохранения
- Логи: Internal до Trace при открытом окне; wintun — загрузка с GitHub, затем wintun.net, таймаут 1 мин без данных
- Core: подсказки «?» у Sing-box и Wintun (Open bin folder, ссылка), краткий статус «❌ not found», Fallback 1.13.1
- Автообновление конфига: 2 попытки, сброс счётчика при Start

</details>

<details>
<summary><b>🇬🇧 Summary (v0.8.3)</b></summary>

- Parser wireguard://; config endpoints section (sing-box 1.11+)
- Wizard template cleaned up; TUN on macOS restored
- Fixed Clash API failing on names with `#` and `//` (jsonc.ToJSON)
- Wizard: preview cache, View uses cache, Get free VPN confirmation, Outbound dialog (Raw→Settings), configurator sync, Scope when editing
- Wizard: model as source of truth (Model(), unified prefix 1:/2:/3:, proxy list building)
- Diagnostics: STUN settings (custom server, on Mac SOCKS5/direct), link to STUN server list
- Diagnostics: two buttons (log window, logs folder) with icons; reopening focuses existing window; Help «Open Logs Folder» removed
- Fixed sources disappearing after reopen, changing prefixes, and save
- Logs: Internal up to Trace when window open; wintun download from GitHub then wintun.net, 1 min idle timeout
- Core: help «?» for Sing-box and Wintun (Open bin folder, link), short status «❌ not found», Fallback 1.13.1
- Auto-update: 2 attempts, counter reset on Start

</details>

**v0.8.2** — полное описание (full details): [docs/release_notes/0-8-2.md](docs/release_notes/0-8-2.md)

<details>
<summary><b>🇷🇺 Кратко (v0.8.2)</b></summary>

- Единый диалог «загрузка не удалась» для sing-box, wintun, шаблона, SRS
- Локальное скачивание SRS в `bin/rule-sets/`; кнопка SRS во вкладке Rules
- Windows: стабильнее start/stop (AttachConsole + taskkill)
- Логи: уровень по сборке (release = Warn); окно логов Diagnostics (Internal, Core, API)
- Clash API: ошибки Ping в tooltip; параллельный test; настраиваемый endpoint; закрепление direct-out и активного прокси
- Визард: вкладка Outbounds, Edit Outbound (Settings + Raw), Save без сети и Update в фоне, статус при сохранении, пустое поле URL при загрузке, префиксы по умолчанию, копирование источника по клику

</details>

<details>
<summary><b>🇬🇧 Summary (v0.8.2)</b></summary>

- Unified “download failed” dialog for sing-box, wintun, template, SRS
- SRS local download to `bin/rule-sets/`; SRS button in Rules tab
- Windows: more stable start/stop (AttachConsole + taskkill)
- Logging: build-based level (release = Warn); Diagnostics log viewer (Internal, Core, API)
- Clash API: Ping errors in tooltip; parallel test; configurable endpoint; pin direct-out and active proxy
- Wizard: Outbounds tab, Edit Outbound (Settings + Raw), Save without network and background Update, save status, empty URL field on load, default prefixes, copy source on click

</details>

**v0.8.1** — полное описание (full details): [docs/release_notes/0-8-1.md](docs/release_notes/0-8-1.md)

**v0.8.0** — полное описание (full details): [docs/release_notes/0-8-0.md](docs/release_notes/0-8-0.md)

<details>
<summary><b>Что не вошло в релиз / Not yet released</b></summary>

Черновик следующего релиза (draft): [upcoming.md](docs/release_notes/upcoming.md)

**🇷🇺 Кратко (черновик):**
- **Рефакторинг Custom Rule (018):** Типы правил — константы (ips, urls, processes, srs, raw). Диалог Add/Edit Rule: название над вкладками Form/Raw; режимы Domains (Exact/Suffix/Keyword/Regex); тип SRS с подсказкой runetfreedom; при Raw→Form восстанавливаются outbound и поля по типу. Состояние UI в params.
- **Правило Processes — Match by path:** В диалоге Add/Edit Rule для типа «Processes» можно включить «Match by path» и задавать сопоставление по пути процесса (regex), а не по имени. Режим Simple: подстановка `*` как «любая последовательность» (например `*/steam/*`). Режим Regex: полные регулярные выражения. В конфиг записывается `process_path_regex` (sing-box 1.10+).
- **Кнопка перезапуска:** На дашборде Core между Start и Stop — кнопка перезапуска (🔄). Завершает процесс sing-box, вотчер поднимает снова; в UI кратко «Restarting...», смена состояния кнопок, затем «Running».
- **Сохранение в визарде:** Только запись файлов и Update (без перезапуска sing-box). Конфиг валидируется через `sing-box check` по временному `config-check.json` до записи в `config.json`; при ошибке — сообщение пользователю, рабочий конфиг не перезаписывается. Clash API перечитывается из `config.json` только при запуске sing-box.
- **Диалог Linux capabilities (issue #34):** Команда setcap в выделяемом поле, кнопка «Copy» в буфер обмена.
- **Поддержка Windows 7 (legacy):** Отдельная сборка `singbox-launcher-<version>-win7-32.zip`; для Win7 используется фиксированная 32-битная версия `sing-box` 1.13.2 и 32-битный `wintun.dll`, а визард применяет секции шаблона для платформ `windows` и `win7`.

**🇬🇧 Summary (draft):**
- **Custom Rule refactor (018):** Rule type constants (ips, urls, processes, srs, raw). Add/Edit Rule: name above Form/Raw tabs; Domains mode (Exact/Suffix/Keyword/Regex); SRS type with runetfreedom hint; Raw→Form restores outbound and fields. UI state in params.
- **Processes rule — Match by path:** In Add/Edit Rule, for type «Processes» enable «Match by path» to match by process path (regex). Simple: `*` as wildcard (e.g. `*/steam/*`). Regex: full regular expressions. Stored as `process_path_regex` (sing-box 1.10+).
- **Restart button:** On Core dashboard between Start and Stop; kills sing-box, watcher restarts it; UI shows «Restarting...» then «Running».
- **Wizard save:** Write files and Update only (no sing-box restart). Config validated with `sing-box check` before overwrite; on failure user sees error. Clash API reloaded from `config.json` only when sing-box starts.
- **Linux capabilities dialog (#34):** setcap command in selectable field, «Copy» button.
- **Windows 7 (legacy) support:** Separate `singbox-launcher-<version>-win7-32.zip` build; on Win7 the launcher always uses a fixed 32-bit `sing-box` 1.13.2 legacy binary and 32-bit `wintun.dll`, and the wizard applies both `windows` and `win7` template sections.

</details>
