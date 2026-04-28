# SPEC 052 — Implementation Plan

Большой структурный refactor. Делаем фазами с green-tests на каждом этапе.

---

## Фаза 1 — Новые pure-data типы (без интеграции)

**Цель:** Завести Go-типы, схему, JSON-сериализацию. Никаких боевых
callsite'ов пока не трогаем.

### Файлы

- `core/state/v5/types.go` (новый):
  - `MetaSection`
  - `ConnectionsSection`
  - `Source`, `SourceType` const'ы
  - `TagSpec`, `UpdateSpec`
  - `SubscriptionMeta`, `UserInfo`
- `core/state/v5/types_test.go`:
  - JSON round-trip
  - omitempty на пустых полях
  - validate Source.Type discriminator
- `core/state/v5/migration.go`:
  - `MigrateV4ToV5(old WizardStateFileV4) State` — pure func, deterministic
- `core/state/v5/migration_test.go`:
  - Реальный fixture v4 (взять `state.json` из install — 5 источников + 11 правил + DNS)
  - Round-trip: v4 → migrate → v5 → JSON dump → diff vs golden expected v5

### Acceptance

- `go test ./core/state/v5/...` — green
- migration_test покрывает: subscription-only, server-only (connections[]),
  mixed (source + connections), disabled, max_nodes default

---

## Фаза 2 — HTTP headers + inline-comments parser

**Цель:** Standalone-парсер subscription-метаданных. Изолированный, тестируемый.

### Файлы

- `core/config/subscription/meta.go` (новый):
  - `ParseHeaders(http.Header) MetaFromHeaders` — выкручивает 6 полей
  - `ParseInlineComments(body []byte) MetaFromHeaders` — сканит первые ~100 строк
  - `MergeMeta(headers, inline)` — headers выигрывают, inline fallback
  - `parseSubscriptionUserinfo(s string) UserInfo` — `upload=N; download=N; total=N; expire=UNIX`
  - `decodeProfileTitle(s string) string` — base64 detect + UTF-8 decode
  - `parseContentDispositionFilename(s string) string`
