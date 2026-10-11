# SPEC 161 — Окно Service: обслуживание демона локально и на удалённых машинах

Статус: N (new). Тип: F. Дата: 2026-10-10.

## 1. Проблема

Краш-тест не пройден. Когда демон лёг, ядро на машине старее, чем нужно
лаунчеру, или сопряжение потерялось, пользователь видит одну строку
(«❌ Daemon not reachable», «unreachable: connection refused», версия как
текст) и дальше остаётся один: нет ни команд перезапуска, ни рецепта
обновления ядра, ни путей к логу, `daemon.json`, state-dir, ни ссылок на гайды.

Конкретный случай владельца 2026-10-09: новый лаунчер требует ядро
`1.14.3-lx.14` (`constants.RequiredCoreVersion`), на роутере стоит
`1.14.2-lx.11`. Лаунчер версию показывает, но не сравнивает, не
предупреждает и не говорит, как обновить. Deploy на старое ядро может
откатиться на last-good, и причина «обнови ядро» нигде не произносится.

Что есть сейчас и разбросано по разным местам:

- Локально (Servers → ⚙ → Local): вкладки Status / Install / Uninstall,
  плашка состояния службы (SPEC 136 §6), команды install/bootstrap,
  `DaemonRepairCommand`, `DaemonShowSecretCommand`; `DaemonKickstartCommand`
  существует, но в окне не показан; путей к логу и `daemon.json` нет.
- Удалённо (вкладка Remote, `ui/machine_list_panel.go`): строка машины с
  версией и статусом, окно ⓘ с сырыми фактами (state dir, sha), ни одной
  команды, ни одного пути к службе/логу.
- Доки: `docs/TROUBLESHOOTING.md` про демон не знает; гайды форка
  (`lxd-daemon`, `openwrt-vpn-ssid`) из лаунчера не достижимы.

## 2. Цель

Одно окно **Service** — раннбук обслуживания демона, одинаковый по устройству
для локальной службы и для каждой удалённой машины. Открывается
шестерёнкой, сразу на вкладке, которую выбрал диагноз. Каждая вкладка —
один случай: не поднимается, ядро, сопряжение, справочник (пути,
`daemon.json`, логи). Внизу каждой вкладки — ссылки на гайды.

## 3. Принципы

1. **Раннбук, а не справочник.** Сверху диагноз, открывается на нужной
   вкладке; проблемная вкладка помечена глифом в ярлыке (`⚠ Core`,
   `✖ Not running`).
2. **Команды точные для этой машины.** Её адрес, её пути из паспорта демона
   (`/admin/info`: `state_dir`, `executable`, `log_path`, `listen`, `tls`).
   Что демон не сообщал — подписано `default`, а не выдано за факт.
3. **Рецепты не пересобирают командную строку службы.** На конкретном
   роутере init-скрипт содержит `-c min.json` и `GOMEMLIMIT`, лаунчер этого
   не знает. Только имя службы (`sing-box-lxd`) и `restart/stop/start`.
4. **Где выполняется — всегда видно.** Для удалённой машины поле показывает
   короткую команду без обёртки; `⧉` копирует её как есть (вставить в свою
   ssh-сессию), `▶` открывает Terminal на этом Mac уже с
   `ssh <ssh-target> '<команда>'`. Для локальной — `sudo …`, `▶` открывает
   Terminal (macOS) или «Run as administrator» (Windows, `daemonOps`).
5. **От дешёвого к дорогому.** status → restart → лог → last-good →
   переустановка. У каждого шага — «если помогло, дальше не надо».
6. **Окно само обновляется.** Шапка подписана на heartbeat машины
   (`ui/machine_heartbeat.go`) / на обновление статуса локального демона;
   выполнил команду в Terminal — через тик шапка позеленела. Кнопок
   «Re-check» нет.
7. **Проверенный рецепт обновления ядра**, а не выдуманный: скачать здесь
   → залить потоком через ssh (на OpenWrt `scp` не работает, нет sftp) →
   проверить новым бинарём `version` и `check -c last_good.json` →
   бэкап → stop → mv → chmod → start → откат из бэкапа.
