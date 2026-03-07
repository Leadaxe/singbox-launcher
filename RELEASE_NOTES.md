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

</details>
