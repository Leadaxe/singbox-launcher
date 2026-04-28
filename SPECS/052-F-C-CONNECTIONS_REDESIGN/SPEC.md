# SPEC 052-F-N — CONNECTIONS_REDESIGN

**Status:** New (N)
**Type:** Feature (F)
**Depends on:** SPEC 045 (state-config decoupling).
**Bump:** state.json v4 → v5.

## Цель

Переписать `state.json` так, чтобы подписки были first-class-сущностями со
своей метаданными, raw-body cache'ировался отдельно, разные типы источников
(subscription vs single server) явно разделялись, а корень файла стал
плоским и понятным.

Текущая структура — наследие `@ParserConfig` блока (выпилен в SPEC 045)
и legacy миграций (selectable_rule_states, rules_library_merged):
двойная вложенность `parser_config.ParserConfig.{...}`, всё в одной куче,
никакой metadata подписок (profile_title, traffic quota, expiry) не
сохраняется, raw body теряется после Update.

## Не-цели

- **Не** меняем sing-box config.json layout (там `outbounds`, `route`,
  `dns` остаются как есть; меняем только intent в state.json).
- **Не** меняем `OutboundConfig` структуру (Tag/Type/Options/AddOutbounds/
  Wizard — без изменений).
- **Не** трогаем `core/build.BuildConfig` существенно (только адаптеры
  «новый Source → старый ProxySource» для парсера).
- **Не** делаем UI-revolution: визард продолжает работать, под капотом
  модель адаптируется.

---

## Финальный layout `state.json` (v5)

```jsonc
{
  "meta": {
    "version":    5,
    "comment":    "",
    "created_at": "2026-04-28T10:00:00Z",
    "updated_at": "2026-04-28T11:23:45Z"
  },

  "connections": {
    "sources":   [...],   // источники нод (subscription + server)
    "outbounds": [...],   // глобальные группы (selector / urltest)
    "defaults":  { "reload": "4h", "max_nodes": 3000 }
  },

  "config_params": [...],
  "custom_rules":  [...],
  "vars":          [...],
  "dns_options":   null
}
```

### Что выпилено из v4

| Поле | Куда делось |
|---|---|
| `id` (top-level) | Удалено. Snapshot-имена живут только в имени файла `bin/wizard_states/<name>.json` |
| `parser_config` (внешняя обёртка) | Раскрыт; содержимое в `connections` |
| `parser_config.ParserConfig` (внутренняя обёртка) | Удалена двойная вложенность |
| `parser_config.parser.last_updated` | Перенесено per-subscription в `meta.last_fetched_at` |
| `rules_library_merged` | Флаг одноразовой миграции, более не нужен |
| `selectable_rule_states` | Не читаем при load, не пишем при save |

### Что добавлено

- `meta` контейнер для версии/timestamp'ов
- `connections` контейнер (sources / outbounds / defaults)
- Per-source metadata (headers, fetch history, preview)

---

## Source — два типа

```go
type Source struct {
    // ─── identity (общее) ───────────────────────────────────────
    ID                string     `json:"id"`                  // ULID
    Type              SourceType `json:"type"`                // "subscription" | "server"
    Enabled           bool       `json:"enabled"`             // default true
    Label             string     `json:"label,omitempty"`
    ExcludeFromGlobal bool       `json:"exclude_from_global,omitempty"`

    // ─── type=subscription only ─────────────────────────────────
    URL                     string              `json:"url,omitempty"`
    Skip                    []map[string]string `json:"skip,omitempty"`
    Tag                     TagSpec             `json:"tag,omitempty"`
    Outbounds               []OutboundConfig    `json:"outbounds,omitempty"`
    ExposeGroupTagsToGlobal bool                `json:"expose_group_tags_to_global,omitempty"`
    Update                  *UpdateSpec         `json:"update,omitempty"`
    MaxNodes                int                 `json:"max_nodes,omitempty"`
    Meta                    *SubscriptionMeta   `json:"meta,omitempty"`

    // ─── type=server only ───────────────────────────────────────
    URI string `json:"uri,omitempty"`
}

type SourceType string

const (
    SourceTypeSubscription SourceType = "subscription"
    SourceTypeServer       SourceType = "server"
)

type TagSpec struct {
    Prefix  string `json:"prefix,omitempty"`
    Postfix string `json:"postfix,omitempty"`
    Mask    string `json:"mask,omitempty"`
}

type UpdateSpec struct {
    IntervalHours int   `json:"interval_hours,omitempty"`
    AutoRefresh   *bool `json:"auto_refresh,omitempty"` // nil → true
}
```

