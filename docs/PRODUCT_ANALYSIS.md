# Продуктовый анализ singbox-launcher

**One-liner:** «Декларативный routing engine + сетевой профайлер + headless control plane поверх sing-box».

---

## Позиционирование

**Десктоп-платформа сетевой маршрутизации и анализа трафика. 15+ VPN-протоколов, глубина настроек и API уровня Enterprise. Поверх sing-box как execution-engine.**

Семь тезисов, описывающих продукт:

1. **VPN observability platform.** Traffic Profiler + Debug API дают полную видимость трафика и состояния — за 30 секунд видно, какой outbound выбрал router, какой DNS resolve дал какой IP, по какому правилу прошло соединение.

2. **Headless mode из коробки.** Bearer-auth HTTP API из 25 endpoints (24 protected + unauthenticated `/ping`), документированный MCP integration path (SPEC 038 §6.5). Пригоден для DevOps-скриптов и AI-агентов: Claude через MCP читает `/state`, переключает прокси, рестартит engine — без открытия UI.

3. **Configuration overlay.** State хранит ссылки на community template + diff пользовательских изменений (SPEC 058). Bump шаблона приносит обновления автоматически, персональные правки переезжают сверху. Паттерн дотфайл-менеджеров (chezmoi / yadm), применённый к VPN-конфигам.

4. **Reliability-first.** Auto-restart with stability window (3 attempts × 180s), atomic writes (`stage → rename`), per-source raw cache с резервированием на failure, power-event aware HTTP transports.

5. **Self-service support через snapshot.** `GET /debug/snapshot` + кнопка «Copy snapshot» в Diagnostics → один клик упаковывает template + state + cache + config в clipboard для bug-report.

6. **Полная карта трафика per-process с CNAME-chain reconstruction.** Аналог уровня Little Snitch ($59 macOS) / GlassWire ($39 Windows) — кросс-платформенно, free, интегрировано с routing engine.

