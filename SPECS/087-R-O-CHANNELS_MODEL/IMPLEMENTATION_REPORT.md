# 087 — Модель каналов: отчёт о реализации (backend)

**Дата:** 2026-07-12 · **Статус:** backend реализован и проверен; seed + UI — follow-up (нужны решения владельца + GUI).

## Сделано (backend, аддитивно — нет каналов в state = нулевое изменение поведения)

1. **Модель** (`core/state/channel_types.go`): `Channel`/`ChannelAuto`/`ChannelBalancer` (порт
   LxBox §125), `AutoTag()`, `IsRequired()` (vpn-1), `RequiredChannelTag`.
2. **Persistence** (`state.go`, `disk_v6.go`, `save.go`, `load_v6.go`): `State.Channels`,
   disk-ключ `channels` (omitempty), round-trip в обе стороны. **Инвариант:** каналы НЕ в
   `connections.outbounds` (тест проверяет).
3. **Материализация** (`core/build/channel_outbounds.go`): `BuildChannelOutbounds(channels)` —
   порт `_buildChannelGroups`. Селектор (Filters из node_filter+invert → `/re/i`/`!/re/i`,
   AddOutbounds direct-out/block-out/auto, PreferredDefault из default_filter) + опциональный
   urltest-двойник (round_robin+balancer в Options). Битый regex → match-all (как LxBox).
   direct-out всегда как non-empty guard. Делегирует фильтрацию/default/skip-пустого генератору.
4. **Инжект в build** (`rebuild_raw_cache.go` + `config_service.go`): copy-then-append каналов в
   `parserCfg.Outbounds` перед `GenerateOutboundsFromParserConfig` (никогда не мутируем
   state-owned slice). Эфемерно — в config.json, не в state.
5. **Автолечение** (`core/state/channel_heal.go`): `HealDanglingChannelRefs(s, deletedTag)` —
   route_final (Vars по .Name) + Rules/CustomRules outbound на удалённый канал/auto → vpn-1.
6. **Шаблон**: `+{"type":"block","tag":"block-out"}` в config.outbounds (для include_block).

## Проверка (DoD)

- `go test ./core/...` — OK (12 пакетов, 0 FAIL): round-trip Channels, инвариант «нет vpn-N в
  connections», heal, BuildChannelOutbounds (selector+urltest+balancer, invert, disabled-skip,
  required-kept, bad-regex-match-all). `go vet` OK.
- **Материализованная форма** (selector + round_robin urltest balancer + block-out) принята
  `sing-box check` на **rc.17 И lx.3**.
- Регрессий нет: инжект no-op при пустом Channels (существующие юзеры не затронуты).

## НЕ сделано (follow-up — решения владельца + GUI)

- **Seed** (`channel_seed.go`, `group_templates`/`default_channels` в шаблоне) — требует решений
  владельца (SPEC §6): миграция legacy `vpn ①/②` из parser_config.outbounds; дефолт
  `route_final` proxy-out→vpn-1. Без seed каналы работают, если заданы в state (программно/через
  будущий UI), но авто-создание vpn-1/vpn-2 на первом запуске отложено до решений.
- **UI каналов** (`ui/configurator/`) — редактор канала (фильтр, флаги, auto/balancer), список.
  Нужен GUI-прогон; строится вместе с UI балансировки (088.1).
- **Вызов heal** из UI при удалении/выключении канала (backend готов, точку вызова добавит UI).

## Инвариант безопасности

Каналы намеренно вне `Connections.Outbounds` → не зеркалятся в legacy ParserConfig, не проходят
preset-ref sync (SPEC 057/058). Риск отката уровня a58a176 не активируется.
