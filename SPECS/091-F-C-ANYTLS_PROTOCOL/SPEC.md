# 091-F-C — AnyTLS протокол (парсер подписок + share-URI)

**Тип:** Feature · **Статус:** C (реализовано) · **Дата:** 2026-07-12 · **Ядро:** rc.17 (`C.TypeAnyTLS`, `option/anytls.go`) — есть.

## Проблема
Ядро rc.17 поддерживает AnyTLS-outbound, но парсер подписок лаунчера не читал `anytls://` —
узлы AnyTLS в подписках молча терялись. AnyTLS — растущий протокол (сессионный пул поверх TLS).

## Решение
Добавлен парсер по образцу TUIC (единый credential в userinfo как Trojan, обязательный TLS):
- `node_parser_anytls.go` — `buildAnyTLSOutbound` (password + `idle_session_check_interval`/
  `idle_session_timeout`/`min_idle_session`) + `buildAnyTLSTLS` (sni/insecure/utls-fp/alpn).
- `node_parser_core.go` — `anytls://` в `IsDirectLink`, scheme-dispatch (порт 443, TLS-валидация
  userinfo), build-dispatch.
- `shareuri_anytls.go` + `share_uri.go` — обратный encode (round-trip).

## Формат URI
`anytls://password@host:port?sni=&insecure=1&alpn=&fp=&idle_session_timeout=&min_idle_session=#name`

## Проверка (DoD)
- `go test ./core/config/subscription/` — OK (parse, build, missing-userinfo reject, share round-trip).
- Ядро rc.17 принимает эмит: `sing-box check` — OK.
- `go vet` — OK.