7. **Декларативный routing для не-программистов.** Preset bundles (SPEC 053) с условиями `if`/`if_or`, локальными SRS rule-sets, типизированными vars (`enum`, `dns_server`, `outbound` с whitelist'ами) превращают написание pfSense-уровневых правил в чек-боксы и dropdown'ы.

---

# Пользовательские возможности

## Split-соединения / per-connection routing

Файлы: `internal/traffic/clash_connections.go`, `api/clash.go`, `ui/clash_api_tab.go`, `core/state/connections.go`, `bin/wizard_template.json`.

### TUN-inbound + декларативный split-tunneling

Шаблон (`bin/wizard_template.json`, 1432 строки) включает TUN inbound с auto-route + auto-redirect (Linux) + `find_process: true` — аналог системного VPN-драйвера. Каждый пакет проходит через routing-engine sing-box с правилами из `state.rules[]`. Decision per-connection: **proxy / direct / block** с гранулярностью домен + CIDR + порт + процесс (по имени или regex пути) + sniff (TLS SNI / HTTP Host).

### Real-time observability per-connection — Clash API `/connections`

`internal/traffic/clash_connections.go` опрашивает `http://127.0.0.1:9090/connections` каждую секунду. По каждому соединению известно: процесс-источник (путь и имя), сеть/протокол, host, destination IP+port, счётчики upload/download, цепочка outbound-селекторов и matched rule.

`ConnPoller` сравнивает соседние snapshot'ы и выдаёт три типа событий:
- **Opened** — новые conn-id'ы → emit `TCPOpen` / `UDPOpen`
- **Closed** — исчезли → emit `*Close` с длительностью `now - start`
- **Bytes** — байт-дельта для активных

Stream lifecycle-событий с per-process attribution.

### Routing decision visibility

Каждое соединение в `/connections` несёт `chains: ["vless-server", "🇫🇮 Финляндия", "vpn-1"]` (leaf→root outbound chain) и `rule: "domain_suffix"`, `rulePayload: "example.com"` — юзер видит, по какому правилу принят routing decision и через какую цепочку селекторов соединение пошло. В UI Traffic Profiler chain раскрывается до полного: `Slack.app → api.slack.com → CNAME slack-edge.com → 104.16.x.x → via vpn-1 → 🇫🇮 Финляндия`. Уровень pfSense / Untangle dashboards с привязкой к процессу-источнику.

### Split-tunneling политики

Типовые preset bundles в шаблоне: `ru-direct` (RU-трафик → direct, остальное → VPN), `ads-all` (block доменов рекламы) и др. Декларативное policy-based routing: «процесс X → direct, домен Y → block, всё остальное → VPN-EU». Конфигурация в Configurator — чекбоксы и dropdown'ы.

---

## Комплексные правила routing'а — firewall-уровень

Файлы: `core/state/rule_types.go`, `SPECS/053-F-N-PRESET_BUNDLES/SPEC.md`, `SPECS/018-F-C-CUSTOM_RULE_SUBSYSTEM_REFACTOR/SPEC.md`, `ui/configurator/tabs/rules_tab.go`, `bin/wizard_template.json`.

### Двухуровневая модель правил

**Уровень 1 — preset bundles** (SPEC 053, schema `presets_v1`). `wizard_template.json` содержит self-contained пресеты с:

- `vars` — параметры (типы: `outbound | dns_server | enum | text | number | bool`) с UI-control'ами (dropdown, checkbox, text entry)
- `rule_set[]` — локальные SRS rule-sets (inline или remote URL), теги префиксуются `<preset_id>:<tag>`
- `dns_servers[]` — локальные DNS-серверы пресета
- `rule` — sing-box routing rule
- `dns_rule` — sing-box dns rule
- `if` / `if_or` условия — `if: ["use_yandex_dns"]` показывает фрагмент только если перечисленные bool vars = true

Каждый var имеет `options` (whitelist) или `select: "local"|"global"` (scope shortcut). `default` обязателен.

**Уровень 2 — user rules** (SPEC 018). 5 типизированных констант:

- `ips` — IP/CIDR
- `urls` — domain / domain_suffix / domain_keyword / domain_regex
- `processes` — process_name или process_path_regex (с режимом Simple/Regex, сохраняемым в `params`)
- `srs` — SRS rule-set по URL (свой или из runetfreedom-каталога)
- `raw` — сырой JSON правила

### Поддержанные matchers

Из `bin/wizard_template.json` и `core/state/rule_types.go` (`Rule.Body` через `InlineBody.Match map[string]interface{}`):

- `domain`, `domain_suffix`, `domain_keyword`, `domain_regex`
- `ip_cidr`, `source_ip_cidr`
- `port`, `port_range`, `source_port`, `source_port_range`
- `network` (tcp/udp), `protocol`
- `process_name`, `process_path`, `process_path_regex`
- `package_name` (Android-совместимость)
- `geosite`, `geoip` (через rule_set с remote URL)
- `rule_set` (SRS binary файлы, локальные)
- `inbound`, `outbound`, `user`, `clash_mode`
- Композиция: вложенные `rules` + `invert: true`

Per-rule action: `route(outbound)`, `route-options`, `reject`, `direct`.

### SRS — Sing-box Rule Sets

`core/state/rule_types.go::SrsBody{Name, SrsURL, Outbound}` + SPEC 014 + SPEC 020 (local download). Лаунчер:

- кеширует `.srs` файлы в `bin/rule_sets/`
- запрещает `type: remote` в финальном `config.json` (`convertRuleSetToLocalRequired()` в `core/build/route_merge.go`, SPEC 045 фаза 9) — иначе sing-box на cold-start пытается скачать rule-set через VPN-прокси, который ещё не поднят, → fatal. Invariant ловит реальный bug
- авто-скачивает SRS на открытие configurator (если файл отсутствует)
- показывает badge ⚠ если SRS file missing

### Уровень сложности

В сравнении с pfSense (даёт `src/dst/proto/port` правила с `action: pass/block/reject` и интерфейсный matcher) singbox-launcher добавляет:

- sniff matcher (TLS SNI, HTTP Host)
- process matcher по пути с regex
- GeoIP/Geosite via SRS
- composite rules с `invert`
- per-rule outbound chain через селекторы (urltest / failover)
- routing decision per-connection с full chain visibility (см. раздел Split)

sing-box не stateful firewall — это L4 routing engine, и продукт остаётся в этих рамках намеренно.

---

## Анализатор логов

Двухконтурная система с typed multi-sink архитектурой.

### Internal sink (для логов лаунчера)

`internal/debuglog/debuglog.go`: 6 уровней (`Off/Error/Warn/Info/Verbose/Trace`), фабрики `ErrorLog/WarnLog/InfoLog/DebugLog`, `StartTiming()` для измерения операций, `LogTextFragment()` для умного логирования больших блоков с обрезкой. Конституция (`SPECS/CONSTITUTION.md §5`) требует: новые участки кода обязаны логировать start / success / error.

Через `SetInternalLogSink` лог-канал лаунчера раздаётся UI-вьюеру в realtime. Дев-режим — verbose, release — warn-only, определяется автоматически по ветке сборки.

### Log Viewer Window (SPEC 007)

`ui/log_viewer_window.go` (singleton окно). 3 параллельные вкладки:

| Tab          | Источник                                                                              | Уровень-фильтр                |
| ------------ | ------------------------------------------------------------------------------------- | ----------------------------- |
| **Internal** | sink `debuglog.SetInternalLogSink` — все вызовы из лаунчера в реальном времени        | да, Error→Trace, на render    |
| **Core**     | tail файла `bin/logs/sing-box.log` (rotation-safe) с авторефрешем 5с                   | визуально по парсингу keyword |
| **API**      | sink `api.writeLog()` — все вызовы `core/services/api_service.go` к Clash API         | да, по уровню                 |

Корпоративная классификация наблюдаемости: 3 концентрических круга (launcher / API клиент / engine), каждый со своим конвейером, в одном окне.

### Log tailer с rotation detection

`internal/traffic/logtail.go` + `inode_unix.go` / `inode_windows.go`: inode/FileIndex-based rotation detection. Каждый read tick сравнивает identity текущего FD с identity файла по path; rename rotate → reopen с начала; truncate → seek to 0. `fsnotify` исключён намеренно — шумит и не покрывает edge case'ы macOS-rename'ов. Уровень журналируемого SRE-tail'а (типа Vector / Filebeat).

### Лог-парсер (анализатор событий) — `internal/traffic/parser.go`

Детерминированный парсер sing-box логов:

- 5 регексов: `reDNSExchanged`, `reDNSFailed`, `reRouterProcess`, `reRouterMatch`, `reInboundOut`
- `LogLine` — структурированное представление одной строки: `TS, Kind (EventDNSResolve|DNSFail|TCPOpen|TCPClose|UDPOpen|UDPClose|RouterMatch), ConnID, Domain, IP, CnameTarget, Port, ProcessPath, Rule, Outbound, FailReason`
- `connIDInner = [0-9A-Za-z._-]+` — ловит numeric и uuid формы (sing-box менял формат)
- толерантен к опциональному timestamp prefix (`reLeadingTS`), 3 разных формата времени
- покрыт `parser_test.go` zoo логов из `testdata/sing-box-logs/`

Поток sing-box логов превращается в типизированные события `LogLine` с kind discriminator, раздаются через `Subscribe()` UI-вьюеру + Traffic Profiler'у одновременно. Cross-source join по `ConnID` с Clash API `/connections` собирает картину: «процесс Slack пошёл в api.slack.com через outbound vpn-1, по правилу domain_suffix, TLS handshake получил RST за 800ms». Observability уровня Grafana Tempo / Datadog APM, применённого к VPN-стеку.

---

## Traffic Profiler — окно сетевого анализатора

Файлы: `SPECS/059-F-N-TRAFFIC_PROFILER/SPEC.md` (730 строк), `internal/traffic/profiler.go`, `session.go`, `types.go`, `clash_connections.go`, `logtail.go`, `parser.go`, `ui/traffic/window.go`, `live_view.go`, `per_process_view.go`, `process_picker.go`, `toolbar.go`, `event_detail.go`.

### Singleton сервис, always-on

`TrafficProfiler` (`internal/traffic/profiler.go`) — singleton, поднимается на app startup в `ui/traffic_bootstrap.go::startProfiler`, живёт до quit. Rolling buffer 60s × 3000 events заполнен постоянно. Стоимость: ~50–200 строк/сек regex-парсинга в background goroutine.

### Cross-source join

Два источника:

1. **Clash API `/connections`** (poll 1s) — list активных соединений с метаданными процесса.
2. **sing-box.log tail** через inode-rotation-safe tailer.

Связывание по conn-id (`[12345]` префиксу в логах = `id` в Clash API). Profiler держит:
- `connProcessMap[conn_id] → process_path` (из лог-строки `router: found process name`)
- `dnsAccum[conn_id] → []string` (CNAME chain накопление)
- `dnsByIP[dest_ip] → DNSAttribution` (10-секундное окно для inferred-attribution через DNS)

### Attribution с confidence уровнями

- `verified` — sing-box лог явно сматчил process name по пути
- `inferred` (〽) — TCP к IP, который был resolved через DNS-запрос, attribute'нутый к target'у в 10s окне
- `unattributed` — ничего не сработало (показывается только в Live system-wide)

### ⚠ Issue классификация

Конкретные diagnostic-сигналы (не статистические аномалии):

- **DnsTimeout** — `dns: exchange failed for X: context deadline exceeded`
- **TcpRstEarly** — TCP закрылся за <1s с 0/0 байт. Firewall RST / TLS fail / block

Отвергнуто как noisy: `geoMismatch`, `unusualPort`, `badLatency` — LxBox прошёл этот путь и выпилил.

### 4 представления per-process

| Sub-tab          | Что показывает                                                                                       |
| ---------------- | ---------------------------------------------------------------------------------------------------- |
| **Live**         | newest-first stream events с цветовой кодировкой kind                                                |
| **Domains**      | aggregated unique domains, sorted by total bytes; tap → CNAME chain, all IPs, outbound chain, issues |
| **IPs**          | aggregated unique destinations; useful для hostless connections (raw TCP без SNI sniff)              |
| **Connections**  | per-connection timeline (open/close); tap → CNAME chain, IPs, rule, outbound, issues                 |

### Verbose toggle с поддержкой revert

🔬 dbg кнопка переключает `vars[log_level]=debug`, делает atomic config rebuild + `ProcessService.KillForRestart()` (sing-box watcher автомат-рестарт). На OFF — revert. Confirmation dialog «Reloading sing-box — active connections will reset».

### Pre-session backfill

Rolling buffer 60s × 3000 events ВСЕХ процессов. На `▶ START` события за last 60s, которые match target, копируются в session с marker 〽 backfilled. Решает классическую проблему «юзер видит проблему, только потом начинает recording — теряет первые секунды». Паттерн observability-best-practice (как `bpftrace` history buffer).

### Lifecycle и edge cases

- Ring-buffer 5 completed sessions. Force-stop = wipe (in-memory only, no persist).
- Sing-box restart mid-session → auto-finalize partial CNAME chains, session continues с новым conn-id space, `verbose_toggled_at` фиксируется.
- Memory cap 50000 events / 3h sliding → `events_dropped` counter в footer.
- Log rotation safe.
- Window закрыто → background capture continues (rolling buffer + clash poll), на reopen видишь активную session.
- Export session JSON через overflow menu (clipboard + file).

### Аналоги в нативной экосистеме

| Нативный аналог       | Что общего                          | Что у нас лучше                                                                |
| --------------------- | ----------------------------------- | ------------------------------------------------------------------------------ |
| Little Snitch (macOS) | per-process соединения, host class. | + cross-platform, + integrated с VPN routing engine, + free                    |
| Wireshark             | packet-level inspection             | L4 (proto/host/process) vs L2-L7 — разные классы; у нас integration с routing |
| GlassWire (Windows)   | per-process traffic graph           | + cross-platform, + per-domain agg, + headless API                              |

В мире VPN-клиентов аналога нет: ни NekoBox, ни V2RayN, ни Clash Verge не дают per-process attribution с CNAME chain reconstruction и DNS-IP inferred matching.

---

## Subscription metadata — видимость состояния провайдера

`SubscriptionMeta` (`core/state/connections.go`):

- `profile_title`, `support_url`, `profile_web_page_url`, `content_disposition_filename` — из HTTP headers и inline `#header:` в первой строке тела (LxBox-совместимый контракт)
- `UserInfo{UploadBytes, DownloadBytes, TotalBytes, ExpireUnix}` — раскрытый `subscription-userinfo` (V2Board / Xboard форматы)
- `LastStatus` (`ok`/`err`), `ErrorCount`, `LastErrorMsg`, `HTTPStatusCode`, `RawBodyBytes`
- `NodesCountFetched`, `Truncated` (если обрезали по `max_nodes`), `PreviewNodes` (first 50)
- `ProviderAnnounce` (SPEC 061) — провайдер шлёт announce-headers даже на failure (для HWID-limit / quota exceeded / IP-bind violation) → UI показывает 📢 на success-with-announce, ⚠ на err-with-announce. URL provider'а — actionable в UI

Per-subscription observability: «у меня осталось 23 GB, до 2027-01-15, последний fetch упал с HWID-limit, провайдер сказал "renew via https://..."».

---

# Технические возможности

## Debug API — headless control plane

Файлы: `core/debugapi/server.go` (345 строк), `state_endpoints.go`, `traffic_endpoints.go`, `snapshot.go`, `SPECS/038-F-C-DEBUG_API/SPEC.md`, `SPECS/050-F-N-DEBUG_API_STATE_MUTATIONS/SPEC.md`.

### Поверхность API — 25 endpoints (24 protected + unauthenticated `/ping`) в 6 группах

| Группа | Что покрывает |
| --- | --- |
| **Health & info** | health-check без авторизации, версия лаунчера / sing-box / API |
| **State (read)** | snapshot текущего состояния, активный прокси, выбранная группа, список прокси, full state, resolved outbounds |
| **State (write)** | rules / dns / dns-rules с режимами replace и append, schema-validation перед commit'ом, mutex per state path |
| **Actions** | start / stop / update-subs / ping-all / rebuild-config — синхронные триггеры всех ключевых действий |
| **Traffic Profiler control** | start / stop / clear capture, live rolling-buffer snapshot, sessions list + export + drop, processes inventory, verbose log-level toggle |
| **Snapshot** | `/debug/snapshot` — template + state + cache + config в одном JSON-ответе |

Контракт версионирован (`api: "debugapi/v1"`), сохраняет обратную совместимость при расширении.

### Snapshot endpoint — фича для support workflow

`GET /debug/snapshot` (`core/snapshot/snapshot.go`) возвращает за один HTTP-вызов 4 файла — template / state / cache / config — каждый как inline JSON, плюс launcher_version / singbox_version / captured_at. Файлов нет на диске → попадают в `Missing`; есть, но битый JSON → `Errors`. Полная картина проблемы одной командой: bug report можно прислать в issue с `curl ... /debug/snapshot > snapshot.json` и весь стейт у разработчика.

Тот же `snapshot.Build()` дёргает UI-кнопка «Copy snapshot» в Diagnostics tab — один Source of Truth.

### Уникальность

Публичного, документированного, скриптуемого HTTP API для управления routing engine и state'ом нет ни у одного desktop-клиента в категории.

### Use cases

- **Скрипты автоматизации**: «прогнать update-subs на 3 разных DNS-конфигах, замерить latency» — обычный bash + curl
- **MCP-обёртки для AI-агентов**: Claude через MCP читает `/state`, переключает прокси, рестартит — без касания UI (SPEC 038 §6.5)
- **Регрессионные фикстуры**: snapshot capture для repro проблем
- **CI/CD**: проверка валидности новых шаблонов через `/action/rebuild-config`
- **Headless deploy**: запустить лаунчер, заскриптовать настройку, никогда не открыть UI

---

## Проработанные аспекты безопасности

`core/debugapi/server.go`:

- bind строго `127.0.0.1` (no LAN, no 0.0.0.0)
- bearer token, 32 байта entropy, `base64.RawURLEncoding` (43 символа)
- `crypto/subtle.ConstantTimeCompare` для проверки токена — защита от timing атак на loopback'е
- mutating endpoints — только POST/PATCH/DELETE (защита от drive-by triggers через открытые web-tabs)
- off by default, токен генерируется при первом enable, сохраняется между OFF/ON
- token не попадает в debug-логи (`urlredact.RedactToken`)
- graceful shutdown с 5s deadline

`core/state/`:

- запрет `type: remote` rule_set в финальном `config.json` (SPEC 045 фаза 9) — иначе cold-start пытается скачать через ещё не поднятый VPN-прокси, fatal
- atomic file writes (SPEC 041): `stage → rename` с `.tmp`/`.swap` суффиксами — защита от обнуления `config.json` / `settings.json` при kill -9 / power loss
- per-source raw body атомарно через `.tmp + Rename`; failure не перезаписывает старый working .raw (SPEC 052)

Privacy:

- Constitution §6.3 — запрет на телеметрию / неявный сбор
- HWID UUID — random, не derived из system serial; юзер может regenerate (SPEC 061)
- opt-out на отправку HWID и хеш модели устройства в Settings

---

# Архитектурные решения

## State-as-template-diff (SPEC 058)

`SPECS/058-R-N-STATE_AS_TEMPLATE_DIFF/SPEC.md` + `core/state/rule_types.go`. State хранит тонкие ссылки на template + diff пользовательских изменений. `Rule` имеет `Kind` (`preset|inline|srs`), `Ref` (для preset), `ID` (для user), `Body` (kind-specific payload). Для пресета `PresetBody.Vars` — мап только тех vars, что юзер изменил относительно template-default.

Bump шаблона (новые домены в block-листе, новые TLD) → юзер получает их без действий с его стороны. Паттерн configuration overlay.

## Auto-update + supervision (SPEC 052)

### Process supervision со stability window

`core/process_service.go`. Параметры: 3 попытки restart, окно стабильности 180 секунд, graceful shutdown с deadline 2 секунды.

Цикл `Monitor()`:

1. Process exited → если рестарт запрошен юзером → restart без счётчика.
2. Счётчик crash-attempts превысил 3 → stop, dialog «restart_failed».
3. Иначе increment, восстановить через delay.
4. После успешного restart goroutine ждёт окно стабильности; если процесс прожил столько без crash'а — счётчик сбрасывается. Stability window pattern (как у systemd `RestartSec` + `StartLimitBurst`, kubelet'а, supervisord). UI отражает `[restart 2/3]` в баннере.