**Семантика:**

| | `subscription` | `server` |
|---|---|---|
| Что | URL → HTTP fetch → пачка нод | Один URI → один outbound |
| Tag | префиксует все ноды (`BL:` → `BL:tokyo`, `BL:fra`) | не используется; `Label` это и есть тэг |
| Outbounds local | да (`BL:auto`, `BL:select`) | нет |
| Update timer | да (per-source `update.interval_hours`) | нет |
| Raw body cache | да (`bin/subscriptions/<id>.raw`) | нет |
| Meta | да (см. ниже) | нет |

---

## SubscriptionMeta — runtime данные подписки

```go
type SubscriptionMeta struct {
    // ─── headers (HTTP response + inline #-comments в body) ─────
    ProfileTitle               string    `json:"profile_title,omitempty"`
    ProfileUpdateIntervalHours int       `json:"profile_update_interval_hours,omitempty"`
    SupportURL                 string    `json:"support_url,omitempty"`
    ProfileWebPageURL          string    `json:"profile_web_page_url,omitempty"`
    ContentDispositionFilename string    `json:"content_disposition_filename,omitempty"`
    UserInfo                   *UserInfo `json:"userinfo,omitempty"`

    // ─── fetch history ──────────────────────────────────────────
    URLAtFetch       string `json:"url_at_fetch,omitempty"`     // снимок URL на момент fetch'а
    LastFetchedAt    string `json:"last_fetched_at,omitempty"`  // RFC3339 UTC
    LastStatus       string `json:"last_status,omitempty"`      // "ok" | "err"
    ErrorCount       int    `json:"error_count,omitempty"`      // подряд (resets на success)
    LastErrorMsg     string `json:"last_error_msg,omitempty"`
    HTTPStatusCode   int    `json:"http_status_code,omitempty"`
    RawBodyBytes     int64  `json:"raw_body_bytes,omitempty"`

    // ─── nodes ──────────────────────────────────────────────────
    NodesCountFetched int      `json:"nodes_count_fetched,omitempty"`
    Truncated         bool     `json:"truncated,omitempty"`         // обрезали по max_nodes
    PreviewNodes      []string `json:"preview_nodes,omitempty"`     // первые 50 URI для UI preview
}

type UserInfo struct {
    UploadBytes   int64 `json:"upload_bytes,omitempty"`
    DownloadBytes int64 `json:"download_bytes,omitempty"`
    TotalBytes    int64 `json:"total_bytes,omitempty"`
    ExpireUnix    int64 `json:"expire_unix,omitempty"`
}
```

### Headers контракт (LxBox-совместимый)

Парсим из HTTP response **и** из inline `#header-name: value` в body
(некоторые провайдеры эмитят оба варианта). Stable de facto convention
из V2Ray/Clash/sing-box ecosystem (V2Board, Xboard, Marzban):

| Header | Формат | → Поле |
|---|---|---|
| `subscription-userinfo` | `upload=N; download=N; total=N; expire=UNIX` | `userinfo.{upload_bytes, download_bytes, total_bytes, expire_unix}` |
| `profile-title` | UTF-8 или base64 | `profile_title` |
| `profile-update-interval` | int hours | `profile_update_interval_hours` |
| `support-url` | URL | `support_url` |
| `profile-web-page-url` | URL | `profile_web_page_url` |
| `content-disposition` | `attachment; filename="..."` | `content_disposition_filename` |

Ссылка на ecosystem-doc: https://github.com/Leadaxe/LxBox/blob/main/docs/PROTOCOLS.md#0-subscription-http-headers

---

## Раскладка файлов

```
bin/
├── state.json                ← intent + meta (всегда маленький, ~16KB на подписку)
├── subscriptions/
│   ├── 01J9X...A.raw         ← raw body для каждой type=subscription с successful fetch
│   ├── 01J9X...B.raw
│   └── ...
└── config.json               ← derived: state + raw + template → BuildConfig
```

