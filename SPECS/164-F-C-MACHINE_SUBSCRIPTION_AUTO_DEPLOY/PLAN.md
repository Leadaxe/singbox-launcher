# PLAN 164 — автообновление подписок машин и авто-деплой

Срез develop `f2936415`. Решения сверх SPEC помечены **[решение]**.

## 1. Слои

```
core                                        core/services                      ui
──────────────────────────────────────      ────────────────────────────       ──────────────────────────────
auto_update.go        heartbeat: после      lxd_remote_registry.go             machine_edit_window.go  галка флага
                      локального обхода —     RemoteDaemon +AutoDeploy…,       machine_list_panel.go   ⓘ: Auto-deploy,
                      ac.runMachineSweep      +LastAutoDeploy; SetAutoDeploy,                           Last auto-deploy;
machine_auto_deploy.go (новый)               SetLastAutoDeploy                                         подписка на событие
   runMachineSweep, autoDeployMachine,       lxd_remote_deploy.go  без изм.    configurator/configurator.go
   autoDeployDecision (чистая),                                                   RebuildMachineConfig — регистрируется
   MachineAutoDeployOutcome                 core/events                            как ac.MachineConfigRebuilder
config_service_subscriptions.go              +MachineAutoDeploy                  remote_tab: Reload по событию
   refreshSubscriptionsIn(statePath, mu)
   — общее тело для local и машины
core/debugapi/remote_endpoints.go
   machineView + поле, PATCH + поле,
   POST …/auto-deploy/run
```

`core` не импортирует `ui`: пересборка конфига машины живёт в
`ui/configurator` (презентер Мастера), поэтому в `AppController`
(`core/controller.go`) появляется хук `MachineConfigRebuilder
func(machineID string) (string, error)`, который `ui` ставит при старте.
Без хука обход машины завершается `build_failed` с понятной записью, а не
паникой **[решение]**. Обратное направление (core → ui: уведомление,
перерисовка панели) идёт через `EventBus`, как `NotifyStateChanged` в
`core/debugapi_wiring.go` (SPEC 160 §E) — отдельного хука уведомлений нет.

## 2. `core/services/lxd_remote_registry.go`

### 2.1 Модель
```go
// AutoDeploySubscriptions — SPEC 164: лаунчер сам обновляет подписки
// профиля машины и деплоит конфиг, когда изменились только узлы.
AutoDeploySubscriptions bool `json:"auto_deploy_subscriptions,omitempty"`
// LastAutoDeploy — итог последнего рассмотрения машины обходом (SPEC 164 §4.3).
LastAutoDeploy *AutoDeployRecord `json:"last_auto_deploy,omitempty"`

type AutoDeployRecord struct {
    At     string `json:"at"`               // RFC3339 UTC
    Result string `json:"result"`           // applied|skipped|failed|build_failed|unreachable
    SHA    string `json:"sha,omitempty"`    // applied: sha256 отправленного конфига
    Note   string `json:"note,omitempty"`   // одна строка причины
}
```
Методы по образцу `SetSSH`: `SetAutoDeploySubscriptions(id string, on bool)`,
`SetLastAutoDeploy(id string, rec AutoDeployRecord)`. Оба под `r.mu`,
через `saveLocked`.

### 2.2 Тест
В существующем `lxd_remote_registry_test.go` — не добавлять: одного теста
волны достаточно (§5 ниже), поля проходят через `saveLocked` как все.

## 3. `core/config_service_subscriptions.go`

Тело `RefreshSingleSubscription` (load → find → refreshOneSubscriptionSource
→ Save → MarkConfigStale) разбивается:

