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
  `chain_detour_conflict`, `chain_detour_ring`, `chain_hop_local_port`,
  `chain_hop_tag_taken`, `chain_rewrite_detour`; корпус; TASKS_LXBOX §187.
- **Сборка и редактор** — одна проверка `config.ChainDetourProblems` (одно место на
  тег, кольцо, локальный порт, тег хопа, rewrite с detour; сверена с ядром
  d9fc75468): правило 7 санитайзера снимает цепочку с кодом, редактор цепочки
  показывает причину до сохранения; конфиг удалённой машины перед локальной проверкой проходит гейт под
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

`sing-box check` ядром из чистого дерева `d9fc75468` (сабмодули — ревизии дерева),
15 малых конфигов, сверка одной командой `scratchpad/cfg165/run.sh <бинарь>`:
проходят цепочка + `lx.detour.mtu_align`, поля сокета у узла позиции 2, группа с
`direct`; отвергаются `lx.mtu_align`, `idle_timeout` цепочки, общий первый хоп двух
цепочек, группа в двух цепочках, узел на двух местах, `[A, Группа{A, X}]` и свой detour
узла позиции 0 (апстримное `circular outbound dependency: a -> c#0 -> a`),
`listen_port`, занятый `c#0`, rewrite с detour. `ChainDetourProblems` на тех же
конфигах даёт ровно те же отказы (кроме двух ключей, которые лаунчер не пишет).
Ядро rc.1: `lx.mtu_align` принимает, `lx.detour` отвергает (гейт оставляет ему старую
форму), конфликт двух цепочек принимает.

Все типы узлов лаунчера несут поля дозвона (`DialerOptions`), поэтому «тип без полей
дозвона» сборку не задевает.

## Не сделано / не проверено

- Пин `RequiredCoreVersion` и `daemonpb` по тегу rc2 — после выхода тега.
- Конфиг не прогонялся `sing-box check` ядром ветки SPEC 165; живой прогон цепочки
  и вид окна Info / Core runtime на ядре rc2 — владелец.
- Редактор цепочки конфликт перехвата не показывает (см. SPEC §2.1).
- Копии узлов и групп вместо отказа — ждут решения владельца.
- Ядро SPEC 165 ещё меняется; ветка сверена с d9fc75468.
- Удалённая машина на ядре старше rc2 с цепочками, законными для неё и
  конфликтующими по новым правилам: её конфиг правило 7 не трогает, а
  локальная проверка своим ядром rc2 его отвергнет — Save не пройдёт, пока ядро
  машины не обновлено. У владельца на 26.11 цепочек нет ни в одном состоянии.
