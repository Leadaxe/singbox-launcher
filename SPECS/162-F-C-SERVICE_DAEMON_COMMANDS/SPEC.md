# SPEC 162 — Окно Service: паспорт платформы, команды демона, troubleshooting аккордеоном

Статус: C (complete, 2026-10-13). Тип: F. Дата: 2026-10-12. Продолжение SPEC 161.

Сторона ядра: SPEC 127 (`/admin/info` с платформой и init), SPEC 128
(`/admin/commands`), SPEC 129 (`sing-box lxd update`) форка sing-box-lx,
ветка `lx`, коммиты `e723bc31a…fd578d935`, тег ещё не срезан.

## 1. Проблема

Окно Service (SPEC 161) опирается на то, что пользователь ввёл руками:

- **Платформа машины** выбирается в окнах Add/Edit. У RouteRich в записи стоит
  `linux/amd64`, роутер на самом деле `arm64`: строка списка врёт, «Download
  core» качает не тот ассет. Сверка с `/admin/host` требует таблицы
  uname→GOARCH и не различает mips softfloat.
- **Init-система** выбирается селектором в шапке, потому что демон её не
  сообщал. Ошибка выбора даёт рецепты не для той системы.
- **Обновление ядра** — пять ssh-шагов. Пользователь без ssh-клиента (Windows)
  или без привычки к консоли не обновится.
- **Тестовая сборка** `1.14.3-lx.15-test128` считается старее `lx.15`:
  неизвестный суффикс после `lx.N` разбирается как пре-релиз. Жёлтый ⚠ на
  шапке ложный.
- **AppTabs** держит минимальную высоту окна по самой высокой вкладке
  (Core): окно не сжимается, когда у пользователя всё в порядке.

Ядро lx.16 (127–129) отдаёт всё это само: платформу и init в паспорте,
именованные команды из `daemon.json` по REST, штатное самообновление.

## 2. Цель

Окно Service становится в первую очередь **штатным пультом**: шапка показывает
платформу и init из паспорта, рядом — селектор команд демона и кнопка Run.
Ssh-раннбук остаётся, но уходит в раздел **Troubleshooting** под шапкой,
аккордеоном: раскрыта одна секция, та, что выбрал диагноз. Когда всё в
порядке, все секции свёрнуты, и окно короткое.

Всё деградирует на старых ядрах без ошибок: нет полей — прежнее поведение,
нет `/admin/commands` — подсказка вместо раздела.

## 3. Принципы

1. **Гейт по факту, а не по версии.** Поддержку 127/128 определяет ответ
   демона (поля в паспорте, статус `/admin/commands`), не сравнение с
   `RequiredCoreVersion`. Тега с 127–129 нет; `RequiredCoreVersion` в этой
   задаче не трогается.
2. **Клиент ничего не передаёт команде.** Ни аргументов, ни тела, ни
   переменных. Это контракт безопасности SPEC 128, не ограничение UI. В
   лаунчере нет ни одного поля ввода, связанного с командами.
3. **Паспорт важнее ввода.** Непустые `goos`/`goarch`/`init_system` из
   паспорта перекрывают запись и селекторы; ручной ввод нужен только до
   первого соединения.
4. **Обрыв при `update` — штатный путь.** Демон перезапускает сам себя; окно
   ждёт возвращения и сверяет версию, а не показывает ошибку.
5. **Пути не хардкодить.** Как в 161: из `/admin/info`, иначе `default`.
6. **`daemon.json` по сети не правится.** Рецепт `commands` копируется в буфер
   и вставляется пользователем руками (SPEC 128 сознательно требует этого).

## 4. Поведение

### 4.1 Паспорт платформы (SPEC 127)

`GET /admin/info` у ядра lx.16+ содержит `goos`, `goarch`, `platform`,
`init_system`, `service`. У ядер старше полей нет: лаунчер читает их как
пустые и ведёт себя как сейчас.

- Паспорт машины (`RemotePassport`) и локального демона кэширует эти пять
  полей.
- При каждом здоровом ответе: если `goos`/`goarch` непусты и отличаются от
  записи машины, запись обновляется, в лог — строка
  `remote registry: <name> platform <old> → <new> (reported by the daemon)`.
  Строка списка машин, шапка Service и визард после этого видят платформу из
  записи, то есть из паспорта.
