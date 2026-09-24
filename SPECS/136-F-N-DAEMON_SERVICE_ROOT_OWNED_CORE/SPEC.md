# 136 · Служба демона на root-owned копии ядра

Парная задача к SPEC 100 форка ядра (`sing-box-lx`, релиз **v1.14.1-lx.11**).
Решение владельца — 24.09.2026. Заменяет сверку путей из SPEC 135 §5.1.
Та же копия ядра — у classic-старта с TUN: [SPEC 137](../137-F-N-CLASSIC_TUN_ROOT_OWNED_CORE/SPEC.md).

Статус: **реализовано в ветке `spec-136-daemon-root-owned`, ждёт сборки
ядра lx.11, ручной проверки владельцем и CI**.

## 1. Проблема

`sudo <SingboxPath> lxd --service=install` пишет в
`/Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist` путь к **тому бинарю,
которым его вызвали**: ядро в бандле (`/Applications/…app/Contents/MacOS/bin/sing-box`)
или в DataDir (`~/Library/Application Support/singbox-launcher/bin/sing-box`).
Оба файла пишутся от имени пользователя, а launchd запускает их от root
(`RunAtLoad`, `KeepAlive`). Любая программа, работающая от имени
пользователя, подменяет файл и на ближайшем перезапуске службы (перезагрузка,
падение, `kickstart`) получает root. Это локальное повышение привилегий, и
лаунчер сам предлагает его установить.

Сверка путей SPEC 135 §5.1 (plist ↔ `SingboxPath`) ловила только переезд
данных и считала «всё хорошо» ровно опасный случай — plist на ядро лаунчера.

## 2. Решение (владелец, 24.09.2026)

- Ядро lx.11 на `lxd --service=install` **копирует себя** в root-owned
  канонический файл и переписывает plist на копию. Копирует ядро, а не
  shell-команда лаунчера; в лаунчере копирования нет.
- Лаунчер классифицирует состояние службы (§4) и для любого непорядка
  показывает **одну** sudo-команду «Install or update service» (§5).
- Старый plist (на пользовательский бинарь) — громкое предупреждение, но
  daemon-движок **не блокируется**: у пользователя работающий VPN, а ремонт —
  одна команда.

## 3. Интерфейс ядра lx.11 (зафиксирован)

| Что | Значение |
|---|---|
| Копия ядра | **плоский файл** `/Library/PrivilegedHelperTools/sing-box-lxd` — `root:wheel 0755`; каталога службы нет (решение владельца 24.09.2026). Имя файла `sing-box-lxd` (решение владельца 24.09.2026, ядро **lx.12**; метка службы, plist и каталог `/Library/PrivilegedHelperTools` прежние), так же зовётся процесс ядра под root — 12 символов, `p_comm` не усекает; не `sing-box`: лаунчер ищет его по командной строке (`platform.PrivilegedPkillPattern`, альтернатива `sing-box-lxd run` — «sing-box run» её не ловит) и по имени (`platform.IsPrivilegedCoreProcessName`) |
| Сайдкар | `/Library/PrivilegedHelperTools/sing-box-lxd.install.json` — `root:wheel 0644`: `{source, sha256, version, installed_at, plist_path, label}` |
| plist | `ProgramArguments[0]` = копия; остальные ключи plist не меняются |
| `lxd --service=install` | идемпотентен по sha (равные sha — копия не трогается); `daemon.json`, секрет и клиенты сохраняются; служба перезапускается (`bootout` + `bootstrap`): ядро ждёт выгрузки старой службы до 10 с и повторяет `bootstrap` |
| `lxd --service=uninstall` | по умолчанию снимает plist, launchd **и копию** с сайдкаром; `--keep-copy` — снимает plist и launchd, копию и сайдкар оставляет (состояние COPY ONLY); `--purge` — ещё и данные демона |
| `lxd --service=status` | exit 0 — OK, 2 — MISMATCH или UNSAFE, 3 — NOT INSTALLED, 4 — COPY ONLY (копия без службы, `--service=copy` SPEC 137), 5 — NOT RUNNING (на диске всё как OK, но launchd: not loaded или state ≠ running; подсказка `sudo launchctl bootstrap system <plist>`), 1 — ошибка; тяжесть OK < COPY ONLY < NOT RUNNING < MISMATCH < UNSAFE; печатает путь, sha256 копии, sha256 вызывающего бинаря, вердикт (ядро `11f440685`) |
| `GET /admin/info` | новые поля `executable`, `executable_sha256`; у ядер до lx.11 их нет — лаунчер проверяет наличие |

