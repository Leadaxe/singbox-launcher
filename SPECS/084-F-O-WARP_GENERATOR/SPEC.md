# 084-F-N — WARP-генератор (Cloudflare API, WireGuard + AmneziaWG)

**Тип:** Feature · **Статус:** N → O (in progress) · **Дата:** 2026-07-12
**Ядро:** `1.14.0-lx.1-rc.17` (WG + AmneziaWG 2.0 доступны; **MASQUE — нет**, см. §9)

---

## 1. Проблема и мотивация

Встроенного WARP в десктопном лаунчере нет. Пользователи (Telegram, топ-1 запрос,
16 человек) просят «WARP как в LxBox или расширенный»; отдельная жалоба: «warp,
сгенерированный в программе, блокируется напрямую, работает только через detour»
(15.06.2026), автор обещал «докрутить встроенный WARP-генератор до AWG 1.5» (15.06).

Сейчас пользователи вручную носят конфиги с `warp-generator.github.io` или из Amnezia.
Это небезопасно (сторонние воркеры отдают приватный ключ со своего сервера) и неудобно.

**Цель:** регистрировать WARP-аккаунт прямо в лаунчере (ключ генерится на устройстве,
приватник никогда не покидает машину — как `wgcf`/официальный клиент), с
AmneziaWG-обфускацией для обхода DPI и рандомизацией endpoint (обход заблокированного
`engage.cloudflareclient.com:2408`).

## 2. Что уже есть (фундамент)

- **Ядро rc.17**: `wireguard`-endpoint с `with_awg` — все AWG 2.0 поля (`jc/jmin/jmax`,
  `s1–s4`, `h1–h4` + ranges, `i1–i5`, `id/ip/ib` masquerade-сахар).
- **Парсер** `core/config/subscription/node_parser_wireguard.go` — уже принимает
  `wireguard://`/`awg://`, промоутит AWG-поля (`applyAWGFields`), клампит MTU AWG→1280,
  нормализует bare-address→CIDR, вычисляет tag. **Идеальный emit-канал для генератора.**
- **Эталон** — `LxBox/app/lib/services/warp/` (проверенный на проде контракт Cloudflare API).
- **Живая проверка (в этой сессии):** `POST https://api.cloudflareclient.com/v0a4005/reg`
  вернул HTTP 200; собранный `wireguard://`-узел с `reserved` открыл rutracker через WARP
  (HTTP/2 301, `server: cloudflare`). Контракт рабочий.

## 3. Область (scope)

**В объёме (эта спека, backend):**
1. Пакет `core/warp/` — клиент регистрации Cloudflare + модель аккаунта + сборка узла.
   - `genKeypair()` — X25519 на устройстве (crypto/ecdh, stdlib Go 1.20+).
   - `Register(ctx, opts)` — `POST /{version}/reg`, парс ответа (peer pubkey, addresses,
     `client_id`→`reserved`, device_id, token).
   - `ApplyLicense(ctx, acc, key)` — WARP+ (`PATCH /reg/{id}/account`), fail-soft.
   - AmneziaWG-пресет (`s1=s2=0`, `h1..h4=1..4` — init/response бит-в-бит как plain WG,
     Cloudflare принимает; `jc/jmin/jmax` + `id/ip/ib` сбивают DPI).
   - Endpoint-пул (`prefixes`/`ports`/`sni_pool`) — рандомный `ip:port` вместо
     заблокированного дефолта.
2. Сборка share-URI `wireguard://…?…&reserved=…` / `awg://…` из аккаунта → скармливается
   существующему парсеру → `ParsedNode`. **Единственный источник emit — парсер**, генератор
   не дублирует построение endpoint.
3. Поддержка `reserved` в парсере (аддитивно): `?reserved=b0,b1,b2` (десятичные) →
   `peer.reserved = [b0,b1,b2]`. WARP требует reserved; сейчас поля нет.
4. Unit-тесты: keypair, сборка body регистрации, парс reg-ответа (fixture), reserved из
   client_id, сборка URI, AWG-пресет, endpoint-пул детерминизм (seeded).

**Вне объёма (следующие итерации / спеки):**
- UI-визард регистрации (диалог в `ui/configurator/`) — отдельная фаза 084.1.
- **MASQUE** — в rc.17 ядра нет (см. §9) → спека в репо ядра, за флагом до бампа пина.
- Реальный сетевой скан живости endpoint (LxBox тоже не сканирует — берёт рандом из блоков).

## 4. Дизайн данных