**Удалено:** `bin/outbounds.cache.json`. Был intermediate parsed cache,
теперь не нужен — Rebuild парсит `.raw` файлы напрямую.

### `bin/subscriptions/<id>.raw`

- Имя файла = `Source.ID + ".raw"` (ULID)
- **Всегда** пишется при `Update success` (atomic `.tmp + Rename`)
- При failed fetch — старый `.raw` **не перезаписывается** (per-source resilience)
- Lazy GC orphan-файлов при следующем Update'е (id не в `state.connections.sources[*].id` → удалить)

### `state.json` size guarantee

- Source(server) ≈ 200 байт (id + type + enabled + label + uri)
- Source(subscription) с meta ≈ 16KB (50 preview-URI × 300 байт + headers + history)
- 5 подписок + 5 серверов = ~85KB
- 50 подписок = ~800KB (всё ещё разумно для переключения снапшотов)

state.json **не зависит** от количества нод в подписках — preview всегда обрезан до 50.

---

## Pipeline (после миграции)

### Update — pure cache-refresh

```
для каждого source where type=subscription и enabled:
    fetch HTTP (with timeout, retry)
    parse headers (HTTP + inline #-comments) → meta
    write atomic: bin/subscriptions/<id>.raw
    update state.connections.sources[i].meta:
        - заполнить headers
        - last_fetched_at = now
        - last_status = "ok"
        - error_count = 0
        - http_status_code = 200
        - raw_body_bytes = len(body)
        - nodes_count_fetched = first-pass URI count
        - preview_nodes[] = first 50 URIs as raw strings
        - truncated = (nodes_count_fetched > effective_max_nodes)

на failed fetch (network down, 4xx/5xx, parse error):
    keep .raw как есть
    state.connections.sources[i].meta:
        - last_fetched_at = now
        - last_status = "err"
        - error_count++
        - last_error_msg = err.Error()
        - http_status_code = response.StatusCode  (если есть)

state.connections.sources[i].MarkCacheStale → ConfigStale via StateService
```

### Rebuild — единственный writer config.json (SPEC 045 invariant)

```
nodes := []
for each source where enabled:
    if type=subscription:
        body := read(bin/subscriptions/<id>.raw)
        if not exists:
            trigger Update first (auto-fallback)
            re-read body
        source_nodes := parse_uri_list(body, source.skip)
        apply tag (prefix/postfix/mask)
        truncate by source.max_nodes (or defaults.max_nodes)
    else: # type=server
        source_nodes := [parse_uri(source.uri)]
        # tag = source.label (если задан) либо URI fragment

    nodes += source_nodes

config := BuildConfig(state, nodes, template)
atomic_write(bin/config.json, config)
ClearConfigStale
```

---

## Migration v4 → v5

Однонаправленная: после save в v5 в формат v4 не возвращаемся (откат
лаунчера → "invalid version, regenerate via wizard").

```
v4 input → v5 output:

# top-level
state.version = 5
state.meta = { version: 5, comment, created_at, updated_at }
drop: id, rules_library_merged, selectable_rule_states
state.connections = { sources: [], outbounds: [], defaults: {} }
state.connections.outbounds = old.parser_config.ParserConfig.outbounds
state.connections.defaults.reload = old.parser_config.ParserConfig.parser.reload
state.connections.defaults.max_nodes = 3000  // const default
drop: old.parser_config.ParserConfig.parser.last_updated

# sources (one v4 ProxySource → one or more v5 Source)
for each old.parser_config.ParserConfig.proxies[i]:
    new_id := ulid.Make().String()

    if old.source != "" and len(old.connections) == 0:
        emit Source{
            id: new_id,
            type: "subscription",
            enabled: !old.disabled,
            url: old.source,
            skip: old.skip,
            tag: { prefix: old.tag_prefix, postfix: old.tag_postfix, mask: old.tag_mask },
            outbounds: old.outbounds,
            exclude_from_global: old.exclude_from_global,
            expose_group_tags_to_global: old.expose_group_tags_to_global,
            // meta пустое — заполнится первым Update'ом
        }

    if len(old.connections) > 0:
        for j, uri := range old.connections:
            emit Source{
                id: ulid.Make().String(),  // each gets own ID
                type: "server",
                enabled: !old.disabled,
                label: extractFragment(uri) or fmt.Sprintf("server-%d", j+1),
                uri: uri,
                exclude_from_global: old.exclude_from_global,
            }

    if old.source != "" and len(old.connections) > 0 (mixed legacy):
        emit оба варианта выше как отдельные записи

# outbounds.cache.json
if exists bin/outbounds.cache.json:
    delete it (no longer used)
```

