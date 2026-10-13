# IMPLEMENTATION_REPORT 164

Дата: 2026-10-13. Статус: C — реализовано, проверено вживую, CI зелёный (run 38401373436).

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
- CI `run_mode=tests` — первый прогон упал на Ubuntu (`shortSHA` из darwin-файла), после `a55c5b90` зелёный на macOS/Windows/Ubuntu (run 38401373436).

## Живая проверка (PLAN §10)

Выполнена 2026-10-13 на сборке `v2.3.8-30-g9ec12226` по слову владельца:
галка включена в окне Edit, затем `POST …/auto-deploy/run?force_refresh=1`.

- Ответ за 5 с: `result: applied`, `sha_before` = активному конфигу машины на
  момент прогона (владелец деплоил его руками ранее), `sha_after` новый,
  `note: 1 subscription(s) changed`.
- Health после: `core_status: started`, `active_sha == last_good_sha ==
  sha_after`, `interrupted_apply: false`.
- Узлы подписки на вкладке Remote из `Error` перешли в 56–124 мс.
- Журнал машины: `last_auto_deploy {applied, sha, note}`; лог лаунчера —
  строки `remote apply`, `remote deploy`, `auto-deploy[api]: … applied`.
- Уведомление системы и перерисовка строки — по событию `MachineAutoDeploy`.

Статус: реализовано, проверено вживую, CI зелёный.

## Решения сверх SPEC

- Исход `not_configured` (нет собранного конфига) добавлен к списку §4.3:
  без него машина без Configure писалась бы как `skipped`.
- Повтор фетча после провала не публикует событие и не деплоит — только
  освежает файл состояния; деплой всегда с тика.
- `on`/`off` в ⓘ — отдельные ключи локали («вкл»/«выкл»).
