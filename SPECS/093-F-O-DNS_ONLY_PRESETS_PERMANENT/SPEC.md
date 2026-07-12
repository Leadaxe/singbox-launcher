# 093-F-O — DNS-only пресеты как постоянные DNS-правила

**Тип:** Feature · **Статус:** O (backend+wiring; GUI-smoke перед релизом) · **Дата:** 2026-07-13

## Проблема
FakeIP — чистая DNS-фича (только dns_servers/dns_rules, нет route-правил), но включалась через
библиотеку на вкладке **Rules** (маршрутизация), а эффект показывался на **DNS**. Непоследовательно.

## Решение (по запросу пользователя)
DNS-only пресеты (`IsDNSOnly()` = есть dns-правила, нет route-правил — сейчас только `fakeip`)
становятся **постоянными DNS-правилами** на вкладке DNS: включены по умолчанию (default true),
отключаются toggle'ом, **не удаляются**, и **не показываются** в библиотеке / на вкладке Rules.

## Изменения
| Файл | Что |
|------|-----|
| `core/template/preset_lite.go` | +`PresetHasRoutingRule()`, `IsDNSOnly()` |
| `bin/wizard_template.json` | fakeip `default_enabled: true` |
| `ui/configurator/models/dns_only_seed.go` (новый) | `EnsureDNSOnlyPresetsSeeded(m)` — идемпотентный авто-сид preset-ref (Enabled=true; НЕ трогает существующий → toggle-off переживает Save/Load) |
| `ui/configurator/models/dns_rule_slot.go` | вызов сида в начале `ReconcileDNSRuleOrder` (гарантия на любом пути: load/fresh/refresh) |
| `ui/configurator/presentation/presenter_state_helpers.go` | сид в `restorePresetRefs` до построения order |
| `ui/configurator/tabs/library_rules_dialog.go` | skip DNS-only в библиотеке Rules |
| `ui/configurator/tabs/rules_unified_rows.go` | skip рендера DNS-only preset-ref на вкладке Rules |

DNS-правило от preset-ref на DNS-вкладке уже рендерилось как toggle-без-delete
(`dns_unified_rules.go` buildSingleDNSPresetRuleRow) — «отключить но не удалить» готово.

## Ключевой инвариант (проверен агентом + unit)
Выключенный пользователем fakeip (`DNSRuleEnabled=false`) **переживает Save/Load**: сид добавляет
только отсутствующий ref, `Enabled=true` (route-активатор), off хранится в `DNSRuleEnabled`.
route-запись fakeip в state.Rules сохраняется (нужна build'у как активатор), но на Rules-вкладке
скрыта.

## Проверка (DoD)
- `go test ./ui/configurator/models/ ./ui/configurator/presentation/ ./core/template/` — OK
  (сид: добавляет missing / идемпотентен / preserves toggle-off / skip route-пресетов / nil-safe;
  reconcile SPEC 062 без регрессий; preset-ref round-trip OK). `go vet` OK.
- **GUI-smoke (перед релизом, зона a58a176):** визард без state → fakeip на DNS (toggle ON, без
  delete), нет на Rules/в библиотеке; выключить → Save → рестарт → остаётся выключенным;
  config.json: ON → fakeip dns servers+rules есть, route.rules без fakeip; OFF → нет.
