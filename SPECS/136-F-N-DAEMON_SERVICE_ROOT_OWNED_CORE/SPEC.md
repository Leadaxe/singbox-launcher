# 136 · Служба демона на root-owned копии ядра

Парная задача к SPEC 100 форка ядра (`sing-box-lx`, релиз **v1.14.1-lx.11**).
Решение владельца — 24.09.2026. Заменяет сверку путей из SPEC 135 §5.1.

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
| Каталог службы | `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/` — `root:wheel 0755` |
| Копия ядра | `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/sing-box` — `root:wheel 0755`; имя `sing-box` сохраняется (граница форка: `pgrep`/`ps` и диагностика по имени процесса не ломаются) |
| Сайдкар | `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/install.json` — `root:wheel 0644`: `{source, sha256, version, installed_at, plist_path, label}` |
| plist | `ProgramArguments[0]` = копия; остальные ключи plist не меняются |
| `lxd --service=install` | идемпотентен по sha (равные sha — копия не трогается); `daemon.json`, секрет и клиенты сохраняются; служба перезапускается (`bootout` + `bootstrap`) |
| `lxd --service=status` | exit 0 — OK, 2 — MISMATCH или UNSAFE, 1 — ошибка; печатает путь, sha256 копии, sha256 вызывающего бинаря, вердикт |
| `GET /admin/info` | новые поля `executable`, `executable_sha256`; у ядер до lx.11 их нет — лаунчер проверяет наличие |

Всё читается без sudo: каталог и файл `0755`, сайдкар и plist `0644`.
`--service=status` лаунчер не вызывает (своя классификация без запуска
процесса, работает и со старым ядром); команда — для ручной проверки (§9).

## 4. Классификатор состояния службы

Вердикт — первый сработавший сверху вниз:

| Состояние | Условие | Показ |
|---|---|---|
| **NotInstalled** | plist нет | ничего; вкладка Install |
| **Unsafe** | plist не разобрался; **или** `ProgramArguments[0]` ≠ каноническая копия (бандл, DataDir, что угодно); **или** цепочка `/Library` → `/Library/PrivilegedHelperTools` → каталог службы → файл не проходит инвариант | красная плашка, модальное предупреждение раз на версию лаунчера, WARN перед apply |
| **Stale** | путь канонический и безопасный, но sha256(копии) ≠ sha256(`EvalSymlinks(SingboxPath)`); **или** копии нет | жёлтая плашка |
| **ProcessStale** | файл совпал, но работающий локальный демон отвечает `executable_sha256` ≠ sha256(копии) или `executable` ≠ каноническому пути; у ядра без этих полей (lx.8/lx.10) — `version` ≠ версии ядра лаунчера | жёлтая плашка |
| **OK** | иначе | ничего |

**Инвариант (Unsafe).** Каждое звено цепочки по `Lstat`: не симлинк; владелец
uid 0; нет записи для группы и остальных (`mode & 0o022 == 0`); каталоги —
каталоги, файл — обычный файл. Sticky-бит `/Library/PrivilegedHelperTools`
(`1755`) инварианту не мешает. Отсутствие файла при целых каталогах — не
Unsafe (создать файл в root-каталоге пользователь не может), а Stale
(«службе нечего запускать», лечится той же командой).

**Хэши.** sha256 файлов кешируется по `(dev, inode, size, mtime)`: снимок
статуса зовётся на каждом открытии окна и перед каждым apply, а ядро весит
десятки мегабайт. Замена файла (скачивание, `install`) меняет ключ.

**Версия** только показывается (из сайдкара для копии, `sing-box version` для
ядра лаунчера) и не судит Stale. Единственное её применение в вердикте —
запасной путь ProcessStale для ядра без `executable_sha256`; значения `""` и
`unknown` (dev-сборка) вердикта не дают.

**Не судим.** sha ядра лаунчера не посчитался (файла нет) — Stale не
выносится, вердикт остаётся по пути и инварианту. Демон недостижим или адрес
не loopback — ProcessStale не выносится.

## 5. Команды

