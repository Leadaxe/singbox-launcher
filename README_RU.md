# Sing-Box Launcher

[![GitHub](https://img.shields.io/badge/GitHub-Leadaxe%2Fsingbox--launcher-blue)](https://github.com/Leadaxe/singbox-launcher)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24%2B-blue)](https://golang.org/)
[![Version](https://img.shields.io/badge/version-1.1.0-blue)](https://github.com/Leadaxe/singbox-launcher/releases)

**Десктоп-платформа сетевой маршрутизации и анализа трафика. 15+ VPN-протоколов, глубина настроек и API уровня Enterprise. Поверх форка [sing-box-lx](https://github.com/Leadaxe/sing-box-lx) (upstream sing-box + XHTTP + AmneziaWG 2.0) как execution-engine — на всех платформах, включая Windows 7 32-бит.**

**Репозиторий**: [https://github.com/Leadaxe/singbox-launcher](https://github.com/Leadaxe/singbox-launcher)

**🌐 Языки**: [English](README.md) | [Русский](README_RU.md)

> **⚠️ Важное предупреждение**
>
> Данный инструмент является профессиональным программным обеспечением, предназначенным для сетевых администраторов и IT-специалистов с целью администрирования и управления сетевыми подключениями в корпоративных и профессиональных средах.
>
> Использование данного инструмента любым нелегальным образом запрещено. Разработчики не несут ответственности за неправомерное использование и призывают соблюдать действующее законодательство.

---

![Главное окно и Configurator](docs/screenshots/01-hero-core-and-wizard.png)

## Что это

Кросс-платформенный десктоп-клиент (Windows, macOS, Linux), который оборачивает sing-box и добавляет вокруг него всю поверхность управления: визуальный конфигуратор, работу с несколькими подписками, переключение серверов с пингом, сетевую observability, декларативную маршрутизацию через preset bundles, локальный HTTP API и self-healing supervision.

Три слоя, которые вместе определяют продукт:

- **Пользовательский слой** — старт/стоп одной кнопкой, поток «subscription URL → работающий VPN», переключение серверов с пингом, правила маршрутизации через чекбоксы, окно Traffic Profiler с per-process атрибуцией.
- **Power-слой** — Configurator с полной семантикой правил sing-box (CIDR, regex доменов, process matching, sniff, GeoIP/Geosite через SRS), preset bundles с условиями `if` / `if_or`, выбор DNS-серверов с условными правилами.
- **Headless-слой** — bearer-auth Debug API на `127.0.0.1`, 24 endpoints: чтение и запись state, action-триггеры, управление capture трафика, snapshot endpoint для support workflows.

## Возможности

### Подключение

- **15 протоколов соединений** — vless, vmess, trojan, shadowsocks, hysteria, hysteria2, tuic, ssh, wireguard, naive (https / quic).
- **XHTTP-транспорт** — `type=xhttp` ноды (VLESS / VMess / Trojan) парсятся, генерируются в `config.json` и round-trip'ятся обратно в share URI (без деградации в httpupgrade). Работает на bundled-ядре sing-box-lx.
- **AmneziaWG 2.0 (AWG2)** — обфускация на wireguard-endpoint'ах: параметры `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` и CPS-пакеты `i1`–`i5`, парсятся из `wireguard://` и `awg://` URI; MTU AWG-endpoint авто-клампится до 1280.
- **Несколько источников в одном профиле** — subscription URL и direct links (`vless://`, `vmess://`, …) можно смешивать в одной конфигурации.
- **Совместимость с VPN-провайдерами** — first-class поддержка HWID-binding панелей (Marzban, Marzneshin, Remnawave, NashVPN, V2Board / Xboard) через канонический XTLS subscription-header протокол (`X-Hwid`, `X-Hwid-Limit`, `Announce`, `Subscription-Userinfo`).
- **Per-source raw cache** — последнее работающее тело подписки сохраняется при failure (никакого сломанного конфига при недоступности провайдера).
- **TUN inbound** — системный VPN-драйвер на Windows / macOS / Linux с `auto-route`, `auto-redirect` и `find_process` по умолчанию.

### Маршрутизация и правила

- **Preset bundles** — community-maintained rule-pack'и с типизированными переменными, локальными SRS rule-sets, условными фрагментами (`if` / `if_or`). Включаются чекбоксами.
- **User rules** — 5 типизированных kind'ов: IP / CIDR, домен (suffix / keyword / regex), процесс (имя или regex пути), SRS URL, raw JSON.
- **17+ matchers** — domain, IP CIDR, ports, network, protocol, process, package name, GeoSite / GeoIP через SRS, composite rules с `invert`.
- **Per-rule outbound chains** через селекторы (urltest / failover).
- **SRS auto-download** — `⚠` badge на missing-file при открытии Configurator; движок не пытается скачивать SRS через ещё не поднятый VPN.

### DNS

- **Несколько DNS-серверов** с независимым включением/отключением (UDP / DoH / local).
- **Per-domain DNS-правила** — направление конкретных имён на конкретные серверы.
- **Resolve strategy**: `prefer_ipv4` / `prefer_ipv6` / `ipv4_only` / `ipv6_only`.

### Сетевая observability

- **Traffic Profiler** — always-on capture с rolling buffer 60 с × 3000 events, per-process атрибуция, реконструкция CNAME-цепочек, inferred matching по DNS-IP.
- **Issue classification** — `⚠ DnsTimeout` / `⚠ TcpRstEarly` поднимают конкретные диагностические сигналы.
- **Pre-session backfill** — last 60s matching events копируются в новую recording-сессию.
- **Three-stream Log Viewer** — Internal логи лаунчера / Core лог-файл sing-box / Clash API клиент, фильтр по уровню, rotation-safe.
- **IP-check tools** — STUN (UDP) и HTTPS-провайдеры (2ip.ru и др.) для внешней проверки IP.

### Надёжность

- **Auto-restart with stability window** — 3 попытки × 180 с, счётчик сбрасывается после стабильной работы; UI показывает `[restart 2/3]` во время recovery.
- **Atomic file writes** — `stage → rename` для config / state / settings; нет полузаписанных файлов при `kill -9` или потере питания.
- **Power-event aware** — sleep/resume listener; HTTP-запросы не висят после wake.
- **Configuration overlay** — state хранит ссылки на template + diff пользовательских изменений; bump шаблона приносит обновления автоматически, персональные правки остаются сверху.
- **Auto-update подписок** — heartbeat раз в час обновляет только просроченные источники; immediate retry на VPN-event с anti-storm cooldown.

### Системная интеграция

- **System tray** — start / stop, переключатель прокси (когда Clash API on), открыть главное окно, выход. Активный outbound отражается в трее.
- **Keyboard shortcuts** — `⌘R` / `Ctrl+R` reconnect (kill sing-box для restart), `⌘U` / `Ctrl+U` обновить подписки, `⌘P` / `Ctrl+P` пинг всех прокси.
- **CLI флаги** — `-start` (auto-start VPN при запуске), `-tray` (старт минимизированным в трей). Удобно для автозапуска, system services, headless deployment.
- **Auto-loaders** — список прокси восстанавливается при каждом старте sing-box; активный outbound сохраняется между перезапусками.
- **Share URI** — правый клик на любой прокси в Servers tab → **Copy link** генерирует share URI (`vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`, `wireguard://`) из соответствующего outbound в `config.json`.

### Power-инструменты

- **Debug API** — локальный HTTP API (24 endpoints, bearer-auth, off by default) для чтения/записи state, action-триггеров, управления capture трафика. См. [Headless control plane](#headless-control-plane--debug-api).
- **Configurator** — 6-tab визуальный редактор (Sources / Outbounds / Rules / DNS / Settings / Preview) со schema validation, named state snapshots, атомарным save.
- **Snapshot для support** — `GET /debug/snapshot` или кнопка **Copy snapshot** упаковывает template + state + cache + config в один JSON для bug-report.
- **Verbose toggle** — `🔬 dbg` кнопка в Traffic Profiler переключает `log_level=debug` с atomic rebuild и revert.

### Дистрибуция

- **Кросс-платформенность** — Windows 10/11 (полностью протестировано), Windows 7 через legacy-сборку, macOS 11+ universal (Apple Silicon + Intel), Linux (сборка из исходников).
- **11 локалей UI** — English, Русский, Deutsch, Español, Français, Italiano, 日本語, 한국어, Português (BR), Türkçe, 中文.
- **Self-update** — pinned версия ядра (форк sing-box-lx с XHTTP + AmneziaWG, на всех платформах включая Win7/386) автоматически перекачивается при mismatch; проверка self-update лаунчера при старте с уведомлением (без тихой установки).

## Быстрый старт

1. Скачайте релиз с [GitHub Releases](https://github.com/Leadaxe/singbox-launcher/releases) и установите (см. [Установка](#установка)).
2. Откройте приложение → вкладка **Core** → кнопка **Download** скачает `sing-box` (и `wintun.dll` на Windows). Fallback на SourceForge mirror, если GitHub недоступен.
3. Кнопка **Configurator** → вставьте subscription URL на вкладке **Sources** → пройдите Outbounds / Rules / DNS / Settings / Preview → **Save**.
4. Назад в **Core** → **Start**. Переключайте серверы во вкладке **Servers**, наблюдайте за трафиком через кнопку **Traffic Profiler** в Diagnostics.

### Флаги командной строки

```bash
singbox-launcher -start         # авто-старт VPN при запуске
singbox-launcher -tray          # старт минимизированным в системный трей
singbox-launcher -start -tray   # комбинация — headless autostart-сценарий
```

Удобно для OS-level автозапуска (`LaunchAgents` / `Task Scheduler` / `systemd --user`) и для запуска лаунчера как background-сервиса, который управляет sing-box без показа окна.

## Тур по возможностям

### Управление несколькими подписками

![Переключение серверов между подписками](docs/screenshots/02-server-switching.png)

Подключайте несколько источников подписок, у каждой — свой график обновления и per-source raw cache. Вкладка **Servers** показывает selector-группы (`proxy-out`, `vpn ①`, `ru VPN` и т. д.), определённые в ParserConfig, и пинг каждого сервера с переключением в один клик. Активный outbound отражается в трее — быстрая смена без открытия главного окна.

`SubscriptionMeta` каждой подписки раскрывает состояние провайдера — название профиля, support URL, использование трафика (`UploadBytes` / `DownloadBytes` / `TotalBytes`), дата окончания, статус последнего fetch, announce-сообщения от провайдера (`📢` при success-with-announce, `⚠` при error-with-announce — actionable URL в UI).

**Share URI** — правый клик на любую строку прокси в Servers tab → первая строка меню показывает Clash API outbound type (lowercase: `vless`, `vmess`, `trojan`, `selector`, `direct`, …), затем **Copy link** генерирует share URI из соответствующего outbound в `config.json` (или из записи WireGuard `endpoint[]`, если tag не является outbound). Удобно для переноса сервера на другое устройство или передачи коллеге.

### Декларативная маршрутизация через preset bundles

![Rules tab и Subscription identification](docs/screenshots/03-rules-and-hwid.png)

Двухуровневая модель правил:

- **Preset bundles** (community-maintained template) — самодостаточные rule-pack'и с типизированными переменными (`enum`, `dns_server`, `outbound` с whitelist'ами), локальными SRS rule-sets, условными фрагментами (`if` / `if_or`), и определениями DNS-серверов. Включаются чекбоксами. В комплекте готовые наборы: `ru-direct` (российский трафик → direct), `ads-all` (ad-block через SRS), маршрутизация Telegram, разделение BitTorrent и др.
- **User rules** — ваши собственные правила с 5 типизированными kind'ами: IP/CIDR, домены (suffix / keyword / regex), процессы (по имени или regex пути), SRS URL, raw JSON.

Matchers покрывают полную поверхность sing-box: domain (4 варианта), IP CIDR, порты, network, protocol, process, package name, GeoSite / GeoIP через SRS, composite rules с `invert`, per-rule outbound chain через селекторы (urltest / failover).

Секция **Subscription identification** (справа) реализует [SPEC 061](SPECS/061-F-N-SUBSCRIPTION_HEADER_PROTOCOL/SPEC.md) — канонический XTLS / Remnawave subscription-header протокол. Random per-installation HWID, opt-out toggle, опциональный hash-based device model. Можно перегенерировать в любой момент.

### Настройка DNS

![Настройка DNS и About](docs/screenshots/04-dns-configuration.png)

Включение/отключение каждого DNS-сервера, разные транспорты (UDP / DoH / local), стратегия (`prefer_ipv4` / `prefer_ipv6` / `ipv4_only` / `ipv6_only`), per-domain DNS-правила, направляющие на конкретные серверы (например, `domain=mysite.ru → test server`), выбор default DNS resolver. SRS-based domain rules поддерживаются через ту же библиотеку, что и правила маршрутизации.

### TUN, диагностика и power-инструменты

![TUN настройки и диагностика](docs/screenshots/05-tun-settings-and-diagnostics.png)

TUN inbound (системный VPN-драйвер), MTU, выбор стека (system / gvisor), TLS root certificate store, URLTest target и интервал, кастомный proxy-in inbound — всё в визарде, без редактирования JSON.

Power-инструменты справа:

- **Log window** — три параллельных потока (Internal лог лаунчера / Core лог sing-box / Clash API клиент), фильтр по уровню, rotation-safe.
- **Logs / Config folder** — открыть папку в Finder/Explorer одним кликом.
- **Kill Sing-Box** — force restart, supervisor подхватит.
- **Traffic Profiler** — см. ниже.
- **IP check services** — STUN (UDP), HTTPS IP-check провайдеры (2ip.ru и др.).
- **Debug API** — переключатель локального HTTP API; bearer token генерируется при первом включении и сохраняется между off/on.

### Traffic Profiler

![Traffic Profiler — event detail](docs/screenshots/06-traffic-profiler.png)

Always-on фоновый capture с rolling buffer 60 секунд × 3000 events. Два источника, объединённые по conn-id: Clash API `/connections` (метаданные процесса от sing-box) и tail sing-box-лога (DNS resolves, router matches, outbound dials).

Per-process view с четырьмя суб-вкладками:

- **Live** — newest-first stream events с цветовой кодировкой по kind.
- **Domains** — aggregated уникальные домены, отсортированные по байтам; tap раскрывает CNAME chain, все IPs, outbound chain, issues.
- **IPs** — полезно для hostless connections (raw TCP без SNI sniff).
- **Connections** — per-connection timeline; tap раскрывает rule, outbound, CNAME chain, issues.

Issue classification — конкретные проблемы: `DnsTimeout` (DNS resolver не ответил), `TcpRstEarly` (TCP закрылся <1с с 0/0 байт — firewall RST / TLS fail / block). Verbose mode (🔬 dbg) переключает `log_level=debug` с atomic config rebuild и revert. Pre-session backfill копирует last 60s matching events в новую сессию — не теряете первые секунды проблемы, ради которой начинали recording.

### Системный трей

Tray-иконка остаётся видимой после закрытия главного окна и даёт:

- **Start / Stop** sing-box.
- **Переключатель прокси** — при включённом Clash API прокси активной группы появляются как submenu с отмеченным текущим выбором. Переключение из трея запускает тот же путь, что и переключение во вкладке Servers.
- **Показать главное окно** / **Exit**.

В сочетании с CLI флагом `-tray` это режим headless-стиля: лаунчер стартует скрытым, всем управляете из трея, главное окно открываете только когда нужна настройка.

Keyboard shortcuts (работают независимо от текущей активной вкладки, кроме случая, когда фокус в текстовом поле):

| Шорткат | Действие |
| --- | --- |
| `⌘R` / `Ctrl+R` | Reconnect (kill sing-box для restart — supervisor авто-восстанавливает) |
| `⌘U` / `Ctrl+U` | Обновить подписки |
| `⌘P` / `Ctrl+P` | Пинг всех прокси (то же, что кнопка ping-all во вкладке Servers) |

### Headless control plane — Debug API

Локальный HTTP API на `127.0.0.1`, bearer-auth, off by default. 24 endpoints в пяти группах:

| Группа | Что покрывает |
| --- | --- |
| **Health & info** | health-check (без авторизации), версия лаунчера / sing-box / API |
| **State read** | running state, активный прокси, группа, список прокси, full state, resolved outbounds |
| **State write** | rules / DNS / DNS-rules с режимами `replace` и `append`, schema-validation перед commit'ом, mutex per state path |
| **Actions** | start / stop / update-subs / ping-all / rebuild-config — синхронные триггеры |
| **Traffic Profiler control** | start / stop / clear / live snapshot / sessions list / export / drop / processes / verbose toggle |
| **Snapshot** | `/debug/snapshot` — template + state + cache + config одним JSON для bug-report |

Use cases: скрипты автоматизации (`bash` + `curl`), MCP-обёртки для AI-агентов, CI/CD валидация новых шаблонов, headless deployment, регрессионные фикстуры. Публичного, документированного, скриптуемого HTTP API такого охвата нет ни у одного другого desktop sing-box клиента.

**Полный референс + curl-cookbook:** [`docs/API.md`](docs/API.md). Дизайн-нотации: [SPEC 038](SPECS/038-F-C-DEBUG_API/SPEC.md).

Включается в **Settings → Debug API (localhost)**. Тот же `snapshot.Build()` дёргает кнопка **Copy snapshot** в Diagnostics — один клик упаковывает полный стейт для bug-report.

### Auto-update и supervision

- **Process supervision** — 3 попытки restart, окно стабильности 180 секунд сбрасывает счётчик, graceful shutdown с deadline 2 секунды. UI показывает `[restart 2/3]` во время recovery. Тот же паттерн, что у `systemd RestartSec` + `StartLimitBurst`.
- **Per-source heartbeat** (раз в час) — обновляются только просроченные подписки, retry через 15 секунд при failure, immediate retry на VPN-event с cooldown 5 секунд против шторма.
- **Atomic writes** — `stage → rename` для config, state, settings, per-source raw cache. Kill -9 или потеря питания никогда не оставит полузаписанный файл. Failed fetch никогда не перезапишет последний рабочий cached subscription body.
- **Power-event aware** — sleep/resume listener; HTTP-запросы не висят после wake.
- **Self-update** — pinned версия ядра-форка sing-box-lx (Leadaxe/sing-box-lx, с XHTTP + AmneziaWG) автоматически перекачивается при mismatch (SPEC 046); self-update лаунчера проверяется один раз при старте, popup при новой версии.

## Совместимость с VPN-провайдерами

Подтверждённая работа с каноническим subscription-header протоколом для следующих панелей:

| Панель | Формат subscription URL | HWID-binding (SPEC 061) |
| --- | --- | --- |
| **Marzban** | `https://panel.example.com/sub/<token>` | да — `X-Hwid` + `X-Hwid-Limit` |
| **Marzneshin** | `https://panel.example.com/sub/<token>` | да |
| **Remnawave** | `https://panel.example.com/sub/<token>` | да — канонические XTLS-format announce headers |
| **NashVPN** | `https://sub.example.com/<token>` | да — провайдер отдаёт пустое тело без HWID-headers |
| **V2Board / Xboard** | `https://panel.example.com/api/v1/client/subscribe?token=<...>` | частично — только `subscription-userinfo` |
| **3X-UI / X-UI** | прямые vless/vmess/trojan URI | n/a |
| **Sing-box subscription** | любой совместимый источник `sing-box export` | n/a |

User Agent: `singbox-launcher/<version> (<os> <arch>)`. Контроли приватности в Settings:

- **Send device ID** — opt-out для `X-Hwid`. При отключении HWID-binding панели могут отказать в выдаче подписки.
- **Hash device model** — отправляет `hash(model)` вместо чистой строки модели.
- **Device ID (HWID)** — random UUIDv4, не выводится из железа. Перегенерировать в любой момент.

## Требования

### Windows

- **Рекомендуется:** Windows 10 / 11 (x64).
- **Legacy:** Windows 7 (x86/x64) через отдельную сборку `singbox-launcher-<version>-win7-32.zip` с форк-ядром sing-box-lx (32-bit `windows-386-legacy-windows-7` — **XHTTP + AmneziaWG 2.0 работают и на Win7**) и 32-bit `wintun.dll`.
- [sing-box-lx](https://github.com/Leadaxe/sing-box-lx/releases) — форк-ядро (XHTTP + AmneziaWG), авто-загрузка через Core tab на **всех** Windows-сборках, включая Win7 32-bit (`legacy-windows-7`).
- [WinTun](https://www.wintun.net/) (wintun.dll, лицензия MIT) — авто-загрузка через Core tab.

### macOS

- **Universal** (рекомендуется): macOS 11+ (Big Sur), поддерживает Apple Silicon и Intel.
- **Intel-only legacy build**: macOS 10.15+ (Catalina).
- [sing-box-lx](https://github.com/Leadaxe/sing-box-lx/releases) — форк-ядро (XHTTP + AmneziaWG), авто-загрузка через Core tab.

### Linux

Готовые сборки не распространяются. Сборка из исходников — см. [Сборка из исходников](#сборка-из-исходников). Помощь с тестированием на популярных дистрибутивах приветствуется — открывайте issue с feedback.

## Установка

### Windows

1. Скачайте релиз с [GitHub Releases](https://github.com/Leadaxe/singbox-launcher/releases) — обычный архив для Win 10/11, `singbox-launcher-<version>-win7-32.zip` для Windows 7.
2. Распакуйте в любую папку (например, `C:\Program Files\singbox-launcher`).
3. Запустите `singbox-launcher.exe`.
4. Вкладка **Core** → **Download** скачает `sing-box.exe`, затем **Download wintun.dll** при необходимости.
5. Откройте **Configurator** → вставьте subscription URL → пройдите вкладки → **Save** → **Start**.

### macOS

#### Вариант 1: install-скрипт (рекомендуется)

```bash
curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash
```

Устанавливает в `/Applications/`, снимает quarantine, чинит permissions, обеспечивает совместимость с Apple Silicon и свежими macOS. Для конкретной версии:

```bash
curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash -s -- v1.1.0
```

#### Вариант 2: ручная установка

1. Скачайте macOS ZIP с [Releases](https://github.com/Leadaxe/singbox-launcher/releases) и распакуйте.
2. Снимите quarantine:
   ```bash
   xattr -cr "singbox-launcher.app" && chmod +x "singbox-launcher.app/Contents/MacOS/singbox-launcher"
   ```
3. Дважды кликните `singbox-launcher.app` или `open singbox-launcher.app` из терминала. Если macOS блокирует, разрешите через **System Settings → Privacy & Security → Open Anyway**.

### Linux

Соберите из исходников ([Сборка из исходников](#сборка-из-исходников)), затем:

```bash
chmod +x singbox-launcher
./singbox-launcher
```

`sing-box` авто-скачивается при первом запуске в `bin/`. Если `sing-box` есть в `PATH` (например, из distro-пакета), лаунчер использует этот бинарь.

## Структура папок

```
singbox-launcher/
├── bin/
│   ├── sing-box(.exe)             — движок, авто-загрузка
│   ├── wintun.dll                  — только Windows, авто-загрузка
│   ├── config.json                 — sing-box runtime config (derived view)
│   ├── wizard_template.json        — community template с preset bundles
│   ├── wizard_states/<name>.json   — named state snapshots
│   ├── subscriptions/<id>.raw      — per-source raw cache (SPEC 052)
│   ├── rule_sets/*.srs             — кешированные SRS rule-sets
│   └── logs/                       — sing-box.log + rotated history
└── singbox-launcher(.exe)
```

Layout `bin/` — стабильный контракт, на который могут полагаться внешние инструменты (backup-скрипты, MCP-серверы, CI).

## Сборка из исходников

См. платформо-специфичные гайды:

- **Windows** — [docs/BUILD_WINDOWS.md](docs/BUILD_WINDOWS.md) (Go 1.24+, требуется GCC, опционально `rsrc` для иконки)
- **macOS** — `./build/build_darwin.sh [universal|arm64|intel|catalina]`, опционально `-i` для установки/обновления в `/Applications`
- **Linux** — [docs/BUILD_LINUX.md](docs/BUILD_LINUX.md) (Go 1.24+, OpenGL + X11 dev-пакеты, или Docker-сборка)

Quick reference:

```bash
git clone https://github.com/Leadaxe/singbox-launcher.git
cd singbox-launcher

# macOS — universal binary, установка в /Applications
./build/build_darwin.sh -i universal

# Linux — сборочный скрипт с проверкой пакетов
./build/build_linux.sh

# Windows — см. docs/BUILD_WINDOWS.md
build\build_windows.bat
```

## Тесты

Используйте централизованные скрипты в `build/` — они исключают GUI-пакеты, требующие OpenGL на headless-runner'ах:

```bash
./build/test_linux.sh    # Linux
./build/test_darwin.sh   # macOS
build\test_windows.bat   # Windows
```

Чтобы запустить GUI-тесты локально, задайте `TEST_PACKAGE` в скрипте или вызовите `go test` напрямую на нужном пути.

## Документация

- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** — полная карта архитектуры проекта.
- **[SPECS/CONSTITUTION.md](SPECS/CONSTITUTION.md)** — архитектурные инварианты.
- **[SPECS/](SPECS/)** — 60+ спецификаций фич (HWID protocol, traffic profiler, debug API, preset bundles, state-as-template-diff, atomic writes, typed event bus, …).
- **[docs/WIZARD_TEMPLATE_RU.md](docs/WIZARD_TEMPLATE_RU.md)** — справочник по синтаксису `wizard_template.json` (для VPN-провайдеров, поставляющих собственный шаблон).
- **[docs/ParserConfig.md](docs/ParserConfig.md)** — справочник по настройке парсера подписок.
- **[docs/TRAFFIC_PROFILER.md](docs/TRAFFIC_PROFILER.md)** — внутренности и использование Traffic Profiler.
- **[docs/TEMPLATE_REFERENCE.md](docs/TEMPLATE_REFERENCE.md)** — справочник схемы `wizard_template.json`.

## Решение проблем

| Симптом | Что проверить первым |
| --- | --- |
| sing-box не стартует | **Core → Download**, затем проверьте наличие `config.json`. Смотрите `bin/logs/sing-box.log`. |
| Configurator открывается, но Save падает | Internal log в **Log window** — там лог schema-validation. |
| Вкладка Clash API недоступна | sing-box не запущен (вкладка намеренно disabled, пока движок не поднят). |
| Подписка возвращает пусто / ошибки | Проверьте **Subscription identification** в Settings — HWID-binding панели требуют `Send device ID` включённым. Смотрите tooltip ⚠ badge — там announce от провайдера. |
| TUN не захватывает трафик (Linux/macOS) | Для TUN-интерфейса обычно нужен root: `sudo ./singbox-launcher` или `sudo setcap cap_net_admin+ep ./singbox-launcher` (Linux). |
| Win7 32-bit: иконка в трее есть, окно пустое / без виджетов | OpenGL 2.0 против требования Fyne 2.1+ — см. [docs/WIN7_OPENGL.md](docs/WIN7_OPENGL.md), drop-in фикс через Mesa3D. |
| Auto-update подписок молчит | Откройте **Settings → Subscriptions** — убедитесь, что `Auto-update subscriptions` включён. Heartbeat раз в час; immediate retry срабатывает на VPN-event. |
| Нужен полный state для bug-report | **Diagnostics → Copy snapshot** упакует template + state + cache + config одним JSON. |

## Вклад в проект

Для значимых фич проект использует spec-driven workflow — пишется SPEC в `SPECS/`, описывающий схему, фазы, инварианты и acceptance criteria до кода. См. [AGENTS.md](AGENTS.md) (contributor guide) и [SPECS/README.md](SPECS/README.md) (closing-task checklist).

Стандартный flow:

1. Форк репозитория.
2. Ветка от `develop` (`git checkout -b feature/your-feature`).
3. Для нетривиальной работы — сначала draft SPEC и обсуждение.
4. Коммит, push, Pull Request в `develop`.

Code style: `gofmt`, `golangci-lint`. Публичные функции должны быть задокументированы. Новые пути должны логировать start / success / error по `SPECS/CONSTITUTION.md §5`.

## Лицензия

GNU General Public License v3.0 — см. [LICENSE](LICENSE).

Коммерческое лицензирование от Leadaxe доступно для использований, несовместимых с GPLv3. Коммерческие условия согласуются приватно и не публикуются в репозитории. Контакт: [leadaxe@gmail.com](mailto:leadaxe@gmail.com). См. [LICENSING.md](LICENSING.md).

## Благодарности

- [SagerNet/sing-box](https://github.com/SagerNet/sing-box) — proxy-движок.
- [Fyne](https://fyne.io/) — кросс-платформенный UI-фреймворк.
- Всем контрибьюторам проекта.

## Поддержка

- **Telegram-канал** — [@singbox_launcher](https://t.me/singbox_launcher)
- **Issues** — [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)

---

*Независимый проект. Не аффилирован с проектом sing-box.*
