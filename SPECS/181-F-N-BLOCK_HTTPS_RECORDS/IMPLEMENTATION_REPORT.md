# IMPLEMENTATION REPORT 181

**Итог:** пресет `block-https` и поле правила `dns.blockHttps` по контракту 1.1.173 (D-150); DNS-опции правила больше не гейтятся портами / протоколами / network.

## Что сделано

- `core/state/rule_types.go`: `RuleDNS.BlockHTTPS` (`blockHttps`, omitempty), учтено в `IsZero`; клон, бэкап и диск — через общий тип, без отдельного кода.
- `core/build/rule_dns_mirror.go`: тело `M + {query_type: [HTTPS, SVCB], action: predefined, rcode: NOERROR}` первым у правила; `RuleDNSBlockHTTPSAvailable` (srs или доменный ключ из `rule_dns_block_https_domains`), `RuleDNSMirrorNarrowed` (ключ из `rule_dns_mirror_unseen`); проверка гейта из `RuleDNSMirrorMatch` убрана. `preset_merge.go` — вставка тела в `dns.rules`.
- `bin/wizard_template.json`: пресет `block-https`, `num: 10`, текст описания — от LxBox дословно.
- Окно правила: галка «Block HTTPS/SVCB records (no ECH)», автоустановка при переходе доменов из пустого в непустое, подсказка про порты; новые тексты недоступности. Вкладки Rules/DNS: метка «no HTTPS records». 5 ключей `locale.T` (2 заменили старые).
- Контракт 1.1.173 и корпус `rule_dns` (+7). `TestContractCorpusRuleDNS` 11/11, `TestRuleDNSMirror` — с пресетом и правилом с портом.

## Отступления

- Подсказка «Works only if the rule set contains domains.» у srs оставлена как была.
- Текст недоступности опций заменён: порты больше не причина («Unavailable for rules that match only IP addresses, ports or protocols.» — правило без условия, видимого DNS).

## Не проверено

- UI окна правила вживую (сборка `go build`, показ владельцу — T9).
- CI полным прогоном.