Всё читается без sudo: каталог и файл `0755`, сайдкар и plist `0644`.
`--service=status` лаунчер не вызывает (своя классификация без запуска
процесса, работает и со старым ядром); команда — для ручной проверки (§9).

## 4. Классификатор состояния службы

Вердикт — первый сработавший сверху вниз:

| Состояние | Условие | Показ |
|---|---|---|
| **NotInstalled** | plist нет | ничего; вкладка Install |
| **Unsafe** | plist не разобрался; **или** `ProgramArguments[0]` ≠ каноническая копия (бандл, DataDir, что угодно); **или** цепочка `/Library` → `/Library/PrivilegedHelperTools` → файл копии не проходит инвариант (на месте файла — каталог: причина «is a directory, not the copy file: remove it»); plist на копию ранней раскладки dev-сборок lx.11 — каталог `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/` (`…/sing-box` внутри) или плоский файл `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd` — Unsafe с причиной «legacy layout of early lx.11 builds: run Install or update service, then remove it (`sudo rm -rf /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd.install.json`)» | красная плашка, модальное предупреждение раз на версию лаунчера, WARN перед apply |
| **Stale** | путь канонический и безопасный, но sha256(копии) ≠ sha256(`EvalSymlinks(SingboxPath)`); **или** копии нет | жёлтая плашка |
| **NotRunning** | файлы как у OK (plist на безопасную копию, sha совпал), но `launchctl print system/com.leadaxe.sing-box-lxd` (без sudo, таймаут 2 с, без кэша) — службы нет (exit 113) или `state` ≠ `running` | жёлтая плашка «The service is installed but not running» с командой `sudo launchctl bootstrap system /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist` — **не** install; WARN перед apply |
| **ProcessStale** | файл совпал, но работающий локальный демон отвечает `executable_sha256` ≠ sha256(копии) или `executable` ≠ каноническому пути; у ядра без этих полей (lx.8/lx.10) — `version` ≠ версии ядра лаунчера | жёлтая плашка |
| **OK** | иначе | ничего |
| **CoreTooOld** (`core_too_old`) | вердикт выше — Unsafe, Stale или ProcessStale (лечится install), но ядро лаунчера ниже `minCoreForRootOwnedService` = **1.14.1-lx.12** (его пре-релизы, `lx.12-rc1`, проходят) или его версия не разбирается (§4.1) | плашка без команды: «The launcher core (…) is older than 1.14.1-lx.12 and cannot install a root-owned service. Update the core first: Local tab → Download/Reinstall v<пин>, then install or update the service.»; поверх Unsafe — красная, иначе жёлтая |

### 4.1 Гейт команд по ядру лаунчера (приёмка 24.09.2026)

Команду install (и copy SPEC 137) исполняет под sudo **ядро лаунчера**. До
lx.11 оно себя не копирует и пишет в plist свой путь — файл пользователя в
DataDir или бандле: это возврат к дыре §1. lx.11 (dev-сборки, пользователям
не выдавался) копирует себя в раннюю раскладку `com.leadaxe.sing-box-lxd` —
у нас это Unsafe (legacy). Каноническую копию кладёт lx.12 (и его rc), пин —
lx.12. Дефект приёмки: служба на копии
1.14.1-lx.12-rc1, ядро лаунчера — 1.14.1-lx.8, плашка звала обновить службу
«на текущее ядро» командой от lx.8.

- Признак «ядро умеет root-owned копию» — версия ядра лаунчера
  (`sing-box version`, кэш по `(dev, inode, size, mtime)` файла: диалог после
  скачивания зовётся до сброса сессионного кэша версии, dev-сборки кладут
  руками) ≥ `minCoreForRootOwnedService` = `1.14.1-lx.12` по базе и номеру
  lx; пре-релиз порогового релиза проходит (`lx.12-rc1` — копия у него уже
  каноническая), `lx.11` и `lx.11-rc*` — нет. Разбор — база `X.Y.Z` и
  `-lx.N` числом, пре-релиз ниже релиза той же базы: `lx.12-rc1` выше
  `lx.11`, но ниже `lx.12`. `CompareVersions` для этого не годится: он
  сравнивает только базу (`lx.10` == `lx.11`). Неразборчивая версия (пусто, `unknown` у
  dev-сборки, апстрим без `-lx.N`) — **не умеет**.
- Гейт один (`serviceCoreGate`), через него идут все каналы команды:
  плашка и шаг 1 вкладки Install (вместо строки — подсказка обновить ядро
  кнопкой «Download v…» / «Reinstall v…» на вкладке Local),
  диалог после обновления ядра (не показывается), модальное предупреждение
  §6 (текст с подсказкой, без команды), classic-гейт SPEC 137 (диалог
  «missing / outdated / not protected» с подсказкой вместо команды copy или
  install, без Retry), Debug API `/daemon/commands` (`install` пустой).
