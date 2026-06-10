# TASKS — 074 TUIC_PROTOCOL

## Парсинг
- [x] `IsDirectLink` += `tuic://`
- [x] `case "tuic"` в `ParseNode` (defaultPort 443)
- [x] `tuic` в проверку userinfo (host + uuid обязательны) и в извлечение password из userinfo
- [x] `node_parser_tuic.go`: `buildTuicOutbound` + `buildTuicTLS` + хелперы
- [x] ветка `tuic` в `buildOutbound`

## Генерация конфига
- [x] ветка `node.Scheme == "tuic"` в `GenerateNodeJSON` (uuid/password/cc/udp_relay_mode/zero_rtt/heartbeat)
- [x] TLS через общий блок (проверено: `tls` выводится)

## Share-URI
- [x] `shareuri_tuic.go`: `shareURIFromTuic`
- [x] `case "tuic"` в `share_uri.go`

## Документация
- [x] `ParserConfig.md`: строка #10 TUIC, убрать из «не поддерживаются», share-таблица, счётчики схем, комментарий-список
- [x] `upcoming.md`: запись EN + RU

## Тесты и проверки
- [x] `node_parser_tuic_test.go` (9 кейсов): parse / buildOutbound / round-trip
- [x] `generator_tuic_test.go`: URI → `GenerateNodeJSON` → валидный JSON
- [x] `go build ./...`, `go test ./core/config/...`, `go vet ./core/...` — зелёные
- [x] `sing-box check` на сгенерированном конфиге (фейк-креды, `/tmp`, удалён) — exit 0

## Закрытие
- [x] IMPLEMENTATION_REPORT.md
- [x] папка → `074-F-C-TUIC_PROTOCOL`