```go
// refreshStaleSubscriptionsAt — обход подписок ОДНОГО файла состояния:
// enabled + URL + устаревшие по effectiveReload (или все при force).
// Возвращает число изменивших состав узлов и ошибки фетча по ID.
// mu — мьютекс этого файла (SubscriptionMu у локального, per-machine у машины).
func (svc *ConfigService) refreshStaleSubscriptionsAt(statePath string, mu sync.Locker,
    settings locale.Settings, force bool, now time.Time) (changed int, failed map[string]error)
```
`RefreshSingleSubscription` и `runScheduledRefresh` поведение не меняют:
локальный обход остаётся per-source с retry-таймерами, как есть; новая
функция используется только обходом машин **[решение]** — так локальный
путь, который уже стабилен, не трогается. `sourceIsStale` и
`effectiveReload` из `auto_update.go` используются обеими сторонами.

`MarkConfigStale` для файла машины не зовётся (он про локальный
`config.json`) — признак устаревания машины выражает sha-сравнение.

Мьютексы машин: `AppController.machineStateMu(id) sync.Locker` — карта
`map[string]*sync.Mutex` под собственным мьютексом; Debug API
(`remote_state_endpoints.go: machineMutex`) переключается на неё, чтобы
PATCH состояния машины и обход не писали файл одновременно **[решение]**.

## 4. `core/machine_auto_deploy.go` (новый)

```go
type MachineAutoDeployOutcome struct {
    Result    string // applied|skipped|failed|build_failed|unreachable|not_configured
    SHABefore string
    SHAAfter  string
    Deployed  bool
    Note      string
}

// autoDeployDecision — чистое решение §3.3–3.4 SPEC. Возвращает ok и
// список причин отказа (для журнала), без сети.
func autoDeployDecision(shaBefore, shaAfter string, h services.RemoteHealth,
    coreWarnAck, requiredCore string) (ok bool, reasons []string)

// runMachineSweep — §4.2 SPEC: последовательно по машинам с флагом.
func (ac *AppController) runMachineSweep(trigger string)

// autoDeployMachine — §4.2.2 для одной машины; force — фетчить все подписки.
// Блокирующий, сеть; зовётся из петли и из Debug API.
func (ac *AppController) autoDeployMachine(d services.RemoteDaemon, trigger string, force bool) MachineAutoDeployOutcome
```

Порядок в `autoDeployMachine`:
1. `HealthWithin(id, 10*time.Second)` → `unreachable`.
2. `shaBefore` — sha256 файла `GetRemoteConfigPathFor` ("" → `not_configured`).
3. `refreshStaleSubscriptionsAt(GetWizardStatePathFor(data, remote, id), machineStateMu(id), settings, force, now)`.
   Провал фетча → один `time.AfterFunc(15s)` повтор того же шага без
   деплоя **[решение]** (повтор не деплоит: деплой только из тика, чтобы
   не было двух деплоев в минуту). `changed == 0` → `skipped`, note
   `subscriptions unchanged`.
4. `ac.MachineConfigRebuilder(id)` → `build_failed`.
5. `shaAfter`; равен `shaBefore` → `skipped`, note `config unchanged`.
6. `autoDeployDecision` → `skipped` с причинами через `; `.
7. `Registry.Deploy(id, nil)` → `applied` / `failed`.
8. `SetLastAutoDeploy`, лог `auto-deploy: …`, событие
   `events.MachineAutoDeploy` с payload `{ID, Name, Result, SHA, Note}` на
   КАЖДЫЙ исход кроме `skipped`/`not_configured`; `ui` по нему шлёт
   `fyne.CurrentApp().SendNotification` и перерисовывает панель — `core`
   не тянет Fyne **[решение]**. После записи файла состояния машины (шаг 3,
   `changed > 0`) — `events.StateChanged` с `Target=remote, MachineID=id,
   Source="auto-update"`, чтобы открытый Конфигуратор этой машины предложил
   перечитать (SPEC 160 §E), а не перезаписал свежие узлы своим Save.

Решение (`autoDeployDecision`), причины словами лога (англ., не локализуются —
это журнал):
- `shaBefore != h.ActiveSHA` → `built config differed from active before refresh (manual Deploy expected)`
- `h.CoreStatus != "started"` → `core status <s>`
- `h.ActiveSHA != h.LastGoodSHA` → `active config is not last-good`
- `h.InterruptedApply` → `previous apply was interrupted`
- `CompareCoreVersion(h.Version, requiredCore) == CoreVersionOlder && coreWarnAck != h.Version` → `core <v> older than required <r>`

