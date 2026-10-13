# TASKS 164

Один исполнитель, ветка `develop`, коммиты своими путями
(`git commit -m "…" -- <пути>`). Локально: `go build ./...` и только
именованный тест волны (`-run TestAutoDeployDecision`). Полный прогон — CI
`gh workflow run ci.yml --ref develop -f run_mode=tests` в конце.
Общий код без `min`/`max`, `slices`, `maps` (go1.20, Win7).

## A — core и services (без UI)
- [ ] A1 `core/services/lxd_remote_registry.go`: `AutoDeploySubscriptions`, `LastAutoDeploy`, `AutoDeployRecord`, `SetAutoDeploySubscriptions`, `SetLastAutoDeploy`.
- [ ] A2 `core/config_service_subscriptions.go`: `refreshStaleSubscriptionsAt(statePath, mu, settings, force, now)`; локальный путь не меняется.
- [ ] A3 `core/controller.go`: `machineStateMu(id)`, хук `MachineConfigRebuilder`.
- [ ] A4 `core/machine_auto_deploy.go`: `autoDeployDecision`, `autoDeployMachine`, `runMachineSweep`, `MachineAutoDeployOutcome`; тест `core/machine_auto_deploy_test.go: TestAutoDeployDecision`.
- [ ] A5 `core/auto_update.go`: вызов `runMachineSweep` после локального обхода на startup и heartbeat.
- [ ] A6 `core/events`: `MachineAutoDeploy` + payload, `StateSourceAutoUpdate`.
- [ ] A7 `core/debugapi/remote_endpoints.go`, `remote_state_endpoints.go`: поле в view/PATCH, `POST …/auto-deploy/run`, per-machine мьютекс из контроллера, `/help`.
- Проверка: `go build ./...`, `go test ./core -run TestAutoDeployDecision -count=1`.

## B — UI
- [ ] B1 `ui/machine_edit_window.go`: галка + подсказка, Save.
- [ ] B2 `ui/machine_list_panel.go`: строки в ⓘ; подписка на `MachineAutoDeploy` → SendNotification, Reload / loadNodes.
- [ ] B3 регистрация хука `MachineConfigRebuilder` при старте UI.
- [ ] B4 `bin/locale/ru.json` — новые ключи.
- Проверка: `go build ./...`; вёрстку смотрит владелец.

## C — закрытие
- [ ] C1 `go run ./tools/l10n/l10n_check --strict`; `go run ./tools/l10n/hardcoded_check --strict`; `go run ./tools/win7guard`.
- [ ] C2 `docs/API.md`, `docs/ARCHITECTURE.md` §11.9, `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`.
- [ ] C3 Живая проверка по слову владельца (PLAN §10); `IMPLEMENTATION_REPORT.md`; CI `run_mode=tests`; папка → `164-F-C-…`.
