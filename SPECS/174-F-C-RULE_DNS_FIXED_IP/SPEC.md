# SPEC 174 — Ответ DNS фиксированным IP у правила (`rules[].dns.fixedIps`)

Статус: **C** (24.10.2026, принята владельцем; решение владельца лаунчера «да делать» — паритет с LxBox §683; поправка владельца LxBox 24.10: поле одно, флага `fixed` нет). Тип: Feature. Контракт: **1.1.139**, норма — `contract/docs/TEMPLATE_LANG.md` §6.11 (блок «Локальный ответ — `fixedIps`»), `BACKUP.md` таблица `rules[]`, `schema/backup.schema.json`, корпус `contract/corpus/rule_dns/`; LxBox — `TASKS_LXBOX.md` §135.

## Проблема

Пользователи просят hosts: «этот домен → этот IP». У LxBox это режим ответа правила (их спека 683), у лаунчера — только свой DNS-правилом в JSON. Поле `rules[].dns.fixedIps` в бэкапе от LxBox лаунчер терял.

## Решение

1. **Данные.** `state.RuleDNS.FixedIPs []string` (`fixedIps`, только непустым). Режим действует, когда хоть одна строка разбирается как IPv4/IPv6 (`FixedActive`); `Enabled`/`ServerTag`/`ForceIPv4` хранятся, но не эмитятся.
2. **Сборка** (`core/build/rule_dns_mirror.go`). Вместо глушилки AAAA и зеркала на сервер — два DNS-правила: `M + {query_type: [A], action: predefined, rcode: NOERROR, answer: ["*. 300 IN A <ip>", …]}` и то же для AAAA; семейство без адресов — без `answer`. Гейт и матчер — те же, что у зеркала (§6.11). Ядро `1.14.3-lx.22` подставляет `*.` под имя запроса (`rewriteRecords`).
3. **UI.** Окно правила, вкладка DNS: «Answer» — Default / Dedicated server / Fixed IP. Fixed IP — многострочное поле (одна строка — один адрес); неразбираемая строка или пустое поле блокируют Save с текстом ошибки; Force IPv4 при Fixed IP недоступен. Переход в другой режим не пишет `fixedIps`, тег сервера хранится. Вкладка DNS Конфигуратора: строка правила — `Fixed IP: <адреса>`; правило в режиме Fixed IP не держит сервер «включённым ради зеркала».
4. **Корпус** `rule_dns` (4 кейса), раннер `core/build/contract_rule_dns_test.go`.

## Не входит

Переделка Resolve правила из спеки LxBox 683 (секция «DNS & Resolve», Resolve only в Action) — у лаунчера своё устройство, не запрошено.