8. **Опасное свёрнуто**: Remove the service, Revoke client, Roll back —
   в сворачиваемых секциях с красной подписью и подтверждением.
9. **Гайды — в каждой вкладке**, по локали (`.ru.md` / `.md`), открываются
   в браузере через `platform.OpenURL`.

## 4. Точки входа

### 4.1 Строка удалённой машины (вкладка Remote)

```
┌ routerich ─ 192.168.10.1:19091 ────────────────────────────────────────┐
│ 1.14.2-lx.11 · started                   [ⓘ] [⚙•] [Stop] [⟳]           │
│ ⚠ Core lx.11 is older than required (lx.14)                            │
│ [Configure] [Deploy•] [RES]  [▸ more]                                  │
└────────────────────────────────────────────────────────────────────────┘
```

- `⚙` — новая кнопка рядом с `ⓘ`, открывает окно Service этой машины.
  Точка в углу (`withCornerDot`): жёлтая — ядро старее требуемого; красная —
  демон не отвечает или `interrupted_apply` вместе с недоступностью.
  Тултип говорит словами, что чинить: «Core is older than required — open
  Service», «Daemon does not answer — open Service».
- Строка предупреждения под статусом: жёлтый `⚠` + текст или красный `✖` +
  текст ошибки. Без кнопки, без цветной полосы — лечится шестерёнкой.
  Без проблем строки нет.
- Три точки единообразны: Deploy• — сохранённый конфиг ≠ активный
  (есть), Configure• — последний deploy откатился (есть), ⚙• — служба или
  ядро (новая).
- `ⓘ` остаётся «что демон сообщает о себе» без изменений.

### 4.2 Локально (Servers → ⚙ → Local)

Та же шестерёнка уже есть. Панель Local перестраивается в то же окно
Service (шапка + те же четыре вкладки) плюс существующая вкладка
**Uninstall** (деструктивная, остаётся отдельной). Существующее содержимое
перераспределяется:

| Сейчас | Куда |
|---|---|
| Status: строки статуса, плашка службы (SPEC 136 §6) | Шапка окна + вкладка Not running |
| Status/Install: `Install or update service`, bootstrap | Core |
| Install: Pair / Re-pair / `client add` / Bearer secret | Pairing |
| `DaemonKickstartCommand` (есть в core, в UI нет) | Not running |
| `DaemonShowSecretCommand` | Reference (daemon.json) |
| Uninstall | Uninstall (без изменений) |

Движок (radio classic/daemon) и `Stop VPN when quitting` остаются там,
где они есть сейчас (над вкладками / на вкладке LOCAL), в окно Service не
входят.

### 4.3 Deploy на машину со старым ядром

```
┌ Deploy to routerich? ────────────────────────────────────────┐
│ ⚠ routerich runs core 1.14.2-lx.11; this launcher builds     │
│   configs for 1.14.3-lx.14. The old core may reject them —   │
│   it will then roll back to the last working config.         │
│ [ ] Don't ask again for lx.11 on this machine                │
│           [How to update]   [Deploy anyway]   [Cancel]       │
└──────────────────────────────────────────────────────────────┘
```

Только предупреждение, без запрета. «How to update» открывает Service на
вкладке Core. Чекбокс запоминает подтверждённую версию в записи машины;
при смене версии ядра на машине предупреждение возвращается.

## 5. Окно Service

### 5.1 Шапка (общая для всех вкладок)

```
┌ Service — routerich ─────────────────────────────────────────────────── ✕ ┐
│ OpenWrt · linux/arm64 · service sing-box-lxd (procd)   Guides: lxd · OpenWrt│
│ Core 1.14.2-lx.11 (required 1.14.3-lx.14 ⚠)                               │
│ ✖ Not answering for 12 min: connection refused, 5 attempts                │
│ Commands run via  ssh root@192.168.10.1                                   │
│ ┌✖ Not running┐┌⚠ Core┐┌Pairing┐┌Reference┐                               │
```

