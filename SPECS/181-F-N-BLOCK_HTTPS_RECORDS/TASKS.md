# TASKS 181

- [x] T1 `core/state/rule_types.go`: `RuleDNS.BlockHTTPS`, `IsZero`
- [x] T2 `core/build/rule_dns_mirror.go`, `preset_merge.go`: тело Block HTTPS первым, `RuleDNSBlockHTTPSAvailable`, `RuleDNSMirrorNarrowed`; гейт снят
- [x] T3 `contract/registry/allowlists.json`: `rule_dns_mirror_unseen`, `rule_dns_block_https_domains`; `presets.json`: `block-https`
- [x] T4 `bin/wizard_template.json`: пресет `block-https`
- [x] T5 Окно правила: галка, автоустановка, подсказка про порты; вкладка DNS — метка; `locale.T` + `bin/locale/ru.json`
- [x] T6 Контракт 1.1.173: §6.11, §6.12, D-150, `BACKUP.md`, схема, README, `TASKS_LXBOX.md` §175, `DECISIONS.md`
- [x] T7 Корпус `rule_dns` +7, раннер `TestContractCorpusRuleDNS`; `TestRuleDNSMirror` расширен пресетом и портом
- [x] T8 `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md`
- [ ] T9 Показ UI владельцу; CI по `ci.yml`