## 5. `core/auto_update.go`

В `startAutoUpdateLoop` после `ac.runScheduledRefresh(trigger)` (startup и
heartbeat) — `ac.runMachineSweep(trigger)` в той же горутине, с теми же
гейтами (`IsAutoUpdateEnabled`, `IsSleeping`, `IsStorageSwitching`).
VPN-event триггер машин не касается.

Тест волны: `core/machine_auto_deploy_test.go: TestAutoDeployDecision` —
таблица: всё в норме → ok; каждое из пяти условий по отдельности → false с
ожидаемой причиной; `coreWarnAck == version` снимает причину о ядре.

## 6. `core/events`

`const MachineAutoDeploy EventType = "machine_auto_deploy"`, payload
`MachineAutoDeployPayload{ID, Name, Result, SHA, Note string}` в
`payloads.go`; `StateSourceAutoUpdate = "auto-update"` рядом с
`StateSourceDebugAPI`.

## 7. `core/debugapi`

- `machineView` + `AutoDeploySubscriptions bool json:"auto_deploy_subscriptions"`,
  `LastAutoDeploy *services.AutoDeployRecord json:"last_auto_deploy,omitempty"`.
- PATCH `/remote/machines/{id}`: поле `auto_deploy_subscriptions *bool` →
  `SetAutoDeploySubscriptions`.
- `POST /remote/machines/{id}/auto-deploy/run[?force_refresh=1]` →
  `ac.autoDeployMachine(d, "api", force)`; ответ `MachineAutoDeployOutcome`
  как JSON (`result, sha_before, sha_after, deployed, note`). Регистрация в
  `remoteEndpoints()` и `/help`.
- `remote_state_endpoints.go`: `machineMutex(id)` → `s.facade`/`ac.machineStateMu(id)`.

## 8. `ui`

- `machine_edit_window.go`: `widget.NewCheck(locale.T("Auto-deploy subscription updates"), nil)`
  + `widget.NewLabel` подсказки (серый, Wrapping=WordWrap). Save →
  `registry.SetAutoDeploySubscriptions`.
- `machine_list_panel.go: showHealthDetails` — две строки:
  `{locale.T("Auto-deploy"), on/off}`, `{locale.T("Last auto-deploy"), "<result> · <relative time> · <note>"}`
  (относительное время — `formatRelativeAge`).
- Регистрация хука при старте UI (рядом с созданием панели машин):
  `ac.MachineConfigRebuilder = configurator.RebuildMachineConfig`.
- Подписка панели машин на `MachineAutoDeploy`: `fyne.CurrentApp().SendNotification`
  («Auto-deploy: <name> updated» / «… failed: <note>»), затем
  `fyne.Do(p.Reload)`; при `applied` на активной машине — `loadNodes()`
  вкладки Remote. Относительное время в ⓘ — `formatRelativeAge`
  (`ui/core_dashboard_tab_helpers.go`).
- Локаль: новые ключи сразу в `bin/locale/ru.json`.

## 9. Документация

- `docs/API.md` §Remote machines: поле и новый endpoint.
- `docs/ARCHITECTURE.md` §11.9 «Machine subscriptions auto-refresh and
  auto-deploy (SPEC 164)»: петля, критерий sha, хуки `core → ui`.
- `docs/release_notes/upcoming.md` (EN Highlights / RU Основное),
  `RELEASE_NOTES.md` — пункт про галку.

## 10. Проверка вживую

Машина с флагом и подпиской, у которой провайдер ротирует адреса:
`POST /remote/machines/{id}/auto-deploy/run?force_refresh=1` через Debug API
→ `applied` (если адреса сменились с последнего деплоя) или `skipped ·
config unchanged`; на вкладке Remote узлы подписки переходят из `Error` в
миллисекунды. Деплой рвёт трафик машины на секунды — запуск только по слову
владельца.
