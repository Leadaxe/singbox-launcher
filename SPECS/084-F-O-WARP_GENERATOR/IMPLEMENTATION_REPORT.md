# 084 — WARP-генератор: отчёт о реализации (backend)

**Дата:** 2026-07-12 · **Статус:** backend реализован (фаза 1); UI-визард — отдельная фаза 084.1.
**Ядро:** `1.14.0-lx.1-rc.17` (WG + AmneziaWG). MASQUE — вне объёма (нет в rc.17, см. §MASQUE).

## Что сделано

1. **Новый пакет `core/warp/`** — stdlib-leaf, HTTP-клиент инъектируется (DI), без зависимости
   от контроллера. Быстро компилируется/тестируется.
   - `client.go` — `GenerateKeypair` (X25519 через `crypto/ecdh`, приватник **на устройстве**,
     в Cloudflare уходит только pubkey), `Register` (`POST /v0a4005/reg`), `applyLicenseSafe`
     (WARP+ `PATCH …/account`, fail-soft), парс ответа. Версия API + заголовок в одной
     константе (комментарий «сверять с wgcf перед релизом»).
   - `account.go` — `Account`, `Reserved()` (client_id base64 → 3 байта), `ToWireguardURI()`
     (строит `wireguard://…` для существующего парсера — **единственный источник emit**),
     `DisplayTag()` (эмодзи как в LxBox).
   - `obfuscation.go` — AmneziaWG-пресет (`s1=s2=0`, `h1..h4=1..4` → init/response как plain WG,
     Cloudflare принимает; `jc/jmin/jmax` + masquerade `id/ip/ib`).
   - `endpoints.go` — anycast-пул (prefixes/ports/SNI, синхронно с LxBox), `RandomEndpoint`/
     `RandomSNI` с инъекцией `*rand.Rand` для детерминизма в тестах.
2. **Парсер `node_parser_wireguard.go`** (аддитивно):
   - `reserved=b0,b1,b2` → `peer.reserved` (`parseReservedTriplet`, валидация 0–255, drop-one-param).
   - **Промоушен masquerade `ip/id/ib`** на endpoint (был пропущен — критичный gap для
     обфускации; подавляется при явном `i1`, как требует ядро).
3. **Share-URI `shareuri_wireguard.go`** — re-emit `reserved` и `ip/id/ib` (lossless round-trip);
   `intSliceFromWireGuardField` helper.

## Проверка (DoD)

- `go build ./core/warp/ ./core/config/subscription/` — **OK**
- `go test ./core/warp/ ./core/config/subscription/` — **OK** (14 warp-кейсов + round-trip reserved/masquerade/i1-suppress)
- `go vet …` — **OK**
- **Живая регистрация** WARP через Cloudflare API — HTTP 200 (проверено в сессии).
- **E2E**: `Account → ToWireguardURI → ParseNode → sing-box check` = **OK**; в конфиге
  `reserved:[29,172,92]`, `mtu:1280`, `ip:quic id:www.google.com ib:chrome jc:4`.

## Безопасность

- Приватный ключ генерится локально, не логируется; в Cloudflare — только pubkey.
- Токен/ключ — secret; исходящий вызов только к `api.cloudflareclient.com`.
- Аккаунт анонимный (install_id пуст) — без деанонимизации (CONSTITUTION §6).

## Осталось (следующие фазы)

- **084.1 UI-визард**: диалог в `ui/configurator/` (транспорт, obfuscate-тумблер, license,
  endpoint override, 🎲 random SNI) + добавление узла как Source/endpoint в state.
- Интеграция сетевого клиента: прокинуть `core.CreateHTTPClient` (proxy-aware, macOS TLS-fix)
  в `warp.NewClient` из слоя сервисов.
- **MASQUE**: спека в репо ядра (бамп пина rc.17→lx.3), в лаунчере — за флагом.

## MASQUE — почему не в этой фазе

`type:"masque"` в ядре появился в **rc.21+/lx.3**; пин лаунчера = **rc.17**. Реализация emit
без поддержки ядра бессмысленна. Запрос на ядро — отдельной спекой в `sing-box-lx`.