- Отказ — отдельное состояние `core_too_old`, а не поле detail: вердикт,
  который вылечила бы команда, сохраняется в `BlockedState` и в
  `service_detail` (`… ; the service is stale: …`). Так плашка знает цвет
  (Unsafe остаётся красной), а Debug API однозначно говорит «команды нет».

**Инвариант (Unsafe).** Каждое звено цепочки по `Lstat`: не симлинк; владелец
uid 0; нет записи для группы и остальных (`mode & 0o022 == 0`); каталоги —
каталоги, файл — обычный файл. Sticky-бит `/Library/PrivilegedHelperTools`
(`1755`) инварианту не мешает. Каталог на месте файла копии — Unsafe с
причиной «is a directory, not the copy file: remove it». Отсутствие файла
при целых каталогах — не
Unsafe (создать файл в root-каталоге пользователь не может), а Stale
(«службе нечего запускать», лечится той же командой).

**Хэши.** sha256 файлов кешируется по `(dev, inode, size, mtime)`: снимок
статуса зовётся на каждом открытии окна и перед каждым apply, а ядро весит
десятки мегабайт. Замена файла (скачивание, `install`) меняет ключ.

**Версия** не судит Stale (из сайдкара для копии — только показ). Версия
ядра лаунчера (`sing-box version`) судит в двух местах: запасной путь
ProcessStale для ядра без `executable_sha256` (значения `""` и `unknown`
вердикта не дают) и гейт команды install — CoreTooOld (§4.1).

**Не судим.** sha ядра лаунчера не посчитался (файла нет) — Stale не
выносится, вердикт остаётся по пути и инварианту. Демон недостижим или адрес
не loopback — ProcessStale не выносится. Пустой `executable_sha256` —
«неизвестно», а не расхождение: lx.11 считает хэш в фоне после старта и
первые мгновения отдаёт `""`; судит запасной путь по версии. `launchctl` не
ответил (нет утилиты, таймаут, непонятный вывод) — NotRunning не выносится.
Демон, запущенный руками при выгруженной службе, NotRunning не отменяет:
судим службу, а не процесс.

## 5. Команды

