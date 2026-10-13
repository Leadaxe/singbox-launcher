# SPEC 165 — Refresh подписки в Конфигураторе машины пишет результат в локальный state.json

Статус: N (new). Тип: B. Дата: 2026-10-13. Найден при закрытии SPEC 164 (§6).

## 1. Симптом

Кнопка Refresh у подписки в Конфигураторе удалённой машины закрепляет
результат фетча на диске по пути **локального** профиля
(`bin/wizard_states/state.json`), а не профиля машины
(`bin/wizard_states/remote/<id>/state.json`). Если в локальном профиле есть
источник с тем же ID, его `nodes`, `update_status`, `meta` и
`pending_disabled` перезаписываются телом, скачанным для машины, и поднимается
признак «локальный конфиг устарел» (`MarkConfigStale`). Если ID в локальном
профиле нет — запись молча пропускается, и на диске машины результат не
появляется до Save.

## 2. Подтверждение

- Код: `ConfigService.RefreshSourceInPlace` → `persistFetchResultForSource`
  (`core/config_service_subscriptions.go`) берёт
  `platform.GetWizardStatePath(dataDir)` безусловно; цели презентера
  (`model.Target`, `model.MachineID`) у него нет. Вызывающие —
  `refreshOneSourceFromUI` (`ui/configurator/tabs/source_tab.go`) и окно
  источника (`source_edit_window.go`) — одинаковы для Local и машины.
- Лог лаунчера, 2026-10-13 11:11:55, Refresh из Конфигуратора машины:
  `persistFetchResultForSource: source <id> not saved by the wizard yet —
  skipped` — путь локальный, пропуск только потому, что ID не совпал.
- Совпадение ID реально: профили машин создаются копией
  (`clone_source.go` из Local, `CopyProfileFrom` между машинами) без
  перевыпуска ULID. В текущих данных у одной машины два ID подписок общие с
  Local; у двух других машин один общий ID между собой.

## 3. Воспроизведение

1. Local и машина M с подпиской одного ID (профиль M скопирован из Local).
2. Открыть Конфигуратор M → Refresh у этой подписки.
3. Сравнить `GET /state/full` и `GET /remote/machines/M/state/full`
   (Debug API): `nodes`/`update_status` источника изменились в **локальном**
   профиле; у M на диске — прежние до Save.

## 4. Ожидаемое

Результат фетча закрепляется в файле состояния той цели, чей Конфигуратор
открыт, под мьютексом этого файла (`SubscriptionMu` для Local,
`AppController.MachineStateLock(id)` для машины, SPEC 164). `MarkConfigStale`
— только для Local; для машины — ничего (её устаревание выражает
sha-сравнение строки машины).

## 5. Набросок исправления

- `ConfigService.RefreshSourceInPlace(src, target, machineID)` (или
  `RefreshSourceInPlaceFor`): путь = `GetWizardStatePathFor(dataDir, target,
  machineID)`, мьютекс по цели; `persistFetchResultForSource` получает путь и
  мьютекс параметрами.
- `business.ConfigServiceInterface` и `ConfigServiceAdapter` передают цель
  из модели презентера; оба вызывающих места в `tabs/` не меняются по
  смыслу.
- Тест (один, data-критичный): два файла состояния с общим ID источника,
  персист для цели `remote/<id>` меняет только файл машины.

## 6. Вне scope

Перевыпуск ULID источников при копировании профиля. Совпадение ID само по
себе не ошибка модели (бэкап адресует источники по ID), и менять это здесь
не нужно — достаточно писать в правильный файл.
