# SPECS — спецификации и технические задания (Spec Kit)

Все задачи (фичи, баги, исследования) в одной папке. Имя папки задаёт **номер**, **тип**, **статус** и **название**.

## Имя папки: `NNN-T-S-NAME`

| Часть | Значение | Расшифровка |
|-------|----------|-------------|
| **NNN** | 001, 002, … | Сквозной номер |
| **T** (тип) | F | Feature — фича |
| | B | Bug — баг |
| | Q | Question — исследование |
| **S** (статус) | N | New — новый / в плане |
| | W | Wait — ожидание |
| | O | Open — в работе / reopen |
| | C | Complete — сделано |
| **NAME** | UPPER_SNAKE или kebab-case | Название задачи |

Примеры: `001-F-C-FEATURES_2025`, `011-B-C-launcher-freeze-after-sleep`, `013-F-C-LOCALIZATION`.

## Формат Spec Kit (внутри папки)

| Файл | Назначение |
|------|------------|
| **SPEC.md** | Что и зачем — проблема, требования, критерии приёмки |
| **PLAN.md** | Как строить — архитектура, изменения в файлах |
| **TASKS.md** | Чеклист задач по этапам |
| **IMPLEMENTATION_REPORT.md** | Отчёт после реализации — статус, изменения, дата |

## Корневой уровень SPECS

| Файл | Назначение |
|------|------------|
| **CONSTITUTION.md** | Неизменяемые принципы проекта — приоритеты, архитектура, запреты |
| **IMPLEMENTATION_PROMPT.md** | Промпт для реализации — философия разработки, DoD, ограничения (Git, консоль) |

## Workflow