Все — через `shellQuote` (одинарные кавычки, `'` → `'\''`), в Terminal —
через экранирование строкового литерала AppleScript (`\` и `"`), как и
прежде в `OpenTerminalWithCommand`.

| Операция | Команда | Бинарь |
|---|---|---|
| **Install or update service** | `sudo '<bin>' lxd --service=install` | всегда `SingboxPath`: ядро копирует **себя** |
| Uninstall (`--purge`) | `sudo '<bin>' lxd --service=uninstall [--purge]` | копия, если plist указывает на неё и цепочка безопасна; иначе `SingboxPath` |
| Свежее приглашение | `sudo '<bin>' lxd client add --name singbox-launcher` | то же правило, что у Uninstall |
| Kickstart | `sudo launchctl kickstart -k system/com.leadaxe.sing-box-lxd` | только Debug API `/daemon/commands`; из UI убран |

Одна команда для всех случаев: первая установка, старый небезопасный plist,
обновление после скачивания ядра. Kickstart из UI убран: после обновления
ядра перезапуск службы поднимает **старую копию**, нужен `install`.

**Диалог после обновления ядра** (`core/core_downloader.go` →
`notifyDaemonServiceAfterCoreUpdate`): условие — plist существует (в любом
движке: служба запускается launchd и без лаунчера) и вердикт по файлам не OK
(скачано то же ядро, что уже в копии, — диалога нет); вместо kickstart
показывает ту же команду install.

## 6. UI (`ui/connection_local_daemon_darwin.go`)

- Вкладка **Status**, под строкой статуса — плашка по состоянию:
  - Unsafe — красная: «The service runs a binary your user can modify.
    Install or update the service to move it to a root-owned copy.» + путь,
    который запускает служба;
  - Stale / ProcessStale — жёлтая: «The service runs an older core (…)» с
    версиями/sha; для отсутствующей копии — свой текст;
  - под текстом — строка команды «Install or update the service».
  OK и NotInstalled — плашки нет.
- Строка «Restart the service (after a core update)» (kickstart) убрана.
- Вкладка **Install**, шаг 1 — «Install or update the service».
- Строка статуса про ядро без lxd — без номера релиза (граница фичи
  проверяется запуском бинаря, не номером).
- **Модальное предупреждение** при Unsafe — одно на версию лаунчера:
  на старте, когда окно видно (как уведомления SPEC 135), в любом движке;
  флаг `daemon_unsafe_notice_version` в `settings.json` хранит версию, на
  которой показано.
- **WARN в лог** перед каждым apply daemon-движка, если служба не OK.

## 7. Удаление данных (SPEC 135 §4.3)

Подсказка удаления службы в диалоге «Remove all data…» и во флаге
`-purge-data` строится тем же правилом, что Uninstall: при безопасной копии —
команда через копию, и текст говорит, что служба переживает удаление данных
(копия вне DataDir); иначе — через `SingboxPath` с прежним «сначала удалите
службу, пока ядро на месте».

## 8. Debug API

`GET /daemon/status` получает `service_state`
(`not_installed|unsafe|stale|process_stale|ok`), `service_path`
(`ProgramArguments[0]`) и `service_detail` (английская причина для
диагностики). Аддитивно, старые поля не меняются.

## 9. Ручная проверка (Mac владельца, ядро lx.11)

1. **До.** plist на бандл (`plutil -p /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist`
   → `ProgramArguments[0]` = `…/Contents/MacOS/bin/sing-box`). Первый старт
   новой версии лаунчера — модальное предупреждение с командой; второй старт
   той же версии — без него (в логе WARN «daemon service is unsafe» — на
   каждом старте). LOCAL → Status — красная плашка; в логе WARN перед apply;
   `GET /daemon/status` → `"service_state": "unsafe"`.
2. Выполнить команду из плашки (sudo вводит владелец).
3. **После.**
   - `ls -ld /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd` и
     `ls -l …/sing-box` — `root wheel`, `drwxr-xr-x` / `-rwxr-xr-x`;
     `cat …/install.json` — поля §3;
   - `plutil -p` plist → `ProgramArguments[0]` = копия, прочие ключи прежние;
   - `shasum -a 256 …/sing-box` == `shasum -a 256 ~/Library/Application\ Support/singbox-launcher/bin/sing-box`;
   - `~/Library/Application\ Support/singbox-launcher/bin/sing-box lxd --service=status`
     (вызывающий бинарь — ядро лаунчера, сверяется с копией) — exit 0;
   - Debug API `GET /daemon/status` → `"service_state": "ok"`;
   - `launchctl print system/com.leadaxe.sing-box-lxd` — `state = running`;
   - плашки нет, сопряжение живо (Start/Stop без пароля, список узлов).
4. **Отрицательный.** Подменить ядро в DataDir (другая сборка) → Refresh →
   жёлтая Stale-плашка с разными sha; команда — возврат в OK.
5. **Обновление ядра кнопкой** → диалог «Core updated» с той же командой
   install (в том числе в classic-движке при установленной службе).
6. **Uninstall-вкладка** и «Need a fresh invite» — команды через копию.
7. **Remove all data…** — подсказка через копию и текст «служба переживает
   удаление».
8. **Перезагрузка** → демон стартует из копии (`ps -o comm= -p <pid>` →
   `…/com.leadaxe.sing-box-lxd/sing-box`), лаунчер из Login Items
   сопрягается без пароля.

## 10. Вне рамок

- Бамп `constants.RequiredCoreVersion` до lx.11 — отдельный коммит при
  релизе. До него команда из плашки со старым ядром (lx.8/lx.10) plist на
  копию не переводит: Unsafe остаётся, это честно.
- Копирование или проверка подписи копии силами лаунчера.
- Вызов `lxd --service=status` из лаунчера.
- Служба на Linux (`service_linux.go` форка) — там своя модель и нет daemon-
  движка лаунчера.

## 11. Что зависит от lx.11

Проверить после сборки ядра:

1. Каноническая раскладка §3 (каталог, имя `sing-box`, сайдкар
   `install.json`) — константы `core/daemon_service_state_darwin.go`.
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
