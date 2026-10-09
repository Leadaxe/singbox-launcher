# Решение проблем

Известные проблемы и где лежат их решения. Быстрые первые проверки — в
[таблице в README](../README.ru.md#troubleshooting); проблемы сборки на Linux — в
[BUILD_LINUX.ru.md](BUILD_LINUX.ru.md).

English version: [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## Все платформы

### Окно перестало отвечать

Через ~30 с зависания лаунчер пишет `ui-freeze-<дата-время>.txt` в папку логов (Диагностика → **Папка логов**).
Приложите этот файл и основной лог к issue — по нему видно, чего ждало окно.

### REALITY-узел показывает «Ошибка», в логе ядра `reality verification failed`

Тест узлов VLESS/Trojan с `security=reality` даёт «Ошибка», `direct-out` при этом отвечает.
В окне логов на вкладке **Ядро** — строки вида
`open connection to … using outbound/…: reality verification failed`, и приходят они
быстро (сотни миллисекунд), без таймаута.

Что это значит: сервер доступен, TLS-рукопожатие прошло, но сервер не опознал клиента
как своего. REALITY в таком случае не рвёт соединение, а молча отдаёт настоящий
сертификат сайта-маскировки (`sni` из ссылки) — так задумано, чтобы сервер нельзя было
вычислить по форме отказа. Поэтому все причины ниже выглядят в логе одинаково.

**Системные сертификаты и настройка «TLS root certificate store» тут ни при чём.** Для
REALITY ядро проверяет свою подпись, а сертификат маскировки сверяет с системным
хранилищем ОС независимо от этой настройки. Если бы системные сертификаты устарели,
ошибка была бы другой — `x509: certificate signed by unknown authority`. Если после
смены этой настройки узел заработал, помогло другое: лаунчер пересобрал конфиг и
перезапустил ядро, или прошло временное состояние сети (см. п. 5).

Что проверить, по порядку:

1. **Отпечаток (Fingerprint) узла.** Серверы Xray v26.9.8 и новее принимают только
   ClientHello с постквантовым key share `X25519MLKEM768`. Его несут `chrome` (и все
   `chrome_*`), а с ядром из лаунчера 1.6.2+ (sing-box-lx ≥ 1.14.1-lx.3) — ещё
   `firefox` (Firefox 148) и `safari` (Safari 26.3). `ios`, `android`, `edge`, `360`,
   `qq` его не несут и такими серверами отвергаются; `random` и `randomized` проходят
   через раз. Быстрая проверка — поставить в узле `chrome`: если заработало, причина
   здесь.
2. **Версия лаунчера и ядра.** На старом ядре тот же симптом дают `firefox`/`safari`
   (до 1.6.2) и отсечка по версии клиента `minClientVer`, которую Xray включает по
   умолчанию с v26.7.11. Обновите лаунчер — ядро идёт в комплекте.
3. **Параметры ссылки.** `pbk` (public key) и `sid` (short id) должны совпадать с
   сервером. Если ссылку перевыпустили, старая перестанет проходить именно так.
4. **Часы.** Сервер может отвергать клиентов с большим расхождением времени. Включите
   автоматическую установку времени и часового пояса в системе.
5. **Сеть.** Проверьте тот же узел через другую сеть, например раздачу с телефона.
   Бывает, что провайдер временно (на 1–2 минуты) портит соединения к конкретному
   адресу после серии неудачных попыток: частые повторные тесты подряд это
   состояние продлевают, поэтому между проверками стоит сделать паузу.

Если ссылка проходит у другого человека на свежей версии, а у вас нет — причина на
вашей стороне (п. 1, 2, 4, 5), а не на сервере.

## Linux

### Пароль спрашивают три раза при старте VPN и один раз при остановке

Десктоп с `systemd-resolved`: sing-box настраивает DNS TUN-интерфейса через `resolvectl`,
и Polkit авторизует каждое из четырёх D-Bus-действий отдельно. `CAP_NET_ADMIN` у бинаря не
помогает — проверка идёт на стороне `systemd-resolved`.

Рецепт от пользователя (отдельная группа и узкое правило Polkit ровно на эти четыре
действия) — в [issue #126](https://github.com/Leadaxe/singbox-launcher/issues/126). Это системная настройка, её делает администратор;
лаунчер её не устанавливает, и на других дистрибутивах проект её не проверял.

## Windows

### Start показывает «Для TUN нужны права администратора»

Лаунчер работает без прав администратора (без запроса UAC на старте), а TUN создаёт
сетевой адаптер и меняет маршруты — Windows разрешает это только администраторам.
В диалоге:

- **Перезапустить от имени администратора** — один запрос UAC; лаунчер
  перезапустится с правами и с той же папкой данных (в заголовке окна —
  `(Администратор)`) и поднимет VPN. Под обычной учётной записью Windows спросит
  пароль администратора, и лаунчер будет работать под его учётной записью. Отказ в
  запросе оставляет диалог открытым.
- **Перейти в режим прокси** — выключает TUN и включает локальный прокси с системным
  (порт `proxy_in_listen_port`, по умолчанию 7890): браузеры и большинство
  приложений пойдут через VPN, остальные — напрямую. Недоступна при открытом
  конфигураторе — сначала закройте его.

Чтобы всегда запускать с правами, используйте ярлык с «Запускать от имени
администратора»; автозапуск (Settings → Connection) всегда запускает лаунчер без прав.

### «Sing-Box appears to be already running», а Kill говорит, что нужны права администратора

sing-box запустил лаунчер с правами, которого уже нет (снят в Диспетчере задач или
упал), и лаунчер без прав его остановить не может. Выберите в сообщении
**Перезапустить от имени администратора**; в лаунчере с правами появится то же
предупреждение, и **Kill Process** снимет ядро. Сетевая очистка (призрачные
адаптеры, NLA-профили, осиротевшие правила брандмауэра) тоже идёт только в лаунчере
с правами: без прав она пропускается одной строкой INFO в логе.

### Данные в `%LOCALAPPDATA%`, хотя рядом с программой лежит `portable.txt`

Папка программы лежит под `Program Files` (или `Windows`), куда пишет только
администратор, поэтому маркер не учитывается — **Settings → Storage → Mode**
показывает «portable.txt не учитывается». Данные старой версии, лежавшие рядом с
программой, при первом запуске скопированы в `%LOCALAPPDATA%\singbox-launcher`;
старая копия осталась на месте. Чтобы данные жили рядом с программой, распакуйте zip в
папку, куда может писать ваша учётная запись. **Remove all data…** без прав
пропускает старую копию в `Program Files` («Нужны права администратора»), и раз
копия осталась, при следующем запуске миграция снова скопирует данные в
`%LOCALAPPDATA%`. Удалить её можно очисткой от имени администратора —
`"<exe>" -purge-data -yes` из командной строки администратора.

## Удалённые машины

### Save и Deploy прошли без ошибок, а на машине работают старые правила

Только версия 2.0.0. Конфигуратор удалённой машины сохраняет состояние, показывает
«Remote config exported», Deploy проходит — но новые правила на машине не действуют.

Причина: перед записью конфиг проверяет локальное ядро, а пути к наборам правил (`.srs`)
в конфиге удалённой машины ведут в её файловую систему. Проверка падала на «no such file»,
и вместо нового конфига на диске молча оставалась прошлая сборка — её Deploy и отправлял.

Как убедиться: дата файла `bin/wizard_states/remote/<id>/config.json` старше последнего
Save, а в логе лаунчера рядом с Save стоит строка
`corereject: the core error does not name a node of ours`.

Исправлено в версиях после 2.0.0: на время проверки пути подменяются на локальные копии
наборов, а отказ ядра показывается ошибкой, а не теряется. Обход для 2.0.0: переименовать
старый `config.json` машины и нажать Save ещё раз — без прежнего файла конфиг записывается.
Делать это придётся перед каждым Save.

## Демон

Демон (`sing-box lxd`) — системная служба, в которой работает ядро: на этом компьютере в
режиме Daemon и на каждой удалённой машине (роутер, VPS). Когда с ним что-то не так,
откройте окно **Service**: ⚙ в строке машины на вкладке Remote или Servers → ⚙ → Local
для этого компьютера. Окно открывается на вкладке, где есть что чинить (`✖ Not running`,
`⚠ Core`, `✖ Pairing`; пути — на Reference), и подставляет в команды ниже адрес и пути
этой машины: ⧉ копирует команду как есть, ▶ открывает с ней Терминал — для удалённой
машины в обёртке `ssh <цель> '…'`. ssh-цель задаётся в окне Edit машины (по умолчанию
`root@<хост демона>`). Шапка окна сама зеленеет через несколько секунд после того, как
демон снова ответил.

В этом разделе — те же рецепты на случай, когда лаунчера под рукой нет.

> **Это пути по умолчанию — у вас они могут отличаться.** Их лаунчер предполагает, пока
> демон не сообщил свои. Например, при ручной установке на OpenWrt бинарь часто лежит в
> `/root/sing-box` (гайд форка
> [§8.3](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.ru.md#83-openwrt--procd-роутеры)).
> Вкладка **Reference** окна Service показывает пути, которые сообщил демон
> (`/admin/info`), и помечает каждый предположенный как `default` — верьте им, а не этой
> таблице.

Служба везде называется `sing-box-lxd` (метка launchd на macOS —
`com.leadaxe.sing-box-lxd`); управляющий канал слушает порт `19091`, если `daemon.json` не
говорит иное. На Linux командам нужен root — под не-root ssh-пользователем добавьте
`sudo` (окно Service делает это само). На Windows выполняйте их в PowerShell **от имени
администратора**.

### Где что лежит

| | OpenWrt (procd) | Linux (systemd) | macOS (launchd) | Windows (SCM) |
|---|---|---|---|---|
| Бинарь ядра | `/usr/bin/sing-box` | `/usr/local/bin/sing-box` | `/Library/PrivilegedHelperTools/sing-box-lxd` | `C:\Program Files\sing-box-lxd\sing-box-lxd.exe` |
| Служба | `/etc/init.d/sing-box-lxd` | `/etc/systemd/system/sing-box-lxd.service` | `/Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist` | служба SCM `sing-box-lxd` |
| State dir | `/etc/sing-box-lxd/state` | `/var/lib/sing-box-lxd/state` | `/Library/Application Support/sing-box-lxd/state` | `C:\ProgramData\sing-box-lxd\state` |
| Лог | `/tmp/lxd.log` | `/var/lib/sing-box-lxd/lxd.log` | `/Library/Application Support/sing-box-lxd/lxd.log` | `C:\ProgramData\sing-box-lxd\logs\lxd.log` |

В state dir: `daemon.json` (настройки), `last_good.json` (последний конфиг, который
стартовал), `clients.json` (сопряжённые клиенты). На macOS и Windows бинарь ядра —
защищённая копия ядра лаунчера, рядом с ней запись установки
(`sing-box-lxd.install.json`); см.
[DAEMON_AND_REMOTE.ru.md §2.1](DAEMON_AND_REMOTE.ru.md#21-служба-запускает-root-owned-копию-ядра-spec-136).

Ключи `daemon.json` (гайд форка
[§3](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.ru.md#3-daemonjson--настройки-демона)):
`listen` (адрес канала, `"host:port"` или `{"address": [...], "port": N}`), `tls` (mTLS
вкл/выкл), `secret` (Bearer-секрет, единственная защита при `tls: false`), `log_file`,
`log_max_size_mb` / `log_max_backups` / `log_max_age_hours` (по умолчанию 1 / 1 / 24).
**Настройка меняется правкой файла и перезапуском службы — никогда не переустановкой.**
Посмотреть файл: `cat <state dir>/daemon.json` (macOS — `sudo cat`, Windows —
`Get-Content`).

### Демон не поднимается

Признаки: в строке машины `✖ …` и красная точка на ⚙; локальная шапка говорит, что демон
не отвечает; `connection refused`, `i/o timeout`, `no route to host`. Идите от дешёвого к
дорогому и останавливайтесь, как только демон снова ответил.

| Шаг | OpenWrt (procd) | Linux (systemd) | macOS | Windows |
|---|---|---|---|---|
| 1. Работает ли? | `/etc/init.d/sing-box-lxd status` | `systemctl status sing-box-lxd --no-pager` | `launchctl print system/com.leadaxe.sing-box-lxd` | `sc.exe query sing-box-lxd` |
| 2. Перезапустить | `/etc/init.d/sing-box-lxd restart` | `systemctl restart sing-box-lxd` | `sudo launchctl kickstart -k system/com.leadaxe.sing-box-lxd` | `Restart-Service -Name sing-box-lxd -Force` |
| 3. Прочитать причину | `tail -n 100 /tmp/lxd.log` | `tail -n 100 /var/lib/sing-box-lxd/lxd.log` | `sudo tail -n 100 '/Library/Application Support/sing-box-lxd/lxd.log'` | `Get-Content -Tail 100 'C:\ProgramData\sing-box-lxd\logs\lxd.log'` |
| Следить за логом | `tail -f /tmp/lxd.log` | `tail -f /var/lib/sing-box-lxd/lxd.log` | `sudo tail -f '…/lxd.log'` | `Get-Content -Wait -Tail 50 '…\lxd.log'` |
| Кто держит порт | `netstat -lnp \| grep :19091` | `ss -ltnp \| grep :19091` | `sudo lsof -nP -i :19091` | `netstat -ano \| findstr :19091` |

macOS: если `launchctl print` не находит службу — она установлена, но не загружена;
загрузите её, а не переустанавливайте:
`sudo launchctl bootstrap system /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist`.

Что говорит лог:

- `lxd: refusing to run as …` — служба запускается из незащищённого бинаря (macOS,
  Windows). Переустановите её из защищённой копии: шаг 5.
- `bind: address already in use` — порт занят кем-то другим; см. «Кто держит порт» или
  смените `listen` в `daemon.json`.
- Ошибка конфига сразу после старта ядра — шаг 4.

**4. Сломал последний конфиг?** Один раз загрузите последний рабочий на переднем плане —
лог останется на экране:

```sh
# OpenWrt
/etc/init.d/sing-box-lxd stop && /usr/bin/sing-box lxd --state-dir /etc/sing-box-lxd/state --config-force /etc/sing-box-lxd/state/last_good.json
# systemd
systemctl stop sing-box-lxd && /usr/local/bin/sing-box lxd --state-dir /var/lib/sing-box-lxd/state --config-force /var/lib/sing-box-lxd/state/last_good.json
# macOS
sudo launchctl bootout system/com.leadaxe.sing-box-lxd ; sudo /Library/PrivilegedHelperTools/sing-box-lxd lxd --state-dir '/Library/Application Support/sing-box-lxd/state' --config-force '/Library/Application Support/sing-box-lxd/state/last_good.json'
```

```powershell
# Windows (PowerShell от имени администратора)
sc.exe stop sing-box-lxd; & 'C:\Program Files\sing-box-lxd\sing-box-lxd.exe' lxd --state-dir 'C:\ProgramData\sing-box-lxd\state' --config-force 'C:\ProgramData\sing-box-lxd\state\last_good.json'
```

Затем Ctrl-C и снова запустите службу (`/etc/init.d/sing-box-lxd start`,
`systemctl start sing-box-lxd`, команда `bootstrap` для macOS выше, `sc.exe start
sing-box-lxd`).

**5. Ничего не помогает** — переустановите ядро: [Обновить ядро на
машине](#обновить-ядро-на-машине) ниже (подойдёт и та же версия). Локально на macOS и
Windows: окно Service → Core → **Install or update service**.

### Обновить ядро на машине

Когда: шапка Service показывает `Core 1.14.2-lx.11 (required 1.14.3-lx.14 ⚠)`, на ⚙
машины жёлтая точка или Deploy предупреждает, что на машине ядро старее. Такое ядро может
отвергнуть конфиг, собранный под новое, — тогда оно откатывается на последний рабочий
конфиг, и новые правила просто не действуют. Deploy только предупреждает, но не
запрещает.

**Этот компьютер (macOS, Windows).** Скачайте ядро на вкладке Core дашборда, затем
выполните **Install or update service** (окно Service → Core): команда обновляет
защищённую копию и перезапускает службу, сохраняя `daemon.json` и сопряжённых клиентов.

**Удалённая машина на macOS или Windows.** Выполните `lxd --service=install` новым ядром
на самой машине (гайд форка
[§7](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.ru.md#7-macos--автоматическая-установка) /
[§7a](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.ru.md#7a-windows--автоматическая-установка)).

**Машина на Linux (OpenWrt, systemd).** Рецепт ниже проверен на роутере с OpenWrt. Пример —
OpenWrt на `root@192.168.10.1`, `linux/arm64`, с `1.14.2-lx.11` на `1.14.3-lx.14`; для
systemd — заметки после него.

1. **Скачайте ядро под платформу машины — на своём компьютере, а не на машине** (роутеру
   может не хватить места на распаковку). Окно Service → Core → **Download** скачивает
   сборку под платформу машины в `~/Downloads/sing-box-1.14.3-lx.14-linux-arm64` и сверяет
   её с `SHA256SUMS` релиза. Вручную: скачайте `sing-box-1.14.3-lx.14-linux-arm64.tar.gz` и
   `SHA256SUMS` из [релиза ядра](https://github.com/Leadaxe/sing-box-lx/releases), сравните
   `shasum -a 256` архива с его строкой в `SHA256SUMS`, распакуйте и возьмите бинарь
   `sing-box`. Имена ассетов: `linux-arm64`, `linux-armv7`, `linux-amd64`,
   `linux-mips-softfloat`, `linux-mipsle-softfloat`.
2. **Залейте потоком через ssh** — `scp` на OpenWrt не работает (нет sftp-server):

   ```sh
   ssh root@192.168.10.1 'cat > /tmp/sing-box.new' < ~/Downloads/sing-box-1.14.3-lx.14-linux-arm64
   ```

   Нестандартный порт ssh: `ssh -p 2222 root@…`. `/tmp` — в памяти; `df -h /tmp` покажет,
   поместится ли бинарь.
3. **Проверьте до замены** — новым бинарём на текущем конфиге:

   ```sh
   chmod +x /tmp/sing-box.new && sha256sum /tmp/sing-box.new && /tmp/sing-box.new version && /tmp/sing-box.new check -c /etc/sing-box-lxd/state/last_good.json
   ```

   Ожидается: sha256 из шага 1, новая версия с `with_lxd` среди тегов и пустой вывод
   `check`. Что-то другое — остановитесь; работающая служба не тронута
   (`rm /tmp/sing-box.new`).
4. **Бэкап, замена, перезапуск:**

   ```sh
   cp /usr/bin/sing-box /root/sing-box.1.14.2-lx.11.bak && /etc/init.d/sing-box-lxd stop && mv /tmp/sing-box.new /usr/bin/sing-box && chmod 755 /usr/bin/sing-box && /etc/init.d/sing-box-lxd start
   ```

   Бэкап кладётся в `/root`, потому что `/tmp` не переживает перезагрузку. Флеш у роутера
   маленький: удалите бэкап, когда новое ядро себя покажет.
5. **Проверка:** через несколько секунд шапка Service показывает
   `1.14.3-lx.14 · started`; на машине — `/etc/init.d/sing-box-lxd status`.
6. **Откат**, если новое ядро ведёт себя плохо:

   ```sh
   cp /root/sing-box.1.14.2-lx.11.bak /usr/bin/sing-box && /etc/init.d/sing-box-lxd restart
   ```

systemd: бинарь — `/usr/local/bin/sing-box`, команды службы —
`systemctl stop|start|restart sing-box-lxd`, бэкап лежит рядом с бинарём
(`/usr/local/bin/sing-box.1.14.2-lx.11.bak`), проверка — на
`/var/lib/sing-box-lxd/state/last_good.json`. Шаг `version` на роутерах важен: бинарь не
той архитектуры или glibc-сборка на musl-роутере не запустится вовсе.

**Службы ещё нет** (новая машина): init-скрипт / unit с путями машины — в окне Service →
Core → *Install from scratch* и в гайде форка
([§8.2 systemd](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.ru.md#82-systemd-обычный-сервердесктоп),
[§8.3 OpenWrt](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.ru.md#83-openwrt--procd-роутеры)).
Включение: `chmod +x /etc/init.d/sing-box-lxd && /etc/init.d/sing-box-lxd enable &&
/etc/init.d/sing-box-lxd start` или `systemctl daemon-reload && systemctl enable --now
sing-box-lxd`. На OpenWrt добавьте бинарь, init-скрипт и state dir в
`/etc/sysupgrade.conf`, иначе обновление прошивки их сотрёт. VPN-Wi-Fi поверх демона —
[openwrt-vpn-ssid](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/openwrt-vpn-ssid.ru.md).

**Удаление службы** state не трогает: `/etc/init.d/sing-box-lxd disable &&
/etc/init.d/sing-box-lxd stop && rm /etc/init.d/sing-box-lxd` или `systemctl disable --now
sing-box-lxd && rm /etc/systemd/system/sing-box-lxd.service && systemctl daemon-reload`.
`rm -r <state dir>` удаляет ещё и сопряжённых клиентов, ключи и last-good — каждому
лаунчеру придётся сопрягаться заново.

### Сопряжение

Пере-сопрягайтесь, когда лаунчер сообщает `certificate changed` (сертификат демона больше
не совпадает с закреплённым отпечатком), `not paired`, `bad certificate` или `403`, после
переустановки службы со стёртым state dir или когда вкладка Pairing окна ⚙ помечена `✖`.
Гайд форка:
[§9](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.ru.md#9-сопряжение-клиента-одинаково-на-всех-ос).

1. **Выпустите приглашение на самой машине** — операторские команды отвечают только на
   loopback:

   ```sh
   # OpenWrt (systemd: /usr/local/bin/sing-box и /var/lib/sing-box-lxd/state)
   /usr/bin/sing-box lxd client add --name singbox-launcher --state-dir /etc/sing-box-lxd/state
   # macOS (state dir находится сам)
   sudo /Library/PrivilegedHelperTools/sing-box-lxd lxd client add --name singbox-launcher
   ```

   На Windows: `& 'C:\Program Files\sing-box-lxd\sing-box-lxd.exe' lxd client add --name
   singbox-launcher` в PowerShell от имени администратора. Команда печатает
   `адрес#отпечаток#код`.
2. **Вставьте его в лаунчер**: окно Service → Pairing → **Pair** (для удалённой машины
   также Edit → Re-pair). Если адрес в приглашении — loopback, замените его на адрес,
   доступный с этого компьютера (`192.168.10.1:19091`), отпечаток и код оставьте.

Ловушки, все пойманы на живом роутере:

- Код живёт в памяти демона: перезапуск демона между `client add` и Pair его убивает
  (`enroll: no active enrollment code`) — выпустите новый.
- Если `listen` — один LAN-адрес, loopback не слушается, и `client add` до демона не
  достучится. Используйте объектную форму с обоими адресами:
  `{"address": ["192.168.10.1", "127.0.0.1"], "port": 19091}`, затем перезапуск.
- Кому доверяет демон: `… lxd client list --state-dir …`; отзыв: `… lxd client remove
  <имя-или-отпечаток> --state-dir …`. Удаление машины из списка лаунчера доступ **не**
  отзывает — только `client remove` на самой машине.
- Plain-режим (`"tls": false`, только loopback/dev): единственный мандат — Bearer
  `secret` из `daemon.json`; его вводят на вкладке Pairing.

Подробнее об окне Service и реестре машин —
[DAEMON_AND_REMOTE.ru.md](DAEMON_AND_REMOTE.ru.md).