- Платформа и архитектура — из записи машины (`RemoteDaemon.Target()`);
  локально — `runtime`.
- Init-система для Linux: `systemd` / `OpenWrt (procd)` — переключатель
  `Select` в шапке, пока демон её не сообщает; выбор хранится в записи
  машины (новое поле). Дефолт: `OpenWrt` для `linux/arm*`, `linux/mipsle`;
  `systemd` для `linux/amd64`. macOS — launchd, Windows — SCM, без
  переключателя.
- Версия: running vs `constants.RequiredCoreVersion`, сравнение
  `parseCoreBuild`/`compareCoreBuilds` (`core/daemon_service_state.go`).
- Состояние: зелёная строка «✅ answers · uptime 3 h» / красная с ошибкой
  и относительным временем «for 12 min» (из истории отказов heartbeat).
- SSH-target показывается; редактируется в окне Edit машины (новое поле
  «SSH» рядом с адресом, по умолчанию `root@<хост адреса демона>`,
  допускает `user@host` и `user@host:port`).
- «Guides:» — ссылки по локали (см. §6).
- Ярлык вкладки с глифом, когда в ней есть что чинить. Окно открывается
  на первой проблемной вкладке в порядке Not running → Core → Pairing;
  иначе на Not running.

### 5.2 Вкладка Not running

Демон не отвечает:

```
│ │ Likely: the service is stopped or crash-looping.                      │ │
│ │ 1  Is it running?                                                     │ │
│ │    /etc/init.d/sing-box-lxd status                           [⧉] [▶]  │ │
│ │ 2  Restart it                                                         │ │
│ │    /etc/init.d/sing-box-lxd restart                          [⧉] [▶]  │ │
│ │    if the header turns green, you are done                            │ │
│ │ 3  Still down? Read why                                               │ │
│ │    tail -n 100 /tmp/lxd.log                                  [⧉] [▶]  │ │
│ │    "refusing to run"  → Core tab · "bind: address in use" → Reference │ │
│ │ 4  The last config broke it? Boot the last working one once           │ │
│ │    /etc/init.d/sing-box-lxd stop && /usr/bin/sing-box lxd \           │ │
│ │      --state-dir /etc/sing-box-lxd/state \                            │ │
│ │      --config-force /etc/sing-box-lxd/state/last_good.json   [⧉] [▶]  │ │
│ │    (foreground; Ctrl-C and `start` the service after)                 │ │
│ │ 5  Nothing helps → Core tab: reinstall the core                       │ │
│ │ Guide: Troubleshooting → Daemon                                       │ │
```

Демон отвечает: «✅ The daemon answers, uptime 3 h — nothing to fix here»,
ниже только Restart и «Read the log». Пять шагов спасения не показываются.

Команды по платформам (имя службы `sing-box-lxd` везде):

| Шаг | OpenWrt (procd) | systemd | macOS (launchd) | Windows (SCM) |
|---|---|---|---|---|
| status | `/etc/init.d/sing-box-lxd status` | `systemctl status sing-box-lxd` | `launchctl print system/com.leadaxe.sing-box-lxd` | `sc.exe query sing-box-lxd` |
| restart | `/etc/init.d/sing-box-lxd restart` | `systemctl restart sing-box-lxd` | `sudo launchctl kickstart -k system/com.leadaxe.sing-box-lxd` (`DaemonKickstartCommand`) | `sc.exe stop` + `sc.exe start` |
| not loaded (macOS NotRunning) | — | — | `DaemonBootstrapCommand` | — |
| log | `tail -n 100 <log_path>` | то же | `sudo tail -n 100 '<log_path>'` | `Get-Content -Tail 100 '<log_path>'` |
| last-good | `stop && <executable> lxd --state-dir <state_dir> --config-force <state_dir>/last_good.json` | то же | то же под sudo | то же в PowerShell от администратора |

