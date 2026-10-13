# PLAN — SPEC 163

Разбор семи пунктов аудита LxBox §591 после решений владельца 13.10.2026 (п.5 — да; п.6 — обратную совместимость `network` снять полностью, параметр освободить под `udp|tcp` как в sing-box, новое не делать; остальное — да).

| П. | Решение | Где |
|---|---|---|
| 1, 2 | Уже закрыты контрактом 1.1.108 (§105); повторная сверка, правок нет | — |
| 3 | `body.core` → 1.14.3-lx.17 после дифа `option/` lx.11…lx.17; `chrome_155` в allowlist и `tls.json` | `registry/*.json`, `allowlists.json`, `tls.json` |
| 4 | Код `protocol_extension_unsupported {scheme, alternative}` + поле `extension_alternative` | `warnings.json`, `hysteria.json`, `registry.schema.json` |
| 5 | Fail-closed при импорте sing-box: `attachChain` возвращает отказ, `parseSingboxConfig` отбраковывает; тексты трёх кодов | `core/config/subscription/detour_chain.go`, `singbox_import.go`, `warnings.json` |
| 6 | `network` не legacy-алиас: `$legacy_flat`, маппер, `skipped`, note, impl vhttp; миграция состояния снимается | `masque.json`, `core/state/masque_uri_migration*.go`, `load_router.go`, корпус |
| 7 | `forbidden_for` + `forbidden_codes` у fragment / record_fragment / fragment_fallback_delay | `tls.json`, текст `tls_not_applicable_quic` |

Закрытие: VERSION 1.1.118, changelog README, TASKS_LXBOX §115, DECISIONS D-128/D-129, gendocs, upcoming.md.
