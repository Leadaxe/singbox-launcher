# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

---

## EN

### Internal / Refactoring

(пункты для следующего релиза)

### Highlights

- **VLESS / Trojan subscription links:** Parser and `GenerateNodeJSON` build sing-box [V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) from URI query: `ws` (path, headers Host), `http` (`host` as JSON list, path), `grpc` (`service_name`), `xhttp` → `httpupgrade` (only `host` / `path` / `headers` per docs; Xray `mode` is not in the schema). VLESS `security=none` omits TLS; plain TLS and Reality (`pbk`) follow [outbound TLS](https://sing-box.sagernet.org/configuration/shared/tls/#outbound). Trojan + WS gets `transport` + `tls`. VMess gRPC uses `service_name` from JSON `path`. Wizard preview deduplicates tags like the main parser (`MakeTagUnique`). Query keys are matched case-insensitively where providers use `allowinsecure=0`; multiply-encoded `alpn` is normalized; `fp=QQ` maps to utls `qq`; `tcp`/`raw` with `headerType=http` maps to HTTP transport; `packetEncoding` is copied to outbound `packet_encoding`.

- **VLESS `xtls-rprx-vision-udp443`:** Subscriptions often use Xray’s vision-udp443 flow; sing-box only accepts `xtls-rprx-vision`. The parser already mapped this internally, but generated `config.json` still wrote the original flow and omitted `packet_encoding`. Generation now matches sing-box (vision + `packet_encoding: xudp` when applicable).

- **SOCKS5 in connections:** Parser now supports `socks5://` and `socks://` direct links in Source and Connections (e.g. `socks5://user:pass@proxy.example.com:1080#Office SOCKS5`). Resulting nodes become sing-box outbounds of type `socks` and participate in selectors like other protocols.

- **Linux build:** `build_linux.sh` now checks for required system packages (OpenGL/X11) and prints install commands for Debian/Ubuntu and Fedora. README and new `docs/BUILD_LINUX.md` document dependencies; optional `build/Dockerfile.linux` allows building without installing dev packages locally (see [Issue #40](https://github.com/Leadaxe/singbox-launcher/issues/40)).

- **Wizard — Sources tab:** Scrollable areas (URL field, sources list, server preview, outer tab scroll) reserve a right gutter so the scrollbar does not overlap text or buttons.

- **macOS build script:** `build_darwin.sh` supports `-i` (if the app already exists in `/Applications`, only the executable is updated so `Contents/MacOS/bin/` and logs are kept; otherwise full `.app` copy; then removes the built `.app` from the project directory), `arm64` for a fast Apple Silicon–only build, and `-h` / `--help` (parsed before `go mod tidy`). README documents the options.

### Technical / Internal

- **Docs:** `docs/ParserConfig.md` — VLESS/Trojan URI: expanded query parameters and link to `SPECS/023-…/SUBSCRIPTION_PARAMS_REPORT.md` (sing-box field reference).

(пункты для следующего релиза)

---

## RU

### Внутреннее / Рефакторинг

(пункты для следующего релиза)

### Основное

- **Ссылки VLESS / Trojan из подписок:** парсер и `GenerateNodeJSON` собирают [V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) sing-box из query: `ws`, `http` (поле `host` — список строк, как в доке), `grpc` (`service_name`), `xhttp` → `httpupgrade` (в схеме только `host` / `path` / `headers`; параметр `mode` из Xray не входит в документированную схему). VLESS: `security=none` без TLS; обычный TLS и Reality (`pbk`) — по [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound). Trojan + WS: `transport` и `tls`. VMess gRPC: `service_name` из `path` в JSON. Превью в визарде: `MakeTagUnique` как в основном парсере. Ключи query сравниваются без учёта регистра (например `allowinsecure=0`); многослойно закодированный `alpn` нормализуется; `fp=QQ` → utls `qq`; `tcp`/`raw` + `headerType=http` → транспорт `http`; `packetEncoding` → `packet_encoding` в outbound.

- **VLESS `xtls-rprx-vision-udp443`:** В подписках часто приходит flow из Xray; sing-box понимает только `xtls-rprx-vision`. Парсер уже переводил значение во внутренней структуре, но в итоговом `config.json` попадал исходный flow без `packet_encoding`. Генерация конфига исправлена (vision + при необходимости `packet_encoding: xudp`).

- **SOCKS5 в connections:** В Source и Connections можно добавлять прямые ссылки `socks5://` и `socks://` (например `socks5://user:pass@proxy.example.com:1080#Office SOCKS5`). Узлы превращаются в outbound типа `socks` и участвуют в селекторах наравне с остальными протоколами.

- **Сборка на Linux:** скрипт `build_linux.sh` проверяет наличие системных пакетов (OpenGL/X11) и выводит команды установки для Debian/Ubuntu и Fedora. В README и в новом `docs/BUILD_LINUX.md` описаны зависимости; добавлен опциональный `build/Dockerfile.linux` для сборки без установки dev-пакетов (см. [Issue #40](https://github.com/Leadaxe/singbox-launcher/issues/40)).

- **Визард — вкладка Sources:** у прокручиваемых блоков (поле URL, список источников, превью серверов, общий скролл вкладки) справа зарезервировано место под полосу прокрутки, чтобы она не наезжала на текст и кнопки.

- **Сборка macOS:** в `build_darwin.sh` флаг `-i` при уже установленном приложении обновляет только исполняемый файл (сохраняются `Contents/MacOS/bin/` и логи), при первой установке копируется весь `.app`, после успеха удаляется собранный `.app` из каталога проекта; режим `arm64`; `-h` / `--help` до `go mod tidy`. В README описаны опции.

### Техническое / Внутреннее

- **Документация:** `docs/ParserConfig.md` — VLESS/Trojan URI: расширен список query-параметров и ссылка на `SPECS/023-…/SUBSCRIPTION_PARAMS_REPORT.md` (справочник полей sing-box).

(пункты для следующего релиза)