`<log_path>`, `<executable>`, `<state_dir>` — из последнего паспорта
демона (кэш в записи машины / в состоянии локального демона); при их
отсутствии — платформенные дефолты с пометкой `default`:
OpenWrt `/usr/bin/sing-box`, `/etc/sing-box-lxd/state`, `/tmp/lxd.log`;
systemd `/usr/local/bin/sing-box`, `/var/lib/sing-box-lxd/state`,
`/var/lib/sing-box-lxd/lxd.log`; macOS и Windows — из
`core/daemon_manager_*.go` (там они известны точно).

### 5.3 Вкладка Core

```
│ │ Running 1.14.2-lx.11 → required 1.14.3-lx.14                          │ │
│ │ 1  Get the core for linux/arm64                                       │ │
│ │    [Download 1.14.3-lx.14]  ▓▓▓▓▓░░░ 62%                              │ │
│ │    ~/Downloads/sing-box-1.14.3-lx.14-linux-arm64  ✓ sha256 matches    │ │
│ │ 2  Upload (streamed over ssh — scp does not work on OpenWrt)          │ │
│ │    ssh root@192.168.10.1 'cat > /tmp/sing-box.new' \                  │ │
│ │      < ~/Downloads/sing-box-1.14.3-lx.14-linux-arm64         [⧉] [▶]  │ │
│ │ 3  Check before swapping: build tags and the current config           │ │
│ │    chmod +x /tmp/sing-box.new && /tmp/sing-box.new version &&         │ │
│ │    /tmp/sing-box.new check -c /etc/sing-box-lxd/state/last_good.json  │ │
│ │                                                              [⧉] [▶]  │ │
│ │    expect: with_lxd … and no error from check                         │ │
│ │ 4  Back up, swap, restart                                             │ │
│ │    cp /usr/bin/sing-box /root/sing-box.lx11.bak &&                    │ │
│ │    /etc/init.d/sing-box-lxd stop &&                                   │ │
│ │    mv /tmp/sing-box.new /usr/bin/sing-box &&                          │ │
│ │    chmod 755 /usr/bin/sing-box &&                                     │ │
│ │    /etc/init.d/sing-box-lxd start                            [⧉] [▶]  │ │
│ │ 5  The header should now say 1.14.3-lx.14 · started                   │ │
│ │ ▸ Roll back if the new core misbehaves                                │ │
│ │ ▸ Install from scratch (no service yet)                               │ │
│ │ ▸ Remove the service                                                  │ │
│ │ Guide: lxd-daemon → Linux (§8) · OpenWrt                              │ │
```

- Шаг 1: скачивание ядра **под платформу машины**, а не под текущую:
  имя ассета по `GOOS/GOARCH` машины (расширить `SingboxAssetSuffix` до
  `SingboxAssetSuffixFor(goos, goarch)`), прямой URL релиза
  `constants.RequiredCoreVersion`, распаковка, сверка sha256 с `SHA256SUMS`
  релиза (если файл есть в релизе; иначе — показать sha256 скачанного и
  попросить сверить в шаге 3). Файл в `~/Downloads/` под именем с версией
  и платформой. Прогресс — как на вкладке Core дашборда
  (`DownloadProgress`). Если файл уже есть и sha совпал — сразу `✓`.
- Шаг 2 — единственная команда, выполняемая **на этом Mac** (поле
  содержит её целиком, без дополнительной ssh-обёртки).
- Шаг 4: имя бэкапа с версией текущего ядра (`sing-box.<ver>.bak`; при
  неизвестной — дата).
- Roll back: `cp <bak> <executable> && restart`.
- Install from scratch (Linux): init-скрипт procd или systemd-unit из
  гайда форка с реальными путями, копируемое поле, затем `enable`/`start`
  (`/etc/init.d/sing-box-lxd enable && … start` /
  `systemctl daemon-reload && systemctl enable --now sing-box-lxd`), и
  напоминание про `/etc/sysupgrade.conf` для OpenWrt.
- Remove the service: `disable && stop` + `rm` init-скрипта; state не
  трогается (отдельная строка `rm -r <state_dir>` с красной подписью).
