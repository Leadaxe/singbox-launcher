# Корпус DNS-зеркала правила маршрута

Фикстуры DNS-правил, которые сборка выводит из объекта `dns` собственного
правила (`rules[]` вида `inline` | `srs`; норма — `docs/TEMPLATE_LANG.md`
§6.11). Гоняют обе стороны — лаунчер через
`core/build/contract_rule_dns_test.go`, LxBox через свой раннер.

## Формат

- `<case>.rule.json` — вход:
  - `_` — что проверяет кейс, по-русски;
  - `rule` — запись правила в форме состояния / бэкапа 1.0 (`kind`, `name`,
    `enabled`, `refs` у `srs`, `body`, `dns`). Наборы `srs` считаются
    скачанными.
- `<case>.expected.json` — нормативный выход:
  - `dns_rules[]` — DNS-правила зеркала в порядке выпуска **без ключей
    матчера** (список реестра `rule_dns_mirror_match`). Пустой массив —
    зеркала нет.

## Что нормативно, а что нет

**Нормативны** число и порядок DNS-правил зеркала и их ключи действия:
`server`, `ip_version`, `query_type`, `action`, `rcode`, `answer` (порядок
элементов `answer` — порядок адресов в `fixedIps`).

**Не проверяется матчер `M`**: его форма — выбор стороны (§6.11, контракт
1.1.132): лаунчер кладёт ключи правила inline, LxBox — headless `rule_set`
плюс `inbound`. Раннер снимает ключи матчера до сравнения. Порядок ключей
внутри объекта не нормативен.

## Частности, которые кейсы фиксируют

- `fixedIps` только с IPv4 — правило A с `answer` и правило AAAA пустым
  NOERROR без `answer`; ни зеркала на `serverTag`, ни глушилки
  `forceIpv4` (`inline_fixed_v4`);
- IPv4 и IPv6 — оба правила с `answer`, порядок адресов как в записи
  (`inline_fixed_v4_v6`);
- `srs` — то же, что `inline` (`srs_fixed_v4`);
- в `fixedIps` нет ни одного разбираемого адреса — режима нет, зеркало по
  `enabled` / `serverTag` и глушилка, как без `fixedIps`
  (`fixed_unparsable_falls_back`).
- Block HTTPS (`blockHttps`, контракт 1.1.173): правило
  `query_type: [HTTPS, SVCB]` пустым NOERROR — первым из DNS-правил правила
  (`inline_port_block_https`); порядок Block HTTPS → глушилка AAAA → зеркало
  на `serverTag` (`inline_block_https_force_ipv4_server`); при Fixed IP —
  перед парой A/AAAA (`inline_fixed_block_https`); у `srs` без доменных ключей
  в теле — выпускается (`srs_block_https`); у `inline` без доменного условия
  — не выпускается, остальные опции как были (`inline_block_https_no_domains`);
  у `inline` только с IP назначения — ничего (`inline_block_https_ip_only`);
- гейта портов / протоколов / network нет (1.1.173): у правила с `port` или
  `network` опции выпускаются, эти ключи в DNS-правило не идут
  (`inline_port_block_https`, `inline_network_force_ipv4`).
