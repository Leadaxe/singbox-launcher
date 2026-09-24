# 137 · Classic TUN на macOS: root запускает только root-owned копию ядра

Решение владельца и главной сессии — 24.09.2026. Приоритет — сразу после
SPEC 136, до следующего релиза лаунчера. Парная задача к SPEC 100 форка
(`sing-box-lx`, релиз **v1.14.1-lx.11**): та же каноническая копия ядра, что
у службы демона SPEC 136, плюс новая команда ядра `lxd --service=copy`.

Статус: **реализовано в ветке `spec-137-classic-root-owned` (от
`spec-136-daemon-root-owned`), ждёт ядра lx.11, ручной проверки владельцем и
CI**.

## 1. Проблема: что исполнялось под root

Classic-движок на macOS поднимает ядро с TUN через
`AuthorizationExecuteWithPrivileges` (AEWP). До этой задачи root исполнял
пользовательские файлы и пользовательское окружение.

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
   `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/sing-box` (SPEC 136
   §3, интерфейс lx.11) и системные утилиты по абсолютным путям —
   `/usr/bin/env`, `/bin/sh`, `/bin/kill`, `/usr/bin/pkill`, `/bin/rm`.
   Никаких файлов из DataDir, бандла или `PATH`.
2. **Окружение root-шелла очищено**: `/usr/bin/env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin`.
3. **Тело шелла — константа** в бинаре лаунчера; пути — позиционными
   аргументами (argv, без шелл-квотинга).
4. **Гейт перед стартом** (§4): нет копии, цепочка владения нарушена или sha
   не совпал — старт с привилегиями не выполняется, AEWP не вызывается.
5. Не-macOS платформы и daemon-режим (SPEC 136) не меняются.

## 3. Привилегированные вызовы после задачи

| Действие | Вызов AEWP |
|---|---|
| Старт TUN | `/usr/bin/env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin /bin/sh -c '<тело>' start-singbox-privileged <Data>/bin <копия> config.json <Logs>/sing-box.log` |
| Stop / рестарт | `/bin/kill -TERM <PID шелла> [<PID ядра>]` |
| «Sing-Box already running» → Kill, Diagnostics → Kill | `/usr/bin/pkill -TERM -f 'sing-box run\|start-singbox-privileged'` |
| Снятие галки TUN | `/bin/rm -rf -- <пути>` |

Тело старта (`platform.privilegedStartBody`):

```sh
echo $$
cd "$1" || exit 1
"$2" run -c "$3" >>"$4" 2>&1 &
echo $!
exec >>"$4" 2>&1
wait
```

- Первые две строки stdout — PID шелла и PID ядра, их читает
  `RunWithPrivileges` (как у прежнего скрипта); затем stdout шелла уходит в
  лог, шелл ждёт ядро, и его выход — выход ядра (`WaitForPrivilegedExit` →
  `onPrivilegedScriptExited`). `env` заменяет себя шеллом через `exec`, PID
  не меняется.
- `$0` — `start-singbox-privileged`, а не `--` из постановки: в `sh -c` первый
  аргумент после тела становится `$0`, и имя держит совпадение с
  `platform.PrivilegedPkillPattern` — `pgrep`/`pkill` находят обёртку, как
  находили прежний скрипт.
- `cd … || exit 1`: без каталога данных ядро не стартует в чужом cwd.
- `ps` показывает `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/sing-box run -c config.json` от root.
- pid-файл `<Data>/bin/singbox.pid` пишет и удаляет лаунчер от имени
  пользователя; `rm` под root больше нет. PID ≤ 0 в `kill` не передаются
  (`kill 0` — группа процессов).
- Прежний `<Data>/bin/start-singbox-privileged.sh` при старте удаляется
  (best-effort) — его больше ничто не исполняет.

## 4. Гейт перед стартом с привилегиями

`checkPrivilegedCoreCopy` (`core/classic_privileged_darwin.go`), по порядку:

| Вердикт | Условие | Что дальше |
|---|---|---|
| **no core** | ядро лаунчера (`EvalSymlinks(SingboxPath)`) не найдено или не читается | обычная ошибка старта: сравнивать не с чем, и команде копирования нечего копировать |
| **missing** | звено цепочки или сама копия отсутствует при root-owned родителях | диалог «missing» |
| **unsafe** | цепочка `/Library` → `/Library/PrivilegedHelperTools` → каталог → файл не проходит инвариант SPEC 136 §4 (`Lstat`: не симлинк, uid 0, `mode & 022 == 0`, каталоги — каталоги, файл — обычный файл), либо копия не прочиталась | диалог «not protected» |
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
| Б | авторизация на одно действие | пароль на каждый старт, каждую остановку, каждый рестарт при применении конфига и каждое снятие TUN; авто-рестарт после падения тоже спросит пароль |

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

