# Интеграция Go-ядра в LxBox и заделы под общий код

## 1. Как LxBox зовёт ядро

**Путь: gomobile/libbox AAR (Android-only), два слоя моста: Dart → Kotlin (MethodChannel/EventChannel) → Go (libbox).** iOS — заглушка (`app/ios/Runner/AppDelegate.swift`, `SceneDelegate.swift`, libbox там нет).

### Подключение ядра
- AAR форка: `app/android/app/libs/libbox.aar` (~110 MB, не в git), пин версии — `app/android/libbox.version` (сейчас `v1.14.0-lx.27-rc.4` по `docs/KERNEL.md`), загрузка `scripts/fetch-libbox.sh`, подключение `app/android/app/build.gradle.kts:144` → `implementation(files("libs/libbox.aar"))`. Java-пакет `io.nekohasekai.libbox`.
- Ядро живёт внутри процесса VpnService: `BoxService.kt` поднимает `CommandServer(this, platformInterface)` и стартует/перезагружает ядро через `cs.startOrReloadService(config, buildOverrideOptions(config))` (фичи форка with_lx_command; `OverrideOptions` — «тень» поверх конфига: includePackage, autoRedirect). `BoxApplication.kt` делает `Libbox.setup(opts)` и `Libbox.setLocale(...)`.
- UI-контроль ядра — **нативный libbox CommandClient**, не Clash HTTP (`docs/KERNEL.md:16`; Makefile.lx форка: clash_api выключен только в Android AAR, «where LxBox manages the core over the native libbox CommandClient»). `BoxCommandClient.kt` держит 4 клиента: status / screen / profiler / ping, подписки через `CommandClientOptions().addCommand(Libbox.CommandStatus|CommandOutbounds|CommandGroup|CommandConnections|CommandDNS)`.
- `ProbeSession.kt` — headless probe-ядро (конфиг без tun) для «Test servers» при выключенном VPN, тоже свой `CommandServer`/`CommandClient`.

### Каналы (единый реестр `lib/services/platform_channels.dart`)
- MethodChannel `com.leadaxe.lxbox/methods` — все команды (обработчик `handleMethodCall` в `android/.../vpn/VpnPlugin.kt`).
- EventChannel'ы: `com.leadaxe.lxbox/status_events` (статус туннеля), `lxbox/coreLog` (логи ядра), `lxbox/cc/status`, `lxbox/cc/outbounds`, `lxbox/cc/groups`, `lxbox/cc/connections`, `lxbox/cc/dns` (push-стримы CommandClient → sink'и в `BoxVpnService.kt`).

### Dart-клиенты и команды
- `lib/vpn/box_vpn_client.dart` + реестр имён `lib/vpn/box_vpn_client/method_names.dart` (класс `_Methods`, зеркало switch-case в VpnPlugin.kt): `saveConfig`/`getConfig`/`getFilesDir`; `startVPN`/`startVpnHeadless`/`stopVPN`/`forceStopVPN`/`getVpnStatus`/`reloadVPN`/`resetNetwork`/`clearDnsCache`; настройки (`setAllowBypass`, `setAutoRedirect`, `setHasTun`, `setMemoryLimit`, `setBackgroundMode`, …); диагностика `getCoreVersion` → `Libbox.version()`, `getMemoryInfo`, `pprofProfile` (libbox PProfServer), `setQuicKnob` → `Libbox.setQuicGSODisabled/setQuicECNDisabled`; **`formatConfig` → статический `Libbox.formatConfig(text)` без живого сервиса**.
- `lib/vpn/cc_channel.dart` (`CcChannel`) — императивы CommandClient: `ccUrlTestOutbound`, `ccUrlTestGroup`, `ccSelectOutbound`, `ccGetRules`, `ccGetGroups`, `ccGetPool`, `ccGetDnsGroups`, `ccGetRunningConfig`, `ccGetUrlViaOutbound`, `ccCloseConnection(s)`, lifecycle `ccConnectScreen`/`ccConnectProfiler`/`ccSetStatusFast`/`ccPauseClients`/`ccResumeClients`/`ccResyncForReopen`/`ccCancelPing`; probe: `probeStart`/`probeUrlTest`/`probeGetUrl`/`probeStop`.

## 2. Экспортируемые API в /Users/macbook/projects/sing-box-lx

