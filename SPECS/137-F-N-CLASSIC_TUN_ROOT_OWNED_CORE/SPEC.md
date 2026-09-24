# 137 · Classic TUN на macOS: root запускает только root-owned копию ядра

Решение владельца и главной сессии — 24.09.2026. Приоритет — сразу после
SPEC 136, до следующего релиза лаунчера. Парная задача к SPEC 100 форка
(`sing-box-lx`, релиз **v1.14.1-lx.11**): та же каноническая копия ядра, что
у службы демона SPEC 136, плюс новая команда ядра `lxd --service=copy`.

Статус: **реализовано в ветке `spec-137-classic-root-owned` (от
`spec-136-daemon-root-owned`); ядро с `--service=copy` и копией
`/Library/PrivilegedHelperTools/sing-box-lxd` выходит как **lx.12** (раскладка
dev-сборок lx.11 — legacy, §9 п. 9); ждёт релиза ядра, ручной проверки
владельцем и CI**.

## 1. Проблема: что исполнялось под root

Classic-движок на macOS поднимает ядро с TUN через
`AuthorizationExecuteWithPrivileges` (AEWP). До этой задачи root исполнял
пользовательские файлы и пользовательское окружение (строки — на базе ветки,
`36210ac6`).

| # | Где | Что исполнял root |
|---|---|---|
| 1 | `core/process_service.go:270-281`, `internal/platform/privileged_darwin.go:114-125` | `/bin/sh <Data>/bin/start-singbox-privileged.sh`. Скрипт лаунчер пишет от имени пользователя (`0700`) перед каждым стартом; внутри — `cd "<Data>/bin"` и `"<SingboxPath>" run -c "config.json" >> "<Logs>/sing-box.log"`. `SingboxPath` — ядро в `<Data>/bin`, в бандле, из `SINGBOX_LAUNCHER_CORE` или из `PATH`: каждое пишется пользователем. Пути вставлялись через `strconv.Quote` (двойные кавычки Go: `$` и `` ` `` в пути раскрывает шелл). |
| 2 | `internal/platform/privileged_darwin.go:129-137` (`KillPrivilegedProcess`) | `/bin/sh -c "kill …; kill …; rm -f \"<pid-файл>\""` — `rm` ищется по `PATH` лаунчера. |
| 3 | `core/process_service.go:124-127, 660`, `ui/diagnostics_tab.go:53-54` | `/bin/sh -c "pkill -TERM -f …"` — `pkill` ищется по `PATH` лаунчера. |
| 4 | `ui/configurator/tabs/settings_tun_darwin.go:129-134` | `/bin/sh -c "rm -rf \"<пути>\""` при снятии галки TUN. Путь кэша — из конфига (`experimental.cache_file.path`), имя вида `$(…).db` под `strconv.Quote` становится подстановкой команды под root. |

**Окружение.** AEWP передаёт инструменту окружение лаунчера, а его задаёт
пользователь (запуск из терминала, LaunchAgent, Login Item). `/bin/sh` на
macOS — bash 3.2, и он импортирует функции из переменных `BASH_FUNC_<имя>%%`:
`env 'BASH_FUNC_echo%%=() { …; }' /bin/sh -c 'echo hi'` выполняет функцию
вместо `echo` (проверено на Mac владельца, Darwin 25). Поэтому даже
«постоянное тело» `sh -c` под root исполнимо чужим кодом, пока окружение не
очищено; `PATH` лаунчера решает, какой `rm`/`pkill` запустит root.

**Авторизация.** `AuthorizationRef` создаётся один раз
(`privileged_darwin.go:19-37`) и живёт до выхода лаунчера
(`core/controller.go:489-491`). После первого пароля каждый следующий AEWP —
без вопроса: рестарт при применении конфига (`KillForRestart` → `Start`),
авто-рестарт после падения (`onPrivilegedScriptExited` → `Start`).

**Сценарий.** Программа от имени пользователя подменяет
`<Data>/bin/sing-box` (или скрипт в окне между записью и запуском, или
окружение лаунчера) и получает root на ближайшем старте VPN в TUN; если
авторизация в сессии уже выдана — без пароля, на первом же рестарте.

Linux и Windows этой задачей не затрагиваются (§8).

## 2. Нормы

1. **root исполняет только root-owned файлы**: копию ядра
   `/Library/PrivilegedHelperTools/sing-box-lxd` (SPEC 136
   §3, интерфейс lx.11) и системные утилиты по абсолютным путям —
   `/usr/bin/env`, `/bin/sh`, `/bin/kill`, `/usr/bin/pkill`, а в теле старта
   `/bin/mkdir`, `/bin/chmod`, `/bin/mv`, `/usr/bin/stat`. Никаких файлов из
   DataDir, бандла или `PATH`.
2. **Окружение root-шелла очищено**: `/usr/bin/env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin`.
3. **Тело шелла — константа** в бинаре лаунчера; пути — позиционными
   аргументами (argv, без шелл-квотинга).
4. **Гейт перед стартом** (§4): нет копии, цепочка владения нарушена или sha
   не совпал — старт с привилегиями не выполняется, AEWP не вызывается.
5. Не-macOS платформы и daemon-режим (SPEC 136) не меняются.
6. **root не пишет по путям пользователя** (137.1): вывод ядра — в
   root-owned `/Library/Logs/sing-box-lxd/classic.log`; остатки прежних
   стартов в каталогах пользователя лаунчер удаляет своим uid (§3.2).

## 3. Привилегированные вызовы после задачи

| Действие | Вызов AEWP |
|---|---|
| Старт TUN | `/usr/bin/env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin /bin/sh -c '<тело>' start-singbox-privileged <Data>/bin <копия> config.json /Library/Logs/sing-box-lxd 0 <uid лаунчера> 2097152` |
| Stop / рестарт | `/bin/kill -TERM <PID шелла> [<PID ядра>]` |
| «Sing-Box already running» → Kill, Diagnostics → Kill | `/usr/bin/pkill -TERM -f 'sing-box run\|sing-box-lxd run\|start-singbox-privileged'` |
| Снятие галки TUN | — (без root, §3.2) |

### 3.1 Тело старта (`platform.privilegedStartBody`)

Аргументы: `$1` каталог bin, `$2` копия ядра, `$3` имя конфига, `$4` каталог
лога, `$5` владелец каталога лога (uid, в проде `0`), `$6` uid пользователя
лаунчера (`os.Getuid()`) — владелец файла лога, `$7` порог ротации (байт, как у
лога в каталоге пользователя — 2 МиБ).

```sh
umask 022
d="$4"
f="$d/classic.log"
u="$6"
case "$u" in ''|*[!0-9]*) echo "refused: invalid uid '$u'"; exit 1;; esac
if [ "$u" -lt 501 ]; then echo "refused: uid $u is not a regular user"; exit 1; fi
g="$(/usr/bin/id -g "$u" 2>/dev/null)" || { echo "refused: no user with uid $u"; exit 1; }
case "$g" in ''|*[!0-9]*) echo "refused: no group for uid $u"; exit 1;; esac
if [ -L "$d" ]; then echo "refused: $d is a symbolic link"; exit 1; fi
/bin/mkdir -p "$d" || { echo "refused: cannot create $d"; exit 1; }
if [ "$(/usr/bin/stat -f '%u:%HT' "$d")" != "$5:Directory" ]; then echo "refused: …"; exit 1; fi
/bin/chmod 0755 "$d" || { …; exit 1; }
if [ -e "$f" ] || [ -L "$f" ]; then
  o="$(/usr/bin/stat -f '%u:%HT' "$f")"
  if [ "$o" != "$u:Regular File" ] && [ "$o" != "0:Regular File" ]; then echo "refused: …"; exit 1; fi
  /bin/chmod 0600 "$f" && /usr/sbin/chown "$u:$g" "$f" || { …; exit 1; }
  if [ "$(/usr/bin/stat -f %z "$f")" -gt "$7" ]; then /bin/mv -f "$f" "$f.old" || { …; exit 1; }; fi
