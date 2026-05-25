# SPEC 059-F-N — TRAFFIC_PROFILER

**Status:** New (N)
**Type:** Feature (F)
**Inspired by:** LxBox §044 «Per-app traffic profiler» (mobile counterpart, v1.7.0 / v1.8.0).
**Не меняет:** sing-box config.json format; state.json schema.

---

## Цель

Дать на desktop инструмент realtime-диагностики: «куда какой process содиняется, через какие домены/IP/порты, сколько байт передал и как давно открыто соединение». Чтобы ответить на вопросы вида:

- «Почему Slack/Telegram/браузер ходит мимо VPN?» — увидеть какой outbound выбрал router для конкретного процесса
- «Куда стучит этот закрытый приложение?» — privacy-аудит
- «Почему X не открывается через VPN» — увидеть DNS chain, CNAME-таргет, через какой outbound идёт
- «Что вообще сейчас происходит на сетевом уровне?» — discovery / overview mode

На LxBox такой инструмент уже есть и оказался основным diagnostic-tool, ускоряющим debug VPN routing'а в 30–50 раз. Переносим концепт на desktop.

## Скоп (что делаем)

**Отдельное окно «Traffic Profiler»**, запускаемое кнопкой с вкладки **Diagnostics**. Singleton — повторный клик focus'ит уже открытое окно. Внутри окна два view'а:

1. **Live** (system-wide, discovery mode) — стрим всех DNS/TCP/UDP событий sing-box'а в realtime. Фильтрация: kind, search, process name. Без явного recording — окно открыто = видишь поток.

2. **Per-process recording** — pick process → ▶ START → видишь только его трафик с агрегатами по доменам/IP/connections. ⏹ STOP финализирует session в ring-buffer последних 5.