- **`experimental/libbox` (gomobile-фасад):** статические `CheckConfig(configContent string) error` (`config.go:50`), `FormatConfig(configContent) (*StringBox, error)` (`config.go:258`), `Version()`, `Setup()`, `AvailablePort`, `RandomHex`, Format*-хелперы (`setup.go`, `service.go`). Внутренний `parseConfig` (`config.go:42`) не экспортирован. LX-расширения CommandClient (`command_client_command_lx.go`, тег `with_lx_command`): `URLTestOutbound`, `GetRules`, `GetGroups`, `GetOutbounds`, `GetRunningConfig` (:282), `GetURLViaOutbound` (:381), `GetPool` (:446), `GetDNSGroups` (:507).
- **`daemon/`** — in-process хостинг ядра (общий для AAR и lxd): `managed_service.proto`/`started_service.proto` (gRPC), `instance_command_lx.go` (снапшот running-config тем же энкодером, что FormatConfig).
- **`lxd/`** — пакет демона: `lxd.Run(ctx, Options)`, `LoadDaemonConfig`/`SaveDaemonConfig`, `NewLocalClient(listen, secret, tls)` c `MintClientCode`/`ListClients`/`RemoveClient`, mTLS-trust клиентов.
- **`sing-box lxd`** (`cmd/sing-box/cmd_lxd_lx.go`, тег `with_lxd`): «Run the sing-box-lx daemon: host the core in-process behind a reload-surviving control channel». Флаги `--state-dir`, `--config-force`, `--run`, `--service=install|install-user|uninstall` (+`--purge`, `--dry-run`); подкоманды `lxd client add|list|remove` (одноразовые invite для mTLS-энролла лаунчера). Настройки соединения — только `<state-dir>/daemon.json`; dev-дефолт `127.0.0.1:9091` h2c без секрета.
- **Парсера подписок в форке НЕТ.** Grep по «subscription» даёт только clashapi/xhttp/трафик-код. Go-парсер живёт в лаунчере (`/Users/macbook/projects/singbox-launcher/core/config/subscription/`, ~40 файлов `node_parser_*`), Dart-парсер — в LxBox (`app/lib/services/parser/`: `parse_all.dart`, `uri_parsers.dart`, `json_parsers.dart`, `singbox_config.dart`, …).
- Есть `experimental/libbox/ffi.json` — генерация FFI-биндингов (C#/Apple) поверх того же пакета; `clients/android|apple|desktop` — пустые каталоги-плейсхолдеры.

## 3. Упоминания планов унификации

Вся зафиксированная «унификация» — на стороне **singbox-launcher/SPECS**, и это осознанный **порт логики (parity), а не общий код**:

- `SPECS/094-F-C-SUBSCRIPTION_PARSER_PARITY/SPEC.md`: «Зонтичная задача: перенос в лаунчер класса возможностей парсера, накопленного в проекте LxBox (Dart/Flutter, `app/lib/services/parser/`). Четыре фазы A–D» и «Санитайзы в лаунчере во многом на уровне LxBox … Фазы A–D их переиспользуют, а не дублируют». Статус — Complete.
- `SPECS/README.md`: **087** CHANNELS_MODEL — «порт LxBox §125/§267»; **090** PRESET_LANGUAGE — «`#if` уже общий с LxBox; … документ shared-формата» (движок подстановки в LxBox: `app/lib/services/builder/if_engine.dart`, описан в `docs/TEMPLATE.md`: «The shared substitution core lives in if_engine.dart»); **094** — итог паритета (дедуп по `NodeIdentityHash` до назначения тегов и т.д.).
- `SPECS/095-F-C-NODE_SUBTITLE_AND_INFO/SPEC.md`: «Перенос из LxBox трёх связанных вещей…», «Эталон — `/Users/macbook/projects/LxBox`» с таблицей соответствий файлов.
- `SPECS/101-F-C-DETOUR_NODE_HASH/SPEC.md`: «Решение (симметрично LxBox)» — идентичность нод держится симметричной в обоих парсерах.
- `SPECS/045-F-N-STATE_CONFIG_DECOUPLING/LXBOX_NOTES.md` — «Фаза 0 — LxBox deep-dive», с явной оговоркой: «Архитектурные идеи переносимы, конкретные примитивы (ChangeNotifier, sealed classes) — нет».
- Со стороны LxBox планов общего кода с лаунчером в `AGENTS.md`/`app/CLAUDE.md` нет; связка — только общее **ядро-форк** (`docs/KERNEL.md`: релизные номера ядра согласуются с «field test builds of the mac launcher», чтобы бинари не коллидировали).

Итого: сегодня общие между проектами — (а) ядро sing-box-lx, (б) формат шаблона/`#if`, (в) зеркально поддерживаемые контракты парсера (дедуп-хеш, санитайзы) — но кодовые базы парсеров дублируются намеренно (Go в лаунчере, Dart в LxBox).

## 4. Техоценка: Go-парсинг подписки из Flutter

**Возможно, и точка расширения уже прожжена** — прецедент «строка в Go → строка обратно, без живого сервиса» существует end-to-end:

1. **Готовый паттерн:** `formatConfig` — Dart `_Methods.formatConfig` (`method_names.dart:101`) → `VpnPlugin.kt` (case `"formatConfig"`, ~строка 900: `io.nekohasekai.libbox.Libbox.formatConfig(text)?.value` на `Dispatchers.IO`, null при любом throw) → статический Go `FormatConfig` в `experimental/libbox/config.go`. Тот же паттерн у `getCoreVersion` и `setQuicKnob`. Гипотетический `Libbox.parseSubscription(body) → StringBox(JSON-массив нод)` ложится в него 1:1: экспорт-функция в `experimental/libbox`, пересборка AAR (`cmd/internal/build_libbox`, теги уже включают `with_lx_command`), один case в `VpnPlugin.kt`, одна константа в `method_names.dart`.
2. **Главный разрыв — сам парсер не в форке.** Go-реализация живёт в `singbox-launcher/core/config/subscription` и завязана на внутренние типы лаунчера (`configtypes.ParsedNode` и др.). Для общего кода её нужно вынести в отдельный Go-модуль, который импортируют и лаунчер, и `experimental/libbox` форка (gomobile собирает именно этот пакет). Ограничение gomobile-фасада — примитивы/строки/итераторы, поэтому контракт «тело подписки строкой → JSON-строка со списком нод» — правильная минимальная поверхность (стиль `StringBox` уже принят).
3. **Альтернативные швы:** headless probe (`ProbeSession.kt`) показывает, что второй экземпляр ядра без tun поднимается штатно — но для парсинга он не нужен; на десктопе аналогичный RPC мог бы жить в `lxd`-демоне (у лаунчера уже есть daemon-режим), на Android демона нет — только статический libbox-вызов.
4. **Контрактные риски:** выход Go-парсера должен состыковаться с Dart-пайплайном `parse_all.dart → NodeSpec → build_config.dart` (нужен ингест NodeSpec-из-JSON) и сохранить симметрию идентичности нод (SPEC 101) и порядок «import rules до вычисления хеша» (`subscription_controller.dart:248`, зафиксировано в SPEC 094) — иначе у пользователей «переедут» выключенные ноды и дедуп.

**Ключевые файлы:** `/Users/macbook/projects/LxBox/app/lib/vpn/box_vpn_client.dart`, `/Users/macbook/projects/LxBox/app/lib/vpn/cc_channel.dart`, `/Users/macbook/projects/LxBox/app/lib/services/platform_channels.dart`, `/Users/macbook/projects/LxBox/app/android/app/src/main/kotlin/com/leadaxe/lxbox/vpn/{VpnPlugin,BoxService,BoxCommandClient,ProbeSession,BoxApplication}.kt`, `/Users/macbook/projects/sing-box-lx/experimental/libbox/{config.go,command_client_command_lx.go,ffi.json}`, `/Users/macbook/projects/sing-box-lx/cmd/sing-box/cmd_lxd_lx.go`, `/Users/macbook/projects/sing-box-lx/lxd/`, `/Users/macbook/projects/singbox-launcher/SPECS/094-F-C-SUBSCRIPTION_PARSER_PARITY/SPEC.md`, `/Users/macbook/projects/singbox-launcher/SPECS/045-F-N-STATE_CONFIG_DECOUPLING/LXBOX_NOTES.md`, `/Users/macbook/projects/singbox-launcher/core/config/subscription/`.