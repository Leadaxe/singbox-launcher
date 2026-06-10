# IMPLEMENTATION_REPORT — 074 TUIC_PROTOCOL

**Дата:** 2026-06-10 · **Статус:** Complete

## Итог

`tuic://` теперь полноценный протокол: распознаётся как прямая ссылка, парсится в sing-box `tuic` outbound, генерируется в конфиг, кодируется обратно в share-URI. Ядро (`sing-box-lx`, `with_quic`) принимает результат — `sing-box check` на сгенерированном конфиге проходит (exit 0).

Закрыта дыра, на которую жаловался пользователь: раньше `IsDirectLink` не знал `tuic://`, узел не появлялся, туннель не стартовал; README при этом обещал TUIC. Теперь обещание правдиво.

## Изменения

**Новые файлы:**
- `core/config/subscription/node_parser_tuic.go` — `buildTuicOutbound` + `buildTuicTLS`; хелперы `isValidTuicCongestionControl` (cubic/new_reno/bbr), `tuicQueryFlagTrue`, `normalizeTuicHeartbeat` (голое число → секунды).
- `core/config/subscription/shareuri_tuic.go` — `shareURIFromTuic` (обратно в `tuic://`).
- `core/config/subscription/node_parser_tuic_test.go` (9 кейсов) + `core/config/generator_tuic_test.go` (URI→JSON).

**Правки:**
- `node_parser_core.go` — `IsDirectLink` += `tuic://`; `case "tuic"` в `ParseNode`; `tuic` в проверку userinfo и извлечение password; ветка в `buildOutbound`; doc-комментарий пакета.
- `outbound_generator.go` — ветка `tuic` в `GenerateNodeJSON` (uuid/password/congestion_control/udp_relay_mode/zero_rtt_handshake/heartbeat); TLS выводится общим блоком; «Supports:»-комментарий.
- `share_uri.go` — `case "tuic"`.
- `docs/ParserConfig.md` — строка таблицы #10, убран из «не поддерживаются», share-таблица, счётчики «10 схем», комментарий-список.
- `docs/release_notes/upcoming.md` — EN + RU.

## Маппинг URI → sing-box

`tuic://uuid:password@host:port?...#name`:
- `uuid` ← userinfo username (обязателен), `password` ← userinfo password (обязателен).
- `congestion_control` (валидируется: cubic/new_reno/bbr; неизвестное → warn+drop), `udp_relay_mode` (native/quic), `heartbeat` (число→`Ns`), `reduce_rtt`/`zero_rtt_handshake` → `zero_rtt_handshake`.
- TLS обязателен (QUIC): `server_name` ← `sni`|server; `alpn` (CSV); `insecure` ← `allow_insecure`/`insecure`/`skip-cert-verify` (важно: `tlsInsecureTrue` сам **не** ловит snake_case `allow_insecure` — обработано явно); `utls` ← `fp`/`fingerprint`.

## Решения

- TLS-блок генератора (общий) выводит enabled/server_name/alpn/utls/insecure/reality — TUIC покрывается им полностью; отдельная ветка для TLS не нужна. Поэтому `disable_sni`/`udp_over_stream` в v1 не поддержаны (потребовали бы правки общего TLS-эмиттера — вне scope).
- Share-URI нормализует `insecure=1` (round-trip смысла, не байт-в-байт; как у naive/hysteria2).

## Проверки

- `go build ./...` (вкл. UI), `go vet ./core/...`, `go test ./core/config/...` — зелёные. gofmt чист.
- `sing-box check` (ядро lx) на сгенерированном TUIC outbound — exit 0.
- Тестовая строка пользователя (`congestion_control=bbr&alpn=h3,spdy/3.1&udp_relay_mode=native&allow_insecure=1`) воспроизведена на фейк-кредах в тестах — парсится и генерируется корректно.

## Безопасность

Боевые креды из присланной строки в репозиторий/диск не попадали; тесты и `check` — на фейковых плейсхолдерах, временный конфиг в `/tmp` удалён.

## Вне scope / дальше

- UI-визард ручного добавления TUIC-ноды (фича работает через URI/подписку).
- `disable_sni`, `udp_over_stream`, `network` — редкие поля; при необходимости — отдельной задачей с правкой общего TLS-эмиттера.
