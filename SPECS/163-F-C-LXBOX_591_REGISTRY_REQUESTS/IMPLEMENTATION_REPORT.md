# IMPLEMENTATION_REPORT — SPEC 163

Дата: 2026-10-13. Статус: C. Контракт 1.1.116 → 1.1.118 (1.1.117 между ними — поле `tls` стороны LxBox, §114, вне этой задачи).

## Решения владельца (13.10.2026)

- п.5 — да: висячий detour импортируемого sing-box конфига = fail-closed (D-128).
- п.6 — обратная совместимость `network` у masque снимается полностью, старые ссылки не читаются и не мигрируются, ключ освобождён под `udp|tcp` как в sing-box, новое не реализуем (D-129).
- Остальное — да.

## Что сделано

| П. | Итог |
|---|---|
| 1 naive | Закрыт в 1.1.108; сверено повторно (note верен, `omit_default` только `quic_congestion_control` → Go пишет 443). Правок нет |
| 2 tailscale | Закрыт в 1.1.108; сверено. Правок нет |
| 3 body.core | Диф `option/` форка lx.11…lx.17: `failover` (уже 1.1.111), снят `passive_check`, uTLS `chrome_155` (с lx.13), комментарии AWG. `body.core` → 1.14.3-lx.17 (22 вхождения); `chrome_155` в `allowlists.json` и трёх списках `tls.json` |
| 4 hysteria | Код `protocol_extension_unsupported {scheme, alternative}`; `extension_alternative: hysteria2` в `hysteria.json`, свойство в `registry.schema.json` |
| 5 detour | `attachChain` возвращает `*detourReject`; `parseSingboxConfig` отбраковывает носителя (`rejected.addCoded`, узел `kind=unsupported` на позиции). Коды — severity error, тексты переписаны. Хоп без узла → `detour_target_missing`; кольцо/самоссылка роняют всех; `direct` и предел глубины — как прежде |
| 6 masque | `masque.json`: note, impl vhttp, маппер без `network`, `skipped.network` = «зарезервировано, не читается», `$legacy_flat` только `server_name`. Удалены `core/state/masque_uri_migration.go` и тест, вызовы в `load_router.go` сняты. Golden `uri/masque/legacy_params_ignored`: две записи `uri_param_unknown` (`$legacy_flat` с value, `network`) |
| 7 tls | `fragment`, `record_fragment`, `fragment_fallback_delay`: `forbidden_for` += hysteria, hysteria2, tuic; `forbidden_codes` → `tls_not_applicable_quic`; текст кода расширен. masque не включён (правила владельца свои) |

## Проверки

- `go build ./...` — ок.
- `TestDetourChain*` (переписаны под fail-closed, + `TestDetourChainUnusableHop`), `TestRegistrySync*`, `TestRegistryWarningCodes*`, `TestMasque_LegacyParamsIgnored`, `TestContractCorpusURI|Body|Authored` — зелёные.
- CI `run_mode=tests` — запущен после коммита.

## Известное поведение ядра

`protocol/masque/legacy_options_lx.go` в 1.14.3-lx.17 ещё читает `network` и падает при расхождении с `vhttp`. Общий стрип `unknown_key` до ядра такое тело не пропускает. Снос legacy в ядре и переименование `network_list` → `network` — отдельная задача ядра, в этой волне не делается.

## Документы

`contract/README.md` (1.1.118), `contract/TASKS_LXBOX.md` §115, `SPECS/103…/DECISIONS.md` D-128/D-129, `contract/docs/generated/*` (gendocs), `docs/release_notes/upcoming.md`.