1. **Лог ядра под root.** Root-шелл открывает `>>"<Logs>/sing-box.log"` —
   путь в каталоге пользователя; подменённый на симлинк, он заставит root
   дописать вывод ядра в чужой файл (часть вывода управляется конфигом).
   Так было и до задачи. Варианты: вывод ядра через канал AEWP в лаунчер
   (лаунчер пишет лог от своего имени; ядро завершится по SIGPIPE вместе с
   лаунчером — смена поведения), либо приёмник вывода от имени пользователя.
2. **Конфиг пользователя под root.** Ядро от root читает `config.json` и
   пишет по путям из него (`experimental.cache_file.path`, `log.output`) —
   как и служба демона; свойство модели «root запускает пользовательский
   конфиг».
3. **`rm -rf` под root при снятии TUN.** Подстановки команд больше нет, но
   `rm` следует симлинкам в каталогах-звеньях DataDir. Файлы в `bin/` и
   `logs/` пользователь может удалить сам (право на удаление даёт каталог), —
   кандидат на отказ от root в этом месте.
4. **Команда copy/install исполняет под sudo ядро лаунчера** —
   пользовательский файл, как в SPEC 136 §5: пароль пользователь вводит в
   своём терминале, диалог показывает оба sha.
5. **Windows** — лаунчер с `requireAdministrator` (`app.manifest`) запускает
   ядро из `%LOCALAPPDATA%\singbox-launcher\bin`, куда пишет процесс обычной
   целостности: тот же класс (повышение до высокой целостности). **Linux** —
   `setcap` привязан к файлу, и запись в файл снимает capability: дыры нет.
   Не трогаются (норма 5).
6. **Бамп `constants.RequiredCoreVersion` до lx.11** — вместе с SPEC 136, при
   релизе. До него ядро без `--service=copy` команду из диалога не выполнит,
   и старт с TUN невозможен, пока копии нет: релиз лаунчера с этой задачей
   без ядра lx.11 не выпускается.

## 9. Ручная проверка (Mac владельца, ядро lx.11, classic-движок, конфиг с TUN)

1. **До.** Службы демона нет (`ls /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist`
   → нет), копии нет (`ls -l /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/`
   → нет). Start → пароль **не** спрашивается, диалог «Core copy for privileged
   start is missing» с командой `sudo '…/sing-box' lxd --service=copy`; в логе
   WARN «privileged start refused». `ls <Data>/bin/start-singbox-privileged.sh`
   → нет (удалён при старте).
2. Run in Terminal → выполнить команду (sudo вводит владелец).
3. **После.**
   - `ls -ld /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd` и
     `ls -l …/sing-box …/install.json` — `root wheel`, `drwxr-xr-x` /
     `-rwxr-xr-x` / `-rw-r--r--`; plist службы не появился;
   - `shasum -a 256 …/sing-box` == `shasum -a 256 ~/Library/Application\ Support/singbox-launcher/bin/sing-box`;
   - Retry → пароль AEWP (раз за сессию), VPN работает;
   - `ps -axo user,pid,command | grep -E 'sing-box run|start-singbox-privileged'`
     — `root … /Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd/sing-box run -c config.json`
     и `root … /bin/sh -c … start-singbox-privileged …`;
   - Stop / Start / применение конфига из визарда (рестарт ядра) — без
     нового пароля (вариант А); Stop гасит оба процесса,
     `<Data>/bin/singbox.pid` удалён.
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
   ядра гаснет (pkill по абсолютному пути).
8. **Снятие TUN** в визарде при остановленном ядре → root-owned `cache.db` и
   логи ядра удалены.

## 10. Что зависит от lx.11

1. **`lxd --service=copy`** (новое, в SPEC 100 форка его ещё нет): под root
   делает только копию и сайдкар §2.3 SPEC 100 (каталог, `O_EXCL` →
   `fsync` → `chown root:wheel` → `chmod 0755` → сверка sha → `rename`),
   без plist и launchd; идемпотентен по sha; копия заменяется только
   `rename` — работающий classic-процесс держит старый inode; нарушение
   инварианта у существующих звеньев — отказ с ненулевым кодом; в сайдкаре
   `plist_path` пустой (или иной признак «копия без службы»).
2. `--service=install` тоже создаёт/обновляет копию (SPEC 100 §2.3) — на него
   лаунчер полагается при установленной службе.
3. `--service=uninstall` по SPEC 100 §2.4 снимает и копию (в том числе
   «осиротевшую»): после удаления службы classic снова попросит команду copy.
   Решение форка: снимать ли копию, у которой есть classic-потребитель.
4. Копия самодостаточна: darwin-релиз линкует libcronet статически — naive
   работает из `/Library/PrivilegedHelperTools/…` без соседних файлов.
5. Самопроверка SPEC 100 §2.5 касается только `lxd`; `run` из копии под root
   её не проходит и не должен падать от неё.

## 11. Связь

- SPEC 136 — служба демона на той же копии; ссылка оттуда (§10).
- SPEC 100 форка — раскладка и копирование; `--service=copy` — дополнение к нему.