### Pre-start config rebuild

`process_service.go:83`: перед каждым `Start` дёргается `RebuildConfigIfDirty()` — если wizard Save поднял `CacheStale` / `ConfigStale` маркеры, config.json пересобирается из state + raw cache + template. На ошибке — best-effort, лог + старт со старым конфигом. Invariant SPEC 045: state.json — source of truth, config.json — derived view, ребилд detects stale-state.

### Per-source heartbeat (SPEC 052 фаза 8)

`core/auto_update.go`. Параметры: heartbeat 1 час, retry-delay 15 секунд, дефолтный reload-interval 1 час, anti-storm cooldown для event-triggers 5 секунд.

Алгоритм:

1. Heartbeat: проход по списку enabled subscription; для каждой смотрим время последнего fetch. Если возраст превысил effective-reload (per-source → global → 1h fallback) → подтягиваем эту подписку. Свежие источники пропускаются.
2. Failure → retry с задержкой. Не рекурсивный — если retry упал, ждём heartbeat или event.
3. VPN-event trigger: подписка на `VpnStateChanged` + `ProxyActiveChanged` → immediate retry для failed source'ов (с cooldown anti-storm).
4. State-level mutex сериализует `load → mutate → save` циклы.
5. Power resume callback — после wake просроченные источники подтягиваются.