- Локально (macOS/Windows): шаг 1 — существующая логика Core дашборда
  (ядро лаунчера), шаги 2–4 заменяются одной строкой
  `Install or update service` (`DaemonInstallCommand`, идемпотентно,
  SPEC 136 §5) и, для NotRunning, bootstrap; Roll back — нет (копия
  переустанавливается тем же install); Remove — ведёт на вкладку
  Uninstall.
- Версия в порядке: вкладка показывает «✅ Core 1.14.3-lx.14 is current»,
  рецепт остаётся доступным свёрнутым («▸ Reinstall the same version»).

### 5.4 Вкладка Pairing

```
│ │ ✅ Paired · certificate ok                                            │ │
│ │ Re-pair when: "certificate changed", "not paired", the service was    │ │
│ │ reinstalled or the state dir was wiped.                               │ │
│ │ 1  Mint an invite on the machine                                      │ │
│ │    /usr/bin/sing-box lxd client add --name singbox-launcher \         │ │
│ │      --state-dir /etc/sing-box-lxd/state                     [⧉] [▶]  │ │
│ │ 2  Paste what it printed                                              │ │
│ │    [ address#fingerprint#code                            ]   [Pair]   │ │
│ │ ▸ Who is trusted      … client list   --state-dir …          [⧉] [▶]  │ │
│ │ ▸ Revoke a client     … client remove <name> --state-dir …   [⧉]      │ │
│ │ ▸ Plain mode (tls:false): Bearer secret [••••••] [Save]               │ │
│ │ Guide: lxd-daemon → Pairing (§9)                                      │ │
```

- Состояние: paired / not paired / certificate changed (из классификации
  ошибок `core/backend_daemon.go:215–250` и `lxdclient`), красным с
  причиной.
- Pair — существующие `PairDaemonWithInvite` (локально) и логика
  re-pair машины из `ui/machine_edit_window.go:140–156` (удалённо);
  после успеха шапка обновляется.
- Локально `client add` — `DaemonRepairCommand`; Bearer secret — поле из
  текущего Install (`SetDaemonSecret`, подсказка `DaemonShowSecretCommand`).

### 5.5 Вкладка Reference

```
│ │ Paths                       reported by the daemon 3 min ago · ᵈ default│
│ │   Core binary    /usr/bin/sing-box                              [⧉]   │ │
│ │   Service        /etc/init.d/sing-box-lxd                     ᵈ [⧉]   │ │
│ │   State dir      /etc/sing-box-lxd/state                        [⧉]   │ │
│ │   Settings       /etc/sing-box-lxd/state/daemon.json            [⧉]   │ │
│ │   Last-good      /etc/sing-box-lxd/state/last_good.json         [⧉]   │ │
│ │   Log            /tmp/lxd.log                                   [⧉]   │ │
│ │ daemon.json — edit, then restart the service (never reinstall)        │ │
│ │   listen        where it answers       192.168.10.1:19091             │ │
│ │   tls           mTLS on/off            true                           │ │
│ │   secret        Bearer for tls:false   (shown on the machine only)    │ │
│ │   log_file      log path               /tmp/lxd.log                   │ │
│ │   log_max_size_mb / log_max_backups / log_max_age_hours   1 / 1 / 24  │ │
│ │   Show it    cat /etc/sing-box-lxd/state/daemon.json         [⧉] [▶]  │ │
│ │ Logs                                                                  │ │
│ │   [Live log window]                 (while the daemon answers)        │ │
│ │   tail -f /tmp/lxd.log                                        [⧉] [▶] │ │
│ │   Who holds the port   netstat -lnp | grep 19091              [⧉] [▶] │ │
│ │ Guide: DAEMON_AND_REMOTE · lxd-daemon → daemon.json (§3)              │ │
```

- Значения `listen`, `tls`, `log_file` — из паспорта; остальные ключи —
  описание и дефолт из гайда форка §3.
- Live log — существующий стрим (`SubscribeLogLines`, локально — окно
  логов ядра daemon-режима).
- macOS: Settings/Log под `sudo cat`/`sudo tail` (каталог root-only);
  Windows: `Get-Content`; «кто держит порт» — `lsof -i :<port>` /
  `netstat -ano | findstr <port>`.