- `core/config/subscription/meta_test.go`:
  - Fixtures из V2Board / Xboard (записать примеры response'ов)
  - Edge cases: пустые headers, malformed userinfo, base64-encoded title

### Acceptance

- Все 6 полей извлекаются из synthetic V2Board response
- Inline-comment вариант даёт тот же результат
- Malformed input не паникует, возвращает empty meta

---

## Фаза 3 — Fetcher 2.0 (raw body cache + meta parse)

**Цель:** Расширить `FetchSubscription` чтобы возвращал meta + писал raw.

### Файлы

- `core/config/subscription/fetcher.go` (модификация):
  ```go
  type FetchResult struct {
      Body          []byte
      Meta          SubscriptionMeta // headers + inline parsed
      HTTPStatus    int
      RawBodyBytes  int64
  }
  
  func FetchSubscriptionWithMeta(url string) (*FetchResult, error)
  ```
  - Старая `FetchSubscription` сохраняется как deprecated wrapper
  - User-Agent остаётся `SubscriptionParserClient` (не ломаем поведение
    провайдеров с whitelist'ом)
- `core/state/v5/raw_cache.go` (новый):
  - `WriteRawBody(execDir, sourceID, body)` — atomic `.tmp + Rename`
  - `ReadRawBody(execDir, sourceID) ([]byte, error)`
  - `DeleteOrphans(execDir, knownIDs []string)` — lazy GC
- Tests с фикстурами

### Acceptance

- `bin/subscriptions/<id>.raw` пишется атомарно
- Fetch error → старый `.raw` не повреждается
- Orphan GC работает: id убран из state → файл удалён при следующем Update

---

## Фаза 4 — State load/save миграция

**Цель:** Переключить `core/state/load.go` + `save.go` на v5 формат, с
auto-migration при detection v4.

### Файлы

- `core/state/load.go`:
  - Detect version: `version: 4` → migrate via `v5.MigrateV4ToV5`
  - `version: 5` → load directly
  - Unknown version → error «regenerate via wizard»
- `core/state/save.go`:
  - Always write v5
- `core/state/state.go` (тип `State`):
  - Перейти на `v5.State` (новые поля)
  - **Adapter** методы для backward-compat callsite'ов:
    `s.GetSubscriptionSources() []Source` etc.

### Acceptance

- Real-world v4 state.json (из `/Applications/.../bin/wizard_states/state.json`)
  читается без потерь
- `go test ./core/state/...` — green
- Round-trip v4→v5→save→load — идемпотентно

---

## Фаза 5 — `Source` adapter в parser pipeline

**Цель:** Парсер (`core/config/...`) не должен знать про v5 на этом этапе.
Переходник: новый `Source` → старая `ProxySource` для существующих парсеров.

### Файлы

- `core/state/v5/adapter.go`:
  - `(*Source).ToProxySourceV4() configtypes.ProxySource` —
    legacy-shim для `GenerateOutboundsFromParserConfig` и компании
- `core/config_service.go`:
  - `loadParserConfigForUpdate` — теперь читает new state, конвертит
    sources[] (subscription) → proxies[] for parser
  - `UpdateConfigFromSubscriptions` — использует `FetchSubscriptionWithMeta`,
    сохраняет meta обратно в state (через StateService)

### Acceptance

- Update flow в runtime работает без изменений output (config.json
  идентичен previous, golden-test green)
- meta заполняется в state.json после первого Update'а
- `bin/subscriptions/<id>.raw` появляется

---

## Фаза 6 — `bin/outbounds.cache.json` deprecation

**Цель:** Убрать parsed cache, перевести Rebuild на `.raw` файлы.

### Файлы

- `core/rebuild.go`:
  - вместо `outboundscache.Load` → читать `bin/subscriptions/*.raw` и
    re-parse через парсер
  - in-memory caching результата на сессию (mutex'нутый map)
- `core/outboundscache/snapshot.go`:
  - **deprecated**, helper остаётся для миграции (delete old file at
    state load if v5)

### Acceptance

- `outbounds.cache.json` старый удаляется при первом v4→v5 переходе
- Rebuild без сети работает на cached `.raw`
- Auto-fallback: нет `.raw` → trigger Update → re-parse — без regression

---

## Фаза 7 — UI визард: новые типы в моделях

**Цель:** Configurator модель внутри визарда работает с новой Source-схемой.

### Файлы

- `ui/configurator/models/wizard_model.go`:
  - WizardModel.Sources []Source (вместо ProxyConfig.Proxies)
- `ui/configurator/business/parser.go`:
  - Save flow → пишет new schema
  - Load flow → читает new schema
- `ui/configurator/tabs/source_tab.go`:
  - UI секция «Subscriptions» (показывает meta: profile_title, quota bar,
    expire badge, last_status)
  - UI секция «Servers» (одиночные URI)
  - Add/Remove/Edit разделены по типам

### Acceptance

- Open визард → видит свои подписки + servers, метаданные показываются
- Save → пишет v5
- E2E: Configurator → Save → Restart → sing-box стартанул на новом config

---

## Фаза 8 — Cleanup + docs

- Удалить deprecated `core/outboundscache` если все ссылки сняты
- Обновить `docs/ARCHITECTURE.md`
- Обновить `docs/release_notes/upcoming.md` — entry про SPEC 052
- Финальный `IMPLEMENTATION_REPORT.md` в SPEC dir
- Переименовать `SPECS/052-F-N-` → `SPECS/052-F-C-` (close-out)

---

## Тестовая стратегия

| Уровень | Что | Где |
|---|---|---|
| Unit | Pure data types, JSON round-trip | `core/state/v5/*_test.go` |
| Unit | Headers/inline parser | `core/config/subscription/meta_test.go` |
| Integration | Fetcher → meta + raw cache | `core/config/subscription/fetcher_test.go` |
| Integration | Migration v4→v5 на real state.json | `core/state/v5/migration_test.go` |
| Golden | BuildConfig output после refactor — byte-equal | `core/build/golden_test.go` |
| E2E | Пройти Configurator → Save → Update → Restart на новом state | manual |

---

## Sequencing constraints

```
Фаза 1 (types)
   ↓
Фаза 2 (meta parser)  ──→  Фаза 4 (load/save)
   ↓                          ↓
Фаза 3 (fetcher)       Фаза 5 (parser adapter)
   ↓                          ↓
   └──────────┬───────────────┘
              ↓
        Фаза 6 (rebuild on .raw)
              ↓
        Фаза 7 (UI)
              ↓
        Фаза 8 (cleanup)
```

Каждая фаза самодостаточна (build green, tests green, no UX regression).
