# PLAN 189

Порядок волн; каждая собирается сама (`go build ./...`), коммит на волну.

1. **gRPC.** `internal/daemonpb` по ветке ядра `lx-spec165-chain-detour` (по тегу rc2,
   когда выйдет). `chainSource`: `ChainFor(tag)` → `GetChainStatus`; `Chains()` для
   списков — по тегам цепочек из своего конфига, у удалённой машины — из
   `GetRunningConfig`. `SetChainPositionEnabled` без `warmupError` (сигнатура
   `(string, error)` → `error`, UI-тост прогрева уходит). Поля звеньев — из
   `core/chain_probe.go`, `core/debugapi/chain_endpoints.go`, секции цепочки окна Info.
   Новые методы бэкенда и транспорта: `OutboundConfig(tag)`, `EffectiveConfig()`.
2. **Контракт.** `max_core` в `registry/core_config.json` (схема, загрузчик
   `core/config/registry`, исполнитель `nodeflow.GateConfigForCore`, неизвестная версия
   снимает ключ); `lx.mtu_align` — `max_core 1.14.4-lx.37-rc.2`, `lx.detour` и
   `lx.detour.mtu_align` — `min_core 1.14.4-lx.37-rc.2`. Тело цепочки: из `chain.json`
   уходят `idle_timeout`, `strip_evasion`, `strip`, `__dialer`; импорт бэкапа снимает
   их с кодом. Коды предупреждений: снятые поля К5, цепочка снята конфликтом
   перехвата. Корпус: `direction/chain_strip_and_timeout`, `backup/chains_roundtrip`,
   затронутые `backup/v10_*`. `contract/VERSION`, `README.md` (строка версии),
   `TASKS_LXBOX.md` §187.
3. **Сборка.** `ChainBody` / `ChainFromBody` / `ChainOutboundObject` без трёх ключей;
   state v7: снятие при загрузке (`core/state`), бэкап (`core/backup`).
   Шаблон: `lx.detour.mtu_align` рядом с `lx.mtu_align`. Санитайзер графа: поля К5 у
   узлов позиций ≥ 1, конфликты К1/К2/К4/К6 (`core/config/nodeflow/sanitize.go`,
   `core/config/chain_validate.go`). Локальная проверка конфига удалённой машины —
   через гейт под версию своего ядра (`draft_reject.go`).
4. **UI.** Редактор цепочки без трёх полей, с причиной конфликта перехвата; `chain-hop`
   скрыт по типу; вкладка JSON окна Info из `GetOutboundConfig` (после согласования
   макета, Р3).
5. **Пин** `1.14.4-lx.37-rc.2` и `daemonpb` по тегу — после выхода rc2.
6. **Доки:** `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md` (если меняется
   поток цепочки), `docs/ParserConfig.md` / `docs/WIZARD_STATE.ru.md` (поля цепочки).

Тест (один на волну, data-критичный): сборка конфига с цепочкой под версию ядра rc2 и
rc.1 — ключи цепочки, `lx.detour`/`lx.mtu_align`, поля К5, конфликт перехвата.
