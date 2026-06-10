# PLAN — 074 TUIC_PROTOCOL

Модель — ближайший аналог `hysteria2` (QUIC + TLS + auth). Паттерн «parser → map (`buildOutbound`) + JSON (`GenerateNodeJSON`) + share-URI» уже устоялся.

## Изменяемые файлы

| Файл | Изменение |
|------|-----------|
| `core/config/subscription/node_parser_core.go` | `IsDirectLink` += `tuic://`; `case "tuic"` в `ParseNode`; добавить `tuic` в проверку userinfo (host+uuid) и в извлечение password из userinfo; ветка `buildTuicOutbound` в `buildOutbound`; апдейт doc-комментария пакета |
| `core/config/outbound_generator.go` | ветка `node.Scheme == "tuic"` в `GenerateNodeJSON` (uuid/password/congestion_control/udp_relay_mode/zero_rtt_handshake/heartbeat); TLS выводит общий блок ниже; апдейт «Supports:»-комментария |
| `core/config/subscription/share_uri.go` | `case "tuic": shareURIFromTuic(out)` |
| `docs/ParserConfig.md` | строка таблицы #10 TUIC; убрать TUIC из «Не поддерживаются»; счётчики схем |
| `docs/release_notes/upcoming.md` | запись про TUIC |
| `README.md` / `README_RU.md` | список «15 протоколов» уже содержит `tuic` — теперь правда, правок не требует |

## Новые файлы

| Файл | Содержимое |
|------|------------|
| `core/config/subscription/node_parser_tuic.go` | `buildTuicOutbound`, `buildTuicTLS`, хелперы `isValidTuicCongestionControl` / `tuicQueryFlagTrue` / `normalizeTuicHeartbeat` |
| `core/config/subscription/shareuri_tuic.go` | `shareURIFromTuic` |
| `core/config/subscription/node_parser_tuic_test.go` | parse / buildOutbound / generator JSON / round-trip |

## Ключевые решения

- `allow_insecure` (snake_case) `tlsInsecureTrue` НЕ ловит (только `insecure`/`allowInsecure`) — обрабатываем явно в `buildTuicTLS`.
- TLS у TUIC обязателен → `buildTuicTLS` всегда ставит `tls.enabled=true`; SNI = `sni` или `node.Server`.
- `congestion_control` валидируем (cubic/new_reno/bbr), неизвестное — warn+drop (как obfs у hysteria2).
- `heartbeat`: голое число → секунды (`10`→`10s`).
- Share-URI нормализует `insecure=1` (round-trip смысла, не байт-в-байт — как у naive).
