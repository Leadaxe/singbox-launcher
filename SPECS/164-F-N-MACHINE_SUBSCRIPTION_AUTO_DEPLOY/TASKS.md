# TASKS 164

Один исполнитель, ветка `develop`, коммиты своими путями
(`git commit -m "…" -- <пути>`). Локально: `go build ./...` и только
именованный тест волны (`-run TestAutoDeployDecision`). Полный прогон — CI
`gh workflow run ci.yml --ref develop -f run_mode=tests` в конце.
Общий код без `min`/`max`, `slices`, `maps` (go1.20, Win7).

## A — core и services (без UI)
- [x] A1 `core/services/lxd_remote_registry.go`: `AutoDeploySubscriptions`, `LastAutoDeploy`, `AutoDeployRecord`, `SetAutoDeploySubscriptions`, `SetLastAutoDeploy`.
- [x] A2 `core/config_service_subscriptions.go`: `refreshStaleSubscriptionsAt(statePath, mu, settings, force, now)`; локальный путь не меняется.
- [x] A3 `core/controller.go`: `machineStateMu(id)`, хук `MachineConfigRebuilder`.
- [x] A4 `core/machine_auto_deploy.go`: `autoDeployDecision`, `autoDeployMachine`, `runMachineSweep`, `MachineAutoDeployOutcome`; тест `core/machine_auto_deploy_test.go: TestAutoDeployDecision`.
- [x] A5 `core/auto_update.go`: вызов `runMachineSweep` после локального обхода на startup и heartbeat.
- [x] A6 `core/events`: `MachineAutoDeploy` + payload, `StateSourceAutoUpdate`.
- [x] A7 `core/debugapi/remote_endpoints.go`, `remote_state_endpoints.go`: поле в view/PATCH, `POST …/auto-deploy/run`, per-machine мьютекс из контроллера, `/help`.
- Проверка: `go build ./...`, `go test ./core -run TestAutoDeployDecision -count=1`.

## B — UI
- [x] B1 `ui/machine_edit_window.go`: галка + подсказка, Save.
- [x] B2 `ui/machine_list_panel.go`: строки в ⓘ; подписка на `MachineAutoDeploy` → SendNotification, Reload / loadNodes.
- [x] B3 регистрация хука `MachineConfigRebuilder` при старте UI.
- [x] B4 `bin/locale/ru.json` — новые ключи.
- Проверка: `go build ./...`; вёрстку смотрит владелец.

## C — закрытие
- [x] C1 `go run ./tools/l10n/l10n_check --strict`; `go run ./tools/l10n/hardcoded_check --strict`; `go run ./tools/win7guard`.
- [x] C2 `docs/API.md`, `docs/ARCHITECTURE.md` §11.9, `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`.
- [x] C3 Живая проверка по слову владельца (PLAN §10) — applied, узлы ожили; `IMPLEMENTATION_REPORT.md` заполнен.
- [ ] C4 CI `run_mode=tests` после пуша develop; папка → `164-F-C-…`.
