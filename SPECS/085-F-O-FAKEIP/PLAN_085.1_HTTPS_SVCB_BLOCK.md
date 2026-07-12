# 085.1 — FakeIP: HTTPS/SVCB predefined-блок (follow-up)

**Статус:** N (spec-only — нужен live-GUI, класс SPEC 062 / a58a176) · **Effort:** M
**Зависит от:** реализованного пресета fakeip (SPEC 085, коммит 57b56cd).

> Design + adversarial verify (Opus). Оба подтвердили: unit-тесты **необходимы, но не
> достаточны**; `dns_rules`-plural трогает DNS-ordering (SPEC 062) + DNS save/load, чей последний
> рефактор откатывался (`a58a176`) из-за GUI-регрессии. **Обязателен live-GUI DNS-save/reorder
> round-trip перед merge.** Поэтому — спека, не автономный merge.

## Проблема

Реализованный fakeip-пресет несёт один `dns_rule` (A/AAAA→fakeip). Полный FakeIP хочет второе
правило — блок HTTPS/SVCB: `{query_type:["HTTPS","SVCB"], action:"predefined", rcode:"NOERROR"}`
(иначе Chrome через HTTPS RR ipv4hint может обойти fakeip). Модель `Preset.DNSRule` — **одиночная**,
а нужно два упорядоченных правила (блок первым, A/AAAA вторым).

## План (минимальный blast-radius: один toggle на две rules)

1. **`core/template/preset_types.go`**: +`DNSRules []map[string]interface{} json:"dns_rules,omitempty"`
   рядом с `DNSRule` (legacy singular оставить — им пользуется `russian`).
2. **`core/template/preset_lite.go`** `PresetHasDNSRule()`: `p.DNSRule != nil || len(p.DNSRules) > 0`.
   Ключ: `sync_dns` создаёт **один** `state.DNSRule{Ref:<id>}` на пресет → один toggle на весь блок
   (как в LxBox — fakeip одна карточка). SPEC 062 ordering не мультиплицируется.
3. **`core/build/preset_expand.go`**: `PresetFragments.DNSRules []map`; секция 5 итерирует
   `append(preset.DNSRules, [preset.DNSRule если !nil])` с сохранением порядка; `isDNSRuleEmpty`
   релакснуть — правило с `action` ИЛИ `query_type` (без `server`) не считать пустым.
4. **`core/build/resolve_dns.go`**: `presetDNSRulesByID map[string][]ResolvedDNSRule` (список);
   Pass 3b/4 append'ят ВСЕ элементы в порядке; enabled — общий по `p.ID`.
5. **`core/template/preset_loader.go`** `validateRuleSetRefs`: продублировать для `p.DNSRules[]`;
   predefined-правило без server (srv=="") пропускать.
6. **`bin/wizard_template.json`**: fakeip-пресет → `dns_rules:[{query_type:["HTTPS","SVCB"],
   action:"predefined",rcode:"NOERROR",if:["@force"]},{query_type:["A","AAAA"],server:"@dns_server"}]`
   + var `force` (bool, default true). `default_enabled` НЕ ставить (как LxBox).
7. **UI** (`dns_preset_bundled.go`, `preset_ref_edit_dialog.go`): preview показывает оба правила;
   один toggle-слот.

## Проверки (обязательны ОБА уровня)

- unit: `fakeip_test`/`preset_expand_test`/`resolve_dns_test` — два правила в порядке
  [HTTPS/SVCB predefined, A/AAAA→fakeip]; predefined без server; при `force=false` первого нет;
  `russian` (singular DNSRule) не сломан.
- `sing-box check` на эмите.
- **live-GUI (gate)**: включить/выключить fakeip, DNS-save round-trip, reorder DNS-правил,
  повторный Load — проверить, что порядок и toggle переживают (класс a58a176).

## Почему не в этой сессии

`dns_rules`-plural — правильное решение с малым радиусом, но затрагивает DNS save/load ordering,
где unit-зелёный ≠ ship-ready. Владелец/следующая сессия лендят с GUI-верификацией.