```go
// core/warp
type Account struct {
    PrivateKey string   // base64 X25519, на диск в state (secret)
    PeerPublic string   // base64, из ответа Cloudflare
    ClientV4   string   // interface address v4
    ClientV6   string   // interface address v6
    ClientID   string   // base64 (3 байта) → reserved
    DeviceID   string
    Token      string   // Bearer для PATCH (license/masque)
    AccountID  string
    Endpoint   string   // host:port (дефолт или рандом из пула)
    License    string   // WARP+ (опц.)
    WarpPlus   bool
    AWG        map[string]any // nil = plain WG; иначе AWG-поля обфускации
    CreatedAt  string
}

type RegisterOptions struct {
    LicenseKey    string        // опц., WARP+
    Endpoint      string        // "" = дефолт; иначе уважается (не перезатирается)
    Obfuscate     bool          // AmneziaWG
    Quic          QuicParams    // ip/id/ib + jc/jmin/jmax (при Obfuscate)
    RandomEndpoint bool         // при Obfuscate+дефолт → рандом из пула
    Now           time.Time     // для tos (детерминизм в тестах)
    Rand          *rand.Rand    // инъекция для тестов
}
```

`Account.ToWireguardURI()` строит `wireguard://<priv>@<host>:<port>?publickey=…&address=…
&allowedips=0.0.0.0/0,::/0&reserved=b0,b1,b2&mtu=1280[&jc=…&jmin=…&…&ip=…&id=…&ib=…]#🔥☁️ WARP`.

## 5. Файлы

| Файл | Что |
|------|-----|
| `core/warp/account.go` | `Account`, `ToWireguardURI`, `parseReserved` |
| `core/warp/client.go` | `Client`, `Register`, `ApplyLicense`, API-константы (версия в одном месте) |
| `core/warp/obfuscation.go` | AWG-пресет, `QuicParams`, `buildAWGFields` |
| `core/warp/endpoints.go` | пул prefixes/ports/sni + `RandomEndpoint(rand)` |
| `core/warp/*_test.go` | unit-тесты (fixtures, seeded rand) |
| `core/config/subscription/node_parser_wireguard.go` | +`reserved` query → `peer.reserved` |
| `core/config/subscription/shareuri_wireguard.go` | +encode `reserved` (round-trip) |

Сетевой клиент — через `core/network_utils` таймауты + `HTTP_PROXY`-совместимый клиент
(SPEC 034, `internal`/`core`), пароли/токены маскируются в логах (`internal/urlredact`).

## 6. Безопасность / приватность (CONSTITUTION §6)

- Приватный ключ генерится локально, **в Cloudflare уходит только pubkey**. Никаких
  сторонних воркеров-генераторов.
- Токен/приватник — `secret`, не логируются (маскировка), не попадают в share-URI-логи.
- Регистрация — исходящий вызов только к `api.cloudflareclient.com` (официальный API).
- Не деанонимизирует: аккаунт анонимный (install_id пуст), никакой привязки к пользователю.

## 7. Критерии приёмки

- [ ] `Register` регистрирует аккаунт, `ToWireguardURI` → парсер даёт валидный WG-endpoint.
- [ ] `reserved` из `client_id` доезжает в `peer.reserved` и в собранный config.
- [ ] Obfuscate=true добавляет AWG-поля; MTU клампится ≤1280; узел проходит `sing-box check`.
- [ ] Рандомный endpoint при Obfuscate+дефолт; кастомный endpoint уважается.
- [ ] `go build ./core/...`, `go test ./core/warp/... ./core/config/subscription/...`,
      `go vet` — зелёные.
- [ ] Токены/ключи не в логах.

## 8. Риски

- Cloudflare периодически бампает версию API → версия/заголовок вынесены в одну константу,
  комментарий «сверять с wgcf перед релизом».
- Первый запуск в РФ: `api.cloudflareclient.com` может быть недоступен напрямую — регистрацию
  разрешить через уже активный туннель (клиент уважает системный/HTTP-прокси).

## 9. MASQUE — почему не здесь

MASQUE-outbound (`type:"masque"`, CONNECT-IP/RFC 9484, WARP) в ядре **появился только в
rc.21+/lx.3**. Пин лаунчера = **rc.17**, где `protocol/masque/` отсутствует. Поэтому:
- эмит `masque://` в лаунчере — за флагом/заготовкой, не активен на rc.17;
- запрос на ядро оформлен отдельной спекой в репо `sing-box-lx`
  (бамп пина rc.17→lx.3 + emit-контракт). См. `SPECS/CORE-MASQUE` там.
