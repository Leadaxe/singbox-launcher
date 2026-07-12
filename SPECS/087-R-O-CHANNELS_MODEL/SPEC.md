# 087-R-N — Модель каналов (порт LxBox §125/§267)

**Тип:** Refactor/Feature · **Статус:** N (спека; реализация spec-only — нужен live-GUI + миграции)
**Дата:** 2026-07-12 · **Ядро:** rc.17 (selector/urltest+balancer — есть, ядру ничего не нужно) · **Effort:** XL

> Спека составлена design-агентом (Opus) и проверена adversarial-верификатором (Opus).
> Вердикт: `needs-fixes` → учтены ниже; `safe_to_implement_this_session: spec-only`.

## 1. Идея

Канал = именованная группа серверов под задачу (как в LxBox): отбирает узлы regex-фильтром по
tag, переключается selector'ом, опционально имеет urltest-двойник (`<tag>-auto`) с
round_robin/balancer. Один сервер входит в N каналов. Правила ссылаются на канал по tag
(`vpn-1`). Это **чистый порт клиентской модели** — ядру ничего не требуется (`option/group.go`
уже даёт mode/balancer/sticky_hash, ядро SPEC 019).

## 2. Ключевое архитектурное решение (снимает риск отката a58a176)

**Channels — ОТДЕЛЬНОЕ top-level поле `state.State.Channels []Channel`, НЕ внутри
`Connections.Outbounds`.** Причина: `Connections.Outbounds` двусторонне зеркалится с legacy
`ParserConfig.Outbounds` на каждый Save/Load (`sync_to_connections.go` / `sync_to_legacy.go`) и
нагружен preset-ref sync-машинерией (SPEC 057/058). Каналы там = гарантированный откат уровня
`a58a176`. Поэтому:
- каналы хранятся отдельно (`diskV6.Channels json:"channels,omitempty"`), единственный источник истины состава;
- в selector/urltest `OutboundConfig` каналы материализуются **только на build-time** (эфемерно,
  в локальной копии `parserCfg.Outbounds` внутри `buildSnapshotFromRawCache`), в `state.json` их нет;
- тест-инвариант: после Save `state.Connections.Outbounds` НЕ содержит `vpn-N` тегов.

## 3. Модель данных (`core/state/channel_types.go`, новый)

```go
type Channel struct {
    Tag, Label                string
    Enabled, IncludeDirect    bool
    IncludeBlock              bool
    NodeFilter                string `json:"node_filter"`        // regex по tag узла; "" = все
    NodeFilterInvert          bool   `json:"node_filter_invert"`
    DefaultFilter             string `json:"default_filter"`     // regex; первый матч → selector.default
    InterruptExistConnections bool   `json:"interrupt_exist_connections"`
    IsDetour                  bool   `json:"detour"`             // канал = detour-прослойка
    Auto                     *ChannelAuto `json:"auto"`          // nil = без urltest-двойника
}
type ChannelAuto struct {
    URL, Interval string; Tolerance int; IdleTimeout string `json:"idle_timeout"`
    InterruptExistConnections bool `json:"interrupt_exist_connections"`
    Mode string; Balancer *ChannelBalancer                     // least_test | round_robin
}
type ChannelBalancer struct { Pool int; PoolTolerance int `json:"pool_tolerance"`; StickyHash []string `json:"sticky_hash"` }
```

`node_filter` → `OutboundConfig.Filters{"tag":"/re/i"}` (или `"!/re/i"` при invert) — механизм
уже есть (`outbound_filter.go`). Ссылка на канал = `tag`; `<tag>-auto` — тоже валидная ссылка.

## 4. Изменения (файлы)

