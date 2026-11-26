# TASKS 189

- [ ] 1. gRPC: `internal/daemonpb` по ветке ядра SPEC 165; `core/backend.go`, `core/backend_daemon.go`, `core/services/lxd_remote_transport.go`, `core/chain_probe.go`, `core/debugapi/chain_endpoints.go`, `ui/servers_node_info_chain.go`; методы `OutboundConfig(tag)` / `EffectiveConfig()`.
- [ ] 2. Контракт: `max_core` (`contract/schema`, `contract/registry/core_config.json`, `core/config/registry`, `core/config/nodeflow/config_gate.go`); `protocols/chain.json`; коды в `warnings.json`; корпус; `contract/VERSION`, `contract/README.md`, `contract/TASKS_LXBOX.md`.
- [ ] 3. Сборка: `core/config/configtypes/chain_body.go`, `types.go`, `core/config/chain_generator.go`, `chain_validate.go`, `nodeflow/sanitize.go`; `core/state/sources_v7.go`; `core/backup/file.go`; `bin/wizard_template.json`; `ui/configurator/presentation/draft_reject.go`.
- [ ] 4. UI: `ui/configurator/tabs/source_chain_tab.go`, `source_edit_window.go`; скрытие `chain-hop`; вкладка JSON в `ui/servers_node_info.go` (после Р3); `bin/locale/ru.json`.
- [ ] 5. Пин rc2, `daemonpb` по тегу.
- [ ] 6. Доки и release notes; тест сборки цепочки под rc2/rc.1.
