# IMPLEMENTATION_REPORT 165

Дата: 2026-10-13. Статус: C — исправлено, тест зелёный.

## Изменения

- `core/config_service_subscriptions.go`: `RefreshSourceInPlace` стал
  обёрткой над `RefreshSourceInPlaceFor(src, target, machineID)`;
  `persistFetchResultForSource` получает цель: для `remote/<id>` — файл
  машины и `AppController.MachineStateLock(id)`, для Local — прежние
  `state.json` и `SubscriptionMu`. `MarkConfigStale` только для Local.
- `ui/configurator/business/config_service.go`: адаптер несёт `Target` и
  `MachineID`, `RefreshSourceInPlace` проксирует их в core.
- `ui/configurator/presentation/presenter.go`: `ConfigServiceAdapter()`
  заполняет цель из модели презентера (`Target`, `MachineID`). Оба
  вызывающих места Refresh (`source_tab.go`, `source_edit_window.go`) не
  менялись: они и так берут адаптер у презентера.
- Тест `core/config_service_subscriptions_persist_test.go:
  TestPersistFetchResultTargetsConfiguratorState` — два файла с общим ID
  источника: персист для машины меняет только её файл, для Local — только
  локальный.

## Проверки

- `go build ./...` — ок; `go test ./core -run TestPersistFetchResultTargetsConfiguratorState -count=1` — ок.
- CI `run_mode=tests` — зелёный на macOS/Ubuntu/Windows (run 38433733326).

## Вне scope (как в SPEC §6)

Перевыпуск ULID при копировании профиля не делался.