Каждый event обогащён: process_path (если sing-box детектит owner'а через `find_process`), domain (если DNS / SNI sniffed), CNAME chain, ip, port, outbound chain, bytes ↑↓, duration.

**Почему отдельное окно, а не tab:**

- Диагностика — secondary workflow. Юзер открывает на 5-10 минут когда что-то ломается, а не держит постоянно.
- Окно можно расположить рядом с проблемным приложением (например, Slack слева, Traffic справа) — tab внутри Configurator'а такого не даст.
- Configurator wizard ещё может быть открыт параллельно — Traffic не конкурирует за tab-bar real estate.
- Закрытие окна = stop live capture (см. §"Lifecycle"), не зависит от закрытия app'а.

## Что НЕ делаем (out of scope MVP)

- HTTP-уровневую инспекцию (URL/headers/method) — sing-box работает на L4, не L7
- Packet capture / pcap — у нас есть routing engine, этого достаточно
- Differential mode (compare session A vs B) — на post-MVP
- Block / Add-to-rule inline actions из profiler view — на post-MVP
- TLS fingerprinting (JA3/JA4) — sing-box capability ещё не expose'ит
- Persistence sessions через restart — in-memory only (как в LxBox)

---

## Адаптация LxBox §044 под desktop

| Аспект | LxBox (mobile/Android) | Desktop singbox-launcher | Решение |
|---|---|---|---|
| Process keying | Package name `ru.tinkoff.investing` | Executable path `/Applications/Slack.app/.../Slack` | Используем canonical executable path; UI display name = basename или Info.plist `CFBundleName` (mac) / VERSIONINFO `FileDescription` (win) / `.desktop` (linux) |
| Process detection | sing-box `find_process: true` + UID resolver | `find_process: true` работает идентично — отдаёт `metadata.processPath` | Шаблон по умолчанию должен иметь `route.find_process: true` (см. §Pre-requisites) |
| Secondary apps (Android WebView) | Multi-pick secondary packages | Не нужно — каждый desktop process отдельный | Single-pick per session |
| App picker icons | Android PackageManager + AppInfoCache | macOS NSWorkspace `iconForFile`; win `SHGetFileInfo`; linux `.desktop`/freedesktop icon lookup | Best-effort через platform code; fallback на generic file icon |
| Log stream source | In-app `ClashLogPump` (Flutter notifier) | sing-box.log file (Go process) | Tail `bin/logs/sing-box.log` через `fsnotify` (rename-rotation safe) или периодический `os.Open + Seek + ReadFrom` |
| Live event push | Server-Sent Events (Flutter http) | Go channel + Fyne `fyne.Do()` UI thread schedule | `chan TrafficEvent` буфер N=256, goroutine drainer → UI |
| Verbose toggle | Setting `vars.log_level=debug` через Debug API + reload | Edit `vars.log_level` через configurator + reload sing-box | Кнопка «🔬 Debug DNS» в toolbar Traffic Profiler окна — на ON делает `vars[log_level] = debug` + рестарт; на OFF revert |
| Recording indicator | ⚡ chip на HomeScreen + Stats tab title | ⚡ badge на кнопке «Traffic Profiler» в Diagnostics tab когда recording active; ⚡ в title окна когда оно открыто | Update button label + window title через TrafficProfiler listener |
| Connection close trigger | Clash API DELETE `/connections/<id>` (есть в LxBox) | Не в MVP скоупе (можно добавить как secondary feature) | Post-MVP |
| Polling cadence | Clash `/connections` poll 1s + log-stream | Идентично — `/connections` poll 1s + log tail | Same |

## Pre-requisites

1. **`route.find_process: true`** в `bin/wizard_template.json` (уже стоит — см. `wizard_template.json:433`). Detection вычисляется per-application через `readFindProcessFromConfig()` в `ui/traffic_bootstrap.go`: читаем актуальный `bin/config.json` и смотрим `route.find_process`. Если выключено (юзер вручную поправил template / отключил) — Live view рендерит banner «Process detection disabled in template — events will lack process attribution. Enable route.find_process and Save in the wizard.» (см. `ui/traffic/live_view.go:145`). Banner enforcement пассивный — юзер сам идёт в wizard и включает. Auto-fix кнопки нет (не в скоупе MVP).

2. **`experimental.clash_api.external_controller`** должен быть настроен (уже есть в template — `127.0.0.1:9090` + secret). Используется как сегодня для `/proxies`, добавляется new endpoint usage `/connections`.

3. **`log.level=info`** минимум; для DNS chain детектирования нужен `debug`. Без debug DNS-уровневые события могут быть неполные — Traffic Profiler окно показывает баннер «Switch to debug logs for full DNS visibility» с кнопкой включения (см. Verbose toggle).

---

## UX flow

### Где живёт

**Кнопка «Traffic Profiler»** в Diagnostics tab (рядом с «Open log», «Kill sing-box» и прочими). Клик открывает singleton окно «Traffic Profiler» (separate `fyne.Window`). Повторный клик focus'ит уже открытое окно.

Окно живёт независимо от Configurator wizard, может располагаться рядом с проблемным приложением (Slack слева, Traffic справа). Внутри окна — два view'а в tab-bar'е: Live + Per-process.

Recording indicator: пока session active — кнопка «Traffic Profiler» в Diagnostics tab показывается как **«Traffic Profiler ⚡»**, и title окна — **«Traffic Profiler ⚡ Recording · 02:34»**.

### Diagnostics tab — точка входа

```
┌─ Diagnostics ─────────────────────────────────────────────────────────┐
│  Logs                                                                  │
│  [Open launcher log]  [Open sing-box log]  [Open config]              │
│                                                                         │
│  Process                                                               │
│  [Kill sing-box (privileged)]                                          │
│                                                                         │
│  Diagnostics                                                           │
│  [Traffic Profiler ⚡]  ← открывает отдельное окно                     │
│  [Run network test]   [STUN check]                                     │
└────────────────────────────────────────────────────────────────────────┘
```

⚡ badge добавляется к label кнопки когда `profiler.ActiveSession() != nil` — юзер видит recording state даже если Traffic окно скрыто за другими.

### Окно — idle state (no recording)

```
┌─ Traffic Profiler ────────────────────────────────────────────────[—□×]┐
│  [Live]  [Per-process]                                                 │
│                                                                         │
│  ─── Live (system-wide, newest first) ──────────────────────── 🔬 dbg ─│
│  [Search…]  [DNS] [DNS×] [TCP] [TCP·] [UDP]  [Filter by process ▾]    │
│                                                                         │
│  12:34:15  DNS  cdn.t-bank-app.ru → 193.17.93.194                     │
│              Slack.app                                                  │
│  12:34:14  DNS  certs.t-bank-app.ru → 81.222.127.186  ⚠               │
│              Telegram.app          → via vpn-1 → 🇫🇮 Финляндия         │
│  12:34:12  TCP  api.example.com:443                                    │
│              Spotify.app           ↑ 458 B  ↓ 2.1 KB  open 3.2s        │
│  ...                                                                    │
└────────────────────────────────────────────────────────────────────────┘
```

- **Live tab** — discovery mode, видишь поток сразу как открыл окно. Не нужно ничего записывать
- **Per-process tab** — pick process → ▶ START
- Чекбоксы фильтрации kind (multi-select); search-поле — substring match по domain/IP/process; «Filter by process» — bottom panel с checkbox per замеченный process

### Окно — per-process recording

```
┌─ Traffic Profiler ⚡ Recording · 02:34 ────────────────────────[—□×]┐
│  [Live]  [Per-process]                                                 │
│                                                                         │
│  Target: [Slack.app ▾]                       [⏹ STOP]      🔬 dbg     │
│                                                                         │
│  ⏺ Recording · 02:34 · 47 doms · 53 ips · 287 ev                      │
│                                                                         │
│  [Live] [Domains] [IPs] [Connections]                                  │
│                                                                         │
│  ─── Live ─────────────────────────────────────────                   │
│  10:42:15  DNS  cdn.t-bank-app.ru → 193.17.93.194                     │
│              ↳ CNAME cl-ead2c819.edgecdn.ru                            │
│  10:42:15  TCP  cdn.t-bank-app.ru:443                                  │
│              ↳ via direct-out                                          │
│  10:42:14  DNS  certs.t-bank-app.ru → 81.222.127.186 ↗  ⚠            │
│              ↳ CNAME eq09pc7nbi.a.trbcdn.net                          │
│  10:42:14  TCP  certs.t-bank-app.ru:443                            ⚠ │
│              ↳ via vpn-1 → 🇫🇮 Финляндия                              │
│  ...                                                                    │
│                                                                         │
│  ─── Saved sessions ──── (when recording = idle)                       │
│  • Slack.app — 02:34 · 47 doms                  [Open] [Share] [Del]  │
│  • Telegram.app — 00:48 · 12 doms               [Open] [Share] [Del]  │
└────────────────────────────────────────────────────────────────────────┘
```

### Sub-tabs (Per-process mode)

| Sub-tab | Содержимое |
|---|---|
| **Live** | Stream newest-first. Event = ts + kind label + summary + bytes + duration + ⚠ icon if issue. Color-coded kind labels: DNS (tertiary), DNS× (error red), TCP (primary), TCP· (closed, dimmed), UDP (secondary) |
| **Domains** | Aggregated unique domains, sorted by total bytes. Search matches domain / IP / CNAME target. Tap row → expand: CNAME chain, all resolved IPs, outbound chain, first/last seen, ⚠ issues |
| **IPs** | Aggregated unique destination IPs, sorted by bytes. Useful для hostless connections (raw TCP без SNI sniff). Tap IP `↗` → переход на Domains tab с auto-fill search этим IP (cross-IP audit) |
| **Connections** | Timeline per-connection (TCP/UDP open/close). Tap row → expand: CNAME chain, all IPs, sing-box rule that matched, outbound, ⚠ issues |

### Recording toolbar

| Кнопка | Действие |
|---|---|
| `[⏹ STOP]` / `[▶ START]` | Start/stop recording session. Process picker заблокирован пока active |
| `🔬 dbg` | Toggle verbose: set `vars[log_level]=debug` + reload sing-box. На OFF revert. Banner «Verbose logs active — battery/CPU impact» внутри tab'а пока ON |
| `[⋮ Overflow]` | Copy session JSON, Export to file, Clear all sessions, Help |

### Pre-session backfill

Profiler service always holds rolling buffer 60s × ~3000 events (все process'ы, не только target). На `▶ START` — события за last 60s, которые match target process, копируются в session с marker `〽 backfilled from pre-recording`. Решает «юзер видит проблему и только потом нажимает Record — теряет первые секунды».

### Saved sessions

Когда session active нет — внизу Per-process tab показываются последние 5 завершённых. Ring-buffer FIFO. Tap → открыть session в read-only режиме (те же 4 sub-tab'а). Force-stop приложения = все sessions стираются (in-memory only).

---

## Confidence levels (perевод из LxBox §048)

Каждое event имеет confidence — насколько точно мы уверены что это traffic нашего target process'а:

| Level | UI marker | Когда |
|---|---|---|
| `verified` | (no marker) | sing-box лог `router: found process name: <target_path>` явный match |
| `inferred` | 〽 | TCP без `find_process`-match, но к IP, который был resolved через DNS query, attribute'нутый к этому target'у (10s window) |
| `unattributed` | ? (dimmed) | никакая strategy не сработала, событие показано как nearby (только в Live system-wide tab'е) |

Tooltip над badge'ом показывает `matched_via` (как сработала attribution) — debug-info.

---

## Connection issues (⚠ маркеры)

Не статистические аномалии, а конкретные diagnostic-сигналы. Два locale-агностичных типа (как в LxBox после §048 cleanup):

| Issue | Условие | Use case |
|---|---|---|
| **DNS timeout** | sing-box лог `dns: exchange failed for <host>: <reason>` (context deadline exceeded и пр.) | Network-level problem, DNS server недоступен |
| **TCP RST early** | TCP conn closed в течение 1s, ↑0 ↓0 байт | Firewall RST / TLS handshake fail / block / unreachable |

Отвергнуто (LxBox прошёл этот путь и выпилил):
- ~~`geoMismatch`~~ — RU-bias via emoji parsing в outbound name; правильная реализация требует user-config home-locale; post-MVP
- ~~`unusualPort`~~ — arbitrary whitelist, шум для torrent / steam / corp
- ~~`badLatency`~~ — dead code на mobile, не нужен

В JSON session events: `events[i].issues: [{kind, description}]`.

---

## Архитектура

### Сервис `TrafficProfiler` (`internal/traffic/profiler.go`)

Singleton, lifetime app'а. Доступ через `traffic.GetInstance()` (`singleton.go` — `sync.Once`-guarded). Поднимается на app startup (см. `ui/traffic_bootstrap.go::startProfiler`), runs до app quit.

```go
type TrafficProfiler struct {
    mu sync.Mutex

    // pipeline pieces (nil until Start)
    poller *ConnPoller   // Clash API /connections 1s poll
    tailer *LogTailer    // sing-box.log file tail + rotation detection

    // rolling buffer of recent events (all processes, system-wide)
    roll []TrafficEvent  // 60s window, max 3000

    // active session + ring of completed
    active    *Session
    completed []*Session  // FIFO max 5

    // cross-source join state — sing-box conn_id is the key
    connProcessMap map[string]string          // conn_id → process_path (from `router: found process name`)
    dnsAccum       map[string][]string        // conn_id → CNAME chain in arrival order
    dnsByIP        map[string]dnsAttribution  // dest IP → recent DNS + process (10s inferred window)

    // subscribers for live UI streaming — index-keyed for cheap Unsubscribe
    subs    map[int]chan TrafficEvent
    nextSub int

    // lifecycle hooks (window title timer / button label badge)
    onSessionChange func()

    stopCh chan struct{}
    bgCtx    context.Context
    bgCancel context.CancelFunc
}

// Lifecycle
func NewTrafficProfiler() *TrafficProfiler
func GetInstance() *TrafficProfiler  // singleton accessor
func (p *TrafficProfiler) Start(cfg StartConfig, logPath string, httpClient *http.Client)
func (p *TrafficProfiler) Stop()
func (p *TrafficProfiler) SetOnSessionChange(fn func())

// Sessions
func (p *TrafficProfiler) StartSession(processPath string, verbose bool) (*Session, error)
func (p *TrafficProfiler) StopSession() *Session
func (p *TrafficProfiler) ActiveSession() *Session
func (p *TrafficProfiler) CompletedSessions() []*Session
func (p *TrafficProfiler) DeleteSession(id string)
func (p *TrafficProfiler) ClearAll()

// UI feed
func (p *TrafficProfiler) Subscribe() (id int, ch <-chan TrafficEvent)
func (p *TrafficProfiler) Unsubscribe(id int)
func (p *TrafficProfiler) Snapshot(lastN time.Duration) []TrafficEvent  // for late-join UI

// Constants — internal limits
const (
    rollingBufferWindow = 60 * time.Second
    rollingBufferMax    = 3000
    dnsInferWindow      = 10 * time.Second  // inferred-confidence DNS→IP window
)
```

Идёт ровно один tailer лог-файла и одна 1-сек poll-loop `/connections`. Goroutines управляются через `bgCtx/bgCancel`; `Stop()` cancel'ит контекст и ждёт graceful shutdown через `stopCh`.

### Data model (`internal/traffic/types.go` + `session.go`)

```go
// EventKind — colored row label + filter chip identifier
type EventKind string
const (
    EventDNSResolve   EventKind = "DNSResolve"
    EventDNSFail      EventKind = "DNSFail"
    EventTCPOpen      EventKind = "TCPOpen"
    EventTCPClose     EventKind = "TCPClose"
    EventUDPOpen      EventKind = "UDPOpen"
    EventUDPClose     EventKind = "UDPClose"
    EventRouterMatch  EventKind = "RouterMatch"  // not surfaced in UI; feeds Rule label
)

// Confidence — attribution strength
type Confidence string
const (
    ConfVerified     Confidence = "verified"      // router_log match
    ConfInferred     Confidence = "inferred"      // prior_dns_10s match — marker 〽
    ConfUnattributed Confidence = "unattributed"  // none — marker ?, Live system-wide only
)

// IssueKind — ⚠ diagnostic markers (kept short, locale-agnostic)
type IssueKind string
const (
    IssueDnsTimeout  IssueKind = "DnsTimeout"
    IssueTcpRstEarly IssueKind = "TcpRstEarly"
)

type ConnectionIssue struct {
    Kind        IssueKind
    Description string
}

type TrafficEvent struct {
    TS             time.Time
    Kind           EventKind
    ConnID         string             // sing-box conn id; "" for events without one
    ProcessPath    string             // canonical executable path; "" if unattributed
    ProcessName    string             // short display name from Clash API metadata.process
    Confidence     Confidence
    MatchedVia     string             // "router_log" / "prior_dns_10s" / "" — debug aid
    Domain         string             // sniffed/resolved hostname; "" for hostless
    CnameChain     []string           // accumulated CNAME chain ending in A-record IP
    IP             string
    Port           int
    Network        string             // "tcp" / "udp"
    OutboundChain  []string           // leaf→root order, from Clash API `chains`
    Rule           string             // matched router rule name
    UpBytes        int64
    DownBytes      int64
    Duration       time.Duration      // only for *Close events
    Issues         []ConnectionIssue
    RawLogLine     string             // debug only
    Backfilled     bool               // copied from pre-session rolling buffer (marker 〽)
}

func (e TrafficEvent) HasIssue(k IssueKind) bool  // helper for issue chip widget

type Session struct {
    mu sync.RWMutex

    ID                 string       // formatted StartedAt: "20060102T150405" (max 6 alive at once)
    TargetProcess      string       // canonical executable path the user picked
    StartedAt          time.Time
    FinishedAt         *time.Time
    WasVerbose         bool
    VerboseToggleTimes []time.Time  // mid-session toggles for forensics

    events        []TrafficEvent  // private, accessed via Events()/Append()
    eventsDropped int             // counter shown in UI footer
}

// Caps — see SPEC §"Edge cases & limits"
const (
    maxEventsPerSession  = 50000
    maxSessionAge        = 3 * time.Hour
    maxCompletedSessions = 5
)

func NewSession(target string, verbose bool) *Session
func (s *Session) Append(e TrafficEvent)        // overflow-safe; evicts head on age + count caps
func (s *Session) Events() []TrafficEvent       // returns a copy (caller iteration without locks)
func (s *Session) EventsDropped() int
func (s *Session) Finalize()                    // idempotent
func (s *Session) Duration() time.Duration      // wall-clock (or since-start if active)

// Aggregates — recomputed on each UI tick from events (no cached fields, simpler than indexing)
func (s *Session) AggregateDomains() []DomainStats
func (s *Session) AggregateIPs() []IPStats
func (s *Session) AggregateConns() []ConnRecord

type DomainStats struct {
    Domain      string
    Connections int
    UpBytes     int64
    DownBytes   int64
    FirstSeen   time.Time
    LastSeen    time.Time
    IPs         []string
    Outbounds   []string
    Issues      []ConnectionIssue
    CnameChain  []string  // first observed chain
}

type IPStats struct {
    IP          string
    Port        int
    Connections int
    UpBytes     int64
    DownBytes   int64
    Domain      string    // first observed domain that resolved to this IP
    Outbounds   []string
}

type ConnRecord struct {
    ConnID    string
    Domain    string
    IP        string
    Port      int
    Network   string
    OpenedAt  time.Time
    ClosedAt  *time.Time
    UpBytes   int64
    DownBytes int64
    Outbounds []string
    Rule      string
    Issues    []ConnectionIssue
}
```

**Aggregation strategy:** не храним cached индексы (`ipCache`/`connCache` etc) — пересчитываем on-demand из `events` slice при каждом UI refresh tick. Для ≤50k events это microseconds; гораздо проще чем поддерживать invalidation.

### Data sources

**1. Clash API `/connections` (poll 1s)**

```
GET http://127.0.0.1:9090/connections
Authorization: Bearer <secret>

Response:
{
  "downloadTotal": 12345, "uploadTotal": 6789,
  "connections": [
    {
      "id": "<uuid>",
      "metadata": {
        "network": "tcp",
        "type": "TLS",
        "host": "api.example.com",
        "destinationIP": "1.2.3.4",
        "destinationPort": "443",
        "processPath": "/Applications/Slack.app/Contents/MacOS/Slack",
        "process": "Slack"
      },
      "upload": 458, "download": 2058,
      "start": "2026-05-24T12:34:00Z",
      "chains": ["vless-server", "🇫🇮 Финляндия", "vpn-1"],
      "rule": "domain_suffix",
      "rulePayload": "example.com"
    }
  ]
}
```

Diff с предыдущим snapshot:
- Новый id → emit `TCPOpen` / `UDPOpen`
- Существующий id → update bytes (without emit if no kind-change)
- Исчез id → emit `TCPClose` / `UDPClose` с duration = now - start

**2. sing-box.log tail (`internal/traffic/logtail.go` + `parser.go`)**

Tail `bin/logs/sing-box.log`. Парсим regex'ами в `internal/traffic/parser.go` (`parser_test.go` — zoo of log samples):

```
[id] dns: exchanged|cached <type> <domain>. -> <ip>|<cname>.
[id] dns: exchange failed for <host>: <reason>
[id] router: found process name: <process_path>
[id] router: match[<rule>] => route(<outbound>)
[id] inbound/tun[<id>]: outbound connection to <host>:<port>
```

Patterns могут разойтись между minor sing-box releases — log format не stable. Регекс'ы изолированы в одном файле + unit-тесты, регрессию ловим до релиза (LxBox §044 impl bug #2/#3 — `dns: cached` пропускался, length-diff scan ломался при ring overflow).

**Rotation detection** — через file identity (inode на Unix, FileIndex/VolumeSerialNumber на Windows). Платформенный код в `internal/traffic/inode_unix.go` / `inode_windows.go`. На каждом read tick сравниваем identity текущего открытого FD с identity файла по path: если разошлись (mv rotate) — reopen с начала; если truncate (тот же FD, размер стал меньше нашего offset'а) — seek to 0. `fsnotify` не используется (он шумит на write events и не покрывает все edge cases переименований на macOS).

**3. Cross-source join**

Conn-ID — это ключ. Sing-box логирует `[<conn_id>]` префиксом в большинстве строк. Clash API возвращает `id` тот же. Profiler:
- На DNS строку — кладёт в `_dnsAccumulator[conn_id]`
- На `router: found process name` — кладёт в `_connProcessMap[conn_id]`
- На Clash poll: matches active conn ↔ rolling buf events

CNAME chain reconstruction: первое `dns: exchanged` для conn_id — это domain юзера; последующие CNAME-targets копятся в chain, A-record IP terminates. Без этого finder в Domains tab показывал бы CDN-домен, а не оригинальный (LxBox §044 impl bug #4).

### Verbose toggle

Реализован в `ui/traffic_bootstrap.go` через три функции:

- `ReadCurrentLogLevel(ac) (level string, set bool, err error)` — читает `vars[log_level]` из активного state; fallback на template default `"warn"` если в vars не задано.
- `ApplyLogLevelAndReload(ac, level) error` — устанавливает `vars[log_level]` и инициирует rebuild config.json + sing-box restart через `ac.ProcessService.KillForRestart()` (sing-box watcher автоматически restart'ит с new config). Никакого вымышленного `core.ReloadSingBox()` — используем существующий KillForRestart pipeline.
- `ConfirmAndApplyLogLevel(ac, parent, level, done)` — обёртка с user-confirmation dialog: «Reloading sing-box — active connections will reset. Continue?». UI вызывает этот wrapper, не raw `ApplyLogLevelAndReload`, чтобы юзер был предупреждён.

Эти три функции передаются в `uitraffic.WindowDeps`: `ConfigReader` / `ConfigWriter` / `ConfigConfirmApply`. Window-side toolbar дёргает только их — knows nothing про `ac.ProcessService` детали.

Toggle ON flow:
1. `ConfigReader()` → читаем текущий `log_level`, сохраняем в state toolbar'а как `_savedLogLevel`
2. `ConfigConfirmApply("debug", win, done)` → показывает confirmation dialog, на confirm:
   - Set `vars[log_level]=debug`
   - Rebuild config.json + KillForRestart sing-box
3. Banner «Verbose logs active — battery/CPU impact» рендерится внутри окна
4. Profiler начинает захватывать debug-уровень DNS events (cached / chain traversal)

Toggle OFF: revert `log_level` к `_savedLogLevel` через `ConfigConfirmApply` → banner hides.

---

## UI компоненты

Файлы в `ui/traffic/`:

- `window.go` — `Manager` (window-singleton lifecycle wrapper) + `WindowDeps` (dependency bundle) + `formatEventRow` / `formatBytes` helpers
- `live_view.go` — system-wide stream + фильтры; subscribe на profiler
- `per_process_view.go` — target picker + ▶/⏹ + 4 sub-tab'а (Live/Domains/IPs/Connections)
- `process_picker.go` — диалог выбора процесса (список с executable path + display name через `internal/platform/proclist.go`)
- `toolbar.go` — window toolbar с verbose toggle, overflow menu, banner стек

Bootstrap в `ui/traffic_bootstrap.go`:
- `startProfiler(ac)` — запускает singleton TrafficProfiler на старте app, передаёт log path + http client
- `trafficWindowManager(ac, parentRefresh)` — lazy creation Manager singleton с `sync.Once` (`trafficManagerOnce`); каждый клик кнопки делает `SetDeps()` с актуальным `ac` для freshness
- `ReadCurrentLogLevel` / `ApplyLogLevelAndReload` / `ConfirmAndApplyLogLevel` — verbose toggle internals
- `readFindProcessFromConfig(path)` — banner driver

### Window singleton — `uitraffic.Manager` pattern

Внешний по отношению к `UIService` объект (НЕ поле `UIService.TrafficWindow`). Один `Manager` per app process; `trafficManagerOnce sync.Once` в bootstrap'е гарантирует одну instance.

```go
package traffic // package alias `uitraffic` from ui/

type WindowDeps struct {
    App      fyne.App
    Profiler *tprof.TrafficProfiler

    // verbose toggle wiring
    ConfigReader       func() (logLevel string, ok bool)
    ConfigWriter       func(level string) error
    ConfigConfirmApply func(level string, parent fyne.Window, done func())

    // banner state
    FindProcessEnabled func() bool   // nil → assume true
    SingBoxRunning     func() bool

    // Diagnostics tab callback: re-render button label after ⚡ badge state change
    ParentRefresh func()
}

type Manager struct {
    mu          sync.Mutex
    win         fyne.Window
    deps        WindowDeps
    titleStopCh chan struct{}  // 1-Hz title refresh ticker
}

func NewManager(deps WindowDeps) *Manager
func (m *Manager) SetDeps(deps WindowDeps)
func (m *Manager) Show()         // create or focus singleton, UI-thread only
func (m *Manager) IsRecording() bool  // for Diagnostics button label
```

`Manager.Show()`:
1. Если `win != nil` → `win.Show() + RequestFocus()`. No deadlock guard needed — Show всегда вызывается из UI thread (Diagnostics button OnTap).
2. Иначе `build()`: создаём window, wrap'аем content в `fynetooltip.AddWindowToolTipLayer` (ttwidget tooltips otherwise warn «no tooltip layer»), tab-bar с Live / Per-process, toolbar сверху, hookup `SetOnClosed` для cleanup + 1-Hz title refresh timer.

`build()` имеет важный нюанс: `m.refreshTitle()` дёргает `ParentRefresh()` → `fyne.Do(...)`. Вызывать `fyne.Do` из UI thread (а build уже на UI thread) = deadlock в Fyne 2.7. Поэтому initial refresh откладывается в `go func() { fyne.Do(...) }()`.

`SetOnClosed`:
- Останавливает live/per-process subscribers (`live.Stop()` + `perProcess.Stop()`)
- `m.win = nil` → следующий Show создаст заново
- `stopTitleTimer()` → останавливает 1-Hz ticker
- `deps.ParentRefresh()` → пере-render кнопки в Diagnostics (badge может остаться ⚡ если recording continues)

**Profiler не останавливается** на window close — singleton TrafficProfiler работает all-app-lifetime; rolling buffer + active session continue в background.

### Точка входа — Diagnostics tab

`ui/diagnostics_tab.go:302` — button «Traffic Profiler ⚡?» в Diagnostics section (рядом с Kill sing-box):

```go
btn := widget.NewButton("Traffic Profiler", func() {
    mgr := trafficWindowManager(ac, func() { refreshBtnLabel() })
    mgr.Show()
})
// refreshBtnLabel toggles "Traffic Profiler" ⟷ "Traffic Profiler ⚡"
// based on mgr.IsRecording()
```

### Process picker (`internal/platform/proclist.go`)

Build-tag separated impl:
- `proclist_darwin.go` — `NSWorkspace.runningApplications` через cgo для GUI apps; CLI tools — fallback на `ps`
- `proclist_windows.go` — `EnumProcesses` + `QueryFullProcessImageName` via win32 API
- `proclist_linux.go` — walk `/proc/<pid>/exe` + readlink; display name из `comm`
- `proclist_unix_test.go` — smoke test через faking `/proc` fixtures

Icon: на MVP — generic file icon (cgo для NSWorkspace iconForFile не trivial, отложено).

### Recording indicator

Два места:

1. **Кнопка в Diagnostics tab** — label `"Traffic Profiler ⚡"` когда `profiler.ActiveSession() != nil`, иначе `"Traffic Profiler"`. Update через TrafficProfiler subscriber на session start/stop.

2. **Title окна** — `"Traffic Profiler ⚡ Recording · 02:34"` при active session (с таймером, обновляется раз в секунду через `fyne.Do`). При idle — `"Traffic Profiler"`.

Update wiring: TrafficProfiler exposes channel событий lifecycle (start/stop session, tick), UI subscriber'ы (Diagnostics button + window title) обновляются через `fyne.Do`. Recording продолжается даже когда окно закрыто — на следующем re-open юзер видит активную session.

### Lifecycle

- **App start** → `startProfiler(ac)` (`ui/traffic_bootstrap.go`) поднимает singleton TrafficProfiler: rolling buffer + log tail + Clash poll always-on. Окно не показывается.
- **Юзер жмёт «Traffic Profiler» в Diagnostics** → `trafficWindowManager(ac, refresh).Show()` → `Manager.build()` создаёт singleton window → live/per-process view'ы subscribe на profiler.
- **Юзер закрывает окно (X)** → `SetOnClosed` → live/per-process Stop (unsubscribe) → `win = nil`. Profiler singleton продолжает работать (если active session — recording продолжается; rolling buffer не останавливается, т.к. он дешёвый и нужен для re-open UX).
- **App quit** → `Profiler.Stop()` через `bgCancel()` → graceful shutdown goroutines. Все sessions wiped (in-memory only).

Отличие vs «Live tab всегда on while window open» (LxBox): мы оставляем background capture (rolling buffer + clash poll) всегда on на app lifetime — это дёшево и решает «открыл окно, увидел уже накопленное» UX. CPU/battery impact мизерный без verbose toggle (regex-парсинг ~50-200 строк/сек).

---

## Edge cases & limits

| Случай | Поведение |
|---|---|
| `find_process: false` в config'е | Live + Per-process tab показывают banner с инструкцией «Enable process detection in wizard template». Profiler работает но process_path везде пустой → все events confidence=unattributed |
| Process detection misses (system-level, kernel TCP) | Fallback: inferred attribution через recent DNS IP (10s window), отметка 〽 |
| Verbose toggle включается/выключается mid-session | Sing-box reload, active connections рвутся. Warning dialog при toggle. Session continues с новым conn-id space, в meta фиксируется `verbose_toggled_at`. |
| Session events overflow (>50000 ev или >3h) | Drop oldest, counter `events_dropped` в session meta, виден в UI footer |
| Memory pressure | Max 6 sessions concurrent (1 active + 5 completed). Old auto-evict'ятся (FIFO) |
| Sing-box restart mid-session | Auto-finalize partial DNS chains. Session continues с новым conn-id space. UI notification «Sing-box reloaded — recording continues» |
| App quit / kill | Все in-memory sessions стираются. Persist принципиально не делается — Share/Copy через overflow menu если нужно сохранить |
| Log file rotation (singbox-launcher's own logrotate) | `fsnotify` detect, reopen без потерь — буфер log-tail'а удерживает ~500ms |
| Clash API недоступен (sing-box не запущен) | Live tab показывает «Sing-box not running»; Per-process pick disabled |
| Hostless connection (raw TCP без SNI / без DNS resolve) | Event пишется с `Domain=""`, `IP+Port` only. В Live event row отображается как `[<ip>]:<port>`. Видим в IPs sub-tab, в Domains tab отсутствует |
| Conn-id collision после sing-box reload | sing-box генерит conn-id заново, реюз практически невозможен; на reload `_dnsAccumulator` и `_connProcessMap` очищаются |

---

## Acceptance

1. Кнопка «Traffic Profiler» в Diagnostics tab открывает отдельное singleton окно. Повторный клик focus'ит существующее.
2. Окно содержит Live + Per-process views (как tabs внутри окна).
3. Live view показывает stream system-wide events realtime без recording.
4. Per-process: pick process → ▶ START → recording active → 4 sub-tab'а (Live/Domains/IPs/Connections) работают.
5. Process picker показывает запущенные процессы с display name + executable path + icon (best-effort per OS).
6. DNS chain reconstruction: CNAME-targets копятся в chain, оригинальный domain виден в Domains sub-tab (не CDN).
7. Connection issues (DnsTimeout, TcpRstEarly) детектируются и помечаются ⚠ + tooltip.
8. Verbose toggle: меняет `vars[log_level]=debug`, reload sing-box, revert на OFF, banner внутри окна.
9. Pre-session backfill: 60s rolling buffer событий target process'а копируется в session на START.
10. Ring-buffer 5 completed sessions. Force-stop = все стираются (in-memory only).
11. Export session JSON через overflow menu (copy to clipboard + save to file).
12. Recording indicator: ⚡ badge на кнопке «Traffic Profiler» в Diagnostics tab + в title окна (с таймером) когда session active.
13. Окно можно закрыть — active session продолжается в background, на повторном open юзер видит её active.
14. Process detection отключен → banner с инструкцией внутри окна.
15. Process attribution через `find_process` работает на macOS и Windows. На Linux при `find_process: true` тоже работает (verify).
16. Fyne UI thread safety: все event push'ы в UI через `fyne.Do()`. Goroutine профайлера не блокирует main thread. Tooltip layer (`fynetooltip.AddWindowToolTipLayer`) подключён к окну для ttwidget виджетов.
17. Memory cap: 50000 events / 3h sliding window. Counter `events_dropped` виден в footer.

---

## План фаз

1. **Phase 1 — Data sources**
   - `internal/traffic/clash_connections.go` — Clash API `/connections` poll + diff
   - `internal/traffic/logtail.go` — `bin/logs/sing-box.log` tail через fsnotify
   - `internal/traffic/parser.go` — regex parsing DNS/router/connection lines + unit-тесты на log samples

2. **Phase 2 — Profiler service**
   - `internal/traffic/profiler.go` — singleton `TrafficProfiler` + Session lifecycle
   - Cross-source join: conn-id ↔ process map, DNS accumulator
   - Subscribe/snapshot API
   - Connection issue classifier (DnsTimeout, TcpRstEarly)
   - Tests: lifecycle, log parsing, attribution, issues

3. **Phase 3 — Process listing platform code**
   - `internal/platform/proclist_darwin.go` / `_windows.go` / `_linux.go`
   - Enumerate running processes with path + display name + icon (path к icon файлу)
   - Tests: smoke-test на macOS host

4. **Phase 4 — UI: window shell + Live system-wide view**
   - `ui/traffic/window.go` — singleton `Manager` (window-lifecycle wrapper) + `WindowDeps` bundle. Открывается через `trafficWindowManager(ac, refresh).Show()` из bootstrap. Fynetooltip layer подключён.
   - Button «Traffic Profiler» в `ui/diagnostics_tab.go` (Diagnostics section).
   - Окно: `container.AppTabs` с двумя tab'ами (Live / Per-process).
   - Live: stream rendering через Subscribe(), filter chips, search, newest-first scroll.

5. **Phase 5 — UI: Per-process recording view**
   - Process picker dialog
   - ▶/⏹ button + recording status
   - 4 sub-tab'а (Live/Domains/IPs/Connections) с aggregated views
   - Saved sessions list + open/share/delete

6. **Phase 6 — Verbose toggle + overflow menu**
   - Toggle wired в `vars[log_level]` через configurator
   - Reload sing-box flow
   - Banner widget
   - Overflow menu: copy/export/clear/help

7. **Phase 7 — Recording indicator + edge cases**
   - ⚡ badge на button label в Diagnostics tab + в title окна
   - Singleton focus-on-reopen behavior
   - find_process=false banner
   - Sing-box-not-running banner
   - Memory cap counter

8. **Phase 8 — Build + tests + reinstall + docs**
   - `docs/TRAFFIC_PROFILER.md` user guide
   - Smoke-test на live machine: запустить Slack/Chrome/Telegram, проверить attribution
   - Update RELEASE_NOTES/upcoming.md

## Риск

**Средний.** Самое рискованное — log parsing: sing-box log format не stable между minor releases. Mitigation: parser изолирован в одном файле + zoo of log samples в `testdata/`; на regression теста ловим до релиза.

Второе по риску — process listing на разных OS: каждая платформа требует своего impl. Mitigation: best-effort + fallback на «pick by path» (вручную ввести путь к executable).

## Final decisions (open vs LxBox)

| # | Вопрос | Decision | Comment |
|---|---|---|---|
| 1 | Где живёт UI | **Отдельное окно**, запуск кнопкой из Diagnostics tab | Diagnostics — secondary workflow, окно можно расположить рядом с проблемным app'ом; не конкурирует за tab-bar с Configurator wizard |
| 2 | Min sing-box version | Текущий pinned (1.13.11) | Лог-формат текущий, если sing-box обновится — обновим parser regex'ы |
| 3 | macOS process icon source | NSWorkspace iconForFile (через cgo) | На MVP — fallback to generic icon если cgo не trivial |
| 4 | Persist sessions | **No** | In-memory only как в LxBox |
| 5 | Session JSON schema version | **No version** | In-memory only, no import path |
| 6 | Recording across sing-box restart (`KillForRestart`) | **Continue with new conn-id space** | Auto-finalize partial; session не теряется |
| 7 | Background capture (rolling buffer + clash poll) lifetime | **Always on while app runs** | Дёшево (~50-200 lines/sec parsing); решает «открыл окно → видишь уже накопленное» UX. Recording продолжается даже при закрытом окне |
| 8 | Pre-session backfill window | **60s × ~3000 ev** | Как в LxBox §048 |
| 9 | Completed sessions cap | **5** | Как в LxBox |
| 10 | Window singleton policy | **Один экземпляр**, повторный клик focus'ит | Не плодим окна; recording state виден после reopen |

## Не в скоупе (post-MVP)

- Inline actions: Add to direct / Block domain / Make preset from selected
- Differential mode (compare session A vs B)
- HTTP-level inspection (URL/headers) — sing-box capability missing
- TLS fingerprinting — sing-box capability missing
- Persist sessions через app restart
- Community export library (share .json sessions)
- Latency / RTT per-domain — только bytes на MVP
