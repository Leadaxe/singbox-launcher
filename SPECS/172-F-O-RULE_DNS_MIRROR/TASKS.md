# TASKS 172

## Этап 0 — решения
- [x] T0 Развилки SPEC §6 решены 15.10 по умолчанию: camelCase как у LxBox; выключенный сервер включается принудительно; зеркала строками во вкладке DNS; `forceIpv4` в этой SPEC

## Этап 1 — контракт
- [x] T1 `contract/docs/TEMPLATE_LANG.md`: §6.11 (D-137), ссылка из §6.10, строка D-137 в §9
- [x] T2 `contract/docs/ONE_NAMESPACE.md` §1; `contract/docs/BACKUP.md` (таблица `rules[]`, «Поля стороны LxBox»)
- [x] T3 `contract/schema/backup.schema.json` (`$defs/rule/properties/dns`); `contract/VERSION`; `contract/README.md`; `contract/TASKS_LXBOX.md` §127; `go generate ./contract/...`

## Этап 2 — данные
- [ ] T4 `core/state/rule_types.go`: `RuleDNS`, `Rule.DNS`; `core/state/record_clone.go`
- [ ] T5 `core/backup/file_keys_10.go` (`dns` из ключей LxBox); импорт/экспорт `1.0`; решение по 0.12
- [ ] T6 `core/state/legacy_types.go`, `core/state/load_v6.go`, `ui/configurator/models/rule_state.go`, загрузка визарда, `ui/configurator/models/preset_ref_sync.go` — поле проходит круг без потерь

## Этап 3 — сборка
- [ ] T7 `core/build/rule_dns_mirror.go`: `ruleDNSMatch`, `ruleDNSMirrors`
- [ ] T8 `core/build/resolve_dns.go` Pass 4: место зеркал по норме; `DNSSourceRuleMirror`; маршрут разрешается один раз
- [ ] T9 Сервер: принудительное включение выключенного; предупреждение об удалённом (`preset_merge.go` `repairDanglingDNSRefs`)
- [ ] T10 Превью Конфигуратора показывает зеркала

## Этап 4 — интерфейс
- [ ] T11 `ui/configurator/dialogs/add_rule_dialog.go`: блок DNS, гейт общим `ruleDNSMatch`, SRS-подсказка, JSON-вкладка
- [ ] T12 `ui/configurator/tabs/rules_unified_rows.go`: метка DNS
- [ ] T13 `ui/configurator/tabs/dns_unified_rules.go`: строки-зеркала только для чтения; подсказка у сервера
- [ ] T14 `bin/locale/ru.json`

## Этап 5 — тест, доки
- [ ] T15 `core/build/rule_dns_mirror_test.go` — `TestRuleDNSMirror` (+ подтест круга данных)
- [ ] T16 `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md`, `docs/ARCHITECTURE.md`, `IMPLEMENTATION_REPORT.md`
- [ ] T17 `go build ./...`; тесты по имени; CI `run_mode=tests` зелёный; живая проверка ядра (риск «IP-наборы»); показ UI владельцу; папка → `172-F-C-…`
