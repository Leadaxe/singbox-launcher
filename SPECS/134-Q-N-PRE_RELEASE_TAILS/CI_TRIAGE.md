# CI triage · прогоны 35426077879 и 35426488431

Дата разбора: 2026-09-19. Код не менялся — только чтение логов `gh run view`.

## Сводка

| Run ID | SHA | Статус | Красных тестов |
|--------|-----|--------|----------------|
| [35426077879](https://github.com/Leadaxe/singbox-launcher/actions/runs/35426077879) | `1d32af49` | success | **0** |
| [35426488431](https://github.com/Leadaxe/singbox-launcher/actions/runs/35426488431) | `fe61dfb7` | success | **0** |

Оба прогона: `workflow_dispatch`, `run_mode=tests`. Все три джобы тестов (ubuntu-24.04, macos-latest, windows-latest) — зелёные. `gh run view <id> --log-failed` — пусто. В полных логах нет строк `--- FAIL:`.

---

## Прогон 35426077879 (`1d32af49`)

Коммит: `refactor(spec133): боевой путь эмита на движке, рукописные эмиттеры сняты`.

**Красных тестов нет.**

---

## Прогон 35426488431 (`fe61dfb7`)

Коммит: `docs(contract): вычитка текстов кодов предупреждений en/ru`.

**Красных тестов нет.**

---

## Делегировано другому исполнителю (не красное в этих прогонах)

В обоих прогонах `TestRegistryWarningCodesAreActuallySet` — **PASS** на всех ОС.

Известный хвост (разбирает другой исполнитель): при снятии Go-констант `WarnXHTTPModeForcedPacketUp` / `WarnXHTTPParamReset` из `parse_warnings.go` (коды `xhttp_mode_forced_packet_up`, `xhttp_param_reset`) без учёта правил реестра тест начнёт падать ложно — коды **ставятся движком** по `contract/registry/transports.json` (`on_implies_written`, `on_when_false`), а не парсером.

| Пакет | Тест | ОС (когда упадёт) | Суть ошибки | Первый подозреваемый коммит | Вердикт |
|-------|------|-------------------|-------------|----------------------------|---------|
| `core/config/subscription` | `TestRegistryWarningCodesAreActuallySet` | все | `код "xhttp_mode_forced_packet_up" / "xhttp_param_reset" … не ставится нигде в коде` при удалении констант без `registrySets` в тесте | `1d32af49` (снятие рукописного транспорта; константы ещё живы, ссылка осталась в `node_parser_transport.go`) | **устаревшее ожидание теста** (коды живы в реестре, корпус `contract/corpus/uri/vless/xhttp_*` зелёный) |

**Статус в прогонах 35426077879 / 35426488431:** не красный; правка — в работе у другого исполнителя (`parse_warnings.go`, `registry_sync_test.go`).

---

## Контекст (соседние красные прогоны, не в объёме)

Для справки — последний красный прогон до зелёной серии: [35420772672](https://github.com/Leadaxe/singbox-launcher/actions/runs/35420772672) (`b57625ea`) — `FAIL singbox-launcher/ui [build failed]` (не тестовый assert). К прогонам 35426077879 / 35426488431 не относится.
