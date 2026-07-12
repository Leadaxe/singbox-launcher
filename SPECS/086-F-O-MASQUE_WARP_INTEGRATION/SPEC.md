# 086-F-N — MASQUE (WARP) интеграция в лаунчер

**Тип:** Feature · **Статус:** N (план; блокирован пином ядра) · **Дата:** 2026-07-12
**Зависит от:** [084 WARP-генератор](../084-F-O-WARP_GENERATOR/SPEC.md) · core `SPECS/021-MASQUE_CONNECT_IP_OUTBOUND`

## 1. Контекст и блокер

MASQUE — второй транспорт WARP (CONNECT-IP/RFC 9484 поверх HTTP/3 или HTTP/2), нужен там,
где провайдер режет WireGuard/QUIC на дефолтном порту. В сообществе просят «WARP как в LxBox
или расширенный»; LxBox уже эмитит MASQUE.

**Блокер:** пин лаунчера = **`1.14.0-lx.1-rc.17`**, где MASQUE-outbound **отсутствует**.
В ядре `sing-box-lx` он реализован и **device-verified** (2026-07-02, warp=on на h3 и h2),
но появился с тега **`v1.14.0-lx.2`** (`protocol/masque/`, core SPEC 021, коммиты
`0f41d00a`/`bd5d1e51`/`ac3d25b8`). Пока пин не поднят до lx.2+/lx.3 — эмитить `masque` в
лаунчере бессмысленно (ядро отвергнет неизвестный тип).

## 2. Решение

**Шаг 0 (за командой ядра / владельцем):** поднять пин `constants.RequiredCoreVersion`
rc.17 → **lx.3** (последний stable; там же Snell/Bridge/L3-forwarding + command-multiplex).
Проверить регрессии (rc.16 без clash_api уже закрыта в rc.17; lx.3 наследует). Это отдельное
решение — эта спека его лишь фиксирует как предусловие.

**Шаг 1 (лаунчер, после бампа):** emit MASQUE-узла. Backend WARP-генератора (SPEC 084) уже
регистрирует аккаунт; для MASQUE нужен **ECDSA P-256** ключ (не X25519) и второй шаг регистрации
(`PATCH /reg/{id}` с `key_type:"secp256r1", tunnel_type:"masque"`) — как в LxBox
`masque_account.dart`/`masque_keys.dart`.

## 3. Контракт emit (из core SPEC 021 — зафиксирован, device-verified)

```jsonc
{
  "type": "masque",
  "tag": "🔥🎭 WARP (MASQUE)",
  "server": "162.159.198.1",     // WARP endpoint IP
  "server_port": 443,
  "profile": "cloudflare",       // cloudflare (деф) | standard
  "network": "h3",               // ТРАНСПОРТ: h3 (QUIC) | h2 (HTTP/2). НЕ tcp/udp!
  "private_key": "<b64 DER EC>", // x509.ParseECPrivateKey
  "public_key":  "<b64 DER PKIX>",
  "ip":   "172.16.0.2/32",
  "ipv6": "2606:4700:110:...::/128",
  "sni": "",                     // деф: consumer-masque.cloudflareclient.com
  "mtu": 1280,                   // h2: ≤16000
  "idle_timeout": "5m",
  "keep_alive_period": "30s",
  "network_list": ["tcp","udp"]
}
```

**Грабли (заложить в UI/emit):**
- `network` = транспорт (`h3`/`h2`), НЕ L4. `"network":"tcp"` → fail-fast ядра. L4 = `network_list`.
- `profile:cloudflare` требует `private_key`+`public_key` (DER base64, парсятся) и хотя бы один из `ip`/`ipv6` CIDR.
- `h2` → `mtu ≤ 16000`. Требует top-level `dns` в конфиге.

## 4. Область (launcher-side)

| Файл | Изменение |
|------|-----------|
| `core/warp/masque.go` (новый) | ECDSA P-256 keygen (DER SEC1/PKIX через `crypto/x509`), 2-шаговая регистрация MASQUE, `MasqueAccount` |
| `core/warp/account.go` | `ToMasqueOutbound()` — map по контракту §3 (не через WG-парсер) |
| `core/config/subscription/node_parser_masque.go` (новый) | парс `masque://…?profile=&network=&…` (для импорта готовых) |
| `core/config/subscription/shareuri_masque.go` (новый) | share-URI encode |
| `core/config/subscription/node_parser_core.go:44` | +case `masque://` в `IsDirectLink`/`ParseNode` |
| `internal/constants/constants.go` | пин ядра rc.17 → lx.3 (Шаг 0) |

## 5. Тесты

- keygen ECDSA P-256 → DER round-trip (`ParseECPrivateKey`/`ParsePKIXPublicKey`).
- `ToMasqueOutbound` даёт валидный map; `network:h3`, ключи base64-DER, ip CIDR.
- парс `masque://` → тот же outbound (round-trip).
- **e2e**: emit → `sing-box check` на **lx.3-бинаре** (не rc.17!) — отложено до бампа.

## 6. Риски

- Бамп пина lx.3 может тянуть другие изменения (command-multiplex DNS-стрим, Snell/Bridge) —
  прогнать полный regression набора лаунчера на lx.3 перед релизом.
- ECDSA-контракт с ядром байт-в-байт (base64 StdEncoding, DER SEC1/PKIX) — сверить с
  `masque_keys.dart` и core-парсером.
- Регистрация MASQUE в РФ: `api.cloudflareclient.com` может быть недоступен напрямую —
  разрешить через активный туннель (клиент уважает системный/HTTP-прокси, SPEC 034).

## 7. Definition of Done

- [ ] (владелец/ядро) пин поднят до lx.3, regression зелёный.
- [ ] `core/warp/masque.go` + emit по контракту §3; парс `masque://`.
- [ ] `go build/test/vet` зелёные; e2e `sing-box check` на lx.3 — OK.
- [ ] Токены/ключи не в логах.

> Пока Шаг 0 не выполнен — задача остаётся в статусе N. Реализация лаунчера тривиальна
> после бампа (контракт закрыт и проверен в ядре).
