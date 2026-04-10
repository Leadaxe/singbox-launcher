# Задачи: 036 — XRAY_JUMP_ANY_PROTOCOL

## Этап 0 — подготовка

- [x] Утвердить модель (`ParsedJump` + `Scheme` / `UUID` / `Flow`) и первый протокол не-SOCKS (**VLESS**).

## Этап 1 — парсер

- [x] Диспетчер по `protocol` для outbound, на который указывает `dialerProxy`.
- [x] Маппинг **VLESS** jump из Xray JSON.
- [x] Сохранить поведение для SOCKS без регрессий.

## Этап 2 — генерация

- [x] `GenerateOutboundsFromParserConfig`: hop по `Jump.Scheme` (vless / socks).
- [x] `applyTagsToXrayNode`: синхронизация `Jump.Outbound["tag"]`.

## Этап 3 — тесты и доки

- [x] Тесты VLESS jump и неподдерживаемый протокол (trojan).
- [x] `docs/ParserConfig.md`, `docs/release_notes/upcoming.md`.
- [x] `go test ./...`, `go vet ./...`, `go build ./...`

## Этап 4 — закрытие

- [x] **IMPLEMENTATION_REPORT.md**; папка **`036-F-C-XRAY_JUMP_ANY_PROTOCOL`**.