Migration runs in `state.Load(path)` when detected v4. Save сразу пишет v5.

---

## Acceptance criteria

1. **Round-trip**: после миграции v4→v5 и save→load итоговая структура идемпотентна
   (повторный save даёт байт-в-байт тот же файл).
2. **Headers parse**: тестовый fetch на V2Board-фикстуре извлекает все 6
   полей (profile_title, userinfo, support_url, profile_web_page_url,
   content_disposition, profile_update_interval).
3. **Inline #-comments**: тот же тест с тем же контентом, но headers
   эмитятся как `#subscription-userinfo: ...` в body первой строкой —
   результат идентичен HTTP-варианту.
4. **Failed fetch resilience**: после 1-3 подряд failed fetch'ей `.raw`
   файл не повреждён, error_count инкрементится, success возвращает 0.
5. **Cache discipline**: после Update — новый `.raw` на диске, размер
   совпадает с `meta.raw_body_bytes`. После Source.Delete — `.raw`
   удаляется при следующем Update.
6. **Preview consistency**: `meta.preview_nodes[]` содержит ровно
   `min(50, nodes_count_fetched)` элементов.
7. **Rebuild без сети**: с пустым `bin/subscriptions/` при первом
   Rebuild'е срабатывает auto-Update; при наличии `.raw` — не дёргает сеть.
8. **`max_nodes` cap**: подписка с 20K нод после Update имеет
   `meta.nodes_count_fetched=20000, truncated=true`, в config.json
   попадает не более `effective_max_nodes` outbound'ов.
9. **state.json size**: для подписки с 50 нод inline-payload meta < 20KB;
   для 20K нод — тот же ≤ 20KB (preview обрезан).
10. **Migration data preservation**: все user-settings (DNS, custom_rules,
    vars, config_params) сохранены без изменений после v4→v5.

---

## Risks & mitigations

| Риск | Mitigation |
|---|---|
| Big migration: 30-50 callsite'ов касаются `parser_config.ParserConfig.*` | Полный sed по всему codebase + grep'нём остатки; компилятор поможет |
| User скрипты опираются на v4 schema | Только локальные пользователи; migration однократно при первом запуске v5; warning в release notes |
| `bin/outbounds.cache.json` всё ещё ссылается из тестов | Тесты переписать или удалить; cache теперь генерируется in-memory из `.raw` |
| Большие raw bodies (>10MB) на slow disk | Atomic write через `.tmp + Rename` уже async-friendly; нет блокирующих stat'ов |
| `.raw` файл corrupted (kill -9 в полёте) | Sane error на parse → mark err in meta → next Update retries |

---

## Out of scope (отдельные SPEC)

- **SPEC 053 PER_SOURCE_AUTO_UPDATE_TIMER** — per-subscription timer
  на основе `update.interval_hours` + exponential backoff. Не в этом SPEC.
- **SPEC 054 SUBSCRIPTION_INSPECT_UI** — кнопка «Refresh this sub», диалог
  «View raw content», UI для traffic quota / expiry visualization.
- **SPEC 055 PER_SOURCE_TRUNCATE_MERGE** — per-source merge при failed
  fetch (сейчас coarse: failed → keep old `.raw`; в будущем — partial
  merge, sources внутри одной подписки).

---

## Связи

- **SPEC 045 STATE_CONFIG_DECOUPLING** — этот SPEC уточняет state.json layout
  не нарушая invariant'ов 045 (Rebuild = sole writer config; Update =
  network-only).
- **SPEC 047 TYPED_EVENT_BUS** — события `SubscriptionFetchSucceeded`,
  `SubscriptionFetchFailed` будут добавлены в типизированный bus в SPEC 053.
- **SPEC 050 DEBUG_API_STATE_MUTATIONS** — endpoint'ы `PATCH /state/connections/sources`,
  `POST /subscriptions/{id}/refresh` — будут опираться на v5 schema.