Все — через `shellQuote` (одинарные кавычки, `'` → `'\''`), в Terminal —
через экранирование строкового литерала AppleScript (`\` и `"`), как и
прежде в `OpenTerminalWithCommand`.

| Операция | Команда | Бинарь |
|---|---|---|
| **Install or update service** | `sudo '<bin>' lxd --service=install` | всегда `SingboxPath`: ядро копирует **себя**; только ядро ≥ lx.12 (и его rc), иначе команды нет (§4.1) |
| Uninstall (вкладка Uninstall, Debug API) | `sudo '<bin>' lxd --service=uninstall --keep-copy [--purge]` | копия, если plist указывает на неё и цепочка безопасна; иначе `SingboxPath`. `--keep-copy`: копию запускает classic-старт с TUN (SPEC 137) |
| Uninstall при удалении данных | `sudo '<bin>' lxd --service=uninstall --purge` | то же правило; без `--keep-copy` — копия уходит вместе с данными (§7) |
| Свежее приглашение | `sudo '<bin>' lxd client add --name singbox-launcher` | то же правило, что у Uninstall |
| Загрузить службу (NotRunning) | `sudo launchctl bootstrap system '/Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist'` | — (plist и копия в порядке, переустанавливать нечего) |
| Kickstart | `sudo launchctl kickstart -k system/com.leadaxe.sing-box-lxd` | только Debug API `/daemon/commands`; из UI убран |

Одна команда для всех случаев: первая установка, старый небезопасный plist,
обновление после скачивания ядра. Kickstart из UI убран: после обновления
ядра перезапуск службы поднимает **старую копию**, нужен `install`.

**Диалог после обновления ядра** (`core/core_downloader.go` →
`notifyDaemonServiceAfterCoreUpdate`): условие — plist существует (в любом
движке: служба запускается launchd и без лаунчера) и вердикт по файлам не OK
(скачано то же ядро, что уже в копии, — диалога нет); вместо kickstart
показывает ту же команду install. Скачанное ядро ниже lx.12 — диалога нет,
WARN в лог (§4.1).

## 6. UI (`ui/connection_local_daemon_darwin.go`)

- Вкладка **Status**, под строкой статуса — плашка по состоянию:
  - Unsafe — красная: «The service runs a binary your user can modify.
    Install or update the service to move it to a root-owned copy.» + путь,
    который запускает служба;
  - Stale / ProcessStale — жёлтая: «The service runs a different core (…)
    than the launcher (…)» с версиями/sha — нейтрально: копия бывает и
    новее ядра лаунчера; для отсутствующей копии — свой текст;
  - CoreTooOld — подсказка обновить ядро, **без команды** (§4.1);
  - NotRunning — жёлтая: «The service is installed but not running.» +
    состояние у launchd;
  - под текстом — строка команды «Install or update the service», а для
    NotRunning — «Load the service into launchd» (bootstrap); у CoreTooOld
    строки команды нет.
  OK и NotInstalled — плашки нет.
- Строка «Restart the service (after a core update)» (kickstart) убрана.
- Вкладка **Install**, шаг 1 — «Install or update the service»; ядро
  лаунчера ниже lx.12 — вместо строки подсказка обновить ядро.
- Строка статуса про ядро без lxd — без номера релиза (граница фичи
  проверяется запуском бинаря, не номером).
- **Модальное предупреждение** при Unsafe — одно на версию лаунчера:
  на старте, когда окно видно (как уведомления SPEC 135), в любом движке;
  флаг `daemon_unsafe_notice_version` в `settings.json` хранит версию, на
  которой показано. Ядро лаунчера ниже lx.12 — то же предупреждение без
  команды, с подсказкой обновить ядро (после обновления команду даст диалог
  «Core updated»).
- **WARN в лог** перед каждым apply daemon-движка, если служба не OK.

## 7. Удаление данных (SPEC 135 §4.3)

Подсказка удаления службы в диалоге «Remove all data…» и во флаге
`-purge-data` строится тем же правилом, что Uninstall: при безопасной копии —
команда через копию, и текст говорит, что служба переживает удаление данных
(копия вне DataDir); иначе — через `SingboxPath` с прежним «сначала удалите
службу, пока ядро на месте». Команда — полный uninstall **без** `--keep-copy`:
копия уходит вместе со службой и данными.

## 8. Debug API

`GET /daemon/status` получает `service_state`
(`not_installed|unsafe|stale|not_running|process_stale|ok|core_too_old`),
`service_path` (`ProgramArguments[0]`) и `service_detail` (английская
причина для диагностики; у `core_too_old` — версия ядра лаунчера, порог
lx.12 и исходный вердикт). Аддитивно, старые поля не меняются.
`GET /daemon/commands` отдаёт `install` пустым, пока ядро лаунчера ниже
lx.12 (§4.1), — в том числе при `not_installed`.

## 9. Ручная проверка (Mac владельца, ядро lx.11)

1. **До.** plist на бандл (`plutil -p /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist`
   → `ProgramArguments[0]` = `…/Contents/MacOS/bin/sing-box`). Первый старт
   новой версии лаунчера — модальное предупреждение с командой; второй старт
   той же версии — без него (в логе WARN «daemon service is unsafe» — на
   каждом старте). LOCAL → Status — красная плашка; в логе WARN перед apply;
   `GET /daemon/status` → `"service_state": "unsafe"`.
2. Выполнить команду из плашки (sudo вводит владелец). Установка может
   занять до ~20 с: ядро ждёт выгрузки старой службы до 10 с и повторяет
   `bootstrap`.
3. **После.**
   - `ls -l /Library/PrivilegedHelperTools/sing-box-lxd` —
     обычный файл `root wheel -rwxr-xr-x`;
     `cat /Library/PrivilegedHelperTools/sing-box-lxd.install.json`
     — поля §3 (`root wheel -rw-r--r--`);
   - `plutil -p` plist → `ProgramArguments[0]` = копия, прочие ключи прежние;
   - `shasum -a 256 /Library/PrivilegedHelperTools/sing-box-lxd` == `shasum -a 256 ~/Library/Application\ Support/singbox-launcher/bin/sing-box`;
   - `~/Library/Application\ Support/singbox-launcher/bin/sing-box lxd --service=status`
     (вызывающий бинарь — ядро лаунчера, сверяется с копией) — exit 0
     (2 — MISMATCH/UNSAFE, 3 — NOT INSTALLED, 4 — COPY ONLY, 5 — NOT RUNNING,
     1 — ошибка);
   - Debug API `GET /daemon/status` → `"service_state": "ok"`;
   - `launchctl print system/com.leadaxe.sing-box-lxd` — `state = running`;
   - плашки нет, сопряжение живо (Start/Stop без пароля, список узлов).
4. **Отрицательный.** Подменить ядро в DataDir (другая сборка) → Refresh →
   жёлтая Stale-плашка «runs a different core (…) than the launcher (…)» с
   разными sha; команда — возврат в OK.
   **Ядро лаунчера ниже lx.12** (§4.1). Служба на копии lx.12, в DataDir —
   ядро lx.8 (или lx.11, или dev-сборка с `version unknown`) → Refresh →
   плашка «The launcher core (1.14.1-lx.8) is older than 1.14.1-lx.12 …
   Update the core first: Local tab → Download/Reinstall v1.14.1-lx.12 …»
   **без строки команды**; на
   вкладке Install вместо шага 1 та же подсказка; `GET /daemon/status` →
   `"service_state": "core_too_old"`, `GET /daemon/commands` → `"install": ""`;
   Start в classic с TUN при отстающей копии — диалог без команды и без
   Retry. Вкладка Local → «Reinstall v1.14.1-lx.12» → диалог «Core updated» с командой
   install → команда → OK.
   **Не запущена.** `sudo launchctl bootout system/com.leadaxe.sing-box-lxd`
   → Refresh → жёлтая плашка «The service is installed but not running»
   (launchd: not loaded) со строкой bootstrap, а не install;
   `--service=status` → exit 5; `GET /daemon/status` →
   `"service_state": "not_running"`; команда из плашки → Refresh → OK.
5. **Обновление ядра кнопкой** → диалог «Core updated» с той же командой
   install (в том числе в classic-движке при установленной службе).
6. **Uninstall-вкладка** и «Need a fresh invite» — команды через копию;
   Uninstall — с `--keep-copy`: после неё plist нет, копия и
   `sing-box-lxd.install.json` на месте, `<ядро-лаунчера> lxd --service=status` → exit 4 (COPY ONLY).
7. **Remove all data…** — подсказка через копию, без `--keep-copy`, и текст
   «служба переживает удаление»; после команды копии нет.
8. **Перезагрузка** → демон стартует из копии (`ps -o comm= -p <pid>` →
   `/Library/PrivilegedHelperTools/sing-box-lxd`), лаунчер из
   Login Items сопрягается без пароля.
9. **Ранняя раскладка (legacy).** От dev-сборок lx.11 остался каталог
   `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/` или плоский
   файл `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd` (+
   `.install.json`), plist на него → красная плашка, в `GET /daemon/status`
   → `service_detail` «… legacy layout of early lx.11 builds: run Install
   or update service, then remove it (`sudo rm -rf /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd.install.json`)». Команда install
   (ядро lx.12) → OK; затем `sudo rm -rf /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd.install.json` — оба варианта legacy
   удаляются той же командой.

## 10. Вне рамок

- Бамп `constants.RequiredCoreVersion` — отдельный коммит при релизе
  (сделан: lx.12). Со старым ядром лаунчера (lx.8/lx.10) команды нет вовсе
  (§4.1): прежнее «команда plist на копию не переводит, Unsafe остаётся»
  было неверно — такая команда переводила plist с копии обратно на DataDir.
- Копирование или проверка подписи копии силами лаунчера.
- Вызов `lxd --service=status` из лаунчера.
- Служба на Linux (`service_linux.go` форка) — там своя модель и нет daemon-
  движка лаунчера.

## 11. Что зависит от lx.11

Проверить после сборки ядра:

1. Каноническая раскладка §3 (плоский файл `sing-box-lxd`, сайдкар
   `sing-box-lxd.install.json`, ядро lx.12) — константы
   `core/daemon_service_state_darwin.go`, имя — `platform.PrivilegedCopyName`.
2. `install` переписывает только `ProgramArguments[0]` и перезапускает службу
   — иначе после команды остаётся ProcessStale.
3. `install` поверх существующей службы сохраняет сопряжение — иначе после
   ремонта нужен Pair.
4. `/admin/info` отдаёт `executable`/`executable_sha256` — иначе
   ProcessStale идёт по версии.
5. `uninstall` и `client add`, вызванные **из копии**, находят state-dir
   службы так же, как из ядра лаунчера.
6. `uninstall` убирает plist; что делать с каталогом копии — решает ядро.
   Лаунчер после удаления plist видит NotInstalled независимо от остатка
   копии, а следующий `install` перезапишет её.
7. Classic-старт с TUN (SPEC 137) исполняет ту же копию под root: вкладка
   Uninstall зовёт `--service=uninstall --keep-copy` (lx.11) и копию
   оставляет; подсказка «Remove all data…» — полный uninstall, после него
   classic попросит `--service=copy`.