1. Создать папку `SPECS/NNN-T-S-NAME/` (номер следующий по списку, тип F/B/Q, статус N для новой задачи).
2. Написать SPEC.md.
3. Написать PLAN.md, разбить на TASKS.md.
4. Реализовать по TASKS с учётом [IMPLEMENTATION_PROMPT.md](IMPLEMENTATION_PROMPT.md), заполнить IMPLEMENTATION_REPORT.md.
5. Пройти [чеклист закрытия задачи](#closing-task-checklist) (тесты, доки, статус папки).
6. При завершении переименовать папку: заменить статус на **C** (Complete).

<a id="closing-task-checklist"></a>

## Закрытие задачи (чеклист)

Ритуал после реализации по **TASKS.md** — чтобы spec-driven давал предсказуемый результат и не терялись доки/релиз.

1. **Сборка и проверки:** `go build ./...`, `go test ./...`, `go vet ./...` (локально; в CI — скрипты в `build/`, см. [docs/TEST_README.md](../docs/TEST_README.md) и корневой README).
2. **Границы задачи:** менять только то, что покрыто **SPEC** / **PLAN** / **TASKS**; новые файлы — как в плане. Расширение scope без явного согласования — см. корневой [AGENTS.md](../AGENTS.md).
3. **Артефакты задачи:** все пункты **TASKS.md** отражают факт (в т.ч. `[x]`); заполнен **IMPLEMENTATION_REPORT.md** (или эквивалент в папке задачи).
4. **Документация для релиза и архитектуры** (подробная таблица — [AGENTS.md § 4–6](../AGENTS.md)):
   - поведение / заметные изменения → **docs/release_notes/upcoming.md** (EN и при необходимости RU);
   - потоки данных, зоны ответственности → **docs/ARCHITECTURE.md** (если менялись);
   - сильный UX → **RELEASE_NOTES.md**.
5. **Принципы проекта:** [CONSTITUTION.md](CONSTITUTION.md) (логирование, запреты, UI), [IMPLEMENTATION_PROMPT.md](IMPLEMENTATION_PROMPT.md) (DoD, Git, консоль).
6. **Статус в имени папки:** переименовать `SPECS/NNN-T-S-NAME/` → `…-C-…` когда задача принята как Complete.

Краткий чеклист для агентов дублируется в [AGENTS.md § 6](../AGENTS.md) (с отсылкой сюда).

## Текущий список (кратко)

- **001–010** — завершённые фичи (F-C): FEATURES_2025, WIZARD_STATE, UNIFIED_CONFIG_TEMPLATE, SRS_LOCAL_DOWNLOAD, DOWNLOAD_FAILED_MANUAL, PING_ERROR_TOOLTIP, DIAGNOSTICS_LOG_VIEWER, OUTBOUNDS_CONFIGURATOR, WIREGUARD_URI, OUTBOUND_EDIT_PREVIEW_TAB
- **011–012** — баги (B): launcher-freeze-after-sleep (C), update-reload-clash-config (O)
- **013** — фича завершена (F-C): LOCALIZATION
- **016** — закрыта без реализации (F-C): SUBSCRIPTION_JSON_ARRAY (JSON-массив sing-box подписок; код не внедрялся; референс для **033** и будущей реализации; **SPECS/016-F-C-SUBSCRIPTION_JSON_ARRAY/**)
- **014** — фича закрыта без отдельной реализации (F-C): RULE_TYPE_SRS_URL (содержание перенесено в 018)
- **017** — фича завершена (F-C): RULE_TYPE_PROCESS_PATH_REGEX (Match by path)
- **018** — фича в плане (F-N): CUSTOM_RULE_SUBSYSTEM_REFACTOR (объединяющая: константы типов ips/urls/processes/raw, вкладка Raw, params в custom_rules, документация по state в docs/)
- **015** — исследование закрыто (Q-C): TELEMETRY
- **019** — фича завершена (F-C): WIN7_ADAPTATION
- **020** — фича завершена (F-C): CUSTOM_SRS_LOCAL_DOWNLOAD
- **021** — фича завершена (F-C): SOCKS5_URI (парсинг socks5:// и socks:// в Source/Connections)
- **022** — поглощена SPEC 135 (F-C): MACOS_APP_SUPPORT_DIRECTORIES (данные в `~/Library` при запуске из `.app`; реализовано в **SPECS/135-F-N-DATA_DIR_LAYOUT/SPEC.md** — модель AppDir/DataDir/LogDir вместо изменяемого Bundle ID; папка **SPECS/022-F-C-MACOS_APP_SUPPORT_DIRECTORIES/** оставлена как история постановки)
- **023** — фича завершена (F-C): SUBSCRIPTION_TRANSPORT_VLESS_TROJAN (transport/TLS для VLESS и Trojan из подписки по схеме sing-box, VMess gRPC `service_name`, MakeTagUnique в превью визарда)
- **024** — фича завершена (F-C): WIZARD_DNS_SECTION (вкладка DNS в визарде; см. **SPEC.md**)
- **025** — фича завершена (F-C): SERVERS_CONTEXT_MENU_SHARE_URI (ПКМ на вкладке Servers, share URI из config.json outbounds/endpoints; см. **IMPLEMENTATION_REPORT.md**)
- **026** — закрыта (F-C): WIZARD_SOURCE_EDIT_LOCAL_OUTBOUNDS (вкладка Sources: **Edit** — Настройки/Просмотр; локальные auto/select с маркерами **WIZARD:**; `exclude_from_global` / `expose_group_tags_to_global`; см. **SPEC.md**)
- **027** — завершена (F-C): WIZARD_RULES_LIBRARY (единый **`custom_rules`**, библиотека пресетов **Add from library**, миграция v2→v3; **`selectable_rules`** в шаблоне — пресеты; см. **SPECS/027-F-C-WIZARD_RULES_LIBRARY/SPEC.md**, **docs/WIZARD_STATE.md**)
- **028** — завершена (F-C): WIZARD_LIST_ROW_HOVER (подсветка строк списка при наведении: **Rules**, **Sources**, **Outbounds** (конфигуратор), **DNS**, модал библиотеки; **HoverRow** + **WireTooltipLabelHover** + **HoverForward*** / **HoverForwardTTButton** для SRS; **SPECS/028-F-C-WIZARD_LIST_ROW_HOVER/SPEC.md**)
- **029** — исследование (Q-С): SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY (доработки парсера подписок под **sing-box**, реализованы; папка исторически от сравнения с [clash-convertor](https://github.com/DikozImpact/clash-convertor); **SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY/SPEC.md**)
- **030** — баг в плане (B-N): WINDOWS_FOREGROUND_FOCUS_LOSS (Windows: периодический слёт фокуса ввода в других приложениях при работающем лаунчере; поиск причины и корреляция с UI/треем; **SPECS/030-B-N-WINDOWS_FOREGROUND_FOCUS_LOSS/SPEC.md**)
- **031** — фича завершена (F-С): LINUX_SINGBOX_LOOKPATH (Linux: сначала `exec.LookPath("sing-box")`, иначе `<ExecDir>/bin/sing-box`; установка ядра из лаунчера — только в локальный `bin/`; **SPECS/031-F-С-LINUX_SINGBOX_LOOKPATH/SPEC.md**)
- **032** — фича завершена (F-C): WIZARD_SETTINGS_TAB (вкладка **Settings**, **`vars`** в шаблоне и state, TUN macOS с **Rules** на **Settings**; **SPECS/032-F-C-WIZARD_SETTINGS_TAB/**)
- **033** — фича завершена (F-N): SUBSCRIPTION_XRAY_JSON_ARRAY (подписка как JSON-массив **Xray/V2Ray** полных конфигов, `remarks` → Label и slug-теги, `dialerProxy`/`dialer` + SOCKS → sing-box **`detour`**; **SPECS/033-F-N-SUBSCRIPTION_XRAY_JSON_ARRAY/**, **IMPLEMENTATION_REPORT.md**)
- **034** — фича завершена (F-C): HTTP_ENV_PROXY (исходящие HTTP(S) через `HTTP_PROXY`/`HTTPS_PROXY`; единый клиент; маскировка паролей в ошибках; UI — `GetURLBytes`; **SPECS/034-F-C-HTTP_ENV_PROXY/SPEC.md**)
- **035** — исследование завершено (Q-C): VLESS_SINGBOX_FLOW_FIELD (поле `flow` у VLESS outbound: исходники sing-box/sing-vmess, отсутствие требования явного ключа в JSON, откат эксперимента с `flow: ""`; **SPECS/035-Q-C-VLESS_SINGBOX_FLOW_FIELD/SPEC.md**, **IMPLEMENTATION_REPORT.md**)
- **036** — фича завершена (F-C): XRAY_JUMP_ANY_PROTOCOL (Xray JSON-массив: hop **`socks`** или **`vless`** по `dialerProxy`; follow-up к **033**; **SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/**)
- **037–072** — см. папки задач (список здесь не велся; статус — в имени папки `NNN-T-S-NAME`)
- **073** — фича (F-N, ядро сделано): AMNEZIAWG_PARAMS (AWG 2.0 параметры на WireGuard-endpoint; сабтаска 073.1 — robustness-фиксы парсинга, v1.1.2; сабтаска 073.2 — диапазоны `H1–H4` с пробросом в ядро ≥ lx.6, v1.1.4; открыта только опциональная UI-фаза; **SPECS/073-F-N-AMNEZIAWG_PARAMS/**)
- **074** — фича завершена (F-C): TUIC_PROTOCOL (TUIC v5, v1.1.2; **SPECS/074-F-C-TUIC_PROTOCOL/**)
- **075** — фича завершена (F-C): AMNEZIA_VPN_IMPORT (импорт `vpn://`-профилей Amnezia — base64url + qCompress + JSON → WG/AWG-узел, v1.1.3; **SPECS/075-F-C-AMNEZIA_VPN_IMPORT/**)
- **076** — фича завершена (F-C): WGCONF_PASTE_IMPORT (вставка голого `[Interface]/[Peer]`-текста в поле Add, v1.1.3; **SPECS/076-F-C-WGCONF_PASTE_IMPORT/**)
- **080** — поглощена SPEC 135 (F-C): XDG_PORTABLE_DATA_DIR (XDG-пути для writable-данных на Linux, portable-режим, миграция без потери данных — issue #85; реализовано в **SPECS/135-F-N-DATA_DIR_LAYOUT/SPEC.md**; папка **SPECS/080-F-C-XDG_PORTABLE_DATA_DIR/** оставлена как история постановки)
- **083** — баг диагностика (B-W): RUTRACKER_SSL_RECORD_TOO_LONG — `SSL_ERROR_RX_RECORD_TOO_LONG` = не дефект туннеля/кода: `rutracker.org` в geosite-ru-blocked → direct → DPI-заглушка РКН вместо TLS. Amnezia открывает (full-tunnel). Диагностика+рекомендации, решение за владельцем.
- **084** — фича (F-O, backend): WARP_GENERATOR — `core/warp` регистрация Cloudflare API (X25519 on-device), AWG-обфускация, endpoint-пул; +парсер `reserved`/`ip/id/ib`. e2e `sing-box check` OK. UI-визард — 084.1.
- **085** — фича (F-O, backend): FAKEIP — пресет `fakeip` + `PresetDNSServer.Inet4/6Range` + `store_fakeip`. HTTPS/SVCB-блок — **085.1** (`dns_rules`-plural, нужен live-GUI).
- **086** — фича (F-N, блокирован пином): MASQUE_WARP_INTEGRATION — MASQUE есть в ядре (SPEC 021, lx.2+), лаунчер на rc.17. Контракт зафиксирован; предусловие — бамп пина rc.17→lx.3.
- **087** — закрыт SPEC 104 (DIRECTION_MODEL): задел «Channels как отдельное top-level state-поле» отвергнут — он дублировал существующий `OutboundConfig`. Вместо этого `OutboundConfig` переименован в `Direction` и дополнен именем, выключением и парной auto-группой; **SPECS/104-F-N-LAUNCHER_CHANNELS_MODEL/**
- **088** — фича (F-O): LOADBALANCING — generator round_robin/balancer + sentinel-guard + детерминированный emit (реализовано); UI-контролы — 088.1.
- **089** — фича (F-N, spec-only): DEEP_RULES — структурный редактор выражений (logical and/or/invert), `editor_mode` в Params, декомпозиция add_rule_dialog. L, UI-heavy.
- **093** — баг завершён (B-C): UTLS_FINGERPRINT_ALLOWLIST — `fp=HelloChrome_120` из подписки уезжал в конфиг как есть → `unknown uTLS fingerprint` и отказ загрузки **всего** конфига (одна нода = VPN не стартует). Валидация по allowlist ядра + маппинг сырых uTLS-идентификаторов, барьер в парсере и эмиттере; v1.2.7, issue [#100](https://github.com/Leadaxe/singbox-launcher/issues/100); **SPECS/093-B-C-UTLS_FINGERPRINT_ALLOWLIST/**
- **090** — фича (F-N, spec-only): PRESET_LANGUAGE — `#if` уже общий с LxBox; конвертер-импорт (bundled 8 уже нативны) отложен; документ shared-формата.
- **094** — фича завершена (F-C): SUBSCRIPTION_PARSER_PARITY — паритет парсера подписок с LxBox: тело sing-box JSON (одиночный outbound / массив / целый конфиг), `outbounds` + `endpoints` одним списком, разрешение `detour`-цепочек до 8 хопов с детекцией циклов, импортированные `selector`/`urltest` как узлы схемы `group`, дедупликация внутри источника по `NodeIdentityHash` **до** назначения тегов; **SPECS/094-F-C-SUBSCRIPTION_PARSER_PARITY/**
- **095** — фича завершена (F-C): NODE_SUBTITLE_AND_INFO — подзаголовок узла (`vless·tcp·Reality+Vision`), значок режима группы и экран информации; данные берутся обратным чтением `config.json` (Clash API их не отдаёт), а вкладка Preview источника — напрямую из `ParsedNode`, пока конфига ещё нет; **SPECS/095-F-C-NODE_SUBTITLE_AND_INFO/**
- **096** — фича завершена (F-C): DAEMON_CORE_ENGINE — второй движок ядра на macOS: ядро внутри системной службы `sing-box lxd` (gRPC + admin REST) вместо спавна `sing-box run`; шов `CoreBackend`, транспорт прокси-операций `ProxyTransport`, mTLS-сопряжение одноразовым приглашением, привилегированные операции — только готовыми sudo-командами в терминале пользователя; classic остаётся дефолтом; **SPECS/096-F-C-DAEMON_CORE_ENGINE/**
- **097** — фича завершена (F-C): REMOTE_CONFIG_TARGET — таргет генерации (`local` | `remote`) как шаг 0 визарда, независимая ось роли (`gateway_mode`), платформа целевой машины подменяет `runtime.GOOS` во всей генерации, реестр машин и доставка конфига по каналу lxd; **SPECS/097-F-C-REMOTE_CONFIG_TARGET/**
- **098** — фича завершена (F-C): LOCAL_REMOTE_TABS — вкладки **Local** и **Remote** вместо Core/Servers (обе двухколоночные: слева список прокси, справа управление); **конфиг на каждую машину** — своя директория `bin/wizard_states/remote/<id>/` с состоянием, снапшотами, `config.json`, `srs/` и `subscriptions/`; GC в границах машины; платформа машины переехала в реестр; миграция singleton-профиля; окно 1000×700; **SPECS/098-F-C-LOCAL_REMOTE_TABS/**
- **099** — фича завершена (F-C): REMOTE_TRAFFIC_PROFILER — профайлер трафика на каждую машину (свой экземпляр, окно и буфер; локальный синглтон не трогается), источники только gRPC, разбивка по клиентам сети вместо процессов, плюс окно телеметрии хоста машины; **SPECS/099-F-C-REMOTE_TRAFFIC_PROFILER/**
- **135** — фича реализована в ветке, ждёт CI и приёмки владельцем (F-N): DATA_DIR_LAYOUT — три роли путей AppDir/DataDir/LogDir, XDG на Linux, `~/Library` на macOS, `%LOCALAPPDATA%` на Windows, portable по маркеру и унаследованной раскладке с переключателем в Settings, миграция копированием, раздел Storage с путями (`-paths`, `/debug/paths`), удаление с очисткой (`-purge-data`); закрывает #85, поглощает SPEC 022 и 080; **SPECS/135-F-N-DATA_DIR_LAYOUT/SPEC.md** (отступления от документа при реализации — его §11)
- **136** — фича реализована в ветке, ядро запинено на lx.12, ждёт приёмки владельцем (F-N): DAEMON_SERVICE_ROOT_OWNED_CORE — служба демона macOS запускает root-owned копию ядра (`/Library/PrivilegedHelperTools/sing-box-lxd`, копирует само ядро lx.12 на `--service=install`, пара к SPEC 100 форка) вместо файла пользователя; классификатор службы (NotInstalled / Unsafe / Stale / ProcessStale / OK) по plist, цепочке владения, sha256 и паспорту демона; одна команда «Install or update service»; Uninstall и подсказка очистки через копию; заменяет сверку путей SPEC 135 §5.1; **SPECS/136-F-N-DAEMON_SERVICE_ROOT_OWNED_CORE/SPEC.md**
- **137** — фича реализована в ветке, ядро запинено на lx.12, ждёт приёмки владельцем (F-N): CLASSIC_TUN_ROOT_OWNED_CORE — classic-старт с TUN на macOS запускает под root только root-owned копию ядра SPEC 136 (гейт: цепочка владения и sha256 против ядра лаунчера) через `env -i` и постоянное тело `sh` вместо скрипта в DataDir; kill/pkill без шелла; 137.1 — root не пишет по путям пользователя (лог ядра в root-owned `/Library/Logs/sing-box-lxd/classic.log`, снятие TUN без root), аудит всех мест исполнения команд (§12); отказ гейта — диалог с одной sudo-командой (`lxd --service=copy` lx.12, при службе — install) и Retry; развилка времени жизни авторизации (А — сессия, Б — одно действие); **SPECS/137-F-N-CLASSIC_TUN_ROOT_OWNED_CORE/SPEC.md**
- **138** — фича, спека и тест в ветке (F-N): LX_WG_IDLE_KEYS — пара к SPEC 098 форка (ядро lx.13: корневой блок `lx`, `route.lx_idle_*` → `lx.wg.*`, новый `lx.masque.idle_timeout`). Разведка показала, что лаунчер ключи сна WG не писал никогда: десктопные сборки ядра без `with_lx_idle_suspend` отвергают их на старте. Гейт по версии, перенос в сборщике и разбор running-config не нужны. `lx.masque.idle_timeout` задаётся только корневым `lx` шаблона, без UI; сквозной путь закреплён `TestBuildConfigPassesRootLXBlock`. Бамп пина на lx.13 — по релизу ядра, без миграции; развилки по простою WARP MASQUE — §6; **SPECS/138-F-N-LX_WG_IDLE_KEYS/SPEC.md**
- **139** — реализована, слита в develop (2fe0139a), ревью пройдено, ждёт ручной приёмки на Windows §11 (F-N): WINDOWS_ASINVOKER_ELEVATION — манифест `asInvoker` вместо `requireAdministrator` (#99): `platform.IsElevated`, proxy-only без прав, TUN без прав — диалог «Install service / Restart as administrator / Switch to proxy mode», перезапуск с повышением через `ShellExecuteEx runas` и `-handoff`, гейты на брандмауэр/Wintun/HKLM, автозапуск «Start with Windows» (HKCU Run, `-autostart=on|off`), защищённые каталоги Windows считаются непишущимися при выборе раскладки; **SPECS/139-F-N-WINDOWS_ASINVOKER_ELEVATION/SPEC.md**
- **140** — реализована в ветке `spec-140-installer` (CI зелёный, установщик собирается), ревью пройдено, правки по ревью вносятся; сливать строго после 139; ждёт приёмки на Windows §11 (F-N): WINDOWS_INSTALLER — установщик Inno Setup: per-machine в Program Files, поставка win64-full без `portable.txt`, задачи (ярлык, автозапуск, Mesa, служба после 141), штатное закрытие лаунчера через мьютекс и событие Quit, удаление с `-purge-data -yes`, job `build-windows-installer` и ассет `…-win64-setup.exe`; **SPECS/140-F-N-WINDOWS_INSTALLER/SPEC.md**
- **141** — этап разделения daemon-кода сделан в ветке `spec-141-daemon-split` (CI зелёный, ревью идёт); Windows-часть начата в ветке `spec-141-windows` (split + 139 + каркас API); ядро: пререлиз v1.14.2-lx.2-rc.1 срезан 24.09, стабильный lx.2 после живого прогона на Windows (F-N): DAEMON_MODE_WINDOWS — daemon-режим на Windows: служба SCM `sing-box-lxd` (пара к SPEC 103 форка), общий daemon-код выносится из `_darwin.go`, команды через `runas`, классификатор по SCM и DACL, копия набора в `Program Files\sing-box-lxd`, системный прокси в daemon-режиме ставит лаунчер, classic под правами исполняет только копию; **SPECS/141-F-N-DAEMON_MODE_WINDOWS/SPEC.md**

Подробное описание каждой задачи — в SPEC.md соответствующей папки.
