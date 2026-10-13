# TASKS — SPEC 163

- [x] T1 п.1–2: сверить с 1.1.108, правок нет.
- [x] T3 п.3: диф `option/` форка lx.11…lx.17; `body.core` → 1.14.3-lx.17 (22 вхождения); `chrome_155` в `allowlists.json` и трёх списках `tls.json`.
- [x] T4 п.4: код `protocol_extension_unsupported`; `extension_alternative` в `hysteria.json` и `registry.schema.json`.
- [x] T5 п.5: `attachChain` → `*detourReject`; `parseSingboxConfig` → `rejected.addCoded`; тексты/severity трёх кодов; тесты `detour_chain_test.go` переписаны (+ `TestDetourChainUnusableHop`).
- [x] T6 п.6: `masque.json` (note, impl vhttp, маппер, skipped, `$legacy_flat`); `core/state/masque_uri_migration.go` + тест удалены, вызовы в `load_router.go` сняты; golden `uri/masque/legacy_params_ignored` переписан.
- [x] T7 п.7: `tls.json` fragment/record_fragment/fragment_fallback_delay `forbidden_for` + `forbidden_codes`; текст `tls_not_applicable_quic`.
- [x] T8: VERSION, README changelog, TASKS_LXBOX §115, DECISIONS D-128/D-129, gendocs, upcoming.md, IMPLEMENTATION_REPORT.
