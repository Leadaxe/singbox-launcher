# IMPLEMENTATION REPORT 174

**Итог:** у собственного правила маршрута режим ответа DNS «Fixed IP» (`rules[].dns.fixedIps`, контракт 1.1.139) — паритет с LxBox §683. Принято владельцем 24.10.2026.

## Что сделано

- `core/state/rule_types.go`: `RuleDNS.FixedIPs`, `FixedAddrs` (по семействам, неразбираемые пропускаются), `FixedActive`; `IsZero` и клон учитывают список.
- `core/build/rule_dns_mirror.go`, `preset_merge.go`: при `FixedActive` — два DNS-правила `predefined` (A, AAAA) с `*. 300 IN …`, без глушилки и зеркала на сервер; семейство без адресов — без `answer`.
- Окно правила, вкладка DNS: «Answer» Default / Dedicated server / Fixed IP; многострочное поле адресов; ошибка строки и пустое поле блокируют Save; Force IPv4 при Fixed IP недоступен. Вкладка DNS Конфигуратора: строка `Fixed IP: …`; правило в этом режиме не держит сервер включённым ради зеркала. 8 ключей `locale.T`, ключ «Send DNS to a dedicated server» снят.
- Контракт 1.1.139 (b8a6ead2): §6.11, `BACKUP.md` таблица `rules[]`, схема; `fixed` из 1.1.138 снят. Корпус `contract/corpus/rule_dns/` (4 кейса), раннер `core/build/contract_rule_dns_test.go`. LxBox: синк f48ac7c0, раннер c5c2842a — 4/4, §135 закрыт.
- Доки: `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`.

## Отступления

- Флаг режима `fixed` не заводился: по поправке владельца LxBox режим = непустой список с разбираемым адресом.
- При переходе в Default / Dedicated server адреса не хранятся (иначе непустой список и был бы режимом).
- Переделка Resolve из спеки LxBox 683 не входила.
