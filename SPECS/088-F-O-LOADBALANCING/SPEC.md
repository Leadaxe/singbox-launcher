# 088-F-O — LoadBalancing (urltest round_robin + balancer)

**Тип:** Feature · **Статус:** O (generator+guard реализованы; UI — follow-up) · **Дата:** 2026-07-12
**Ядро:** `1.14.0-lx.1-rc.17` — всё есть (проверено на теге). Спека ядра НЕ нужна.

## 1. Что даёт ядро (rc.17)

`option/group.go`: `URLTestOutboundOptions` уже несёт `Mode ("least_test"|"round_robin")` +
`Balancer{Pool int, PoolTolerance uint16, StickyHash []string}`. Балансировщик
(`protocol/group/urltest_balance_lx.go`, ядро SPEC 019):
- `mode:"least_test"` (деф) — один лучший узел по delay (upstream-поведение).
- `mode:"round_robin"` — пул живых узлов + sticky-сессии.
- **sentinel-контракт**: `sticky_hash` len 0 → **default** `["process","domain"]` (НЕ off);
  `["none"]` → off. Bare `[]` схлопывается декодером в nil → тоже default. Поэтому эмитить
  пустой `sticky_hash:[]` **нельзя** — это молча включает дефолтную липкость.

## 2. Реализовано в этой сессии (backend, verdict verify = *sound*)

Генератор уже эмитит любой ключ `OutboundConfig.Options` через generic-цикл, значит `mode`
и вложенный `balancer:{...}` проходят **без правок**. Сделаны две точечные правки в
`core/config/outbound_generator.go`:

1. **`sanitizeBalancerOptions`** — перед эмитом дропает `balancer.sticky_hash`, если он пустой
   (`[]`/`[]string{}`/nil). Липкость-off делается только через `["none"]` из UI; пустой `[]`
   больше не «протекает» в конфиг и не включает дефолтную липкость по ошибке.
2. **Детерминированный порядок Options** — ключи сортируются перед эмитом (`sort.Strings`).
   Go-`range` по map неупорядочен → пара `mode`/`balancer` меняла позицию → byte-exact
   golden-фикстуры флейкали (эмпирически 18/200 прогонов). Сортировка чинит корень для любых
   multi-key Options, безвредна для sing-box (порядок ключей объекта незначим).

Тесты (`core/config/loadbalance_test.go`): round_robin+balancer проходят; пустой sticky_hash
дропается; `["none"]` сохраняется; plain urltest не получает mode/balancer; эмит детерминирован
(20 прогонов идентичны). `go test ./core/config/... ./core/build/` — зелёные, без регрессий.

## 3. Что НЕ сделано (follow-up 088.1 — UI, verify = spec-only)

Реальный gap — не «нет create-группы» (create УЖЕ есть: `configurator.go` addBtn →
`ShowEditDialog(...,nil,...)`), а **отсутствие balancing-контролов** в `edit_dialog.go`:
- блок LoadBalancing виден, когда тип группы = auto/urltest (привязка к `urltestVisible()` /
  `typeSelect.Selected == locale.T("wizard.outbound.type_auto")`, НЕ к литералу "urltest");
- `mode`-select (least_test/round_robin); при round_robin — `pool` (int), `pool_tolerance`
  (0..65535, валидировать верхнюю границу — ядро uint16), `sticky_hash`-чекбоксы
  (process/domain/source_ip/dest_ip/dest_port);
- «disable stickiness» пишет `["none"]`, а НЕ `[]`;
- при switch urltest→selector чистить `mode`/`balancer` (сейчас `edit_dialog.go` чистит только
  `url`/`interval`/`tolerance` в selector-ветке — иначе balancer протечёт в selector → `sing-box
  check` упадёт);
- Raw↔Form round-trip: `pool` из JSON приходит float64 → в populate нужен `toInt`.

**Локали:** новые ключи — в `internal/locale/en.json` (embed) + зеркало в `bin/locale/ru.json`
(НЕ `internal/locale/ru.json` — его нет). `go test ./internal/locale/` проверит паритет.

**@-подстановка**: `substituteOptionsMap` ходит только по top-level Options → `@var` внутри
`balancer` не резолвится. `pool`/`pool_tolerance` задаются литералами (документировать).

## 4. DoD

- [x] generator emit round_robin/balancer + sentinel-guard + детерминизм; тесты зелёные.
- [x] `go build/test/vet ./core/config/...` — OK, без регрессий golden.
- [ ] (088.1) UI-контролы балансировки в edit_dialog + локали + Raw round-trip + selector-cleanup.

## 5. Связь с каналами (SPEC 089)

round_robin/balancer — это `auto`-двойник канала в модели каналов LxBox. UI-контролы
балансировки логично строить вместе с UI каналов (089), чтобы `ChannelAuto{mode,balancer}`
и группо-редактор не разошлись.