- **Download core** использует `platform` из паспорта как суффикс ассета:
  `sing-box-<ver>-<platform>.tar.gz` (`.zip` для windows). Без паспорта —
  прежняя таблица `SingboxAssetSuffixFor(goos, goarch)`; таблица остаётся как
  запасной путь; строки в неё добавляются только вместе с новой сборкой
  в релизах форка (linux-386, linux-riscv64 — issue #146).
- **Init:** `init_system` непустой → селектор в шапке не показывается,
  рецепты строятся по нему, запись `init_system` машины перезаписывается
  этим значением (чтобы рецепты были верны и когда демон лёг). Пустой или
  поля нет → селектор как сейчас. `openrc` рецептами не поддержан: шапка
  пишет `openrc`, секции Not running и Core показывают строку «рецептов для
  openrc нет, см. гайд», селектор не показывается.
- **`service:false`** при непустом `init_system`: в шапке
  `service sing-box-lxd (procd, not installed as a service)` жёлтым; в
  секции Not running первым шагом встаёт «Install the service» (это
  существующий рецепт «Install from scratch», поднятый наверх); команда
  `update` в этом случае завершится «binary replaced, restart the service
  yourself» — так и будет показано из лога.
- **Окна Add/Edit:** селекторы платформы остаются. В Edit, если у записи
  есть паспорт с `goos`/`goarch`, селекторы неактивны с подписью
  `reported by the daemon`. Предупреждение о расхождении с `/admin/host`
  (`hostPlatformMismatch`) снимается, когда есть паспорт с платформой;
  без паспорта остаётся.

### 4.2 Шапка: команды демона (SPEC 128)

Под строкой состояния демона появляется строка команд:

```
Daemon command: [ update          ▾ ]  [ Run ]   done · exit 0 · 3 days ago
                /usr/bin/sing-box lxd update
```

- Список — `GET /admin/commands`, отсортирован демоном по имени. Опрос
  идёт вместе с health (heartbeat 5 с удалённо, 5 с локально), без сети в
  окне.
- Селектор показывает имя. Под ним серым — текст команды выбранной записи.
  Справа от Run — последний исход: `idle` / `running` /
  `done · exit N · <когда>` / `lost`. Клиент (`client`) добавляется в
  подсказку.
- **Run** → `POST /admin/commands/<name>` без тела. `202` — запуск; `409` —
  уже идёт, окно переходит к наблюдению за текущим прогоном без ошибки;
  `404` — список перечитывается (команду убрали из конфига).
- После запуска строка становится `Running…`, под ней раскрывается лог
  (моноширинный, только чтение, ≤ 64 КиБ — хвост, как отдаёт демон).
  `GET /admin/commands/<name>` раз в секунду до `done`/`lost`. По завершении
  исход обновляется, лог остаётся раскрытым, кнопка `▴` сворачивает его.
- Чужой прогон (`running` при открытии окна, `client` другой) показывается
  так же: Run неактивна, лог опрашивается.
- **Нет поддержки.** `404` на `GET /admin/commands` (старое ядро или ключа
  нет): селектор показывает одну неактивную запись `not available`, Run
  неактивна, справа подсказка `add "commands" to daemon.json → Troubleshooting
  → Suggested commands`. Демон не отвечает → строка неактивна с подписью
  `the daemon does not answer`.
- Гейт — тот же клиентский сертификат / Bearer, что у остальных admin-маршрутов;
  отдельной настройки нет.
- Локальный демон (macOS) получает ту же строку: `/admin/commands` работает
  и на darwin. На Windows ключ демоном игнорируется, строка показывает
  `not available`.

### 4.3 Команда `update` (SPEC 129)

Команда с именем `update` обрабатывается особо, остальные — общим путём §4.2.

1. Перед запуском — подтверждение: «The daemon will download the latest
   core (~60 MB) and restart itself. Connections through it will drop for
   a few seconds. Continue?».
2. Запоминается `version` из паспорта. POST → 202. Опрос как в §4.2.
3. Обрыв соединения (ошибка GET, heartbeat упал) — не ошибка: строка
   `Restarting the daemon — reconnecting… <N> s`. Переподключение через
   `HealthWithin` с задержками 2 → 5 → 10 с, всего до 90 с. Строка машины
   в списке за это время краснеет честно, как при любом обрыве; об
   ожидаемости знает только окно Service, где строка состояния на время
   ожидания заменяется на «reconnecting».
4. Демон ответил → паспорт перечитан, затем `GET /admin/commands/update`:
   - `200`, `exit_code 0` → `✔ Updated <old> → <new>` или, если версия не
     сменилась, `✔ Already up to date (<ver>)`;
   - `exit_code 1` → `Nothing changed` жёлтым, лог раскрыт;
   - `exit_code 2` → `✖ <new> did not come up, rolled back to <old>`
     красным, лог раскрыт;
   - `404` → `✔ Updated to <new>. Commands are not available in this core
     version` нейтрально: ядро стало версией без 128 (так на стенде), строка
     команд переходит в состояние «нет поддержки».
5. 90 с без ответа → `✖ The daemon did not come back` красным, раскрывается
   секция Not running.

**`check-update`** — обычная команда §4.2, но из лога вынимается строка
`installed … available …` и показывается как исход вместо `done · exit 0`.
Имя `check-update` закреплено рецептом §4.5; команда с другим именем идёт
общим путём.

### 4.4 Troubleshooting аккордеоном

Вкладки `Not running / Core / Pairing / Reference (+ Uninstall)` заменяются
на `widget.Accordion` под заголовком `Troubleshooting · commands run via
ssh <target>` (локально — `commands run on this computer`).

- Секции те же, содержимое то же, что у вкладок 161 §5.2–5.5. Добавляется
  секция **Suggested commands** (§4.5). Порядок: Not running, Core, Pairing,
  Reference, Suggested commands, Uninstall (локально).
- Открыта одна секция (`MultiOpen = false`). При открытии окна раскрывается
  та, которую выбрал диагноз (порядок как сейчас: Not running → Core →
  Pairing), либо явно запрошенная (⚙ из строки машины с `serviceTabCore`).
  Если диагноз чистый — все секции свёрнуты.
- Заголовок несёт глиф и причину: `✖ Not running — connection refused`,
  `⚠ Core — lx.15 is older than required lx.16`, `✖ Pairing — certificate
  changed`. Без проблем — просто имя.
- Тик не угоняет раскрытую секцию (как сейчас не переключает вкладку);
  состояние раскрытия переживает перестройку содержимого.
- Переходы «Open the Core tab» становятся «Open Core» и раскрывают секцию.
- Внутренние свёртки (Roll back, Install from scratch, Remove the service,
  Reinstall the same version) остаются вложенными аккордеонами.
- Секция Core получает первой строкой, когда есть команда `update`:
  «The daemon can update itself: choose `update` above and press Run. The
  steps below are the manual way.»
- Константа `serviceTabScrollMinHeight` и трюк с минимумом прокрутки
  остаются для содержимого секции: одна раскрытая секция в прокрутке.

### 4.5 Suggested commands

Секция Troubleshooting с готовым фрагментом `daemon.json` под init из
паспорта (или из селектора):

```json
"commands": {
  "update":       "<executable> lxd update",
  "update-rc":    "<executable> lxd update --prerelease",
  "check-update": "<executable> lxd update --dry-run",
  "restart":      "<restart-команда init>"
}
```

- `<executable>` — из паспорта, иначе платформенный дефолт с пометкой
  `default`. `restart`: procd `/etc/init.d/sing-box-lxd restart`, systemd
  `systemctl restart sing-box-lxd`, launchd
  `launchctl kickstart -k system/com.leadaxe.sing-box-lxd`. Для openrc и SCM
  фрагмент не предлагается (строка «no recipe for this init»).
- Для darwin/windows строки `update*` не предлагаются (SPEC 129: только
  Linux); остаётся `restart`.
- Кнопка ⧉ копирует фрагмент. Ниже два ssh-шага обычного вида: «Edit
  daemon.json» (`vi <state_dir>/daemon.json`) и «Restart the service»
  (существующий `ServiceStepRestart`).
- Подпись: «Paste into the top level of daemon.json next to "listen". The
  daemon validates names on start: a bad name keeps it from starting.»

### 4.6 Версии с неизвестным суффиксом

`parseCoreBuild`: суффикс после `lx.N` вида `-rc<K>` / `-rc.<K>` — пре-релиз
(как сейчас). Любой другой суффикс (`-test128`, `-dev`) → `ok=false`, то есть
`CoreVersionUnknown`: шапка пишет «cannot compare», ⚠ не ставится, Deploy не
спрашивает. Тест `TestCompareCoreVersion` дополняется случаями.

### 4.7 Чего не делать

- Поля ввода, аргументы, переменные, шаблоны для команд.
- Запись `daemon.json` по сети, `DELETE`/kill команд, история прогонов.
- Менять `RequiredCoreVersion`.
- Новые строки в `SingboxAssetSuffixFor`; таблица остаётся запасным путём.
- Трогать `/admin/host` и его окно.
- Запускать `update` на стенде без явного слова владельца.

## 5. Критерии приёмки

1. На стенде RouteRich (ядро `1.14.3-lx.15-test128`): после Connect запись
   машины становится `linux/arm64`, шапка `OpenWrt · linux/arm64 · service
   sing-box-lxd (procd)`, селектора init нет, ⚠ по версии нет (cannot
   compare). Download core предлагает `sing-box-1.14.3-lx.15-linux-arm64.tar.gz`.
2. Селектор команд содержит `echo` и `update`. Run `echo` → `done · exit 0`,
   лог `platform=linux-arm64 init=procd client=singbox-launcher`.
3. Run `update` (по слову владельца): подтверждение → Running → reconnecting
   → демон вернулся с `1.14.3-lx.15` → `404` на `/admin/commands` → строка
   `✔ Updated to 1.14.3-lx.15. Commands are not available in this core
   version`, селектор в состоянии `not available` с подсказкой.
4. Машина с ядром lx.14 (без 127): поля пустые, селектор init на месте,
   строка команд `not available`, рецепты как в 161.
5. Окно с чистым диагнозом открывается со свёрнутыми секциями и короче, чем
   сейчас; окно с ⚠ Core открывается с раскрытой Core.
6. Local (macOS): строка команд работает через loopback-клиент; без ключа —
   `not available`.
7. `go build ./...`, `TestCompareCoreVersion` с новыми случаями, CI
   `run_mode=tests` зелёный.

## 6. Документация при закрытии

- `docs/release_notes/upcoming.md` (EN/RU).
- `docs/ARCHITECTURE.md`: окно Service — источник платформы, команды демона.
- `docs/TROUBLESHOOTING.md` раздел Daemon: `commands` и Update.
- `RELEASE_NOTES.md`: кнопка Update и платформа из паспорта.
- `bin/locale/ru.json`: все новые ключи.
