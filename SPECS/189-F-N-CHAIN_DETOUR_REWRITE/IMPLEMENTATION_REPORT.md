# IMPLEMENTATION_REPORT 189

Ветка `spec189-chain-detour-rewrite` (26.11.2026), в `develop` не влита: ждёт тега ядра
`v1.14.4-lx.37-rc.2`.

## Сделано

- **gRPC** — `internal/daemonpb` по ветке ядра `lx-spec165-chain-detour`
  (`SYNC_REV`); состояние цепочки по тегу (`GetChainStatus`), тумблер позиции без
  прогрева, без полей звеньев; `GetOutboundConfig(tag)` / `GetEffectiveConfig` у
  своего демона и удалённой машины; debug API `/chains` и `/chains/{tag}/probe`.
- **Контракт 1.1.184** — тело цепочки без `idle_timeout` / `strip_evasion` / `strip`
  (снимаются при загрузке состояния и импорте, не пишутся); `max_core` в
  `core_config.json`; `lx.mtu_align` → `lx.detour.mtu_align`; коды
  `chain_detour_conflict`, `chain_hop_socket_fields_dropped`; корпус; TASKS_LXBOX §187.
- **Сборка** — правило 7 санитайзера графа (К1, К2, К5, К6 ядра) для ядра с
  перезаписью; конфиг удалённой машины перед локальной проверкой проходит гейт под
  версию своего ядра.
- **UI** — редактор цепочки без простоя и фильтров; вкладка JSON окна Info из ядра;
  вкладка Effective config в Core runtime; `chain-hop` не попадает в список узлов
  удалённой машины.

## Проверки

Локально: `go build ./...`; по имени — `TestSanitizeChainDetourRewrite` и соседние
`TestSanitize*`, `TestGateConfigForCore`, `TestContractCorpusDirection`,
`TestRegistry*`, `TestChain*`, тесты `core/backup` и `core/state` по цепочкам,
`TestWizardTemplateConfigUnchanged`; `l10n_check`, `hardcoded_check`, `paths_guard`.
Полный прогон — CI ветки.

## Не сделано / не проверено

- Пин `RequiredCoreVersion` и `daemonpb` по тегу rc2 — после выхода тега.
- Конфиг не прогонялся `sing-box check` ядром ветки SPEC 165; живой прогон цепочки
  и вид окна Info / Core runtime на ядре rc2 — владелец.
- Редактор цепочки конфликт перехвата не показывает (см. SPEC §2.1).
