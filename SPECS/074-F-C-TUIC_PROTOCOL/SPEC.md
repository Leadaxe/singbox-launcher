# SPEC — 074 TUIC_PROTOCOL

## Проблема

Вставка ссылки `tuic://…` в лаунчер ничего не даёт — узел не появляется, туннель не стартует. Причина: парсер подписок не знает схему `tuic://` (`IsDirectLink` её не распознаёт, ветки в `ParseNode`/`buildOutbound`/`GenerateNodeJSON` нет). При этом:

- ядро (`sing-box-lx`, `with_quic`) **умеет** `type: "tuic"` — проверено `sing-box check`;
- README обещает TUIC в списке «15 протоколов», а `docs/ParserConfig.md` честно помечает его как **не реализован** — README врёт.

## Требования

1. Распознавать `tuic://` как прямую ссылку (smart-paste, Source, `connections[]`).
2. Парсить TUIC v5 URI клиентского формата:
   `tuic://uuid:password@host:port?congestion_control=&alpn=&udp_relay_mode=&allow_insecure=&sni=#name`
   - `uuid` — userinfo username (обязателен), `password` — userinfo password (обязателен);
   - query: `congestion_control` (cubic/new_reno/bbr), `udp_relay_mode` (native/quic), `alpn` (CSV), `sni`, `allow_insecure`/`insecure`/`skip-cert-verify`, `reduce_rtt`/`zero_rtt_handshake`, `heartbeat`, `fp`/`fingerprint`.
3. Генерировать корректный sing-box `tuic` outbound (с обязательным TLS-блоком — TUIC работает поверх QUIC).
4. Share-URI обратно в `tuic://` (round-trip по смыслу, как у остальных протоколов).
5. Документация (`ParserConfig.md`: добавить в поддерживаемые, убрать из «не поддерживаются»; `upcoming.md`).
6. Тесты: parse, buildOutbound, JSON-генерация, round-trip.

## Вне scope

- UI-визард ручного добавления TUIC-ноды (фича работает через URI/подписку; форма — отдельная задача).
- `disable_sni`, `udp_over_stream`, `network` — редкие поля; не покрываем в v1 (общий TLS-эмиттер генератора их не выводит — трогать его ради этого не будем).

## Критерии приёмки

- Тестовая строка `tuic://<uuid>:<pass>@host:443/?congestion_control=bbr&alpn=h3,spdy/3.1&udp_relay_mode=native&allow_insecure=1#name` парсится в `tuic` outbound; сгенерированный конфиг проходит `sing-box check`.
- `go build ./...`, `go test ./...`, `go vet ./...` зелёные.
- README перестаёт врать (TUIC теперь реально работает).
