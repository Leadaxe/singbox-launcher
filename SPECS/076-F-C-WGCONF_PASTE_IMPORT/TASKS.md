# TASKS 076 — Вставка голого WG/AWG `.conf`-текста

- [x] `core/config/subscription/wgconf_text.go`: `ExtractWGConfBlocks` + `ConvertWGConfText` (label = хост Endpoint)
- [x] `ui/configurator/business/parser.go`: блоки выделяются в `classifyInputLines` до построчного разбора
- [x] Тесты: разрезка (0/1/2 блока, текст до блока), AWG-конвертация end-to-end через `ParseNode`, IPv6-endpoint, невалидный блок; + смешанная вставка в `parser_wgconf_test.go` (business)
- [x] `go build ./... && go test ./... && go vet ./...`
- [x] `docs/ParserConfig.md` + `docs/release_notes/upcoming.md`
- [x] `IMPLEMENTATION_REPORT.md`