- Локально дополнительно: Service binary (root-owned копия), Install
  record (`.install.json`), Service plist / SCM-имя — из
  `core/daemon_service_state_*.go`.

## 6. Гайды (ссылки)

| Вкладка | Ссылки |
|---|---|
| Шапка | `lxd-daemon`, для OpenWrt-машин ещё `openwrt-vpn-ssid` |
| Not running | `docs/TROUBLESHOOTING` → раздел «Daemon» (новый) |
| Core | `lxd-daemon` §7 (macOS) / §7a (Windows) / §8 (Linux), `openwrt-vpn-ssid` |
| Pairing | `lxd-daemon` §9 |
| Reference | `docs/DAEMON_AND_REMOTE`, `lxd-daemon` §3 |

База: `https://github.com/Leadaxe/singbox-launcher/blob/main/docs/<NAME>.md`
и `https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/<name>.md`;
при локали `ru` — `.ru.md`. Константы URL — в `internal/constants`.
Открытие — `platform.OpenURL`, виджет — `widget.Hyperlink`.

## 7. Данные

Новые поля записи машины (`services.RemoteDaemon`): `ssh` (string,
опционально), `init_system` (`""|systemd|procd`), `core_warn_ack`
(подтверждённая версия ядра для диалога Deploy), кэш паспорта
(`executable`, `state_dir`, `log_path`, `listen`, `tls`, `version`,
`seen_at`) — чтобы пути были видны и когда демон лёг. Хранение — там, где
живёт реестр машин сейчас; PLAN проверяет, входит ли реестр в бэкап/контракт
LxBox, и если да — поля добавляются в форме контракта (`contract/VERSION`
+ changelog + параграф в `contract/TASKS_LXBOX.md`).

Локальный демон: кэш паспорта — в состоянии `AppController` (уже есть
`DaemonUIStatus`; добавить `LogPath`, `Executable`, `Listen`).

## 8. Документация

- `docs/TROUBLESHOOTING.md` (+ `.ru.md`): раздел «Daemon: не поднимается /
  обновить ядро / сопряжение / где что лежит» — те же рецепты по
  платформам, что в окне.
- `docs/DAEMON_AND_REMOTE.md` (+ `.ru.md`): §2 и §4 — окно Service, точки
  входа, предупреждение о версии при Deploy.
- `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md` (значимый UX).
- Новые ключи `locale.T` — сразу с переводами в `bin/locale/ru.json`.

## 9. Вне рамок

- Автообновление ядра на удалённой машине (ручка в демоне, подмена
  бинаря) — отдельная задача форка; здесь только рецепт.
- Запрет Deploy при старом ядре — нет, только предупреждение.
- Изменение Local-панели сверх перераспределения по вкладкам (движок,
  `Stop VPN when quitting`) — не трогать.

## 10. Критерии приёмки

1. На строке машины со старым ядром: `⚙•` жёлтая, строка `⚠ Core … older
   than required`, Deploy показывает диалог с «Deploy anyway» и чекбоксом.
2. При недоступном демоне: `⚙•` красная, строка `✖ …`, окно открывается на
   `✖ Not running` с рецептом под init-систему машины, команды содержат
   реальные пути из последнего паспорта (или `default`).
3. `▶` открывает Terminal с `ssh <target> '<cmd>'`; `⧉` копирует команду
   без обёртки.
4. Вкладка Core скачивает ядро под `linux/arm64` машины, сверяет sha256,
   шаги 2–4 содержат путь скачанного файла и реальные пути машины.
5. После `restart` на машине шапка зеленеет без кликов.
6. Локальная панель Local: та же шапка и вкладки; все прежние действия
   (install, bootstrap, pair, re-pair, secret, uninstall, purge) доступны
   и работают как раньше; `kickstart` появился.
7. Ссылки на гайды есть в шапке и в каждой вкладке, открываются по локали.
8. `docs/TROUBLESHOOTING` содержит раздел «Daemon».
9. Тесты: UI без тестов. Логика «версия старее требуемой» и построение
   команд по платформе — один табличный тест в `core`.
