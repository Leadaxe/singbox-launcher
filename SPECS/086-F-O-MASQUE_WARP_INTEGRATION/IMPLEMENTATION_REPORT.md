# 086 — MASQUE (WARP) интеграция: отчёт о реализации

**Дата:** 2026-07-12 · **Статус:** реализовано (parser + generator), device-verified на lx.3.
**Ядро:** пин поднят rc.17 → **1.14.0-lx.3** (коммит бампа) — MASQUE-outbound доступен.

## Сделано

1. **Парсер `masque://`** (`core/config/subscription/node_parser_masque.go`) — импорт готовых
   MASQUE-конфигов (из LxBox/wgcf-подобных). Маппит на sing-box masque outbound: `private_key`/
   `public_key` (base64 DER), `ip`/`ipv6` (tunnel addr, не `address`), `profile`/`network`(h3/h2)/
   `sni`/`mtu`/`idle_timeout`/`keep_alive_period`. Raw-`/` в userinfo-ключе — через существующий
   `percentEncodeWGUserinfoSlashes`. Диспатч в `IsDirectLink`/`ParseNode` (как wireguard).
2. **Share-URI encoder** (`shareuri_masque.go` + `share_uri.go`) — обратный round-trip.
3. **Генератор** (`core/warp/masque.go`) — `GenerateECDSAKeypair` (P-256, SEC1/PKIX DER — контракт
   ядра), `RegisterMasque` (2-шаговая регистрация Cloudflare: POST /reg с throwaway WG-ключом →
   PATCH /reg/{id} `key_type:secp256r1, tunnel_type:masque`), `ToMasqueOutbound`.

## Найдено и исправлено живым тестированием (важно)

- **Серверный pubkey приходит как PEM** → нормализация в чистый base64(DER)
  (`normalizePEMToBase64DER`), иначе `sing-box: parse public_key: illegal base64`.
- **Data-plane endpoint** — под `peer.endpoint.v4` (+ `ports`), а НЕ `endpoint.host`
  (тот = control-plane hostname `engage.cloudflareclient.com`). Реальный сервер = `162.159.198.x`.

## Проверка (DoD)

- `go test ./core/warp/ ./core/config/subscription/` — OK (keygen DER-валидность,
  emit, parse, round-trip, network-валидация).
- `go vet` — OK.
- **Живая регистрация** MASQUE через Cloudflare API — успех (server 162.159.198.2, clean 124-char PKIX key).
- **E2E на бинаре lx.3**: emit → `sing-box check` — **OK** (и на живом аккаунте, и на fixture).

## Безопасность

- ECDSA-приватник генерится на устройстве, в Cloudflare уходит только pubkey. Токен/приватник —
  secret, не логируются (сводка редактится).

## Осталось (UI, follow-up 086.1)

- UI-визард выбора транспорта (h3/h2) + кнопка «Add WARP (MASQUE)» — по образцу WARP-визарда (084.1).
- Прокидка proxy-aware HTTP-клиента (`core.CreateHTTPClient`) в `warp.NewClient` из слоя сервисов.
