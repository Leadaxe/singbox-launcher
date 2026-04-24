# Реализация: 044 — NAIVE_PROXY_PARSER

**Статус:** реализовано в одном коммите. Ветка `develop`, 2026-04-24.

## Что сделано

### Parser (`core/config/subscription/node_parser.go`)

- `IsDirectLink`: +`naive+https://` и +`naive+quic://` (оба возвращают true; plain `naive://` без суффикса — false).
- `ParseNode` dispatch: новый case для `naive+https://` / `naive+quic://` — нормализует URI-схему к `https://` для стандартного `url.Parse`, выставляет `scheme = "naive"`, `defaultPort = 443`.
- Post-parse блок для `scheme == "naive"`:
  - Если исходный URI начинался с `naive+quic://` → `node.Query["quic"] = "true"`.
  - Если в query есть `padding=...` → warning + `node.Query.Del("padding")` (чтобы не утёк в outbound как unknown option).
- Password-extraction: `scheme == "naive"` добавлен в условие (вместе с ssh / trojan / socks / socks5).
- Label-from-user fallback: `scheme != "naive"` исключение (иначе `naive+quic://manhole:114514@host` устанавливал Label="manhole" — имя пользователя как имя ноды).
- `buildOutbound` dispatch: новая ветка `} else if node.Scheme == "naive" { buildNaiveOutbound(node, outbound) }`.

### Naive-specific helpers (`core/config/subscription/node_parser_naive.go` — новый файл, 110 LOC)

- `naiveHeaderNameCharset` — lookup map из allowed-характеров имён HTTP-заголовков (по спеке DuckSoft).
- `isValidNaiveHeaderName(s string) bool` — charset-валидатор.
- `parseNaiveExtraHeaders(s string) map[string]string` — парсит `"Header1: Value1\r\nHeader2: Value2"`, skip'ает невалидные пары с warning, сохраняя остальные.
- `buildNaiveOutbound(node, outbound)`:
  - `username` / `password` — из `node.UUID` / `node.Query["password"]`.
  - `quic: true` + `quic_congestion_control: "bbr"` для QUIC-варианта.
  - `extra_headers: {H: V, ...}` — распаршенная map'а.
  - `tls: {enabled: true, server_name: <host>}` — всегда (naive без TLS не имеет смысла).
  - Намеренно НЕ эмитит `alpn / utls / reality / min_version` в TLS-блоке — sing-box naive их не поддерживает.

### Share URI encoder (`core/config/subscription/share_uri_encode.go`)

- `ShareURIFromOutbound` switch: добавлен `case "naive"` → `shareURIFromNaive(out)`.
- `shareURIFromNaive(out)` — обратная сборка:
  - `naive+https` / `naive+quic` в зависимости от `out["quic"]`.
  - Userinfo: user+pass / password-only (ложится в user-slot) / anonymous.
  - `extra-headers`: ключи сортируются лексикографически (детерминированный round-trip); pairs склеиваются `\r\n`; значения с CR/LF/NUL skip'аются (defensive — теоретически не должно случиться, т.к. парсер такие уже отклонил).
  - `padding` в URI **не восстанавливается** (by design — нет поля в outbound).
  - Fragment — из `out["tag"]` через существующий `fragmentFromTag`.

### Тесты

**`core/config/subscription/node_parser_naive_test.go`** — новый файл, 22 теста:

- `TestIsValidNaiveHeaderName` — 12 sub-cases на charset.
- `TestParseNaiveExtraHeaders` — 9 sub-cases (валидные / невалидные пары, UTF-8 в values, trim spaces).
- `TestParseNode_Naive_Canonical` — 4 примера из DuckSoft-спеки.
- `TestParseNode_Naive_DefaultPort` / `_CustomPort`.
- `TestParseNode_Naive_PasswordOnly` — password в user-slot.
- `TestParseNode_Naive_Anonymous` — без userinfo.
- `TestParseNode_Naive_FragmentUTF8` — `%E2%9C%85 JP-01`.
- `TestParseNode_Naive_InvalidSchemeRejected` — `naive://` отклоняется.
- `TestParseNode_Naive_MaxURILength` — > 8 KB → error.
- `TestBuildOutbound_Naive_{HTTPS,QUIC,WithExtraHeaders,Anonymous}` — JSON-shape с проверкой запрещённых TLS-полей.
- `TestShareURIRoundtrip_Naive_{HTTPS,QUIC,ExtraHeaders}` — URI → ParseNode → buildOutbound → ShareURI.
- `TestShareURIFromNaive_EmptyServer` — `ErrShareURINotSupported`.
- `TestShareURIFromNaive_PaddingDroppedOnRoundtrip` — by-design loss.

**`core/config/subscription/node_parser_test.go TestIsDirectLink`** — добавлены 3 case'а для naive URIs.

### Документация

- `docs/ParserConfig.md`:
  - §Назначение — добавлен `NaïveProxy` в список поддерживаемых протоколов.
  - Таблица «Поддерживаемые типы `outbound.type`» — +строка `naive`.
  - Новая подсекция **«NaïveProxy (`naive+https://` / `naive+quic://`)»** (60+ строк): schema, query-params, examples, output JSON, TLS-ограничения, share-URI round-trip, ссылки на код и спеку.
- `docs/release_notes/upcoming.md` — запись в EN + RU «Highlights».
- `README.md` — строка «Supports multiple subscription URLs and direct links …» расширена.
- `README_RU.md` — аналогично.
- Спека `SPECS/044-F-C-NAIVE_PROXY_PARSER/{SPEC,PLAN,TASKS,IMPLEMENTATION_REPORT}.md`.

## Что не сделано (в рамках TODO на будущее)

- **sing-box binary feature-probe.** При первом Update вызывать `sing-box check` на тестовом конфиге с naive и при ошибке `unknown outbound type "naive"` показывать warning в UI. Сейчас — только документирование требования в `docs/ParserConfig.md` и release-notes.
- **Custom SNI** через `sni=` query-param (`tls.server_name` ≠ host). В v1 `tls.server_name` всегда = `server`.
- **Live-tests** — намеренно пропущены (нет публичных anonymous naive-серверов).

## Проверки

- `go build ./...` — зелёный.
- `go vet ./...` — чистый.
- `go test -race ./core/config/subscription/` — все тесты (новые + существующие) зелёные.
- `go test -race $(go list ./... | grep -v /ui/ | grep -v fyne.io)` — full green.
- Bu-фиксы в соседних местах: `node_parser.go` label-from-user fallback теперь исключает `"naive"` (как исключает `"hysteria2"`), иначе `naive+quic://manhole:114514@host` использовал бы `manhole` как имя ноды.

## Связи

- **SPEC 011** (wake-from-sleep) — не затрагивается; naive outbound использует тот же platform.PowerContext.
- **SPEC 040** (wizard vars с `{title, value}`) — не зависят; naive-ноды идут через обычный node-parser, не через wizard-шаблонные vars.
- **SPEC 037** (subscription toggle) — наивные ноды тоже корректно отключаются переключателем источника.