### Self-update

`core/core_version.go` + `auto_update.go`:

- pinned sing-box версия через `constants.RequiredCoreVersion` (SPEC 046) — на mismatch автоматически перекачивается
- pinned template ref через `constants.RequiredTemplateRef` — CI ldflags инжектит SHA конкретного commit'а, template одобряется git'ом
- launcher self-update — checked on startup один раз, popup на новую версию (без авто-installation — пользователь решает)

---

## Layered + DI

`core` (engine, fyne-free) → `core/services/` (fyne-free сервисы) → `core/uiservice/` (fyne-зависимая обёртка) → `ui` (Fyne).

Архитектурный инвариант зафиксирован в `SPECS/CONSTITUTION.md §1.5`:

- UI не обращается к core/network напрямую
- парсер детерминирован, без побочных эффектов
- платформозависимый код изолирован в `internal/platform`

Контракт, не привычка.

## Pipeline парсинга и сборки конфига

Парсинг подписок → нормализация (`core/config/configtypes/`) → trех-проходный генератор outbounds с топологической сортировкой селекторов (`core/config/outbound_generator.go`) → resolver presets (`core/state/`, SPEC 057/058) → атомарная запись `config.json` (SPEC 041) → `sing-box check` валидация перед commit'ом → supervised запуск (`core/process_service.go`).

