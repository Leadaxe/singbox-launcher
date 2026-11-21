# SPEC 181 — Block HTTPS/SVCB: пресет `block-https` и `rules[].dns.blockHttps`

Статус: **N** (19.11.2026, реализовано; UI владельцу не показан). Тип: Feature. Контракт: **1.1.173**, D-150; норма — `contract/docs/TEMPLATE_LANG.md` §6.11 и §6.12 (новый), `BACKUP.md` таблица `rules[]`, `schema/backup.schema.json`, `registry/presets.json`, `registry/allowlists.json`; корпус `contract/corpus/rule_dns/`; LxBox — задача 755, `TASKS_LXBOX.md` §175. Повод — LxBox issue #227.

## Проблема

Без FakeIP Chrome получает из HTTPS-записи DNS (тип 65) ECH-конфиг Cloudflare и прячет имя сайта: сниффер видит внешнее имя `cloudflare-ech.com`, правило `domain_suffix` не срабатывает, трафик уходит по `route.final`. Глушилка HTTPS/SVCB была только внутри пресета `fakeip` (`force`).

Вторая часть: DNS-опции правила (свой сервер, Force IPv4, Fixed IP) были недоступны правилу с `port` / `port_range` / `protocol` / `network` — гейт `rule_dns_mirror_gate`.

## Решение (решения владельца, окончательные)

1. **Пресет `block-https`** в `bin/wizard_template.json`: подпись «Block HTTPS/SVCB records», описание — текст LxBox; одно DNS-правило `{"query_type": ["HTTPS","SVCB"], "action": "predefined", "rcode": "NOERROR"}`; `num: 10` (сразу после `traffic-processing`), `default_enabled` не задан (выключен, существующим не добавляется), не `locked`, сортируемый. `force` в `fakeip` не тронут.
2. **Поле `state.RuleDNS.BlockHTTPS`** (`blockHttps`, только `true`). Выпуск: непустой матчер зеркала И (`srs` ИЛИ у inline ключ списка реестра `rule_dns_block_https_domains`). Не зависит от `Enabled`/`ServerTag`, выпускается и при Fixed IP.
3. **Порядок внутри правила:** Block HTTPS (`query_type`) → Force IPv4 (`ip_version: 6`) → сервер или пара Fixed IP. Место среди `dns.rules` — прежнее (§6.11).
4. **Порты inline-правила** на маршрутном правиле — у лаунчера наборов для inline нет (матчер — inline-ключи), норма для стороны с матчером-набором.
5. **Гейт снят** со всех DNS-аспектов: `rule_dns_mirror_gate` → `rule_dns_mirror_unseen` (ключи не копируются в матчер и не блокируют). В DNS-вкладке окна правила при порте / протоколе / network — подсказка «DNS options apply to these domains on any port and protocol».
6. **UI.** Галка «Block HTTPS/SVCB records (no ECH)» в DNS-вкладке окна правила: доступна, когда сборка её выпустит; у inline ставится сама, когда доменное условие из пустого стало непустым (Form или JSON); при открытии сохранённого правила не трогается; у srs по умолчанию снята. Метка правила на вкладках Rules/DNS — «no HTTPS records».
7. **Принятые риски:** адресный srs под опцией валит старт ядра; `query_type` включает новый DNS-режим ядра 1.14. Предупреждений нет.

## Не входит

`remove_client_subnet`, пометка ECH в профайлере.
