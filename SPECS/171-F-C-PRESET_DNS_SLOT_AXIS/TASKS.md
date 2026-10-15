# TASKS 171

## Этап 1 — контракт (1.1.127)
- [x] T1 `contract/docs/TEMPLATE_LANG.md`: §6.10 (D-135), §6.5.2, §6.7.5, строка D-135 в §9
- [x] T2 `contract/docs/BACKUP.md`: строка `rules[].ref` таблицы `dns`
- [x] T3 `contract/VERSION` → 1.1.127; `contract/README.md` changelog; `contract/TASKS_LXBOX.md` §124 (+ ссылка из §123); `go generate ./contract/...`

## Этап 2 — код
- [x] T4 `core/state/sync_dns.go`: `PresetDNSSlotIndex`, `presetRuleNums`; вставка по оси в `SyncDNSOptionsWithActivePresets`
- [x] T5 `ui/configurator/models/dns_rule_slot.go`: `ReconcileDNSRuleOrder` — слот пресета по оси
- [x] T6 `core/build/resolve_dns.go`: Pass 4 по синхронизированной копии, fallback снят
- [x] T7 Комментарии: `sync_dns.go`, `dns_rule_slot.go`, `resolve_dns.go`, `rule_order.go`

## Этап 3 — тест, доки
- [x] T8 `core/build/late_preset_dns_order_test.go` — `TestLatePresetDNSRuleTakesAxisPosition`
- [x] T9 `docs/release_notes/upcoming.md` (EN/RU), `docs/ARCHITECTURE.md`, отчёт SPEC 170 (ссылка), `IMPLEMENTATION_REPORT.md`
- [x] T10 `go build ./...`; тесты по имени; CI `run_mode=tests` зелёный; папка → `171-F-C-…`