## Typed State Engine

`core/state/`: `state.go`, `connections.go`, `rule_types.go`, `dns_options.go`, `diff.go`, `migration_v5_to_v6.go`, `legacy_migration.go`, `raw_cache.go`, `disk_v6.go`, `adapter.go`, `provider_announce.go`, `ulid.go`.

Полноценная модель домена с дискриминаторами (`SourceType`, `RuleKind`, `DNSSource`), множественными миграциями (v2 → v3 → v4 → v5 → v6), диффами (SPEC 058).

## Typed EventBus (SPEC 047)

`core/events/`. `MemoryBus` с `Subscribe(kind, handler) Cancel`, типизированные payload'ы (`StateChanged`, `ConfigBuilt`, `SubscriptionUpdated`, `VpnStateChanged`, `ProxyActiveChanged`, `PowerResume`, `AutoUpdateStatus`). Контракт: panic в одном handler'е не ломает доставку другим.

SRE-фичи (auto-update on VPN-event, SPEC 052) и observability (UI icon Core tab подписан на `VpnStateChanged`) развязаны от core.

## Atomic file writes (SPEC 041)

`core/config_service.go`, `internal/locale/settings.go`, `core/state/save.go`. Паттерн `stage → rename` с `.tmp`/`.swap` суффиксами. На macOS/Linux — POSIX-rename атомарен; на Windows — через `MoveFileEx` (Go 1.22+).

