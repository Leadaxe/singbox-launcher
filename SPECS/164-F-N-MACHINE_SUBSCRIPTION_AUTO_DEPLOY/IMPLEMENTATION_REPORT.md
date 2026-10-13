# IMPLEMENTATION_REPORT 164

Дата: 2026-10-13. Статус: реализовано, ждёт живой проверки владельца и CI.

## Что сделано

- `core/services/lxd_remote_registry.go`: `RemoteDaemon` += `AutoDeploySubscriptions`,
  `LastAutoDeploy`; типы `AutoDeployRecord`, `MachineAutoDeployOutcome`, константы
  исходов; `SetAutoDeploySubscriptions`, `SetLastAutoDeploy`.
- `core/events`: `MachineAutoDeploy` + `MachineAutoDeployPayload`, `StateSourceAutoUpdate`.
- `core/controller.go`: хук `MachineConfigRebuildFunc`, `MachineStateLock(id)`.
- `core/config_service_subscriptions.go`: `refreshStaleSubscriptionsAt` — обход
  подписок одного файла состояния под его мьютексом; «изменился состав»
  считается по дайджесту `nodes[]` до и после, провал фетча — в `Failed`.
  Локальный путь не тронут.
- `core/machine_auto_deploy.go`: `runMachineSweep`, `AutoDeployMachine`,
  `autoDeployDecision`, повтор фетча через 15 с без деплоя.
  `core/auto_update.go`: обход машин после локального на startup и heartbeat.
- `core/debugapi`: `RemoteAPI.AutoDeployRun`, `RemoteAPI.MachineStateLock`
  (общий мьютекс с обходом), поля в `machineView`, `PATCH` принимает
  `auto_deploy_subscriptions`, `POST /remote/machines/{id}/auto-deploy/run`.
- `ui/machine_edit_window.go`: галка с подсказкой; `ui/machine_list_panel.go`:
  хук пересборки, подписка на `MachineAutoDeploy` → уведомление системы +
  Reload / loadNodes, строки «Auto-deploy» и «Last auto-deploy» в ⓘ.
- `bin/locale/ru.json`: 13 ключей. Доки: `docs/API.md`, `docs/ARCHITECTURE.md`
  §11.9, `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`.

## Проверки

- `go build ./...` — ок.
- `go test ./core -run TestAutoDeployDecision -count=1` — ок (9 случаев).
- `l10n_check --strict`, `hardcoded_check --strict`, `win7guard` — чисто.
- CI `run_mode=tests` — после пуша develop.

## Живая проверка (PLAN §10)

Не выполнялась: деплой на машину — по слову владельца. Сценарий: включить
галку в Edit, `POST /remote/machines/{id}/auto-deploy/run?force_refresh=1`
через Debug API, ожидать `applied` или `skipped · config unchanged`.
Требует перезапуска лаунчера на новой сборке.

## Решения сверх SPEC

- Исход `not_configured` (нет собранного конфига) добавлен к списку §4.3:
  без него машина без Configure писалась бы как `skipped`.
- Повтор фетча после провала не публикует событие и не деплоит — только
  освежает файл состояния; деплой всегда с тика.
- `on`/`off` в ⓘ — отдельные ключи локали («вкл»/«выкл»).
