# Задачи: 044 — NAIVE_PROXY_PARSER

## Этап 1 — parser

- [ ] `IsDirectLink`: +`naive+https://`, +`naive+quic://` в `node_parser.go`.
- [ ] `ParseNode` dispatch: case для naive+https / naive+quic, normalization URI схемы к https:// для `url.Parse`.
- [ ] `ParsedNode` заполнение: Scheme="naive", Server, Port (default 443), UUID=username, Query["password"]=password, Query["quic"]=true если quic-variant.
- [ ] Warning + remove padding query-param.
- [ ] Label из fragment с URL-decode + UTF-8 fix.
- [ ] Новый файл `node_parser_naive.go`:
  - [ ] `parseNaiveExtraHeaders(s string) map[string]string` — split на `\r\n`, валидация charset'а header name.
  - [ ] `isValidNaiveHeaderName(s string) bool` — по charset'у спеки.
  - [ ] `buildNaiveOutbound(node, outbound)` — заполнение outbound map'а для naive.

## Этап 2 — buildOutbound dispatch

- [ ] В `buildOutbound` (после `scheme == "ssh"`): `} else if node.Scheme == "naive" { buildNaiveOutbound(node, outbound) }`.
- [ ] Убедиться что `outbound["type"] = node.Scheme` поставит `"naive"` корректно (текущий switch).

## Этап 3 — share URI encoder

- [ ] `ShareURIFromOutbound` switch: case "naive" → `shareURIFromNaive`.
- [ ] `shareURIFromNaive(out)` — scheme, userinfo, host:port, sorted extra-headers, fragment.
- [ ] Проверка, что `url.URL.String()` корректно экранирует `\r\n` → `%0D%0A`.

## Этап 4 — тесты

- [ ] `core/config/subscription/node_parser_naive_test.go`:
  - [ ] `TestParseNode_Naive_Canonical` (4 примера из DuckSoft спеки).
  - [ ] `TestParseNode_Naive_DefaultPort`.
  - [ ] `TestParseNode_Naive_OnlyPassword`.
  - [ ] `TestParseNode_Naive_NoAuth`.
  - [ ] `TestParseNode_Naive_QUIC`.
  - [ ] `TestParseNode_Naive_ExtraHeaders` (включая edge: empty value, UTF-8 в value).
  - [ ] `TestParseNode_Naive_PaddingIgnored`.
  - [ ] `TestParseNode_Naive_InvalidScheme` (`naive://` без `+`).
  - [ ] `TestParseNode_Naive_EmptyHost`.
  - [ ] `TestParseNode_Naive_FragmentUTF8` (`%E2%9C%85 JP-01`).
  - [ ] `TestParseNode_Naive_MaxURILength` (> 8 KB → error).
  - [ ] `TestBuildOutbound_Naive_HTTPS` — JSON shape + TLS-block.
  - [ ] `TestBuildOutbound_Naive_QUIC` — +quic:true + congestion.
  - [ ] `TestBuildOutbound_Naive_WithHeaders` — map structure.
  - [ ] `TestBuildOutbound_Naive_NoAuth` — без username/password.
  - [ ] `TestParseNaiveExtraHeaders_ValidAndInvalid` — на helper'е.
  - [ ] `TestIsValidNaiveHeaderName`.

- [ ] В `share_uri_encode_test.go`:
  - [ ] `TestShareURIFromNaive_HTTPS`.
  - [ ] `TestShareURIFromNaive_QUIC`.
  - [ ] `TestShareURIFromNaive_ExtraHeaders` — sorted keys, `%0D%0A`.
  - [ ] `TestShareURIFromNaive_NoAuth`.
  - [ ] `TestShareURIFromNaive_EmptyServer` — `ErrShareURINotSupported`.
  - [ ] `TestShareURIRoundtrip_Naive` — `URI → ParseNode → buildOutbound → ShareURI` равенство (с оговорками §4.2).

- [ ] В `node_parser_test.go` `TestIsDirectLink`:
  - [ ] Case для `naive+https://host` → true.
  - [ ] Case для `naive+quic://host` → true.
  - [ ] Case для `naive://host` (без +) → false.

## Этап 5 — документация

- [ ] `docs/ParserConfig.md` — раздел NaïveProxy.
- [ ] `docs/release_notes/upcoming.md` — EN + RU запись.
- [ ] `README.md` и `README_RU.md` — в списке supported protocols.

## Этап 6 — проверки

- [ ] `go build ./...`.
- [ ] `go vet ./...`.
- [ ] `go test -race ./core/config/subscription/` — все зелёные.
- [ ] `go test -race ./...` — все зелёные.
- [ ] Ручной smoke: подписка с реальной naive-нодой → парсится → `config.json` содержит `{"type":"naive", ...}` → `sing-box check` не падает.

## Этап 7 — post-ship (TODO для следующей итерации)

- [ ] sing-box binary feature-probe (§5.4 SPEC). Добавить `StateService.SingboxFeaturesSupported`, проверять на startup.
- [ ] Custom `sni=` query-param support (§9.2).
- [ ] Update IMPLEMENTATION_REPORT.md после merge.