| Файл | Изменение |
|------|-----------|
| `core/state/channel_types.go` (новый) | модель Channel/ChannelAuto/ChannelBalancer |
| `core/state/state.go` | +`Channels []Channel` (после Rules) |
| `core/state/disk_v6.go` + `save.go` + `load_v6.go` | round-trip `Channels` (⚠️ verify: нужны ОБА — marshal и unmarshal, иначе поле не переживёт Save/Load) |
| `core/state/channel_seed.go` (новый) | seed из template `group_templates`+`default_channels`, guard-маркер `channels_migrated` |
| `core/state/channel_heal.go` (новый) | автолечение dangling: `route_final`/rule.outbound на удалённый канал → `vpn-1`. ⚠️ verify: `route_final` живёт в `state.Vars []SettingVar` (slice, по `.Name`), НЕ map |
| `core/build/channel_outbounds.go` (новый) | **ядро**: `BuildChannelOutbounds(channels, td) []OutboundConfig` — порт `_buildChannelGroups` (build_config.dart:520-668); selector + urltest-двойник; пустой набор → fallback |
| `core/rebuild_raw_cache.go` + `config_service.go` | инжект materialized каналов в локальный `parserCfg.Outbounds` ПЕРЕД `GenerateOutboundsFromParserConfig` (после Sync/Merge) |
| `bin/wizard_template.json` | +`group_templates`+`default_channels` (порт §267); +`{"tag":"block-out","type":"block"}` в `config.outbounds` |
| `core/template/loader.go` | типы `GroupTemplates/DefaultChannel/ChannelTemplate/AutoTemplate` + `(td).GroupTemplates()` |
| golden-тесты `core/build` | кейс с `channels[]` |

## 5. Правки от adversarial verify (учесть при реализации)

1. **IncludeBlock тривиален**: у sing-box ЕСТЬ `type:"block"` outbound (`C.TypeBlock`). Добавить
   один `{"tag":"block-out","type":"block"}` в template и класть его в `AddOutbounds` при
   `include_block`. Риск «нет block-out» из дизайна снят — 1 строка.
2. **route_final** — итерировать `s.Vars []SettingVar` по `.Name=="route_final"`, не map-индексом.
3. **DefaultFilter** → `OutboundConfig.PreferredDefault{"tag": pattern}` (матч по резолвнутым узлам).
4. **Save/Load**: добавить `Channels` и в `marshalDisk`, и в `load_v6` (оба, с nil-guard как Rules).
5. **Инжект-безопасность**: `parserCfg := s.ParserConfig` копирует только header; `append(...,chans...)`
   в конец безопасен, но НЕ трогать существующие элементы slice in-place.

## 6. Открытые решения (для владельца — до реализации)

- **Миграция legacy `vpn ①`/`vpn ②`** (сейчас в `Connections.Outbounds` как `#TEMPLATE#`-ref) →
  каналы: one-shot удалить legacy vpn-selectors при seed, ИЛИ сосуществование с разными тегами.
- **`route_final` proxy-out→vpn-1**: seed-миграция переводит дефолт, ИЛИ оставить `proxy-out` в шаблоне.
- **Пустой канал**: всегда класть `direct-out` в `AddOutbounds` (гарантия непустоты) или block-out+PreferredDefault.

## 7. DoD (реализация — отдельная сессия)

- [ ] Channels round-trip Save/Load; инвариант «нет vpn-N в Connections.Outbounds».
- [ ] BuildChannelOutbounds → selector+urltest; golden + `sing-box check` зелёные.
- [ ] Автолечение dangling; seed из template; миграция legacy решена.
- [ ] **live-GUI**: создать/удалить/переключить канал, DNS/route save round-trip (обязательно — класс a58a176).
- [ ] UI каналов (совместно с UI балансировки SPEC 088.1).

## 8. Почему spec-only, а не реализация сейчас

XL-объём + затрагивает state persistence, seed-миграцию (решения владельца), и требует
live-GUI прогона Save/Load (класс регрессии `a58a176`, которую unit-тесты не ловят). Порт
`_buildChannelGroups` — детерминированный и покрывается unit+golden, но безопасно лендить только
с GUI-верификацией. Backend `channel_outbounds.go` + модель можно реализовать первым, изолированно,
в следующей сессии.