fi
: >>"$f" || { …; exit 1; }
/bin/chmod 0600 "$f" && /usr/sbin/chown "$u:$g" "$f" || { …; exit 1; }
cd "$1" || { echo "refused: cannot enter $1"; exit 1; }
echo $$
"$2" run -c "$3" >>"$f" 2>&1 &
echo $!
exec >>"$f" 2>&1
wait
```

- **Лог (137.1).** Каталог `/Library/Logs/sing-box-lxd` (`root:wheel 0755`,
  родитель `/Library/Logs` — `root:wheel 0755`); файл `classic.log` —
  **пользователя лаунчера, `0600`** (решение координатора 24.09.2026),
  прошлый — `classic.log.old` с тем же владельцем и правами. Другие
  локальные учётные записи лог не читают, а подменить файл (переименовать,
  заменить симлинком) пользователь не может — каталог root-owned. Не
  `/Library/Application Support/sing-box-lxd`: тот `0700`, и лаунчер без root
  его не прочитает.
- **uid владельца** приходит аргументом из лаунчера (`os.Getuid()`) и
  проверяется телом: только цифры, не меньше 501, учётная запись существует
  (`/usr/bin/id -g` по абсолютному пути даёт и группу для `chown`). Проверка
  идёт до любых действий с каталогом.
- Ни каталог, ни файл тело не трогает, если на их месте симлинк, другой тип
  или другой владелец (`/usr/bin/stat` без `-L` смотрит на саму запись):
  файл допускается только этого пользователя или root (лог сборок 137.1 до
  этого решения был `root 0644` — его тело забирает пользователю); ротация —
  `rename` внутри root-каталога, `.old` уже принадлежит пользователю.
- **Отказ.** До первого PID тело печатает `refused: <причина>` и выходит;
  `RunWithPrivileges` возвращает эту строку ошибкой (первая строка stdout
  вместо PID), ядро не стартует, пользователь видит причину в «Failed to
  start sing-box».
- Первые две строки stdout при успехе — PID шелла и PID ядра, их читает
  `RunWithPrivileges` (как у прежнего скрипта); затем stdout шелла уходит в
  лог, шелл ждёт ядро, и его выход — выход ядра (`WaitForPrivilegedExit` →
  `onPrivilegedScriptExited`). `env` заменяет себя шеллом через `exec`, PID
  не меняется.
- `$0` — `start-singbox-privileged`, а не `--` из постановки: в `sh -c` первый
  аргумент после тела становится `$0`, и имя держит совпадение с
  `platform.PrivilegedPkillPattern` — `pgrep`/`pkill` находят обёртку, как
  находили прежний скрипт.
- `ps` показывает `/Library/PrivilegedHelperTools/sing-box-lxd run -c config.json` от root.
  Копия — плоский файл `sing-box-lxd` (SPEC 136 §3, решение владельца
  24.09.2026, ядро lx.12), и процесс ядра зовётся так же, а не `sing-box`:
  `platform.PrivilegedPkillPattern` = `sing-box run|sing-box-lxd run|start-singbox-privileged`
  («sing-box run» копию не ловит — после `sing-box` идёт `-lxd`) — `pgrep`
  в диалоге «already running», `pkill` в Kill; проверка живого PID из
  pid-файла при удалении данных узнаёт имя `sing-box-lxd`
  (`platform.IsPrivilegedCoreProcessName`, 12 символов — `p_comm` не
  усекает). Демон службы (`… lxd --state-dir`) под шаблон не попадает.
- pid-файл `<Data>/bin/singbox.pid` пишет и удаляет лаунчер от имени
  пользователя; `rm` под root больше нет. PID ≤ 0 в `kill` не передаются
  (`kill 0` — группа процессов).
- Прежний `<Data>/bin/start-singbox-privileged.sh` при старте удаляется
  (best-effort) — его больше ничто не исполняет.
- **Кто читает лог.** `AppController.CoreLogPath()`: после старта с TUN —
  root-owned лог, после обычного старта — `<Logs>/sing-box.log`, до первого
  старта в сессии — по конфигу (TUN → root-owned). По нему идут Core-вкладка
  окна логов (`ui/log_viewer_window.go`) и тейлер профайлера трафика
  (`ui/traffic_bootstrap.go` → `TrafficProfiler.StartFollowing`, путь
  пересчитывается на каждом тике). `<Logs>/sing-box.log` classic без TUN
  по-прежнему пишет сам лаунчер от имени пользователя.

### 3.2 Снятие TUN без root (137.1)

Галка TUN снята в визарде при остановленном ядре → кэш
(`experimental.cache_file.path` под `bin/`) и `<Logs>/sing-box.log[.old]`,
которые ядро под root могло оставить root-owned, лаунчер удаляет **своим
uid**: право удаления даёт каталог (`bin/`, `logs/` — пользователя), а не
владелец файла. Перед удалением (`removeTunLeftover`): путь внутри
`bin/`/`logs/` лексически и после `EvalSymlinks` родителя, сам путь не
симлинк. Интерфейс и маршруты ядро снимает само при выходе — AEWP здесь не
вызывается вовсе.

## 4. Гейт перед стартом с привилегиями

`checkPrivilegedCoreCopy` (`core/classic_privileged_darwin.go`), по порядку:

| Вердикт | Условие | Что дальше |
|---|---|---|
| **no core** | ядро лаунчера (`EvalSymlinks(SingboxPath)`) не найдено или не читается | обычная ошибка старта: сравнивать не с чем, и команде копирования нечего копировать |
| **missing** | звено цепочки или сама копия отсутствует при root-owned родителях | диалог «missing» |
| **unsafe** | цепочка `/Library` → `/Library/PrivilegedHelperTools` → файл копии не проходит инвариант SPEC 136 §4 (`Lstat`: не симлинк, uid 0, `mode & 022 == 0`, каталоги — каталоги, файл — обычный файл; каталог на месте файла — «is a directory, not the copy file: remove it»), либо копия не прочиталась | диалог «not protected» |
| **outdated** | sha256(копии) ≠ sha256(ядра лаунчера) | диалог «outdated», WARN с обоими sha |
| **ok** | иначе | старт копии |

Переиспользуется из SPEC 136 без дублирования: `checkRootOwnedChain`,
`errDaemonCopyMissing`, раскладка `daemonServiceLayout` /
`systemDaemonServiceLayout`, кэш sha256 `daemonServiceHashes` (ключ
`dev, inode, size, mtime` — ядро читается один раз, пока файл не заменён),
`shortSHA`, `daemonServiceCommand` (квотинг команды).

В отличие от классификатора службы, гейт **закрыт по умолчанию**: не
посчитался хэш — старта нет. Гейт зовётся под `CmdMutex` в `Start`: и при
нажатии Start, и при рестарте после применения конфига, и при авто-рестарте
после падения.

## 5. Диалог и команда

Отказ гейта (missing / unsafe / outdated) — WARN в лог с деталью и обоими
sha, старт прекращается без диалога «Failed to start sing-box», вместо него —
диалог:

| Вердикт | Заголовок |
|---|---|
| missing | Core copy for privileged start is missing |
| outdated | Core copy for privileged start is outdated (в тексте — sha копии и ядра лаунчера) |
| unsafe | Core copy for privileged start is not protected (в тексте — нарушившее звено) |

Одна sudo-команда (через `daemonServiceCommand`: одинарные кавычки, `'` → `'\''`):

| Условие | Команда |
|---|---|
| plist службы демона есть | `sudo '<SingboxPath>' lxd --service=install` — та же, что в SPEC 136 (install тоже создаёт копию; служба перезапускается); текст диалога это объясняет |
| plist нет | `sudo '<SingboxPath>' lxd --service=copy` — новое в lx.11: только root-owned копия и сайдкар, без plist и launchd |

Кнопки: **Copy the command**, **Run in Terminal** (`OpenTerminalWithCommand`
SPEC 136), **Retry** (закрывает диалог и повторяет Start через
`StartSingBoxProcess`), **Close**. Сам диалог ничего привилегированного не
запускает (`internal/dialogs.ShowCommandRetry`).

## 6. Время жизни авторизации — развилка

| | Вариант | Цена |
|---|---|---|
| **А (текущий выбор, рекомендация — ждёт слова владельца)** | авторизация на сессию лаунчера, как было | root исполняет только root-owned копию после сверки sha и системные утилиты с фиксированными аргументами — подменять нечего; пароль — один раз за сессию |
| Б | авторизация на одно действие | пароль на каждый старт, каждую остановку, каждый рестарт при применении конфига и каждый Kill; авто-рестарт после падения тоже спросит пароль (снятие TUN root не требует, §3.2) |

Переключение — одна константа `privilegedAuthReuse` в
`internal/platform/privileged_darwin.go` (`true` — А, `false` — Б: ссылка
освобождается после каждого вызова AEWP). Остальной код от варианта не
зависит.

## 7. Обновление ядра

После скачивания ядра (`DownloadCore`, шаг 6.7):

- plist службы есть — диалог install SPEC 136 (без изменений; он обновляет
  ту же копию);
- plist нет, копия есть и отстаёт — WARN в лог с обоими sha
  (`notifyPrivilegedCopyAfterCoreUpdate`); ближайший старт с TUN не пройдёт
  гейт и покажет диалог «outdated» с командой copy — не молча и не на старой
  копии.

## 8. Остаточные риски и вне рамок

Решения владельца — отдельными задачами:

1. ~~Лог ядра под root по пути пользователя~~ — **закрыто в 137.1** (§3.1):
   лог в root-owned `/Library/Logs/sing-box-lxd/`.
2. **Конфиг пользователя под root.** Ядро от root читает `config.json` и
   пишет по путям из него (`experimental.cache_file.path` — по умолчанию
   `bin/cache.db`, `log.output`) — как и служба демона; свойство модели
   «root запускает пользовательский конфиг». Кэш остаётся root-owned в
   `bin/` и удаляется при снятии TUN без root (§3.2).
3. ~~`rm -rf` под root при снятии TUN~~ — **закрыто в 137.1** (§3.2): root
   там не нужен.
4. **Команда copy/install исполняет под sudo ядро лаунчера** —
   пользовательский файл, как в SPEC 136 §5: пароль пользователь вводит в
   своём терминале, диалог показывает оба sha.
5. **Windows — следующей задачей.** Лаунчер с `requireAdministrator`
   (`app.manifest`) запускает ядро из `%LOCALAPPDATA%\singbox-launcher\bin`,
   куда пишет процесс обычной целостности: тот же класс (повышение до
   высокой целостности). PowerShell-диалоги файлов, найденные аудитом (§12),
   исправлены отдельно. **Linux** —
   `setcap` привязан к файлу, и запись в файл снимает capability: дыры нет.
   Не трогаются (норма 5).
6. **Бамп `constants.RequiredCoreVersion` до lx.11** — вместе с SPEC 136, при
   релизе. До него ядро без `--service=copy` команду из диалога не выполнит,
   и старт с TUN невозможен, пока копии нет: релиз лаунчера с этой задачей
   без ядра lx.11 не выпускается.
7. ~~Лог читают все локальные пользователи~~ — **решено**: `classic.log`
   принадлежит пользователю лаунчера, `0600` (§3.1); как и прежний
   `~/Library/Logs/…/sing-box.log`, другим учётным записям он закрыт.
8. **Что попадает в лог.** Уровень `log.level` по умолчанию — `warn`
   (`bin/wizard_template.json`, переменная `log_level`; то же значение
   отдаёт `GET /state/log-level`). `debug`/`trace` включаются только явно:
   переменная `log_level` в визарде, переключатель verbose профайлера
   трафика (с подтверждением `ConfirmAndApplyLogLevel`) или
   `PATCH /state/log-level` Debug API. Тело направляет в лог и stderr ядра
   (`2>&1`): паника Go, `FATAL` старта с фрагментами ошибок разбора
   конфига попадут в файл как есть. **Риск приемлем:** лог `0600` у того же
   пользователя, которому принадлежит и сам конфиг.
9. **Остатки под root вне данных.** `/Library/Logs/sing-box-lxd/` «Remove
   all data…» не удаляет (нужен root): `sudo rm -rf /Library/Logs/sing-box-lxd`.

## 9. Ручная проверка (Mac владельца, ядро lx.11, classic-движок, конфиг с TUN)

1. **До.** Службы демона нет (`ls /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist`
   → нет), копии нет (`ls -l /Library/PrivilegedHelperTools/sing-box-lxd`
   → нет). Start → пароль **не** спрашивается, диалог «Core copy for privileged
   start is missing» с командой `sudo '…/sing-box' lxd --service=copy`; в логе
   WARN «privileged start refused». `ls <Data>/bin/start-singbox-privileged.sh`
   → нет (удалён при старте).
2. Run in Terminal → выполнить команду (sudo вводит владелец). В выводе —
   `lxd: copied <src> -> /Library/PrivilegedHelperTools/sing-box-lxd (sha256 <hex>, root:wheel 0755)`;
   повтор той же команды — `lxd: already up to date <hex>`.
3. **После.**
   - `ls -l /Library/PrivilegedHelperTools/sing-box-lxd*` —
     обычный файл копии `root wheel -rwxr-xr-x` и сайдкар
     `sing-box-lxd.install.json` `root wheel -rw-r--r--`; каталога
     службы нет; plist службы не появился;
     `cat /Library/PrivilegedHelperTools/sing-box-lxd.install.json`
     — `"plist_path": ""`;
   - `~/Library/Application\ Support/singbox-launcher/bin/sing-box lxd --service=status`
     — exit 4 (COPY ONLY: копия без службы);
   - `shasum -a 256 /Library/PrivilegedHelperTools/sing-box-lxd` == `shasum -a 256 ~/Library/Application\ Support/singbox-launcher/bin/sing-box`;
   - Retry → пароль AEWP (раз за сессию), VPN работает;
   - `ps -axo user,pid,command | grep -E 'sing-box-lxd run|start-singbox-privileged'`
     — `root … /Library/PrivilegedHelperTools/sing-box-lxd run -c config.json`
     и `root … /bin/sh -c … start-singbox-privileged …`;
   - Stop / Start / применение конфига из визарда (рестарт ядра) — без
     нового пароля (вариант А); Stop гасит оба процесса,
     `<Data>/bin/singbox.pid` удалён.
   - **Лог (137.1).** `ls -ld /Library/Logs/sing-box-lxd` — `root wheel
     drwxr-xr-x`; `ls -l /Library/Logs/sing-box-lxd/classic.log` — `<вы>
     staff -rw-------`, растёт при работе; из другой учётной записи
     `cat` → `Permission denied`; `<Logs>/sing-box.log` при старте с TUN
     не меняется (`ls -l` до и после). Окно логов → Core — строки из
     `classic.log`; профайлер трафика видит DNS-события.
   - **Отказ по логу.** `sudo rm -rf /Library/Logs/sing-box-lxd && sudo ln -s /tmp /Library/Logs/sing-box-lxd`
     → Start → «Failed to start sing-box: … refused: /Library/Logs/sing-box-lxd
     is a symbolic link»; ядро не запущено, в `/tmp` ничего не создано.
     Вернуть: `sudo rm /Library/Logs/sing-box-lxd`.
4. **Отрицательный.** Подменить ядро в DataDir другой сборкой (тот же путь)
   → Start → диалог «outdated» с двумя разными sha, в логе WARN с полными sha;
   AEWP не вызывается. Команда из диалога → Retry → старт.
5. **Обновление ядра кнопкой** (Core → Update) при копии и без службы → в
   логе WARN «root-owned copy … outdated» с обоими sha; следующий Start →
   диалог «outdated». Со службой демона — диалог install SPEC 136, а после
   него Start проходит без второго диалога (install обновил ту же копию).
6. **Служба демона установлена**, копия устарела → Start в classic → диалог с
   командой `--service=install` и строкой о службе.
7. **Kill** в диалоге «Sing-Box already running» и в Diagnostics → процесс
   ядра (`sing-box-lxd run`) и шелл обёртки гаснут (pkill по
   абсолютному пути); `pgrep -f 'sing-box run|sing-box-lxd run|start-singbox-privileged'`
   до Kill находит оба, после — ничего.
8. **Снятие TUN** в визарде при остановленном ядре → пароль **не**
   спрашивается; root-owned `bin/cache.db` и `<Logs>/sing-box.log[.old]`
   прежних версий удалены (в логе лаунчера INFO «removed …»).
9. **Ранняя раскладка (legacy).** Копии `sing-box-lxd` нет, а от dev-сборок
   lx.11 остался каталог `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/`
   или плоский файл `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd`
   → Start → диалог «missing» с командой copy; в логе WARN с причиной
   «… legacy layout of early lx.11 builds, not used: remove it
   (`sudo rm -rf /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd.install.json`)». Команда copy (ядро lx.12) → Retry → старт; затем
   та же `sudo rm -rf` убирает оба варианта legacy.

## 10. Что зависит от lx.11

Подтверждено ядром (ветка форка `27e245e2e`, 24.09.2026):

1. **`lxd --service=copy`** — как в §5: под root делает только копию и
   сайдкар по SPEC 100 §2.3 (каталог, `O_EXCL` → `fsync` → `chown root:wheel`
   → `chmod 0755` → сверка sha → `rename`), без plist и launchd; в сайдкаре
   `plist_path: ""`. Вывод: `lxd: copied <src> -> <dst> (sha256 <hex>,
   root:wheel 0755)`, при совпадении sha — `lxd: already up to date <hex>`.
   Копия заменяется только `rename` — работающий classic-процесс держит
   старый inode.
2. **`sing-box run` под root из бинаря, не прошедшего инвариант, — WARN, не
   отказ.** Ядро само такой запуск не остановит: гейт лаунчера (§4) —
   единственный барьер, и запуск именно копии обязателен на стороне
   лаунчера.
3. **`--service=status`**: exit 0 — OK, 2 — MISMATCH/UNSAFE, 3 — NOT
   INSTALLED, 4 — COPY ONLY (копия без службы), 5 — NOT RUNNING (служба
   не загружена у launchd, ядро `11f440685`), 1 — ошибка. Лаунчер его не
   вызывает (гейт — свой, без запуска процесса); команда — для ручной
   проверки (§9).
4. `--service=install` тоже создаёт/обновляет копию (SPEC 100 §2.3) — на него
   лаунчер полагается при установленной службе.
5. Копия самодостаточна: darwin-релиз линкует libcronet статически — naive
   работает из `/Library/PrivilegedHelperTools/…` без соседних файлов.

6. **`--service=uninstall`** по умолчанию снимает и копию; `--keep-copy`
   снимает plist и launchd, копию и сайдкар оставляет (COPY ONLY, exit 4).
   Лаунчер: вкладка Uninstall службы — с `--keep-copy` (копию запускает
   classic), подсказка «Remove all data…» — полный uninstall (SPEC 136 §5, §7).

Открыто:

7. Релиз lx.11 и бамп `constants.RequiredCoreVersion` (§8 п. 6).

## 11. Связь

- SPEC 136 — служба демона на той же копии; ссылка оттуда (§10).
- SPEC 100 форка — раскладка и копирование; `--service=copy` — дополнение к нему.

## 12. Проверено: где лаунчер собирает и исполняет команды (аудит 24.09.2026)

Grep по всему коду лаунчера (без тестов и `tools/`): `exec.Command`, AEWP,
`sh -c`, `osascript`, `powershell`, sudo-строки, `fmt.Sprintf` в команду —
все платформы. Исправлялись только привилегированные вызовы macOS; остальное
— вердикт. Строки — на коммите `5aac87f3`.

| Файл:строка | Что подставляется | Откуда значение | Под кем | Вердикт |
|---|---|---|---|---|
| `internal/platform/privileged_darwin.go:252` (`StartPrivilegedCore` ← `core/process_service.go:335`) | argv `env -i … /bin/sh -c <тело> <bin> <копия> <конфиг> <каталог лога> 0 <порог>` | bin — DataDir; копия — константа SPEC 136 после гейта; конфиг — `basename(ConfigPath)`; остальное — константы | root (AEWP) | **исправлено** (§1 п. 1 → §3.1): был скрипт из DataDir с путями через `strconv.Quote` и окружение лаунчера |
| `internal/platform/privileged_darwin.go:267` (`KillPrivilegedProcess` ← `core/process_service.go:589, 678`) | argv `/bin/kill -TERM <pid> [<pid>]` | PID из вывода тела старта | root | **исправлено**: было `sh -c "kill …; rm -f \"<pid-файл>\""`, `rm` по PATH лаунчера |
| `internal/platform/privileged_darwin.go:284` (`KillPrivilegedByPattern` ← `core/process_service.go:718`, `ui/diagnostics_tab.go:52`) | argv `/usr/bin/pkill -TERM -f <шаблон>` | константа | root | **исправлено**: было `sh -c "pkill …"`, `pkill` по PATH |
| `ui/configurator/tabs/settings_tun_darwin.go` (снятие TUN) | — | — | пользователь | **исправлено**: было `sh -c "rm -rf \"<пути>\""` под root с путём кэша из конфига (`$(…)` → команда под root); теперь `os.RemoveAll` своим uid (§3.2) |
| `core/daemon_manager_darwin.go:549` (`daemonServiceCommand`: install / copy / uninstall / client add) | `sudo '<bin>' lxd …` — показывается, лаунчер не исполняет | `SingboxPath` или копия | пользователь в своём терминале (sudo) | безопасно: `shellQuote`; тесты `TestDaemonServiceCommandQuoting`, `TestPrivilegedCoreCopyGate` (`sh -n`, разбор аргументов) |
| `core/daemon_manager_darwin.go:457`, `:555` | `sudo grep … '<daemon.json>'`, `sudo launchctl kickstart …` — показываются | константы | пользователь (sudo) | безопасно |
| `core/daemon_manager_darwin.go:627` (`OpenTerminalWithCommand`) | `osascript -e 'tell application "Terminal" … do script "<команда>"'` | команды строк выше | пользователь | безопасно: `appleScriptString` экранирует `\` и `"`, round-trip в `TestDaemonServiceCommandQuoting` |
| `ui/machine_add_window.go:105`, `ui/machine_edit_window.go:137` | `sudo sing-box lxd client add` — показывается для удалённой машины | константа | пользователь удалённой машины | безопасно |
| `internal/platform/file_dialog_darwin.go:14, 34, 136` | `osascript -e <скрипт>` с подписью, расширениями, именем файла | locale (каталог в DataDir), константы, `backup.SuggestFileName(дата)` | пользователь | безопасно: `appleScriptStringLiteral` экранирует `\` и `"`; привилегий нет |
| `core/process_service.go:260`, `core/rebuild.go:39`, `core/core_capabilities.go:76, 168, 254`, `core/core_version.go:45`, `core/core_chain_capability.go:81`, `core/daemon_manager_darwin.go:331` | ядро лаунчера `run -c` / `check -c` / `version` / `lxd --help` (argv) | `SingboxPath`, `ConfigPath` | пользователь | безопасно: argv, свой uid |
| `core/process_detect_darwin.go:21`, `core/tls_roots_darwin.go:63`, `core/netiface/friendly_darwin.go:68`, `internal/platform/proclist_darwin.go:23`, `internal/platform/platform_darwin.go:26, 31, 36, 41, 80, 121`, `internal/platform/device_info_darwin.go:63`, `internal/platform/restart_other.go:27` | `pgrep`, `/usr/bin/security`, `networksetup`, `ps`, `open`, `killall`, `kill -9`, `sw_vers`, `sysctl`, перезапуск себя (argv) | константы, вывод системных утилит, пути лаунчера | пользователь | безопасно: без шелла; поиск по PATH под своим uid повышения не даёт |
| `core/controller.go:555` (`RunHidden`) | argv | — | — | безопасно; вызовов нет — мёртвый код, кандидат на удаление |
| `internal/platform/platform_linux.go:90` (`GetSetCapCommand` ← `core/controller.go:594`, `core/process_service.go:218`) | `sudo setcap '…' %s` — путь **без кавычек**, показывается | `SingboxPath` | пользователь в терминале (sudo) | вне рамок (Linux, лаунчер не исполняет): путь с пробелом или метасимволом даст неверную команду — `shellQuote` отдельной правкой |
| `internal/platform/platform_linux.go:29, 34, 39, 44, 73`, `internal/platform/file_dialog_linux.go:36, 61` | `xdg-open`, `killall`, `kill`, `getcap`, `zenity`/`kdialog` (argv) | пути, URL, подписи | пользователь | безопасно: argv без шелла |
| `internal/platform/file_dialog_windows.go:24, 49, 92` | `powershell -Command <скрипт>` с подписью, фильтром, именем файла через `psSingleQuote` | подпись — locale: `ru.json` в `%LOCALAPPDATA%\singbox-launcher\bin\locale`, скачивается и пишется процессом обычной целостности; имя — дата; расширения — константы | **администратор** (`requireAdministrator`) | **исправлено** отдельным коммитом `fix(windows)`: `psSingleQuote` удваивал только ASCII `'`, а PowerShell считает кавычками и `‘ ’ ‚ ‛` (U+2018–U+201B) — перевод с такой кавычкой выходил из литерала. Теперь скрипт целиком собирается в `file_dialog_ps.go`, каждое значение — base64 от UTF-8, раскодируемое в самом скрипте, и уходит как `-EncodedCommand` (base64 от UTF-16LE); тест `TestPowerShellEncodedDialogScript` |
| `core/process_service.go:813` | `tasklist /FI "IMAGENAME eq sing-box.exe"` (`fmt.Sprintf`) | константа | администратор | безопасно |
| `internal/platform/platform_windows.go:30, 35, 44, 48, 57, 61`, `internal/platform/device_info_windows.go:58`, `internal/platform/singtun_fwrules_windows.go:60`, `internal/platform/glprobe_windows.go:747`, `internal/platform/restart_windows.go:37` | `explorer`, `rundll32 url.dll,FileProtocolHandler <url>`, `taskkill`, `wmic`, `netsh … name=<правило>`, перезапуск себя (argv) | пути, URL, PID, имена правил файрвола (создать правило может только администратор) | администратор | вне рамок (Windows): шелла нет, системный PATH идёт раньше пользовательского; сам запуск ядра из `%LOCALAPPDATA%` под администратором — §8 п. 5 |

