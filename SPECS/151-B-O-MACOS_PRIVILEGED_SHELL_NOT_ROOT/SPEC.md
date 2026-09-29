# SPEC 151 — macOS: шелл привилегированного старта работает без root

Статус: **O** (правка сделана 2026-09-29, причина измерена на macOS 26.7; старт classic TUN с правкой не прогонялся).
Тип: Bug. Issue [#138](https://github.com/Leadaxe/singbox-launcher/issues/138).
Связь: SPEC 137 §3, 137.1.

## 1. Проблема

Classic TUN без службы на macOS не стартует с v2.1.0 (SPEC 137), на любой
версии системы. В issue — macOS 27.0, лаунчер v2.3.3. Старт даёт
`privileged start failed: refused: cannot open /Library/Logs/sing-box-lxd/classic.log`.
Если создать `classic.log` руками (владелец — пользователь, `0600`), ядро
стартует и падает: `configure tun interface: Connect: operation not permitted`.

Оба симптома воспроизводятся телом старта (SPEC 137 §3), если его шелл
работает под uid пользователя, а каталог `/Library/Logs/sing-box-lxd`
(`root:wheel 0755`) уже есть:

| Шаг тела | Под uid пользователя |
|---|---|
| `mkdir -p`, проверка владельца каталога | проходят: каталог есть, владелец root |
| `chmod 0755` каталога | проходит: режим не меняется, `chmod(1)` вызов не делает |
| `: >>classic.log` | отказ записи в каталог root → `cannot open` |
| то же при готовом файле пользователя | `chmod`/`chown` на себя проходят, лог открыт |
| старт ядра | ядро без root, utun не создаётся → `operation not permitted` |

## 2. Причина

AEWP запускает инструмент с euid 0 и real uid пользователя. bash при
euid ≠ uid сбрасывает euid к uid, если не задан `-p`. AEWP кладёт в
окружение инструмента `_BASH_IMPLICIT_DASH_PEE=-p`, и bash от Apple читает
её как неявный `-p`. SPEC 137 поставила перед шеллом `env -i`: переменная
стирается вместе с окружением лаунчера, и шелл теряет root.

Измерено на macOS 26.7 (bash 3.2.57) программой, которая зовёт AEWP и
печатает uid:

| Вызов AEWP | euid | real uid |
|---|---|---|
| `/usr/bin/id` без шелла | 0 | 501 |
| `/bin/sh -c` (v2.0.2) | 0 | 501 |
| `env -i PATH=… /bin/sh -c` (v2.1.0–v2.3.3) | 501 | 501 |
| `env -i PATH=… /bin/bash -c` | 501 | 501 |
| `env -u _BASH_IMPLICIT_DASH_PEE /bin/sh -c` | 501 | 501 |
| `env -i PATH=… _BASH_IMPLICIT_DASH_PEE=-p /bin/sh -c` | 0 | 501 |
| `env -i PATH=… /bin/bash -p -c` (правка) | 0 | 501 |

## 3. Поведение после правки

1. Шелл старта — `/bin/bash -p` вместо `/bin/sh`. `-p` отключает сброс
   euid и импорт функций из окружения; окружение по-прежнему режет
   `env -i`. bash назван по имени: `/bin/sh` может оказаться dash, а тот
   флага `-p` не знает.
2. Тело первым делом сверяет свой euid с ожидаемым (аргумент `$8`, в проде
   `0`). Несовпадение — отказ до любых действий с логом:
   `refused: the start shell runs as uid <euid> (real uid <uid>), not 0`.
3. Отказ открыть `classic.log` несёт причину от системы:
   `refused: cannot open <файл>: Permission denied`.

Вызов AEWP: `/usr/bin/env -i PATH=… /bin/bash -p -c '<тело>'
start-singbox-privileged <Data>/bin <копия> config.json
/Library/Logs/sing-box-lxd 0 <uid лаунчера> 2097152 0`.

## 4. Приёмка

- Тест тела без root: прод-argv даёт отказ п. 2 и не трогает каталог лога.
- Старт classic TUN сборкой с правкой: ядро под root, TUN поднят,
  `classic.log` создан телом.