## Power-events

`internal/platform/`. Sleep/resume listener с `IsSleeping()` контрактом, на который подписаны таймеры трея, AutoLoadProxies, HTTP requests (PowerContext, ErrPlatformInterrupt), Clash API tab. SPEC 011 — фикс «launcher freeze after sleep»: сетевые запросы не висят после resume.

## Bin layout как ABI

`bin/` — стабильный layout:

- `config.json` — sing-box runtime config (derived)
- `wizard_template.json` — community template (1432 строки)
- `wizard_states/<name>.json` — named state snapshots
- `subscriptions/<source_id>.raw` — per-source raw body cache (SPEC 052)
- `rule_sets/*.srs` — cached SRS files
- `logs/sing-box.log[.old]` — engine logs

Контракт: внешние инструменты (бэкап-скрипты, MCP-серверы, CI) знают где искать.

---

# Anti-features (намеренно отсутствует)

1. Editor sing-box config.json. Конфиг — derived view от state + template; ручная правка возможна, но не поддерживается как первый класс.
2. CRUD subscriptions через API (SPEC 038 §5). Десктоп wizard покрывает это полностью; дублирование через HTTP — лишний surface для багов. Mobile LxBox даёт полный CRUD, на desktop урезано намеренно.
3. Log streaming endpoint в Debug API. In-memory log ring в `debuglog` отсутствует. Streaming / SSE / WebSocket — нет.
4. Packet capture / pcap. sing-box работает на L4, лаунчер — на L4 событиях.
5. TLS fingerprinting (JA3/JA4). sing-box не expose'ит.
6. Persist Traffic Profiler sessions через restart. In-memory only (как LxBox §044).
7. Auth roles. Bearer-token = full-power. SPEC 050: «кто получил токен — может всё».
8. Remote доступ к Debug API. 127.0.0.1 only, любой remote — через adb-forward или ssh-tunnel явно.
9. Телеметрия / неявный сбор. Constitution §6.3.
10. Bundling sing-box / wintun. Лицензионная чистота — отдельный лаунчер скачивает обе binary по версии, pinned (SPEC 046).
11. Один файл с настройками. State разделён: settings.json (язык, debug API token, theme), state.json (wizard), config.json (sing-box runtime), wizard_states/ (named snapshots), subscriptions/*.raw (per-source cache), rule_sets/*.srs (SRS cache), logs/. Разделение интенциональное.

---

# Ключевые файлы для навигации

- `docs/ARCHITECTURE.md` — карта проекта (1556 строк)
- `SPECS/CONSTITUTION.md` — архитектурные инварианты
